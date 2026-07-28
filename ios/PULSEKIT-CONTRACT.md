# PulseKit Interface Contract

This document specifies the interface contract for PulseKit, the Swift SDK for the Pulse API.
Four lanes implement against this contract: LANE-MODELS, LANE-NET, LANE-VM. A fourth lane
(LANE-APP) consumes the API but writes no files inside PulseKit.

All types must build and test on Linux (swift 6.1.2 is on PATH). No UIKit, SwiftUI,
AVFoundation, or Security import outside a `canImport` guard.

---

## A. Module Layout

### Sources: `ios/PulseKit/Sources/PulseKit/`

| File | Lane | Purpose |
|------|------|---------|
| `PulseKit.swift` | LANE-NET | Re-export of public surface |
| **Models/** | | |
| `Models/Error.swift` | LANE-MODELS | `Error` wire schema |
| `Models/Pagination.swift` | LANE-MODELS | `PaginatedMeta` |
| `Models/Live.swift` | LANE-MODELS | `LiveOverview`, `ProtocolMix`, `AppOverview`, `NodeHealth`, `LiveStreamList`, `LiveStream` |
| `Models/QoE.swift` | LANE-MODELS | `QoeSummaryResponse`, `QoeTotals`, `BitrateBucket` |
| `Models/Alerts.swift` | LANE-MODELS | `AlertHistoryList`, `AlertHistoryEntry`, `AlertScope` |
| `Models/Fleet.swift` | LANE-MODELS | `FleetNodeList`, `FleetNode` |
| `Models/Anomalies.swift` | LANE-MODELS | `AnomalyList`, `AnomalyFlag` |
| `Models/Health.swift` | LANE-MODELS | `HealthStatus`, `ComponentStatus` |
| `Models/Auth.swift` | LANE-MODELS | `User`, `AuthMeResponse` |
| `Models/License.swift` | LANE-MODELS | `LicenseInfo`, `TierLimits` |
| **Networking/** | | |
| `Networking/PulseAPIError.swift` | LANE-NET | Error enum |
| `Networking/PulseTransport.swift` | LANE-NET | Transport protocol and URLSession conformance |
| `Networking/PulseAPIClient.swift` | LANE-NET | API client actor |
| `Networking/ServerURLValidator.swift` | LANE-NET | Security validation for user-entered URLs |
| **Credentials/** | | |
| `Credentials/PulseServer.swift` | LANE-NET | Server profile value type |
| `Credentials/TokenStore.swift` | LANE-NET | Token storage protocol and conformances |
| **Formatting/** | | |
| `Formatting/Formatters.swift` | LANE-VM | Pure formatting functions |
| **ViewModels/** | | |
| `ViewModels/LoadingState.swift` | LANE-VM | Loading state enum |
| `ViewModels/ServerStore.swift` | LANE-VM | Server CRUD + persistence |
| `ViewModels/OverviewModel.swift` | LANE-VM | Live overview view model |
| `ViewModels/StreamsModel.swift` | LANE-VM | Streams with pagination |
| `ViewModels/AlertsModel.swift` | LANE-VM | Alert history view model |

### Tests: `ios/PulseKit/Tests/PulseKitTests/`

| File | Lane |
|------|------|
| `ModelDecodingTests.swift` | LANE-MODELS |
| `PaginationTests.swift` | LANE-MODELS |
| `ErrorDecodingTests.swift` | LANE-MODELS |
| `PulseAPIErrorTests.swift` | LANE-NET |
| `ServerURLValidatorTests.swift` | LANE-NET |
| `MockTransport.swift` | LANE-NET |
| `PulseAPIClientTests.swift` | LANE-NET |
| `TokenStoreTests.swift` | LANE-NET |
| `FormatterTests.swift` | LANE-VM |
| `ViewModelTests.swift` | LANE-VM |
| **Fixtures/** | |
| `Fixtures/live_overview.json` | LANE-MODELS |
| `Fixtures/live_streams.json` | LANE-MODELS |
| `Fixtures/qoe_summary.json` | LANE-MODELS |
| `Fixtures/alerts_history.json` | LANE-MODELS |
| `Fixtures/fleet_nodes.json` | LANE-MODELS |
| `Fixtures/anomalies.json` | LANE-MODELS |
| `Fixtures/healthz.json` | LANE-MODELS |
| `Fixtures/auth_me.json` | LANE-MODELS |
| `Fixtures/license.json` | LANE-MODELS |
| `Fixtures/error_401.json` | LANE-MODELS |
| `Fixtures/error_429.json` | LANE-MODELS |

Fixtures must be derived from the OpenAPI examples in `contracts/openapi/pulse-api.yaml`,
not invented.

---

## B. Model Types

All models: `public`, `Sendable`, `Decodable`, `Equatable`, `Hashable` (where cost-free).

### Timestamp wire format

The spec description at line 27 states:
> **Timestamps:** All time parameters accept either Unix epoch milliseconds (integer) or
> RFC 3339 strings (e.g. `2026-01-01T00:00:00Z`). Responses always return Unix epoch
> milliseconds integers.

Therefore all `ts`, `last_seen`, `created_at`, `updated_at`, `expires_at`, `started_at`,
`cooldown_until` fields decode as `Int64` (epoch milliseconds). The decoder does not need
date decoding strategy; it reads the integer directly.

### Error

Spec lines 1856-1869 (required: `code`, `message`; optional: `details`):

```swift
public struct APIError: Decodable, Sendable, Equatable {
    public let code: String
    public let message: String
    public let details: [String: AnyCodableValue]?
    
    enum CodingKeys: String, CodingKey {
        case code
        case message
        case details
    }
}
```

`AnyCodableValue` is a type-erased JSON value (string, int, double, bool, array, object)
needed because `details` is `additionalProperties: true`.

### PaginatedMeta

Spec lines 1872-1880 (both optional):

```swift
public struct PaginatedMeta: Decodable, Sendable, Equatable, Hashable {
    public let nextCursor: String?
    public let total: Int?
    
    enum CodingKeys: String, CodingKey {
        case nextCursor = "next_cursor"
        case total
    }
}
```

### LiveOverview

Spec lines 1886-1906 (required: `ts`, `total_viewers`, `total_publishers`, `protocol_mix`,
`apps`, `nodes`):

```swift
public struct LiveOverview: Decodable, Sendable, Equatable {
    public let ts: Int64
    public let totalViewers: Int
    public let totalPublishers: Int
    public let protocolMix: ProtocolMix
    public let apps: [AppOverview]
    public let nodes: [NodeHealth]
    
    enum CodingKeys: String, CodingKey {
        case ts
        case totalViewers = "total_viewers"
        case totalPublishers = "total_publishers"
        case protocolMix = "protocol_mix"
        case apps
        case nodes
    }
}
```

### ProtocolMix

Spec lines 1908-1926. All properties optional with default 0:

```swift
public struct ProtocolMix: Decodable, Sendable, Equatable, Hashable {
    public let webrtc: Int
    public let hls: Int
    public let rtmp: Int
    public let dash: Int
    public let other: Int
    
    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        webrtc = try container.decodeIfPresent(Int.self, forKey: .webrtc) ?? 0
        hls = try container.decodeIfPresent(Int.self, forKey: .hls) ?? 0
        rtmp = try container.decodeIfPresent(Int.self, forKey: .rtmp) ?? 0
        dash = try container.decodeIfPresent(Int.self, forKey: .dash) ?? 0
        other = try container.decodeIfPresent(Int.self, forKey: .other) ?? 0
    }
    
    enum CodingKeys: String, CodingKey {
        case webrtc, hls, rtmp, dash, other
    }
}
```

### AppOverview

Spec lines 1928-1939 (required: `app`, `viewers`, `publishers`, `streams`):

```swift
public struct AppOverview: Decodable, Sendable, Equatable, Hashable {
    public let app: String
    public let viewers: Int
    public let publishers: Int
    public let streams: Int
}
```

Wire keys match Swift property names (camelCase in spec).

### NodeHealth

Spec lines 1941-1965 (required: `node_id`, `status`; optional: `role`, `last_seen`,
`cpu_pct`, `mem_pct`, `version`):

```swift
public struct NodeHealth: Decodable, Sendable, Equatable, Hashable {
    public let nodeId: String
    public let role: NodeRole?
    public let status: NodeStatus
    public let lastSeen: Int64?
    public let cpuPct: Double?
    public let memPct: Double?
    public let version: String?
    
    enum CodingKeys: String, CodingKey {
        case nodeId = "node_id"
        case role
        case status
        case lastSeen = "last_seen"
        case cpuPct = "cpu_pct"
        case memPct = "mem_pct"
        case version
    }
}

public enum NodeRole: String, Decodable, Sendable, Equatable, Hashable {
    case origin
    case edge
    case standalone
}

public enum NodeStatus: String, Decodable, Sendable, Equatable, Hashable {
    case up
    case degraded
}
```

### LiveStreamList

Spec lines 1967-1976:

```swift
public struct LiveStreamList: Decodable, Sendable, Equatable {
    public let items: [LiveStream]
    public let meta: PaginatedMeta
}
```

### LiveStream

Spec lines 1978-2024 (required: `stream_id`, `app`, `viewers`, `publisher_state`,
`health_score`; optional: `node_id`, `tenant`, `protocol_mix`, `bitrate_kbps`,
`started_at`, `viewer_rtt_ms`, `viewer_jitter_ms`, `viewer_loss_pct`):

```swift
public struct LiveStream: Decodable, Sendable, Equatable, Hashable, Identifiable {
    public var id: String { streamId }
    
    public let streamId: String
    public let app: String
    public let nodeId: String?
    public let tenant: String?
    public let viewers: Int
    public let publisherState: PublisherState
    public let healthScore: Double
    public let protocolMix: ProtocolMix?
    public let bitrateKbps: Double?
    public let startedAt: Int64?
    public let viewerRttMs: Double?
    public let viewerJitterMs: Double?
    public let viewerLossPct: Double?
    
    enum CodingKeys: String, CodingKey {
        case streamId = "stream_id"
        case app
        case nodeId = "node_id"
        case tenant
        case viewers
        case publisherState = "publisher_state"
        case healthScore = "health_score"
        case protocolMix = "protocol_mix"
        case bitrateKbps = "bitrate_kbps"
        case startedAt = "started_at"
        case viewerRttMs = "viewer_rtt_ms"
        case viewerJitterMs = "viewer_jitter_ms"
        case viewerLossPct = "viewer_loss_pct"
    }
}

public enum PublisherState: String, Decodable, Sendable, Equatable, Hashable {
    case publishing
    case idle
    case offline
}
```

### QoeSummaryResponse

Spec lines 2147-2156 (required: `totals`, `bitrate_timeline`):

```swift
public struct QoeSummaryResponse: Decodable, Sendable, Equatable {
    public let totals: QoeTotals
    public let bitrateTimeline: [BitrateBucket]
    
    enum CodingKeys: String, CodingKey {
        case totals
        case bitrateTimeline = "bitrate_timeline"
    }
}
```

### QoeTotals

Spec lines 2158-2176 (required: `startup_p50_ms`, `startup_p95_ms`, `rebuffer_ratio`,
`error_rate`):

```swift
public struct QoeTotals: Decodable, Sendable, Equatable, Hashable {
    public let startupP50Ms: Double
    public let startupP95Ms: Double
    public let rebufferRatio: Double
    public let errorRate: Double
    
    enum CodingKeys: String, CodingKey {
        case startupP50Ms = "startup_p50_ms"
        case startupP95Ms = "startup_p95_ms"
        case rebufferRatio = "rebuffer_ratio"
        case errorRate = "error_rate"
    }
}
```

### BitrateBucket

Spec lines 2178-2190 (required: `ts`, `bitrate_kbps_p50`; optional: `bitrate_kbps_p95`):

```swift
public struct BitrateBucket: Decodable, Sendable, Equatable, Hashable {
    public let ts: Int64
    public let bitrateKbpsP50: Double
    public let bitrateKbpsP95: Double?
    
    enum CodingKeys: String, CodingKey {
        case ts
        case bitrateKbpsP50 = "bitrate_kbps_p50"
        case bitrateKbpsP95 = "bitrate_kbps_p95"
    }
}
```

### AlertHistoryList

Spec lines 2542-2551:

```swift
public struct AlertHistoryList: Decodable, Sendable, Equatable {
    public let items: [AlertHistoryEntry]
    public let meta: PaginatedMeta
}
```

### AlertHistoryEntry

Spec lines 2553-2585 (required: `id`, `rule_id`, `state`, `severity`, `ts`, `metric`,
`value`, `threshold`; optional: `scope`, `cooldown_until`, `group_key`):

```swift
public struct AlertHistoryEntry: Decodable, Sendable, Equatable, Hashable, Identifiable {
    public var id: String { alertId }
    
    public let alertId: String
    public let ruleId: String
    public let state: AlertState
    public let severity: AlertSeverity
    public let ts: Int64
    public let metric: String
    public let value: Double
    public let threshold: Double
    public let scope: AlertScope?
    public let cooldownUntil: Int64?
    public let groupKey: String?
    
    enum CodingKeys: String, CodingKey {
        case alertId = "id"
        case ruleId = "rule_id"
        case state
        case severity
        case ts
        case metric
        case value
        case threshold
        case scope
        case cooldownUntil = "cooldown_until"
        case groupKey = "group_key"
    }
}

public enum AlertState: String, Decodable, Sendable, Equatable, Hashable {
    case firing
    case resolved
    case deliveryFailure = "delivery_failure"
}

public enum AlertSeverity: String, Decodable, Sendable, Equatable, Hashable {
    case info
    case warning
    case critical
}
```

### AlertScope

Spec lines 2427-2443 (all optional):

```swift
public struct AlertScope: Decodable, Sendable, Equatable, Hashable {
    public let nodeId: String?
    public let app: String?
    public let streamId: String?
    public let tenant: String?
    
    enum CodingKeys: String, CodingKey {
        case nodeId = "node_id"
        case app
        case streamId = "stream_id"
        case tenant
    }
}
```

### FleetNodeList

Spec lines 2707-2716:

```swift
public struct FleetNodeList: Decodable, Sendable, Equatable {
    public let items: [FleetNode]
    public let meta: PaginatedMeta
}
```

### FleetNode

Spec lines 2718-2762 (required: `node_id`, `role`, `status`, `last_seen`; optional:
`version`, `cpu_pct`, `mem_pct`, `net_in_mbps`, `net_out_mbps`, `os_name`, `os_arch`,
`java_version`, `processor_count`):

```swift
public struct FleetNode: Decodable, Sendable, Equatable, Hashable, Identifiable {
    public var id: String { nodeId }
    
    public let nodeId: String
    public let role: NodeRole
    public let status: NodeStatus
    public let lastSeen: Int64
    public let version: String?
    public let cpuPct: Double?
    public let memPct: Double?
    public let netInMbps: Double?
    public let netOutMbps: Double?
    public let osName: String?
    public let osArch: String?
    public let javaVersion: String?
    public let processorCount: Int?
    
    enum CodingKeys: String, CodingKey {
        case nodeId = "node_id"
        case role
        case status
        case lastSeen = "last_seen"
        case version
        case cpuPct = "cpu_pct"
        case memPct = "mem_pct"
        case netInMbps = "net_in_mbps"
        case netOutMbps = "net_out_mbps"
        case osName = "os_name"
        case osArch = "os_arch"
        case javaVersion = "java_version"
        case processorCount = "processor_count"
    }
}
```

### AnomalyList

Spec lines 2764-2773:

```swift
public struct AnomalyList: Decodable, Sendable, Equatable {
    public let items: [AnomalyFlag]
    public let meta: PaginatedMeta
}
```

### AnomalyFlag

Spec lines 2775-2800 (required: `id`, `metric`, `scope`, `observed`, `expected`, `sigma`,
`ts`):

```swift
public struct AnomalyFlag: Decodable, Sendable, Equatable, Hashable, Identifiable {
    public let id: String
    public let metric: String
    public let scope: AlertScope
    public let observed: Double
    public let expected: Double
    public let sigma: Double
    public let ts: Int64
}
```

### HealthStatus

Spec lines 3063-3088 (required: `status`, `components`; optional: `ams_env_configured`):

```swift
public struct HealthStatus: Decodable, Sendable, Equatable {
    public let status: HealthState
    public let amsEnvConfigured: Bool?
    public let components: HealthComponents
    
    enum CodingKeys: String, CodingKey {
        case status
        case amsEnvConfigured = "ams_env_configured"
        case components
    }
}

public enum HealthState: String, Decodable, Sendable, Equatable, Hashable {
    case ok
    case degraded
    case down
}

public struct HealthComponents: Decodable, Sendable, Equatable {
    public let clickhouse: ComponentStatus
    public let metaStore: ComponentStatus
    public let collector: ComponentStatus
    public let kafka: ComponentStatus?
    
    enum CodingKeys: String, CodingKey {
        case clickhouse
        case metaStore = "meta_store"
        case collector
        case kafka
    }
}
```

### ComponentStatus

Spec lines 3090-3101 (required: `status`; optional: `latency_ms`, `message`):

```swift
public struct ComponentStatus: Decodable, Sendable, Equatable, Hashable {
    public let status: HealthState
    public let latencyMs: Int?
    public let message: String?
    
    enum CodingKeys: String, CodingKey {
        case status
        case latencyMs = "latency_ms"
        case message
    }
}
```

### AuthMeResponse

Spec lines 1227-1239 (required: `name`, `role`, `auth_method`):

```swift
public struct AuthMeResponse: Decodable, Sendable, Equatable, Hashable {
    public let name: String
    public let role: String
    public let authMethod: AuthMethod
    
    enum CodingKeys: String, CodingKey {
        case name
        case role
        case authMethod = "auth_method"
    }
}

public enum AuthMethod: String, Decodable, Sendable, Equatable, Hashable {
    case bearer
    case cookie
}
```

### User

Spec lines 3392-3405 (required: `id`, `username`, `role`, `created_at`):

```swift
public struct User: Decodable, Sendable, Equatable, Hashable, Identifiable {
    public let id: String
    public let username: String
    public let role: UserRole
    public let createdAt: Int64
    
    enum CodingKeys: String, CodingKey {
        case id
        case username
        case role
        case createdAt = "created_at"
    }
}

public enum UserRole: String, Decodable, Sendable, Equatable, Hashable {
    case admin
    case viewer
}
```

### LicenseInfo

Spec lines 3220-3253 (required: `tier`, `valid`; optional: `limits`, `expires_at`,
`offline_file`):

```swift
public struct LicenseInfo: Decodable, Sendable, Equatable {
    public let tier: LicenseTier
    public let valid: Bool
    public let limits: TierLimits?
    public let expiresAt: Int64?
    public let offlineFile: Bool?
    
    enum CodingKeys: String, CodingKey {
        case tier
        case valid
        case limits
        case expiresAt = "expires_at"
        case offlineFile = "offline_file"
    }
}

public enum LicenseTier: String, Decodable, Sendable, Equatable, Hashable {
    case free
    case pro
    case business
    case enterprise
}
```

### TierLimits

Spec lines 3254-3268 (all optional):

```swift
public struct TierLimits: Decodable, Sendable, Equatable, Hashable {
    public let maxStreams: Int?
    public let maxNodes: Int?
    public let retentionDays: Int?
    public let dataApi: Bool?
    public let whiteLabel: Bool?
    
    enum CodingKeys: String, CodingKey {
        case maxStreams = "max_streams"
        case maxNodes = "max_nodes"
        case retentionDays = "retention_days"
        case dataApi = "data_api"
        case whiteLabel = "white_label"
    }
}
```

---

## C. Errors

```swift
public enum PulseAPIError: Error, Sendable, Equatable {
    /// The server URL string could not be parsed as a URL.
    case invalidServerURL(String)
    
    /// The server URL uses HTTP on a non-local host. See section E.
    case insecureTransport(host: String)
    
    /// A transport-level error (network unreachable, DNS failure, TLS error, timeout).
    case transport(description: String)
    
    /// HTTP 401: bearer token missing, expired, or invalid.
    case unauthorized
    
    /// HTTP 403: authenticated but not permitted (tier gate, role check).
    case forbidden
    
    /// HTTP 404: resource not found.
    case notFound
    
    /// HTTP 429: rate limit exceeded. `retryAfter` is seconds from the Retry-After
    /// header, or nil if the header is absent.
    case rateLimited(retryAfter: Int?)
    
    /// HTTP 5xx: server error. `apiError` is the decoded Error body if parseable.
    case server(status: Int, apiError: APIError?)
    
    /// The response body could not be decoded. `context` is a developer-readable
    /// description of what failed (e.g., "missing required field 'ts'").
    case decoding(context: String)
    
    /// The request was cancelled (Task cancellation, URLSession invalidation).
    case cancelled
}

extension PulseAPIError: LocalizedError {
    public var errorDescription: String? {
        switch self {
        case .invalidServerURL(let url):
            return "The server URL '\(url)' is not valid."
        case .insecureTransport(let host):
            return "Cannot connect to '\(host)' over HTTP. Use HTTPS, or connect to a local address."
        case .transport(let description):
            return "Network error: \(description)"
        case .unauthorized:
            return "Your session has expired. Please sign in again."
        case .forbidden:
            return "You do not have permission to access this resource."
        case .notFound:
            return "The requested resource was not found."
        case .rateLimited(let retryAfter):
            if let s = retryAfter {
                return "Too many requests. Please wait \(s) seconds."
            }
            return "Too many requests. Please wait and try again."
        case .server(let status, let apiError):
            if let msg = apiError?.message {
                return "Server error (\(status)): \(msg)"
            }
            return "Server error (\(status))."
        case .decoding(let context):
            return "Could not read the server response: \(context)"
        case .cancelled:
            return "The request was cancelled."
        }
    }
}
```

Map HTTP status codes:
- 401 -> `.unauthorized`
- 403 -> `.forbidden`
- 404 -> `.notFound`
- 429 -> `.rateLimited(retryAfter:)` (parse `Retry-After` header as Int)
- 500-599 -> `.server(status:apiError:)`
- URLError.cancelled, CancellationError -> `.cancelled`

---

## D. Transport Seam

```swift
/// A request value type passed to `PulseTransport.send`.
public struct TransportRequest: Sendable {
    public let method: String          // "GET", "POST", etc.
    public let url: URL
    public let headers: [String: String]
    public let body: Data?
    public let timeoutInterval: TimeInterval
}

/// A response value type returned by `PulseTransport.send`.
public struct TransportResponse: Sendable {
    public let statusCode: Int
    public let headers: [String: String]
    public let body: Data
}

/// The transport protocol. One async throwing method, Sendable.
public protocol PulseTransport: Sendable {
    func send(_ request: TransportRequest) async throws -> TransportResponse
}
```

### URLSession Conformance

```swift
public struct URLSessionTransport: PulseTransport {
    private let session: URLSession
    
    public init(session: URLSession = .shared) {
        self.session = session
    }
    
    public func send(_ request: TransportRequest) async throws -> TransportResponse {
        var urlRequest = URLRequest(url: request.url)
        urlRequest.httpMethod = request.method
        urlRequest.httpBody = request.body
        urlRequest.timeoutInterval = request.timeoutInterval
        for (key, value) in request.headers {
            urlRequest.setValue(value, forHTTPHeaderField: key)
        }
        
        let (data, response) = try await session.data(for: urlRequest)
        guard let http = response as? HTTPURLResponse else {
            throw PulseAPIError.transport(description: "Non-HTTP response")
        }
        
        let headers = http.allHeaderFields.reduce(into: [String: String]()) { dict, pair in
            if let k = pair.key as? String, let v = pair.value as? String {
                dict[k.lowercased()] = v
            }
        }
        
        return TransportResponse(statusCode: http.statusCode, headers: headers, body: data)
    }
}
```

Every test runs with a stub transport; no test ever touches the network. This matches the
beacon-swift SDK idiom where `MockSender` captures requests for assertions.

---

## E. Security Rule for Server URLs

Pulse is self-hosted: users enter their own server URL. The client must protect the user's
bearer token from being sent over an unencrypted channel to an untrusted network.

### Policy

- **HTTPS**: Accept for any host.
- **HTTP**: Accept ONLY for hosts that are:
  - Loopback: `127.0.0.0/8`, `::1`, `localhost`
  - Link-local: `.local` suffix
  - RFC1918 private: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`
  - RFC4193 unique-local: `fc00::/7`
  - IPv4-mapped IPv6 forms of the above: `::ffff:10.0.0.1`

### Difference from server-side ssrfguard

The server's `ssrfguard` (file `/home/aytek/repo/ams-pulse/server/internal/ssrfguard/ssrfguard.go`)
DENIES link-local (169.254.0.0/16) and cloud IMDS addresses because it protects
server-to-server SSRF. The client-side rule ALLOWS private ranges because the user is
connecting to their own AMS instance on their own network. The client-side rule exists to
protect the user's bearer token, not to prevent SSRF.

### Validation Table

These cases become a table-driven test:

| URL | Accepted | Reason |
|-----|----------|--------|
| `https://pulse.example.com` | YES | HTTPS |
| `https://10.0.0.5:8090` | YES | HTTPS (private IP is fine) |
| `https://[::1]:8090` | YES | HTTPS |
| `http://127.0.0.1:8090` | YES | HTTP loopback |
| `http://localhost:8090` | YES | HTTP loopback |
| `http://[::1]:8090` | YES | HTTP loopback |
| `http://192.168.1.100:8090` | YES | HTTP RFC1918 |
| `http://10.0.0.5:8090` | YES | HTTP RFC1918 |
| `http://172.16.0.1:8090` | YES | HTTP RFC1918 |
| `http://mynas.local:8090` | YES | HTTP .local |
| `http://[fc00::1]:8090` | YES | HTTP RFC4193 |
| `http://[::ffff:10.0.0.1]:8090` | YES | HTTP mapped IPv4 RFC1918 |
| `http://pulse.example.com` | NO | HTTP public |
| `http://8.8.8.8:8090` | NO | HTTP public |
| `http://[2001:db8::1]:8090` | NO | HTTP public IPv6 |
| `http://169.254.169.254` | YES | HTTP link-local (differs from SSRF guard) |
| `ftp://pulse.example.com` | NO | Wrong scheme |
| `not-a-url` | NO | Unparseable |

### Function Signature

```swift
public enum ServerURLValidation {
    case valid(URL)
    case invalidURL
    case insecureTransport(host: String)
}

public func validateServerURL(_ urlString: String) -> ServerURLValidation
```

---

## F. Client

```swift
public actor PulseAPIClient {
    private let baseURL: URL
    private let transport: PulseTransport
    private let tokenProvider: @Sendable () async throws -> String
    private let userAgent: String
    private let timeout: TimeInterval
    
    public init(
        baseURL: URL,
        transport: PulseTransport = URLSessionTransport(),
        tokenProvider: @escaping @Sendable () async throws -> String,
        userAgent: String = "PulseKit/1.0",
        timeout: TimeInterval = 30
    ) {
        // Normalize: strip trailing slash from baseURL
        var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: false)!
        if components.path.hasSuffix("/") {
            components.path = String(components.path.dropLast())
        }
        self.baseURL = components.url!
        self.transport = transport
        self.tokenProvider = tokenProvider
        self.userAgent = userAgent
        self.timeout = timeout
    }
}
```

### URL Construction

The spec's server URL is `/api/v1` (line 39). Health and auth endpoints are root-mounted.

Given a `baseURL` of `https://pulse.example.com` or `https://pulse.example.com/`:
- `/api/v1/live/overview` -> `https://pulse.example.com/api/v1/live/overview`
- `/healthz` -> `https://pulse.example.com/healthz`

Given a `baseURL` of `https://proxy.example.com/pulse` (path prefix):
- `/api/v1/live/overview` -> `https://proxy.example.com/pulse/api/v1/live/overview`

The client MUST join paths correctly: `baseURL.appendingPathComponent(path)` or
`URL(string: path, relativeTo: baseURL)?.absoluteURL` with proper slash handling.

### Methods

```swift
extension PulseAPIClient {
    // F1 - Live
    public func getLiveOverview(
        app: String? = nil,
        node: String? = nil,
        tenant: String? = nil
    ) async throws -> LiveOverview
    
    public func getLiveStreams(
        app: String? = nil,
        node: String? = nil,
        tenant: String? = nil,
        limit: Int? = nil,
        cursor: String? = nil
    ) async throws -> LiveStreamList
    
    // F3 - QoE
    public func getQoeSummary(
        from: Int64? = nil,
        to: Int64? = nil,
        app: String? = nil,
        stream: String? = nil,
        tenant: String? = nil,
        interval: QueryInterval? = nil,
        country: String? = nil,
        device: String? = nil
    ) async throws -> QoeSummaryResponse
    
    // F5 - Alerts
    public func getAlertHistory(
        from: Int64? = nil,
        to: Int64? = nil,
        limit: Int? = nil,
        cursor: String? = nil,
        ruleId: String? = nil,
        state: AlertState? = nil
    ) async throws -> AlertHistoryList
    
    // F7 - Fleet
    public func getFleetNodes(
        limit: Int? = nil,
        cursor: String? = nil
    ) async throws -> FleetNodeList
    
    // F9 - Anomalies
    public func getAnomalies(
        from: Int64? = nil,
        to: Int64? = nil,
        app: String? = nil,
        stream: String? = nil,
        limit: Int? = nil,
        cursor: String? = nil,
        metric: String? = nil,
        minSigma: Double? = nil
    ) async throws -> AnomalyList
    
    // Health (root-mounted, no /api/v1 prefix)
    public func getHealth() async throws -> HealthStatus
    
    // Auth (root-mounted)
    public func getAuthMe() async throws -> AuthMeResponse
    
    // Admin
    public func getLicense() async throws -> LicenseInfo
}

public enum QueryInterval: String, Sendable {
    case hour
    case day
}
```

Query parameter handling: include only non-nil parameters. Encode values URL-safe.

### Bearer Auth

Every request except `/healthz` and `/auth/oidc/*` includes:
```
Authorization: Bearer <token>
```

The token comes from `tokenProvider`. If it throws, propagate the error.

---

## G. Credentials

### PulseServer

```swift
public struct PulseServer: Sendable, Equatable, Hashable, Identifiable, Codable {
    public let id: UUID
    public var displayName: String
    public var baseURL: URL
    public var notes: String?
    public let createdAt: Date
    
    public init(
        id: UUID = UUID(),
        displayName: String,
        baseURL: URL,
        notes: String? = nil,
        createdAt: Date = Date()
    ) {
        self.id = id
        self.displayName = displayName
        self.baseURL = baseURL
        self.notes = notes
        self.createdAt = createdAt
    }
}
```

### TokenStore Protocol

```swift
public protocol TokenStore: Sendable {
    func getToken(forServerID id: UUID) async throws -> String?
    func setToken(_ token: String, forServerID id: UUID) async throws
    func deleteToken(forServerID id: UUID) async throws
}
```

### InMemoryTokenStore

For tests and Linux. Thread-safe via actor isolation.

```swift
public actor InMemoryTokenStore: TokenStore {
    private var tokens: [UUID: String] = [:]
    
    public init() {}
    
    public func getToken(forServerID id: UUID) async throws -> String? {
        tokens[id]
    }
    
    public func setToken(_ token: String, forServerID id: UUID) async throws {
        tokens[id] = token
    }
    
    public func deleteToken(forServerID id: UUID) async throws {
        tokens[id] = nil
    }
}
```

### KeychainTokenStore

Guarded by `#if canImport(Security)`. Uses the system Keychain on iOS/macOS.
The import must be conditional so the package builds on Linux.

```swift
#if canImport(Security)
import Security

public final class KeychainTokenStore: TokenStore, @unchecked Sendable {
    private let service: String
    private let accessGroup: String?
    
    public init(service: String = "com.pulse.api", accessGroup: String? = nil) {
        self.service = service
        self.accessGroup = accessGroup
    }
    
    // Implementation uses SecItemCopyMatching, SecItemAdd, SecItemUpdate, SecItemDelete
    // with kSecAttrAccount = serverId.uuidString
}
#endif
```

---

## H. View Models

Use the Observation module (`import Observation`). Verified to work on Linux with Swift 6.

### LoadingState

```swift
public enum LoadingState<T: Sendable>: Sendable {
    case idle
    case loading
    case loaded(T)
    case failed(PulseAPIError)
}
```

### ServerStore

```swift
public protocol ServerPersistence: Sendable {
    func loadServers() async throws -> [PulseServer]
    func saveServers(_ servers: [PulseServer]) async throws
}

@Observable
public final class ServerStore: @unchecked Sendable {
    public private(set) var servers: [PulseServer] = []
    public var selectedServerID: UUID?
    
    private let persistence: ServerPersistence
    
    public init(persistence: ServerPersistence) {
        self.persistence = persistence
    }
    
    public var selectedServer: PulseServer? {
        guard let id = selectedServerID else { return nil }
        return servers.first { $0.id == id }
    }
    
    @MainActor
    public func load() async throws {
        servers = try await persistence.loadServers()
    }
    
    @MainActor
    public func add(_ server: PulseServer) async throws {
        servers.append(server)
        try await persistence.saveServers(servers)
    }
    
    @MainActor
    public func update(_ server: PulseServer) async throws {
        guard let index = servers.firstIndex(where: { $0.id == server.id }) else { return }
        servers[index] = server
        try await persistence.saveServers(servers)
    }
    
    @MainActor
    public func delete(id: UUID) async throws {
        servers.removeAll { $0.id == id }
        if selectedServerID == id { selectedServerID = nil }
        try await persistence.saveServers(servers)
    }
}
```

Concurrency: `@unchecked Sendable` because mutations are constrained to `@MainActor`.
The `@Observable` macro generates the observation machinery.

### OverviewModel

```swift
@Observable
public final class OverviewModel: @unchecked Sendable {
    public private(set) var state: LoadingState<LiveOverview> = .idle
    
    private let client: PulseAPIClient
    
    public init(client: PulseAPIClient) {
        self.client = client
    }
    
    @MainActor
    public func refresh() async {
        state = .loading
        do {
            let overview = try await client.getLiveOverview()
            state = .loaded(overview)
        } catch let error as PulseAPIError {
            state = .failed(error)
        } catch {
            state = .failed(.transport(description: error.localizedDescription))
        }
    }
}
```

### StreamsModel

```swift
@Observable
public final class StreamsModel: @unchecked Sendable {
    public private(set) var state: LoadingState<[LiveStream]> = .idle
    public private(set) var streams: [LiveStream] = []
    public private(set) var nextCursor: String?
    public private(set) var hasMore: Bool = true
    public var searchQuery: String = ""
    
    private let client: PulseAPIClient
    
    public init(client: PulseAPIClient) {
        self.client = client
    }
    
    @MainActor
    public func refresh() async {
        streams = []
        nextCursor = nil
        hasMore = true
        await loadPage()
    }
    
    @MainActor
    public func loadNextPage() async {
        guard hasMore, case .loaded = state else { return }
        await loadPage()
    }
    
    @MainActor
    private func loadPage() async {
        state = .loading
        do {
            let list = try await client.getLiveStreams(cursor: nextCursor)
            streams.append(contentsOf: list.items)
            nextCursor = list.meta.nextCursor
            hasMore = list.meta.nextCursor != nil
            state = .loaded(streams)
        } catch let error as PulseAPIError {
            state = .failed(error)
        } catch {
            state = .failed(.transport(description: error.localizedDescription))
        }
    }
}
```

### AlertsModel

```swift
@Observable
public final class AlertsModel: @unchecked Sendable {
    public private(set) var state: LoadingState<[AlertHistoryEntry]> = .idle
    public private(set) var alerts: [AlertHistoryEntry] = []
    public private(set) var nextCursor: String?
    public private(set) var hasMore: Bool = true
    
    private let client: PulseAPIClient
    
    public init(client: PulseAPIClient) {
        self.client = client
    }
    
    @MainActor
    public func refresh() async {
        alerts = []
        nextCursor = nil
        hasMore = true
        await loadPage()
    }
    
    @MainActor
    public func loadNextPage() async {
        guard hasMore, case .loaded = state else { return }
        await loadPage()
    }
    
    @MainActor
    private func loadPage() async {
        state = .loading
        do {
            let list = try await client.getAlertHistory(cursor: nextCursor)
            alerts.append(contentsOf: list.items)
            nextCursor = list.meta.nextCursor
            hasMore = list.meta.nextCursor != nil
            state = .loaded(alerts)
        } catch let error as PulseAPIError {
            state = .failed(error)
        } catch {
            state = .failed(.transport(description: error.localizedDescription))
        }
    }
}
```

---

## I. Formatting

Pure functions, Linux-testable. No implicit clock; `now` is always a parameter.

```swift
public enum Formatters {
    /// Format bitrate in kbps to human-readable string.
    /// 1234.5 -> "1.2 Mbps", 456 -> "456 kbps"
    public static func bitrateString(kbps: Double) -> String
    
    /// Abbreviate large counts. 12847 -> "12.8K", 1234567 -> "1.2M"
    public static func abbreviatedCount(_ count: Int) -> String
    
    /// Format duration in seconds. 3661 -> "1h 1m 1s", 65 -> "1m 5s"
    public static func durationString(seconds: Int) -> String
    
    /// Relative time from a date. 
    /// now - 30s -> "30s ago", now - 3600s -> "1h ago"
    public static func relativeTime(from date: Date, to now: Date) -> String
    
    /// Map health status to a token name from tokens.json.
    /// "ok" -> "healthy", "degraded" -> "warning", "down" -> "critical"
    public static func healthTokenName(_ status: HealthState) -> String
}
```

Token name mapping (from `brandkit/design-system/tokens.json`):
- `ok` -> `"healthy"` (color.dark.healthy / color.light.healthy)
- `degraded` -> `"warning"`
- `down` -> `"critical"`

---

## J. Test Plan

### LANE-MODELS: `ModelDecodingTests.swift`

**Unit tests:**
- `test_LiveOverview_decodesRequiredFields`
- `test_LiveOverview_decodesOptionalFieldsWhenPresent`
- `test_ProtocolMix_defaultsToZeroForMissingFields`
- `test_LiveStream_decodesAllFields`
- `test_QoeTotals_decodesSnakeCaseFields`
- `test_BitrateBucket_optionalP95`
- `test_AlertHistoryEntry_decodesAllStates`
- `test_FleetNode_decodesAllFields`
- `test_AnomalyFlag_decodesScope`
- `test_HealthStatus_decodesComponents`
- `test_ComponentStatus_optionalFields`
- `test_AuthMeResponse_decodesAuthMethod`
- `test_LicenseInfo_allTiers`
- `test_TierLimits_allOptional`
- `test_User_decodesRole`

**Edge cases:**
- `test_LiveStreamList_emptyItems`
- `test_PaginatedMeta_nullNextCursor`
- `test_PaginatedMeta_nullTotal`
- `test_ProtocolMix_allFieldsZero`
- `test_LiveStream_maxViewerCount` (Int.max)
- `test_AlertHistoryEntry_unicodeMetricName`
- `test_FleetNode_unicodeVersion`
- `test_Timestamp_maxInt64Value`

**Failure paths:**
- `test_LiveOverview_missingRequiredField_throws`
- `test_NodeHealth_invalidStatusEnum_throws`
- `test_AlertState_unknownValue_throws`
- `test_truncatedJSON_throws`
- `test_wrongContentType_arrayInsteadOfObject`

**Regression (from fixtures):**
- `test_liveOverviewFixture_matchesSpec`
- `test_liveStreamsFixture_matchesSpec`
- `test_qoeSummaryFixture_matchesSpec`
- `test_alertsHistoryFixture_matchesSpec`
- `test_fleetNodesFixture_matchesSpec`
- `test_anomaliesFixture_matchesSpec`
- `test_healthzFixture_matchesSpec`
- `test_authMeFixture_matchesSpec`
- `test_licenseFixture_matchesSpec`

### LANE-MODELS: `ErrorDecodingTests.swift`

- `test_APIError_decodesCodeAndMessage`
- `test_APIError_optionalDetails`
- `test_APIError_detailsWithNestedObject`

### LANE-MODELS: `PaginationTests.swift`

- `test_PaginatedMeta_decodesNextCursor`
- `test_PaginatedMeta_decodesTotal`
- `test_PaginatedMeta_bothNull`

### LANE-NET: `PulseAPIErrorTests.swift`

- `test_localizedDescription_allCases`
- `test_equatable_sameCase`
- `test_equatable_differentCase`

### LANE-NET: `ServerURLValidatorTests.swift`

Table-driven tests from section E:
- `test_httpsAnyHost_accepted`
- `test_httpLoopback127_accepted`
- `test_httpLoopbackLocalhost_accepted`
- `test_httpLoopbackIPv6_accepted`
- `test_httpRFC1918_10_accepted`
- `test_httpRFC1918_172_accepted`
- `test_httpRFC1918_192_accepted`
- `test_httpDotLocal_accepted`
- `test_httpRFC4193_accepted`
- `test_httpMappedIPv4Private_accepted`
- `test_httpPublicDomain_rejected`
- `test_httpPublicIPv4_rejected`
- `test_httpPublicIPv6_rejected`
- `test_httpLinkLocal169_accepted` (differs from server SSRF)
- `test_ftpScheme_rejected`
- `test_invalidURL_rejected`
- `test_emptyString_rejected`

### LANE-NET: `PulseAPIClientTests.swift`

**Unit tests:**
- `test_getLiveOverview_correctPath`
- `test_getLiveOverview_bearerHeader`
- `test_getLiveStreams_queryParams`
- `test_getQoeSummary_allQueryParams`
- `test_getAlertHistory_stateFilter`
- `test_getFleetNodes_pagination`
- `test_getAnomalies_minSigmaParam`
- `test_getHealth_noAuthHeader`
- `test_getAuthMe_rootMounted`
- `test_getLicense_apiV1Path`
- `test_baseURLWithTrailingSlash_normalized`
- `test_baseURLWithPathPrefix_joined`
- `test_userAgentHeader_included`
- `test_timeout_appliedToRequest`

**Failure paths:**
- `test_401_throwsUnauthorized`
- `test_403_throwsForbidden`
- `test_404_throwsNotFound`
- `test_429_throwsRateLimited`
- `test_429_parsesRetryAfterHeader`
- `test_500_throwsServer`
- `test_502_throwsServerWithNilError`
- `test_malformedJSON_throwsDecoding`
- `test_emptyBody_throwsDecoding`
- `test_wrongContentType_throwsDecoding`
- `test_networkError_throwsTransport`
- `test_taskCancellation_throwsCancelled`

### LANE-NET: `TokenStoreTests.swift`

- `test_inMemory_setAndGet`
- `test_inMemory_delete`
- `test_inMemory_getNonexistent_returnsNil`
- `test_inMemory_overwrite`

Keychain tests (Darwin only):
- `test_keychain_setAndGet`
- `test_keychain_delete`
- `test_keychain_overwrite`

### LANE-VM: `FormatterTests.swift`

- `test_bitrateString_kbps`
- `test_bitrateString_mbps`
- `test_bitrateString_gbps`
- `test_bitrateString_zero`
- `test_abbreviatedCount_thousands`
- `test_abbreviatedCount_millions`
- `test_abbreviatedCount_zero`
- `test_abbreviatedCount_exact1000`
- `test_durationString_seconds`
- `test_durationString_minutes`
- `test_durationString_hours`
- `test_durationString_zero`
- `test_relativeTime_seconds`
- `test_relativeTime_minutes`
- `test_relativeTime_hours`
- `test_relativeTime_days`
- `test_relativeTime_future_returnsJustNow`
- `test_healthTokenName_ok`
- `test_healthTokenName_degraded`
- `test_healthTokenName_down`

### LANE-VM: `ViewModelTests.swift`

- `test_OverviewModel_refresh_setsLoading`
- `test_OverviewModel_refresh_setsLoaded`
- `test_OverviewModel_refresh_setsFailed`
- `test_StreamsModel_refresh_clearsExisting`
- `test_StreamsModel_loadNextPage_appends`
- `test_StreamsModel_loadNextPage_setsHasMoreFalse`
- `test_AlertsModel_refresh_clearsExisting`
- `test_AlertsModel_pagination_works`
- `test_ServerStore_add_persistsCalled`
- `test_ServerStore_delete_updatesSelectedServer`
- `test_ServerStore_load_populatesServers`

---

## Decisions a Reader Would Be Surprised By

1. **Timestamps are epoch milliseconds, not ISO strings.** The spec description says
   responses "always return Unix epoch milliseconds integers" (line 27). No RFC3339
   decoding is needed for response models.

2. **ProtocolMix has defaults.** All properties are optional with `default: 0` in the spec.
   The Swift type uses non-optional `Int` with a custom `init(from:)` that applies
   the defaults.

3. **Link-local 169.254.x.x is allowed client-side.** The server's ssrfguard denies
   169.254.0.0/16 (IMDS addresses). The client-side rule allows it because the client
   is not at risk of SSRF; the user might legitimately have a local AMS on a link-local
   address. The client-side rule protects the bearer token, not cloud metadata.

4. **Health endpoint is root-mounted.** `/healthz` does not have the `/api/v1` prefix
   (spec line 981). The client must not prepend the API prefix for this path.

5. **Auth /auth/me is also root-mounted.** The spec shows `servers: [{url: /}]` for
   `/auth/me` (line 1206). Same handling as healthz.

6. **PaginatedMeta uses snake_case.** The wire field is `next_cursor` (line 1876), so
   CodingKeys must map to `nextCursor`.

7. **AlertHistoryEntry's `id` conflicts with Identifiable.** The wire field is `id`, but
   Swift's `Identifiable` requires an `id` property. Solved by using `alertId` as the
   CodingKey target and a computed `var id` that returns `alertId`.

8. **View models use @unchecked Sendable.** The `@Observable` macro does not automatically
   synthesize Sendable conformance. The models are safe because all mutations happen on
   `@MainActor`.

9. **All tests use injected "now" for time.** `relativeTime(from:to:)` takes both dates
   as parameters, not an implicit clock, so tests are deterministic.

10. **No UIKit import outside canImport guard.** The beacon-swift SDK demonstrates this
    pattern for lifecycle hooks. PulseKit follows the same rule so it builds on Linux.
