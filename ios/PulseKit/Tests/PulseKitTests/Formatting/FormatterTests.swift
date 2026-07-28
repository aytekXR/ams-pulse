import Foundation
import Testing
@testable import PulseKit

/// Tests for pure formatting functions.
/// All time-dependent functions take an explicit `now` parameter for determinism.
@Suite("Formatters")
struct FormatterTests {
    // MARK: - Bitrate String

    @Test("bitrateString kbps range")
    func bitrateString_kbps() {
        #expect(Formatters.bitrateString(kbps: 456) == "456 kbps")
        #expect(Formatters.bitrateString(kbps: 999) == "999 kbps")
    }

    @Test("bitrateString Mbps range")
    func bitrateString_mbps() {
        #expect(Formatters.bitrateString(kbps: 1234.5) == "1.2 Mbps")
        #expect(Formatters.bitrateString(kbps: 1000) == "1.0 Mbps")
        #expect(Formatters.bitrateString(kbps: 5500) == "5.5 Mbps")
        #expect(Formatters.bitrateString(kbps: 999_999) == "1000.0 Mbps")
    }

    @Test("bitrateString Gbps range")
    func bitrateString_gbps() {
        #expect(Formatters.bitrateString(kbps: 1_000_000) == "1.0 Gbps")
        #expect(Formatters.bitrateString(kbps: 2_500_000) == "2.5 Gbps")
    }

    @Test("bitrateString Tbps range")
    func bitrateString_tbps() {
        #expect(Formatters.bitrateString(kbps: 1_000_000_000) == "1.0 Tbps")
    }

    @Test("bitrateString Pbps range (petabit)")
    func bitrateString_pbps() {
        #expect(Formatters.bitrateString(kbps: 1_000_000_000_000) == "1.0 Pbps")
        #expect(Formatters.bitrateString(kbps: 2_500_000_000_000) == "2.5 Pbps")
    }

    @Test("bitrateString zero")
    func bitrateString_zero() {
        #expect(Formatters.bitrateString(kbps: 0) == "0 kbps")
    }

    @Test("bitrateString negative clamps to zero")
    func bitrateString_negative() {
        #expect(Formatters.bitrateString(kbps: -100) == "0 kbps")
    }

    @Test("bitrateString fractional kbps")
    func bitrateString_fractional() {
        #expect(Formatters.bitrateString(kbps: 0.5) == "0 kbps")
        #expect(Formatters.bitrateString(kbps: 100.7) == "100 kbps")
    }

    // MARK: - Abbreviated Count

    @Test("abbreviatedCount below thousand")
    func abbreviatedCount_belowThousand() {
        #expect(Formatters.abbreviatedCount(0) == "0")
        #expect(Formatters.abbreviatedCount(1) == "1")
        #expect(Formatters.abbreviatedCount(999) == "999")
    }

    @Test("abbreviatedCount thousands")
    func abbreviatedCount_thousands() {
        #expect(Formatters.abbreviatedCount(1000) == "1.0K")
        #expect(Formatters.abbreviatedCount(1234) == "1.2K")
        #expect(Formatters.abbreviatedCount(12847) == "12.8K")
        #expect(Formatters.abbreviatedCount(999_999) == "1000.0K")
    }

    @Test("abbreviatedCount millions")
    func abbreviatedCount_millions() {
        #expect(Formatters.abbreviatedCount(1_000_000) == "1.0M")
        #expect(Formatters.abbreviatedCount(1_234_567) == "1.2M")
        #expect(Formatters.abbreviatedCount(999_999_999) == "1000.0M")
    }

    @Test("abbreviatedCount billions")
    func abbreviatedCount_billions() {
        #expect(Formatters.abbreviatedCount(1_000_000_000) == "1.0B")
        #expect(Formatters.abbreviatedCount(2_500_000_000) == "2.5B")
    }

    @Test("abbreviatedCount negative")
    func abbreviatedCount_negative() {
        #expect(Formatters.abbreviatedCount(-100) == "-100")
        #expect(Formatters.abbreviatedCount(-1234) == "-1.2K")
    }

    // MARK: - Duration String

    @Test("durationString seconds only")
    func durationString_seconds() {
        #expect(Formatters.durationString(seconds: 0) == "0s")
        #expect(Formatters.durationString(seconds: 1) == "1s")
        #expect(Formatters.durationString(seconds: 59) == "59s")
    }

