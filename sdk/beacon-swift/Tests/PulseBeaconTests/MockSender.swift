import Foundation
@testable import PulseBeacon

/// A `BeaconSender` that captures requests instead of hitting the network, so tests
/// can assert the exact URL, headers, and JSON body. Thread-safe (the transport
/// calls it from its serial queue).
final class MockSender: BeaconSender {
    struct Request {
        let url: URL
        let headers: [String: String]
        let body: Data
    }

    private let lock = NSLock()
    private var _requests: [Request] = []
    private var _nextResult = true

    /// Called (outside the lock) after each captured send, with the new total count.
    var onSend: ((Int) -> Void)?

    /// Result the next `send` reports to its completion (true = 2xx, false = failure).
    var nextResult: Bool {
        get { lock.lock(); defer { lock.unlock() }; return _nextResult }
        set { lock.lock(); _nextResult = newValue; lock.unlock() }
    }

    var requests: [Request] {
        lock.lock(); defer { lock.unlock() }; return _requests
    }

    func send(url: URL, headers: [String: String], body: Data, completion: @escaping (Bool) -> Void) {
        lock.lock()
        _requests.append(Request(url: url, headers: headers, body: body))
        let count = _requests.count
        let ok = _nextResult
        lock.unlock()
        onSend?(count)
        completion(ok)
    }

    /// Parse a captured request body as a JSON object.
    func jsonBody(_ index: Int) -> [String: Any]? {
        let reqs = requests
        guard index < reqs.count else { return nil }
        return (try? JSONSerialization.jsonObject(with: reqs[index].body)) as? [String: Any]
    }

    /// The captured request body as a UTF-8 string (for exact `"key":value` checks).
    func bodyString(_ index: Int) -> String {
        let reqs = requests
        guard index < reqs.count else { return "" }
        return String(data: reqs[index].body, encoding: .utf8) ?? ""
    }
}
