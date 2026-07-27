# Operator TODO — the items only YOU can do

*Forward-looking only: open items and pending decisions. This file carries no history — what
has already been done lives in `agents/handoffs/decisions.md` and `agents/handoffs/sessions/`.*

> **▶ Where things stand:** **v0.4.3 is released**, and `main` is now ahead of it by **code**,
> not just copy. Prod is healthy and untouched. **Items 1 and 2 below block submission.**
>
> **New since you last looked (D-179 — external review round 6):** nine findings, all confirmed,
> eight fixed. The one worth your attention is the one the reviewer *could not test*: they had no
> live AMS, so they filed a LOW-severity guess that AMS might expose node CPU/memory on a console
> endpoint we never call. It does. **LIM-01 is closed** — the Fleet CPU, memory and disk gauges
> now work on a standalone AMS with **no Kafka**, which is the first Priority-1 limitation the
> product has carried since its first release. Verified live against your AMS 3.0.3.
>
> That is why **item 2 is new**: the fixes that matter to an evaluator now include working code,
> so submitting against the `v0.4.3` tag means submitting something older than what we have.

---

## Your queue (leverage order)

1. **⚠ Rotate `CLICKHOUSE_PASSWORD` — the hard blocker.** A 32-hex prefix of the live
   48-character production password sits in the **public** repo's git history (since `98b011c`,
   via an old test input). The source is scrubbed, but history cannot be un-published — only
   rotation closes it. It is not remotely exploitable today (ClickHouse is Docker-internal and
   never published to the host), but it is 128 bits of the secret in a public repo that anyone
   can find with `git log -S`. **Rotate before the repo gets marketplace traffic.**

   *Mechanics — no volume surgery needed (verified against the live container 2026-07-27):* the
   ClickHouse user is defined in `users_xml`, and the image entrypoint rewrites it from the
   environment on **every** container start — so rotation is just: new value into `deploy/.env`
   → `up -d` with the standing five-overlay combo. The **backup sidecar reads the same
   variables**, so it must be recreated in that same `up -d` or it keeps the stale password.
   Afterwards, rotate the remaining chat-exposed set (`deploy/.env`, `oguz-testing.md` — the
   latter is already mode 600).
2. **⚠ Decide: cut `v0.4.4` and submit against it, or submit against `v0.4.3`?**
   (external review round 6, H-02 — **one word from you unblocks it**)

   The `v0.4.3` tag does not contain: the **LIM-01 fix** (standalone CPU/memory/disk gauges), the
   corrected `cosign verify` command, `SHA256SUMS` covering all four release assets, the
   installer's degraded exit code, the HLS-accuracy caveat in the listing, or the AMS 2.17
   compatibility work. Round 5 ruled that doc fixes could ride the next release; that was before
   the delta contained code.

   **The loop recommends cutting v0.4.4.** It is one tag push — but only **after main's
   post-merge CI goes green** (tagging early fails the release gate cleanly; that is a standing
   rule now). Everything else is staged and verified.

   *Alternative:* submit against `v0.4.3` and tell their reviewer that the docs to read are on
   `main`. Cheaper, weaker, and it means an evaluator installing from the tag gets a Pulse whose
   Fleet gauges are blank for a reason we have already fixed.

