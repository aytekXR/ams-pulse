import Foundation
import Security

// MARK: - Keychain Service
//
// Secure storage for the Pulse API token. Uses kSecClassGenericPassword, which
// is the appropriate class for API tokens (vs. kSecClassInternetPassword which
// is for web-form-style credentials).
//
// Why Keychain over UserDefaults:
//   1. Tokens are encrypted at rest.
//   2. Not included in backups unless the user enables keychain backup.
//   3. Survives app reinstallation if kSecAttrAccessible allows.
//   4. Standard practice for secrets; App Store review expects it.
//
// Thread safety: All operations are synchronous and thread-safe by virtue of
// the Security framework's internal locking. Callers don't need to synchronize.

/// Error types for Keychain operations.
public enum KeychainError: Error, Sendable {
    case unexpectedData
    case osStatus(OSStatus)

    public var localizedDescription: String {
        switch self {
        case .unexpectedData:
            return "Keychain returned unexpected data format"
        case .osStatus(let status):
            // SecCopyErrorMessageString is not available on all platforms.
            return "Keychain error: \(status)"
        }
    }
}

/// Secure storage for Pulse credentials in the iOS Keychain.
///
/// Thread-safe. All methods are synchronous; for async use, call from a Task.
/// The service is Sendable because it holds no mutable state — all state lives
/// in the Keychain itself.
public struct KeychainService: Sendable {

    // MARK: - Configuration

    /// Keychain service identifier. Using the bundle ID ensures isolation.
    /// If the bundle ID is unavailable (e.g., in tests), falls back to a constant.
    private static let service: String = {
        Bundle.main.bundleIdentifier ?? "com.pulse.app"
    }()

    /// Account names for stored credentials.
    private enum Account: String {
        case serverURL = "pulse.server.url"
        case apiToken = "pulse.api.token"
    }

    // MARK: - Public API

    /// Stores the Pulse server URL and API token in the Keychain.
    ///
    /// - Parameters:
    ///   - serverURL: The base URL of the Pulse server (e.g., "https://pulse.example.com").
    ///   - token: The bearer token for API authentication.
    /// - Throws: `KeychainError` if the operation fails.
    public static func saveCredentials(serverURL: String, token: String) throws {
        try saveItem(account: .serverURL, data: serverURL.data(using: .utf8)!)
        try saveItem(account: .apiToken, data: token.data(using: .utf8)!)
    }

    /// Retrieves the stored Pulse server URL.
    ///
    /// - Returns: The server URL, or nil if not stored.
    /// - Throws: `KeychainError` if the operation fails (other than item not found).
    public static func getServerURL() throws -> String? {
        guard let data = try loadItem(account: .serverURL) else { return nil }
        return String(data: data, encoding: .utf8)
    }

    /// Retrieves the stored API token.
    ///
    /// - Returns: The API token, or nil if not stored.
    /// - Throws: `KeychainError` if the operation fails (other than item not found).
    public static func getToken() throws -> String? {
        guard let data = try loadItem(account: .apiToken) else { return nil }
        return String(data: data, encoding: .utf8)
    }

    /// Retrieves both the server URL and token as a tuple.
    ///
    /// - Returns: A tuple of (serverURL, token), or nil if either is missing.
    /// - Throws: `KeychainError` if the operation fails.
    public static func getCredentials() throws -> (serverURL: String, token: String)? {
        guard let url = try getServerURL(),
              let token = try getToken() else {
            return nil
        }
        return (url, token)
    }

    /// Deletes all stored Pulse credentials from the Keychain.
    ///
    /// This is the "sign out" operation. Silently succeeds if no credentials exist.
    ///
    /// - Throws: `KeychainError` if deletion fails for a reason other than "not found".
    public static func deleteCredentials() throws {
        try deleteItem(account: .serverURL)
        try deleteItem(account: .apiToken)
    }

    /// Checks whether credentials are stored.
    ///
    /// Cheaper than `getCredentials()` because it doesn't retrieve the data.
    public static func hasCredentials() -> Bool {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: Account.apiToken.rawValue,
            kSecReturnData as String: false  // Don't return data, just check existence.
        ]
        let status = SecItemCopyMatching(query as CFDictionary, nil)
        return status == errSecSuccess
    }

    // MARK: - Private Implementation

    /// Saves or updates an item in the Keychain.
    private static func saveItem(account: Account, data: Data) throws {
        // First, try to delete any existing item (update = delete + add).
        // This is simpler than SecItemUpdate and handles the "doesn't exist" case.
        try? deleteItem(account: account)

        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account.rawValue,
            kSecValueData as String: data,
            // Accessible after first unlock; survives background execution.
            // This is the right balance between security and usability.
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlock
        ]

        let status = SecItemAdd(query as CFDictionary, nil)
        guard status == errSecSuccess else {
            throw KeychainError.osStatus(status)
        }
    }

    /// Loads an item from the Keychain.
    private static func loadItem(account: Account) throws -> Data? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account.rawValue,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne
        ]

        var result: AnyObject?
        let status = SecItemCopyMatching(query as CFDictionary, &result)

        switch status {
        case errSecSuccess:
            guard let data = result as? Data else {
                throw KeychainError.unexpectedData
            }
            return data
        case errSecItemNotFound:
            return nil
        default:
            throw KeychainError.osStatus(status)
        }
    }

    /// Deletes an item from the Keychain.
    private static func deleteItem(account: Account) throws {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account.rawValue
        ]

        let status = SecItemDelete(query as CFDictionary)
        // errSecItemNotFound is not an error for delete operations.
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw KeychainError.osStatus(status)
        }
    }
}
