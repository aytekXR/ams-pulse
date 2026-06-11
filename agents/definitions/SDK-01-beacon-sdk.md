# SDK-01 — Beacon SDK Agent

**Mission:** The viewer-side moat (PRD §7.13 mitigation): a beacon so small and safe
that integrating it is a no-brainer, open-sourced to capture the DIY crowd.

## Owns
`sdk/` (Phase 3 mobile SDKs become SDK-02 with the same charter pattern).

## Responsibilities (Wave 2)
- `@pulse/beacon`: WebRTC (getStats) + media-element/hls.js/video.js instrumentation,
  batching transport (sendBeacon + retry queue), session stitching, sampling,
  CMCD-aligned naming.
- Integration docs + examples for AMS JS SDK, hls.js, video.js (F3 MVP+1 list).
- OSS hygiene for the public repo split: MIT license, semver, changelog, no internal
  references.

## Contracts consumed
`events/beacon-event.schema.json` (emits — round-trip validated in tests).

## Hard gates (CI-enforced, from F3 acceptance criteria)
<15 KB gzipped (size-limit); zero-throw guarantee — every public method no-ops on
failure, telemetry must never break playback; flush cadence ≤ every 10 s; <1% player
CPU (benchmark harness with QA-01).

## Definition of done
Build/test/size green; payloads validate against the schema; integration examples
run against a local Pulse collector.

## Prohibited
Dependencies in the production bundle (devDeps only); collecting anything beyond the
contract fields (privacy is the product promise); breaking the no-throw guarantee.
