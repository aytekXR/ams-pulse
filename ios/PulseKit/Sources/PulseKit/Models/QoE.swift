import Foundation

// MARK: - QoeSummaryResponse

/// Viewer-side QoE summary with totals and bitrate timeline.
/// Spec reference: lines 2147-2156 (`QoeSummaryResponse` schema).
///
/// Answers from hourly/daily rollups. Sliceable by region, device, stream.
public struct QoeSummaryResponse: Decodable, Sendable, Equatable {
    public let totals: QoeTotals
    /// Bitrate timeline buckets. Decodes as empty array if null or missing (older server compat).
    public let bitrateTimeline: [BitrateBucket]

    enum CodingKeys: String, CodingKey {
        case totals
        case bitrateTimeline = "bitrate_timeline"
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        totals = try container.decode(QoeTotals.self, forKey: .totals)
        bitrateTimeline = try container.decodeArrayOrEmpty([BitrateBucket].self, forKey: .bitrateTimeline)
    }
}

// MARK: - QoeTotals

/// Aggregate QoE metrics: startup time, rebuffer ratio, error rate.
/// Spec reference: lines 2158-2176 (`QoeTotals` schema).
public struct QoeTotals: Decodable, Sendable, Equatable, Hashable {
    /// Median startup time in milliseconds.
    public let startupP50Ms: Double

    /// 95th percentile startup time in milliseconds.
    public let startupP95Ms: Double

    /// Ratio of rebuffer time to total watch time (0-1).
    public let rebufferRatio: Double

    /// Fraction of sessions with at least one error (0-1).
    public let errorRate: Double

    enum CodingKeys: String, CodingKey {
        case startupP50Ms = "startup_p50_ms"
        case startupP95Ms = "startup_p95_ms"
        case rebufferRatio = "rebuffer_ratio"
        case errorRate = "error_rate"
    }
}

// MARK: - BitrateBucket

/// A single bucket in the bitrate timeline.
/// Spec reference: lines 2178-2190 (`BitrateBucket` schema).
public struct BitrateBucket: Decodable, Sendable, Equatable, Hashable {
    /// Bucket start timestamp (Unix epoch ms).
    public let ts: Int64

    /// Median bitrate in kbps.
    public let bitrateKbpsP50: Double

    /// 95th percentile bitrate in kbps; optional.
    public let bitrateKbpsP95: Double?

    enum CodingKeys: String, CodingKey {
        case ts
        case bitrateKbpsP50 = "bitrate_kbps_p50"
        case bitrateKbpsP95 = "bitrate_kbps_p95"
    }
}
