import SwiftUI

// MARK: - Content View
//
// Root view that switches between sign-in (ConnectView) and the main app
// (MainTabView) based on authentication state.
//
// The transition is automatic: when AppState.authState changes from
// .signedOut to .signedIn (or vice versa), SwiftUI re-evaluates the body
// and shows the appropriate view.
//
// A smooth cross-fade animation makes the transition feel polished.

struct ContentView: View {

    // MARK: - Environment

    @Environment(\.appState) private var appState

    // MARK: - Body

    var body: some View {
        Group {
            switch appState?.authState ?? .signedOut {
            case .signedOut:
                ConnectView()
            case .signedIn:
                MainTabView()
            }
        }
        // Animate transitions between auth states.
        .animation(.easeInOut(duration: 0.3), value: isSignedIn)
    }

    /// Helper to make animation work with enum.
    private var isSignedIn: Bool {
        if case .signedIn = appState?.authState {
            return true
        }
        return false
    }
}

// MARK: - Preview

#Preview("Signed Out") {
    ContentView()
        .environment(\.appState, AppState())
}
