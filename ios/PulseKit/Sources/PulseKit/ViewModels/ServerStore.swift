import Foundation
import Observation

// MARK: - ServerPersistence

/// Protocol for server list persistence.
///
/// Implementations must be `Sendable` for use across concurrency domains.
public protocol ServerPersistence: Sendable {
    /// Load all saved servers.
    func loadServers() async throws -> [PulseServer]

    /// Save the server list, replacing any previous data.
    func saveServers(_ servers: [PulseServer]) async throws
}

// MARK: - ServerStore

/// Observable store for managing saved Pulse server profiles.
///
/// Provides CRUD operations with persistence through an injected `ServerPersistence`.
///
/// ## Selection Behavior
///
/// When the selected server is deleted:
/// - `selectedServerID` is set to nil
/// - The caller must handle the nil state (e.g., prompt to select another)
///
/// ## Duplicate URLs
///
/// Duplicate base URLs are allowed because:
/// - The user might have different tokens for the same server
/// - Testing scenarios benefit from multiple profiles for one server
///
/// ## Concurrency
///
/// See `OverviewModel` for concurrency justification.
@Observable
public final class ServerStore: @unchecked Sendable {
    /// All saved servers.
    public private(set) var servers: [PulseServer] = []

    /// The currently selected server ID, or nil if none selected.
    public var selectedServerID: UUID?

    /// The currently selected server, or nil if none selected.
    public var selectedServer: PulseServer? {
        guard let id = selectedServerID else { return nil }
        return servers.first { $0.id == id }
    }

    private let persistence: ServerPersistence

    /// Create a server store.
    ///
    /// - Parameter persistence: The persistence backend.
    public init(persistence: ServerPersistence) {
        self.persistence = persistence
    }

    /// Load servers from persistence.
    ///
    /// Call this on app launch to populate the server list.
    @MainActor
    public func load() async throws {
        servers = try await persistence.loadServers()
    }

    /// Add a new server.
    ///
    /// - Parameter server: The server to add.
    /// - Throws: If persistence fails.
    @MainActor
    public func add(_ server: PulseServer) async throws {
        servers.append(server)
        try await persistence.saveServers(servers)
    }

    /// Update an existing server.
    ///
    /// - Parameter server: The server with updated values (matched by ID).
    /// - Throws: If persistence fails.
    ///
    /// If the server ID is not found, no action is taken.
    @MainActor
    public func update(_ server: PulseServer) async throws {
        guard let index = servers.firstIndex(where: { $0.id == server.id }) else { return }
        servers[index] = server
        try await persistence.saveServers(servers)
    }

    /// Delete a server by ID.
    ///
    /// - Parameter id: The server ID to delete.
    /// - Throws: If persistence fails.
    ///
    /// If the deleted server was selected, `selectedServerID` is set to nil.
    @MainActor
    public func delete(id: UUID) async throws {
        servers.removeAll { $0.id == id }
        if selectedServerID == id {
            selectedServerID = nil
        }
        try await persistence.saveServers(servers)
    }
}

// MARK: - InMemoryServerPersistence

/// In-memory persistence for testing.
public actor InMemoryServerPersistence: ServerPersistence {
    private var servers: [PulseServer] = []

    public init() {}

    public init(servers: [PulseServer]) {
        self.servers = servers
    }

    public func loadServers() async throws -> [PulseServer] {
        servers
    }

    public func saveServers(_ servers: [PulseServer]) async throws {
        self.servers = servers
    }
}
