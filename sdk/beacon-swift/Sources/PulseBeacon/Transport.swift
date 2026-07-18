import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking  // URLSession/URLRequest live here on Linux
#endif

/// Performs the HTTP POST of a batch. Abstracted so tests can assert the request
/// (URL, headers, body) without a real network, and so an app can supply a custom
/// `URLSession` (e.g. a background session on iOS). Mirrors the JS SDK's fetch path.
public protocol BeaconSender {
    /// Deliver `body` to `url` with `headers`; call `completion(true)` on a 2xx
    /// response, `completion(false)` on any error or non-2xx status.
    func send(url: URL, headers: [String: String], body: Data, completion: @escaping (Bool) -> Void)
}

/// Default `BeaconSender` backed by `URLSession`.
public final class URLSessionSender: BeaconSender {
    private let session: URLSession

    public init(session: URLSession = .shared) {
        self.session = session
    }

    public func send(url: URL, headers: [String: String], body: Data, completion: @escaping (Bool) -> Void) {
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.httpBody = body
        for (key, value) in headers {
            request.setValue(value, forHTTPHeaderField: key)
        }
        session.dataTask(with: request) { _, response, error in
            if error != nil {
                completion(false)
                return
            }
            if let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) {
                completion(true)
            } else {
                completion(false)
            }
        }.resume()
    }
}

/// Batches events (flush at ≤10 s or 25 events, or on demand), POSTs them to
/// `<ingestURL>/ingest/beacon` with the `X-Pulse-Ingest-Token` header, and keeps a
/// bounded retry queue with exponential backoff. All mutable state is confined to a
/// single serial queue, so it is safe to `push` from any thread. Mirrors
/// `sdk/beacon-js/src/transport.ts`; never throws.
final class Transport {
    struct Config {
        var ingestURL: String
        var token: String
        var sessionID: String
        var streamID: String
        var app: String?
        var meta: [String: String]?
        var playerKind: PlayerKind
        var sdkVersion: String
    }

    static let maxBatchSize = 25
    static let flushIntervalMs = 10_000
    static let maxQueueDepth = 100
    static let backoffBaseMs = 1_000
    static let backoffCapMs = 60_000

    private let cfg: Config
    private let sender: BeaconSender
    private let queue = DispatchQueue(label: "dev.pulse.beacon.transport")
    private var buffer: [BeaconEventItem] = []
    private var retryQueue: [BeaconBatch] = []
    private var backoffMs = Transport.backoffBaseMs
    private var retryPending = false
    private var destroyed = false
    private var flushTimer: DispatchSourceTimer?

    /// - Parameter startTimer: start the periodic ≤10 s flush timer. Tests pass
    ///   `false` and drive flushes explicitly for determinism.
    init(cfg: Config, sender: BeaconSender, startTimer: Bool = true) {
        self.cfg = cfg
        self.sender = sender
        if startTimer {
            queue.async { self.startFlushTimerLocked() }
        }
    }

    /// Enqueue an event; flush immediately once the batch is full.
    func push(_ item: BeaconEventItem) {
        queue.async {
            guard !self.destroyed else { return }
            self.buffer.append(item)
            if self.buffer.count >= Transport.maxBatchSize {
                self.flushLocked()
            }
        }
    }

    /// Flush the current buffer (no-op if empty).
    func flush() {
        queue.async { self.flushLocked() }
    }

    /// Flush and tear down. Idempotent.
    func dispose() {
        queue.sync {
            guard !self.destroyed else { return }
            self.destroyed = true
            self.flushLocked()
            self.flushTimer?.cancel()
            self.flushTimer = nil
        }
    }

    /// Test/synchronization helper: block until all queued work has drained.
    func drain() {
        queue.sync {}
    }

    /// Test hook: current retry-queue depth (synchronized).
    var retryQueueDepthForTesting: Int {
        queue.sync { retryQueue.count }
    }

    // MARK: - Internal (run on `queue`)

    private func startFlushTimerLocked() {
        let timer = DispatchSource.makeTimerSource(queue: queue)
        let interval = DispatchTimeInterval.milliseconds(Transport.flushIntervalMs)
        timer.schedule(deadline: .now() + interval, repeating: interval)
        timer.setEventHandler { [weak self] in
            self?.flushLocked()
        }
        timer.resume()
        flushTimer = timer
    }

    private func flushLocked() {
        guard !buffer.isEmpty else { return }
        let events = buffer
        buffer.removeAll()
        send(buildBatch(events))
    }

    private func buildBatch(_ events: [BeaconEventItem]) -> BeaconBatch {
        BeaconBatch(
            sessionId: cfg.sessionID,
            streamId: cfg.streamID,
            app: cfg.app,
            meta: cfg.meta,
            player: PlayerInfo(kind: cfg.playerKind.rawValue, sdkVersion: cfg.sdkVersion),
            events: events
        )
    }

    private func send(_ batch: BeaconBatch) {
        guard let url = URL(string: cfg.ingestURL + "/ingest/beacon"),
              let body = try? BeaconCoding.encoder.encode(batch) else { return }
        let headers = [
            "Content-Type": "application/json",
            "X-Pulse-Ingest-Token": cfg.token,
        ]
        sender.send(url: url, headers: headers, body: body) { [weak self] ok in
            guard let self = self else { return }
            self.queue.async {
                if ok {
                    self.backoffMs = Transport.backoffBaseMs
                } else {
                    self.enqueueRetryLocked(batch)
                }
            }
        }
    }

    private func enqueueRetryLocked(_ batch: BeaconBatch) {
        guard !destroyed else { return }
        if retryQueue.count >= Transport.maxQueueDepth {
            retryQueue.removeFirst()  // drop-oldest at cap
        }
        retryQueue.append(batch)
        scheduleRetryLocked()
    }

    private func scheduleRetryLocked() {
        guard !destroyed, !retryPending, !retryQueue.isEmpty else { return }
        retryPending = true
        let delay = DispatchTimeInterval.milliseconds(backoffMs)
        backoffMs = min(backoffMs * 2, Transport.backoffCapMs)
        queue.asyncAfter(deadline: .now() + delay) { [weak self] in
            guard let self = self else { return }
            self.retryPending = false
            self.retryNextLocked()
        }
    }

    private func retryNextLocked() {
        guard !destroyed, !retryQueue.isEmpty else { return }
        let batch = retryQueue.removeFirst()
        send(batch)
        if !retryQueue.isEmpty {
            scheduleRetryLocked()
        }
    }
}