    @Test("durationString minutes and seconds")
    func durationString_minutes() {
        #expect(Formatters.durationString(seconds: 60) == "1m 0s")
        #expect(Formatters.durationString(seconds: 65) == "1m 5s")
        #expect(Formatters.durationString(seconds: 3599) == "59m 59s")
    }

    @Test("durationString hours minutes seconds")
    func durationString_hours() {
        #expect(Formatters.durationString(seconds: 3600) == "1h 0m 0s")
        #expect(Formatters.durationString(seconds: 3661) == "1h 1m 1s")
        #expect(Formatters.durationString(seconds: 7384) == "2h 3m 4s")
    }

    @Test("durationString large values")
    func durationString_large() {
        // 100 hours
        #expect(Formatters.durationString(seconds: 360_000) == "100h 0m 0s")
    }

    @Test("durationString negative clamps to zero")
    func durationString_negative() {
        #expect(Formatters.durationString(seconds: -100) == "0s")
    }

    // MARK: - Relative Time

    @Test("relativeTime seconds ago")
    func relativeTime_seconds() {
        let now = Date(timeIntervalSince1970: 1000)
        let past = Date(timeIntervalSince1970: 970)
        #expect(Formatters.relativeTime(from: past, to: now) == "30s ago")
    }

    @Test("relativeTime minutes ago")
    func relativeTime_minutes() {
        let now = Date(timeIntervalSince1970: 1000)
        let past = Date(timeIntervalSince1970: 700)
        #expect(Formatters.relativeTime(from: past, to: now) == "5m ago")
    }

    @Test("relativeTime hours ago")
    func relativeTime_hours() {
        let now = Date(timeIntervalSince1970: 10000)
        let past = Date(timeIntervalSince1970: 6400)
        #expect(Formatters.relativeTime(from: past, to: now) == "1h ago")
    }

    @Test("relativeTime days ago")
    func relativeTime_days() {
        let now = Date(timeIntervalSince1970: 172800)
        let past = Date(timeIntervalSince1970: 0)
        #expect(Formatters.relativeTime(from: past, to: now) == "2d ago")
    }

    @Test("relativeTime future returns just now")
    func relativeTime_future() {
        let now = Date(timeIntervalSince1970: 1000)
        let future = Date(timeIntervalSince1970: 2000)
        #expect(Formatters.relativeTime(from: future, to: now) == "just now")
    }

    @Test("relativeTime just now threshold")
    func relativeTime_justNow() {
        let now = Date(timeIntervalSince1970: 1000)
        let almostNow = Date(timeIntervalSince1970: 999)
        #expect(Formatters.relativeTime(from: almostNow, to: now) == "just now")
    }

    @Test("relativeTime across DST boundary")
    func relativeTime_dstBoundary() {
        // Using a fixed calendar and time zone for determinism.
        // March 14, 2027 02:30 UTC -> March 14, 2027 03:30 UTC (1 hour later)
        // This crosses DST in US timezones, but we use UTC so it's just 1 hour.
        let beforeDST = Date(timeIntervalSince1970: 1_805_000_000)
        let afterDST = Date(timeIntervalSince1970: 1_805_003_600)
        #expect(Formatters.relativeTime(from: beforeDST, to: afterDST) == "1h ago")
    }

    @Test("relativeTime across leap day")
    func relativeTime_leapDay() {
        // Feb 28, 2028 00:00 UTC to Mar 1, 2028 00:00 UTC = 2 days (leap year)
        let feb28 = Date(timeIntervalSince1970: 1_835_395_200) // 2028-02-28 00:00 UTC
        let mar1 = Date(timeIntervalSince1970: 1_835_568_000)  // 2028-03-01 00:00 UTC
        #expect(Formatters.relativeTime(from: feb28, to: mar1) == "2d ago")
    }

    // MARK: - Health Token Name

    @Test("healthTokenName ok")
    func healthTokenName_ok() {
        #expect(Formatters.healthTokenName(.ok) == "healthy")
    }

    @Test("healthTokenName degraded")
    func healthTokenName_degraded() {
        #expect(Formatters.healthTokenName(.degraded) == "warning")
    }

    @Test("healthTokenName down")
    func healthTokenName_down() {
        #expect(Formatters.healthTokenName(.down) == "critical")
    }

