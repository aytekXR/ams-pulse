import Foundation

// MARK: - APIError (Wire Schema)

/// Wire schema for API error responses.
/// Spec reference: lines 1856-1869 (`Error` schema).
///
/// The `details` field is an optional dictionary of additional context for the error.
/// This uses `AnyCodableValue` to handle the `additionalProperties: true` schema.
public struct APIError: Decodable, Sendable, Equatable {
    public let code: String
    public let message: String
    public let details: [String: AnyCodableValue]?

    enum CodingKeys: String, CodingKey {
        case code
        case message
        case details
    }
}

// MARK: - AnyCodableValue

/// A type-erased JSON value supporting string, int, double, bool, array, and object.
/// Needed because the API `details` field is `additionalProperties: true`.
public enum AnyCodableValue: Sendable, Equatable {
    case string(String)
    case int(Int)
    case int64(Int64)
    case double(Double)
    case bool(Bool)
    case array([AnyCodableValue])
    case object([String: AnyCodableValue])
    case null

    public init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()

        if container.decodeNil() {
            self = .null
            return
        }

        // Try bool first (before int, since JSON bools can decode as int)
        if let boolValue = try? container.decode(Bool.self) {
            self = .bool(boolValue)
            return
        }

        // Try int
        if let intValue = try? container.decode(Int.self) {
            self = .int(intValue)
            return
        }

        // Try int64 for larger integers
        if let int64Value = try? container.decode(Int64.self) {
            self = .int64(int64Value)
            return
        }

        // Try double
        if let doubleValue = try? container.decode(Double.self) {
            self = .double(doubleValue)
            return
        }

        // Try string
        if let stringValue = try? container.decode(String.self) {
            self = .string(stringValue)
            return
        }

        // Try array
        if let arrayValue = try? container.decode([AnyCodableValue].self) {
            self = .array(arrayValue)
            return
        }

        // Try object
        if let objectValue = try? container.decode([String: AnyCodableValue].self) {
            self = .object(objectValue)
            return
        }

        throw DecodingError.typeMismatch(
            AnyCodableValue.self,
            DecodingError.Context(
                codingPath: decoder.codingPath,
                debugDescription: "Unable to decode AnyCodableValue"
            )
        )
    }
}

extension AnyCodableValue: Decodable {}

extension AnyCodableValue: Hashable {
    public func hash(into hasher: inout Hasher) {
        switch self {
        case .string(let s):
            hasher.combine(0)
            hasher.combine(s)
        case .int(let i):
            hasher.combine(1)
            hasher.combine(i)
        case .int64(let i):
            hasher.combine(2)
            hasher.combine(i)
        case .double(let d):
            hasher.combine(3)
            hasher.combine(d)
        case .bool(let b):
            hasher.combine(4)
            hasher.combine(b)
        case .array(let a):
            hasher.combine(5)
            hasher.combine(a)
        case .object(let o):
            hasher.combine(6)
            for (k, v) in o.sorted(by: { $0.key < $1.key }) {
                hasher.combine(k)
                hasher.combine(v)
            }
        case .null:
            hasher.combine(7)
        }
    }
}
