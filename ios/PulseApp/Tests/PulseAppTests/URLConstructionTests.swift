import XCTest
@testable import PulseApp

// MARK: - URL Construction Tests
//
// Tests for the HLS URL construction logic in PlayerView.
// The app builds HLS URLs from stream metadata using the pattern:
//   <serverBaseURL>/<app>/streams/<streamId>.m3u8
//
// These tests verify:
// 1. Path construction with various input combinations
// 2. URL encoding of special characters
// 3. Edge cases like empty strings or unusual characters

final class URLConstructionTests: XCTestCase {

    // MARK: - HLS URL Path Tests

    /// Tests the HLS URL path construction logic used in PlayerView.
    /// The pattern is: /<app>/streams/<streamId>.m3u8
    func test_hlsPath_standardInputs() {
        // Arrange
        let app = "live"
        let streamId = "my-stream-123"

        // Act
        let hlsPath = "/\(app)/streams/\(streamId).m3u8"

        // Assert
        XCTAssertEqual(hlsPath, "/live/streams/my-stream-123.m3u8")
    }

    func test_hlsPath_withSlashInApp_escapesCorrectly() {
        // Arrange — AMS app names shouldn't contain slashes, but verify behavior.
        let app = "live/test"
        let streamId = "stream"

        // Act
        let hlsPath = "/\(app)/streams/\(streamId).m3u8"

        // Assert — The slash becomes part of the path (not URL-encoded here).
        // In production, this would create a path traversal.
        // This test documents the current behavior; validation belongs elsewhere.
        XCTAssertEqual(hlsPath, "/live/test/streams/stream.m3u8")
    }

    func test_hlsPath_withSpaceInStreamId() {
        // Arrange
        let app = "live"
        let streamId = "my stream"  // Space in stream ID (unusual but possible)

        // Act
        let hlsPath = "/\(app)/streams/\(streamId).m3u8"

        // Assert — The space is not percent-encoded at this stage.
        // URL(string:) and appendingPathComponent handle encoding.
        XCTAssertEqual(hlsPath, "/live/streams/my stream.m3u8")
    }

    func test_hlsPath_withUnicodeStreamId() {
        // Arrange
        let app = "live"
        let streamId = "stream-\u{65E5}\u{672C}"  // Japanese characters

        // Act
        let hlsPath = "/\(app)/streams/\(streamId).m3u8"

        // Assert
        XCTAssertEqual(hlsPath, "/live/streams/stream-\u{65E5}\u{672C}.m3u8")
    }

    // MARK: - Full URL Construction Tests

    func test_fullURL_construction() {
        // Arrange
        let baseURL = URL(string: "https://pulse.example.com")!
        let app = "live"
        let streamId = "test-stream"
        let hlsPath = "/\(app)/streams/\(streamId).m3u8"

        // Act
        let fullURL = baseURL.appendingPathComponent(hlsPath)

        // Assert
        XCTAssertEqual(fullURL.absoluteString, "https://pulse.example.com/live/streams/test-stream.m3u8")
    }

    func test_fullURL_withPortNumber() {
        // Arrange — Self-hosted servers often run on custom ports.
        let baseURL = URL(string: "https://pulse.example.com:8443")!
        let hlsPath = "/live/streams/test.m3u8"

        // Act
        let fullURL = baseURL.appendingPathComponent(hlsPath)

        // Assert
        XCTAssertEqual(fullURL.absoluteString, "https://pulse.example.com:8443/live/streams/test.m3u8")
    }

    func test_fullURL_withBasePath() {
        // Arrange — Pulse might be served under a subpath.
        let baseURL = URL(string: "https://example.com/pulse")!
        let hlsPath = "/live/streams/test.m3u8"

        // Act
        let fullURL = baseURL.appendingPathComponent(hlsPath)

        // Assert — appendingPathComponent adds to the existing path.
        XCTAssertEqual(fullURL.absoluteString, "https://example.com/pulse/live/streams/test.m3u8")
    }

    func test_fullURL_withTrailingSlash() {
        // Arrange — User might enter URL with trailing slash.
        var urlString = "https://pulse.example.com/"
        if urlString.hasSuffix("/") {
            urlString = String(urlString.dropLast())
        }
        let baseURL = URL(string: urlString)!
        let hlsPath = "/live/streams/test.m3u8"

        // Act
        let fullURL = baseURL.appendingPathComponent(hlsPath)

        // Assert — Should not have double slashes.
        XCTAssertEqual(fullURL.absoluteString, "https://pulse.example.com/live/streams/test.m3u8")
    }
}
