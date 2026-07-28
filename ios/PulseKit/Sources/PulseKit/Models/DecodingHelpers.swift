import Foundation

// MARK: - Decoding Helpers for Field Resilience

/// PulseKit is a CLIENT for self-hosted Pulse servers. The app ships via the App Store
/// and cannot be updated in lockstep with every server. These helpers ensure graceful
/// handling of:
///
/// 1. **Null or missing arrays** - An older server may send `null` where the current
///    contract says array (observed on a 2026-07-13 build that sent `{"items":null,...}`).
///
/// 2. **Unknown enum values** - A newer server may send enum values this build has never
///    seen. Rather than throwing (which breaks the entire response), we decode into an
///    `.unknown(rawValue)` case that preserves what the server actually said.

// MARK: - Null-Coalescing Array Decoder

extension KeyedDecodingContainer {
    /// Decodes an array that may be `null` or missing, returning an empty array in either case.
    ///
    /// Use this for array fields that an older server might send as `null` or omit entirely.
    /// The distinction between "server sent null" vs "server sent []" is deliberately collapsed
    /// because:
    /// - The UI behavior is identical (show "no items")
    /// - Callers would need to display "unknown" vs "none" only if they could take a different
    ///   action, but there is no such action for a paginated list response
    /// - Preserving the distinction would require `[T]?` throughout the codebase, which trades
    ///   a single decode site for nil-checks everywhere the array is used
    ///
    /// If a future use case genuinely needs "unknown" semantics (e.g., distinguishing "server
    /// didn't answer this question" from "server answered: zero"), add a parallel method that
    /// returns `[T]?` and use it for that specific field.
    public func decodeArrayOrEmpty<T: Decodable>(
        _ type: [T].Type,
        forKey key: Key
    ) throws -> [T] {
        // Try to decode. If key is missing or value is null, return empty array.
        guard contains(key) else {
            return []
        }
        // decodeIfPresent returns nil for both missing key AND explicit null
        return try decodeIfPresent([T].self, forKey: key) ?? []
    }
}

// MARK: - Resilient Enum Protocol

/// A protocol for enums that can decode unknown wire values without throwing.
///
/// ## Trade-off
///
/// Adding an `unknown(String)` case makes the app resilient to server-side enum additions,
/// but it removes the compile-time guarantee that every new value gets deliberate UI treatment.
///
/// **To find all switch sites when adding a known case:**
/// 1. Add the new case (e.g., `case newValue`)
/// 2. Remove the `@unknown default` from all switches (if present)
/// 3. Build - the compiler will flag every switch that needs the new case
/// 4. Restore `@unknown default` once all sites are handled
///
/// Alternatively, search for `switch.*<EnumName>` or `case \.<caseName>` patterns.
///
/// ## Usage
///
/// Conform your enum to `ResilientRawRepresentable` and add:
/// ```swift
/// case unknown(String)
///
/// var rawValue: String {
///     switch self {
///     case .knownCase: return "known_case"
///     case .unknown(let v): return v
///     }
/// }
///
/// init(rawValue: String) {
///     switch rawValue {
///     case "known_case": self = .knownCase
///     default: self = .unknown(rawValue)
///     }
/// }
/// ```
public protocol ResilientRawRepresentable: Decodable, Sendable, Equatable, Hashable {
    var rawValue: String { get }
    init(rawValue: String)
}

extension ResilientRawRepresentable {
    public init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        let raw = try container.decode(String.self)
        self.init(rawValue: raw)
    }
}
