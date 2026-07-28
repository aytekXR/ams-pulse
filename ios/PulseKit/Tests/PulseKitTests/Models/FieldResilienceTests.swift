import Foundation
import Testing
@testable import PulseKit

// MARK: - Field Resilience Tests

/// Tests for forward/backward compatibility with real Pulse servers.
///
/// Context: PulseKit is a CLIENT for self-hosted Pulse servers. The app ships via
/// the App Store and cannot be updated in lockstep with every server the operator
/// runs. An older server may omit fields or send null where the current contract
/// says array; a newer server may send enum values this build has never seen.
///
/// These tests ensure the client survives both scenarios gracefully.
@Suite("Field Resilience")
struct FieldResilienceTests {

    // MARK: - A. Null/Missing Arrays Decode as Empty

    // Regression fixture: exact wire body captured from a Pulse server build dated
    // 2026-07-13, which returned {"items":null,...} instead of {"items":[],...}.
    // The fix landed 2026-07-18; older deployments still exhibit this behavior.

    @Test("LiveStreamList.items null decodes as empty array")
    func test_LiveStreamList_nullItems_decodesAsEmpty() throws {
        // Exact wire capture from Pulse build 2026-07-13
        let json = """
        {"items":null,"meta":{"next_cursor":null}}
        """

        let data = json.data(using: .utf8)!
        let list = try JSONDecoder().decode(LiveStreamList.self, from: data)

        #expect(list.items.isEmpty)
        #expect(list.meta.nextCursor == nil)
    }

    @Test("AlertHistoryList.items null decodes as empty array")
    func test_AlertHistoryList_nullItems_decodesAsEmpty() throws {
        let json = """
        {"items":null,"meta":{"next_cursor":null}}
        """

        let data = json.data(using: .utf8)!
        let list = try JSONDecoder().decode(AlertHistoryList.self, from: data)

        #expect(list.items.isEmpty)
    }

    @Test("FleetNodeList.items null decodes as empty array")
    func test_FleetNodeList_nullItems_decodesAsEmpty() throws {
        let json = """
        {"items":null,"meta":{"next_cursor":null}}
        """

        let data = json.data(using: .utf8)!
        let list = try JSONDecoder().decode(FleetNodeList.self, from: data)

        #expect(list.items.isEmpty)
    }

    @Test("AnomalyList.items null decodes as empty array")
    func test_AnomalyList_nullItems_decodesAsEmpty() throws {
        let json = """
        {"items":null,"meta":{"next_cursor":null}}
        """

        let data = json.data(using: .utf8)!
        let list = try JSONDecoder().decode(AnomalyList.self, from: data)

        #expect(list.items.isEmpty)
    }

    @Test("LiveOverview.apps null decodes as empty array")
    func test_LiveOverview_nullApps_decodesAsEmpty() throws {
        let json = """
        {
            "ts": 1700000000000,
            "total_viewers": 0,
            "total_publishers": 0,
            "protocol_mix": {},
            "apps": null,
            "nodes": []
        }
        """

        let data = json.data(using: .utf8)!
        let overview = try JSONDecoder().decode(LiveOverview.self, from: data)

        #expect(overview.apps.isEmpty)
    }

    @Test("LiveOverview.nodes null decodes as empty array")
    func test_LiveOverview_nullNodes_decodesAsEmpty() throws {
        let json = """
        {
            "ts": 1700000000000,
            "total_viewers": 0,
            "total_publishers": 0,
            "protocol_mix": {},
            "apps": [],
            "nodes": null
        }
        """

        let data = json.data(using: .utf8)!
        let overview = try JSONDecoder().decode(LiveOverview.self, from: data)

        #expect(overview.nodes.isEmpty)
    }

    @Test("QoeSummaryResponse.bitrateTimeline null decodes as empty array")
    func test_QoeSummaryResponse_nullTimeline_decodesAsEmpty() throws {
        let json = """
        {
            "totals": {
                "startup_p50_ms": 100.0,
                "startup_p95_ms": 200.0,
                "rebuffer_ratio": 0.01,
                "error_rate": 0.001
            },
            "bitrate_timeline": null
        }
        """

        let data = json.data(using: .utf8)!
        let response = try JSONDecoder().decode(QoeSummaryResponse.self, from: data)

        #expect(response.bitrateTimeline.isEmpty)
    }

    @Test("Missing items key decodes as empty array")
    func test_LiveStreamList_missingItems_decodesAsEmpty() throws {
        // An older server might omit the key entirely
        let json = """
        {"meta":{"next_cursor":null}}
        """

        let data = json.data(using: .utf8)!
        let list = try JSONDecoder().decode(LiveStreamList.self, from: data)

        #expect(list.items.isEmpty)
    }

