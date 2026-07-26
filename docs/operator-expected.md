# Operator TODO — the items only YOU can do

*Updated 2026-07-26, SESSION-107 (D-174). Rewritten per your directive to keep **open items
only** — the superseded S93–S106 status stack lives in git history and
`agents/handoffs/sessions/`.*

> **▶ S107 in one paragraph:** you sent the reviewer's second pass (REVIEW-MP3, findings
> N1–N9 on the post-S106 tree). Every claim was verified first (4 findings partially refuted
> with evidence, the rest confirmed), all confirmed defects were fixed — including three that
> would have shipped broken inside v0.4.2: a real cluster's transient 500 silently flipping
> the Fleet view to "standalone", every cluster node collapsing onto one identity, and the
> hardened Compose overlay not booting at all. **v0.4.2 is RELEASED and verified**: tagged
> only after those fixes landed (PR #218, 15/15 checks), release pipeline green on its first
> quarantine-flow run — image Trivy-scanned on both arches BEFORE the public tags existed,
> `0.4.2` and `latest` confirmed anonymously pullable at the SAME digest, cosign-signed,
> binaries + SHA256SUMS + SDK tarball + Helm chart attached. **The published artifact now
> matches the code and docs — the #1 finding across all three review passes is closed.**
> Prod untouched (v0.4.0-139, 719 rows/h at session close). Full record: `decisions.md`
> D-174 · `sessions/SESSION-107.md` · review verbatim:
> `docs/assessment/marketplace-compliance-review-2026-07-26.md`.

---

## Your queue (leverage order)

1. **Review `docs/marketplace/listing.md`** — the submission copy (updated again in S107:
   category "Analytics & Monitoring", Go 1.25, internal references stripped). Override
   anything; the category and all price wording are yours.
2. **Submit the listing** to the Ant Media Marketplace (your account) — paste from
   `listing.md`, never from `listing-draft.md` (internal). Artifact index:
   `docs/marketplace/submission-package.md` (its gate list is now the real one).
3. **Set up billing** in the marketplace (tiers / Founding-Operators campaign / trial).
4. **Send the Ankush reply** — draft ready: `docs/marketplace/ankush-reply-draft.md`
   (fill the [brackets], send from your account).
5. **Load lane on a PAYG AMS** → the real capacity number for the listing. Same instance,
   three birds: set `server.kafka_brokers` in red5.properties so the loop runs **AV-15**
   (live Kafka validation → drops the EXPERIMENTAL label), and make it a **2-node cluster**
   to close **LIM-10** — which now also live-validates the S106+S107 cluster-path rebuild.
6. **Add an `NPM_TOKEN` repo secret** if you want `npm install ams-pulse-beacon` to work —
   the release workflow then publishes automatically on the next tag (or via
   workflow_dispatch `publish_tag`). v0.4.2 shipped without it; the tarball is attached to
   the release and nothing failed.
7. **Flip the Helm chart OCI package public** on GHCR — it EXISTS now (the v0.4.2 release
   pushed `ghcr.io/aytekxr/charts/pulse`) and starts private. Web-UI only, no API, exactly
   like the image was: GitHub → Packages → package settings → Change visibility → Public.
   Until flipped, `helm install` from OCI fails anonymously (the in-repo chart path works).
8. **Demo FINAL** — re-record voiceover over the dark rough-cut attached to the release.
9. **Confirm the licensor legal name** stamped in LICENSE/licensing docs:
   "Aytek Erdoğan (beyondkaira.com)" — one word if right, or give the exact form.
10. **Rotate the chat-exposed / VPS-group-readable secrets** (`deploy/.env`,
    `oguz-testing.md`) — carried; do it before the repo gets marketplace traffic. (Related
    S107 decision: the VPS IP stays only where it is functional — nginx vhost, load-lane
    prod-guard blocklist, your own runbooks' `--resolve` commands — since the public DNS of
    beyondkaira.com resolves to it anyway; illustrative doc mentions are scrubbed.)
11. *Optional:* **roll prod to v0.4.2** (deliberate `deployment.sh` deploy on your
    go-ahead; prod is healthy on its stamped v0.4.0 build) · VPS Chromium deps
    (`sudo npx playwright install-deps chromium`) if you want captures to run natively on
    the VPS.

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

- Cluster-path P2 remainder from REVIEW-MP3: AMS `status`/`lastUpdateTime`-based node
  liveness, edge-dedup inertness disclosure, poller/discovery cadence consolidation,
  WS-broadcast marshal-error logging.
- Helm nits awaiting the next chart change (golden regeneration): configmap comment wording,
  NetworkPolicy golden variant.
- compose-boot `--wait` compose-version sensitivity (watch item).

*Prod: healthy and untouched — v0.4.0-139, collector `ok`. A prod roll to 0.4.2 is item 11,
never automatic.*
