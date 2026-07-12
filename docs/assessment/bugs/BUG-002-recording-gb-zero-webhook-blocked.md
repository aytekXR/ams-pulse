# BUG-002: `recording_gb` is always 0 — vodReady only arrives via webhooks AMS 3.0.3 cannot sign

**Severity:** high (blocks the recording/billing use case: PRD F6)
**Component:** collector (webhook path) + reports
**Status:** **FIXED (S23 / D-085, 2026-07-12)** — VoD REST poll fallback per the
design note (`BUG-002-design-note-vod-rest-poll.md`): `restpoller.pollVods`
(every 12th tick), persistent seen-set dedup on the stable AMS `vodId`
(`vod_poll_state`, meta migration 0003), `mv_recording_1d` ClickHouse MV
(migration 0009). Live-validated: TC-REC-01 3/3 vs real AMS —
recording\_gb = 0.003126 vs 3,125,555-byte ground truth (0.02% reconciliation).
The webhook path remains fail-closed (unchanged).

## Reproduction Steps

1. AMS has real VoD assets (S16 capture: WebRTCAppEE ≈1006 VoDs / ~24 GB;
   re-confirm with per-app `GET /{app}/rest/v2/vods/count`).
2. `GET /api/v1/reports/usage` on any Pulse deployment pointed at that AMS.
3. `totals.recording_gb == 0` despite AMS ground truth > 0.

## Expected (AMS Ground Truth)

AMS endpoint: per-app `GET /{app}/rest/v2/vods/count` → `{"number": N}` with N > 0
(also `vods/list` with per-file sizes).
Pulse usage reports should reflect recorded bytes.

## Actual (Pulse Output)

Pulse endpoint: `GET /api/v1/reports/usage`
Response: `totals.recording_gb: 0` — always, on every AMS 3.0.3 deployment.

## Root Cause

Chain of three facts:
1. Pulse ingests recording data ONLY from the `vodReady` webhook event
   (`webhook.go:translateWebhook()` → `EventRecordingReady` → `recording_bytes`
   in `rollup_usage_1d`). The VoD REST endpoints are never polled.
2. AMS 3.0.3 cannot HMAC-sign lifecycle hooks (no HMAC fields — O3, closed-N/A,
   decisions.md:2404).
3. Pulse's webhook listener is fail-closed (rejects unsigned deliveries), and per
   O3 the operator must NOT point `listenerHookURL` at it.

Therefore `EventRecordingReady` never fires → `recording_bytes` never written →
`recording_gb` is structurally 0, not transiently 0.

**Aggravating S17 finding:** AMS build 20260504_1443 now returns HTTP 405 on
`GET /rest/v2/applications/info` (the S16-era per-app vodCount/storage source) —
so even the read-only ground-truth shortcut drifted; per-app `vods/count` is the
stable path.

## Fix Suggestion

P0 roadmap item (matches session-plan.md § Phase 8 pre-populated roadmap):
**VoD REST poll fallback** — extend the restpoller with a low-frequency (e.g.
60 s) per-app `vods/list` incremental poll (keyed on vodId/creationDate high-water
mark) emitting the same `EventRecordingReady` events the webhook would. Keeps the
fail-closed webhook posture intact; no AMS-side config needed. Alternative (or
complement): the unsigned-webhook ingest mode with IP allowlist — blocked on the
operator's D-V2-1 decision.

## Evidence

- `qa/realams/evidence/S17-TC-WH-03-*/` (verdict + BUG-002-recording-gap.txt,
  AMS vodCount ground truth vs Pulse usage report)
- `docs/assessment/capability-map.md` § AV Triage — S17 (AV-09: CONFIRMED —
  prod `recording_gb: 0`)
- capability-map.md §8 (VoD/Recording: PARTIAL, webhook-only ingestion)