    // MARK: - B. Unknown Enum Values

    @Test("PublisherState unknown value decodes with rawValue preserved")
    func test_PublisherState_unknownValue_preservesRaw() throws {
        let json = """
        {
            "stream_id": "stream-001",
            "app": "live",
            "viewers": 100,
            "publisher_state": "reconnecting",
            "health_score": 90.0
        }
        """

        let data = json.data(using: .utf8)!
        let stream = try JSONDecoder().decode(LiveStream.self, from: data)

        // The enum should decode without throwing
        if case .unknown(let raw) = stream.publisherState {
            #expect(raw == "reconnecting")
        } else {
            Issue.record("Expected .unknown case with preserved rawValue")
        }
    }

    @Test("AlertState unknown value decodes with rawValue preserved")
    func test_AlertState_unknownValue_preservesRaw() throws {
        let json = """
        {
            "id": "alert-1",
            "rule_id": "rule-1",
            "state": "acknowledged",
            "severity": "info",
            "ts": 1700000000000,
            "metric": "test",
            "value": 1.0,
            "threshold": 0.5
        }
        """

        let data = json.data(using: .utf8)!
        let entry = try JSONDecoder().decode(AlertHistoryEntry.self, from: data)

        if case .unknown(let raw) = entry.state {
            #expect(raw == "acknowledged")
        } else {
            Issue.record("Expected .unknown case")
        }
    }

    @Test("AlertSeverity unknown value decodes with rawValue preserved")
    func test_AlertSeverity_unknownValue_preservesRaw() throws {
        let json = """
        {
            "id": "alert-1",
            "rule_id": "rule-1",
            "state": "firing",
            "severity": "emergency",
            "ts": 1700000000000,
            "metric": "test",
            "value": 1.0,
            "threshold": 0.5
        }
        """

        let data = json.data(using: .utf8)!
        let entry = try JSONDecoder().decode(AlertHistoryEntry.self, from: data)

        if case .unknown(let raw) = entry.severity {
            #expect(raw == "emergency")
        } else {
            Issue.record("Expected .unknown case")
        }
    }

    @Test("HealthState unknown value decodes with rawValue preserved")
    func test_HealthState_unknownValue_preservesRaw() throws {
        let json = """
        {
            "status": "maintenance"
        }
        """

        let data = json.data(using: .utf8)!
        let status = try JSONDecoder().decode(ComponentStatus.self, from: data)

        if case .unknown(let raw) = status.status {
            #expect(raw == "maintenance")
        } else {
            Issue.record("Expected .unknown case")
        }
    }

    @Test("NodeRole unknown value decodes with rawValue preserved")
    func test_NodeRole_unknownValue_preservesRaw() throws {
        let json = """
        {
            "node_id": "node-1",
            "role": "relay",
            "status": "up",
            "last_seen": 1700000000000
        }
        """

        let data = json.data(using: .utf8)!
        let node = try JSONDecoder().decode(FleetNode.self, from: data)

        if case .unknown(let raw) = node.role {
            #expect(raw == "relay")
        } else {
            Issue.record("Expected .unknown case")
        }
    }

    @Test("NodeStatus unknown value decodes with rawValue preserved")
    func test_NodeStatus_unknownValue_preservesRaw() throws {
        let json = """
        {
            "node_id": "node-1",
            "role": "origin",
            "status": "draining",
            "last_seen": 1700000000000
        }
        """

        let data = json.data(using: .utf8)!
        let node = try JSONDecoder().decode(FleetNode.self, from: data)

        if case .unknown(let raw) = node.status {
            #expect(raw == "draining")
        } else {
            Issue.record("Expected .unknown case")
        }
    }

    @Test("LicenseTier unknown value decodes with rawValue preserved")
    func test_LicenseTier_unknownValue_preservesRaw() throws {
        let json = """
        {
            "tier": "platinum",
            "valid": true
        }
        """

        let data = json.data(using: .utf8)!
        let license = try JSONDecoder().decode(LicenseInfo.self, from: data)

        if case .unknown(let raw) = license.tier {
            #expect(raw == "platinum")
        } else {
            Issue.record("Expected .unknown case")
        }
    }

    @Test("UserRole unknown value decodes with rawValue preserved")
    func test_UserRole_unknownValue_preservesRaw() throws {
        let json = """
        {
            "id": "user-1",
            "username": "alice",
            "role": "operator",
            "created_at": 1700000000000
        }
        """

        let data = json.data(using: .utf8)!
        let user = try JSONDecoder().decode(User.self, from: data)

        if case .unknown(let raw) = user.role {
            #expect(raw == "operator")
        } else {
            Issue.record("Expected .unknown case")
        }
    }

