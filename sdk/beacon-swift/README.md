# PulseBeacon (Swift)

Player-side QoE telemetry SDK for [Pulse](../../README.md) — self-hosted analytics
and QoE monitoring for Ant Media Server. iOS/tvOS/macOS via Xcode; the cross-platform
core also builds and is unit-tested on **Linux** (Foundation + Dispatch only), which is
how CI gates it. **Zero third-party dependencies.**

It is the native-app counterpart of [`sdk/beacon-js`](../beacon-js) and posts the exact
same wire payload — `contracts/events/beacon-event.schema.json` (frozen, D-004). Do not
let the two diverge.

## Install (Swift Package Manager)

A dedicated, tagged SPM repository for `PulseBeacon` is planned. Until then, two
working options are available:

### Option A — local path (recommended)

Clone the `ams-pulse` repository and reference the SDK via a local path in your
`Package.swift`:

```swift
// Package.swift
dependencies: [
    .package(path: "../ams-pulse/sdk/beacon-swift"),
],
targets: [
    .target(name: "YourApp", dependencies: [
        .product(name: "PulseBeacon", package: "beacon-swift"),
    ]),
]
```

Adjust the relative path (`../ams-pulse/sdk/beacon-swift`) to match your checkout
location.

### Option B — copy the sources

Copy the Swift source files directly into your project:

```
sdk/beacon-swift/Sources/PulseBeacon/
  JSONValue.swift
  Types.swift
  Session.swift
  Transport.swift
  PulseBeacon.swift
```

No build system integration required — add the five files to your Xcode target or
`Package.swift` sources list.

## Usage

```swift
import PulseBeacon

// One beacon per viewer session.
let beacon = PulseBeacon(config: .init(
    ingestURL: "https://pulse.example.com",   // your Pulse collector (HTTPS)
    token: "plt_your_ingest_token",           // an ingest token issued by Pulse
    streamID: "my-stream",                    // matches the AMS stream name
    app: "live",                              // AMS application (optional)
    metadata: ["tenant": "acme"],             // optional string tags
    sampleRate: 1.0,                          // 0…1; decided once per session
    playerKind: .amsWebRTC
))

// Report QoE as playback proceeds (typed helpers build the schema `data` shapes):
beacon.sessionStart(autoplay: true)
beacon.startupComplete(startupMs: 320, bitrateKbps: 1500)
beacon.heartbeat(watchMs: 30_000, bitrateKbps: 1500, bufferMs: 4_000, droppedFrames: 0)
beacon.rebufferStart(bufferMs: 0)
beacon.rebufferEnd(durationMs: 850)
beacon.bitrateChange(fromKbps: 1500, toKbps: 800)
beacon.resolutionChange(from: "1280x720", to: "854x480")
beacon.error(code: "MEDIA_ERR_NETWORK", message: "stalled", fatal: false)

// When playback ends (flushes any pending events):
beacon.dispose(reason: "user_exit")
```

For a custom event shape, use the generic `event(_:data:)` with a
`[String: JSONValue]` payload.

## Behavior

- **Batching:** events are buffered and flushed every ≤10 s, at 25 buffered events, on
  `dispose()`, and — on iOS — when the app enters the background.
- **Delivery:** `POST <ingestURL>/ingest/beacon` with `X-Pulse-Ingest-Token`. Failed
  sends go to a bounded (100-deep) retry queue with exponential backoff (1 s → 60 s cap);
  never throws, never blocks the caller (all work is on a private serial queue).
- **Sampling:** decided once per session from `sampleRate`; an unsampled session is a
  complete no-op (nothing is sent).
- **Privacy:** carries only what you pass plus the session UUID — no more than the JS SDK.

## Platform notes

The core (`Types`, `Session`, `Transport`, `PulseBeacon`) uses only `Foundation` and
`Dispatch`, so it builds and unit-tests on Linux. The only iOS-specific code — a
background-flush observer on `UIApplication.didEnterBackgroundNotification` — is behind
`#if canImport(UIKit)` and is exercised on Xcode/CI, not on Linux.

## Size discipline

Swift compiles to a static library, so there is no shipped-bundle byte count to gate like
the JS SDK's 15 KB gzip limit. The analog here is **zero third-party dependencies** and a
small source footprint (~600 LOC). Keep both.

## Develop

```bash
cd sdk/beacon-swift
swift build          # debug
swift build -c release
swift test           # 22 tests, Linux-clean
```

## Status

**Phase 1** (this package): the cross-platform core + full unit tests, buildable and gated
on Linux. **Phase 2** (needs Xcode/an Apple CI runner): a background `URLSession`
configuration and a worked SwiftUI/AVPlayer integration sample.
