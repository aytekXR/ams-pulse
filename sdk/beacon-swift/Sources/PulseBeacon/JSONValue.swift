import Foundation

/// A minimal JSON scalar used for the type-specific `data` payload of a beacon
/// event. The beacon schema's `data` object is open with per-type shapes
/// (`startup_ms`, `bitrate_kbps`, `code`, `from_kbps`, …), so the SDK models it
/// as `[String: JSONValue]` and the strongly-typed `PulseBeacon` event helpers
/// build the correct keys. Encodes to the same primitives the JS SDK emits.
public enum JSONValue: Codable, Equatable {
    case string(String)
    case int(Int)
    case double(Double)
    case bool(Bool)

    public init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        // Order matters: Bool before Int (Foundation rejects `1` as Bool, so this
        // does not mis-decode numbers), Int before Double so whole numbers stay ints.
        if let b = try? container.decode(Bool.self) {
            self = .bool(b)
        } else if let i = try? container.decode(Int.self) {
            self = .int(i)
        } else if let d = try? container.decode(Double.self) {
            self = .double(d)
        } else if let s = try? container.decode(String.self) {
            self = .string(s)
        } else {
            throw DecodingError.dataCorruptedError(
                in: container,
                debugDescription: "JSONValue supports only string, int, double, and bool"
            )
        }
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .string(let s): try container.encode(s)
        case .int(let i): try container.encode(i)
        case .double(let d): try container.encode(d)
        case .bool(let b): try container.encode(b)
        }
    }
}
