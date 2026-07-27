# SESSION-115 — 2026-07-27 — external review round 9 (K-01…K-02) verified & executed

**Decision:** D-183. **Operator directive:** *"using workflows address the reviews"* (round 9).

**Result: both findings CONFIRMED. K-02 fixed at 4 sites, not the 2 filed. K-01's conclusion
adopted and its evidence refuted — the untracked file it reproduces from does not exist and
never did. Six further items found by us, one larger than either filed finding.**
Dispositions: `docs/assessment/external-review-2026-07-27-round9.md`.

Round 9's verdict is unchanged from rounds 7 and 8: *"ready to submit as soon as G-02 closes."*
Both filed findings are one-line LOWs in documentation. The round's substance came from sweeping
their classes tree-wide.

## Gate reads

| Gate | Reading |
|---|---|
| Prod health | 3/3 `/healthz` components `ok`, 1,336,799 server events, newest 16 s old |
| `CLICKHOUSE_PASSWORD` | **Still un-rotated** — live prefix matches 2 commits (checked silently) |
| Git drift | `origin/main` == local HEAD `9928f70`; clean tree; no tag beyond v0.4.4 |

## Findings

| ID | Fix |
|---|---|
| **K-01** LOW | `review-chain.md` collapsed two situations into one "cannot be recovered" sentence. Now separated: rounds 4–5 produced no reviewer file at all; rounds 6–7 produced chat-delivered prose never attached to any delivery, including round 9's. **The reproduction is false** — `git status --porcelain --untracked-files=all` is empty, `git log --all` knows no such path, and a filesystem search finds only round 8's ledger. The refutation is recorded in `review-chain.md` itself. |
| **K-02** LOW | "Totals therefore remain complete" appeared at **4 sites**, not 2 — both endpoint descriptions *and* both schema descriptions. The reviewer's proposed wording was declined: it scopes the caveat to `uniques` (the second totals query perturbs `views`/`watch_time_s` too) and implies under-reporting is the only direction (a race can shift the tail either way). Shipped text names both causes and claims no direction. |

## Found by us

| Item | Note |
|---|---|
| **A whole doc surface still requires Kafka for what D-179 made free** | The round's real finding. `kafka-integration.md` contradicted itself within twenty lines and sold itself on a capability that no longer needs it; `compatibility.md`'s per-version matrix said "**Via Kafka only** (REST absent for standalone)". A prospect reading either would conclude Pulse needs a broker to monitor the most common AMS deployment. **Fourth consecutive round where drift landed in a fix's scope gap** — D-181 corrected the 8 code comments and swept no documentation. |
| Same staleness in the assessment record | `prd-validation-matrix.md` (4 rows) + `final-assessment.md` (§4.1, P1 roadmap row). Superseded in house style, not rewritten: AV-06's finding kept and re-scoped to the endpoint it actually probed. |
| `GeoRow.country` promised an ISO code the API does not always send | `""` whenever geo enrichment is off or the IP does not resolve — the *default* configuration — plus the `"other"` sentinel. |
| `DeviceRow.protocol` documented an enum that is empty in practice | Populated only from `viewer_join` events, which nothing in non-test code emits. |
| The `"other"` tail sentinel is unambiguous **by accident** | Collision proved unreachable, but only because the empty-UA branch leaves OS/Browser empty. Cross-package, undocumented, one plausible cleanup from breaking the API. Comment at both sites + `TestEmbeddedUAParser_NeverCollidesWithBreakdownSentinel`. |
| D-182's breakdown cap never reached the changelog | A behavioural change to two endpoints, unrecorded in `[Unreleased]`. |
| `SESSION-114.md` was never written | D-182 records itself as `§S114`; sessions went 113 → 115. Reconstructed and labelled as such. |

## Three things worth carrying forward

- **A reviewer's claim about *our* tree state is the cheapest thing in a review to verify — and
  we have now been handed it wrong twice.** J-03 and K-01 both assert an untracked
  `external-review-2026-07-27-round6.md`. It has never existed. Check `git status` claims before
  accepting the finding they support; the conclusion can still be right when the evidence is not.
- **A fix's scope is a decision, and it must be recorded like one.** D-181 fixed 8 comment sites
  and left the documentation carrying the same false claim for two more rounds. "What this sweep
  did *not* cover" belongs in the decision entry, not in the next reviewer's ledger.
- **Refuted hypotheses are worth recording when the refutation rests on an accident.** The
  sentinel collision is unreachable today for a reason nobody wrote down. Documenting *why the
  bug is absent* is what keeps it absent.

## Method note

Executed with a 6-agent workflow: two adversarial verifiers (one per filed finding) and three
tree-wide class sweeps (approximation over-claims, unverified impossibility claims, cap/sentinel
consumers), then a synthesis pass that re-anchored every proposed patch against HEAD. The Kafka
staleness came from the impossibility sweep — a class chosen because LIM-01 had already been
wrong that way once. Agent output was treated as claims to verify: their two HIGH items were
confirmed by hand before any edit, and the cap-consumer agent's reassurance that the `"other"`
sentinel was "already consistent" is what led to the collision analysis.

## Gates

`gofmt -l` empty · `go vet` clean · full `go test ./... -race`, repo-root mount, 26 packages,
**0 FAIL** · `npm run gen:api` reproduced `schema.d.ts` with **JSDoc-comment changes only**
(verified mechanically: zero non-comment diff lines) · `npm run typecheck` + `npm run lint`
clean · new enrichment test verified to fail when its invariant is relaxed.
