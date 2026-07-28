import Foundation

// MARK: - QueryInterval

/// Query interval for time-series endpoints.
public enum QueryInterval: String, Sendable {
    case hour
    case day
}

// MARK: - PulseAPIClient

/// Actor-based API client for the Pulse API.
///
/// All methods are async and throw `PulseAPIError` on failure.
/// The client is thread-safe through actor isolation.
///
/// ## Usage
///
/// ```swift
/// let client = PulseAPIClient(
///     baseURL: URL(string: "https://pulse.example.com")!,
///     tokenProvider: { await tokenStore.getToken(forServerID: serverId)! }
/// )
///
/// let overview = try await client.getLiveOverview()
/// ```
public actor PulseAPIClient {
    private let baseURL: URL
    private let transport: PulseTransport
    private let tokenProvider: @Sendable () async throws -> String
    private let userAgent: String
    private let timeout: TimeInterval

    /// Create a new API client.
    ///
    /// - Parameters:
    ///   - baseURL: The base URL of the Pulse server. Trailing slashes are normalized.
    ///   - transport: The transport to use for requests. Defaults to URLSession.
    ///   - tokenProvider: A closure that provides the bearer token for requests.
    ///   - userAgent: The User-Agent header value. Defaults to "PulseKit/1.0".
    ///   - timeout: Request timeout in seconds. Defaults to 30.
    public init(
        baseURL: URL,
        transport: PulseTransport = URLSessionTransport(),
        tokenProvider: @escaping @Sendable () async throws -> String,
        userAgent: String = "PulseKit/1.0",
        timeout: TimeInterval = 30
    ) {
        // Normalize: strip trailing slash from baseURL path
        var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: false)!
        if components.path.hasSuffix("/") {
            components.path = String(components.path.dropLast())
        }
        self.baseURL = components.url!
        self.transport = transport
        self.tokenProvider = tokenProvider
        self.userAgent = userAgent
        self.timeout = timeout
    }

    // MARK: - Live Endpoints

    /// Get the live overview snapshot.
    ///
    /// - Parameters:
    ///   - app: Filter by application name.
    ///   - node: Filter by node ID.
    ///   - tenant: Filter by tenant.
    /// - Returns: The live overview data.
    public func getLiveOverview(
        app: String? = nil,
        node: String? = nil,
        tenant: String? = nil
    ) async throws -> LiveOverview {
        var query: [(String, String)] = []
        if let app = app { query.append(("app", app)) }
        if let node = node { query.append(("node", node)) }
        if let tenant = tenant { query.append(("tenant", tenant)) }

        return try await get(path: "/api/v1/live/overview", query: query)
    }

    /// Get the list of active streams.
    ///
    /// - Parameters:
    ///   - app: Filter by application name.
    ///   - node: Filter by node ID.
    ///   - tenant: Filter by tenant.
    ///   - limit: Maximum number of results.
    ///   - cursor: Pagination cursor.
    /// - Returns: The paginated list of streams.
    public func getLiveStreams(
        app: String? = nil,
        node: String? = nil,
        tenant: String? = nil,
        limit: Int? = nil,
        cursor: String? = nil
    ) async throws -> LiveStreamList {
        var query: [(String, String)] = []
        if let app = app { query.append(("app", app)) }
        if let node = node { query.append(("node", node)) }
        if let tenant = tenant { query.append(("tenant", tenant)) }
        if let limit = limit { query.append(("limit", String(limit))) }
        if let cursor = cursor { query.append(("cursor", cursor)) }

        return try await get(path: "/api/v1/live/streams", query: query)
    }

    // MARK: - QoE Endpoints

    /// Get QoE summary metrics.
    ///
    /// - Parameters:
    ///   - from: Start timestamp (epoch ms).
    ///   - to: End timestamp (epoch ms).
    ///   - app: Filter by application.
    ///   - stream: Filter by stream ID.
    ///   - tenant: Filter by tenant.
    ///   - interval: Aggregation interval.
    ///   - country: Filter by country code.
    ///   - device: Filter by device type.
    /// - Returns: The QoE summary response.
    public func getQoeSummary(
        from: Int64? = nil,
        to: Int64? = nil,
        app: String? = nil,
        stream: String? = nil,
        tenant: String? = nil,
        interval: QueryInterval? = nil,
        country: String? = nil,
        device: String? = nil
    ) async throws -> QoeSummaryResponse {
        var query: [(String, String)] = []
        if let from = from { query.append(("from", String(from))) }
        if let to = to { query.append(("to", String(to))) }
        if let app = app { query.append(("app", app)) }
        if let stream = stream { query.append(("stream", stream)) }
        if let tenant = tenant { query.append(("tenant", tenant)) }
        if let interval = interval { query.append(("interval", interval.rawValue)) }
        if let country = country { query.append(("country", country)) }
        if let device = device { query.append(("device", device)) }

        return try await get(path: "/api/v1/qoe/summary", query: query)
    }

    // MARK: - Alert Endpoints

    /// Get alert history.
    ///
    /// - Parameters:
    ///   - from: Start timestamp (epoch ms).
    ///   - to: End timestamp (epoch ms).
    ///   - limit: Maximum number of results.
    ///   - cursor: Pagination cursor.
    ///   - ruleId: Filter by rule ID.
    ///   - state: Filter by alert state.
    /// - Returns: The paginated alert history.
    public func getAlertHistory(
        from: Int64? = nil,
        to: Int64? = nil,
        limit: Int? = nil,
        cursor: String? = nil,
        ruleId: String? = nil,
        state: AlertState? = nil
    ) async throws -> AlertHistoryList {
        var query: [(String, String)] = []
        if let from = from { query.append(("from", String(from))) }
        if let to = to { query.append(("to", String(to))) }
        if let limit = limit { query.append(("limit", String(limit))) }
        if let cursor = cursor { query.append(("cursor", cursor)) }
        if let ruleId = ruleId { query.append(("rule_id", ruleId)) }
        if let state = state { query.append(("state", state.rawValue)) }

        return try await get(path: "/api/v1/alerts/history", query: query)
    }

    // MARK: - Fleet Endpoints

    /// Get fleet nodes.
    ///
    /// - Parameters:
    ///   - limit: Maximum number of results.
    ///   - cursor: Pagination cursor.
    /// - Returns: The paginated list of nodes.
    public func getFleetNodes(
        limit: Int? = nil,
        cursor: String? = nil
    ) async throws -> FleetNodeList {
        var query: [(String, String)] = []
        if let limit = limit { query.append(("limit", String(limit))) }
        if let cursor = cursor { query.append(("cursor", cursor)) }

        return try await get(path: "/api/v1/fleet/nodes", query: query)
    }

    // MARK: - Anomaly Endpoints

    /// Get anomaly flags.
    ///
    /// - Parameters:
    ///   - from: Start timestamp (epoch ms).
    ///   - to: End timestamp (epoch ms).
    ///   - app: Filter by application.
    ///   - stream: Filter by stream ID.
    ///   - limit: Maximum number of results.
    ///   - cursor: Pagination cursor.
    ///   - metric: Filter by metric name.
    ///   - minSigma: Minimum sigma threshold.
    /// - Returns: The paginated list of anomalies.
    public func getAnomalies(
        from: Int64? = nil,
        to: Int64? = nil,
        app: String? = nil,
        stream: String? = nil,
        limit: Int? = nil,
        cursor: String? = nil,
        metric: String? = nil,
        minSigma: Double? = nil
    ) async throws -> AnomalyList {
        var query: [(String, String)] = []
        if let from = from { query.append(("from", String(from))) }
        if let to = to { query.append(("to", String(to))) }
        if let app = app { query.append(("app", app)) }
        if let stream = stream { query.append(("stream", stream)) }
        if let limit = limit { query.append(("limit", String(limit))) }
        if let cursor = cursor { query.append(("cursor", cursor)) }
        if let metric = metric { query.append(("metric", metric)) }
        if let minSigma = minSigma { query.append(("min_sigma", String(minSigma))) }

        return try await get(path: "/api/v1/anomalies", query: query)
    }

    // MARK: - Health Endpoint (Root-Mounted, No Auth)

    /// Get server health status.
    ///
    /// This endpoint is UNAUTHENTICATED - no bearer token is sent.
    /// Use it to verify the server is reachable before prompting for credentials.
    ///
    /// - Returns: The health status.
    public func getHealth() async throws -> HealthStatus {
        return try await getUnauthenticated(path: "/healthz")
    }

    // MARK: - Auth Endpoint (Root-Mounted)

    /// Get the current authenticated user.
    ///
    /// - Returns: The authentication response.
    public func getAuthMe() async throws -> AuthMeResponse {
        return try await get(path: "/auth/me", query: [])
    }

    // MARK: - Admin Endpoints

    /// Get license information.
    ///
    /// - Returns: The license info.
    public func getLicense() async throws -> LicenseInfo {
        return try await get(path: "/api/v1/admin/license", query: [])
    }

    // MARK: - Private Request Helpers

    /// Perform an authenticated GET request.
    private func get<T: Decodable>(path: String, query: [(String, String)]) async throws -> T {
        let url = try buildURL(path: path, query: query)
        let token = try await tokenProvider()

        let headers: [String: String] = [
            "Authorization": "Bearer \(token)",
            "User-Agent": userAgent,
            "Accept": "application/json"
        ]

        let request = TransportRequest(
            method: "GET",
            url: url,
            headers: headers,
            body: nil,
            timeoutInterval: timeout
        )

        return try await execute(request)
    }

    /// Perform an unauthenticated GET request (for health endpoint).
    private func getUnauthenticated<T: Decodable>(path: String) async throws -> T {
        let url = try buildURL(path: path, query: [])

        let headers: [String: String] = [
            "User-Agent": userAgent,
            "Accept": "application/json"
        ]

        let request = TransportRequest(
            method: "GET",
            url: url,
            headers: headers,
            body: nil,
            timeoutInterval: timeout
        )

        return try await execute(request)
    }

    /// Build a URL from path and query parameters.
    private func buildURL(path: String, query: [(String, String)]) throws -> URL {
        // Join baseURL path with the endpoint path
        var components = URLComponents()
        components.scheme = baseURL.scheme
        components.host = baseURL.host
        components.port = baseURL.port

        // Combine base path with endpoint path
        let basePath = baseURL.path
        components.path = basePath + path

        // Add query parameters if present
        if !query.isEmpty {
            components.queryItems = query.map { URLQueryItem(name: $0.0, value: $0.1) }
        }

        guard let url = components.url else {
            throw PulseAPIError.invalidServerURL(baseURL.absoluteString + path)
        }

        return url
    }

    /// Execute a request and decode the response.
    private func execute<T: Decodable>(_ request: TransportRequest) async throws -> T {
        let response: TransportResponse

        do {
            response = try await transport.send(request)
        } catch is CancellationError {
            throw PulseAPIError.cancelled
        } catch let urlError as URLError where urlError.code == .cancelled {
            throw PulseAPIError.cancelled
        } catch {
            throw PulseAPIError.transport(description: error.localizedDescription)
        }

        // Map HTTP status codes to errors
        switch response.statusCode {
        case 200..<300:
            // Success - decode the response
            break
        case 401:
            throw PulseAPIError.unauthorized
        case 403:
            throw PulseAPIError.forbidden
        case 404:
            throw PulseAPIError.notFound
        case 429:
            let retryAfter = parseRetryAfter(from: response.headers)
            throw PulseAPIError.rateLimited(retryAfter: retryAfter)
        case 500..<600:
            let apiError = try? JSONDecoder().decode(APIError.self, from: response.body)
            throw PulseAPIError.server(status: response.statusCode, apiError: apiError)
        default:
            throw PulseAPIError.server(status: response.statusCode, apiError: nil)
        }

        // Decode the response body
        do {
            let decoder = JSONDecoder()
            return try decoder.decode(T.self, from: response.body)
        } catch let decodingError as DecodingError {
            throw PulseAPIError.decoding(context: decodingErrorContext(decodingError))
        } catch {
            throw PulseAPIError.decoding(context: error.localizedDescription)
        }
    }

    /// Parse the Retry-After header value.
    private func parseRetryAfter(from headers: [String: String]) -> Int? {
        guard let value = headers["retry-after"] else { return nil }
        return Int(value)
    }

    /// Extract a user-friendly context from a DecodingError.
    private func decodingErrorContext(_ error: DecodingError) -> String {
        switch error {
        case .keyNotFound(let key, _):
            return "missing required field '\(key.stringValue)'"
        case .typeMismatch(let type, let context):
            let path = context.codingPath.map { $0.stringValue }.joined(separator: ".")
            return "type mismatch at '\(path)': expected \(type)"
        case .valueNotFound(let type, let context):
            let path = context.codingPath.map { $0.stringValue }.joined(separator: ".")
            return "null value at '\(path)': expected \(type)"
        case .dataCorrupted(let context):
            return "corrupted data: \(context.debugDescription)"
        @unknown default:
            return "unknown decoding error"
        }
    }
}
