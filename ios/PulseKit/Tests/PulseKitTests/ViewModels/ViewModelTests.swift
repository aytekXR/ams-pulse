import Foundation
import Testing
@testable import PulseKit

/// Tests for Observation-based view models.
/// All tests use stub transports to avoid network access.
@Suite("ViewModels")
struct ViewModelTests {
    // MARK: - OverviewModel Tests

    @Test("OverviewModel initial state is idle")
    @MainActor
    func overviewModel_initialState() async {
        let transport = MockTransport()
        let client = PulseAPIClient(
            baseURL: URL(string: "https://test.example.com")!,
            transport: transport,
            tokenProvider: { "test-token" }
        )
        let model = OverviewModel(client: client)

        guard case .idle = model.state else {
            Issue.record("Expected idle state, got \(model.state)")
            return
        }
    }

    @Test("OverviewModel refresh transitions idle -> loading -> loaded")
    @MainActor
    func overviewModel_refresh_setsLoaded() async {
        let transport = MockTransport()
        await transport.setResponse(
            statusCode: 200,
            body: liveOverviewJSON.data(using: .utf8)!
        )

        let client = PulseAPIClient(
            baseURL: URL(string: "https://test.example.com")!,
            transport: transport,
            tokenProvider: { "test-token" }
        )
        let model = OverviewModel(client: client)

        await model.refresh()

        guard case .loaded(let overview) = model.state else {
            Issue.record("Expected loaded state, got \(model.state)")
            return
        }

        #expect(overview.totalViewers == 150)
        #expect(overview.totalPublishers == 10)
    }

    @Test("OverviewModel refresh transitions idle -> loading -> failed on error")
    @MainActor
    func overviewModel_refresh_setsFailed() async {
        let transport = MockTransport()
        await transport.setResponse(
            statusCode: 401,
            body: Data()
        )

        let client = PulseAPIClient(
            baseURL: URL(string: "https://test.example.com")!,
            transport: transport,
            tokenProvider: { "test-token" }
        )
        let model = OverviewModel(client: client)

        await model.refresh()

        guard case .failed(let error) = model.state else {
            Issue.record("Expected failed state, got \(model.state)")
            return
        }

        #expect(error == .unauthorized)
    }

    @Test("OverviewModel refresh after success keeps data on failure")
    @MainActor
    func overviewModel_refreshFailureKeepsData() async {
        let transport = MockTransport()

        // First load succeeds
        await transport.setResponse(
            statusCode: 200,
            body: liveOverviewJSON.data(using: .utf8)!
        )

        let client = PulseAPIClient(
            baseURL: URL(string: "https://test.example.com")!,
            transport: transport,
            tokenProvider: { "test-token" }
        )
        let model = OverviewModel(client: client)

        await model.refresh()

        guard case .loaded(let firstOverview) = model.state else {
            Issue.record("Expected loaded state after first refresh")
            return
        }

        #expect(firstOverview.totalViewers == 150)

        // Second load fails
        await transport.setResponse(statusCode: 500, body: Data())
        await model.refresh()

        // State should be failed, but lastLoaded should preserve the data
        guard case .failed(let error) = model.state else {
            Issue.record("Expected failed state after second refresh")
            return
        }
        #expect(error == .server(status: 500, apiError: nil))

        // The last loaded data should still be accessible
        #expect(model.lastLoaded?.totalViewers == 150)
    }

    // MARK: - StreamsModel Tests

    @Test("StreamsModel initial state is idle")
    @MainActor
    func streamsModel_initialState() async {
        let transport = MockTransport()
        let client = PulseAPIClient(
            baseURL: URL(string: "https://test.example.com")!,
            transport: transport,
            tokenProvider: { "test-token" }
        )
        let model = StreamsModel(client: client)

        guard case .idle = model.state else {
            Issue.record("Expected idle state, got \(model.state)")
            return
        }
        #expect(model.streams.isEmpty)
        #expect(model.hasMore)
    }

    @Test("StreamsModel refresh loads first page")
    @MainActor
    func streamsModel_refresh_clearsExisting() async {
        let transport = MockTransport()
        await transport.setResponse(
            statusCode: 200,
            body: liveStreamsPage1JSON.data(using: .utf8)!
        )

        let client = PulseAPIClient(
            baseURL: URL(string: "https://test.example.com")!,
            transport: transport,
            tokenProvider: { "test-token" }
        )
        let model = StreamsModel(client: client)

        await model.refresh()

        guard case .loaded = model.state else {
            Issue.record("Expected loaded state")
            return
        }

        #expect(model.streams.count == 2)
        #expect(model.nextCursor == "cursor-page-2")
        #expect(model.hasMore)
    }

