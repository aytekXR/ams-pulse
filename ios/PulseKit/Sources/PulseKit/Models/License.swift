import Foundation

// MARK: - LicenseInfo

/// License status, tier, limits, and validity.
/// Spec reference: lines 3220-3253 (`LicenseInfo` schema).
///
/// Tier entitlement matrix (PRD 7.11):
/// - free: 1 node, 7-day retention, email alerts only
/// - pro: 1-2 nodes, 90-day retention, Slack/Telegram
/// - business: <=5 nodes, 13-month retention, PagerDuty/webhooks, multi-tenant
/// - enterprise: unlimited nodes, SSO, white-label, anomaly detection
public struct LicenseInfo: Decodable, Sendable, Equatable {
    public let tier: LicenseTier
    public let valid: Bool
    public let limits: TierLimits?
    public let expiresAt: Int64?
    public let offlineFile: Bool?

    enum CodingKeys: String, CodingKey {
        case tier
        case valid
        case limits
        case expiresAt = "expires_at"
        case offlineFile = "offline_file"
    }
}

// MARK: - LicenseTier

/// License tier.
/// Spec reference: line 3236.
///
/// ## Unknown Value Handling
/// If the server sends an unrecognized tier (e.g., "platinum" added in a future version),
/// it decodes to `.unknown(rawValue)`. See `NodeRole` in Live.swift for the trade-off discussion.
public enum LicenseTier: ResilientRawRepresentable {
    case free
    case pro
    case business
    case enterprise
    case unknown(String)

    public var rawValue: String {
        switch self {
        case .free: return "free"
        case .pro: return "pro"
        case .business: return "business"
        case .enterprise: return "enterprise"
        case .unknown(let v): return v
        }
    }

    public init(rawValue: String) {
        switch rawValue {
        case "free": self = .free
        case "pro": self = .pro
        case "business": self = .business
        case "enterprise": self = .enterprise
        default: self = .unknown(rawValue)
        }
    }
}

// MARK: - TierLimits

/// Tier entitlement limits (nil = unlimited).
/// Spec reference: lines 3254-3268 (`TierLimits` schema).
public struct TierLimits: Decodable, Sendable, Equatable, Hashable {
    public let maxStreams: Int?
    public let maxNodes: Int?
    public let retentionDays: Int?
    public let dataApi: Bool?
    public let whiteLabel: Bool?

    enum CodingKeys: String, CodingKey {
        case maxStreams = "max_streams"
        case maxNodes = "max_nodes"
        case retentionDays = "retention_days"
        case dataApi = "data_api"
        case whiteLabel = "white_label"
    }
}
