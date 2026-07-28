import Foundation

// MARK: - AuthMeResponse

/// Current authenticated identity.
/// Spec reference: lines 1227-1239 (`/auth/me` response schema).
///
/// Returns the name, role, and authentication method of the currently
/// authenticated principal.
public struct AuthMeResponse: Decodable, Sendable, Equatable, Hashable {
    public let name: String
    public let role: String
    public let authMethod: AuthMethod

    enum CodingKeys: String, CodingKey {
        case name
        case role
        case authMethod = "auth_method"
    }
}

// MARK: - AuthMethod

/// Authentication method used for the current session.
/// Spec reference: line 1238.
///
/// ## Unknown Value Handling
/// If the server sends an unrecognized method (e.g., "oauth2" added in a future version),
/// it decodes to `.unknown(rawValue)`. See `NodeRole` in Live.swift for the trade-off discussion.
public enum AuthMethod: ResilientRawRepresentable {
    case bearer
    case cookie
    case unknown(String)

    public var rawValue: String {
        switch self {
        case .bearer: return "bearer"
        case .cookie: return "cookie"
        case .unknown(let v): return v
        }
    }

    public init(rawValue: String) {
        switch rawValue {
        case "bearer": self = .bearer
        case "cookie": self = .cookie
        default: self = .unknown(rawValue)
        }
    }
}

// MARK: - User

/// Local user account.
/// Spec reference: lines 3392-3405 (`User` schema).
public struct User: Decodable, Sendable, Equatable, Hashable, Identifiable {
    public let id: String
    public let username: String
    public let role: UserRole
    public let createdAt: Int64

    enum CodingKeys: String, CodingKey {
        case id
        case username
        case role
        case createdAt = "created_at"
    }
}

// MARK: - UserRole

/// User role.
/// Spec reference: line 3402.
///
/// ## Unknown Value Handling
/// If the server sends an unrecognized role (e.g., "operator" added in a future version),
/// it decodes to `.unknown(rawValue)`. See `NodeRole` in Live.swift for the trade-off discussion.
public enum UserRole: ResilientRawRepresentable {
    case admin
    case viewer
    case unknown(String)

    public var rawValue: String {
        switch self {
        case .admin: return "admin"
        case .viewer: return "viewer"
        case .unknown(let v): return v
        }
    }

    public init(rawValue: String) {
        switch rawValue {
        case "admin": self = .admin
        case "viewer": self = .viewer
        default: self = .unknown(rawValue)
        }
    }
}
