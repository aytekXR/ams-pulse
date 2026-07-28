import Foundation

// MARK: - FleetNodeList

/// Paginated list of cluster nodes.
/// Spec reference: lines 2707-2716 (`FleetNodeList` schema).
public struct FleetNodeList: Decodable, Sendable, Equatable {
    /// Fleet nodes. Decodes as empty array if null or missing (older server compat).
    public let items: [FleetNode]
    public let meta: PaginatedMeta

    enum CodingKeys: String, CodingKey {
        case items
        case meta
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        items = try container.decodeArrayOrEmpty([FleetNode].self, forKey: .items)
        meta = try container.decode(PaginatedMeta.self, forKey: .meta)
    }
}

// MARK: - FleetNode

/// A cluster node with role, health, and system info.
/// Spec reference: lines 2718-2762 (`FleetNode` schema).
///
/// Nodes are auto-discovered within 2 minutes of first contact (PRD F7).
public struct FleetNode: Decodable, Sendable, Equatable, Hashable, Identifiable {
    public var id: String { nodeId }

    public let nodeId: String
    public let role: NodeRole
    public let status: NodeStatus
    public let lastSeen: Int64
    public let version: String?
    public let cpuPct: Double?
    public let memPct: Double?
    public let netInMbps: Double?
    public let netOutMbps: Double?
    public let osName: String?
    public let osArch: String?
    public let javaVersion: String?
    public let processorCount: Int?

    enum CodingKeys: String, CodingKey {
        case nodeId = "node_id"
        case role
        case status
        case lastSeen = "last_seen"
        case version
        case cpuPct = "cpu_pct"
        case memPct = "mem_pct"
        case netInMbps = "net_in_mbps"
        case netOutMbps = "net_out_mbps"
        case osName = "os_name"
        case osArch = "os_arch"
        case javaVersion = "java_version"
        case processorCount = "processor_count"
    }
}
