# iOS CI — measured runner facts

Last updated: 2026-07-28

> **This file records what was MEASURED, not what was assumed.** Every line below came out of a
> throwaway capability probe run on a real GitHub-hosted macOS runner
> ([run 30392825535](https://github.com/aytekXR/ams-pulse/actions/runs/30392825535), branch
> `ci/macos-probe`, deleted after its answers were recorded). This repository's most expensive
> recurring defect class is a plausible claim about a third party that nobody ran. iOS is the
> worst possible place to repeat it: there is no Apple toolchain on the VPS, so *every* statement
> about Xcode, simulators or App Store Connect is a claim about a machine we cannot see.
>
> **Re-measure before trusting this file after a runner-image bump.** GitHub rolls the
> `macos-15` image roughly monthly and the *default* Xcode moves with it.

---

## 1. The runner

| Fact | Measured value |
|---|---|
| Label | `macos-15` |
| Image | `macos-15-arm64`, release `20260715.0234` |
| OS | macOS 15.7.7 |
| Architecture | `arm64` (Apple silicon — **not** x86_64) |
| Cost | free for public repositories; `aytekXR/ams-pulse` is public |

## 2. Xcode

| Fact | Measured value |
|---|---|
| **Default** `xcodebuild -version` | **Xcode 16.4** (build `16F6`) |
| Installed under `/Applications` | `Xcode_16.0` · `16.1` · `16.2` · `16.3` · `16.4` · `26.0` · `26.0.1` · `26.1` · `26.1.1` · `26.2.0` · `26.3.0` |

⚠️ **The default Xcode cannot ship an App Store build.** Apple's published requirement, verified
against [developer.apple.com/news/upcoming-requirements](https://developer.apple.com/news/upcoming-requirements/):

> *"Apps uploaded to App Store Connect must be built with Xcode 26 or later using an SDK for
> iOS 26, iPadOS 26, tvOS 26, visionOS 26, or watchOS 26."* — in force **since 2026-04-28**.

So a job that takes the runner default (16.4) produces an artifact that App Store Connect
**rejects at upload**, long after CI has gone green. Any job whose output is destined for
TestFlight **must** select Xcode 26.x explicitly (`sudo xcode-select -s /Applications/Xcode_26.x.app`)
and the selection must be asserted, not assumed — print `xcodebuild -version` and fail the job if
the major version is not 26.

A simulator-only *check* job may pin an older Xcode deliberately (it is cheaper and warms faster),
but then it is verifying a different compiler than the one that ships. Prefer one pinned Xcode 26
everywhere unless there is a written reason not to.

## 3. Simulator runtimes and devices

| Fact | Measured value |
|---|---|
| iOS runtimes installed | **18.5, 18.6, 26.0, 26.1, 26.2** |
| iPhone devices available | iPhone 16 / 16 Plus / 16 Pro / 16 Pro Max / 16e, iPhone 17 / 17 Pro / 17 Pro Max |

There is **no iOS 17 runtime** on this image. A deployment target of iOS 17.0 is still fine — that
is a *minimum*, and the app is compiled against the iOS 26 SDK — but a `-destination` string that
names `OS=17.x` will not resolve.

⚠️ `xcodebuild` treats an unresolvable `-destination` inconsistently depending on the flags in
play, and a run that quietly picks a different device is indistinguishable from a green build in
the log. Pin the destination and **assert it**: resolve the device with `xcrun simctl list` and
fail the step if the expected runtime/device pair is absent, rather than letting `xcodebuild`
choose.

## 4. Tooling

| Fact | Measured value |
|---|---|
| `brew install xcodegen` | works; **XcodeGen 2.45.4**; 4.5 s wall clock (bottle, no compile) |
| SwiftPM package → iOS Simulator | `xcodebuild -scheme PulseBeacon -destination 'generic/platform=iOS Simulator' CODE_SIGNING_ALLOWED=NO build` → `** BUILD SUCCEEDED **` against the existing `sdk/beacon-swift` package |

The last row matters: it proves the *whole shape* of the plan (a Foundation-only SwiftPM package
in this repo, compiled for an iOS Simulator destination on a GitHub-hosted runner, with signing
off) works end to end — before a line of the app existed.

## 5. What this probe did NOT establish

Written down deliberately; a guard's uncovered band is where the next defect lands.

- **Nothing about code signing.** Every measurement above ran with `CODE_SIGNING_ALLOWED=NO`.
  Archiving, provisioning profiles, and the App Store Connect API key path are unverified and
  stay unverified until the operator's Apple Developer account exists.
- **Nothing about upload.** `xcrun altool` / Transporter behaviour, and whether App Store Connect
  accepts our `PrivacyInfo.xcprivacy`, are unverified.
- **Nothing about test execution on a booted simulator** — the probe built, it did not `test`.
- **Runner-image drift.** These values are a snapshot of image `20260715.0234`.
