import Foundation
import Observation

// MARK: - OverviewModel

/// View model for the live overview dashboard.
///
/// Driven by `PulseAPIClient.getLiveOverview()`. The model tracks:
/// - Current loading state (idle/loading/loaded/failed)
/// - Last successfully loaded data (preserved across failures)
///
/// ## Concurrency
///
/// Annotated `@MainActor` for all mutations because SwiftUI observes from the main thread.
/// This is correct on Linux because:
/// 1. The `@Observable` macro generates observation machinery
/// 2. All state changes happen synchronously on MainActor
/// 3. The actor isolation is enforced at compile time, not via platform threading
///
/// The `@unchecked Sendable` conformance is safe because:
/// - All mutable state is protected by `@MainActor` isolation
/// - The `client` is an actor with its own isolation
@Observable
public final class OverviewModel: @unchecked Sendable {
    /// Current loading state.
    public private(set) var state: LoadingState<LiveOverview> = .idle

    /// Last successfully loaded overview (preserved across refresh failures).
    ///
    /// When a refresh fails after a successful load, the UI can continue showing
    /// the last good data while indicating staleness via the `state`.
    public private(set) var lastLoaded: LiveOverview?

    private let client: PulseAPIClient

    /// Create an overview model.
    ///
    /// - Parameter client: The API client to use for fetching data.
    public init(client: PulseAPIClient) {
        self.client = client
    }

    /// Refresh the live overview data.
    ///
    /// State transitions:
    /// - On success: `idle` -> `loading` -> `loaded(overview)`
    /// - On failure: `idle` -> `loading` -> `failed(error)`
    ///
    /// If a refresh fails after a previous success, `lastLoaded` preserves the
    /// previous data so the UI can continue displaying it.
    @MainActor
    public func refresh() async {
        state = .loading
        do {
            let overview = try await client.getLiveOverview()
            lastLoaded = overview
            state = .loaded(overview)
        } catch let error as PulseAPIError {
            state = .failed(error)
        } catch {
            state = .failed(.transport(description: error.localizedDescription))
        }
    }
}
