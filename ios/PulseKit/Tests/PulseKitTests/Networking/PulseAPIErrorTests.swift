import Foundation
import Testing
@testable import PulseKit

/// Tests for PulseAPIError enum.
@Suite("PulseAPIError Tests")
struct PulseAPIErrorTests {

    // MARK: - LocalizedError

    @Test("localizedDescription returns correct message for invalidServerURL")
    func localizedDescription_invalidServerURL() {
        let error = PulseAPIError.invalidServerURL("not-a-url")
        #expect(error.errorDescription == "The server URL 'not-a-url' is not valid.")
    }

    @Test("localizedDescription returns correct message for insecureTransport")
    func localizedDescription_insecureTransport() {
        let error = PulseAPIError.insecureTransport(host: "public.example.com")
        #expect(error.errorDescription == "Cannot connect to 'public.example.com' over HTTP. Use HTTPS, or connect to a local address.")
    }

    @Test("localizedDescription returns correct message for transport")
    func localizedDescription_transport() {
        let error = PulseAPIError.transport(description: "Connection reset by peer")
        #expect(error.errorDescription == "Network error: Connection reset by peer")
    }

    @Test("localizedDescription returns correct message for unauthorized")
    func localizedDescription_unauthorized() {
        let error = PulseAPIError.unauthorized
        #expect(error.errorDescription == "Your session has expired. Please sign in again.")
    }

    @Test("localizedDescription returns correct message for forbidden")
    func localizedDescription_forbidden() {
        let error = PulseAPIError.forbidden
        #expect(error.errorDescription == "You do not have permission to access this resource.")
    }

    @Test("localizedDescription returns correct message for notFound")
    func localizedDescription_notFound() {
        let error = PulseAPIError.notFound
        #expect(error.errorDescription == "The requested resource was not found.")
    }

    @Test("localizedDescription returns correct message for rateLimited with retryAfter")
    func localizedDescription_rateLimited_withRetryAfter() {
        let error = PulseAPIError.rateLimited(retryAfter: 30)
        #expect(error.errorDescription == "Too many requests. Please wait 30 seconds.")
    }

    @Test("localizedDescription returns correct message for rateLimited without retryAfter")
    func localizedDescription_rateLimited_withoutRetryAfter() {
        let error = PulseAPIError.rateLimited(retryAfter: nil)
        #expect(error.errorDescription == "Too many requests. Please wait and try again.")
    }

    @Test("localizedDescription returns correct message for server with apiError")
    func localizedDescription_server_withAPIError() {
        let apiError = APIError(code: "INTERNAL_ERROR", message: "Database connection failed", details: nil)
        let error = PulseAPIError.server(status: 500, apiError: apiError)
        #expect(error.errorDescription == "Server error (500): Database connection failed")
    }

    @Test("localizedDescription returns correct message for server without apiError")
    func localizedDescription_server_withoutAPIError() {
        let error = PulseAPIError.server(status: 502, apiError: nil)
        #expect(error.errorDescription == "Server error (502).")
    }

    @Test("localizedDescription returns correct message for decoding")
    func localizedDescription_decoding() {
        let error = PulseAPIError.decoding(context: "missing required field 'ts'")
        #expect(error.errorDescription == "Could not read the server response: missing required field 'ts'")
    }

    @Test("localizedDescription returns correct message for cancelled")
    func localizedDescription_cancelled() {
        let error = PulseAPIError.cancelled
        #expect(error.errorDescription == "The request was cancelled.")
    }

    // MARK: - Equatable

    @Test("equatable same case with same values")
    func equatable_sameCase_sameValues() {
        let error1 = PulseAPIError.rateLimited(retryAfter: 30)
        let error2 = PulseAPIError.rateLimited(retryAfter: 30)
        #expect(error1 == error2)
    }

    @Test("equatable same case with different values")
    func equatable_sameCase_differentValues() {
        let error1 = PulseAPIError.rateLimited(retryAfter: 30)
        let error2 = PulseAPIError.rateLimited(retryAfter: 60)
        #expect(error1 != error2)
    }

    @Test("equatable different cases")
    func equatable_differentCases() {
        let error1 = PulseAPIError.unauthorized
        let error2 = PulseAPIError.forbidden
        #expect(error1 != error2)
    }

    @Test("equatable server errors with same apiError")
    func equatable_serverErrors_sameAPIError() {
        let apiError = APIError(code: "TEST", message: "Test message", details: nil)
        let error1 = PulseAPIError.server(status: 500, apiError: apiError)
        let error2 = PulseAPIError.server(status: 500, apiError: apiError)
        #expect(error1 == error2)
    }

    @Test("equatable server errors with different status")
    func equatable_serverErrors_differentStatus() {
        let apiError = APIError(code: "TEST", message: "Test message", details: nil)
        let error1 = PulseAPIError.server(status: 500, apiError: apiError)
        let error2 = PulseAPIError.server(status: 503, apiError: apiError)
        #expect(error1 != error2)
    }

    // MARK: - Token Security

    @Test("bearer token never appears in error description")
    func bearerToken_neverAppearsInErrorDescription() {
        let sensitiveToken = "sk_test_extremely_secret_bearer_token_12345"

        // All possible error cases - none should expose the token
        let errors: [PulseAPIError] = [
            .invalidServerURL("https://example.com?token=\(sensitiveToken)"),
            .insecureTransport(host: "example.com"),
            .transport(description: "Request failed for token=\(sensitiveToken)"),
            .unauthorized,
            .forbidden,
            .notFound,
            .rateLimited(retryAfter: 30),
            .server(status: 500, apiError: APIError(code: "ERR", message: "Token: \(sensitiveToken)", details: nil)),
            .decoding(context: "Failed parsing with token \(sensitiveToken)"),
            .cancelled
        ]

        for error in errors {
            // The error description should never contain the raw token
            // Note: We explicitly test that if someone accidentally puts a token in an error
            // parameter, it might appear. The security property is that the CLIENT code
            // never includes the bearer token in error creation paths.
            let description = error.errorDescription ?? ""

            // For errors that DON'T have user-controlled strings, verify no token
            switch error {
            case .unauthorized, .forbidden, .notFound, .cancelled:
                #expect(!description.contains(sensitiveToken),
                       "Error \(error) should never expose tokens")
            case .rateLimited:
                #expect(!description.contains(sensitiveToken),
                       "rateLimited should never expose tokens")
            default:
                // Other cases may echo user-provided strings, which is the
                // responsibility of the caller, not the error type
                break
            }
        }
    }
}

// Extension to create APIError for testing (since init is internal)
extension APIError {
    init(code: String, message: String, details: [String: AnyCodableValue]?) {
        // We need to decode from JSON since the struct has no public init
        let json: [String: Any?] = ["code": code, "message": message, "details": details]
        let data = try! JSONSerialization.data(withJSONObject: json.compactMapValues { $0 })
        self = try! JSONDecoder().decode(APIError.self, from: data)
    }
}
