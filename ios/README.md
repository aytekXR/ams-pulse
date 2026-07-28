# Pulse iOS App

SwiftUI iOS client for [Pulse](../README.md), self-hosted analytics for Ant Media Server.

## Architecture

The iOS codebase is split into two layers:

| Layer | Path | Platform | Purpose |
|-------|------|----------|---------|
| **PulseKit** | `ios/PulseKit/` | Foundation (Linux + Darwin) | API client, models, formatters. Zero UIKit. |
| **PulseApp** | `ios/PulseApp/` | iOS 17+ | SwiftUI views, Keychain, asset catalog. |

**Why the split?** PulseKit builds and tests on Linux in CI before the expensive macOS
runner spins up. This catches Foundation-only regressions early (model decoding, API
calls, formatters) and keeps the logic layer portable. The SwiftUI layer (PulseApp)
is necessarily iOS/macOS-only, but it is thin: views, environment, and Security framework.

### Targets

XcodeGen produces two targets from `project.yml`:

| Target | Type | Description |
|--------|------|-------------|
| `Pulse` | application | The iOS app. Depends on PulseKit and PulseBeacon. |
| `PulseTests` | bundle.unit-test | Unit tests for the app layer. Hosted by Pulse. |

The test target is named `PulseTests` (not `PulseAppTests`) to match the CI workflow's
`-only-testing:PulseTests` flag in `.github/workflows/ios.yml`.

## Requirements

