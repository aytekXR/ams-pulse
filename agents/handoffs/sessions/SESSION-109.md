# SESSION-109 — 2026-07-27 — external review round 4 (F-01…F-14) verified & executed

**Decision:** D-176. **Review record + disposition table:**
`docs/assessment/marketplace-compliance-review-2026-07-27-round4.md`.
**Operator directive:** the round-4 review, delivered as the session's work item.

## No stale premise this time (a first)

The two prior reviews each carried a stale premise about what was published. **Round 4's
tree-state table was verified exactly correct** — commit `bea0108`, tag `v0.4.2` at `e318a05`,
the 3-commit delta, chart 0.3.1 on main, the release timestamp. The reviewer also correctly
caught that GitHub's `/releases` *index* was serving a stale "v0.4.1 · Latest" during their
window and went to the tag page instead — i.e. they actively avoided the failure mode the
brief warns about.

They were also honest about what they could **not** check (ghcr.io egress blocked, no Docker
daemon, Go 1.25 toolchain download blocked, no cluster, no DNS). Those were treated as
declared-unverified, not as passes.

## Verification summary (verify-first, before any edit)

**14 of 14 CONFIRMED. One embedded sub-claim REFUTED.**

| Finding | Verdict | Note |
|---|---|---|
| F-01 prod CH password prefix in public history | **CONFIRMED — still open** | Verified without printing the secret: live 48-char value's first 32 chars still hit under `git log -S`. Operator item #1. |
| F-02 release notes claim unshipped behaviour | CONFIRMED (both halves) | `stream_ingest_error` has zero read paths; the plain compose path publishes no ports and is not what CI boots |
| F-03 README/LIM-10 cluster claims inert | CONFIRMED | Whole chain verified in code; **propagated to 6 docs, 2 more than the review found** |
| F-04 degraded ladder unreliable on clusters | CONFIRMED (all 4 mechanisms) | Disclosed, not fixed — see below |
| F-05 `down` state invisible; unit unverified | CONFIRMED | Mock timestamp + assumption doc fixed; surfacing deferred |
| F-06 mode-flip blind window | CONFIRMED | Disclosed in LIM-10 |
| F-07 GHCR cleanup can't execute | **CONFIRMED — worse than reported** | Exits **0** at the list call, reporting success; never reaches their predicted warning branch |
| F-08 quickstart port remedy + re-run fail | CONFIRMED (both) | Fixed; parameterised compose ships by cutting v0.4.3 |
| F-09 helm alias inverted; guard #16 inert | CONFIRMED (both) | Fixed and render-verified across 4 cases |
| F-10 install.md residue batch | CONFIRMED | **Their counts were right and my first count was wrong** — see below |
| F-11 disposition overstatements | CONFIRMED | All corrected |
| F-12 prod IP: 3 of 8 not functional | CONFIRMED | nginx file's live directive is `127.0.0.1` — the IP is in a dead Caddy-era comment |
| F-13 disclosure gaps | CONFIRMED (one sub-claim refuted) | LIM-28 extended, LIM-01 corrected, Kafka marked EXPERIMENTAL |
| F-14 smaller items batch | CONFIRMED | `CPUPctOK`, poller empty-ID, guard #13, 4 doc items |

**Refuted:** F-13's claim that `originAdress` "exists nowhere in the tree". It is in the
**real AMS 3.0.3 response captures** (`server/pkg/amsclient/testdata/broadcasts_real_*.json`)
— not decoded into the DTO, which is presumably what their search matched, but the LIM-28
roadmap claim is grounded in captured wire data, not speculation.

## Two corrections to my own work, recorded

1. **My first schema count was wrong.** I initially counted 9 ClickHouse tables / 5 MVs with a
   sloppy regex and would have "confirmed" the existing doc. Re-counted properly against
   `contracts/db/`: **11 tables / 7 MVs**, and meta is **16** not 14. The reviewer's numbers
   were right. Lesson: when a review disputes a count, re-derive it, don't spot-check it.
2. **My first golden regeneration used the wrong helm.** A newer helm injected spurious blank
   lines into every golden — which would itself have been CI drift. Regenerated with CI's
   pinned **helm 3.17.0**; the real diff is comment-only with rendered values unchanged.

## The headline: F-03 — wrong, not merely unvalidated

AMS 3.x exposes no `role` and no `version` on its cluster-nodes endpoint. Discovery therefore
defaults every node to `origin`, version stays empty, and `IsEdgeStream()` — gated on
`role == "edge"` — **can never activate**. LIM-10 called this a confidence gap. It is a
statically provable fact.

The claim had spread to six documents, including **`demo-video-script.md`**, the voiceover the
operator is about to re-record: *"Edge and origin viewers are deduplicated, so the numbers are
real."* That one mattered most — it would have been said out loud in the marketplace demo.

## Disclosed rather than fixed — F-04/F-05/F-06