3. **Review `docs/marketplace/listing.md`** — the submission copy. It is free of placeholders
   and internal notes, so it is safe to paste verbatim. Override anything; the category and all
   price wording are yours. *Two lines changed in D-178:* the screenshot note now says the shots
   are the shipping UI with **representative demo data** (it previously claimed "captured from a
   live deployment", which was not true — they are route-mocked captures), and the Helm bullet
   now advertises the published OCI chart instead of understating it as a local chart path.
   *Changed again in D-179:* the viewer-count bullet, the long description and the SS1 caption
   now carry the HLS accuracy caveat (AMS's HLS count overcounts concurrent sessions — LIM-02),
   and the compatibility line names the AMS versions actually profiled instead of implying
   everything from 2.10 up.
4. **Submit the listing** to the Ant Media Marketplace (your account) — paste from
   `listing.md`, never from `listing-draft.md` (internal). **Submit against whichever tag you
   choose in item 2.** Artifact index: `docs/marketplace/submission-package.md`.

   ⚠ **If their reviewer verifies our image signature, tell them to use cosign v3 or newer.**
   Our images are correctly signed, but cosign v3 stores the signature as an OCI 1.1 *referrer*
   rather than under the old `sha256-<digest>.sig` tag, so a **cosign v2 client reports
   `Error: no signatures found`** — verified both ways against `0.4.3` (v2.4.3 fails, v3.0.2
   passes). Saying this up front costs one sentence; not saying it risks a security reviewer
   concluding we ship unsigned images. *(If you cut v0.4.4, the README at that tag states the
   v3 requirement itself and the published command is the anchored one — you still get one
   sentence of value from saying it up front.)*
5. **Set up billing** in the marketplace (tiers / Founding-Operators campaign / trial).
6. **Send the Ankush reply** — draft ready at `docs/marketplace/ankush-reply-draft.md` (fill
   the [brackets], send from your account).
7. **Load lane on a PAYG AMS** → the real capacity number for the listing. Same instance, three
   birds: set `server.kafka_brokers` in red5.properties so the loop runs **AV-15** (live Kafka
   validation → drops the EXPERIMENTAL label — note this is now *only* about the Kafka path's own
   status; node CPU/memory/disk no longer need Kafka at all since D-179), and make it a
   **2-node cluster** to close
   **LIM-10**. This is the single highest-value technical unblock: several ways cluster node
   alerting can miss during an AMS API outage are known, and **none is safely fixable without a
   real cluster to verify against** — so they are disclosed in LIM-10 rather than guessed at.
   A 2-node cluster converts that disclosure into either a fix or a proof.
8. **Add an `NPM_TOKEN` repo secret** if you want `npm install ams-pulse-beacon` to work — the
   release workflow then publishes automatically on the next tag (or via workflow_dispatch
   `publish_tag`). Without it nothing fails; the tarball is still attached to the release.
9. **Optional: add a `GHCR_CLEANUP_TOKEN` repo secret** (a PAT with `delete:packages`) so the
   release pipeline's quarantine-tag cleanup can run. **Without it nothing breaks**: the step
   warns loudly and you delete the `candidate-<sha>` tag by hand — and only after a release
   that actually *failed* its vulnerability scan.

   *Separately, and cosmetic:* every **successful** release also leaves a `candidate-<sha>` tag
   visible on the GHCR package (round 6, H-09). It is an alias of the released digest, not a
   second image, and `SECURITY.md` now says so for anyone who notices. It cannot be deleted
   without deleting the release itself — GHCR deletes by digest, not by tag — so the real fix is
   a pipeline change tracked as engineering debt, not something you need to do.
10. **Demo FINAL** — re-record the voiceover over the dark rough-cut attached to the release.
   ⚠ **Re-read `docs/marketplace/demo-video-script.md` before recording:** the edge/origin
   viewer-dedup line was corrected, because it claimed behaviour AMS 3.x cannot support.
11. **Confirm the licensor legal name** stamped in `LICENSE` and `licensing-public.md`:
    "Aytek Erdoğan (beyondkaira.com)". Both files already carry this identical string — this is
    purely your sign-off that it is the correct legal form.
12. *Optional:* **roll prod forward** (a deliberate `deployment.sh` deploy on your go-ahead;
    prod is healthy on its stamped v0.4.0-139 build). Worth noting: prod will not show the new
    Fleet CPU/memory/disk gauges until it is rolled, since that is a code change · VPS Chromium
    deps
    (`sudo npx playwright install-deps chromium`) if you want screenshot captures to run
    natively on the VPS rather than via the Playwright container.

## Decision-gated engineering (one word each unblocks a build)

- **§2.45** Pulse-native self-alert paging half — maintenance-window semantics plus the
  tier/channels ruling. (The Prometheus half has already shipped.)
- **§2.44 `[FO-1]`** firing-orphan behaviour for node/QoE alerts whose subject vanishes:
  auto-resolve-after-grace (the loop's lean) / stay-firing / leave-as-is.
- **Dependabot queue** (17 PRs open, operator-held): confirm the hold, or authorise a
  batch-absorb session per `docs/dependabot-policy.md`.

## Standing meeting items (A-ledger)

Bring to the Ant Media developer meeting: listing format/category confirmation (A2/A10), asset
specs (A3), post-year-1 revenue terms **in writing** (A4), review flow/SLA (A5), trial
expectations (A6), AMS version-support expectations (A7), docs-linking policy (A8), and
load-evidence format (A9). Details: `docs/marketplace/submission-process.md`.

## Tracked engineering debt (loop-owned, non-blocking — no action from you)

- **Cluster node-alerting rework — waits on your item 6.** Four independent gaps: the
  `node_degraded` threshold and the stale-node eviction threshold are both 3×poll, so the
  degraded state can be evicted before it alerts; a discovery poll that succeeds mid-outage
  resets the streak; an `/applications` outage short-circuits the poll before the cluster
  branch; and the computed `down` state reaches no event, API or alert. All four are
  **disclosed in LIM-10**; fixing them blind risks trading a missed alert for a false one.
- **Verify the `ClusterNodeDTO.lastUpdateTime` unit against AMS source** (assumed epoch ms; if
  it is actually seconds, every node is silently marked down). Recorded as an explicit
  unverified assumption in `AMS-INTEGRATION.md` §1.1.
- Surface `stream_ingest_error` rows in ingest health plus an alert condition (disclosed
  meanwhile as LIM-27, with the query operators can run today).
- Thread the owning cluster node through per-app polling so stream-level node filtering matches
  the Fleet view (LIM-28) — deferred until a real cluster exists to verify against.
- Poller/discovery cadence consolidation (the two emitters agree key-for-key and are
  test-pinned, but still poll independently); WS-broadcast marshal-error logging.
- **Release pipeline: push candidates by digest** (buildx `push-by-digest=true`) so a promoted
  image stops carrying a public `candidate-<sha>` alias. This is the correct fix for round 6's
  H-09; it changes how images are published and only a real tag exercises it, so it is deferred
  until after the marketplace submission rather than altered untested. Reasoning is recorded in
  `release.yml` beside the code.
- **Re-probe the LIM-04 (FPS always 0) assumption.** LIM-01 turned out to rest on an unprobed
  generalization — "this endpoint omits it" had become "AMS cannot report it". LIM-04 has the
  same shape. Worth one pass now that the pattern is known.
- Helm NetworkPolicy golden variant (no CI coverage on that template).
- compose-boot `--wait` compose-version sensitivity (watch item).
- One web test (`SettingsPage` tabpanel ARIA wiring) fails only under full-suite parallel
  execution and passes in isolation — test pollution, not a product defect.

---

*Prod: healthy and untouched — v0.4.0-139, all three `/healthz` components `ok`, 1,328,195
server events, collector actively ingesting. A prod roll is item 12, never automatic.*

*One thing we noticed while probing your AMS: its licence shows `type: trial`,
`endDate 2026-07-27` — expiring today. It does not affect anything we ship, but it does affect
future live validation against that instance.*
