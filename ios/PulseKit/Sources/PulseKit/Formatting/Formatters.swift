import Foundation

// MARK: - Formatters

/// Pure formatting functions for display values.
///
/// All functions are stateless, locale-independent, and Linux-testable.
/// Time-dependent functions take an explicit `now` parameter for deterministic testing.
public enum Formatters {
    // MARK: - Bitrate Formatting

    /// Format bitrate in kbps to human-readable string.
    ///
    /// - Parameter kbps: Bitrate in kilobits per second.
    /// - Returns: Formatted string (e.g., "1.2 Mbps", "456 kbps").
    ///
    /// Scale: kbps -> Mbps -> Gbps -> Tbps -> Pbps
    /// Negative values are clamped to zero.
    public static func bitrateString(kbps: Double) -> String {
        // A non-finite value is not a measurement — render it as absent.
        // This guard is load-bearing, not defensive style: the kbps range below
        // ends in Int(clamped), and Int(Double.nan) TRAPS. See the non-finite
        // suite in FormatterTests for how such a value reaches us from the wire.
        guard kbps.isFinite else { return "-- kbps" }
        let clamped = max(0, kbps)

        // Petabit range (1e12 kbps = 1 Pbps)
        if clamped >= 1_000_000_000_000 {
            let pbps = clamped / 1_000_000_000_000
            return String(format: "%.1f Pbps", pbps)
        }
        // Terabit range (1e9 kbps = 1 Tbps)
        if clamped >= 1_000_000_000 {
            let tbps = clamped / 1_000_000_000
            return String(format: "%.1f Tbps", tbps)
        }
        // Gigabit range (1e6 kbps = 1 Gbps)
        if clamped >= 1_000_000 {
            let gbps = clamped / 1_000_000
            return String(format: "%.1f Gbps", gbps)
        }
        // Megabit range (1e3 kbps = 1 Mbps)
        if clamped >= 1_000 {
            let mbps = clamped / 1_000
            return String(format: "%.1f Mbps", mbps)
        }
        // Kilobit range
        return "\(Int(clamped)) kbps"
    }

    // MARK: - Count Abbreviation

    /// Abbreviate large counts to human-readable form.
    ///
    /// - Parameter count: The integer count.
    /// - Returns: Abbreviated string (e.g., "12.8K", "1.2M").
    ///
    /// Scale: K (thousand) -> M (million) -> B (billion)
    /// Negative values preserve the sign.
    public static func abbreviatedCount(_ count: Int) -> String {
        let absCount = abs(count)
        let sign = count < 0 ? "-" : ""

        // Billion range
        if absCount >= 1_000_000_000 {
            let b = Double(absCount) / 1_000_000_000
            return "\(sign)\(String(format: "%.1f", b))B"
        }
        // Million range
        if absCount >= 1_000_000 {
            let m = Double(absCount) / 1_000_000
            return "\(sign)\(String(format: "%.1f", m))M"
        }
        // Thousand range
        if absCount >= 1_000 {
            let k = Double(absCount) / 1_000
            return "\(sign)\(String(format: "%.1f", k))K"
        }
        // Below thousand
        return "\(count)"
    }

    // MARK: - Duration Formatting

    /// Format duration in seconds to human-readable string.
    ///
    /// - Parameter seconds: Duration in seconds.
    /// - Returns: Formatted string (e.g., "1h 1m 1s", "5m 30s").
    ///
    /// Negative values are clamped to zero.
    public static func durationString(seconds: Int) -> String {
        let clamped = max(0, seconds)

        let hours = clamped / 3600
        let minutes = (clamped % 3600) / 60
        let secs = clamped % 60

        if hours > 0 {
            return "\(hours)h \(minutes)m \(secs)s"
        }
        if minutes > 0 {
            return "\(minutes)m \(secs)s"
        }
        return "\(secs)s"
    }

    // MARK: - Relative Time

