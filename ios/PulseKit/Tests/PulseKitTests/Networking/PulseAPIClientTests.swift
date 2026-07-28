import Foundation
import Testing
@testable import PulseKit

/// Tests for PulseAPIClient - the API client actor.
@Suite("PulseAPIClient Tests")
struct PulseAPIClientTests {

    // MARK: - Test Helpers

    /// Creates a client with the given transport and test token.
    private func makeClient(
        baseURL: URL = URL(string: "https://pulse.example.com")!,
        transport: MockTransport,
        token: String = "test-bearer-token"
    ) -> PulseAPIClient {
        PulseAPIClient(
            baseURL: baseURL,
            transport: transport,
            tokenProvider: { token },
            userAgent: "PulseKitTests/1.0",
            timeout: 30
        )
    }

    // MARK: - URL Construction

    @Test("getLiveOverview uses correct path")
    func getLiveOverview_correctPath() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        // Set up successful response with fixture data
        let body = try loadFixture("live_overview.json")
        await transport.setResponse(statusCode: 200, body: body)

        _ = try await client.getLiveOverview()

        let request = await transport.lastRequest
        #expect(request?.url.path == "/api/v1/live/overview")
    }

    @Test("getLiveOverview includes bearer header")
    func getLiveOverview_bearerHeader() async throws {
        let transport = MockTransport()
        let secretToken = "my-super-secret-token"
        let client = PulseAPIClient(
            baseURL: URL(string: "https://pulse.example.com")!,
            transport: transport,
            tokenProvider: { secretToken },
            userAgent: "PulseKitTests/1.0",
            timeout: 30
        )

        let body = try loadFixture("live_overview.json")
        await transport.setResponse(statusCode: 200, body: body)

        _ = try await client.getLiveOverview()

        let request = await transport.lastRequest
        #expect(request?.headers["Authorization"] == "Bearer \(secretToken)")
    }

    @Test("getLiveStreams includes query params")
    func getLiveStreams_queryParams() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("live_streams.json")
        await transport.setResponse(statusCode: 200, body: body)

        _ = try await client.getLiveStreams(
            app: "live",
            node: "node-1",
            tenant: "acme",
            limit: 50,
            cursor: "abc123"
        )

        let request = await transport.lastRequest
        let components = URLComponents(url: request!.url, resolvingAgainstBaseURL: false)!
        let queryItems = components.queryItems ?? []

        #expect(queryItems.contains { $0.name == "app" && $0.value == "live" })
        #expect(queryItems.contains { $0.name == "node" && $0.value == "node-1" })
        #expect(queryItems.contains { $0.name == "tenant" && $0.value == "acme" })
        #expect(queryItems.contains { $0.name == "limit" && $0.value == "50" })
        #expect(queryItems.contains { $0.name == "cursor" && $0.value == "abc123" })
    }

    @Test("getQoeSummary includes all query params")
    func getQoeSummary_allQueryParams() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("qoe_summary.json")
        await transport.setResponse(statusCode: 200, body: body)

        _ = try await client.getQoeSummary(
            from: 1700000000000,
            to: 1700003600000,
            app: "live",
            stream: "stream-001",
            tenant: "acme",
            interval: .hour,
            country: "US",
            device: "mobile"
        )

        let request = await transport.lastRequest
        let components = URLComponents(url: request!.url, resolvingAgainstBaseURL: false)!
        let queryItems = components.queryItems ?? []

        #expect(queryItems.contains { $0.name == "from" && $0.value == "1700000000000" })
        #expect(queryItems.contains { $0.name == "to" && $0.value == "1700003600000" })
        #expect(queryItems.contains { $0.name == "app" && $0.value == "live" })
        #expect(queryItems.contains { $0.name == "stream" && $0.value == "stream-001" })
        #expect(queryItems.contains { $0.name == "tenant" && $0.value == "acme" })
        #expect(queryItems.contains { $0.name == "interval" && $0.value == "hour" })
        #expect(queryItems.contains { $0.name == "country" && $0.value == "US" })
        #expect(queryItems.contains { $0.name == "device" && $0.value == "mobile" })
    }

    @Test("getAlertHistory includes state filter")
    func getAlertHistory_stateFilter() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("alerts_history.json")
        await transport.setResponse(statusCode: 200, body: body)

        _ = try await client.getAlertHistory(state: .firing)

        let request = await transport.lastRequest
        let components = URLComponents(url: request!.url, resolvingAgainstBaseURL: false)!
        let queryItems = components.queryItems ?? []

        #expect(queryItems.contains { $0.name == "state" && $0.value == "firing" })
    }

    @Test("getFleetNodes includes pagination params")
    func getFleetNodes_pagination() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("fleet_nodes.json")
        await transport.setResponse(statusCode: 200, body: body)

        _ = try await client.getFleetNodes(limit: 25, cursor: "page2")

        let request = await transport.lastRequest
        let components = URLComponents(url: request!.url, resolvingAgainstBaseURL: false)!
        let queryItems = components.queryItems ?? []

        #expect(queryItems.contains { $0.name == "limit" && $0.value == "25" })
        #expect(queryItems.contains { $0.name == "cursor" && $0.value == "page2" })
    }

    @Test("getAnomalies includes minSigma param")
    func getAnomalies_minSigmaParam() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("anomalies.json")
        await transport.setResponse(statusCode: 200, body: body)

        _ = try await client.getAnomalies(minSigma: 3.5)

        let request = await transport.lastRequest
        let components = URLComponents(url: request!.url, resolvingAgainstBaseURL: false)!
        let queryItems = components.queryItems ?? []

        #expect(queryItems.contains { $0.name == "min_sigma" && $0.value == "3.5" })
    }

    @Test("getHealth has no auth header")
    func getHealth_noAuthHeader() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("healthz.json")
        await transport.setResponse(statusCode: 200, body: body)

        _ = try await client.getHealth()

        let request = await transport.lastRequest
        #expect(request?.headers["Authorization"] == nil)
    }

    @Test("getHealth is root-mounted (no /api/v1 prefix)")
    func getHealth_rootMounted() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("healthz.json")
        await transport.setResponse(statusCode: 200, body: body)

        _ = try await client.getHealth()

        let request = await transport.lastRequest
        #expect(request?.url.path == "/healthz")
    }

    @Test("getAuthMe is root-mounted")
    func getAuthMe_rootMounted() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("auth_me.json")
        await transport.setResponse(statusCode: 200, body: body)

        _ = try await client.getAuthMe()

        let request = await transport.lastRequest
        #expect(request?.url.path == "/auth/me")
    }

    @Test("getLicense uses /api/v1 path")
    func getLicense_apiV1Path() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("license.json")
        await transport.setResponse(statusCode: 200, body: body)

        _ = try await client.getLicense()

        let request = await transport.lastRequest
        #expect(request?.url.path == "/api/v1/admin/license")
    }

    @Test("baseURL with trailing slash is normalized")
    func baseURLWithTrailingSlash_normalized() async throws {
        let transport = MockTransport()
        let client = PulseAPIClient(
            baseURL: URL(string: "https://pulse.example.com/")!,  // Trailing slash
            transport: transport,
            tokenProvider: { "token" },
            userAgent: "Test",
            timeout: 30
        )

        let body = try loadFixture("live_overview.json")
        await transport.setResponse(statusCode: 200, body: body)

        _ = try await client.getLiveOverview()

        let request = await transport.lastRequest
        // Should not have double slashes
        #expect(request?.url.absoluteString == "https://pulse.example.com/api/v1/live/overview")
    }

    @Test("baseURL with path prefix is joined correctly")
    func baseURLWithPathPrefix_joined() async throws {
        let transport = MockTransport()
        let client = PulseAPIClient(
            baseURL: URL(string: "https://proxy.example.com/pulse")!,  // Path prefix
            transport: transport,
            tokenProvider: { "token" },
            userAgent: "Test",
            timeout: 30
        )

        let body = try loadFixture("live_overview.json")
        await transport.setResponse(statusCode: 200, body: body)

        _ = try await client.getLiveOverview()

        let request = await transport.lastRequest
        #expect(request?.url.absoluteString == "https://proxy.example.com/pulse/api/v1/live/overview")
    }

    @Test("User-Agent header is included")
    func userAgentHeader_included() async throws {
        let transport = MockTransport()
        let client = PulseAPIClient(
            baseURL: URL(string: "https://pulse.example.com")!,
            transport: transport,
            tokenProvider: { "token" },
            userAgent: "MyApp/2.0",
            timeout: 30
        )

        let body = try loadFixture("live_overview.json")
        await transport.setResponse(statusCode: 200, body: body)

        _ = try await client.getLiveOverview()

        let request = await transport.lastRequest
        #expect(request?.headers["User-Agent"] == "MyApp/2.0")
    }

    @Test("timeout is applied to request")
    func timeout_appliedToRequest() async throws {
        let transport = MockTransport()
        let client = PulseAPIClient(
            baseURL: URL(string: "https://pulse.example.com")!,
            transport: transport,
            tokenProvider: { "token" },
            userAgent: "Test",
            timeout: 45
        )

        let body = try loadFixture("live_overview.json")
        await transport.setResponse(statusCode: 200, body: body)

        _ = try await client.getLiveOverview()

        let request = await transport.lastRequest
        #expect(request?.timeoutInterval == 45)
    }

    // MARK: - URL Encoding Edge Cases

    @Test("cursor with special characters is percent-encoded")
    func cursorWithSpecialChars_encoded() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("live_streams.json")
        await transport.setResponse(statusCode: 200, body: body)

        // Cursor that requires escaping: = and & must be encoded
        let cursor = "abc=def&ghi"
        _ = try await client.getLiveStreams(cursor: cursor)

        let request = await transport.lastRequest
        let urlString = request!.url.absoluteString
        // The cursor should be percent-encoded for = and &
        // %3D is =, %26 is &
        #expect(urlString.contains("cursor=abc%3Ddef%26ghi"))
    }

    @Test("query params with unicode are encoded correctly")
    func unicodeQueryParams_encoded() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("live_streams.json")
        await transport.setResponse(statusCode: 200, body: body)

        _ = try await client.getLiveStreams(app: "live")

        let request = await transport.lastRequest
        #expect(request?.url.absoluteString.contains("app=live") == true)
    }

    // MARK: - Failure Paths

    @Test("401 throws unauthorized")
    func status401_throwsUnauthorized() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("error_401.json")
        await transport.setResponse(statusCode: 401, body: body)

        do {
            _ = try await client.getLiveOverview()
            Issue.record("Expected unauthorized error")
        } catch let error as PulseAPIError {
            #expect(error == .unauthorized)
        }
    }

    @Test("403 throws forbidden")
    func status403_throwsForbidden() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = Data("""
            {"code": "FORBIDDEN", "message": "Insufficient permissions"}
            """.utf8)
        await transport.setResponse(statusCode: 403, body: body)

        do {
            _ = try await client.getLiveOverview()
            Issue.record("Expected forbidden error")
        } catch let error as PulseAPIError {
            #expect(error == .forbidden)
        }
    }

    @Test("404 throws notFound")
    func status404_throwsNotFound() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = Data("""
            {"code": "NOT_FOUND", "message": "Resource not found"}
            """.utf8)
        await transport.setResponse(statusCode: 404, body: body)

        do {
            _ = try await client.getLiveOverview()
            Issue.record("Expected notFound error")
        } catch let error as PulseAPIError {
            #expect(error == .notFound)
        }
    }

    @Test("429 throws rateLimited")
    func status429_throwsRateLimited() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("error_429.json")
        await transport.setResponse(statusCode: 429, body: body)

        do {
            _ = try await client.getLiveOverview()
            Issue.record("Expected rateLimited error")
        } catch let error as PulseAPIError {
            guard case .rateLimited = error else {
                Issue.record("Expected rateLimited, got \(error)")
                return
            }
        }
    }

    @Test("429 parses Retry-After header")
    func status429_parsesRetryAfterHeader() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("error_429.json")
        await transport.setResponse(
            statusCode: 429,
            headers: ["retry-after": "60"],
            body: body
        )

        do {
            _ = try await client.getLiveOverview()
            Issue.record("Expected rateLimited error")
        } catch let error as PulseAPIError {
            guard case .rateLimited(let retryAfter) = error else {
                Issue.record("Expected rateLimited, got \(error)")
                return
            }
            #expect(retryAfter == 60)
        }
    }

    @Test("500 throws server error with parsed body")
    func status500_throwsServer() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = Data("""
            {"code": "INTERNAL_ERROR", "message": "Database connection failed"}
            """.utf8)
        await transport.setResponse(statusCode: 500, body: body)

        do {
            _ = try await client.getLiveOverview()
            Issue.record("Expected server error")
        } catch let error as PulseAPIError {
            guard case .server(let status, let apiError) = error else {
                Issue.record("Expected server error, got \(error)")
                return
            }
            #expect(status == 500)
            #expect(apiError?.code == "INTERNAL_ERROR")
            #expect(apiError?.message == "Database connection failed")
        }
    }

    @Test("502 throws server error with nil apiError")
    func status502_throwsServerWithNilError() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        // HTML response instead of JSON (common with load balancers)
        let body = Data("<html><body>Bad Gateway</body></html>".utf8)
        await transport.setResponse(statusCode: 502, body: body)

        do {
            _ = try await client.getLiveOverview()
            Issue.record("Expected server error")
        } catch let error as PulseAPIError {
            guard case .server(let status, let apiError) = error else {
                Issue.record("Expected server error, got \(error)")
                return
            }
            #expect(status == 502)
            #expect(apiError == nil)
        }
    }

    @Test("malformed JSON throws decoding error")
    func malformedJSON_throwsDecoding() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = Data("{invalid json".utf8)
        await transport.setResponse(statusCode: 200, body: body)

        do {
            _ = try await client.getLiveOverview()
            Issue.record("Expected decoding error")
        } catch let error as PulseAPIError {
            guard case .decoding = error else {
                Issue.record("Expected decoding error, got \(error)")
                return
            }
        }
    }

    @Test("empty body throws decoding error")
    func emptyBody_throwsDecoding() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        await transport.setResponse(statusCode: 200, body: Data())

        do {
            _ = try await client.getLiveOverview()
            Issue.record("Expected decoding error")
        } catch let error as PulseAPIError {
            guard case .decoding = error else {
                Issue.record("Expected decoding error, got \(error)")
                return
            }
        }
    }

    @Test("truncated JSON throws decoding error")
    func truncatedJSON_throwsDecoding() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        // Truncated JSON (missing closing braces)
        let body = Data("""
            {"ts": 1700000000000, "total_viewers": 1234
            """.utf8)
        await transport.setResponse(statusCode: 200, body: body)

        do {
            _ = try await client.getLiveOverview()
            Issue.record("Expected decoding error")
        } catch let error as PulseAPIError {
            guard case .decoding = error else {
                Issue.record("Expected decoding error, got \(error)")
                return
            }
        }
    }

    @Test("network error throws transport error")
    func networkError_throwsTransport() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        await transport.setError(URLError(.networkConnectionLost))

        do {
            _ = try await client.getLiveOverview()
            Issue.record("Expected transport error")
        } catch let error as PulseAPIError {
            guard case .transport = error else {
                Issue.record("Expected transport error, got \(error)")
                return
            }
        }
    }

    @Test("task cancellation throws cancelled error")
    func taskCancellation_throwsCancelled() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        await transport.setCancellation()

        do {
            _ = try await client.getLiveOverview()
            Issue.record("Expected cancelled error")
        } catch let error as PulseAPIError {
            guard case .cancelled = error else {
                Issue.record("Expected cancelled error, got \(error)")
                return
            }
        }
    }

    // MARK: - Token Security

    @Test("bearer token never appears in error description")
    func bearerToken_neverInErrorDescription() async throws {
        let transport = MockTransport()
        let sensitiveToken = "sk_live_extremely_secret_token_that_must_not_leak"

        let client = PulseAPIClient(
            baseURL: URL(string: "https://pulse.example.com")!,
            transport: transport,
            tokenProvider: { sensitiveToken },
            userAgent: "Test",
            timeout: 30
        )

        // Test various error scenarios
        let errorScenarios: [(Int, Data)] = [
            (401, try loadFixture("error_401.json")),
            (403, Data("{\"code\": \"FORBIDDEN\", \"message\": \"No\"}".utf8)),
            (404, Data("{\"code\": \"NOT_FOUND\", \"message\": \"Gone\"}".utf8)),
            (429, try loadFixture("error_429.json")),
            (500, Data("{\"code\": \"ERR\", \"message\": \"Oops\"}".utf8)),
        ]

        for (status, body) in errorScenarios {
            await transport.setResponse(statusCode: status, body: body)

            do {
                _ = try await client.getLiveOverview()
            } catch let error as PulseAPIError {
                let description = error.errorDescription ?? ""
                #expect(!description.contains(sensitiveToken),
                       "Token leaked in error description for status \(status): \(description)")
            }
        }

        // Test transport error
        await transport.setError(URLError(.timedOut))
        do {
            _ = try await client.getLiveOverview()
        } catch let error as PulseAPIError {
            let description = error.errorDescription ?? ""
            #expect(!description.contains(sensitiveToken),
                   "Token leaked in transport error: \(description)")
        }

        // Test decoding error
        await transport.setResponse(statusCode: 200, body: Data("{bad json".utf8))
        do {
            _ = try await client.getLiveOverview()
        } catch let error as PulseAPIError {
            let description = error.errorDescription ?? ""
            #expect(!description.contains(sensitiveToken),
                   "Token leaked in decoding error: \(description)")
        }
    }

    // MARK: - Response Decoding

    @Test("getLiveOverview decodes fixture correctly")
    func getLiveOverview_decodesFixture() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("live_overview.json")
        await transport.setResponse(statusCode: 200, body: body)

        let overview = try await client.getLiveOverview()

        #expect(overview.ts == 1700000000000)
        #expect(overview.totalViewers == 1234)
        #expect(overview.totalPublishers == 56)
        #expect(overview.protocolMix.webrtc == 500)
        #expect(overview.apps.count == 2)
        #expect(overview.nodes.count == 2)
    }

    @Test("getLiveStreams decodes fixture correctly")
    func getLiveStreams_decodesFixture() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("live_streams.json")
        await transport.setResponse(statusCode: 200, body: body)

        let list = try await client.getLiveStreams()

        #expect(list.items.count == 2)
        #expect(list.items[0].streamId == "stream-001")
        #expect(list.meta.nextCursor == "eyJsYXN0X2lkIjoic3RyZWFtLTAwMiJ9")
        #expect(list.meta.total == 150)
    }

    @Test("getHealth decodes fixture correctly")
    func getHealth_decodesFixture() async throws {
        let transport = MockTransport()
        let client = makeClient(transport: transport)

        let body = try loadFixture("healthz.json")
        await transport.setResponse(statusCode: 200, body: body)

        let health = try await client.getHealth()

        #expect(health.status == .ok)
        #expect(health.amsEnvConfigured == true)
        #expect(health.components.clickhouse.status == .ok)
        #expect(health.components.collector.status == .degraded)
    }

    // MARK: - Helpers

    private func loadFixture(_ name: String) throws -> Data {
        // Load fixture from the test bundle's copied resources
        guard let url = Bundle.module.url(forResource: name, withExtension: nil, subdirectory: "Fixtures") else {
            throw FixtureLoadError.fileNotFound(name)
        }
        return try Data(contentsOf: url)
    }
}

// MARK: - FixtureLoadError

enum FixtureLoadError: Error, LocalizedError {
    case fileNotFound(String)

    var errorDescription: String? {
        switch self {
        case .fileNotFound(let name):
            return "Fixture file not found: \(name)"
        }
    }
}
