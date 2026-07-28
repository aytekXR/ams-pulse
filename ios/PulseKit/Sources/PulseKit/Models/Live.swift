import Foundation

// MARK: - LiveOverview

/// Real-time dashboard snapshot: viewers, publishers, protocol mix, per-app stats.
/// Spec reference: lines 1886-1906 (`LiveOverview` schema).
///
/// Data comes from in-memory aggregates (not ClickHouse), updated every poll cycle
/// (~10s lag per PRD F1).
public struct LiveOverview: Decodable, Sendable, Equatable {
    /// Snapshot timestamp (Unix epoch ms).
    public let ts: Int64

    /// Current total concurrent viewers across all streams.
    public let totalViewers: Int

    /// Current total active publishers.
    public let totalPublishers: Int

    /// Viewer counts broken down by delivery protocol.
    public let protocolMix: ProtocolMix

    /// Per-application overview stats.
    /// Decodes as empty array if null or missing (older server compat).
    public let apps: [AppOverview]

    /// Per-node health status.
    /// Decodes as empty array if null or missing (older server compat).
    public let nodes: [NodeHealth]

    enum CodingKeys: String, CodingKey {
        case ts
        case totalViewers = "total_viewers"
        case totalPublishers = "total_publishers"
        case protocolMix = "protocol_mix"
        case apps
        case nodes
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        ts = try container.decode(Int64.self, forKey: .ts)
        totalViewers = try container.decode(Int.self, forKey: .totalViewers)
        totalPublishers = try container.decode(Int.self, forKey: .totalPublishers)
        protocolMix = try container.decode(ProtocolMix.self, forKey: .protocolMix)
        apps = try container.decodeArrayOrEmpty([AppOverview].self, forKey: .apps)
        nodes = try container.decodeArrayOrEmpty([NodeHealth].self, forKey: .nodes)
    }
}

// MARK: - ProtocolMix

/// Viewer counts per delivery protocol.
/// Spec reference: lines 1908-1926 (`ProtocolMix` schema).
///
/// All properties are optional in the spec with `default: 0`. We use non-optional `Int`
/// with a custom decoder that applies the defaults, per the contract.
public struct ProtocolMix: Decodable, Sendable, Equatable, Hashable {
    public let webrtc: Int
    public let hls: Int
    public let rtmp: Int
    public let dash: Int
    public let other: Int

    public init(
        webrtc: Int = 0,
        hls: Int = 0,
        rtmp: Int = 0,
        dash: Int = 0,
        other: Int = 0
    ) {
        self.webrtc = webrtc
        self.hls = hls
        self.rtmp = rtmp
        self.dash = dash
        self.other = other
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        webrtc = try container.decodeIfPresent(Int.self, forKey: .webrtc) ?? 0
        hls = try container.decodeIfPresent(Int.self, forKey: .hls) ?? 0
        rtmp = try container.decodeIfPresent(Int.self, forKey: .rtmp) ?? 0
        dash = try container.decodeIfPresent(Int.self, forKey: .dash) ?? 0
        other = try container.decodeIfPresent(Int.self, forKey: .other) ?? 0
    }

    enum CodingKeys: String, CodingKey {
        case webrtc, hls, rtmp, dash, other
    }
}

// MARK: - AppOverview

/// Per-application summary in the live overview.
/// Spec reference: lines 1928-1939 (`AppOverview` schema).
///
/// Wire keys match Swift property names (camelCase in spec).
public struct AppOverview: Decodable, Sendable, Equatable, Hashable {
    /// AMS application name.
    public let app: String

    /// Current viewer count for this app.
    public let viewers: Int

    /// Current publisher count for this app.
    public let publishers: Int

    /// Number of active streams in this app.
    public let streams: Int
}

// MARK: - NodeHealth

/// Per-node health in the live overview.
/// Spec reference: lines 1941-1965 (`NodeHealth` schema).
///
/// D-090: "down" removed from status enum — AMS has no native transient-down state.
/// Lifecycle is up -> degraded (ConsecAPIErrors >= 3) -> evicted (node disappears).
public struct NodeHealth: Decodable, Sendable, Equatable, Hashable {
    public let nodeId: String
    public let role: NodeRole?
    public let status: NodeStatus
    public let lastSeen: Int64?
    public let cpuPct: Double?
    public let memPct: Double?
    public let version: String?

    enum CodingKeys: String, CodingKey {
        case nodeId = "node_id"
        case role
        case status
        case lastSeen = "last_seen"
        case cpuPct = "cpu_pct"
        case memPct = "mem_pct"
        case version
    }
}

