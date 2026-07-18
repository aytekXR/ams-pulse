import XCTest
@testable import PulseBeacon

final class PulseBeaconTests: XCTestCase {

    private func makeConfig(sampleRate: Double = 1.0) -> PulseBeacon.Config {
        PulseBeacon.Config(
            ingestURL: "https://pulse.test",
            token: "tok",
            streamID: "s1",
            app: "live",
            metadata: ["tenant": "acme"],
            sampleRate: sampleRate,
            playerKind: .amsWebRTC
        )
    }

    func testTypedEventsProduceSchemaPayload() throws {
        let sender = MockSender()
        let beacon = PulseBeacon(config: makeConfig(), sender: sender)
        XCTAssertTrue(beacon.isSampled)

        beacon.startupComplete(startupMs: 300, bitrateKbps: 900)
        beacon.error(code: "MEDIA_ERR_NETWORK", message: "boom", fatal: true)
        beacon.dispose()  // flushes synchronously

        XCTAssertEqual(sender.requests.count, 1)
        let obj = try XCTUnwrap(sender.jsonBody(0))
        XCTAssertEqual(obj["session_id"] as? String, beacon.sessionID)
        XCTAssertEqual(obj["stream_id"] as? String, "s1")
        XCTAssertEqual(obj["app"] as? String, "live")

        let player = try XCTUnwrap(obj["player"] as? [String: Any])
        XCTAssertEqual(player["kind"] as? String, "ams-webrtc")
        XCTAssertEqual(player["sdk_version"] as? String, pulseBeaconSDKVersion)

        let events = try XCTUnwrap(obj["events"] as? [[String: Any]])
        XCTAssertEqual(events.count, 2)
        XCTAssertEqual(events[0]["type"] as? String, "startup_complete")
        XCTAssertEqual((events[0]["data"] as? [String: Any])?["startup_ms"] as? Int, 300)
        XCTAssertEqual(events[1]["type"] as? String, "error")
        XCTAssertEqual((events[1]["data"] as? [String: Any])?["code"] as? String, "MEDIA_ERR_NETWORK")

        // Bool/number exactness via the raw JSON avoids NSNumber-bridging ambiguity.
        let body = sender.bodyString(0)
        XCTAssertTrue(body.contains("\"fatal\":true"), body)
        XCTAssertTrue(body.contains("\"version\":1"), body)
    }

    func testUnsampledSessionSendsNothing() {
        let sender = MockSender()
        let beacon = PulseBeacon(config: makeConfig(sampleRate: 0), sender: sender)
        XCTAssertFalse(beacon.isSampled)
        beacon.heartbeat(watchMs: 1000)
        beacon.startupComplete(startupMs: 10)
        beacon.dispose()
        XCTAssertEqual(sender.requests.count, 0)
    }

    func testDisposeWithReasonEmitsSessionEnd() throws {
        let sender = MockSender()
        let beacon = PulseBeacon(config: makeConfig(), sender: sender)
        beacon.dispose(reason: "user_exit")

        XCTAssertEqual(sender.requests.count, 1)
        let events = try XCTUnwrap(sender.jsonBody(0)?["events"] as? [[String: Any]])
        XCTAssertEqual(events.last?["type"] as? String, "session_end")
        XCTAssertEqual((events.last?["data"] as? [String: Any])?["reason"] as? String, "user_exit")
    }

    func testGenericEventPassesThroughCustomData() throws {
        let sender = MockSender()
        let beacon = PulseBeacon(config: makeConfig(), sender: sender)
        beacon.event(.bitrateChange, data: ["from_kbps": .double(500), "to_kbps": .double(1500)])
        beacon.dispose()

        let events = try XCTUnwrap(sender.jsonBody(0)?["events"] as? [[String: Any]])
        XCTAssertEqual(events.count, 1)
        XCTAssertEqual(events[0]["type"] as? String, "bitrate_change")
        let body = sender.bodyString(0)
        XCTAssertTrue(body.contains("\"from_kbps\":500"), body)
        XCTAssertTrue(body.contains("\"to_kbps\":1500"), body)
    }

    func testSessionIDIsStableForTheInstance() {
        let beacon = PulseBeacon(config: makeConfig(), sender: MockSender())
        let first = beacon.sessionID
        XCTAssertEqual(beacon.sessionID, first)
        XCTAssertNotNil(UUID(uuidString: beacon.sessionID))
    }
}
