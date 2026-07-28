import Foundation
import Testing
@testable import PulseKit

// MARK: - Pagination Tests

@Suite("Models")
struct PaginationTests {

    @Test("PaginatedMeta decodes next_cursor")
    func test_PaginatedMeta_decodesNextCursor() throws {
        let json = """
        {
            "next_cursor": "eyJsYXN0X2lkIjoiYWJjMTIzIn0=",
            "total": 100
        }
        """

        let data = json.data(using: .utf8)!
        let meta = try JSONDecoder().decode(PaginatedMeta.self, from: data)

        #expect(meta.nextCursor == "eyJsYXN0X2lkIjoiYWJjMTIzIn0=")
        #expect(meta.total == 100)
    }

    @Test("PaginatedMeta decodes total")
    func test_PaginatedMeta_decodesTotal() throws {
        let json = """
        {
            "total": 500
        }
        """

        let data = json.data(using: .utf8)!
        let meta = try JSONDecoder().decode(PaginatedMeta.self, from: data)

        #expect(meta.nextCursor == nil)
        #expect(meta.total == 500)
    }

    @Test("PaginatedMeta both null")
    func test_PaginatedMeta_bothNull() throws {
        let json = """
        {
            "next_cursor": null,
            "total": null
        }
        """

        let data = json.data(using: .utf8)!
        let meta = try JSONDecoder().decode(PaginatedMeta.self, from: data)

        #expect(meta.nextCursor == nil)
        #expect(meta.total == nil)
    }

    @Test("PaginatedMeta empty object")
    func test_PaginatedMeta_emptyObject() throws {
        let json = "{}"

        let data = json.data(using: .utf8)!
        let meta = try JSONDecoder().decode(PaginatedMeta.self, from: data)

        #expect(meta.nextCursor == nil)
        #expect(meta.total == nil)
    }

    @Test("PaginatedMeta null next_cursor")
    func test_PaginatedMeta_nullNextCursor() throws {
        let json = """
        {
            "next_cursor": null
        }
        """

        let data = json.data(using: .utf8)!
        let meta = try JSONDecoder().decode(PaginatedMeta.self, from: data)

        #expect(meta.nextCursor == nil)
    }

    @Test("PaginatedMeta null total")
    func test_PaginatedMeta_nullTotal() throws {
        let json = """
        {
            "total": null
        }
        """

        let data = json.data(using: .utf8)!
        let meta = try JSONDecoder().decode(PaginatedMeta.self, from: data)

        #expect(meta.total == nil)
    }

    @Test("PaginatedMeta Hashable conformance")
    func test_PaginatedMeta_hashable() throws {
        let meta1 = PaginatedMeta(nextCursor: "abc", total: 100)
        let meta2 = PaginatedMeta(nextCursor: "abc", total: 100)
        let meta3 = PaginatedMeta(nextCursor: "def", total: 100)

        #expect(meta1 == meta2)
        #expect(meta1 != meta3)
        #expect(meta1.hashValue == meta2.hashValue)
    }
}