Cluster node alerting can still miss during an AMS API outage, via six independent mechanisms
(eviction race at the same 3× constant, discovery streak reset, `/applications` short-circuit,
invisible `down` state, unverified `lastUpdateTime` unit, mode-flip blind window).

**Not fixed deliberately.** Every candidate fix retunes alert timing, and there is no live
multi-node cluster to verify against. Guessing risks trading a missed alert for a false one —
worse than the disclosed status quo. All six are now written into LIM-10 as product behaviour,
and the CHANGELOG's R1/R6 entries carry explicit scope corrections rather than continuing to
claim the ladder works. Gated on operator queue item 7 (2-node PAYG cluster).

## Gates at close

| Gate | Result |
|---|---|
| Server `gofmt -l` | clean |
| Server `go build ./...` | pass |
| Server `go vet ./...` | pass |
| Server `go test -race ./...` | **PASS** — full suite, repo-root mounted (D-028/D-064); zero FAIL lines, exit 0 |
| Server `go test ./...` package tally | **26/26 ok, 0 fail, 0 without tests** |
| Server gates re-run after version bump | gofmt clean · vet clean · all packages ok |
| SDK build + tests + size | **6/6 files, 70/70 tests, 3.52 kB** (15 kB budget) |
| Web `tsc --noEmit` | clean |
| Release guard — all 16 checks, local dry-run | **16/16 PASS** at 0.4.3 |
| `qa/mock-ams` build + gofmt | pass |
| Helm render precedence (4 cases) | 4/4 PASS — defaults 768 MB, alias-only 6 GB, new-key 4 GB, both → new key |
| Helm goldens (helm 3.17.0) | regenerated, comment-only diff |
| shellcheck `install.sh` | clean |
| actionlint | clean |
| Prod health | untouched — v0.4.0-139, all 3 components `ok`, 1,321,757 events |

**Note on container gates:** the first `-race` run appeared to fail 20 tests with
"meta DDL not found (repo-root mount required, D-028/D-064)". That was **my mount error**
(I mounted `server/` rather than the repo root), not a regression — re-run correctly with
`-v "$PWD":/repo -w /repo/server`.

## v0.4.3 cut (operator said go; rotation explicitly deferred)

The operator authorised the release cut and chose to **skip the ClickHouse rotation for now**.
Noted once and proceeded — the history exposure exists either way, so the tag does not make it
materially worse, but it stays open as the pre-submission item.

Version surfaces bumped 0.4.2 → 0.4.3 and **all 16 release-guard checks re-run locally against
the tree before committing** (the guard only runs at tag time in CI, so a local dry-run is the
only way to avoid a failed tag push):

`VERSION` · `Chart.yaml` appVersion · doc header stamps (product/faq/known-limitations) ·
SDK `package.json` **and `package-lock.json`** · compose image pins (base + quickstart) ·
helm `values.yaml` tag · helm README table · `install.sh` `PULSE_IMAGE` + `PULSE_REF` ·
Swift SDK constant · README pins + prose · `listing.md` version claim · helm test values +
regenerated goldens · `submission-package.md` header · beacon tarball references.

**Two pre-existing defects found while bumping:**
1. **`sdk/beacon-js/package-lock.json` was stale at 0.4.1** — a full release behind
   `package.json` (0.4.2), and it shipped that way in v0.4.2. No guard covers the lockfile.
   Synced to 0.4.3.
2. **`submission-package.md`'s beacon-tarball row pointed at the wrong release page** (0.4.2
   tarball name on the v0.4.2 link, now both 0.4.3). Also added the v0.4.3 cut and the
   credential rotation to its gate list so the pack matches `operator-expected.md`.

CHANGELOG `[Unreleased]` → `[0.4.3] - 2026-07-27`; `release-notes.md` gained a customer-facing
"New in 0.4.3" section written as corrections rather than features.

## Deliberately NOT done

- No prod roll; no tag (the v0.4.3 cut is the operator's call — recommended in their queue).
- No outbound/marketplace action.
- `install.md`'s `main`-ref downloads kept: the compose carries its own image pin so that path
  is self-consistent, and repointing it at the tag would reintroduce the hardcoded-8090 bug
  that F-08 is about. Rationale documented in the file.
- Guard #11's false-positive risk left alone — no current content triggers it, and every
  tightening weakens the check it exists to perform.

## For the next session

1. **Gate reads first:** prod health (component-scoped + a ClickHouse count), git/PR drift,
   and **whether the operator rotated `CLICKHOUSE_PASSWORD`** (re-run the silent prefix check).
2. **If the operator says go: cut v0.4.3.** Guard #16 now actually runs — the chart is at
   0.3.1 (unpublished; published is 0.3.0), which satisfies it.
3. **If a 2-node cluster appears:** F-04/F-05/F-06 become fixable with verification instead of
   guesswork. That is the single highest-value technical unblock.
4. **Expect another review round.** The pattern holds: verify every claim first, and check the
   reviewer's tree-state assumptions as carefully as their findings.
