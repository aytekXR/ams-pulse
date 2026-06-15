# V3b DOC-01 Report — Documentation Reconciliation

**Agent:** DOC-01 (documentation)
**Date:** 2026-06-15
**Commit:** see below
**VDs addressed:** VD-28, VD-29, VD-30, VD-01, VD-35, VD-33, VD-36, VD-S1, VD-S2, VD-S3, VD-02, VD-39, VD-12, VD-13, VD-03, VD-22, VD-40, VD-17, VD-23 (open), VD-X3-A (open)

---

## Summary

Reconciled `docs/ARCHITECTURE.md`, `README.md`, `docs/runbooks/alerting.md`, and
`docs/runbooks/reports.md` to match the verified V3a+V3b build. No documented-but-unimplemented
behavior remains. All Phase-3 / deferred items are labeled explicitly.

---

## Files changed

| File | Changes |
|------|---------|
| `docs/ARCHITECTURE.md` | Clarified V3b remaining-limitations table: rewritten as OPEN/CLOSED two-state table, added VD-23 (IngestTracker OPEN), added VD-X3-A (reachable field OPEN), corrected VD-21 description (keys present, live-data D-002 waiver), corrected VD-23 OPEN status in V3b fix-loop summary row |
| `docs/runbooks/alerting.md` | Fixed PagerDuty/Webhook tier to "Business tier and above" (was "Enterprise only"); added `group_by` field to Rule fields table; replaced vague "Phase-3 roadmap: group_by" storm-protection note with actual implementation description + example; updated `node_down` description to match V3b implementation (absence detection, not CPU proxy); corrected Phase-3 roadmap note for rebuffer_ratio to accurately describe the live heuristic proxy; updated Known issues to remove "group_by not implemented" (now implemented); last-updated header updated |
| `docs/runbooks/reports.md` | Fixed tier header from "Enterprise required" to "Business required"; fixed Overview to say "Business+ tier"; expanded cron section to document 5-field cron (VD-36 fix); updated creating-a-schedule API examples to include 5-field cron; added Business+ requirement note to API call; updated Known limitations to close the "Edge-origin viewer dedup" row (VD-03 fixed V3a); added peak_concurrency note; fixed white-label section to distinguish Business vs Enterprise access |

---

## Accuracy verification

All documented behaviors cross-checked against:
- `V3b-QA-gate-report.md` (the authoritative truth for what passed)
- `V3a-QA-report.md` (V3a mini-gate)
- `V3a-BE01-report.md`, `V3b-BE02B-report.md`, `V3b-BE02C-report.md`, `V3b-FE-report.md`
- `V3a-SDK-report.md`
- `V3a-INT-report.md`
- `server/internal/license/license.go` (entitlement matrix)

### Key accuracy corrections made

1. **PagerDuty/Webhook tier**: License.go and V3b INT-01 clearly show `businessTierEntitlements.Channels` includes `pagerduty` and `webhook`. Previous runbook said "Enterprise only" — corrected to "Business and above".

2. **group_by in alerting.md**: "Phase-3 roadmap" note removed; VD-29 (group_by implemented) confirmed PASS in V3b QA gate. Rule fields table now documents the `group_by` field.

3. **node_down behavior**: Updated to reflect VD-30 fix — fires on node absence from snapshot, not CPU>95 proxy. `EvictStaleNodes` wired.

4. **Reports tier**: VD-35 confirmed PASS in V3b QA gate — all 5 report handlers gated to Business+. `CheckReports()` enforced. Previous runbook said "Enterprise tier required" — corrected.

5. **Cron format (reports)**: VD-36 confirmed PASS — 5-field cron accepted by server parser. Updated runbook to document both 5-field and 3-field formats with a comparison table.

6. **VD-23 (IngestTracker)**: V3b QA gate lists as OPEN. Corrected ARCHITECTURE.md V3b fix-loop summary row (was claiming fixed) and added to remaining-limitations table.

7. **VD-21 clarification**: Both `timeseries` and `drop_events` keys are present in the response (V3a QA confirmed). Live-data population requires real ClickHouse (D-002 waiver). Doc now accurately describes this partial-pass status.

8. **rebuffer_ratio Phase-3 note**: Updated from "planned for Wave 3" (Wave 3 is now complete and did not include this) to "Phase-3 roadmap item" with accurate description of the current heuristic formula.

---

## Build verification

```
cd server && timeout 150 bash -c 'CGO_ENABLED=0 go build -o /tmp/pulse ./cmd/pulse/'
→ EXIT 0 (clean)
```

No code changes made — doc-only edits.

---

## Still-open defects (for ORCH-00)

| VD | Description | Owner | Notes |
|----|-------------|-------|-------|
| VD-23 | IngestTracker interface type mismatch; SetIngestTracker never called | BE-02 | `handleIngestHealth` reads from live snapshot; dead interface |
| VD-X3-A | POST /admin/sources/{id}/test missing `reachable` field | BE-02 | Spec is already correct; handler needs fix |
| VD-04 | Dashboard render time at 500 streams not measured | QA-01/FE-01 | Phase-3 Playwright benchmark |
| VD-14 | Player CPU <1% budget — no measurement | SDK-01 | Phase-3 |
| VD-24 | No qoe/ingest integration tests with seeded ClickHouse data | BE-02 | D-002 waiver |
| VD-26 | No frontend tests for IngestPage | FE-01 | Phase-3 |
| VD-27 | Kafka Lag/ParseErrors not in /healthz | BE-02 | Phase-3 (GAP-2-003) |
| VD-31 | Detection budget measured with fake clock only | QA-01 | Formal bound documented |
| VD-38 | peak_concurrency = session count not true concurrent peak | BE-02 | Phase-3 schema change |