- Xcode 16.0+ (CI uses 16.2; TestFlight uploads require Xcode 26+)
- iOS 17.0+ deployment target
- Swift 6 (strict concurrency)
- [XcodeGen](https://github.com/yonaskolb/XcodeGen) 2.45.4 (installed via Homebrew)

## Build Locally

```bash
# Install XcodeGen (one-time)
brew install xcodegen

# Generate the Xcode project
cd ios
xcodegen generate

# Open in Xcode
open Pulse.xcodeproj
```

Then select the "Pulse" scheme and build for a simulator (Cmd+B) or device.

## CI Build

The `.github/workflows/ios.yml` workflow runs two jobs:

1. **ios-kit** (Linux, Swift 6.1 container): builds and tests PulseKit. REQUIRED gate.
2. **ios-app** (macOS-15, Xcode 16.2): generates project, builds, and tests PulseApp.

The ios-app job:
1. Selects Xcode 16.2 (`sudo xcode-select -s /Applications/Xcode_16.2.app`)
2. Installs XcodeGen via Homebrew
3. Runs `xcodegen generate` to produce `Pulse.xcodeproj`
4. Resolves local Swift packages (PulseKit, PulseBeacon)
5. Builds for iOS Simulator (iPhone 16, iOS 18.5)
6. Runs tests with `-only-testing:PulseTests`

### Deployment Target vs. Simulator Runtime

**The deployment target is iOS 17.0 but CI runs on iOS 18.5.** This is correct.

A deployment target is a *floor*: the oldest iOS version the app can run on. The app
is compiled against the iOS 18 SDK (from Xcode 16.2), which can target iOS 17.0+.
CI tests on iOS 18.5 because that is what the macos-15 runner ships; there is no
iOS 17 simulator runtime on that runner. Testing on 18.5 validates the build and
tests pass on a supported version; it does not change backward compatibility.

This is standard practice. Do not "fix" it by changing the deployment target to 18.x.

## Bundle Identifier

Default: `com.beyondkaira.pulse` (set in `project.yml` under `settings.base`).

CI can override at build time:

```bash
xcodebuild build \
  PRODUCT_BUNDLE_IDENTIFIER=com.example.pulse \
  ...
```

This is how TestFlight builds use a team-specific bundle ID without modifying the repo.

## Code Signing

Simulator builds disable signing:
```bash
CODE_SIGNING_ALLOWED=NO
```

TestFlight builds require:
- `DEVELOPMENT_TEAM=XXXXXXXXXX` (the 10-character team ID)
- `CODE_SIGN_STYLE=Automatic` (already set in project.yml)
- Valid provisioning profile (Xcode auto-manages for Automatic signing)

## Project Structure

```
ios/
  project.yml           # XcodeGen spec (source of truth)
  .gitignore            # Excludes generated Pulse.xcodeproj
  PulseKit/             # Foundation-only SDK (SwiftPM package)
    Package.swift
    Sources/PulseKit/
    Tests/PulseKitTests/
  PulseApp/             # iOS app target
    Info.plist          # App configuration (ATS, export compliance)
    PrivacyInfo.xcprivacy  # Privacy manifest (required since May 2024)
    Sources/PulseApp/
      PulseApp.swift    # @main entry point
      AppState.swift    # Observable state container
      BrandColors.swift # Design tokens from brandkit/
      Views/            # SwiftUI views
      Services/         # KeychainService
      Assets.xcassets/  # App icon, accent color
    Tests/PulseAppTests/
      BrandColorsTests.swift
      KeychainServiceTests.swift
```

## App Transport Security (ATS)

Pulse is self-hosted software. Users may run it on:
- **HTTPS with valid CA cert** — works by default.
- **HTTPS with self-signed cert** — user must trust in iOS Settings.
- **HTTP on local network** — allowed via `NSAllowsLocalNetworking`.

The Info.plist enables `NSAllowsLocalNetworking`, which allows HTTP connections
to local network addresses (192.168.x.x, 10.x.x.x, localhost, *.local) without
affecting public internet connections. This does NOT blanket-disable ATS.

For production deployments, HTTPS with a valid certificate is recommended.

## Export Compliance (ITSAppUsesNonExemptEncryption)

The app declares `ITSAppUsesNonExemptEncryption = false` in Info.plist.

**Reasoning:** The app uses only standard HTTPS via URLSession for API
communication. Per Apple TN3157, apps using standard web protocols (HTTPS, TLS)
for server communication are exempt from export documentation requirements
under ECCN 5D992.

This declaration:
- Prevents the "Missing Export Compliance" warning in TestFlight.
- Requires no annual self-classification report.
- Is accurate because no custom encryption is implemented.

## Privacy Manifest (PrivacyInfo.xcprivacy)

Apple requires a privacy manifest since May 2024. The manifest declares:

| Category | Value | Reason |
|----------|-------|--------|
| Tracking | NO | App does not track users. |
| Tracking Domains | (empty) | No third-party trackers. |
| Collected Data | (empty) | No data sent to developer/third parties. |
| Required-Reason APIs | (empty) | None of the listed APIs are used. |

### Required-Reason API Audit

The codebase was audited for Apple's required-reason APIs (WWDC23-10060):

| API Category | Used? | Location | Notes |
|--------------|-------|----------|-------|
| File timestamp APIs | NO | — | No `NSFileModificationDate` or `modificationDate` access. |
| System boot time APIs | NO | — | No `CACurrentMediaTime`, `mach_absolute_time`, `systemBootTime`. |
| Disk space APIs | NO | — | No `NSFileManager` disk space queries. |
| Active keyboards API | NO | — | No `UITextInputMode` queries. |
| User defaults APIs | NO | — | Credentials stored in Keychain, not UserDefaults. |

**Verification method:** `grep -rn` over `ios/PulseKit/Sources/` and `ios/PulseApp/Sources/`
for all required-reason API patterns. Only match: a comment in KeychainService.swift
explaining *why* Keychain is used instead of UserDefaults.

**Why Keychain over UserDefaults:** API tokens are secrets. The iOS Keychain
encrypts data at rest, excludes it from unencrypted backups, and is the
App Store-expected practice for credentials. See `Services/KeychainService.swift`.

## App Icon

The app uses iOS's modern single-size icon format (iOS 18+):
- One 1024x1024 PNG for light mode.
- One 1024x1024 PNG for dark mode.

Source files: `brandkit/icons/png/app-icon-ios-{light,dark}-1024.png`
Target: `PulseApp/Sources/PulseApp/Assets.xcassets/AppIcon.appiconset/`

Both icons are RGBA PNG, verified 1024x1024.

## Accent Color

The accent color adapts to light/dark mode per tokens.json:
- Light mode: `#087A59` (darker signal green, WCAG AA on white)
- Dark mode: `#2CE5A7` (signal green)

Defined in `Assets.xcassets/AccentColor.colorset/`.

## Testing

```bash
# Run PulseKit tests on Linux (CI does this first)
cd ios/PulseKit && swift test

# Run PulseApp tests in Xcode
cd ios
xcodegen generate
xcodebuild test \
  -scheme Pulse \
  -destination 'platform=iOS Simulator,name=iPhone 16'
```

## Design System

All colors and typography come from `brandkit/design-system/tokens.json`.
See `BrandColors.swift` for the SwiftUI adaptation layer.

Key rules from `brandkit/documentation/design-rationale.md`:
- **Status is shape + color, never hue alone.** Healthy/warn/critical/offline
  each pair a fixed shape with the color for accessibility.
- **Signal green = live/healthy/primary action.** The brand accent and healthy
  state are the same hue intentionally.
- **Amber and red are reserved for state.** Never decorative use.

## What Has Not Been Verified

There is no Mac in this development environment. The following are unverified:

1. **Code signing** — all measurements used `CODE_SIGNING_ALLOWED=NO`.
2. **Archive/export** — `xcodebuild archive` has not been run.
3. **TestFlight upload** — `xcrun altool` / Transporter behavior is untested.
4. **Provisioning profiles** — automatic signing flow is theoretical.
5. **App Store Connect acceptance** of `PrivacyInfo.xcprivacy` is untested.
6. **Device builds** — only simulator builds have been verified.
7. **Xcode 26 builds** — TestFlight requires Xcode 26 since 2026-04-28, but CI
   pins Xcode 16.2 for deterministic checks. A separate workflow for TestFlight
   uploads will need to select Xcode 26.x.

These items require operator action on a Mac with an Apple Developer account.

## License

See [LICENSE](../LICENSE) in the repository root.
