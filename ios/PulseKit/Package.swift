// swift-tools-version:6.0
// The swift-tools-version declares the minimum version of Swift required to build this package.
//
// PulseKit: Foundation-only API client and models for Pulse analytics.
// This package MUST build and test on Linux (no UIKit/SwiftUI/AVFoundation imports).
// All platform-specific UI code belongs in the sibling PulseApp target.

import PackageDescription

let package = Package(
    name: "PulseKit",
    // Platforms constraint applies only to Apple platforms; on Linux these are ignored.
    // The package must compile on Linux (Swift 6.1+) for CI verification.
    platforms: [
        .iOS(.v17),
        .macOS(.v14)
    ],
    products: [
        .library(
            name: "PulseKit",
            targets: ["PulseKit"]
        )
    ],
    // HARD RULE: No third-party dependencies. Foundation only.
    // This matches sdk/beacon-swift and keeps the App Store review surface small.
    dependencies: [],
    targets: [
        .target(
            name: "PulseKit",
            dependencies: [],
            swiftSettings: [
                // Swift 6 strict concurrency checking — types crossing concurrency
                // boundaries must be Sendable. No silencing warnings without justification.
                .swiftLanguageMode(.v6)
            ]
        ),
        .testTarget(
            name: "PulseKitTests",
            dependencies: ["PulseKit"],
            resources: [
                .copy("Fixtures")
            ],
            swiftSettings: [
                .swiftLanguageMode(.v6)
            ]
        )
    ]
)
