# SESSION-107 — 2026-07-26 — REVIEW-MP3 executed (N1–N9), v0.4.2 cut completed, operator-expected rewritten

**Decision:** D-174. **Review record:** `docs/assessment/marketplace-compliance-review-2026-07-26.md` (Part 2 of the reviewer's combined document; Part 1 is the 07-25 file). **Operator directive:** act on the review + "update the operator expected — only keep open items".

## Sequencing call

The review's core warning — do NOT tag v0.4.2 while N1/N2 falsify the changelog's cluster
claim — was already implicitly honored: the previous session's release waiter reported MAIN
GREEN at `28e11da` but the tag was never pushed. This session fixed N1–N3 (+ the rest),
extended the [0.4.2] changelog with the D-174 entries, and only then merged + tagged.

## Verification summary (4 parallel read-only verifiers)

| Finding | Verdict | Note |
|---|---|---|
| N1 (500→standalone + cache poison + inert test) | CONFIRMED (all 6 sub-claims) | test passed for the wrong reason — fixture lacked system-status |
| N2 (NodeID override collapses fleet) | CONFIRMED | discovery uses real IDs → N+1 phantom fleet |
| N3 (hardened overlay can't boot) | CONFIRMED | verified via live `docker compose config` merge |
| N4 (stream_ingest_error write-only) | CONFIRMED (a–d) | changelog said "captured" |
| N5 (source .env injection) | CONFIRMED except PULSE_REF sub-claim (already pinned by #216) | |
| N6 (Trivy after push) | CONFIRMED | scan failure left image pullable under release tags |
| N7 (SDK_VERSION define) | CONFIRMED, severity corrected | ReferenceError swallowed by init() catch → silent NoOp, not a crash |
| N-cluster a–h | ALL CONFIRMED | a/g/h + WS-marshal logging deferred as tracked P2 debt |
| N8 | CONFIRMED except listing version pin (fixed by #216) | |
| N9 batch | CONFIRMED except schema.d.ts (was already correct) | |

## Shipped

Full inventory in D-174. Headlines: cluster path hardened (probe owns the mode cache;
errors are errors; real node IDs; no fabricated metrics; NaN clamp; page cap + deadline;
probe backoff) with 5 new regression tests incl. a cache-survival pin; hardened overlay
boots (+ merged-config CI assert); 0011 migration persists ingest-error action/stream_name
(+ schema clause + fixtures); quickstart re-run injection fixed; Trivy quarantine-promote
release flow; `__SDK_VERSION__` runtime fallback + dist grep; licensegen→license.New
end-to-end ladder test; version guard 10→13; ~25-item docs/CI accuracy batch.

## Deferred (tracked debt, non-blocking)

- N-cluster remainder: AMS `status`/`lastUpdateTime`-based liveness (dead node stays "ok"
  while AMS keeps it listed; `IsEdgeStream` stickiness), edge-dedup inertness disclosure
  vs real 3.x (roles absent on the wire), restpoller/discovery double-polling
  consolidation, WS-broadcast marshal-error logging (`api/server.go` slow-client path).
- Helm nits needing golden regeneration (configmap comment wording, NetworkPolicy golden
  variant) — bundle with the next chart change.
- compose-boot `up -d --wait` + one-shot migrate container: compose-version-sensitive
  watch item.

## Gates

- Server: gofmt clean, vet clean, FULL `go test -race ./...` green (golang:1.25 container).
- qa/mock-ams + qa/licensegen: green. New mint-path e2e: green (all 4 tiers).
- beacon-js: build + dist-version literal grep + 15 KB size gate + lint + tests green.
- web: tsc --noEmit + full suite green (schema.d.ts regen was a 1-line JSDoc diff).
- shellcheck (install.sh), contracts (ajv fixtures incl. the 2 new ones + redocly): green.
- Merged base+hardened compose config assert: verified locally before CI.
- Release: PR merged on 13 green required checks; **v0.4.2 tag pushed on the merge
  commit**; release.yml ran the new candidate→scan→promote flow.

## Prod

Untouched (v0.4.0-139). Rolling prod to 0.4.2 remains an operator-gated deliberate
`deployment.sh` deploy.
