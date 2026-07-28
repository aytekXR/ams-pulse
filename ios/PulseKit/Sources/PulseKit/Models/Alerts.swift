import Foundation

// MARK: - AlertHistoryList

/// Paginated list of alert firing history.
/// Spec reference: lines 2542-2551 (`AlertHistoryList` schema).
public struct AlertHistoryList: Decodable, Sendable, Equatable {
    /// Alert entries. Decodes as empty array if null or missing (older server compat).
    public let items: [AlertHistoryEntry]
    public let meta: PaginatedMeta

    enum CodingKeys: String, CodingKey {
        case items
        case meta
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        items = try container.decodeArrayOrEmpty([AlertHistoryEntry].self, forKey: .items)
        meta = try container.decode(PaginatedMeta.self, forKey: .meta)
    }
}

// MARK: - AlertHistoryEntry

/// A single alert firing record.
/// Spec reference: lines 2553-2585 (`AlertHistoryEntry` schema).
///
/// Note: The wire field is `id`, but Swift's `Identifiable` requires an `id` property.
/// We use `alertId` as the CodingKey target and a computed `var id` that returns `alertId`.
public struct AlertHistoryEntry: Decodable, Sendable, Equatable, Hashable, Identifiable {
    public var id: String { alertId }

    public let alertId: String
    public let ruleId: String
    public let state: AlertState
    public let severity: AlertSeverity
    public let ts: Int64
    public let metric: String
    public let value: Double
    public let threshold: Double
    public let scope: AlertScope?
    public let cooldownUntil: Int64?
    public let groupKey: String?

    enum CodingKeys: String, CodingKey {
        case alertId = "id"
        case ruleId = "rule_id"
        case state
        case severity
        case ts
        case metric
        case value
        case threshold
        case scope
        case cooldownUntil = "cooldown_until"
        case groupKey = "group_key"
    }
}

// MARK: - AlertState

/// Alert firing state.
/// Spec reference: line 2563.
///
/// ## Unknown Value Handling
/// If the server sends an unrecognized state (e.g., "acknowledged" added in a future version),
/// it decodes to `.unknown(rawValue)`. See `NodeRole` in Live.swift for the trade-off discussion.
public enum AlertState: ResilientRawRepresentable {
    case firing
    case resolved
    case deliveryFailure
    case unknown(String)

    public var rawValue: String {
        switch self {
        case .firing: return "firing"
        case .resolved: return "resolved"
        case .deliveryFailure: return "delivery_failure"
        case .unknown(let v): return v
        }
    }

    public init(rawValue: String) {
        switch rawValue {
        case "firing": self = .firing
        case "resolved": self = .resolved
        case "delivery_failure": self = .deliveryFailure
        default: self = .unknown(rawValue)
        }
    }
}

// MARK: - AlertSeverity

/// Alert severity level.
/// Spec reference: line 2566.
///
/// ## Unknown Value Handling
/// If the server sends an unrecognized severity, it decodes to `.unknown(rawValue)`.
/// See `NodeRole` in Live.swift for the trade-off discussion.
public enum AlertSeverity: ResilientRawRepresentable {
    case info
    case warning
    case critical
    case unknown(String)

    public var rawValue: String {
        switch self {
        case .info: return "info"
        case .warning: return "warning"
        case .critical: return "critical"
        case .unknown(let v): return v
        }
    }

    public init(rawValue: String) {
        switch rawValue {
        case "info": self = .info
        case "warning": self = .warning
        case "critical": self = .critical
        default: self = .unknown(rawValue)
        }
    }
}

// MARK: - AlertScope

/// Scope filters for alert evaluation.
/// Spec reference: lines 2427-2443 (`AlertScope` schema).
///
/// All fields are optional; omit to evaluate globally.
public struct AlertScope: Decodable, Sendable, Equatable, Hashable {
    public let nodeId: String?
    public let app: String?
    public let streamId: String?
    public let tenant: String?

    enum CodingKeys: String, CodingKey {
        case nodeId = "node_id"
        case app
        case streamId = "stream_id"
        case tenant
    }
}