    // MARK: - Unicode Handling

    @Test("abbreviatedCount with unicode context")
    func abbreviatedCount_unicodeContext() {
        // The formatter should handle locale-independent numeric formatting
        // This tests that the output is plain ASCII digits.
        let result = Formatters.abbreviatedCount(12345)
        #expect(result == "12.3K")
        #expect(result.allSatisfy { $0.isASCII })
    }

    // MARK: - Viewer Count

    @Test("viewerCount singular")
    func viewerCount_singular() {
        #expect(Formatters.viewerCount(1) == "1 viewer")
    }

    @Test("viewerCount plural")
    func viewerCount_plural() {
        #expect(Formatters.viewerCount(0) == "0 viewers")
        #expect(Formatters.viewerCount(2) == "2 viewers")
        #expect(Formatters.viewerCount(100) == "100 viewers")
    }

    @Test("viewerCount abbreviated")
    func viewerCount_abbreviated() {
        #expect(Formatters.viewerCount(1234) == "1.2K viewers")
        #expect(Formatters.viewerCount(1_000_000) == "1.0M viewers")
    }

    @Test("viewerCount nil")
    func viewerCount_nil() {
        #expect(Formatters.viewerCount(nil) == "-- viewers")
    }

    @Test("viewerCount negative")
    func viewerCount_negative() {
        // Negative -1 has abs value 1, so singular form
        #expect(Formatters.viewerCount(-1) == "-1 viewer")
        #expect(Formatters.viewerCount(-1234) == "-1.2K viewers")
    }

    // MARK: - Percentage

    @Test("percentage from ratio")
    func percentage_fromRatio() {
        #expect(Formatters.percentage(0.156) == "15.6%")
        #expect(Formatters.percentage(0.5) == "50.0%")
        #expect(Formatters.percentage(1.0) == "100.0%")
    }

    @Test("percentage zero")
    func percentage_zero() {
        #expect(Formatters.percentage(0.0) == "0.0%")
    }

    @Test("percentage clamped above 1")
    func percentage_clampedAbove() {
        #expect(Formatters.percentage(1.5) == "100.0%")
        #expect(Formatters.percentage(2.0) == "100.0%")
    }

    @Test("percentage clamped below 0")
    func percentage_clampedBelow() {
        #expect(Formatters.percentage(-0.5) == "0.0%")
    }

    @Test("percentage nil")
    func percentage_nil() {
        #expect(Formatters.percentage(nil) == "--%")
    }

    @Test("percentage fractional")
    func percentage_fractional() {
        #expect(Formatters.percentage(0.001) == "0.1%")
        #expect(Formatters.percentage(0.999) == "99.9%")
    }

    @Test("percentageRaw values")
    func percentageRaw_values() {
        #expect(Formatters.percentageRaw(85.5) == "85.5%")
        #expect(Formatters.percentageRaw(0.0) == "0.0%")
        #expect(Formatters.percentageRaw(100.0) == "100.0%")
    }

    @Test("percentageRaw clamped")
    func percentageRaw_clamped() {
        #expect(Formatters.percentageRaw(150.0) == "100.0%")
        #expect(Formatters.percentageRaw(-10.0) == "0.0%")
    }

    @Test("percentageRaw nil")
    func percentageRaw_nil() {
        #expect(Formatters.percentageRaw(nil) == "--%")
    }

    // MARK: - Latency

    @Test("latencyMs milliseconds range")
    func latencyMs_millisecondsRange() {
        #expect(Formatters.latencyMs(45) == "45 ms")
        #expect(Formatters.latencyMs(0) == "0 ms")
        #expect(Formatters.latencyMs(999) == "999 ms")
    }

    @Test("latencyMs seconds range")
    func latencyMs_secondsRange() {
        #expect(Formatters.latencyMs(1000) == "1.0 s")
        #expect(Formatters.latencyMs(1500) == "1.5 s")
        #expect(Formatters.latencyMs(5000) == "5.0 s")
    }

    @Test("latencyMs nil")
    func latencyMs_nil() {
        let nilInt: Int? = nil
        #expect(Formatters.latencyMs(nilInt) == "-- ms")
    }

    @Test("latencyMs negative clamped")
    func latencyMs_negativeClamped() {
        #expect(Formatters.latencyMs(-100) == "0 ms")
    }