    @Test("StreamsModel loadNextPage appends")
    @MainActor
    func streamsModel_loadNextPage_appends() async {
        let transport = MockTransport()
        await transport.setResponse(
            statusCode: 200,
            body: liveStreamsPage1JSON.data(using: .utf8)!
        )

        let client = PulseAPIClient(
            baseURL: URL(string: "https://test.example.com")!,
            transport: transport,
            tokenProvider: { "test-token" }
        )
        let model = StreamsModel(client: client)

        // First page
        await model.refresh()
        #expect(model.streams.count == 2)

        // Second page
        await transport.setResponse(
            statusCode: 200,
            body: liveStreamsPage2JSON.data(using: .utf8)!
        )
        await model.loadNextPage()

        #expect(model.streams.count == 3)
        #expect(model.nextCursor == nil)
        #expect(!model.hasMore)
    }

    @Test("StreamsModel empty next page terminates pagination")
    @MainActor
    func streamsModel_emptyPage_terminates() async {
        let transport = MockTransport()
        await transport.setResponse(
            statusCode: 200,
            body: liveStreamsEmptyJSON.data(using: .utf8)!
        )

        let client = PulseAPIClient(
            baseURL: URL(string: "https://test.example.com")!,
            transport: transport,
            tokenProvider: { "test-token" }
        )
        let model = StreamsModel(client: client)

        await model.refresh()

        #expect(model.streams.isEmpty)
        #expect(!model.hasMore)
        #expect(model.nextCursor == nil)
    }

    @Test("StreamsModel repeated cursor does not loop")
    @MainActor
    func streamsModel_repeatedCursor_noLoop() async {
        let transport = MockTransport()
        // Response always returns the same cursor
        await transport.setResponse(
            statusCode: 200,
            body: liveStreamsSameCursorJSON.data(using: .utf8)!
        )

        let client = PulseAPIClient(
            baseURL: URL(string: "https://test.example.com")!,
            transport: transport,
            tokenProvider: { "test-token" }
        )
        let model = StreamsModel(client: client)

        await model.refresh()
        let cursorAfterFirst = model.nextCursor

        // Try to load next - cursor hasn't changed
        await model.loadNextPage()
        let cursorAfterSecond = model.nextCursor

        // The model should detect the repeated cursor and stop
        #expect(cursorAfterFirst == cursorAfterSecond)
        // After detecting repeated cursor, hasMore should be false
        #expect(!model.hasMore)
    }

    @Test("StreamsModel search filters client-side")
    @MainActor
    func streamsModel_searchFiltersClientSide() async {
        let transport = MockTransport()
        await transport.setResponse(
            statusCode: 200,
            body: liveStreamsPage1JSON.data(using: .utf8)!
        )

        let client = PulseAPIClient(
            baseURL: URL(string: "https://test.example.com")!,
            transport: transport,
            tokenProvider: { "test-token" }
        )
        let model = StreamsModel(client: client)

        await model.refresh()
        let requestCountBefore = await transport.capturedRequests.count

        // Set search filter
        model.searchQuery = "stream-1"
        let filtered = model.filteredStreams

        // No additional network call should be made
        let requestCountAfter = await transport.capturedRequests.count
        #expect(requestCountAfter == requestCountBefore)

        // Only matching stream should be in filtered results
        #expect(filtered.count == 1)
        #expect(filtered.first?.streamId == "stream-1")
    }

    @Test("StreamsModel refresh failure after success keeps data")
    @MainActor
    func streamsModel_refreshFailureKeepsData() async {
        let transport = MockTransport()
        await transport.setResponse(
            statusCode: 200,
            body: liveStreamsPage1JSON.data(using: .utf8)!
        )

        let client = PulseAPIClient(
            baseURL: URL(string: "https://test.example.com")!,
            transport: transport,
            tokenProvider: { "test-token" }
        )
        let model = StreamsModel(client: client)

        await model.refresh()
        #expect(model.streams.count == 2)

        // Fail on next refresh
        await transport.setResponse(statusCode: 500, body: Data())
        await model.refresh()

        // State should be failed, but streams should still have previous data
        guard case .failed = model.state else {
            Issue.record("Expected failed state")
            return
        }
        // The model preserves last good data in the streams array
        #expect(model.lastLoadedStreams.count == 2)
    }

    // MARK: - AlertsModel Tests

    @Test("AlertsModel refresh clears existing")
    @MainActor
    func alertsModel_refresh_clearsExisting() async {
        let transport = MockTransport()
        await transport.setResponse(
            statusCode: 200,
            body: alertHistoryJSON.data(using: .utf8)!
        )

        let client = PulseAPIClient(
            baseURL: URL(string: "https://test.example.com")!,
            transport: transport,
            tokenProvider: { "test-token" }
        )
        let model = AlertsModel(client: client)

        await model.refresh()

        guard case .loaded = model.state else {
            Issue.record("Expected loaded state")
            return
        }

        #expect(model.alerts.count == 2)
    }

