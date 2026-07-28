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
}
