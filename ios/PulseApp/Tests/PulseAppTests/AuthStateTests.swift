import XCTest
@testable import PulseApp

// MARK: - Auth State Tests
//
// Tests for the AuthState enum used to track authentication state.
//
// AuthState represents:
// - .signedOut: No connection to any Pulse server
// - .signedIn(serverURL:): Connected to a specific server
//
// These tests verify the enum's semantics and pattern matching behavior.

final class AuthStateTests: XCTestCase {

    // MARK: - Basic State Tests

    func test_signedOut_matchesSignedOutPattern() {
        // Arrange
        let state = AuthState.signedOut

        // Act & Assert
        if case .signedOut = state {
            // Pass
        } else {
            XCTFail("Expected .signedOut")
        }
    }

    func test_signedIn_containsServerURL() {
        // Arrange
        let serverURL = "https://pulse.example.com"
        let state = AuthState.signedIn(serverURL: serverURL)

        // Act & Assert
        if case .signedIn(let url) = state {
            XCTAssertEqual(url, serverURL)
        } else {
            XCTFail("Expected .signedIn")
        }
    }

    func test_signedIn_matchesSignedInPattern() {
        // Arrange
        let state = AuthState.signedIn(serverURL: "https://pulse.example.com")

        // Act
        let isSignedIn: Bool
        if case .signedIn = state {
            isSignedIn = true
        } else {
            isSignedIn = false
        }

        // Assert
        XCTAssertTrue(isSignedIn)
    }

    func test_signedOut_doesNotMatchSignedIn() {
        // Arrange
        let state = AuthState.signedOut

        // Act
        let isSignedIn: Bool
        if case .signedIn = state {
            isSignedIn = true
        } else {
            isSignedIn = false
        }

        // Assert
        XCTAssertFalse(isSignedIn)
    }

    // MARK: - URL Variations

    func test_signedIn_withHTTPURL() {
        // Arrange — HTTP allowed for local development servers.
        let state = AuthState.signedIn(serverURL: "http://localhost:8090")

        // Assert
        if case .signedIn(let url) = state {
            XCTAssertTrue(url.hasPrefix("http://"))
        } else {
            XCTFail("Expected .signedIn")
        }
    }

    func test_signedIn_withHTTPSURL() {
        // Arrange
        let state = AuthState.signedIn(serverURL: "https://pulse.example.com")

        // Assert
        if case .signedIn(let url) = state {
            XCTAssertTrue(url.hasPrefix("https://"))
        } else {
            XCTFail("Expected .signedIn")
        }
    }

    func test_signedIn_withPortNumber() {
        // Arrange
        let state = AuthState.signedIn(serverURL: "https://pulse.example.com:8443")

        // Assert
        if case .signedIn(let url) = state {
            XCTAssertTrue(url.contains(":8443"))
        } else {
            XCTFail("Expected .signedIn")
        }
    }

    // MARK: - Sendable Conformance

    func test_authState_isSendable() {
        // This test verifies that AuthState conforms to Sendable.
        // If it doesn't compile, AuthState is not Sendable.
        let state: any Sendable = AuthState.signedOut
        XCTAssertNotNil(state)

        let signedInState: any Sendable = AuthState.signedIn(serverURL: "https://example.com")
        XCTAssertNotNil(signedInState)
    }
}