    @Test("AlertsModel pagination works")
    @MainActor
    func alertsModel_pagination_works() async {
        let transport = MockTransport()
        await transport.setResponse(
            statusCode: 200,
            body: alertHistoryPage1JSON.data(using: .utf8)!
        )

        let client = PulseAPIClient(
            baseURL: URL(string: "https://test.example.com")!,
            transport: transport,
            tokenProvider: { "test-token" }
        )
        let model = AlertsModel(client: client)

        await model.refresh()
        #expect(model.alerts.count == 1)
        #expect(model.hasMore)

        // Second page
        await transport.setResponse(
            statusCode: 200,
            body: alertHistoryPage2JSON.data(using: .utf8)!
        )
        await model.loadNextPage()

        #expect(model.alerts.count == 2)
        #expect(!model.hasMore)
    }

    // MARK: - ServerStore Tests

    @Test("ServerStore load populates servers")
    @MainActor
    func serverStore_load_populatesServers() async throws {
        let persistence = MockServerPersistence()
        let server1 = PulseServer(
            displayName: "Server 1",
            baseURL: URL(string: "https://s1.example.com")!
        )
        await persistence.setServers([server1])

        let store = ServerStore(persistence: persistence)
        try await store.load()

        #expect(store.servers.count == 1)
        #expect(store.servers.first?.displayName == "Server 1")
    }

    @Test("ServerStore add persists")
    @MainActor
    func serverStore_add_persistsCalled() async throws {
        let persistence = MockServerPersistence()
        let store = ServerStore(persistence: persistence)

        let server = PulseServer(
            displayName: "New Server",
            baseURL: URL(string: "https://new.example.com")!
        )

        try await store.add(server)

        #expect(store.servers.count == 1)
        let saved = await persistence.savedServers
        #expect(saved?.count == 1)
    }

    @Test("ServerStore rename updates server")
    @MainActor
    func serverStore_rename() async throws {
        let persistence = MockServerPersistence()
        let original = PulseServer(
            displayName: "Original",
            baseURL: URL(string: "https://orig.example.com")!
        )
        await persistence.setServers([original])

        let store = ServerStore(persistence: persistence)
        try await store.load()

        var updated = original
        updated.displayName = "Renamed"
        try await store.update(updated)

        #expect(store.servers.first?.displayName == "Renamed")
    }

    @Test("ServerStore delete updates selectedServer")
    @MainActor
    func serverStore_delete_updatesSelectedServer() async throws {
        let persistence = MockServerPersistence()
        let server1 = PulseServer(
            displayName: "Server 1",
            baseURL: URL(string: "https://s1.example.com")!
        )
        let server2 = PulseServer(
            displayName: "Server 2",
            baseURL: URL(string: "https://s2.example.com")!
        )
        await persistence.setServers([server1, server2])

        let store = ServerStore(persistence: persistence)
        try await store.load()
        store.selectedServerID = server1.id

        try await store.delete(id: server1.id)

        #expect(store.servers.count == 1)
        #expect(store.selectedServerID == nil)
    }

    @Test("ServerStore delete non-selected preserves selection")
    @MainActor
    func serverStore_delete_preservesSelection() async throws {
        let persistence = MockServerPersistence()
        let server1 = PulseServer(
            displayName: "Server 1",
            baseURL: URL(string: "https://s1.example.com")!
        )
        let server2 = PulseServer(
            displayName: "Server 2",
            baseURL: URL(string: "https://s2.example.com")!
        )
        await persistence.setServers([server1, server2])

        let store = ServerStore(persistence: persistence)
        try await store.load()
        store.selectedServerID = server1.id

        try await store.delete(id: server2.id)

        #expect(store.servers.count == 1)
        #expect(store.selectedServerID == server1.id)
    }

    @Test("ServerStore duplicate baseURL allowed")
    @MainActor
    func serverStore_duplicateBaseURL() async throws {
        let persistence = MockServerPersistence()
        let store = ServerStore(persistence: persistence)

        let server1 = PulseServer(
            displayName: "Prod 1",
            baseURL: URL(string: "https://pulse.example.com")!
        )
        let server2 = PulseServer(
            displayName: "Prod 2",
            baseURL: URL(string: "https://pulse.example.com")!
        )

        try await store.add(server1)
        try await store.add(server2)

        // Duplicate base URLs are allowed - user might have different tokens
        #expect(store.servers.count == 2)
    }

    @Test("ServerStore persistence round trip")
    @MainActor
    func serverStore_persistenceRoundTrip() async throws {
        let persistence = MockServerPersistence()
        let store = ServerStore(persistence: persistence)

        let server = PulseServer(
            displayName: "Round Trip",
            baseURL: URL(string: "https://rt.example.com")!,
            notes: "Test notes"
        )

        try await store.add(server)

        // Create new store with same persistence
        let store2 = ServerStore(persistence: persistence)
        try await store2.load()

        #expect(store2.servers.count == 1)
        #expect(store2.servers.first?.displayName == "Round Trip")
        #expect(store2.servers.first?.notes == "Test notes")
    }