    @Test("latencyMs large value")
    func latencyMs_largeValue() {
        #expect(Formatters.latencyMs(60000) == "60.0 s")
        #expect(Formatters.latencyMs(3600000) == "3600.0 s")
    }

    @Test("latencyMs from Double")
    func latencyMs_fromDouble() {
        #expect(Formatters.latencyMs(45.6) == "46 ms")
        #expect(Formatters.latencyMs(1500.3) == "1.5 s")
    }

    @Test("latencyMs Double nil")
    func latencyMs_doubleNil() {
        let nilDouble: Double? = nil
        #expect(Formatters.latencyMs(nilDouble) == "-- ms")
    }

    // MARK: - Health Score Display

    @Test("healthScoreDisplay values")
    func healthScoreDisplay_values() {
        #expect(Formatters.healthScoreDisplay(95.5) == "95.5")
        #expect(Formatters.healthScoreDisplay(100.0) == "100.0")
        #expect(Formatters.healthScoreDisplay(0.0) == "0.0")
    }

    @Test("healthScoreDisplay clamped")
    func healthScoreDisplay_clamped() {
        #expect(Formatters.healthScoreDisplay(105.0) == "100.0")
        #expect(Formatters.healthScoreDisplay(-5.0) == "0.0")
    }

    @Test("healthScoreDisplay nil")
    func healthScoreDisplay_nil() {
        #expect(Formatters.healthScoreDisplay(nil) == "--")
    }

    @Test("healthScoreDisplay fractional")
    func healthScoreDisplay_fractional() {
        #expect(Formatters.healthScoreDisplay(50.123) == "50.1")
        #expect(Formatters.healthScoreDisplay(99.999) == "100.0")
    }

    // MARK: - Health Score Normalized

    @Test("healthScoreNormalized values")
    func healthScoreNormalized_values() {
        #expect(Formatters.healthScoreNormalized(100.0) == 1.0)
        #expect(Formatters.healthScoreNormalized(50.0) == 0.5)
        #expect(Formatters.healthScoreNormalized(0.0) == 0.0)
    }

    @Test("healthScoreNormalized clamped")
    func healthScoreNormalized_clamped() {
        #expect(Formatters.healthScoreNormalized(150.0) == 1.0)
        #expect(Formatters.healthScoreNormalized(-50.0) == 0.0)
    }

    @Test("healthScoreNormalized nil")
    func healthScoreNormalized_nil() {
        #expect(Formatters.healthScoreNormalized(nil) == nil)
    }

    @Test("healthScoreNormalized fractional")
    func healthScoreNormalized_fractional() {
        let result = Formatters.healthScoreNormalized(75.0)
        #expect(result == 0.75)
    }

    // MARK: - Relative Time from Epoch Milliseconds

    @Test("relativeTime fromEpochMs seconds ago")
    func relativeTime_epochMs_seconds() {
        let now = Date(timeIntervalSince1970: 1000)
        let epochMs: Int64 = 970_000  // 970 seconds = 30 seconds before now
        #expect(Formatters.relativeTime(fromEpochMs: epochMs, to: now) == "30s ago")
    }

    @Test("relativeTime fromEpochMs minutes ago")
    func relativeTime_epochMs_minutes() {
        let now = Date(timeIntervalSince1970: 1000)
        let epochMs: Int64 = 700_000  // 700 seconds = 5 minutes before now
        #expect(Formatters.relativeTime(fromEpochMs: epochMs, to: now) == "5m ago")
    }

    @Test("relativeTime fromEpochMs hours ago")
    func relativeTime_epochMs_hours() {
        let now = Date(timeIntervalSince1970: 10000)
        let epochMs: Int64 = 6_400_000  // 6400 seconds = 1 hour before now
        #expect(Formatters.relativeTime(fromEpochMs: epochMs, to: now) == "1h ago")
    }

    @Test("relativeTime fromEpochMs days ago")
    func relativeTime_epochMs_days() {
        let now = Date(timeIntervalSince1970: 172800)
        let epochMs: Int64 = 0  // epoch = 2 days before now
        #expect(Formatters.relativeTime(fromEpochMs: epochMs, to: now) == "2d ago")
    }

