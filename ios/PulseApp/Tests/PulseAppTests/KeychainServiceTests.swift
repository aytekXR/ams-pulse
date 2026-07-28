import XCTest
@testable import PulseApp

// MARK: - Keychain Service Tests
//
// Tests for the KeychainService secure credential storage.
//
// Note: These tests interact with the real iOS Keychain. They:
// 1. Clean up after themselves by deleting test credentials.
// 2. Use unique test values to avoid collisions.
// 3. May fail in sandboxed environments without Keychain access.
//
// The Keychain is the right choice over UserDefaults for API tokens because:
// - Tokens are encrypted at rest.
// - Not included in unencrypted backups.
// - Standard practice for secrets; App Store review expects it.

final class KeychainServiceTests: XCTestCase {

    // MARK: - Setup / Teardown

    override func setUp() async throws {
        try await super.setUp()
        // Clean slate before each test.
        try? KeychainService.deleteCredentials()
    }

    override func tearDown() async throws {
        // Clean up after each test.
        try? KeychainService.deleteCredentials()
        try await super.tearDown()
    }

    // MARK: - Save and Retrieve Tests

    func test_saveAndRetrieveCredentials_roundTrip() throws {
        // Arrange
        let serverURL = "https://pulse.example.com"
        let token = "test-token-12345"

        // Act
        try KeychainService.saveCredentials(serverURL: serverURL, token: token)
        let retrieved = try KeychainService.getCredentials()

        // Assert
        XCTAssertNotNil(retrieved)
        XCTAssertEqual(retrieved?.serverURL, serverURL)
        XCTAssertEqual(retrieved?.token, token)
    }

    func test_getServerURL_returnsStoredURL() throws {
        // Arrange
        let serverURL = "https://pulse.example.com"
        try KeychainService.saveCredentials(serverURL: serverURL, token: "token")

        // Act
        let retrieved = try KeychainService.getServerURL()

        // Assert
        XCTAssertEqual(retrieved, serverURL)
    }

    func test_getToken_returnsStoredToken() throws {
        // Arrange
        let token = "my-secret-token"
        try KeychainService.saveCredentials(serverURL: "https://example.com", token: token)

        // Act
        let retrieved = try KeychainService.getToken()

        // Assert
        XCTAssertEqual(retrieved, token)
    }

    // MARK: - Empty State Tests

    func test_getCredentials_whenEmpty_returnsNil() throws {
        // Act
        let credentials = try KeychainService.getCredentials()

        // Assert
        XCTAssertNil(credentials)
    }

    func test_getServerURL_whenEmpty_returnsNil() throws {
        // Act
        let url = try KeychainService.getServerURL()

        // Assert
        XCTAssertNil(url)
    }

    func test_getToken_whenEmpty_returnsNil() throws {
        // Act
        let token = try KeychainService.getToken()

        // Assert
        XCTAssertNil(token)
    }

    func test_hasCredentials_whenEmpty_returnsFalse() {
        // Act
        let has = KeychainService.hasCredentials()

        // Assert
        XCTAssertFalse(has)
    }

    // MARK: - Has Credentials Tests

    func test_hasCredentials_whenStored_returnsTrue() throws {
        // Arrange
        try KeychainService.saveCredentials(serverURL: "https://example.com", token: "token")

        // Act
        let has = KeychainService.hasCredentials()

        // Assert
        XCTAssertTrue(has)
    }

    // MARK: - Delete Tests

    func test_deleteCredentials_removesAllData() throws {
        // Arrange
        try KeychainService.saveCredentials(serverURL: "https://example.com", token: "token")
        XCTAssertTrue(KeychainService.hasCredentials())

        // Act
        try KeychainService.deleteCredentials()

        // Assert
        XCTAssertFalse(KeychainService.hasCredentials())
        XCTAssertNil(try KeychainService.getServerURL())
        XCTAssertNil(try KeychainService.getToken())
    }

    func test_deleteCredentials_whenEmpty_doesNotThrow() throws {
        // Act & Assert — Should not throw.
        XCTAssertNoThrow(try KeychainService.deleteCredentials())
    }

    // MARK: - Overwrite Tests

    func test_saveCredentials_overwritesExisting() throws {
        // Arrange
        try KeychainService.saveCredentials(serverURL: "https://old.com", token: "old-token")

        // Act
        try KeychainService.saveCredentials(serverURL: "https://new.com", token: "new-token")
        let credentials = try KeychainService.getCredentials()

        // Assert
        XCTAssertEqual(credentials?.serverURL, "https://new.com")
        XCTAssertEqual(credentials?.token, "new-token")
    }

    // MARK: - Edge Case Tests

    func test_saveCredentials_withUnicode_roundTrips() throws {
        // Arrange
        let serverURL = "https://pulse.example.com/api"
        let token = "token-with-emoji-\u{1F680}-and-kanji-\u{65E5}\u{672C}"

        // Act
        try KeychainService.saveCredentials(serverURL: serverURL, token: token)
        let retrieved = try KeychainService.getCredentials()

        // Assert
        XCTAssertEqual(retrieved?.token, token)
    }

    func test_saveCredentials_withLongToken_roundTrips() throws {
        // Arrange — JWT tokens can be quite long.
        let longToken = String(repeating: "a", count: 4096)
        try KeychainService.saveCredentials(serverURL: "https://example.com", token: longToken)

        // Act
        let retrieved = try KeychainService.getToken()

        // Assert
        XCTAssertEqual(retrieved?.count, 4096)
    }
}
