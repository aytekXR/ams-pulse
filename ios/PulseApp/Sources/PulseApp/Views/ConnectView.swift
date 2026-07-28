import SwiftUI
import PulseKit

// MARK: - Connect View
//
// The sign-in screen where users enter their Pulse server URL and API token.
//
// UX design choices:
//   1. Clear error messages that distinguish "server unreachable" from "bad token"
//      — these require different user actions.
//   2. Server URL field uses URL keyboard and auto-corrects common mistakes.
//   3. Token field is secure (dots) but has a reveal toggle for verification.
//   4. Validation happens on submit, not inline, to avoid premature errors.
//   5. Loading state prevents double-submit and shows progress.
//
// The view owns local text field state and delegates to AppState.signIn() on submit.

struct ConnectView: View {

    // MARK: - Environment

    @Environment(\.appState) private var appState

    // MARK: - Local State

    @State private var serverURL: String = ""
    @State private var token: String = ""
    @State private var isLoading: Bool = false
    @State private var errorMessage: String?
    @State private var showToken: Bool = false

    // MARK: - Body

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 32) {
                    // Logo and tagline.
                    headerSection

                    // Input fields.
                    inputSection

                    // Error display.
                    if let errorMessage = errorMessage {
                        errorView(message: errorMessage)
                    }

                    // Connect button.
                    connectButton
                }
                .padding(.horizontal, 24)
                .padding(.vertical, 48)
            }
            .background(BrandColors.bg.ignoresSafeArea())
            .navigationTitle("Connect to Pulse")
            .navigationBarTitleDisplayMode(.inline)
        }
    }

    // MARK: - Subviews

    private var headerSection: some View {
        VStack(spacing: 16) {
            // Placeholder for logo — the actual logo asset is not available on Linux.
            // See Resources/README.md for asset requirements.
            Image(systemName: "waveform.path.ecg")
                .font(.system(size: 64, weight: .light))
                .foregroundColor(BrandColors.signal)

            Text("Pulse")
                .font(.system(size: 32, weight: .bold))
                .foregroundColor(BrandColors.textPrimary)

            Text("Self-hosted analytics for Ant Media Server")
                .font(.system(size: 14, weight: .regular))
                .foregroundColor(BrandColors.textSecondary)
                .multilineTextAlignment(.center)
        }
    }

    private var inputSection: some View {
        VStack(spacing: 20) {
            // Server URL field.
            VStack(alignment: .leading, spacing: 8) {
                Text("Server URL")
                    .font(.system(size: 12, weight: .medium))
                    .foregroundColor(BrandColors.textSecondary)
                    .textCase(.uppercase)

                TextField("https://pulse.example.com", text: $serverURL)
                    .textFieldStyle(.plain)
                    .padding(16)
                    .background(BrandColors.surface)
                    .cornerRadius(12)
                    .overlay(
                        RoundedRectangle(cornerRadius: 12)
                            .stroke(BrandColors.border, lineWidth: 1)
                    )
                    .keyboardType(.URL)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .foregroundColor(BrandColors.textPrimary)
            }

            // API token field.
            VStack(alignment: .leading, spacing: 8) {
                Text("API Token")
                    .font(.system(size: 12, weight: .medium))
                    .foregroundColor(BrandColors.textSecondary)
                    .textCase(.uppercase)

                HStack(spacing: 12) {
                    Group {
                        if showToken {
                            TextField("Bearer token", text: $token)
                        } else {
                            SecureField("Bearer token", text: $token)
                        }
                    }
                    .textFieldStyle(.plain)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .foregroundColor(BrandColors.textPrimary)

                    Button {
                        showToken.toggle()
                    } label: {
                        Image(systemName: showToken ? "eye.slash" : "eye")
                            .foregroundColor(BrandColors.textMuted)
                    }
                }
                .padding(16)
                .background(BrandColors.surface)
                .cornerRadius(12)
                .overlay(
                    RoundedRectangle(cornerRadius: 12)
                        .stroke(BrandColors.border, lineWidth: 1)
                )
            }
        }
    }

    private func errorView(message: String) -> some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundColor(BrandColors.critical)

            Text(message)
                .font(.system(size: 14, weight: .regular))
                .foregroundColor(BrandColors.critical)
                .multilineTextAlignment(.leading)

            Spacer()
        }
        .padding(16)
        .background(BrandColors.critical.opacity(0.1))
        .cornerRadius(12)
    }

    private var connectButton: some View {
        Button {
            Task {
                await connect()
            }
        } label: {
            HStack(spacing: 12) {
                if isLoading {
                    ProgressView()
                        .tint(BrandColors.onSignal)
                }
                Text(isLoading ? "Connecting..." : "Connect")
                    .font(.system(size: 16, weight: .semibold))
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, 16)
            .background(isValid ? BrandColors.signal : BrandColors.textMuted)
            .foregroundColor(BrandColors.onSignal)
            .cornerRadius(12)
        }
        .disabled(!isValid || isLoading)
    }

    // MARK: - Validation

    private var isValid: Bool {
        !serverURL.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty &&
        !token.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    // MARK: - Actions

    private func connect() async {
        guard let appState = appState else { return }

        isLoading = true
        errorMessage = nil

        do {
            try await appState.signIn(
                serverURL: serverURL,
                token: token
            )
            // Success — AppState will switch to signedIn, and the root view
            // will automatically transition away from ConnectView.
        } catch let error as PulseAPIError {
            // Map PulseAPIError to user-friendly messages.
            errorMessage = userFriendlyError(error)
        } catch {
            errorMessage = "An unexpected error occurred. Please try again."
        }

        isLoading = false
    }

    /// Maps PulseAPIError to user-actionable messages.
    ///
    /// The goal is to tell the user what to do, not dump technical details.
    /// A wrong token and an unreachable server require different actions.
    private func userFriendlyError(_ error: PulseAPIError) -> String {
        switch error {
        case .invalidServerURL:
            return "The server URL is not valid. Please check the format."

        case .insecureTransport(let host):
            return "Cannot connect to '\(host)' over HTTP. Use HTTPS, or connect to a local address."

        case .unauthorized:
            // 401 — token is invalid, expired, or missing.
            return "Invalid API token. Please check that you entered the correct token from your Pulse server's API settings."

        case .forbidden:
            // 403 — token is valid but lacks permissions.
            return "Access denied. Your API token does not have permission to access this resource. Check that the token has the required scopes."

        case .notFound:
            // 404 — endpoint doesn't exist.
            return "The Pulse API was not found at this URL. Please verify the server URL is correct and that Pulse is running."

        case .rateLimited(let retryAfter):
            if let seconds = retryAfter {
                return "Too many requests. Please wait \(seconds) seconds and try again."
            }
            return "Too many requests. Please wait and try again."

        case .server(let status, _):
            // 5xx — server error.
            return "The server returned an error (HTTP \(status)). Please try again later or check the server logs."

        case .transport(let description):
            // Network error — most common for self-hosted servers.
            // Could be DNS, TLS, firewall, server not running, etc.
            let lowered = description.lowercased()
            if lowered.contains("ssl") || lowered.contains("tls") || lowered.contains("certificate") {
                return "Could not establish a secure connection. If your server uses a self-signed certificate, you may need to trust it in your device settings."
            } else if lowered.contains("timeout") {
                return "Connection timed out. Please check that the server is running and reachable from your network."
            } else {
                return "Could not connect to the server. Please check the URL and your network connection.\n\nDetails: \(description)"
            }

        case .decoding(let context):
            // JSON parsing failed — likely a version mismatch.
            return "Received an unexpected response from the server. This may indicate a version mismatch between the app and server.\n\nDetails: \(context)"

        case .cancelled:
            return "The connection was cancelled."
        }
    }
}

#Preview {
    ConnectView()
        .environment(\.appState, AppState())
}