    @Test("AuthMethod unknown value decodes with rawValue preserved")
    func test_AuthMethod_unknownValue_preservesRaw() throws {
        let json = """
        {
            "name": "admin",
            "role": "admin",
            "auth_method": "oauth2"
        }
        """

        let data = json.data(using: .utf8)!
        let auth = try JSONDecoder().decode(AuthMeResponse.self, from: data)

        if case .unknown(let raw) = auth.authMethod {
            #expect(raw == "oauth2")
        } else {
            Issue.record("Expected .unknown case")
        }
    }

    // MARK: - Combined Resilience

    @Test("Full payload with both null array and unknown enum decodes")
    func test_combinedResilience_nullArrayAndUnknownEnum() throws {
        let json = """
        {
            "ts": 1700000000000,
            "total_viewers": 0,
            "total_publishers": 0,
            "protocol_mix": {},
            "apps": null,
            "nodes": [
                {
                    "node_id": "node-1",
                    "role": "gateway",
                    "status": "initializing"
                }
            ]
        }
        """

        let data = json.data(using: .utf8)!
        let overview = try JSONDecoder().decode(LiveOverview.self, from: data)

        #expect(overview.apps.isEmpty)
        #expect(overview.nodes.count == 1)

        if case .unknown(let role) = overview.nodes[0].role {
            #expect(role == "gateway")
        } else {
            Issue.record("Expected unknown role, got \(String(describing: overview.nodes[0].role))")
        }

        if case .unknown(let status) = overview.nodes[0].status {
            #expect(status == "initializing")
        } else {
            Issue.record("Expected unknown status")
        }
    }

    // MARK: - Regression Fixtures

    @Test("2026-07-13 wire capture: LiveStreamList items null")
    func test_regressionFixture_20260713_liveStreamListNullItems() throws {
        // This is the EXACT body that caused the original failure when
        // PulseKit was first pointed at a real Pulse server running build 2026-07-13.
        // The server code was correct; the fix landed 2026-07-18; the client must
        // tolerate older deployments.
        let json = """
        {"items":null,"meta":{"next_cursor":null}}
        """

        let data = json.data(using: .utf8)!
        let list = try JSONDecoder().decode(LiveStreamList.self, from: data)

        #expect(list.items.isEmpty)
        #expect(list.meta.nextCursor == nil)
    }
}

// MARK: - D. Leniency must not swallow real failures (D-187 gate)
//
// This is the suite that matters most, and it exists because the fix above is
// dangerous in exactly one direction. Making a decoder tolerant of null and
// missing arrays buys resilience against older servers. Taken one step too far it
// buys something much worse: a client that renders "no streams" when the truth is
// "the server returned an HTML error page". The user cannot tell those apart, and
// the app will look calm while the system is broken.
//
// So the boundary is pinned here, deliberately, as executable assertions:
//   TOLERATE  a well-formed response whose list is null or omitted
//   REJECT    a response that is not the shape it claims to be
// If a future change makes any of these decode successfully, that change is wrong
// no matter how convenient it looks at the call site.
@Suite("Field Resilience — leniency has a floor")
struct LeniencyFloorTests {
    private func decode<T: Decodable>(_ type: T.Type, _ s: String) throws -> T {
        try JSONDecoder().decode(type, from: Data(s.utf8))
    }

    @Test("an HTML error page is not an empty list")
    func htmlBodyStillThrows() {
        // A reverse proxy in front of Pulse returning its own 502 page is the
        // realistic source of this, and it is exactly when the user most needs to
        // be told something is wrong.
        let html = "<!doctype html><html><head><title>502 Bad Gateway</title></head><body>nginx</body></html>"
        #expect(throws: (any Error).self) { try decode(LiveStreamList.self, html) }
        #expect(throws: (any Error).self) { try decode(LiveOverview.self, html) }
    }

    @Test("an empty object is not an empty list")
    func emptyObjectStillThrows() {
        // items may be omitted; meta may not. A body carrying neither is not a
        // response, and pretending it is an empty page would hide the failure.
        #expect(throws: (any Error).self) { try decode(LiveStreamList.self, "{}") }
        #expect(throws: (any Error).self) { try decode(AlertHistoryList.self, "{}") }
        #expect(throws: (any Error).self) { try decode(FleetNodeList.self, "{}") }
        #expect(throws: (any Error).self) { try decode(AnomalyList.self, "{}") }
    }

    @Test("a top-level array where an object is expected still throws")
    func topLevelArrayStillThrows() {
        #expect(throws: (any Error).self) { try decode(LiveStreamList.self, "[]") }
        #expect(throws: (any Error).self) { try decode(LiveOverview.self, "[1,2,3]") }
    }

