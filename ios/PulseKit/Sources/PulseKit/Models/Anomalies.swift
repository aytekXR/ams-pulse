import Foundation

// MARK: - AnomalyList

/// Paginated list of anomaly flags.
/// Spec reference: lines 2764-2773 (`AnomalyList` schema).
public struct AnomalyList: Decodable, Sendable, Equatable {
    /// Anomaly flags. Decodes as empty array if null or missing (older server compat).
    public let items: [AnomalyFlag]
    public let meta: PaginatedMeta

    enum CodingKeys: String, CodingKey {
        case items
        case meta
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        items = try container.decodeArrayOrEmpty([AnomalyFlag].self, forKey: .items)
        meta = try container.decode(PaginatedMeta.self, forKey: .meta)
    }
}

// MARK: - AnomalyFlag

/// A single anomaly detection flag.
/// Spec reference: lines 2775-2800 (`AnomalyFlag` schema).
///
/// Anomaly flags are generated when a metric deviates beyond `sigma` standard
/// deviations from the stored baseline.
public struct AnomalyFlag: Decodable, Sendable, Equatable, Hashable, Identifiable {
    public let id: String
    public let metric: String
    public let scope: AlertScope
    public let observed: Double
    public let expected: Double
    public let sigma: Double
    public let ts: Int64
}
