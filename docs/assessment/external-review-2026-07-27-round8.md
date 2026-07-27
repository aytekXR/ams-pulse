# External review — Round 8 (2026-07-27)

**Reviewed tree:** `1380b5e` (`main`, `v0.4.4-3-g1380b5e`) — the D-181 round-7-fixes commit · **published Latest:** still tag `v0.4.4` = commit `34a25fc` (verified via `/releases/latest` and GHCR; **no new tag exists** — "new release" in this round's trigger is the D-181 drop on `main`, not a new cut)
**main at review time:** same `1380b5e`
**Reviewer:** Claude (Cowork session; same reviewer as rounds 6–7) · **Phases completed:** blackbox / docs / code
**Finding prefix:** `J-` (per EXTERNAL-REVIEW-PROMPT §0 for round 8)
**Prior-round input:** `docs/assessment/marketplace-compliance-review-2026-07-27-round7.md`
**Verdict:** Ready to submit as soon as G-02 (the `CLICKHOUSE_PASSWORD` rotation) closes — all six round-7 dispositions verified in the tree (two fixed wider than filed), the published surface is coherent, and this round's three new findings (1 MEDIUM, 2 LOW) are non-blocking.

---

## Maintainer disposition (D-182, §S114)

**Our verdict: all three findings CONFIRMED as defects — but J-02's stated mechanism is
REFUTED, and it downgrades to LOW by the reviewer's own written rule. Two of the three were
wider than filed. The reviewer's reassurance about `GeoBreakdown` was wrong, and the safe
sibling turned out to be the bigger instance.**

| ID | Sev filed | Sev after verification | Verified? | Disposition |
|---|---|---|---|---|
| **J-02** | MEDIUM | **LOW** (filed instance) / **MEDIUM** (class) | **CONFIRMED as a class; mechanism REFUTED** | **FIXED, and wider than filed.** The premise that `client_device`/`client_os`/`client_browser` are viewer-controlled is false: `collector/enrichment.go` maps *any* User-Agent through literal switch statements into a closed set — `detectDevice` 4 values, `detectOS` 8, `detectBrowser` 14. Ten thousand distinct UAs yield the same 448 combos. `protocol` is emptier still: its only writer is `stitcher.go handleJoin` reading a `viewer_join` ServerEvent, and **nothing in non-test code emits one**, so the column is `""` in production. Max cardinality today is 4×8×14×1 = **448 rows**, not Θ(N) — so the reviewer's own downgrade condition fires and this is LOW as filed. What survives is the part they were right about: the response was uncapped. Both breakdowns now cap at `breakdownRowCap = 100` with an aggregated `other` tail row that preserves totals. |
| **J-01** | LOW | LOW | **CONFIRMED** | **FIXED structurally, and the sweep found two more instances the reviewer missed.** The README sentence now states the invariant (the OCI 1.1 referrer layout is a property of `release.yml`, not a per-release decision) plus a *closed*, dated spot-verification naming the exact clients (v2.4.3, v3.0.2) and the exact releases tested (`0.3.0`, `0.4.3`, `0.4.4`). It cannot go stale in either direction. See "Found by us" for the two additional instances. |
| **J-03** | LOW | LOW | **CONFIRMED — and wider than filed** | **FIXED, with the gap disclosed rather than papered over.** The reviewer filed rounds 6–7; a tree-scoped grep for the reviewer schema markers shows the gap actually runs **rounds 4 through 7** (0 markers in all four files). Rounds 1–3 do preserve substantial per-issue narrative in a pre-standardization format; round 8 is the first round whose reviewer ledger lands in-tree as written. Root cause is a self-contradiction in the prompt itself — §0 named the maintainer file as the prior-round input while §5 told the reviewer to write `external-review-…`. Both sections now agree and state which party owns which file. New `docs/assessment/review-chain.md` indexes all eight rounds and states the rounds 4–7 gap explicitly. **Not backfilled:** the round 4–7 reviewer source text is not available to this repo, and reconstructing it would mean inventing reviewer prose. |
| **G-02 / F-01** | — | — | **STILL OPEN** | **Operator-gated, unchanged.** Re-checked silently this session: the live `CLICKHOUSE_PASSWORD` 32-hex prefix still matches 2 commits in public history. Nothing in D-182 touches it — only the operator can. Still the sole submission blocker. |

### Found by us (not filed by the reviewer)

| Item | Sev | Disposition |
|---|---|---|
| **`GeoBreakdown` is the larger instance of J-02's class** | MEDIUM | **FIXED.** The review reassured us this sibling was safe — "domain-bounded (~250 countries), so it is fine". It is not: `query.go` switches `groupBy` to `geo_country, geo_region` when `p.Region` is set, and the API exposes that via `?region=true`. Country × region is thousands of rows, equally uncapped. Same cap + tail row applied. A refuted reassurance is worth more than a confirmed finding — it is the one place a reviewer tells you *not* to look. |
| **Tail-row subtraction could emit a negative count** | MEDIUM | **FIXED.** Caught while gating our own fix. The first implementation computed `tail = total − Σ(capped)` and asserted in-comment that this was exact for all three metrics. It is not: `uniq(session_id)` is an **approximate** aggregate (ClickHouse adaptive HLL) whose total and per-group estimates are computed independently, so the sum can exceed the total even over provably disjoint sets; and the totals query is a second round trip against a live table, so neither reads a snapshot of the other. Reproduced numerically — `views=-100 uniques=-20 watch_s=-1000` — then clamped at zero via `clampTailAggregate`, with the comment corrected to state why disjointness is necessary but not sufficient. Pinned by `TestBreakdownTailRow_NeverNegative`. |
| **`submission-package.md` carried a stale closed range** | LOW | **FIXED.** Round 7's I-01 class (under-claim) survived in the marketplace submission package itself: "cosign v2.4.3 fails on `0.3.0`…`0.4.3`" went stale the moment v0.4.4 shipped. Round 7 fixed the README and never swept the highest-stakes document. De-literalized to match the README's new structural form. |
| **`install.md` contradicted its own runnable command** | LOW | **FIXED.** Round 7's I-02 class: "verified 2026-07-27 against chart `0.3.1` / appVersion `0.4.3`" sat directly above `helm install … --version 0.3.2`. The I-02 fix landed in `deploy/helm/pulse/README.md` only. Now points at `Chart.yaml` and the pin below it, so it cannot drift a third time. |
| **Beacon ingest limits were undocumented** | LOW | **FIXED.** `docs/beacon-sdk.md` described `metadata` as an unconstrained string map while ingest enforces four real limits — 64 KB body (413), 100 events/batch (422), 64-byte tenant and 64-byte data-string values (both silently truncated). All four verified against `beacon.go` and now documented with the reject-vs-truncate rationale. |

### Reviewer premises we refuted

- **"Values originate from viewer-side input"** (J-02) — refuted; server-side enum mapping, evidence above.
- **"`GeoBreakdown` … is domain-bounded (~250 countries), so it is fine"** — refuted; ignores the `region=true` parameter. It was the bigger instance.
- **Round 7 I-03/I-05 undercounts are calibration data, and this round repeats the pattern in a third place**: the reviewer's errata correctly note their sweeps were file-scoped. J-03 as filed (rounds 6–7) undercounted the same way — the real span is rounds 4–7.

### Verification performed

Full Go suite under `-race` with the **repo-root mount**, 26 packages, **0 FAIL, 0 unexpected SKIP**
(`internal/api` ran 153 s, confirming the ~90 api tests were not silently skipped). `gofmt -l`
empty, `go vet` clean, `npm run typecheck` and `npm run lint` clean. Contract edit is
description-only: `npm run gen:api` reproduced `web/src/lib/api/schema.d.ts` with **JSDoc comment
changes only** — no type, property, or enum shape moved. Prod gate at session start: all three
`/healthz` components `ok`, 1,334,294 server events with the newest 9 s old.

---

## Environment and limits

- **The device bridge disconnected mid-round.** Initial recon (git log/tags/VERSION, confirming `main`=`1380b5e`=`origin/main`, no tag beyond v0.4.4) ran on the operator's tree before the disconnect; everything after ran against the **public mirror** via WebFetch (github.com pages, raw files at `main` and `v0.4.4`, the D-181 commit page). Source reads this round are therefore extraction-mediated rather than grep-exact; every load-bearing quote was pulled with pointed verbatim prompts. The blind black-box pass again ran in an isolated agent with no source access.
- **Two stale-cache serves were caught and refuted, not reported:** an un-busted GHCR versions fetch aged ~2 h mimicked a post-release image re-push, and an un-busted raw README@main served the pre-D-181 copy. Both were falsified by cache-busted re-fetches (direct-URL-wins rule). Recorded here because either would have been a false MEDIUM.
- **Could NOT be tested:** everything from round 7's list (no docker/cosign/helm execution, no live AMS, SHA256SUMS bytes, OCI chart internals) plus, this round, anything requiring exact grep across the tree (bridge down) — noted per-finding where it matters. The evaluator/query deep-dive deferred in rounds 6–7 was **partially executed** this round (see correct-list and J-02); channels/webhook/OIDC internals remain un-re-audited since their dedicated prior rounds.

---

## Findings

| ID | Sev | Phase | Subject | Claim (falsifiable) | Evidence | Proposed fix | Disposition |
|---|---|---|---|---|---|---|---|
| J-02 | MEDIUM | code | `/analytics/devices` breakdown is uncapped over viewer-influenced group keys | `query.go → DeviceBreakdown` runs `GROUP BY client_device, client_os, client_browser, protocol ORDER BY views DESC` with **no LIMIT**; the values originate from viewer-side input (the beacon SDK documents no device/os/browser fields, so they are derived server-side from client-controlled signals such as User-Agent), making distinct-combo count — and therefore response size and aggregation cost — unbounded under hostile or merely diverse traffic | query.go → DeviceBreakdown (verbatim SELECT captured, no LIMIT); docs/beacon-sdk.md → no device/os/browser payload fields, `metadata` is `Record<string,string>` with no documented size limits | Add a top-N cap (e.g. `LIMIT 100`) with an aggregated "other" bucket, mirroring the 500-row clamp LiveStreams/FleetNodes already have; document the cap in the OpenAPI response description. **Assumption stated:** if ingest normalizes these columns to a closed enum, cardinality is bounded and this downgrades to a LOW (response still uncapped); what would raise confidence: reading the sessions collector's UA/device mapping (bridge was down) or a 10k-distinct-UA replay against a running instance | **CONFIRMED as a class; mechanism REFUTED → LOW as filed. FIXED wider than filed** — cap + `other` tail row on both `DeviceBreakdown` and `GeoBreakdown`. See disposition above. |
| J-01 | LOW | docs | README's cosign-history claim is now open-ended in the wrong direction | Post-D-181, main README reads "Verified 2026-07-27: cosign v2.4.3 fails on `0.3.0` **through the current release**…" — a dated verification claim over an unbounded range: the moment v0.4.5 ships, the unchanged sentence asserts a v2.4.3 test result for a build that was never tested (the inverse of the round-7 staleness class: over-claim instead of under-claim) | raw README.md@main (cache-busted) → cosign-v3 note; contrast with the tag's closed form "fails on `0.3.0` through `0.4.3`" | Make the claim structural, not enumerative: "every release from `0.3.0` on uses the OCI 1.1 referrer layout, which cosign v2 cannot read (spot-verified 2026-07-27 on 0.4.3/0.4.4)" — true forever without re-verification | **CONFIRMED. FIXED structurally**, plus two more instances of the same class found tree-wide (`submission-package.md`, `install.md`). See disposition above. |
| J-03 | LOW | process | The round-7 ledger never landed in `docs/assessment/` | The output contract stores each round's reviewer file at `docs/assessment/external-review-<date>-round<N>.md`; round 7's file exists only outside the repo (the bridge dropped mid-commit), so the ledger chain in-tree jumps from the round-6 file to the maintainer's round-7 disposition file, which cites a document the repo does not contain | `docs/assessment/` listing at `1380b5e` → `external-review-2026-07-27-round6.md` (untracked on the operator's machine, also absent from the public tree — note: round 6's file is committed nowhere either; only the maintainer-side `marketplace-compliance-review-*round6/7.md` files are) | Commit `external-review-2026-07-27-round6.md`, `…round7.md`, and this file to `docs/assessment/` (all three are re-attached in the delivery chat); optionally have the disposition files link them once present | **CONFIRMED — wider than filed (rounds 4–7, not 6–7). FIXED**: prompt self-contradiction resolved, `review-chain.md` added, gap disclosed not backfilled. See disposition above. |

---

## Finding detail

### J-02 — Device breakdown uncapped over viewer-influenced group keys  [MEDIUM · code]
**Claim.** `DeviceBreakdown` (behind `GET /api/v1/analytics/devices`, Data-API-gated Pro+) executes `SELECT client_device, client_os, client_browser, protocol, count(), uniq(session_id), sum(watch_time_s) FROM viewer_sessions FINAL … GROUP BY client_device, client_os, client_browser, protocol ORDER BY views DESC` with no LIMIT and no "other" bucketing. The grouped columns are not in the beacon SDK's documented payload (only `player.kind`, a 5-value enum, and free-form `metadata` are), so they are derived server-side from client-side signals — i.e., an attacker with a valid ingest token, or simply a large diverse audience, controls distinct-combo cardinality. Response rows and ClickHouse aggregation state grow linearly with it.
**Reproduction (proposed — not executable here).** Replay N beacon batches with N distinct User-Agent strings against a Pro instance; `GET /api/v1/analytics/devices` returns Θ(N) rows. Compare `GET /api/v1/live/streams`, which clamps to 500.
**Why it matters.** This is the brief's phase-3 item 5 verbatim ("cardinality traps"): the dashboard page rendering this response and any API consumer paginating nothing will degrade first; ClickHouse absorbs the rest. The sibling `GeoBreakdown` is shape-identical but domain-bounded (~250 countries), so it is fine — which is exactly why the unbounded sibling is easy to miss.
**Proposed fix.** `LIMIT 100` plus roll-up of the tail into an `other` row (keeps totals honest), matching the existing 500-cap pattern in LiveStreams/FleetNodes; note the cap in the OpenAPI description.
**Confidence.** Medium. The SQL and the beacon field table are verbatim; the "viewer-influenced, not enum-normalized" premise is an assumption (the sessions collector's mapping was unreadable with the bridge down). If ingest clamps these columns to a closed enum, downgrade to LOW — the response is still uncapped, but bounded.

### J-01 — Open-ended cosign verification claim  [LOW · docs]
**Claim.** The D-181 fix de-literalized the README's cosign-history sentence to "…fails on `0.3.0` through **the current release**". As a *verification* claim ("Verified 2026-07-27") it now asserts results for releases that do not exist yet: at the next cut the sentence silently claims v2.4.3 was tested against it. Round 7's I-01 was an under-claim going stale; this is the same sentence rebuilt to over-claim by default.
**Reproduction.** Read the sentence at `main` today; re-read it the day v0.4.5 ships — no edit will have occurred, the claim's scope will have grown.
**Why it matters.** This project's credibility with a marketplace security reviewer rests on dated, falsifiable verification statements (the house style everywhere else). One structural rewrite ends the maintenance treadmill for this sentence permanently.
**Proposed fix.** State the invariant, not the enumeration: "Releases from `0.3.0` on publish the signature as an OCI 1.1 referrer, which cosign v2 cannot read (`Error: no signatures found`); spot-verified 2026-07-27 with v2.4.3/v3.0.2 against 0.4.3 and 0.4.4."
**Confidence.** High (sentence quoted from cache-busted raw fetch).

### J-03 — Ledger chain incomplete in-tree  [LOW · process]
**Claim.** Neither `external-review-2026-07-27-round6.md` nor `…round7.md` is committed: round 6's sits untracked on the operator's machine, round 7's exists only in the session chat (its `device_commit_files` write failed when the bridge dropped), while the committed round-6/round-7 disposition files both cite them as their input. An auditor reconstructing the review chain from the repo alone finds dispositions referencing absent documents.
**Reproduction.** `ls docs/assessment/` at `1380b5e`; `git status` on the operator's tree (pre-disconnect: round-6 file untracked).
**Why it matters.** The pack sells this loop as evidence discipline ("prompt in, ledger out"); the evidence should be checked in. Cosmetic today, awkward mid-qualification-review.
**Proposed fix.** Commit both prior ledgers plus this one (all re-attached in the delivery chat).
**Confidence.** High.

---

## Prior-round re-audit (round 7: I-01…I-06, plus the standing gate)

Verified against the tree at `1380b5e` (raw fetches, cache-busted) and the published artifacts — not against the changelog. The maintainer's dispositions claimed all six CONFIRMED-FIXED, two "larger than filed"; both verified.

| Prior ID | Reported as | Verified state now | Note |
|---|---|---|---|
| I-01 (README self-contradicts on current release) | FIXED + new guard #19 | **VERIFIED at main; frozen at tag as designed** | main: "(current: **v0.4.4**)", footer "(D-181) … latest release **v0.4.4**", pins 0.4.4. Guard #19 verified verbatim in release.yml: exact-string match on both prose patterns for the tagged version **plus** a reverse check that no other version appears in those patterns — the class is guarded, not just the instance. Tag v0.4.4 still carries the stale strings (confirmed at the tag, not assumed) — reaches the release surface only via a future cut, per round 7's own recommendation; not re-flagged. The de-literalized cosign sentence introduced J-01 |
| I-02 (helm README stale chart-version example) | FIXED by de-literalizing | **VERIFIED at main; frozen at tag** | New sentence points at Chart.yaml and cites "round-7 review I-02" in place; literal "0.3.1 carries appVersion 0.4.3" absent at main (explicit string search), still present at the tag; runnable `--version 0.3.2` pin retained and matches the shipped `pulse-0.3.2.tgz` |
| I-03 (doc stamps lag) | FIXED "plus two more" (ARCHITECTURE.md, licensing.md) | **VERIFIED for the filed instance** | compatibility.md@main now stamps "D-179 (2026-07-27) — AMS 2.16/2.17 coverage added and source citations de-numbered". The two additional stamp fixes (ARCHITECTURE.md, licensing.md) are visible in the D-181 file list (+2−1, licensing.md touched) but their header lines were not individually re-fetched — accepted on the commit evidence, marked as such |
| I-04 (fallback RTT window spans 3 calls) | FIXED, per-call, "reproduced numerically", 2 regression tests | **VERIFIED** | restpoller.go@main now times each call separately (`t1`/`resRTTMS`, `t2`/`statsRTTMS`), `sysRTTMS` carries the RTT of the call that produced the event; the in-place comment explains the old one-window defect and cites I-04; D-087 comment restated to the per-call rule. `system_resources_test.go` appears in the D-181 file list (regression tests claimed there; not individually read) |
| I-05 (stale "AMS 3.x never reports cpu_pct" comments) | FIXED at 8 sites (not the 3 cited) | **VERIFIED for the primary site** | wave3.go's presence-guard comment now states the durable invariant ("skip on ABSENCE, never on an assumption about which AMS versions report what — review round 7, I-05") and preserves the history ("used to read… stopped being true in D-179"). The additional sites (anomaly.go, meta/anomaly.go, two test files) are in the D-181 file list; spot-verified only via wave3.go |
| I-06 (LIM-10 scoped to 3.x) | FIXED | **VERIFIED — title and body** | LIM-10 now titles "…not exposed by AMS 2.14–3.x…" and the body states "This holds across the whole supported range, not only 3.x: `ClusterNode.java` is field-identical at tags `ams-v2.14.0`, `ams-v2.16.2`, `ams-v2.17.1` and `ams-v3.0.3`" — exactly the round-6 evidence, now in the disclosure. LIM count steady at 28 |
| G-02 / F-01 (rotate `CLICKHOUSE_PASSWORD`) | STILL OPEN, operator-gated | **STILL OPEN** | Reconfirmed as the sole blocker in the round-7 disposition file; nothing in D-181 touches it (correctly — only the operator can) |

---

## What I checked and found correct

- **The published surface did not move and did not need to**: Latest = v0.4.4 (27 Jul 09:19), 7 assets unchanged incl. SHA256SUMS at 343 B; GHCR newest row `latest/0.4.4/candidate-34a25fc4/0.4/0` on digest `sha256:81673359…` with age consistent with the release publish — **no post-release tag mutation** (an apparent re-push and an apparently still-stale README were both proven to be this sandbox's cache artifacts, not product facts). Listing regression checks pass at main: HLS/LIM-02 caveat, narrowed 2.10/2.14/2.17 compatibility line, `--version 0.3.2` Helm pin.
- **Deferred-area pass, evaluator (`alert/evaluator.go`, ~1,400 lines, extraction-mediated):** state eviction explicitly never evicts a `firing` entry ("dropping it would silently discard an unresolved alert") — the disciplined half of the LIM-26 trade-off, in code; firing→resolved edges are time-window based with immediate pending-reset and cooldown suppression; wildcard-offline pages once per present→gone transition via cross-tick diffing; delivery failure exhausts 1+3 attempts with exponential backoff and jitter, then writes a `delivery_failure` alert-history row — no silent drops. No TODO/FIXME in the file. Nothing here contradicts the disclosed behavior set.
- **Deferred-area pass, query layer (`query/query.go`, ~1,200 lines):** LiveStreams/FleetNodes clamp page sizes (≤500, default 50); the two "LATENT BUG" comments both sit in `AnomalyBaselineForMetric`, which is **verified dead code** — "no non-test callers" (grep receipt in the comment), pinned deliberately with a fix-when-wired plan and a decisions.md ref (D-121), while the live anomaly detector uses meta-store Welford baselines. I went looking for a live defect and found the tree already honest about a dormant one — recorded as refutation-grade evidence, not a finding. The one genuine gap found became J-02; GeoBreakdown shares its shape but is domain-bounded (~250 countries).
- **The round-7 → D-181 loop held the standard**: all six fixes landed wider than filed where reality was wider (3 stamps not 1; 8 comment sites not 3), the I-01 fix came with a class-guard (#19) whose reverse check catches the *next* release's staleness too, and the I-04 fix carries its own rationale in place. No disposition claim failed verification.

---

## Errata — my own prior-round errors

- **Round 7 undercounted twice, both in the safe direction, both now on record:** I-03 named one stale stamp (there were three — ARCHITECTURE.md and licensing.md escaped my grep because I only checked the file I had diffed), and I-05 named three stale-comment sites (there were eight across the anomaly and meta-store packages). Neither changes a verdict; both are calibration data — my sweeps were file-scoped where the maintainer's were tree-scoped. This round's J-01/J-02 proposed fixes are phrased to survive that failure mode (structural claims, pattern-level caps).
- No round-7 claim was refuted by this round's evidence; the round-6 errata (H-09 fix retraction, H-08 severity) stand as recorded.
