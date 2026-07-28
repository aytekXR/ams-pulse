import Foundation
import Testing
@testable import PulseKit

/// Tests for TokenStore implementations.
@Suite("TokenStore Tests")
struct TokenStoreTests {

    // MARK: - InMemoryTokenStore

    @Test("InMemoryTokenStore set and get")
    func inMemory_setAndGet() async throws {
        let store = InMemoryTokenStore()
        let serverId = UUID()
        let token = "test-bearer-token"

        try await store.setToken(token, forServerID: serverId)
        let retrieved = try await store.getToken(forServerID: serverId)

        #expect(retrieved == token)
    }

    @Test("InMemoryTokenStore delete")
    func inMemory_delete() async throws {
        let store = InMemoryTokenStore()
        let serverId = UUID()
        let token = "test-bearer-token"

        try await store.setToken(token, forServerID: serverId)
        try await store.deleteToken(forServerID: serverId)
        let retrieved = try await store.getToken(forServerID: serverId)

        #expect(retrieved == nil)
    }

    @Test("InMemoryTokenStore get nonexistent returns nil")
    func inMemory_getNonexistent_returnsNil() async throws {
        let store = InMemoryTokenStore()
        let serverId = UUID()

        let retrieved = try await store.getToken(forServerID: serverId)

        #expect(retrieved == nil)
    }

    @Test("InMemoryTokenStore overwrite")
    func inMemory_overwrite() async throws {
        let store = InMemoryTokenStore()
        let serverId = UUID()

        try await store.setToken("token-v1", forServerID: serverId)
        try await store.setToken("token-v2", forServerID: serverId)
        let retrieved = try await store.getToken(forServerID: serverId)

        #expect(retrieved == "token-v2")
    }

    @Test("InMemoryTokenStore isolates servers")
    func inMemory_isolatesServers() async throws {
        let store = InMemoryTokenStore()
        let server1 = UUID()
        let server2 = UUID()

        try await store.setToken("token-1", forServerID: server1)
        try await store.setToken("token-2", forServerID: server2)

        #expect(try await store.getToken(forServerID: server1) == "token-1")
        #expect(try await store.getToken(forServerID: server2) == "token-2")
    }

    @Test("InMemoryTokenStore delete does not affect other servers")
    func inMemory_deleteIsolated() async throws {
        let store = InMemoryTokenStore()
        let server1 = UUID()
        let server2 = UUID()

        try await store.setToken("token-1", forServerID: server1)
        try await store.setToken("token-2", forServerID: server2)
        try await store.deleteToken(forServerID: server1)

        #expect(try await store.getToken(forServerID: server1) == nil)
        #expect(try await store.getToken(forServerID: server2) == "token-2")
    }

    @Test("InMemoryTokenStore handles empty token")
    func inMemory_emptyToken() async throws {
        let store = InMemoryTokenStore()
        let serverId = UUID()

        try await store.setToken("", forServerID: serverId)
        let retrieved = try await store.getToken(forServerID: serverId)

        #expect(retrieved == "")
    }

    @Test("InMemoryTokenStore handles unicode token")
    func inMemory_unicodeToken() async throws {
        let store = InMemoryTokenStore()
        let serverId = UUID()
        let token = "token-"

        try await store.setToken(token, forServerID: serverId)
        let retrieved = try await store.getToken(forServerID: serverId)

        #expect(retrieved == token)
    }
}

// MARK: - KeychainTokenStore (Darwin only)

#if canImport(Security)
@Suite("KeychainTokenStore Tests")
struct KeychainTokenStoreTests {

    @Test("KeychainTokenStore set and get")
    func keychain_setAndGet() async throws {
        let store = KeychainTokenStore(service: "com.pulse.test.\(UUID().uuidString)")
        let serverId = UUID()
        let token = "test-bearer-token"

        try await store.setToken(token, forServerID: serverId)
        let retrieved = try await store.getToken(forServerID: serverId)

        #expect(retrieved == token)

        // Cleanup
        try await store.deleteToken(forServerID: serverId)
    }

    @Test("KeychainTokenStore delete")
    func keychain_delete() async throws {
        let store = KeychainTokenStore(service: "com.pulse.test.\(UUID().uuidString)")
        let serverId = UUID()
        let token = "test-bearer-token"

        try await store.setToken(token, forServerID: serverId)
        try await store.deleteToken(forServerID: serverId)
        let retrieved = try await store.getToken(forServerID: serverId)

        #expect(retrieved == nil)
    }

    @Test("KeychainTokenStore overwrite")
    func keychain_overwrite() async throws {
        let store = KeychainTokenStore(service: "com.pulse.test.\(UUID().uuidString)")
        let serverId = UUID()

        try await store.setToken("token-v1", forServerID: serverId)
        try await store.setToken("token-v2", forServerID: serverId)
        let retrieved = try await store.getToken(forServerID: serverId)

        #expect(retrieved == "token-v2")

        // Cleanup
        try await store.deleteToken(forServerID: serverId)
    }

    @Test("KeychainTokenStore get nonexistent returns nil")
    func keychain_getNonexistent_returnsNil() async throws {
        let store = KeychainTokenStore(service: "com.pulse.test.\(UUID().uuidString)")
        let serverId = UUID()

        let retrieved = try await store.getToken(forServerID: serverId)

        #expect(retrieved == nil)
    }
}
#endif
