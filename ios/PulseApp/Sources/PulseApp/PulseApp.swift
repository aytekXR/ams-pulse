import SwiftUI

// MARK: - Pulse App
//
// The main entry point for the Pulse iOS app.
//
// Architecture:
//   - @main marks this as the app entry point.
//   - AppState is created here and injected into the environment.
//   - ContentView switches between ConnectView and MainTabView based on auth state.
//
// Design decisions:
//   - Single AppState instance shared via environment (not @StateObject in views).
//   - AppState is @Observable (iOS 17+), not ObservableObject, for efficiency.
//   - The app delegates credential storage to KeychainService.
//   - On launch, AppState automatically attempts to restore a saved session.

@main
struct PulseApp: App {

    // MARK: - State

    /// The shared app state, created once and injected into the view hierarchy.
    /// Using @State here because AppState is a class, and we want it to persist
    /// across view updates.
    @State private var appState = AppState()

    // MARK: - Body

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(\.appState, appState)
                // Apply brand colors to system UI elements.
                .tint(BrandColors.signal)
                // Set preferred color scheme based on system setting.
                // The app respects the user's system-wide dark/light mode choice.
                .preferredColorScheme(nil) // nil = follow system
        }
    }
}