    @Test("truncated JSON still throws")
    func truncatedStillThrows() {
        #expect(throws: (any Error).self) { try decode(LiveStreamList.self, "{\"items\":[{\"stream_id\":") }
    }

    @Test("a structurally required field is still required")
    func missingRequiredFieldStillThrows() {
        // LiveOverview may omit apps/nodes. It may not omit ts or the viewer
        // totals — a response without those is not a snapshot of anything.
        let noTs = """
        {"total_viewers":3,"total_publishers":1,"protocol_mix":{"webrtc":3,"hls":0,"rtmp":0,"dash":0,"other":0}}
        """
        #expect(throws: (any Error).self) { try decode(LiveOverview.self, noTs) }
    }

    @Test("a JSON number where an enum string is expected is an honest error, not an unknown")
    func numberForEnumStillThrows() {
        // The unknown-enum case absorbs unrecognised STRINGS. A number means the
        // wire shape is wrong, which is a different problem and must not be
        // laundered into .unknown.
        let json = """
        {"items":[{"stream_id":"s1","app":"live","node_id":"n1","viewers":1,"publisher_state":42,"health_score":100.0,"protocol_mix":{"webrtc":1,"hls":0,"rtmp":0,"dash":0,"other":0},"bitrate_kbps":1000.0}],"meta":{"next_cursor":null}}
        """
        #expect(throws: (any Error).self) { try decode(LiveStreamList.self, json) }
    }
}

// MARK: - E. Absent keys, for every array property (D-187)
//
// `null` and `absent` reach a decoder through different paths — decodeIfPresent
// returns nil for both only if the key handling is written to cover both, and it
// is entirely possible to fix one and not the other. The null cases are covered
// above; these pin the absent cases for the remaining six properties.
@Suite("Field Resilience — absent array keys")
struct AbsentArrayKeyTests {
    private func decode<T: Decodable>(_ type: T.Type, _ s: String) throws -> T {
        try JSONDecoder().decode(type, from: Data(s.utf8))
    }

    @Test("AlertHistoryList with no items key")
    func alertHistoryAbsent() throws {
        let v = try decode(AlertHistoryList.self, "{\"meta\":{\"next_cursor\":null}}")
        #expect(v.items.isEmpty)
    }

    @Test("FleetNodeList with no items key")
    func fleetAbsent() throws {
        let v = try decode(FleetNodeList.self, "{\"meta\":{\"next_cursor\":null}}")
        #expect(v.items.isEmpty)
    }

    @Test("AnomalyList with no items key")
    func anomaliesAbsent() throws {
        let v = try decode(AnomalyList.self, "{\"meta\":{\"next_cursor\":null}}")
        #expect(v.items.isEmpty)
    }

    @Test("LiveOverview with no apps key")
    func overviewAppsAbsent() throws {
        let json = """
        {"ts":1785275252439,"total_viewers":0,"total_publishers":0,"protocol_mix":{"webrtc":0,"hls":0,"rtmp":0,"dash":0,"other":0},"nodes":[]}
        """
        let v = try decode(LiveOverview.self, json)
        #expect(v.apps.isEmpty)
    }

    @Test("LiveOverview with no nodes key")
    func overviewNodesAbsent() throws {
        let json = """
        {"ts":1785275252439,"total_viewers":0,"total_publishers":0,"protocol_mix":{"webrtc":0,"hls":0,"rtmp":0,"dash":0,"other":0},"apps":[]}
        """
        let v = try decode(LiveOverview.self, json)
        #expect(v.nodes.isEmpty)
    }

    @Test("QoeSummaryResponse with no bitrate_timeline key")
    func qoeTimelineAbsent() throws {
        let json = """
        {"totals":{"startup_p50_ms":0.0,"startup_p95_ms":0.0,"rebuffer_ratio":0.0,"error_rate":0.0}}
        """
        let v = try decode(QoeSummaryResponse.self, json)
        #expect(v.bitrateTimeline.isEmpty)
    }

    @Test("an unknown enum keeps the raw value the server actually sent")
    func unknownEnumPreservesRaw() throws {
        let json = """
        {"items":[{"node_id":"n1","role":"witness","status":"quarantined","last_seen":1785275252439}],"meta":{"next_cursor":null}}
        """
        let v = try decode(FleetNodeList.self, json)
        #expect(v.items.count == 1)
        // Preserving the string is the point: an operator support conversation can
        // be about "witness" instead of about a decode error.
        #expect(String(describing: v.items[0].role).contains("witness"))
        #expect(String(describing: v.items[0].status).contains("quarantined"))
    }
}
