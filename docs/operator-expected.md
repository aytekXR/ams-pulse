# Operator TODO — the items only YOU can do

*Updated 2026-07-26, SESSION-108 (D-175). Kept to **open items only** per your directive —
the superseded S93–S107 status stack lives in git history and `agents/handoffs/sessions/`.*

> **▶ S108 in one paragraph:** you sent a third-party review's third pass (R1–R15) with
> "add this to your fixes". Every finding was re-verified against the tree first: 14 confirmed,
> 1 (the VPS-IP scrub) already settled by your own D-174 ruling, and one premise of the review
> stale — it assumed the v0.4.2 tag was unpushed, when v0.4.2 had in fact already been released.
> All 14 are fixed, with regression tests for the four code defects. **A parallel verify-first
> audit of the published artifact then found five more that the review missed**, including two
> that matter to you directly: the **production ClickHouse password prefix is in public git
> history** (item 1 below — this is why rotation moved to the top), and the **flagship listing
> screenshot showed every stream as UNKNOWN** with an empty per-application panel — a monitoring
> product whose own dashboard displayed nothing monitored. Both fixed; screenshots regenerated
> and visually checked. Prod untouched. Full record: `decisions.md` D-175 ·
> `sessions/SESSION-108.md` · review + disposition table:
> `docs/assessment/marketplace-compliance-review-2026-07-26-round3.md`.

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
2. **Review `docs/marketplace/listing.md`** — the submission copy. The internal
   "*Proposed pending Ant Media confirmation*" note under Category has been removed, so the
   file is now safe to paste verbatim. Override anything; the category and all price wording
   are yours.
3. **Submit the listing** to the Ant Media Marketplace (your account) — paste from
   `listing.md`, never from `listing-draft.md` (internal). Artifact index:
   `docs/marketplace/submission-package.md` (header and gate list now current).
4. **Set up billing** in the marketplace (tiers / Founding-Operators campaign / trial).
5. **Send the Ankush reply** — draft ready: `docs/marketplace/ankush-reply-draft.md`
   (fill the [brackets], send from your account).
6. **Load lane on a PAYG AMS** → the real capacity number for the listing. Same instance,
   three birds: set `server.kafka_brokers` in red5.properties so the loop runs **AV-15**
   (live Kafka validation → drops the EXPERIMENTAL label), and make it a **2-node cluster**
   to close **LIM-10** — which also live-validates the S106/S107/S108 cluster-path work.
   Four of this session's fixes (R1, R2, R5, R6) are cluster-path and have only ever been
   proven against tests and the real AMS wire format, never a live multi-node cluster.
7. **Add an `NPM_TOKEN` repo secret** if you want `npm install ams-pulse-beacon` to work —
   the release workflow then publishes automatically on the next tag (or via
   workflow_dispatch `publish_tag`). v0.4.2 shipped without it; the tarball is attached to
   the release and nothing failed.
8. **Demo FINAL** — re-record voiceover over the dark rough-cut attached to the release.
9. **Confirm the licensor legal name** stamped in LICENSE/licensing docs:
   "Aytek Erdoğan (beyondkaira.com)" — one word if right, or give the exact form. (Both
   `LICENSE` and `licensing-public.md` already carry this identical string; nothing is
   inconsistent, this is purely your sign-off that it is the correct legal form.)
10. *Optional:* **roll prod to v0.4.2** (deliberate `deployment.sh` deploy on your
    go-ahead; prod is healthy on its stamped v0.4.0 build) · VPS Chromium deps
    (`sudo npx playwright install-deps chromium`) if you want captures to run natively on
    the VPS (they currently run fine via the Playwright container).

**Closed since the last update — no action needed:**
- ~~Flip the Helm chart OCI package public~~ — **already public.** Verified anonymously this
  session: `ghcr.io/aytekxr/charts/pulse:0.3.0` returns HTTP 200 on both tags-list and
  manifest with a token-less pull scope.
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
  test-pinned, but still poll independently); edge-dedup inertness disclosure;
  WS-broadcast marshal-error logging.
- Helm NetworkPolicy golden variant (no CI coverage on that template).
- compose-boot `--wait` compose-version sensitivity (watch item).
- One web test (`SettingsPage` tabpanel ARIA wiring) fails only under full-suite parallel
  execution and passes in isolation — test pollution, not a product defect. Worth isolating.

**Cleared this session:** AMS `status`/`lastUpdateTime` node liveness (R6, shipped with
tests) · helm configmap scoping + goldens (R4) · the doc-citation drift mechanism (R3).

*Prod: healthy and untouched — v0.4.0-139, collector `ok`. A prod roll to 0.4.2 is item 10,
never automatic.*