    /// Format relative time from a date to now.
    ///
    /// - Parameters:
    ///   - date: The past date.
    ///   - now: The current date (explicit for testing).
    /// - Returns: Relative string (e.g., "30s ago", "1h ago", "2d ago").
    ///
    /// Future dates or very recent times return "just now".
    public static func relativeTime(from date: Date, to now: Date) -> String {
        let interval = now.timeIntervalSince(date)

        // A Date built from a non-finite interval yields a non-finite delta, and
        // Int(interval) below traps on it. "just now" would be a lie; "--" is not.
        guard interval.isFinite else { return "--" }

        // Future or very recent
        if interval < 2 {
            return "just now"
        }

        let seconds = Int(interval)

        // Days
        if seconds >= 86400 {
            let days = seconds / 86400
            return "\(days)d ago"
        }
        // Hours
        if seconds >= 3600 {
            let hours = seconds / 3600
            return "\(hours)h ago"
        }
        // Minutes
        if seconds >= 60 {
            let minutes = seconds / 60
            return "\(minutes)m ago"
        }
        // Seconds
        return "\(seconds)s ago"
    }

    // MARK: - Metric-aware values

    /// Format an alert's value or threshold using the METRIC's semantics.
    ///
    /// - Parameters:
    ///   - value: The raw number the API returned.
    ///   - metric: The metric name that came with it, e.g. "cpu_pct".
    /// - Returns: A display string with the right unit, or "--" if not finite.
    ///
    /// ⚠ Decide from the NAME, never from the magnitude. The app previously used
    /// "if the value is between 0 and 100, add a % sign", which is a guess about
    /// semantics inferred from a value — and it was wrong in both directions on
    /// one screen: viewer_count 50 rendered as "50.0%", and rebuffer_ratio 0.18
    /// (i.e. 18%) rendered as "0.2%". cpu_pct was right only because its range
    /// happened to suit the heuristic.
    ///
    /// The conventions come from the metric names the API actually emits:
    ///   *_pct    already 0-100
    ///   *_ratio  0-1, scale by 100
    ///   anything else — counts, kbps, ms — carries no unit here
    /// An unrecognised metric is rendered as a plain number rather than being
    /// dressed up as a percentage, because being silently wrong about a unit is
    /// worse than being plain.
    public static func metricValue(_ value: Double, metric: String) -> String {
        guard value.isFinite else { return "--" }

        let name = metric.lowercased()
        if name.hasSuffix("_pct") {
            return String(format: "%.1f%%", value)
        }
        if name.hasSuffix("_ratio") {
            return String(format: "%.1f%%", value * 100)
        }
        if value == value.rounded() && abs(value) < 1e15 {
            return "\(Int(value))"
        }
        return String(format: "%.1f", value)
    }

    // MARK: - Health Token Name

    /// Map health status to a design system token name.
    ///
    /// - Parameter status: The health state.
    /// - Returns: Token name from brandkit/design-system/tokens.json.
    ///
    /// Mapping:
    /// - `.ok` -> "healthy"
    /// - `.degraded` -> "warning"
    /// - `.down` -> "critical"
    /// - `.unknown` -> "neutral"
    public static func healthTokenName(_ status: HealthState) -> String {
        switch status {
        case .ok:
            return "healthy"
        case .degraded:
            return "warning"
        case .down:
            return "critical"
        case .unknown:
            // A state this build has never heard of, from a newer server. "warning"
            // was the obvious choice and it is wrong: it asserts something is
            // degraded when the truth is that we do not know, which is the same
            // invention the honest-absent rule exists to prevent. "neutral" is grey
            // — it cannot be mistaken for healthy, and it does not claim a fault.
            // The raw value is preserved on the case, so the UI can show what the
            // server actually said instead of hiding it behind a colour.
            return "neutral"
        }
    }

    // MARK: - Viewer Count

    /// Format viewer count for display.
    ///
    /// - Parameter count: Number of viewers; nil displays as "--".
    /// - Returns: Formatted viewer string (e.g., "12.8K viewers", "1 viewer").
    ///
    /// Uses abbreviatedCount for large numbers. Singular/plural "viewer(s)".
    public static func viewerCount(_ count: Int?) -> String {
        guard let count = count else {
            return "-- viewers"
        }
        let abbreviated = abbreviatedCount(count)
        let suffix = abs(count) == 1 ? "viewer" : "viewers"
        return "\(abbreviated) \(suffix)"
    }

    // MARK: - Percentage

