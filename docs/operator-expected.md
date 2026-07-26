# Operator TODO — the items only YOU can do

*Updated 2026-07-27, SESSION-109 (D-176). Kept to **open items only** per your directive —
the superseded S93–S108 status stack lives in git history and `agents/handoffs/sessions/`.*

> **▶ S109 in one paragraph:** you sent the external review's **round 4** (F-01…F-14), which
> for the first time got the tree state exactly right — no stale premise to correct. Every
> finding was re-verified against the code before any fix: **all of them confirmed**, plus one
> sub-claim refuted (it said the `originAdress` field appears nowhere in the tree; it is in the
> real-AMS 3.0.3 capture fixtures, so LIM-28's roadmap is sound). Its verdict — *"the residual
> risk is concentrated almost entirely in claims, not code"* — is fair, and the claims are now
> fixed. Headlines: the README/LIM-10 **cluster capability claims were wrong, not merely
> unvalidated** (AMS 3.x sends no node role or version, so all nodes show as `origin` and
> edge/origin viewer dedup can never activate — that claim had propagated into six documents
> including **the demo voiceover script you are about to re-record**); `release-notes.md`
> asserted two things about v0.4.2 that an evaluator could falsify in minutes; and the release
> pipeline's GHCR quarantine cleanup **could never have run** — it authenticated with a token
> type that has no access to the endpoint it called, and exited reporting success. **Nothing
> here changes your #1 item: the ClickHouse password is still un-rotated** (verified directly
> this session — the live 48-char value's first 32 characters still match what is in public git
> history). Prod untouched and healthy. Full record: `decisions.md` D-176 ·
> `sessions/SESSION-109.md` · review + disposition table:
> `docs/assessment/marketplace-compliance-review-2026-07-27-round4.md`.
>
> **▶ Recommended next move (the review's, and mine): cut `v0.4.3` from `main` and submit
> against that tag, not against v0.4.2.** The fixes an evaluator would hit in their first ten
> minutes — the honest docs, the regenerated screenshots, the evaluator compose overlay, the
> quickstart host-port parameterization — all live on `main` and are *not* in the published
> v0.4.2. Releasing costs one tag push; an evaluator hitting a fixed-on-main defect costs the
> review. See item 2.

---

## Your queue (leverage order)

1. **⚠ Rotate `CLICKHOUSE_PASSWORD` first.** `server/cmd/pulse/migrate_test.go` used the
   first 32 of the 48 hex characters of the live production password as its test input, and
   that value has been in the **public** repo since commit `98b011c`. The source is fixed, but
   git history cannot be un-published — only rotation closes it. ClickHouse is not
   internet-facing (Docker-internal only), so this is not remotely exploitable today; it is
   still 128 bits of the secret sitting in a public repo, and anyone can find it with
   `git log -S`. Rotate before the repo gets marketplace traffic. Then rotate the rest
   (`deploy/.env`, `oguz-testing.md` — the latter's file mode is now 600).
2. **Say go on cutting `v0.4.3`, then submit against it.** One word from you and the loop
   tags it — the release pipeline does the rest. Why it matters: everything the round-4
   review says an evaluator would trip over in their first ten minutes is fixed **on `main`
   only**. The published v0.4.2 still has the wrong cluster claims, the pre-fix screenshots,
   no evaluator compose overlay, and a quickstart compose that ignores `PULSE_HOST_PORT`.
   Submitting v0.4.2 while the corrected tree sits on `main` is the likeliest way this
   submission goes sideways. (Do item 1 first — the tag makes the repo interesting.)
3. **Review `docs/marketplace/listing.md`** — the submission copy. The internal
   "*Proposed pending Ant Media confirmation*" note under Category has been removed, so the
   file is now safe to paste verbatim. Override anything; the category and all price wording
   are yours.
4. **Submit the listing** to the Ant Media Marketplace (your account) — paste from
   `listing.md`, never from `listing-draft.md` (internal). Artifact index:
   `docs/marketplace/submission-package.md` (header and gate list now current).
5. **Set up billing** in the marketplace (tiers / Founding-Operators campaign / trial).
6. **Send the Ankush reply** — draft ready: `docs/marketplace/ankush-reply-draft.md`
   (fill the [brackets], send from your account).
7. **Load lane on a PAYG AMS** → the real capacity number for the listing. Same instance,
   three birds: set `server.kafka_brokers` in red5.properties so the loop runs **AV-15**
   (live Kafka validation → drops the EXPERIMENTAL label), and make it a **2-node cluster**
   to close **LIM-10** — which also live-validates the S106–S109 cluster-path work. This is
   now the single highest-value technical unblock: round 4 found several ways cluster node
   alerting can still miss during an AMS API outage, and **none of it can be fixed
   confidently without a real cluster to verify against** — so it is disclosed in LIM-10
   rather than guessed at. A 2-node cluster converts that whole disclosure into either a
   fix or a proof.
8. **Add an `NPM_TOKEN` repo secret** if you want `npm install ams-pulse-beacon` to work —
   the release workflow then publishes automatically on the next tag (or via
   workflow_dispatch `publish_tag`). v0.4.2 shipped without it; the tarball is attached to
   the release and nothing failed.
9. **Optional: add a `GHCR_CLEANUP_TOKEN` repo secret** (a PAT with `delete:packages`).
   Round 4 found the release pipeline's quarantine cleanup could never execute — it used
   `GITHUB_TOKEN`, which has no access to the package-deletion endpoint, and exited
   reporting "nothing to clean up". Now fixed to use the right endpoint and to assert the
   HTTP status, but it needs a real PAT. **Without it nothing breaks**: the step warns
   loudly and you delete the `candidate-<sha>` tag by hand — and only after a release that
   actually *failed* its vulnerability scan, which has never happened.
10. **Demo FINAL** — re-record voiceover over the dark rough-cut attached to the release.
    ⚠ **The script changed this session:** the old line *"Edge and origin viewers are
    deduplicated, so the numbers are real"* claimed a feature that cannot work on AMS 3.x
    (no node roles on the wire). `docs/marketplace/demo-video-script.md` now has the
    corrected line — re-read it before recording.
11. **Confirm the licensor legal name** stamped in LICENSE/licensing docs:
   "Aytek Erdoğan (beyondkaira.com)" — one word if right, or give the exact form. (Both
   `LICENSE` and `licensing-public.md` already carry this identical string; nothing is
   inconsistent, this is purely your sign-off that it is the correct legal form.)
12. *Optional:* **roll prod to v0.4.2 or v0.4.3** (deliberate `deployment.sh` deploy on your
    go-ahead; prod is healthy on its stamped v0.4.0-139 build) · VPS Chromium deps
    (`sudo npx playwright install-deps chromium`) if you want captures to run natively on
    the VPS (they currently run fine via the Playwright container).

**Closed since the last update — no action needed:**
- ~~Flip the Helm chart OCI package public~~ — **already public** (verified anonymously in
  S108: `ghcr.io/aytekxr/charts/pulse:0.3.0` returns HTTP 200 token-lessly).
- ~~Confirm compose-boot ran un-deferred after the release~~ — **it did**, run `30215677015`
  series on main, all 12 jobs green.

## Decision-gated engineering (one word each unblocks a build)

- **§2.45** Pulse-native self-alert paging half (maintenance-window semantics + tier/channels
  ruling; the Prometheus half shipped in D-167).
- **§2.44 `[FO-1]`** firing-orphan behavior for node/QoE alerts whose subject vanishes:
  auto-resolve-after-grace (loop's lean) / stay-firing / leave-as-is.
- **Dependabot queue** (17 PRs, operator-held): confirm the hold, or authorize a batch-absorb
  session per `docs/dependabot-policy.md`.

## Standing meeting items (unchanged A-ledger)

Bring to the Ant Media developer meeting: listing format/category confirmation (A2/A10),
asset specs (A3), post-year-1 revenue terms **in writing** (A4), review flow/SLA (A5), trial
expectations (A6), AMS version-support expectations (A7), docs-linking policy (A8),
load-evidence format (A9). Details: `docs/marketplace/submission-process.md`.

## Tracked engineering debt (loop-owned, non-blocking — no action from you)

- Surface `stream_ingest_error` rows in ingest health + an alert condition (disclosed
  meanwhile as LIM-27, with the query operators can run today).
- Thread the owning cluster node through per-app polling so stream-level node filtering
  matches the Fleet view (LIM-28) — deliberately deferred until a real cluster exists to
  verify against, rather than guessed.
- Poller/discovery cadence consolidation (the two emitters now agree key-for-key and are
  test-pinned, but still poll independently); WS-broadcast marshal-error logging.
- Helm NetworkPolicy golden variant (no CI coverage on that template).
- compose-boot `--wait` compose-version sensitivity (watch item).
- One web test (`SettingsPage` tabpanel ARIA wiring) fails only under full-suite parallel
  execution and passes in isolation — test pollution, not a product defect. Worth isolating.
- **Cluster node-alerting rework (round 4, F-04/F-05/F-06) — waits on your item 7.** Four
  independent gaps: the `node_degraded` threshold and the stale-node eviction threshold are
  both 3×poll so the degraded state can be evicted before it alerts; a discovery poll
  succeeding mid-outage resets the streak; an `/applications` outage short-circuits the poll
  before the cluster branch; and the computed `down` state reaches no event, API or alert.
  All four are **disclosed in LIM-10** and none is safely fixable without a live cluster —
  fixing them blind risks trading a missed alert for a false one.
- **Verify the `ClusterNodeDTO.lastUpdateTime` unit against AMS source** (assumed epoch ms;
  if it is seconds, every node is silently marked down). Recorded as an explicit unverified
  assumption in `AMS-INTEGRATION.md` §1.1.

**Cleared this session (S109):** the cluster capability overclaims across 6 documents · the
two false v0.4.2 release-note claims · the `install.md` tier inversion and 6 other residues ·
the GHCR cleanup token/endpoint · the Helm deprecated-alias inversion + inert guard #16 ·
`CPUPctOK` fabricating a measured 0% · the mock-AMS 2024 timestamp that put every mock
cluster node permanently "down" · the quickstart re-run hard-fail.

*Prod: healthy and untouched — v0.4.0-139, collector `ok`, 1.32 M events. A prod roll is
item 12, never automatic.*
