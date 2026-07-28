import Foundation
import PulseKit

// MARK: - App State
//
// Central state object for the Pulse app. Manages:
//   1. Authentication state (signed out vs. signed in)
//   2. The PulseAPIClient actor instance
//   3. Server connection info for the Settings screen
//
// Design note: This is an @Observable class (iOS 17+) rather than ObservableObject
// because @Observable is more efficient — it tracks property access at the
// property level, not the whole object. The entire class is @MainActor isolated
// because SwiftUI views must update on the main thread.
//
// Swift 6 concurrency: @MainActor ensures all property access is on the main
// thread. The PulseAPIClient actor is safe to call from here because actor method
// calls are always async and properly isolated.

import SwiftUI

/// Authentication state for the app.
public enum AuthState: Sendable {
    /// User has not connected to a Pulse server.
    case signedOut
    /// User is connected to a Pulse server.
    case signedIn(serverURL: String)
}

/// Central app state, shared across all views via the environment.
@MainActor
@Observable
public final class AppState {

    // MARK: - Published State

    /// Current authentication state.
    public private(set) var authState: AuthState = .signedOut

    /// The PulseAPIClient actor, available only when signed in.
    /// Views should check `authState` before using this.
    public private(set) var api: PulseAPIClient?

    /// Server health status, fetched on sign-in and periodically.
    /// Nil before the first health check completes.
    public private(set) var health: HealthStatus?

    /// Error from the last operation, if any.
    /// Views can display this and clear it when the user acknowledges.
    public var lastError: PulseAPIError?

    // MARK: - Private State

    /// The currently stored token (needed to create API client).
    private var storedToken: String?

    // MARK: - Initialization

    public init() {
        // Check for stored credentials on launch.
        Task {
            await restoreSession()
        }
    }

    // MARK: - Session Management

    /// Attempts to restore a session from stored Keychain credentials.
    ///
    /// Called automatically on init. If credentials exist and the server is
    /// reachable, transitions to `.signedIn`. Otherwise stays `.signedOut`.
    public func restoreSession() async {
        do {
            guard let credentials = try KeychainService.getCredentials() else {
                // No stored credentials — stay signed out.
                return
            }

            guard let url = URL(string: credentials.serverURL) else {
                // Malformed URL in keychain — clear it.
                try? KeychainService.deleteCredentials()
                return
            }

            // Store the token for the tokenProvider closure.
            self.storedToken = credentials.token

            // Create API client with a token provider that captures self.
            // PulseAPIClient.init requires a Sendable tokenProvider closure.
            let token = credentials.token
            let client = PulseAPIClient(
                baseURL: url,
                tokenProvider: { token }
            )
            let healthStatus = try await client.getHealth()

            // Connection successful — sign in.
            self.api = client
            self.health = healthStatus
            self.authState = .signedIn(serverURL: credentials.serverURL)

        } catch let error as PulseAPIError {
            // Connection failed — stay signed out but don't clear credentials.
            // The server might just be temporarily unreachable.
            self.lastError = error
        } catch {
            // Unexpected error (e.g., Keychain failure).
            // Stay signed out.
        }
    }

    /// Connects to a Pulse server and stores credentials on success.
    ///
    /// - Parameters:
    ///   - serverURL: The base URL of the Pulse server.
    ///   - token: The bearer token for API authentication.
    /// - Throws: `PulseAPIError` if connection fails.
    public func signIn(serverURL: String, token: String) async throws {
        // Normalize URL: ensure no trailing slash.
        var normalizedURL = serverURL.trimmingCharacters(in: .whitespacesAndNewlines)
        if normalizedURL.hasSuffix("/") {
            normalizedURL = String(normalizedURL.dropLast())
        }

        guard let url = URL(string: normalizedURL) else {
            throw PulseAPIError.invalidServerURL(normalizedURL)
        }

        // Create API client with the provided token.
        let client = PulseAPIClient(
            baseURL: url,
            tokenProvider: { token }
        )

        // Verify health first (unauthenticated — proves server is reachable).
        let healthStatus = try await client.getHealth()

        // Then verify token works (authenticated endpoint).
        // getLiveOverview requires auth — if the token is bad, we'll get .unauthorized.
        _ = try await client.getLiveOverview()

        // Both checks passed — store credentials and sign in.
        try KeychainService.saveCredentials(serverURL: normalizedURL, token: token)

        self.storedToken = token
        self.api = client
        self.health = healthStatus
        self.authState = .signedIn(serverURL: normalizedURL)
        self.lastError = nil
    }

    /// Signs out and clears stored credentials.
    public func signOut() {
        try? KeychainService.deleteCredentials()
        self.storedToken = nil
        self.api = nil
        self.health = nil
        self.authState = .signedOut
        self.lastError = nil
    }

    /// Refreshes the health status from the server.
    ///
    /// Call this periodically or when the user explicitly requests a refresh.
    public func refreshHealth() async {
        guard let api = api else { return }
        do {
            self.health = try await api.getHealth()
        } catch let error as PulseAPIError {
            self.lastError = error
        } catch {
            // Ignore non-PulseAPIError errors for health checks.
        }
    }
}

// MARK: - Environment Key

/// Environment key for accessing AppState throughout the view hierarchy.
private struct AppStateKey: EnvironmentKey {
    static let defaultValue: AppState? = nil
}

extension EnvironmentValues {
    /// The shared app state, injected at the root of the view hierarchy.
    public var appState: AppState? {
        get { self[AppStateKey.self] }
        set { self[AppStateKey.self] = newValue }
    }
}
