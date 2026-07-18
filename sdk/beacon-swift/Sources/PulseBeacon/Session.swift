import Foundation

/// Session helpers, mirroring `sdk/beacon-js/src/session.ts`.
public enum Session {
    /// Generate a lowercase RFC 4122 v4 session UUID. `Foundation.UUID` is a
    /// cryptographically-random v4 UUID; the JS SDK uses `crypto.randomUUID()`
    /// with the same shape. Lowercased to match the JS output.
    public static func generateSessionID() -> String {
        return UUID().uuidString.lowercased()
    }

    /// Decide once per session whether it is sampled.
    /// - Parameter sampleRate: 0…1 (1 = always report, 0 = never report).
    /// - Returns: true if the session should be reported.
    public static func isSampled(_ sampleRate: Double) -> Bool {
        if sampleRate >= 1 { return true }
        if sampleRate <= 0 { return false }
        return Double.random(in: 0..<1) < sampleRate
    }
}
