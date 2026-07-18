import Foundation

/// SDK version string reported in every batch's `player.sdk_version`.
public let pulseBeaconSDKVersion = "0.1.0"

/// Player kind, matching the schema enum (`player.kind`).
public enum PlayerKind: String, Codable, Equatable {
    case amsWebRTC = "ams-webrtc"
    case hlsJS = "hls.js"
    case videoJS = "video.js"
    case native
    case other
}

/// Beacon event type, matching the schema enum.
public enum BeaconEventType: String, Codable, Equatable {
    case sessionStart = "session_start"
    case startupComplete = "startup_complete"
    case heartbeat
    case rebufferStart = "rebuffer_start"
    case rebufferEnd = "rebuffer_end"
    case error
    case bitrateChange = "bitrate_change"
    case resolutionChange = "resolution_change"
    case sessionEnd = "session_end"
}

/// One event item in a batch. `data` carries the type-specific payload keyed by
/// the schema's snake_case field names.
public struct BeaconEventItem: Codable, Equatable {
    /// Event type (schema enum value, e.g. "heartbeat").
    public var type: String
    /// Client-side Unix epoch milliseconds.
    public var ts: Int
    /// Type-specific payload, or nil when the event carries no data.
    public var data: [String: JSONValue]?

    public init(type: String, ts: Int, data: [String: JSONValue]? = nil) {
        self.type = type
        self.ts = ts
        self.data = data
    }

    public init(type: BeaconEventType, ts: Int, data: [String: JSONValue]? = nil) {
        self.init(type: type.rawValue, ts: ts, data: data)
    }
}

/// Player identification block (`player`).
public struct PlayerInfo: Codable, Equatable {
    public var kind: String
    public var sdkVersion: String

    public init(kind: String, sdkVersion: String) {
        self.kind = kind
        self.sdkVersion = sdkVersion
    }
}

/// The full batch payload POSTed to `/ingest/beacon`. Matches
/// `contracts/events/beacon-event.schema.json` (frozen, D-004). Property names are
/// camelCase and encoded to the schema's snake_case via the encoder's
/// `.convertToSnakeCase` key strategy (see `BeaconCoding`).
public struct BeaconBatch: Codable, Equatable {
    /// Schema version — always 1.
    public let version: Int
    public var sessionId: String
    public var streamId: String
    public var app: String?
    public var meta: [String: String]?
    public var player: PlayerInfo?
    public var events: [BeaconEventItem]

    public init(
        sessionId: String,
        streamId: String,
        app: String? = nil,
        meta: [String: String]? = nil,
        player: PlayerInfo? = nil,
        events: [BeaconEventItem]
    ) {
        self.version = 1
        self.sessionId = sessionId
        self.streamId = streamId
        self.app = app
        self.meta = meta
        self.player = player
        self.events = events
    }
}

/// Shared JSON encoder configured to match the beacon wire format: camelCase Swift
/// properties (`sessionId`, `streamId`, `sdkVersion`) are written as the schema's
/// snake_case keys. The `data` dictionary keys are authored in snake_case by the
/// event helpers and pass through unchanged (already lowercase → idempotent).
///
/// The SDK is encode-only (it POSTs batches; it never reads them back), so no
/// matching decoder is provided — a `.convertFromSnakeCase` decoder would wrongly
/// camelCase the `data` payload keys. Tests assert the wire shape with
/// `JSONSerialization` instead.
public enum BeaconCoding {
    public static var encoder: JSONEncoder {
        let e = JSONEncoder()
        e.keyEncodingStrategy = .convertToSnakeCase
        return e
    }
}
