import SwiftUI

// MARK: - Main Tab View
//
// The main navigation structure for authenticated users.
//
// Three tabs:
//   1. Live — real-time dashboard and stream list
//   2. Alerts — recent alert history
//   3. Settings — server info and sign out
//
// The tab bar uses brand colors for the selected state. Tab icons are
// SF Symbols chosen for clarity at small sizes.

struct MainTabView: View {

    // MARK: - State

    @State private var selectedTab: Tab = .live

    // MARK: - Tab Definition

    enum Tab: String, CaseIterable {
        case live
        case alerts
        case settings

        var title: String {
            switch self {
            case .live: return "Live"
            case .alerts: return "Alerts"
            case .settings: return "Settings"
            }
        }

        var icon: String {
            switch self {
            case .live: return "waveform.path.ecg"
            case .alerts: return "bell.fill"
            case .settings: return "gearshape.fill"
            }
        }
    }

    // MARK: - Body

    var body: some View {
        TabView(selection: $selectedTab) {
            LiveView()
                .tabItem {
                    Label(Tab.live.title, systemImage: Tab.live.icon)
                }
                .tag(Tab.live)

            AlertsView()
                .tabItem {
                    Label(Tab.alerts.title, systemImage: Tab.alerts.icon)
                }
                .tag(Tab.alerts)

            SettingsView()
                .tabItem {
                    Label(Tab.settings.title, systemImage: Tab.settings.icon)
                }
                .tag(Tab.settings)
        }
        .tint(BrandColors.signal)
    }
}

// MARK: - Preview

#Preview {
    MainTabView()
        .environment(\.appState, AppState())
}