// MARK: - NodeRole

/// Node role in the AMS cluster.
/// Spec reference: line 1949.
///
/// ## Unknown Value Handling
/// If the server sends an unrecognized role (e.g., a value added in a newer server version),
/// it decodes to `.unknown(rawValue)` rather than throwing. This ensures the app remains
/// functional against newer servers.
///
/// **Trade-off:** Adding `.unknown` removes compile-time exhaustiveness for new known cases.
/// To find all switch sites when adding a case: temporarily remove `@unknown default`, build,
/// and fix all flagged sites.
public enum NodeRole: ResilientRawRepresentable {
    case origin
    case edge
    case standalone
    case unknown(String)

    public var rawValue: String {
        switch self {
        case .origin: return "origin"
        case .edge: return "edge"
        case .standalone: return "standalone"
        case .unknown(let v): return v
        }
    }

    public init(rawValue: String) {
        switch rawValue {
        case "origin": self = .origin
        case "edge": self = .edge
        case "standalone": self = .standalone
        default: self = .unknown(rawValue)
        }
    }
}

// MARK: - NodeStatus

/// Node health status.
/// Spec reference: line 1952-1954.
///
/// D-090: "down" removed — AMS evicts nodes instead of marking them down.
///
/// ## Unknown Value Handling
/// If the server sends an unrecognized status, it decodes to `.unknown(rawValue)`.
/// See `NodeRole` for the trade-off discussion.
public enum NodeStatus: ResilientRawRepresentable {
    case up
    case degraded
    case unknown(String)

    public var rawValue: String {
        switch self {
        case .up: return "up"
        case .degraded: return "degraded"
        case .unknown(let v): return v
        }
    }

    public init(rawValue: String) {
        switch rawValue {
        case "up": self = .up
        case "degraded": self = .degraded
        default: self = .unknown(rawValue)
        }
    }
}

// MARK: - LiveStreamList

/// Paginated list of currently active streams.
/// Spec reference: lines 1967-1976 (`LiveStreamList` schema).
public struct LiveStreamList: Decodable, Sendable, Equatable {
    /// Active streams. Decodes as empty array if null or missing (older server compat).
    public let items: [LiveStream]
    public let meta: PaginatedMeta

    enum CodingKeys: String, CodingKey {
        case items
        case meta
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        items = try container.decodeArrayOrEmpty([LiveStream].self, forKey: .items)
        meta = try container.decode(PaginatedMeta.self, forKey: .meta)
    }
}

// MARK: - LiveStream

/// A single active stream with per-stream metrics.
/// Spec reference: lines 1978-2024 (`LiveStream` schema).
///
/// Note: Identifiable conformance uses `streamId` as the identity.
public struct LiveStream: Decodable, Sendable, Equatable, Hashable, Identifiable {
    public var id: String { streamId }

    public let streamId: String
    public let app: String
    public let nodeId: String?
    public let tenant: String?
    public let viewers: Int
    public let publisherState: PublisherState
    public let healthScore: Double
    public let protocolMix: ProtocolMix?
    public let bitrateKbps: Double?
    public let startedAt: Int64?
    public let viewerRttMs: Double?
    public let viewerJitterMs: Double?
    public let viewerLossPct: Double?

    enum CodingKeys: String, CodingKey {
        case streamId = "stream_id"
        case app
        case nodeId = "node_id"
        case tenant
        case viewers
        case publisherState = "publisher_state"
        case healthScore = "health_score"
        case protocolMix = "protocol_mix"
        case bitrateKbps = "bitrate_kbps"
        case startedAt = "started_at"
        case viewerRttMs = "viewer_rtt_ms"
        case viewerJitterMs = "viewer_jitter_ms"
        case viewerLossPct = "viewer_loss_pct"
    }
}

// MARK: - PublisherState

/// Publisher state for a live stream.
/// Spec reference: line 1998.
///
/// ## Unknown Value Handling
/// If the server sends an unrecognized state (e.g., "reconnecting" added in a future version),
/// it decodes to `.unknown(rawValue)`. See `NodeRole` for the trade-off discussion.
public enum PublisherState: ResilientRawRepresentable {
    case publishing
    case idle
    case offline
    case unknown(String)

    public var rawValue: String {
        switch self {
        case .publishing: return "publishing"
        case .idle: return "idle"
        case .offline: return "offline"
        case .unknown(let v): return v
        }
    }

    public init(rawValue: String) {
        switch rawValue {
        case "publishing": self = .publishing
        case "idle": self = .idle
        case "offline": self = .offline
        default: self = .unknown(rawValue)
        }
    }
}
