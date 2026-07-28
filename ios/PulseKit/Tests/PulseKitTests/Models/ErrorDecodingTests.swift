import Foundation
import Testing
@testable import PulseKit

// MARK: - Error Decoding Tests

@Suite("Models")
struct ErrorDecodingTests {

    @Test("APIError decodes code and message")
    func test_APIError_decodesCodeAndMessage() throws {
        let json = """
        {
            "code": "UNAUTHORIZED",
            "message": "Invalid or expired bearer token"
        }
        """

        let data = json.data(using: .utf8)!
        let error = try JSONDecoder().decode(APIError.self, from: data)

        #expect(error.code == "UNAUTHORIZED")
        #expect(error.message == "Invalid or expired bearer token")
        #expect(error.details == nil)
    }

    @Test("APIError optional details")
    func test_APIError_optionalDetails() throws {
        let json = """
        {
            "code": "RATE_LIMITED",
            "message": "Too many requests",
            "details": {
                "retry_after_seconds": 30,
                "limit": 100
            }
        }
        """

        let data = json.data(using: .utf8)!
        let error = try JSONDecoder().decode(APIError.self, from: data)

        #expect(error.code == "RATE_LIMITED")
        #expect(error.details != nil)
        #expect(error.details?["retry_after_seconds"] == .int(30))
        #expect(error.details?["limit"] == .int(100))
    }

    @Test("APIError details with nested object")
    func test_APIError_detailsWithNestedObject() throws {
        let json = """
        {
            "code": "VALIDATION_ERROR",
            "message": "Request validation failed",
            "details": {
                "field_errors": {
                    "name": "required",
                    "threshold": "must be positive"
                },
                "request_id": "req-12345",
                "is_retryable": false
            }
        }
        """

        let data = json.data(using: .utf8)!
        let error = try JSONDecoder().decode(APIError.self, from: data)

        #expect(error.code == "VALIDATION_ERROR")
        #expect(error.details != nil)

        if case .object(let fieldErrors) = error.details?["field_errors"] {
            #expect(fieldErrors["name"] == .string("required"))
            #expect(fieldErrors["threshold"] == .string("must be positive"))
        } else {
            Issue.record("Expected field_errors to be an object")
        }

        #expect(error.details?["request_id"] == .string("req-12345"))
        #expect(error.details?["is_retryable"] == .bool(false))
    }

    @Test("AnyCodableValue decodes string")
    func test_AnyCodableValue_decodesString() throws {
        let json = "\"hello\""
        let data = json.data(using: .utf8)!
        let value = try JSONDecoder().decode(AnyCodableValue.self, from: data)
        #expect(value == .string("hello"))
    }

    @Test("AnyCodableValue decodes int")
    func test_AnyCodableValue_decodesInt() throws {
        let json = "42"
        let data = json.data(using: .utf8)!
        let value = try JSONDecoder().decode(AnyCodableValue.self, from: data)
        #expect(value == .int(42))
    }

    @Test("AnyCodableValue decodes double")
    func test_AnyCodableValue_decodesDouble() throws {
        let json = "3.14"
        let data = json.data(using: .utf8)!
        let value = try JSONDecoder().decode(AnyCodableValue.self, from: data)
        #expect(value == .double(3.14))
    }

    @Test("AnyCodableValue decodes bool")
    func test_AnyCodableValue_decodesBool() throws {
        let json = "true"
        let data = json.data(using: .utf8)!
        let value = try JSONDecoder().decode(AnyCodableValue.self, from: data)
        #expect(value == .bool(true))
    }

    @Test("AnyCodableValue decodes null")
    func test_AnyCodableValue_decodesNull() throws {
        let json = "null"
        let data = json.data(using: .utf8)!
        let value = try JSONDecoder().decode(AnyCodableValue.self, from: data)
        #expect(value == .null)
    }

    @Test("AnyCodableValue decodes array")
    func test_AnyCodableValue_decodesArray() throws {
        let json = "[1, \"two\", true]"
        let data = json.data(using: .utf8)!
        let value = try JSONDecoder().decode(AnyCodableValue.self, from: data)

        if case .array(let items) = value {
            #expect(items.count == 3)
            #expect(items[0] == .int(1))
            #expect(items[1] == .string("two"))
            #expect(items[2] == .bool(true))
        } else {
            Issue.record("Expected array")
        }
    }

    @Test("AnyCodableValue decodes object")
    func test_AnyCodableValue_decodesObject() throws {
        let json = "{\"key\": \"value\", \"count\": 5}"
        let data = json.data(using: .utf8)!
        let value = try JSONDecoder().decode(AnyCodableValue.self, from: data)

        if case .object(let dict) = value {
            #expect(dict["key"] == .string("value"))
            #expect(dict["count"] == .int(5))
        } else {
            Issue.record("Expected object")
        }
    }

    @Test("AnyCodableValue Hashable conformance")
    func test_AnyCodableValue_hashable() throws {
        let v1 = AnyCodableValue.string("test")
        let v2 = AnyCodableValue.string("test")
        let v3 = AnyCodableValue.string("other")

        #expect(v1 == v2)
        #expect(v1 != v3)
        #expect(v1.hashValue == v2.hashValue)
    }

    @Test("APIError Equatable conformance")
    func test_APIError_equatable() throws {
        let error1 = try JSONDecoder().decode(APIError.self, from: """
        {"code": "A", "message": "B"}
        """.data(using: .utf8)!)

        let error2 = try JSONDecoder().decode(APIError.self, from: """
        {"code": "A", "message": "B"}
        """.data(using: .utf8)!)

        let error3 = try JSONDecoder().decode(APIError.self, from: """
        {"code": "C", "message": "D"}
        """.data(using: .utf8)!)

        #expect(error1 == error2)
        #expect(error1 != error3)
    }
}
