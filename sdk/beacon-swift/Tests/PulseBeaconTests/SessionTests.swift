import XCTest
@testable import PulseBeacon

final class SessionTests: XCTestCase {

    func testGenerateSessionIDIsLowercaseV4UUID() {
        let id = Session.generateSessionID()
        XCTAssertEqual(id.count, 36)
        XCTAssertEqual(id, id.lowercased())
        XCTAssertNotNil(UUID(uuidString: id))
        // RFC 4122 v4: the version nibble (index 14) is '4'.
        XCTAssertEqual(Array(id)[14], "4")
    }

    func testSessionIDsAreUnique() {
        let ids = Set((0..<200).map { _ in Session.generateSessionID() })
        XCTAssertEqual(ids.count, 200)
    }

    func testSamplingEdges() {
        XCTAssertTrue(Session.isSampled(1))
        XCTAssertTrue(Session.isSampled(2))
        XCTAssertFalse(Session.isSampled(0))
        XCTAssertFalse(Session.isSampled(-1))
    }

    func testSamplingIsProbabilistic() {
        var trues = 0
        for _ in 0..<2000 where Session.isSampled(0.5) { trues += 1 }
        // ~1000 expected; a wide band keeps this non-flaky.
        XCTAssertGreaterThan(trues, 700)
        XCTAssertLessThan(trues, 1300)
    }
}
