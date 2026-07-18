# SESSION-90 — iOS Swift beacon SDK, Phase 1 (§2.12, operator-directed D-152)

> Written by the D-152 decision batch (2026-07-18). Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`
> (**this host IS prod**; no SSH). **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-119** (unchanged — an SDK is a
> client library; SESSION-90 does NOT touch the server and does NOT roll prod).
> **The low-frequency wait is over:** the operator green-lit §2.12 mobile SDKs ("add it to the implementation plan and next
> session"). Swift 6.1.2 is on this host → the **iOS SDK is buildable/testable here**; the Android Kotlin SDK is
> **toolchain-blocked** (no JDK/Gradle/Kotlin) and is surfaced to the operator, NOT started.

## ⚡ STANDING DIRECTIVE — carried
Ultracode is on (apply to the *quality* of the SDK). **Workflow gotcha:** no backticks in workflow prompt prose.
`gofmt -l` before any Go push (N/A this session unless server touched — it should not be). Contracts before code
(CLAUDE.md §3): the beacon wire shape is **frozen** — `contracts/events/beacon-event.schema.json` (D-004). Mirror it
exactly; do NOT diverge from the JS beacon schema.

## Goal — `sdk/beacon-swift` Phase 1 (cross-platform core, buildable on Linux)
A SwiftPM library that lets a native iOS app report player-side QoE telemetry to Pulse, mirroring `sdk/beacon-js`:
- **Wire contract:** POST batched events to `<ingestUrl>/ingest/beacon` with header `X-Pulse-Ingest-Token: <token>`; body
  shape = `beacon-event.schema.json` (session_id, stream_id, events[], etc.). Re-read `sdk/beacon-js/src/{types,session,
  transport,index}.ts` + the schema before coding — the Swift types must match field-for-field.
- **Core modules (mirror beacon-js):** `Types` (Codable structs matching the schema), `Session` (session lifecycle +
  event buffer + flush policy), `Transport` (URLSession POST + batching + retry/beacon-on-background), `PulseBeacon`
  (public façade / config). Keep the public API shape analogous to beacon-js's `index.ts`.
- **Cross-platform discipline:** the core uses only `Foundation` (URLSession) so `swift build` + `swift test` run on
  **Linux** here. Any iOS-only lifecycle hook (UIKit `didEnterBackground` → final flush, `URLSession` background config)
  goes behind `#if canImport(UIKit)` and is documented as "verified on Xcode/CI only" — do NOT let it break the Linux
  build/tests.
- **Tests:** XCTest under `Tests/PulseBeaconTests/` — session buffering, batch/flush, transport request shape (assert the
  URL, the `X-Pulse-Ingest-Token` header, and the JSON body matches the schema), Codable round-trip vs the frozen schema.
  Aim for parity with the beacon-js test coverage of the same behaviors.
- **Size gate:** define a per-platform analog to the JS 15 KB gate at scoping (source-size / no-heavy-deps rule; Swewift
  has no bundler — track LOC + zero third-party deps as the discipline). Document the chosen gate in the SDK README.
  Swift builds a static library, so the "size gate" is a source-discipline analog, not a shipped-bundle byte count.

## Environment (this session)
- **Swift:** `swift --version` → 6.1.2 (Linux). Build: `swift build` ; test: `swift test` — both from `sdk/beacon-swift/`.
  No Xcode/xcodebuild here (iOS simulator tests are NOT possible — that's the documented Linux limitation).
- **Reference:** `sdk/beacon-js/src/*.ts` (the model to mirror); `contracts/events/beacon-event.schema.json` (frozen wire
  shape); the server ingest handler `server/internal/collector/ingest` + `POST /ingest/beacon` (64 KB body cap, VD-S4;
  `X-Pulse-Ingest-Token` bearer). Do NOT change the server.

## Pipeline
1. Verify-at-open: git clean (only `Caddyfile.prod`); `swift --version` present. Record **D-153 IN PROGRESS**. Branch
   `s90-d153-ios-sdk` (D-152 docs already merged separately).
2. Contracts first: read the frozen schema + beacon-js; write the Swift `Types` to match, then Session/Transport/façade.
3. Validate: `swift build` (release + debug) + `swift test` green on Linux; Codable round-trip test pins schema parity;
   the size/no-deps gate holds. (Optional: a `swift build -c release` size check.)
4. Adversarial review: transport correctness (batching, retry, background flush), schema parity, no PII beyond the JS SDK,
   token never logged. (SDK handles a token + viewer telemetry — treat as a light security surface.)
5. PR → CI. **⚠ CI GAP:** the current `sdk` CI job builds beacon-js (npm) only — there is no Swift job. Add a minimal
   `sdk-swift` CI job (`swift build && swift test`) in `.github/workflows/ci.yml` in THIS PR so the SDK is gated, OR flag
   it as a follow-up if the CI runner lacks Swift (check `swift` availability on `ubuntu-latest` — it is NOT preinstalled;
   use `swift-actions/setup-swift` or the official swift container). Decide at open; do not claim CI coverage that isn't
   wired.
6. Squash-merge --delete-branch → verify origin/main. **NO prod roll** (client library; server untouched).
7. Close docs: D-153, ROADMAP §2.12 (iOS Phase 1 done; Android still blocked), RESUME → SESSION-91, operator-expected,
   SESSION-90 CLOSED, SESSION-91 written. Re-arm the `/loop`.

## Scope discipline
- Phase 1 = the buildable cross-platform CORE + tests + a clear README. Full iOS-lifecycle integration (background
  URLSession, UIKit hooks) is Phase 2 and needs Xcode/CI to verify — scope it but don't fake-verify it on Linux.
- **Do NOT start the Android Kotlin SDK** — toolchain-blocked (operator-expected.md). If the operator provisions a
  JDK+Gradle+Kotlin environment, that becomes a later session.
- Do NOT touch the server / prod. If a schema ambiguity appears, the JS SDK + the frozen schema are the source of truth.

## Carried operator items (unchanged — none block this session)
§2.1 branch protection (operator runs the `gh api` PUT); **[20] audit-read model still open** (status-quo reads-open);
AMS trial-licence expiry confirmation; rotate chat-exposed creds; §2.7 CI-promotions auto-unlock 2026-07-23. Prod
**v0.4.0-119**; rollback tags `pulse-prod-pulse:pre-d151` etc.; do-not-commit `deploy/config/Caddyfile.prod`. Commit
trailer `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with
[Claude Code](https://claude.com/claude-code)`.
