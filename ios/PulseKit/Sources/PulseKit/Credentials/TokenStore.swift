import Foundation

// MARK: - TokenStore Protocol

/// Protocol for storing bearer tokens securely.
///
/// Implementations must be `Sendable` for use with Swift concurrency.
/// Tokens are keyed by server UUID to support multiple server profiles.
public protocol TokenStore: Sendable {
    /// Retrieve the token for a server.
    ///
    /// - Parameter id: The server's UUID.
    /// - Returns: The stored token, or nil if not found.
    func getToken(forServerID id: UUID) async throws -> String?

    /// Store a token for a server.
    ///
    /// - Parameters:
    ///   - token: The bearer token to store.
    ///   - id: The server's UUID.
    func setToken(_ token: String, forServerID id: UUID) async throws

    /// Delete the token for a server.
    ///
    /// - Parameter id: The server's UUID.
    func deleteToken(forServerID id: UUID) async throws
}

// MARK: - InMemoryTokenStore

/// In-memory token store for tests and Linux.
///
/// Thread-safe through actor isolation. Tokens are lost when the process exits.
public actor InMemoryTokenStore: TokenStore {
    private var tokens: [UUID: String] = [:]

    public init() {}

    public func getToken(forServerID id: UUID) async throws -> String? {
        tokens[id]
    }

    public func setToken(_ token: String, forServerID id: UUID) async throws {
        tokens[id] = token
    }

    public func deleteToken(forServerID id: UUID) async throws {
        tokens[id] = nil
    }
}

// MARK: - KeychainTokenStore (Darwin only)

#if canImport(Security)
import Security

/// Keychain-based token store for iOS and macOS.
///
/// Uses the system Keychain to securely store bearer tokens.
/// Marked `@unchecked Sendable` because Keychain APIs are thread-safe.
public final class KeychainTokenStore: TokenStore, @unchecked Sendable {
    private let service: String
    private let accessGroup: String?

    /// Create a new Keychain token store.
    ///
    /// - Parameters:
    ///   - service: The Keychain service name. Defaults to "com.pulse.api".
    ///   - accessGroup: Optional Keychain access group for sharing between apps.
    public init(service: String = "com.pulse.api", accessGroup: String? = nil) {
        self.service = service
        self.accessGroup = accessGroup
    }

    public func getToken(forServerID id: UUID) async throws -> String? {
        var query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: id.uuidString,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne
        ]

        if let accessGroup = accessGroup {
            query[kSecAttrAccessGroup as String] = accessGroup
        }

        var result: AnyObject?
        let status = SecItemCopyMatching(query as CFDictionary, &result)

        switch status {
        case errSecSuccess:
            guard let data = result as? Data,
                  let token = String(data: data, encoding: .utf8) else {
                return nil
            }
            return token
        case errSecItemNotFound:
            return nil
        default:
            throw KeychainError.readFailed(status)
        }
    }

    public func setToken(_ token: String, forServerID id: UUID) async throws {
        guard let tokenData = token.data(using: .utf8) else {
            throw KeychainError.encodingFailed
        }

        // Try to update existing item first
        var query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: id.uuidString
        ]

        if let accessGroup = accessGroup {
            query[kSecAttrAccessGroup as String] = accessGroup
        }

        let attributes: [String: Any] = [
            kSecValueData as String: tokenData
        ]

        let updateStatus = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)

        if updateStatus == errSecItemNotFound {
            // Item doesn't exist, add it
            var addQuery = query
            addQuery[kSecValueData as String] = tokenData
            addQuery[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly

            let addStatus = SecItemAdd(addQuery as CFDictionary, nil)
            guard addStatus == errSecSuccess else {
                throw KeychainError.writeFailed(addStatus)
            }
        } else if updateStatus != errSecSuccess {
            throw KeychainError.writeFailed(updateStatus)
        }
    }

    public func deleteToken(forServerID id: UUID) async throws {
        var query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: id.uuidString
        ]

        if let accessGroup = accessGroup {
            query[kSecAttrAccessGroup as String] = accessGroup
        }

        let status = SecItemDelete(query as CFDictionary)

        // errSecItemNotFound is not an error - the item is already gone
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw KeychainError.deleteFailed(status)
        }
    }
}

/// Errors from Keychain operations.
public enum KeychainError: Error, Sendable {
    case readFailed(OSStatus)
    case writeFailed(OSStatus)
    case deleteFailed(OSStatus)
    case encodingFailed
}
#endif
