import Foundation
import Observation

// MARK: - AlertsModel

/// View model for alert history with pagination.
///
/// Driven by `PulseAPIClient.getAlertHistory()`. Supports cursor-based pagination.
///
/// ## Concurrency
///
/// See `OverviewModel` for concurrency justification.
@Observable
public final class AlertsModel: @unchecked Sendable {
    /// Current loading state.
    public private(set) var state: LoadingState<[AlertHistoryEntry]> = .idle

    /// All loaded alerts (accumulated across pages).
    public private(set) var alerts: [AlertHistoryEntry] = []

    /// Cursor for the next page, or nil if no more pages.
    public private(set) var nextCursor: String?

    /// Whether more pages are available.
    public private(set) var hasMore: Bool = true

    private let client: PulseAPIClient

    /// Create an alerts model.
    ///
    /// - Parameter client: The API client to use for fetching data.
    public init(client: PulseAPIClient) {
        self.client = client
    }

    /// Refresh the alert history (loads first page).
    ///
    /// Clears all existing data before loading.
    @MainActor
    public func refresh() async {
        alerts = []
        nextCursor = nil
        hasMore = true
        await loadPage()
    }

    /// Load the next page of alerts.
    ///
    /// Only loads if `hasMore` is true and the current state is `loaded`.
    @MainActor
    public func loadNextPage() async {
        guard hasMore, case .loaded = state else { return }
        await loadPage()
    }

    /// Internal page loading logic.
    @MainActor
    private func loadPage() async {
        state = .loading
        do {
            let list = try await client.getAlertHistory(cursor: nextCursor)
            alerts.append(contentsOf: list.items)
            nextCursor = list.meta.nextCursor
            hasMore = list.meta.nextCursor != nil
            state = .loaded(alerts)
        } catch let error as PulseAPIError {
            state = .failed(error)
        } catch {
            state = .failed(.transport(description: error.localizedDescription))
        }
    }
}
