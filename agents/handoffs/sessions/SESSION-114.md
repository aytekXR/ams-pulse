# SESSION-114 — 2026-07-27 — external review round 8 (J-01…J-03) verified & executed

> **Reconstructed in S115 (D-183).** This file was never written at the time: D-182 records
> itself as `§S114` and the sessions directory jumped 113 → 115. It is rebuilt from D-182's
> decision entry and the round-8 ledger, both written in-session, and contains nothing not
> already recorded there. Flagged rather than backfilled silently — the same evidence-chain
> standard round 9's K-01 filed against `review-chain.md`, applied to our own ledger.

**Decision:** D-182. **Operator directive:** *"here are the reviews"* (round 8).

**Result: all three findings CONFIRMED as defects — but J-02's stated mechanism REFUTED, and it
downgrades to LOW by the reviewer's own written rule. Two of three were wider than filed, and
five further items were found by us.**
Dispositions: `docs/assessment/external-review-2026-07-27-round8.md` — the first round whose
reviewer ledger and maintainer dispositions live in the same in-tree file.

## Gate reads

| Gate | Reading |
|---|---|
| Prod health | 3/3 `/healthz` components `ok`, 1,334,294 server events, newest 9 s old |
| `CLICKHOUSE_PASSWORD` | **Still un-rotated** — live prefix matches 2 commits (checked silently) |
| Git drift | `origin/main` == local HEAD `1380b5e`; clean tree |

## Findings

| ID | Fix |
|---|---|
| **J-01** LOW | The README's dated cosign claim became **structural** — the OCI 1.1 referrer layout is a property of `release.yml`, not a per-release decision — plus a *closed* spot-verification naming v2.4.3/v3.0.2 against `0.3.0`/`0.4.3`/`0.4.4`. A tree sweep found **two more instances outside the filed file**: `submission-package.md` (inside the marketplace submission document itself) and `runbooks/install.md`. |
| **J-02** MED→**LOW** filed / MED class | Mechanism refuted: `enrichment.go` maps *any* UA into closed enums (4 × 8 × 14 = 448 combos; `protocol` empty in production), so cardinality is not viewer-controlled. What survived is that the response was **uncapped**: both breakdowns now cap at `breakdownRowCap = 100` with an aggregated `other` tail row preserving totals. |
| **J-03** LOW | Filed against rounds 6–7; a tree-scoped grep for the reviewer schema markers returns 0 in **rounds 4, 5, 6 and 7**. Root cause was a self-contradiction inside `EXTERNAL-REVIEW-PROMPT.md` (§0 vs §5), now resolved. New `docs/assessment/review-chain.md` indexes all eight rounds and **states the gap rather than backfilling it**. |

## Found by us

| Item | Note |
|---|---|
| `GeoBreakdown` is the **larger** instance of J-02's class | The review reassured us this sibling was safe ("~250 countries"). `?region=true` switches the GROUP BY to `geo_country, geo_region` — thousands of pairs, equally uncapped. |
| Tail-row subtraction could emit a **negative** count | Our own first cap implementation asserted in-comment that `tail = total − Σ(capped)` was exact. Reproduced numerically at `views=-100 uniques=-20 watch_s=-1000`, then clamped via `clampTailAggregate` and pinned by `TestBreakdownTailRow_NeverNegative`. |
| `submission-package.md` carried a stale closed range | Round 7's I-01 class, surviving in the highest-stakes document. |
| `install.md` contradicted its own runnable command | Round 7's I-02 class; that fix had been scoped to the Helm README only. |
| Beacon ingest limits were undocumented | Four real limits (64 KB body/413, 100 events/422, two 64-byte truncations) now in `beacon-sdk.md`. |

## Three things worth carrying forward

- **Verify the exculpations, not just the accusations.** Round 8's most valuable sentence was a
  reassurance, not a finding — and it was wrong, and it was hiding a defect strictly larger than
  the one filed. A reviewer's "this one is safe" is the one place nobody looks twice.
- **An in-code proof is a claim to test, not evidence.** Our disjointness argument was sound and
  insufficient: `uniq()` is approximate and the totals query is a second round trip. Both the
  authoring lane and its adversarial verifier certified that code SOUND. Reproduce numerically
  before believing any arithmetic argument, including our own.
- **Third consecutive round where drift landed in a known scoping gap.** Round 7's own fixed
  classes were still live in two unswept files. When a guard or a fix is scoped to one file,
  write down what the scoping leaves uncovered.

## Gates

`gofmt -l` empty · `go vet` clean · full `go test ./... -race`, repo-root mount, 26 packages,
**0 FAIL, 0 unexpected SKIP** (`internal/api` ran 153 s, proving ~90 api tests were not silently
skipped) · `npm run typecheck` + `npm run lint` clean · `npm run gen:api` reproduced
`schema.d.ts` with JSDoc-comment changes only.
