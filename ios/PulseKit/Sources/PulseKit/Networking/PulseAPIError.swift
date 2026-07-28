import Foundation

// MARK: - PulseAPIError

/// Error type for all Pulse API operations.
///
/// Maps HTTP status codes and transport failures to specific cases.
/// Conforms to `LocalizedError` for user-friendly error messages.
public enum PulseAPIError: Error, Sendable, Equatable {
    /// The server URL string could not be parsed as a URL.
    case invalidServerURL(String)

    /// The server URL uses HTTP on a non-local host. See ServerURLValidator.
    case insecureTransport(host: String)

    /// A transport-level error (network unreachable, DNS failure, TLS error, timeout).
    case transport(description: String)

    /// HTTP 401: bearer token missing, expired, or invalid.
    case unauthorized

    /// HTTP 403: authenticated but not permitted (tier gate, role check).
    case forbidden

    /// HTTP 404: resource not found.
    case notFound

    /// HTTP 429: rate limit exceeded. `retryAfter` is seconds from the Retry-After
    /// header, or nil if the header is absent.
    case rateLimited(retryAfter: Int?)

    /// HTTP 5xx: server error. `apiError` is the decoded Error body if parseable.
    case server(status: Int, apiError: APIError?)

    /// The response body could not be decoded. `context` is a developer-readable
    /// description of what failed (e.g., "missing required field 'ts'").
    case decoding(context: String)

    /// The request was cancelled (Task cancellation, URLSession invalidation).
    case cancelled
}

// MARK: - LocalizedError

extension PulseAPIError: LocalizedError {
    public var errorDescription: String? {
        switch self {
        case .invalidServerURL(let url):
            return "The server URL '\(url)' is not valid."
        case .insecureTransport(let host):
            return "Cannot connect to '\(host)' over HTTP. Use HTTPS, or connect to a local address."
        case .transport(let description):
            return "Network error: \(description)"
        case .unauthorized:
            return "Your session has expired. Please sign in again."
        case .forbidden:
            return "You do not have permission to access this resource."
        case .notFound:
            return "The requested resource was not found."
        case .rateLimited(let retryAfter):
            if let s = retryAfter {
                return "Too many requests. Please wait \(s) seconds."
            }
            return "Too many requests. Please wait and try again."
        case .server(let status, let apiError):
            if let msg = apiError?.message {
                return "Server error (\(status)): \(msg)"
            }
            return "Server error (\(status))."
        case .decoding(let context):
            return "Could not read the server response: \(context)"
        case .cancelled:
            return "The request was cancelled."
        }
    }
}
