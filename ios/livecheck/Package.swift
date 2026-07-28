// swift-tools-version:6.0
// livecheck: live-server contract validation for PulseKit.
//
// This executable points PulseKit at a REAL Pulse server and calls every GET endpoint.
// It proves that the decoders in PulseKit match a server that actually exists — which
// the unit tests cannot prove, because they decode from static fixtures.
//
// NOT part of CI: there is no server in CI to point at. Run it manually against a dev
// or staging Pulse deployment when validating a new PulseKit release or debugging a
// wire-format mismatch.
//
// Usage:
//   export PULSE_URL="https://pulse.example.com"
//   export PULSE_TOKEN="plt_abc123..."
//   swift run livecheck

import PackageDescription

let package = Package(
    name: "livecheck",
    platforms: [
        .macOS(.v14),
        .iOS(.v17)
    ],
    products: [
        .executable(
            name: "livecheck",
            targets: ["livecheck"]
        )
    ],
    dependencies: [
        // Local path dependency to PulseKit — they live in the same repo.
        .package(path: "../PulseKit")
    ],
    targets: [
        .executableTarget(
            name: "livecheck",
            dependencies: [
                .product(name: "PulseKit", package: "PulseKit")
            ],
            swiftSettings: [
                .swiftLanguageMode(.v6)
            ]
        )
    ]
)
