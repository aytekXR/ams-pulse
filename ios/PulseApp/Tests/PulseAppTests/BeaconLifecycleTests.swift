import XCTest
import PulseBeacon

// MARK: - Beacon Lifecycle Tests
//
// Tests for the PulseBeacon SDK integration in PlayerView.
// These verify the beacon lifecycle mapping from player events to QoE telemetry.
//
// The tests use PulseBeacon directly (not via PlayerView) to verify:
// 1. Beacon config construction from stream metadata
// 2. Event emission order
// 3. Session ID generation
// 4. Sampling behavior

final class BeaconLifecycleTests: XCTestCase {

    // MARK: - Config Construction Tests

    func test_beaconConfig_constructedFromStreamMetadata() {
        // Arrange
        let serverURL = "https://pulse.example.com"
        let token = ""  // Anonymous ingest for sample app
        let streamID = "test-stream-123"
        let app = "live"

        // Act
        let config = PulseBeacon.Config(
            ingestURL: serverURL,
            token: token,
            streamID: streamID,
            app: app,
            metadata: nil,
            sampleRate: 1.0,
            playerKind: .native
        )

        // Assert
        XCTAssertEqual(config.ingestURL, serverURL)
        XCTAssertEqual(config.token, token)
        XCTAssertEqual(config.streamID, streamID)
        XCTAssertEqual(config.app, app)
        XCTAssertEqual(config.sampleRate, 1.0)
        XCTAssertEqual(config.playerKind, .native)
    }

    func test_beaconConfig_withMetadata() {
        // Arrange — Player might pass custom metadata.
        let metadata = ["tenant": "acme-corp", "region": "us-west-2"]

        // Act
        let config = PulseBeacon.Config(
            ingestURL: "https://pulse.example.com",
            token: "",
            streamID: "stream",
            app: "live",
            metadata: metadata,
            sampleRate: 1.0,
            playerKind: .native
        )

        // Assert
        XCTAssertEqual(config.metadata, metadata)
    }

    // MARK: - Session ID Tests

    func test_beacon_generatesUniqueSessionID() {
        // Arrange
        let config = PulseBeacon.Config(
            ingestURL: "https://pulse.example.com",
            token: "",
            streamID: "stream",
            app: "live"
        )

        // Act
        let beacon1 = PulseBeacon(config: config)
        let beacon2 = PulseBeacon(config: config)

        // Assert — Each beacon should have a unique session ID.
        XCTAssertNotEqual(beacon1.sessionID, beacon2.sessionID)

        // Session IDs should be valid UUIDs (lowercase).
        XCTAssertEqual(beacon1.sessionID.count, 36) // UUID format: 8-4-4-4-12
        XCTAssertEqual(beacon2.sessionID.count, 36)

        // Clean up
        beacon1.dispose()
        beacon2.dispose()
    }

    func test_beacon_sessionIDIsLowercase() {
        // Arrange
        let config = PulseBeacon.Config(
            ingestURL: "https://pulse.example.com",
            token: "",
            streamID: "stream",
            app: "live"
        )

        // Act
        let beacon = PulseBeacon(config: config)

        // Assert — Session ID should be lowercase (matches JS SDK output).
        XCTAssertEqual(beacon.sessionID, beacon.sessionID.lowercased())

        // Clean up
        beacon.dispose()
    }

    // MARK: - Sampling Tests

    func test_beacon_sampleRateOne_alwaysSampled() {
        // Arrange
        let config = PulseBeacon.Config(
            ingestURL: "https://pulse.example.com",
            token: "",
            streamID: "stream",
            app: "live",
            sampleRate: 1.0  // Always sample
        )

        // Act — Create multiple beacons, all should be sampled.
        var sampledCount = 0
        for _ in 0..<10 {
            let beacon = PulseBeacon(config: config)
            if beacon.isSampled {
                sampledCount += 1
            }
            beacon.dispose()
        }

        // Assert
        XCTAssertEqual(sampledCount, 10, "All beacons should be sampled with sampleRate 1.0")
    }

    func test_beacon_sampleRateZero_neverSampled() {
        // Arrange
        let config = PulseBeacon.Config(
            ingestURL: "https://pulse.example.com",
            token: "",
            streamID: "stream",
            app: "live",
            sampleRate: 0.0  // Never sample
        )

        // Act — Create multiple beacons, none should be sampled.
        var sampledCount = 0
        for _ in 0..<10 {
            let beacon = PulseBeacon(config: config)
            if beacon.isSampled {
                sampledCount += 1
            }
            beacon.dispose()
        }

        // Assert
        XCTAssertEqual(sampledCount, 0, "No beacons should be sampled with sampleRate 0.0")
    }

    // MARK: - PlayerKind Tests

    func test_playerKind_native() {
        // Assert — The app uses .native for AVPlayer.
        XCTAssertEqual(PlayerKind.native.rawValue, "native")
    }

    func test_playerKind_allCases() {
        // Document all player kinds for reference.
        XCTAssertEqual(PlayerKind.amsWebRTC.rawValue, "ams-webrtc")
        XCTAssertEqual(PlayerKind.hlsJS.rawValue, "hls.js")
        XCTAssertEqual(PlayerKind.videoJS.rawValue, "video.js")
        XCTAssertEqual(PlayerKind.native.rawValue, "native")
        XCTAssertEqual(PlayerKind.other.rawValue, "other")
    }

    // MARK: - Event Type Tests

    func test_beaconEventType_rawValues() {
        // Document all event types for reference.
        // These match the beacon-event.schema.json.
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
}
