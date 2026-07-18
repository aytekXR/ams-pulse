import XCTest
@testable import PulseBeacon

final class TransportTests: XCTestCase {

    private func makeConfig() -> Transport.Config {
        Transport.Config(
            ingestURL: "https://pulse.test",
            token: "tok-123",
            sessionID: "sess",
            streamID: "stream",
            app: "app1",
            meta: nil,
            playerKind: .native,
            sdkVersion: pulseBeaconSDKVersion
        )
    }

    func testFlushPostsToBeaconEndpointWithToken() {
        let sender = MockSender()
        let transport = Transport(cfg: makeConfig(), sender: sender, startTimer: false)
        transport.push(BeaconEventItem(type: .heartbeat, ts: 1, data: ["watch_ms": .int(100)]))
        transport.flush()
        transport.drain()

        XCTAssertEqual(sender.requests.count, 1)
        XCTAssertEqual(sender.requests[0].url.absoluteString, "https://pulse.test/ingest/beacon")
        XCTAssertEqual(sender.requests[0].headers["X-Pulse-Ingest-Token"], "tok-123")
        XCTAssertEqual(sender.requests[0].headers["Content-Type"], "application/json")
    }

    func testAutoFlushAtBatchSize() {
        let sender = MockSender()
        let transport = Transport(cfg: makeConfig(), sender: sender, startTimer: false)
        for i in 0..<Transport.maxBatchSize {
            transport.push(BeaconEventItem(type: .heartbeat, ts: i))
        }
        transport.drain()

        XCTAssertEqual(sender.requests.count, 1, "a full batch flushes on its own")
        XCTAssertEqual((sender.jsonBody(0)?["events"] as? [[String: Any]])?.count, Transport.maxBatchSize)
    }

    func testBelowBatchSizeDoesNotAutoFlush() {
        let sender = MockSender()
        let transport = Transport(cfg: makeConfig(), sender: sender, startTimer: false)
        for i in 0..<(Transport.maxBatchSize - 1) {
            transport.push(BeaconEventItem(type: .heartbeat, ts: i))
        }
        transport.drain()
        XCTAssertEqual(sender.requests.count, 0, "no flush until the timer, a full batch, or dispose")
    }

    func testEmptyFlushIsNoop() {
        let sender = MockSender()
        let transport = Transport(cfg: makeConfig(), sender: sender, startTimer: false)
        transport.flush()
        transport.drain()
        XCTAssertEqual(sender.requests.count, 0)
    }

    func testFailedSendIsEnqueuedForRetryNotDropped() {
        let sender = MockSender()
        sender.nextResult = false
        let transport = Transport(cfg: makeConfig(), sender: sender, startTimer: false)
        transport.push(BeaconEventItem(type: .error, ts: 1, data: ["code": .string("X")]))
        transport.flush()
        transport.drain()  // flush → send → completion(false)
        transport.drain()  // completion's async enqueue runs

        XCTAssertEqual(sender.requests.count, 1)
        XCTAssertEqual(transport.retryQueueDepthForTesting, 1, "a failed batch is retained for retry")
    }

    func testFailedSendIsEventuallyRetriedAndSucceeds() {
        let sender = MockSender()
        sender.nextResult = false
        let sentTwice = expectation(description: "batch is re-sent after the first failure")
        sender.onSend = { count in
            if count >= 2 { sentTwice.fulfill() }
        }
        let transport = Transport(cfg: makeConfig(), sender: sender, startTimer: false)
        transport.push(BeaconEventItem(type: .error, ts: 1, data: ["code": .string("X")]))
        transport.flush()
        // Let the first send fail, then allow the backoff retry (~1 s) to succeed.
        DispatchQueue.global().asyncAfter(deadline: .now() + 0.2) { sender.nextResult = true }

        wait(for: [sentTwice], timeout: 5.0)
        XCTAssertGreaterThanOrEqual(sender.requests.count, 2)
    }

    func testDisposeFlushesRemainingBuffer() {
        let sender = MockSender()
        let transport = Transport(cfg: makeConfig(), sender: sender, startTimer: false)
        transport.push(BeaconEventItem(type: .heartbeat, ts: 1))
        transport.dispose()  // dispose flushes synchronously
        XCTAssertEqual(sender.requests.count, 1)
    }

    func testPushAfterDisposeIsIgnored() {
        let sender = MockSender()
        let transport = Transport(cfg: makeConfig(), sender: sender, startTimer: false)
        transport.dispose()
        transport.push(BeaconEventItem(type: .heartbeat, ts: 1))
        transport.flush()
        transport.drain()
        XCTAssertEqual(sender.requests.count, 0)
    }
}
