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
public enum LicenseTier: String, Decodable, Sendable, Equatable, Hashable {
    case free
    case pro
    case business
    case enterprise
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
