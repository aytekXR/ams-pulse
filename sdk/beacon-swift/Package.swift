// swift-tools-version:5.9
//
// PulseBeacon — player-side QoE telemetry SDK for Pulse (Ant Media Server analytics).
// iOS/tvOS/macOS via Xcode; the cross-platform core also builds + tests on Linux
// (Foundation + Dispatch only), which is how CI gates it. Zero third-party dependencies.
//
// Wire contract: contracts/events/beacon-event.schema.json (frozen, D-004) — kept
// field-for-field in sync with sdk/beacon-js. Do not diverge from the JS beacon schema.
import PackageDescription

let package = Package(
    name: "PulseBeacon",
    products: [
        .library(name: "PulseBeacon", targets: ["PulseBeacon"]),
    ],
    targets: [
        .target(name: "PulseBeacon"),
        .testTarget(name: "PulseBeaconTests", dependencies: ["PulseBeacon"]),
    ]
)
