import Foundation
#if canImport(UIKit)
import UIKit
#endif

/// Player-side QoE telemetry client for Pulse. Create one per viewer session,
/// call the typed event helpers (or `event(_:data:)`) during playback, and
/// `dispose()` when playback ends. Safe to call from any thread. Never throws.
///
/// Mirrors `sdk/beacon-js`'s `PulseBeacon`/`SessionHandle`. The wire payload matches
/// `contracts/events/beacon-event.schema.json` (frozen, D-004).
public final class PulseBeacon {

    /// SDK configuration, mirroring the JS `PulseConfig`.
    public struct Config {
        /// Pulse collector base URL (HTTPS), e.g. `https://pulse.example.com`.
        public var ingestURL: String
        /// Ingest token issued by Pulse.
        public var token: String
        /// Stream identifier matching the AMS stream name.
        public var streamID: String
        /// AMS application name (optional).
        public var app: String?
        /// Customer-supplied string metadata (e.g. a tenant tag for F6).
        public var metadata: [String: String]?
        /// Sampling rate 0…1; default 1 (always report). Decided once per session.
        public var sampleRate: Double
        /// Player kind reported in `player.kind`; default `.native`.
        public var playerKind: PlayerKind

        public init(
            ingestURL: String,
            token: String,
            streamID: String,
            app: String? = nil,
            metadata: [String: String]? = nil,
            sampleRate: Double = 1.0,
            playerKind: PlayerKind = .native
        ) {
            self.ingestURL = ingestURL
            self.token = token
            self.streamID = streamID
            self.app = app
            self.metadata = metadata
            self.sampleRate = sampleRate
            self.playerKind = playerKind
        }
    }

    /// The viewer session UUID (v4) generated at init, used for session stitching.
    public let sessionID: String

    /// Whether this session is sampled (reported). When false, all events are no-ops.
    public let isSampled: Bool

    private let transport: Transport?

    #if canImport(UIKit)
    private var lifecycleObserver: NSObjectProtocol?
    #endif

    /// - Parameters:
    ///   - config: SDK configuration.
    ///   - sender: HTTP sender; defaults to `URLSessionSender()`. Injectable for tests
    ///     or a custom (e.g. background) `URLSession`.
    public init(config: Config, sender: BeaconSender = URLSessionSender()) {
        self.sessionID = Session.generateSessionID()
        self.isSampled = Session.isSampled(config.sampleRate)
        if isSampled {
            let tcfg = Transport.Config(
                ingestURL: config.ingestURL,
                token: config.token,
                sessionID: sessionID,
                streamID: config.streamID,
                app: config.app,
                meta: config.metadata,
                playerKind: config.playerKind,
                sdkVersion: pulseBeaconSDKVersion
            )
            self.transport = Transport(cfg: tcfg, sender: sender)
        } else {
            self.transport = nil
        }
        attachLifecycle()
    }

    // MARK: - Generic event

    /// Emit an event with an optional pre-built `data` payload. Prefer the typed
    /// helpers below, which build the schema's `data` keys for you.
    public func event(_ type: BeaconEventType, data: [String: JSONValue]? = nil) {
        transport?.push(BeaconEventItem(type: type, ts: PulseBeacon.nowMs(), data: data))
    }

    // MARK: - Typed events (build the schema `data` shapes)

    public func sessionStart(pageURL: String? = nil, autoplay: Bool? = nil) {
        emit(.sessionStart, [
            ("page_url", pageURL.map(JSONValue.string)),
            ("autoplay", autoplay.map(JSONValue.bool)),
        ])
    }

    public func startupComplete(startupMs: Int, bitrateKbps: Double? = nil) {
        emit(.startupComplete, [
            ("startup_ms", .int(startupMs)),
            ("bitrate_kbps", bitrateKbps.map(JSONValue.double)),
        ])
    }

    public func heartbeat(watchMs: Int, bitrateKbps: Double? = nil, bufferMs: Int? = nil, droppedFrames: Int? = nil) {
        emit(.heartbeat, [
            ("watch_ms", .int(watchMs)),
            ("bitrate_kbps", bitrateKbps.map(JSONValue.double)),
            ("buffer_ms", bufferMs.map(JSONValue.int)),
            ("dropped_frames", droppedFrames.map(JSONValue.int)),
        ])
    }

    public func rebufferStart(bufferMs: Int? = nil) {
        emit(.rebufferStart, [("buffer_ms", bufferMs.map(JSONValue.int))])
    }

    public func rebufferEnd(durationMs: Int) {
        emit(.rebufferEnd, [("duration_ms", .int(durationMs))])
    }

    public func error(code: String, message: String? = nil, fatal: Bool? = nil) {
        emit(.error, [
            ("code", .string(code)),
            ("message", message.map(JSONValue.string)),
            ("fatal", fatal.map(JSONValue.bool)),
        ])
    }

    public func bitrateChange(fromKbps: Double, toKbps: Double) {
        emit(.bitrateChange, [
            ("from_kbps", .double(fromKbps)),
            ("to_kbps", .double(toKbps)),
        ])
    }

    public func resolutionChange(from: String, to: String) {
        emit(.resolutionChange, [
            ("from", .string(from)),
            ("to", .string(to)),
        ])
    }

    public func sessionEnd(watchMs: Int? = nil, reason: String? = nil) {
        emit(.sessionEnd, [
            ("watch_ms", watchMs.map(JSONValue.int)),
            ("reason", reason.map(JSONValue.string)),
        ])
    }

    // MARK: - Lifecycle

    /// Flush pending events now (also happens automatically every ≤10 s and at 25
    /// buffered events).
    public func flush() {
        transport?.flush()
    }

    /// Emit a final `session_end` (if `reason` given), flush, and tear down. Idempotent.
    public func dispose(reason: String? = nil) {
        if reason != nil {
            sessionEnd(reason: reason)
        }
        detachLifecycle()
        transport?.dispose()
    }

    // MARK: - Internal

    private func emit(_ type: BeaconEventType, _ pairs: [(String, JSONValue?)]) {
        var data: [String: JSONValue] = [:]
        for (key, value) in pairs {
            if let value = value { data[key] = value }
        }
        transport?.push(BeaconEventItem(type: type, ts: PulseBeacon.nowMs(), data: data.isEmpty ? nil : data))
    }

    static func nowMs() -> Int {
        Int(Date().timeIntervalSince1970 * 1000)
    }

    private func attachLifecycle() {
        #if canImport(UIKit)
        // iOS: flush when the app is backgrounded (analog of the JS SDK's
        // visibilitychange/pagehide flush). Verified on Xcode/CI only — this branch
        // compiles out on Linux, where the SDK core is unit-tested.
        lifecycleObserver = NotificationCenter.default.addObserver(
            forName: UIApplication.didEnterBackgroundNotification,
            object: nil,
            queue: nil
        ) { [weak self] _ in
            self?.transport?.flush()
        }
        #endif
    }

    private func detachLifecycle() {
        #if canImport(UIKit)
        if let observer = lifecycleObserver {
            NotificationCenter.default.removeObserver(observer)
        }
        lifecycleObserver = nil
        #endif
    }
}
