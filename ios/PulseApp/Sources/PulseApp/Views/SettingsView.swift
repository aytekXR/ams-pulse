import SwiftUI
import PulseKit

// MARK: - Settings View
//
// App settings and server information.
//
// Features:
//   - Server connection info (URL, health status, component status)
//   - Sign out button (clears Keychain credentials)
//   - App version and build info
//
// This is a simple settings screen for the initial TestFlight release.
// Future versions may add:
//   - Push notification preferences
//   - Refresh interval settings
//   - Theme selection (system/light/dark)
//   - About/credits screen

struct SettingsView: View {

    // MARK: - Environment

    @Environment(\.appState) private var appState

    // MARK: - State

    @State private var showingSignOutConfirmation: Bool = false

    // MARK: - Body

    var body: some View {
        NavigationStack {
            List {
                // Server section.
                serverSection

                // Components health section (if available).
                if let health = appState?.health {
                    componentsSection(health: health)
                }

                // App section.
                appSection

                // Sign out section.
                signOutSection
            }
            .listStyle(.insetGrouped)
            .background(BrandColors.bg.ignoresSafeArea())
            .scrollContentBackground(.hidden)
            .navigationTitle("Settings")
            .navigationBarTitleDisplayMode(.large)
            .confirmationDialog(
                "Sign Out",
                isPresented: $showingSignOutConfirmation,
                titleVisibility: .visible
            ) {
                Button("Sign Out", role: .destructive) {
                    appState?.signOut()
                }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("This will disconnect from the Pulse server and clear your stored credentials.")
            }
        }
    }

    // MARK: - Server Section

    private var serverSection: some View {
        Section {
            if case .signedIn(let serverURL) = appState?.authState {
                // Server URL.
                HStack {
                    Label("Server", systemImage: "server.rack")
                        .foregroundColor(BrandColors.textPrimary)
                    Spacer()
                    Text(serverURL)
                        .font(.system(size: 14, weight: .regular))
                        .foregroundColor(BrandColors.textSecondary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                }

                // Overall health status.
                if let health = appState?.health {
                    HStack {
                        Label("Status", systemImage: "heart.fill")
                            .foregroundColor(BrandColors.textPrimary)
                        Spacer()
                        statusBadge(status: health.status)
                    }
                }
            }
        } header: {
            Text("Server")
        }
        .listRowBackground(BrandColors.surface)
    }

    // MARK: - Components Section

    private func componentsSection(health: HealthStatus) -> some View {
        Section {
            componentRow(name: "ClickHouse", status: health.components.clickhouse)
            componentRow(name: "Meta Store", status: health.components.metaStore)
            componentRow(name: "Collector", status: health.components.collector)

            if let kafka = health.components.kafka {
                kafkaRow(kafka: kafka)
            }
        } header: {
            Text("Components")
        }
        .listRowBackground(BrandColors.surface)
    }

    private func componentRow(name: String, status: ComponentStatus) -> some View {
        HStack {
            Text(name)
                .foregroundColor(BrandColors.textPrimary)

            Spacer()

            // Latency (if available).
            if let latencyMs = status.latencyMs {
                Text(Formatters.latencyMs(latencyMs))
                    .font(.system(size: 12, weight: .regular))
                    .foregroundColor(BrandColors.textMuted)
                    .padding(.trailing, 8)
            }

            statusBadge(status: status.status)
        }
    }

    private func kafkaRow(kafka: KafkaComponentStatus) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("Kafka")
                    .foregroundColor(BrandColors.textPrimary)

                Spacer()

                statusBadge(status: kafka.status)
            }

            // Kafka-specific metrics.
            HStack(spacing: 16) {
                if let lag = kafka.lag {
                    HStack(spacing: 4) {
                        Text("Lag:")
                            .font(.system(size: 11, weight: .medium))
                            .foregroundColor(BrandColors.textMuted)
                        Text(Formatters.abbreviatedCount(lag))
                            .font(.system(size: 11, weight: .semibold))
                            .foregroundColor(lag > 1000 ? BrandColors.warning : BrandColors.textSecondary)
                    }
                }

                if let parseErrors = kafka.parseErrors, parseErrors > 0 {
                    HStack(spacing: 4) {
                        Text("Parse errors:")
                            .font(.system(size: 11, weight: .medium))
                            .foregroundColor(BrandColors.textMuted)
                        Text("\(parseErrors)")
                            .font(.system(size: 11, weight: .semibold))
                            .foregroundColor(BrandColors.warning)
                    }
                }
            }
        }
    }

    private func statusBadge(status: HealthState) -> some View {
        let (text, bgColor, fgColor): (String, Color, Color) = {
            switch status {
            case .ok:
                return ("OK", BrandColors.healthy.opacity(0.15), BrandColors.healthy)
            case .degraded:
                return ("Degraded", BrandColors.warning.opacity(0.15), BrandColors.warning)
            case .down:
                return ("Down", BrandColors.critical.opacity(0.15), BrandColors.critical)
            }
        }()

        return Text(text)
            .font(.system(size: 11, weight: .semibold))
            .foregroundColor(fgColor)
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
            .background(bgColor)
            .cornerRadius(4)
    }

    // MARK: - App Section

    private var appSection: some View {
        Section {
            // App version.
            HStack {
                Label("Version", systemImage: "info.circle")
                    .foregroundColor(BrandColors.textPrimary)
                Spacer()
                Text(appVersion)
                    .font(.system(size: 14, weight: .regular))
                    .foregroundColor(BrandColors.textSecondary)
            }

            // Build number.
            HStack {
                Label("Build", systemImage: "hammer")
                    .foregroundColor(BrandColors.textPrimary)
                Spacer()
                Text(buildNumber)
                    .font(.system(size: 14, weight: .regular))
                    .foregroundColor(BrandColors.textSecondary)
            }
        } header: {
            Text("App")
        }
        .listRowBackground(BrandColors.surface)
    }

    // MARK: - Sign Out Section

    private var signOutSection: some View {
        Section {
            Button {
                showingSignOutConfirmation = true
            } label: {
                HStack {
                    Spacer()
                    Text("Sign Out")
                        .font(.system(size: 16, weight: .semibold))
                        .foregroundColor(BrandColors.critical)
                    Spacer()
                }
            }
        }
        .listRowBackground(BrandColors.surface)
    }

    // MARK: - Version Info

    /// App version from Info.plist (CFBundleShortVersionString).
    private var appVersion: String {
        Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "1.0.0"
    }

    /// Build number from Info.plist (CFBundleVersion).
    private var buildNumber: String {
        Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "1"
    }
}

// MARK: - Preview

#Preview {
    SettingsView()
        .environment(\.appState, AppState())
}