    /// Format a ratio as a percentage string.
    ///
    /// - Parameter ratio: Value between 0 and 1 (e.g., 0.156 -> "15.6%").
    /// - Returns: Percentage string with one decimal place.
    ///
    /// Values are clamped to [0, 1] range. Nil returns "--".
    public static func percentage(_ ratio: Double?) -> String {
        // Non-finite does not trap here, but it renders the literal text "nan%"
        // in the UI, which is worse than an honest placeholder.
        guard let ratio = ratio, ratio.isFinite else {
            return "--%"
        }
        let clamped = max(0, min(1, ratio))
        let pct = clamped * 100
        return String(format: "%.1f%%", pct)
    }

    /// Format a raw percentage value (already 0-100 scale).
    ///
    /// - Parameter value: Percentage value (e.g., 85.5 -> "85.5%").
    /// - Returns: Percentage string with one decimal place.
    ///
    /// Values are clamped to [0, 100] range. Nil returns "--".
    public static func percentageRaw(_ value: Double?) -> String {
        guard let value = value, value.isFinite else {
            return "--%"
        }
        let clamped = max(0, min(100, value))
        return String(format: "%.1f%%", clamped)
    }

    // MARK: - Latency

    /// Format latency in milliseconds for display.
    ///
    /// - Parameter ms: Latency in milliseconds; nil displays as "--".
    /// - Returns: Formatted latency string (e.g., "45 ms", "1.2 s").
    ///
    /// Values >= 1000 ms are shown in seconds. Negative clamped to zero.
    public static func latencyMs(_ ms: Int?) -> String {
        guard let ms = ms else {
            return "-- ms"
        }
        let clamped = max(0, ms)
        if clamped >= 1000 {
            let seconds = Double(clamped) / 1000.0
            return String(format: "%.1f s", seconds)
        }
        return "\(clamped) ms"
    }

    /// Format latency from a Double value.
    ///
    /// - Parameter ms: Latency in milliseconds; nil displays as "--".
    /// - Returns: Formatted latency string (e.g., "45 ms", "1.2 s").
    public static func latencyMs(_ ms: Double?) -> String {
        // isFinite must be checked BEFORE the Int conversion below: max(0, nan)
        // is nan (every NaN comparison is false), so clamping does not protect
        // the conversion — Int(nan) and Int(infinity) both trap.
        guard let ms = ms, ms.isFinite else {
            return "-- ms"
        }
        let clamped = max(0, Int(ms.rounded()))
        return latencyMs(clamped)
    }

    // MARK: - Health Score

    /// Format health score as a display string.
    ///
    /// - Parameter score: Health score 0-100; nil displays as "--".
    /// - Returns: Formatted score (e.g., "95.5", "100.0").
    ///
    /// Clamped to [0, 100] range.
    public static func healthScoreDisplay(_ score: Double?) -> String {
        guard let score = score, score.isFinite else {
            return "--"
        }
        let clamped = max(0, min(100, score))
        return String(format: "%.1f", clamped)
    }

    /// Convert health score to normalized 0-1 value.
    ///
    /// - Parameter score: Health score 0-100; nil returns nil.
    /// - Returns: Normalized value in [0, 1] range.
    ///
    /// Useful for progress indicators and color interpolation.
    public static func healthScoreNormalized(_ score: Double?) -> Double? {
        // Returning a NaN here would poison whatever the caller does with it —
        // a progress bar width, a colour interpolation — far from this call site.
        guard let score = score, score.isFinite else {
            return nil
        }
        return max(0, min(1, score / 100.0))
    }

    // MARK: - Relative Time (Epoch Milliseconds)

    /// Format relative time from an epoch timestamp in milliseconds.
    ///
    /// - Parameters:
    ///   - epochMs: Unix timestamp in milliseconds.
    ///   - now: The current date (explicit for testing).
    /// - Returns: Relative string (e.g., "30s ago", "1h ago", "2d ago").
    ///
    /// This variant takes the wire format (epoch milliseconds) directly.
    public static func relativeTime(fromEpochMs epochMs: Int64, to now: Date) -> String {
        let epochSeconds = TimeInterval(epochMs) / 1000.0
        let date = Date(timeIntervalSince1970: epochSeconds)
        return relativeTime(from: date, to: now)
    }
}
