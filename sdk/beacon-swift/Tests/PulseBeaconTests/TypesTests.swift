import XCTest
@testable import PulseBeacon

/// Pins the encoded wire format against the frozen beacon-event.schema.json:
/// snake_case keys, `version: 1`, nested `player`, and per-type `data` shapes.
final class TypesTests: XCTestCase {

    func testBatchEncodesToSchemaWireKeys() throws {
        let batch = BeaconBatch(
            sessionId: "sess-1",
            streamId: "stream-x",
            app: "live",
            meta: ["tenant": "acme"],
            player: PlayerInfo(kind: PlayerKind.native.rawValue, sdkVersion: pulseBeaconSDKVersion),
            events: [
                BeaconEventItem(type: .startupComplete, ts: 1000, data: ["startup_ms": .int(250)]),
            ]
        )
        let data = try BeaconCoding.encoder.encode(batch)
        let obj = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])

        XCTAssertEqual(obj["version"] as? Int, 1)
        XCTAssertEqual(obj["session_id"] as? String, "sess-1")
        XCTAssertEqual(obj["stream_id"] as? String, "stream-x")
        XCTAssertEqual(obj["app"] as? String, "live")
        XCTAssertEqual((obj["meta"] as? [String: Any])?["tenant"] as? String, "acme")

        let player = try XCTUnwrap(obj["player"] as? [String: Any])
        XCTAssertEqual(player["kind"] as? String, "native")
        XCTAssertEqual(player["sdk_version"] as? String, pulseBeaconSDKVersion)

        let events = try XCTUnwrap(obj["events"] as? [[String: Any]])
        XCTAssertEqual(events.count, 1)
        XCTAssertEqual(events[0]["type"] as? String, "startup_complete")
        XCTAssertEqual(events[0]["ts"] as? Int, 1000)
        XCTAssertEqual((events[0]["data"] as? [String: Any])?["startup_ms"] as? Int, 250)
    }

    func testOptionalTopLevelFieldsOmittedWhenNil() throws {
        let batch = BeaconBatch(
            sessionId: "s",
            streamId: "x",
            events: [BeaconEventItem(type: .heartbeat, ts: 1)]
        )
        let obj = try XCTUnwrap(
            JSONSerialization.jsonObject(with: try BeaconCoding.encoder.encode(batch)) as? [String: Any]
        )
        XCTAssertEqual(obj["version"] as? Int, 1)
        XCTAssertNil(obj["app"])
        XCTAssertNil(obj["meta"])
        XCTAssertNil(obj["player"])
    }

    func testPlayerKindRawValuesMatchSchemaEnum() {
        XCTAssertEqual(PlayerKind.amsWebRTC.rawValue, "ams-webrtc")
        XCTAssertEqual(PlayerKind.hlsJS.rawValue, "hls.js")
        XCTAssertEqual(PlayerKind.videoJS.rawValue, "video.js")
        XCTAssertEqual(PlayerKind.native.rawValue, "native")
        XCTAssertEqual(PlayerKind.other.rawValue, "other")
    }

    func testEventTypeRawValuesMatchSchemaEnum() {
        XCTAssertEqual(BeaconEventType.sessionStart.rawValue, "session_start")
        XCTAssertEqual(BeaconEventType.startupComplete.rawValue, "startup_complete")
        XCTAssertEqual(BeaconEventType.heartbeat.rawValue, "heartbeat")
        XCTAssertEqual(BeaconEventType.rebufferStart.rawValue, "rebuffer_start")
        XCTAssertEqual(BeaconEventType.rebufferEnd.rawValue, "rebuffer_end")
        XCTAssertEqual(BeaconEventType.error.rawValue, "error")
        XCTAssertEqual(BeaconEventType.bitrateChange.rawValue, "bitrate_change")
        XCTAssertEqual(BeaconEventType.resolutionChange.rawValue, "resolution_change")
        XCTAssertEqual(BeaconEventType.sessionEnd.rawValue, "session_end")
    }

    func testJSONValueEncodesScalars() throws {
        // Wrapped in a dictionary (a valid top-level JSON object) to avoid the
        // top-level-fragment restriction some Foundation versions enforce.
        let dict: [String: JSONValue] = ["i": .int(7), "s": .string("hi"), "b": .bool(true), "d": .double(1.5)]
        let json = String(data: try BeaconCoding.encoder.encode(dict), encoding: .utf8) ?? ""
        XCTAssertTrue(json.contains("\"i\":7"), json)
        XCTAssertTrue(json.contains("\"s\":\"hi\""), json)
        XCTAssertTrue(json.contains("\"b\":true"), json)
        XCTAssertTrue(json.contains("\"d\":1.5"), json)
    }
}
