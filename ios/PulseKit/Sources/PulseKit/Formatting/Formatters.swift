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
    public static func healthTokenName(_ status: HealthState) -> String {
        switch status {
        case .ok:
            return "healthy"
        case .degraded:
            return "warning"
        case .down:
            return "critical"
        }
    }
}
