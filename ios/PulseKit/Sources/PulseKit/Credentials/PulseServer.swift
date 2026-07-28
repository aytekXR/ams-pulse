import Foundation

// MARK: - PulseServer

/// A saved Pulse server profile.
///
/// Contains the connection details for a Pulse server instance. The token
/// is stored separately in a `TokenStore` for security.
public struct PulseServer: Sendable, Equatable, Hashable, Identifiable, Codable {
    /// Unique identifier for this server profile.
    public let id: UUID

    /// User-friendly display name for the server.
    public var displayName: String

    /// Base URL of the Pulse server (e.g., "https://pulse.example.com").
    public var baseURL: URL

    /// Optional notes about this server.
    public var notes: String?

    /// When this profile was created.
    public let createdAt: Date

    /// Create a new server profile.
    ///
    /// - Parameters:
    ///   - id: Unique identifier. Defaults to a new UUID.
    ///   - displayName: User-friendly name for the server.
    ///   - baseURL: Base URL of the Pulse server.
    ///   - notes: Optional notes.
    ///   - createdAt: Creation timestamp. Defaults to now.
    public init(
        id: UUID = UUID(),
        displayName: String,
        baseURL: URL,
        notes: String? = nil,
        createdAt: Date = Date()
    ) {
        self.id = id
        self.displayName = displayName
        self.baseURL = baseURL
        self.notes = notes
        self.createdAt = createdAt
    }
}
