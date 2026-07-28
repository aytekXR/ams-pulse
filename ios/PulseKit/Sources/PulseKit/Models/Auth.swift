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
public enum AuthMethod: String, Decodable, Sendable, Equatable, Hashable {
    case bearer
    case cookie
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
public enum UserRole: String, Decodable, Sendable, Equatable, Hashable {
    case admin
    case viewer
}
