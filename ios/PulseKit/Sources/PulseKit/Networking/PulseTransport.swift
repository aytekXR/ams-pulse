import Foundation

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

// MARK: - TransportRequest

/// A request value type passed to `PulseTransport.send`.
public struct TransportRequest: Sendable {
    /// HTTP method: "GET", "POST", etc.
    public let method: String

    /// The full URL including query parameters.
    public let url: URL

    /// HTTP headers. Keys are case-sensitive as provided.
    public let headers: [String: String]

    /// Optional request body for POST/PUT/PATCH.
    public let body: Data?

    /// Timeout interval in seconds.
    public let timeoutInterval: TimeInterval

    public init(
        method: String,
        url: URL,
        headers: [String: String] = [:],
        body: Data? = nil,
        timeoutInterval: TimeInterval = 30
    ) {
        self.method = method
        self.url = url
        self.headers = headers
        self.body = body
        self.timeoutInterval = timeoutInterval
    }
}

// MARK: - TransportResponse

/// A response value type returned by `PulseTransport.send`.
public struct TransportResponse: Sendable {
    /// HTTP status code.
    public let statusCode: Int

    /// Response headers. Keys are lowercased for consistency.
    public let headers: [String: String]

    /// Response body data.
    public let body: Data

    public init(
        statusCode: Int,
        headers: [String: String] = [:],
        body: Data
    ) {
        self.statusCode = statusCode
        self.headers = headers
        self.body = body
    }
}

// MARK: - PulseTransport Protocol

/// The transport protocol for making HTTP requests.
///
/// Implementations must be `Sendable` for use with Swift concurrency.
/// The default implementation uses `URLSession`.
public protocol PulseTransport: Sendable {
    /// Send a request and return a response.
    ///
    /// - Parameter request: The request to send.
    /// - Returns: The response from the server.
    /// - Throws: Transport-level errors (network issues, cancellation).
    func send(_ request: TransportRequest) async throws -> TransportResponse
}

// MARK: - URLSessionTransport

/// URLSession-based implementation of `PulseTransport`.
///
/// This is the default transport for production use. Tests use a mock transport.
public struct URLSessionTransport: PulseTransport {
    private let session: URLSession

    public init(session: URLSession = .shared) {
        self.session = session
    }

    public func send(_ request: TransportRequest) async throws -> TransportResponse {
        var urlRequest = URLRequest(url: request.url)
        urlRequest.httpMethod = request.method
        urlRequest.httpBody = request.body
        urlRequest.timeoutInterval = request.timeoutInterval

        for (key, value) in request.headers {
            urlRequest.setValue(value, forHTTPHeaderField: key)
        }

        let (data, response) = try await session.data(for: urlRequest)

        guard let http = response as? HTTPURLResponse else {
            throw PulseAPIError.transport(description: "Non-HTTP response")
        }

        // Normalize header keys to lowercase for consistent access
        let headers = http.allHeaderFields.reduce(into: [String: String]()) { dict, pair in
            if let k = pair.key as? String, let v = pair.value as? String {
                dict[k.lowercased()] = v
            }
        }

        return TransportResponse(statusCode: http.statusCode, headers: headers, body: data)
    }
}
