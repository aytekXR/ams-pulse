import Foundation
@testable import PulseKit

/// A stub transport for testing PulseAPIClient without network access.
/// Captures requests and returns canned responses.
public actor MockTransport: PulseTransport {
    /// All requests captured by this transport.
    public private(set) var capturedRequests: [TransportRequest] = []

    /// The canned response to return from `send(_:)`.
    private var response: TransportResponse?

    /// An error to throw instead of returning a response.
    private var errorToThrow: (any Error)?

    /// Whether to simulate task cancellation.
    private var simulateCancellation: Bool = false

    public init() {}

    /// Configure a successful response.
    public func setResponse(
        statusCode: Int,
        headers: [String: String] = [:],
        body: Data
    ) {
        self.response = TransportResponse(
            statusCode: statusCode,
            headers: headers,
            body: body
        )
        self.errorToThrow = nil
        self.simulateCancellation = false
    }

    /// Configure an error to throw.
    public func setError(_ error: any Error) {
        self.errorToThrow = error
        self.response = nil
        self.simulateCancellation = false
    }

    /// Configure to simulate task cancellation.
    public func setCancellation() {
        self.simulateCancellation = true
        self.response = nil
        self.errorToThrow = nil
    }

    /// Clear captured requests.
    public func clearRequests() {
        capturedRequests = []
    }

    /// Return the last captured request, if any.
    public var lastRequest: TransportRequest? {
        capturedRequests.last
    }

    // MARK: - PulseTransport

    public func send(_ request: TransportRequest) async throws -> TransportResponse {
        capturedRequests.append(request)

        // Check for simulated cancellation first
        if simulateCancellation {
            throw CancellationError()
        }

        // Check for configured error
        if let error = errorToThrow {
            throw error
        }

        // Return configured response
        guard let response = response else {
            fatalError("MockTransport: no response configured. Call setResponse() before send().")
        }

        return response
    }
}

/// A URLError subtype for testing network errors.
extension URLError {
    static var networkConnectionLost: URLError {
        URLError(.networkConnectionLost)
    }

    static var timedOut: URLError {
        URLError(.timedOut)
    }
}