    @Test("ServerStore selectedServer computed property")
    @MainActor
    func serverStore_selectedServer() async throws {
        let persistence = MockServerPersistence()
        let server = PulseServer(
            displayName: "Selected",
            baseURL: URL(string: "https://sel.example.com")!
        )
        await persistence.setServers([server])

        let store = ServerStore(persistence: persistence)
        try await store.load()

        #expect(store.selectedServer == nil)

        store.selectedServerID = server.id
        #expect(store.selectedServer?.displayName == "Selected")
    }

    // MARK: - Concurrency Safety Tests

    @Test("OverviewModel is MainActor safe")
    @MainActor
    func overviewModel_mainActorSafe() async {
        // This test verifies the model can be touched from MainActor
        // while a request might be in flight.
        let transport = MockTransport()
        await transport.setResponse(
            statusCode: 200,
            body: liveOverviewJSON.data(using: .utf8)!
        )

        let client = PulseAPIClient(
            baseURL: URL(string: "https://test.example.com")!,
            transport: transport,
            tokenProvider: { "test-token" }
        )
        let model = OverviewModel(client: client)

        // Start refresh (non-blocking)
        let task = Task { @MainActor in
            await model.refresh()
        }

        // Access state while refresh might be running
        // This should not cause data races because @MainActor
        _ = model.state

        await task.value
    }
}

// MARK: - Mock ServerPersistence

actor MockServerPersistence: ServerPersistence {
    private var servers: [PulseServer] = []
    private(set) var savedServers: [PulseServer]?

    func setServers(_ servers: [PulseServer]) {
        self.servers = servers
    }

    func loadServers() async throws -> [PulseServer] {
        servers
    }

    func saveServers(_ servers: [PulseServer]) async throws {
        self.servers = servers
        self.savedServers = servers
    }
}

// MARK: - JSON Fixtures

private let liveOverviewJSON = """
{
    "ts": 1700000000000,
    "total_viewers": 150,
    "total_publishers": 10,
    "protocol_mix": {"webrtc": 100, "hls": 50},
    "apps": [{"app": "live", "viewers": 150, "publishers": 10, "streams": 5}],
    "nodes": [{"node_id": "node-1", "status": "up"}]
}
"""

private let liveStreamsPage1JSON = """
{
    "items": [
        {"stream_id": "stream-1", "app": "live", "viewers": 100, "publisher_state": "publishing", "health_score": 0.95},
        {"stream_id": "stream-2", "app": "live", "viewers": 50, "publisher_state": "idle", "health_score": 0.90}
    ],
    "meta": {"next_cursor": "cursor-page-2", "total": 3}
}
"""

private let liveStreamsPage2JSON = """
{
    "items": [
        {"stream_id": "stream-3", "app": "live", "viewers": 25, "publisher_state": "publishing", "health_score": 0.85}
    ],
    "meta": {"next_cursor": null, "total": 3}
}
"""

private let liveStreamsEmptyJSON = """
{
    "items": [],
    "meta": {"next_cursor": null, "total": 0}
}
"""

private let liveStreamsSameCursorJSON = """
{
    "items": [
        {"stream_id": "stream-1", "app": "live", "viewers": 100, "publisher_state": "publishing", "health_score": 0.95}
    ],
    "meta": {"next_cursor": "same-cursor", "total": 1}
}
"""

private let alertHistoryJSON = """
{
    "items": [
        {"id": "alert-1", "rule_id": "rule-1", "state": "firing", "severity": "critical", "ts": 1700000000000, "metric": "cpu_pct", "value": 95.0, "threshold": 90.0},
        {"id": "alert-2", "rule_id": "rule-2", "state": "resolved", "severity": "warning", "ts": 1699999000000, "metric": "mem_pct", "value": 80.0, "threshold": 85.0}
    ],
    "meta": {"next_cursor": null, "total": 2}
}
"""

private let alertHistoryPage1JSON = """
{
    "items": [
        {"id": "alert-1", "rule_id": "rule-1", "state": "firing", "severity": "critical", "ts": 1700000000000, "metric": "cpu_pct", "value": 95.0, "threshold": 90.0}
    ],
    "meta": {"next_cursor": "page-2", "total": 2}
}
"""

private let alertHistoryPage2JSON = """
{
    "items": [
        {"id": "alert-2", "rule_id": "rule-2", "state": "resolved", "severity": "warning", "ts": 1699999000000, "metric": "mem_pct", "value": 80.0, "threshold": 85.0}
    ],
    "meta": {"next_cursor": null, "total": 2}
}
"""