    @Test("relativeTime fromEpochMs future")
    func relativeTime_epochMs_future() {
        let now = Date(timeIntervalSince1970: 1000)
        let epochMs: Int64 = 2_000_000  // 2000 seconds = future
        #expect(Formatters.relativeTime(fromEpochMs: epochMs, to: now) == "just now")
    }

    @Test("relativeTime fromEpochMs zero")
    func relativeTime_epochMs_zero() {
        let now = Date(timeIntervalSince1970: 86400)  // 1 day after epoch
        let epochMs: Int64 = 0
        #expect(Formatters.relativeTime(fromEpochMs: epochMs, to: now) == "1d ago")
    }

    @Test("relativeTime fromEpochMs large value")
    func relativeTime_epochMs_largeValue() {
        // Timestamp close to Int64.max / 1000 to stay in valid Date range
        let now = Date(timeIntervalSince1970: 1_700_000_000)  // Nov 2023
        let epochMs: Int64 = 1_699_913_600_000  // about 1 day before
        #expect(Formatters.relativeTime(fromEpochMs: epochMs, to: now) == "1d ago")
    }

    @Test("relativeTime fromEpochMs negative")
    func relativeTime_epochMs_negative() {
        // Negative epoch = before 1970
        let now = Date(timeIntervalSince1970: 100)
        let epochMs: Int64 = -100_000  // -100 seconds before epoch
        // 100 - (-100) = 200 seconds = 3m 20s -> "3m ago"
        #expect(Formatters.relativeTime(fromEpochMs: epochMs, to: now) == "3m ago")
    }
}

// MARK: - Non-finite input (D-186 regression)
//
// JSON cannot encode NaN or Infinity by name, which is why nobody wrote these
// tests the first time. It CAN encode a numeric literal that overflows: 1e400
// decodes to +Infinity in a Double field without raising. So a hostile — or
// simply buggy — server can put a non-finite value into any Double the API
// returns, and every one of these formatters is on a path that renders such a
// value in the UI. Int(Double.nan) and Int(Double.infinity) are a *trap*, not an
// error: the app terminates.
//
// The rule this suite pins: a non-finite number is not a measurement, so it
// renders as absent — the same placeholder a nil renders as. Never a crash,
// never the string "nan".
@Suite("Formatters — non-finite input")
struct FormatterNonFiniteTests {
    @Test("bitrateString does not trap on NaN")
    func bitrate_nan() {
        #expect(Formatters.bitrateString(kbps: Double.nan) == "-- kbps")
    }

    @Test("bitrateString does not trap on +infinity")
    func bitrate_posInf() {
        #expect(Formatters.bitrateString(kbps: Double.infinity) == "-- kbps")
    }

    @Test("bitrateString does not trap on -infinity")
    func bitrate_negInf() {
        #expect(Formatters.bitrateString(kbps: -Double.infinity) == "-- kbps")
    }

    @Test("latencyMs Double does not trap on NaN")
    func latency_nan() {
        #expect(Formatters.latencyMs(Double.nan) == "-- ms")
    }

    @Test("latencyMs Double does not trap on infinity")
    func latency_inf() {
        #expect(Formatters.latencyMs(Double.infinity) == "-- ms")
    }

    @Test("percentage renders NaN as absent, not as the text nan")
    func percentage_nan() {
        #expect(Formatters.percentage(Double.nan) == "--%")
    }

    @Test("percentageRaw renders NaN as absent, not as the text nan")
    func percentageRaw_nan() {
        #expect(Formatters.percentageRaw(Double.nan) == "--%")
    }

    @Test("healthScoreDisplay renders NaN as absent")
    func healthScore_nan() {
        #expect(Formatters.healthScoreDisplay(Double.nan) == "--")
    }

    @Test("healthScoreNormalized returns nil for NaN rather than a NaN")
    func healthScoreNormalized_nan() {
        #expect(Formatters.healthScoreNormalized(Double.nan) == nil)
    }

    @Test("healthScoreNormalized returns nil for infinity")
    func healthScoreNormalized_inf() {
        #expect(Formatters.healthScoreNormalized(Double.infinity) == nil)
    }

    @Test("relativeTime does not trap on a non-finite date")
    func relativeTime_nonFiniteDate() {
        let now = Date(timeIntervalSince1970: 1_700_000_000)
        let bogus = Date(timeIntervalSince1970: Double.nan)
        #expect(Formatters.relativeTime(from: bogus, to: now) == "--")
    }
}
