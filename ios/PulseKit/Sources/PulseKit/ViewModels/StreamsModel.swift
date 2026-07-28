import Foundation
import Observation

// MARK: - StreamsModel

/// View model for the live streams list with pagination.
///
/// Driven by `PulseAPIClient.getLiveStreams()`. Supports:
/// - Cursor-based pagination with automatic termination
/// - Client-side search filtering (no network call for filter changes)
/// - Preservation of last good data across refresh failures
///
/// ## Pagination
///
/// - `refresh()` clears all data and loads the first page
/// - `loadNextPage()` appends the next page when `hasMore` is true
/// - Pagination terminates when `nextCursor` is nil or empty
/// - Repeated cursor detection prevents infinite loops
///
/// ## Search Filtering
///
/// The `searchQuery` property enables client-side filtering. When set:
/// - `filteredStreams` returns only streams matching the query
/// - No additional network call is made
/// - Filtering matches against `streamId` and `app` fields
///
/// ## Concurrency
///
/// See `OverviewModel` for concurrency justification.
@Observable
public final class StreamsModel: @unchecked Sendable {
    /// Current loading state.
    public private(set) var state: LoadingState<[LiveStream]> = .idle

    /// All loaded streams (accumulated across pages).
    public private(set) var streams: [LiveStream] = []

    /// Last successfully loaded streams (preserved across refresh failures).
    public private(set) var lastLoadedStreams: [LiveStream] = []

    /// Cursor for the next page, or nil if no more pages.
    public private(set) var nextCursor: String?

    /// Whether more pages are available.
    public private(set) var hasMore: Bool = true

    /// Client-side search query.
    ///
    /// Setting this filters `filteredStreams` without a network call.
    public var searchQuery: String = ""

    /// Streams filtered by `searchQuery`.
    ///
    /// This is computed client-side from `streams`. If `searchQuery` is empty,
    /// returns all streams.
    public var filteredStreams: [LiveStream] {
        guard !searchQuery.isEmpty else { return streams }
        let query = searchQuery.lowercased()
        return streams.filter {
            $0.streamId.lowercased().contains(query) ||
            $0.app.lowercased().contains(query)
        }
    }

    private let client: PulseAPIClient

    /// Create a streams model.
    ///
    /// - Parameter client: The API client to use for fetching data.
    public init(client: PulseAPIClient) {
        self.client = client
    }

    /// Refresh the stream list (loads first page).
    ///
    /// Clears all existing data before loading.
    @MainActor
    public func refresh() async {
        // Preserve last good data before clearing
        if !streams.isEmpty {
            lastLoadedStreams = streams
        }
        streams = []
        nextCursor = nil
        hasMore = true
        await loadPage()
    }

    /// Load the next page of streams.
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
        // Capture the cursor we're about to use for this request
        let requestCursor = nextCursor

        state = .loading
        do {
            let list = try await client.getLiveStreams(cursor: requestCursor)

            // Detect repeated cursor (infinite loop protection)
            // If the response cursor equals the request cursor, we'd loop forever
            if let responseCursor = list.meta.nextCursor,
               responseCursor == requestCursor {
                // Same cursor returned - terminate pagination to prevent infinite loop
                streams.append(contentsOf: list.items)
                nextCursor = responseCursor
                hasMore = false
                lastLoadedStreams = streams
                state = .loaded(streams)
                return
            }

            streams.append(contentsOf: list.items)
            nextCursor = list.meta.nextCursor
            hasMore = list.meta.nextCursor != nil
            lastLoadedStreams = streams
            state = .loaded(streams)
        } catch let error as PulseAPIError {
            state = .failed(error)
        } catch {
            state = .failed(.transport(description: error.localizedDescription))
        }
    }
}
