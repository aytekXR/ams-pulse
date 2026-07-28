import Foundation

// MARK: - HealthStatus

/// Top-level health check response.
/// Spec reference: lines 3063-3088 (`HealthStatus` schema).
///
/// This endpoint is UNAUTHENTICATED - it's the one call the app can make without
/// a token to verify the server is reachable before prompting for credentials.
public struct HealthStatus: Decodable, Sendable, Equatable {
    /// Aggregate status.
    public let status: HealthState

    /// True when PULSE_AMS_URL was set via environment (vs. ams_sources table).
    /// The UI uses this to skip the onboarding wizard for env-configured deployments.
    public let amsEnvConfigured: Bool?

    /// Per-component health breakdown.
    public let components: HealthComponents

    enum CodingKeys: String, CodingKey {
        case status
        case amsEnvConfigured = "ams_env_configured"
        case components
    }
}

// MARK: - HealthState

/// Health status enum.
/// Spec reference: line 3069.
///
/// ## Unknown Value Handling
/// If the server sends an unrecognized state (e.g., "maintenance" added in a future version),
/// it decodes to `.unknown(rawValue)`. See `NodeRole` in Live.swift for the trade-off discussion.
public enum HealthState: ResilientRawRepresentable {
    case ok
    case degraded
    case down
    case unknown(String)

    public var rawValue: String {
        switch self {
        case .ok: return "ok"
        case .degraded: return "degraded"
        case .down: return "down"
        case .unknown(let v): return v
        }
    }

    public init(rawValue: String) {
        switch rawValue {
        case "ok": self = .ok
        case "degraded": self = .degraded
        case "down": self = .down
        default: self = .unknown(rawValue)
        }
    }
}

// MARK: - HealthComponents

/// Health status for individual Pulse components.
/// Spec reference: lines 3077-3088.
public struct HealthComponents: Decodable, Sendable, Equatable {
    public let clickhouse: ComponentStatus
    public let metaStore: ComponentStatus
    public let collector: ComponentStatus
    public let kafka: KafkaComponentStatus?

    enum CodingKeys: String, CodingKey {
        case clickhouse
        case metaStore = "meta_store"
        case collector
        case kafka
    }
}

// MARK: - ComponentStatus

/// Health status for a single component.
/// Spec reference: lines 3090-3101 (`ComponentStatus` schema).
public struct ComponentStatus: Decodable, Sendable, Equatable, Hashable {
    /// Status.
    public let status: HealthState

    /// Latency in milliseconds; nil if not measured or component is down.
    public let latencyMs: Int?

    /// Human-readable message (e.g., error detail); nil when healthy.
    public let message: String?

    enum CodingKeys: String, CodingKey {
        case status
        case latencyMs = "latency_ms"
        case message
    }
}

// MARK: - KafkaComponentStatus

/// Health status for the Kafka component with additional metrics.
/// Spec reference: lines 3102-3112 (`KafkaComponentStatus` schema).
///
/// Extends ComponentStatus with:
/// - `lag`: Total consumer group lag across all topic partitions
/// - `parse_errors`: Number of message parse errors since last reset
public struct KafkaComponentStatus: Decodable, Sendable, Equatable, Hashable {
    /// Status.
    public let status: HealthState

    /// Latency in milliseconds; nil if not measured or component is down.
    public let latencyMs: Int?

    /// Human-readable message (e.g., error detail); nil when healthy.
    public let message: String?

    /// Total consumer group lag across all topic partitions.
    public let lag: Int?

    /// Number of message parse errors since last reset.
    public let parseErrors: Int?

    enum CodingKeys: String, CodingKey {
        case status
        case latencyMs = "latency_ms"
        case message
        case lag
        case parseErrors = "parse_errors"
    }
}
