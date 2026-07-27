# Operator TODO — the items only YOU can do

*Open items only. No history, no "what changed", no loop-owned work — those live in
`agents/handoffs/decisions.md`, `agents/handoffs/sessions/` and
`agents/handoffs/RESUME-PROMPT.md`.*

> **Blocking submission: item 1, and nothing else.** The submission target is **v0.4.4**
> (you authorised the cut; it contains the closed LIM-01, the anchored `cosign verify`
> command, checksums for all four release assets, and the installer exit code).
> Prod is healthy and untouched.

---

## Your queue (leverage order)

1. **⚠ Rotate `CLICKHOUSE_PASSWORD` — the hard blocker.** A 32-hex prefix of the live
   48-character production password sits in the **public** repo's git history (since `98b011c`,
   via an old test input). The source is scrubbed, but history cannot be un-published — only
   rotation closes it. Not remotely exploitable today (ClickHouse is Docker-internal and never
   published to the host), but it is 128 bits of the secret in a public repo that anyone can
   find with `git log -S`. **Rotate before the repo gets marketplace traffic.**

   *Mechanics — no volume surgery (verified against the live container 2026-07-27):* the
   ClickHouse user is defined in `users_xml` and the image entrypoint rewrites it from the
   environment on **every** container start. So: new value into `deploy/.env` → `up -d` with the
   standing five-overlay combo. The **backup sidecar reads the same variables**, so it must be
   recreated in that same `up -d` or it keeps the stale password. Afterwards rotate the
   remaining chat-exposed set (`deploy/.env`, `oguz-testing.md` — already mode 600).

2. **Review `docs/marketplace/listing.md`** — the submission copy. Free of placeholders and
   internal notes, so it is safe to paste verbatim. Override anything; the category and all
   price wording are yours.

3. **Submit the listing** to the Ant Media Marketplace (your account) — paste from
   `listing.md`, never from `listing-draft.md` (internal). **Submit against `v0.4.4`.**
   Artifact index: `docs/marketplace/submission-package.md`.

   ⚠ **If their reviewer verifies our image signature, tell them to use cosign v3 or newer.**
   Our images are correctly signed, but the signature is stored as an OCI 1.1 *referrer* rather
   than under the old `sha256-<digest>.sig` tag, so a **cosign v2 client reports
   `Error: no signatures found`** (v2.4.3 fails, v3.0.2 passes — verified both ways). The
   README at the tag says so too, but saying it up front costs one sentence and avoids a
   security reviewer concluding we ship unsigned images.

4. **Set up billing** in the marketplace (tiers / Founding-Operators campaign / trial).

5. **Send the Ankush reply** — draft at `docs/marketplace/ankush-reply-draft.md` (fill the
   [brackets], send from your account).

6. **Load lane on a PAYG AMS** → the real capacity number for the listing. Same instance, two
   birds: set `server.kafka_brokers` in `red5.properties` so the loop can run **AV-15** (live
   Kafka validation → drops the EXPERIMENTAL label from the Kafka path), and make it a
   **2-node cluster** to close **LIM-10**. This is the single highest-value technical unblock:
   several ways cluster node alerting can miss during an AMS API outage are known, and **none is
   safely fixable without a real cluster to verify against** — so they are disclosed in LIM-10
   rather than guessed at. A 2-node cluster converts that disclosure into a fix or a proof.

7. **Add an `NPM_TOKEN` repo secret** if you want `npm install ams-pulse-beacon` to work — the
   release workflow then publishes automatically on the next tag (or via `workflow_dispatch`
   `publish_tag`). Without it nothing fails; the tarball still attaches to the release.

8. *Optional:* **add a `GHCR_CLEANUP_TOKEN` repo secret** (a PAT with `delete:packages`) so the
   release pipeline's quarantine-tag cleanup can run. **Without it nothing breaks**: the step
   warns loudly, and you would delete the `candidate-<sha>` tag by hand only after a release
   that actually *failed* its vulnerability scan.

9. **Demo FINAL** — re-record the voiceover over the dark rough-cut attached to the release.
   ⚠ **Re-read `docs/marketplace/demo-video-script.md` first:** the edge/origin viewer-dedup
   line was corrected because it claimed behaviour AMS 3.x cannot support.

10. **Confirm the licensor legal name** stamped in `LICENSE` and `licensing-public.md`:
    "Aytek Erdoğan (beyondkaira.com)". Both files already carry this identical string — this is
    purely your sign-off that it is the correct legal form.

11. *Optional:* **roll prod forward** (a deliberate `deployment.sh` deploy on your go-ahead;
    prod is healthy on its stamped v0.4.0-139 build). Note that prod will not show the new Fleet
    CPU/memory/disk gauges until it is rolled — that is a code change. · **VPS Chromium deps**
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

---

*Prod: healthy and untouched — v0.4.0-139, all three `/healthz` components `ok`, 1,336,799
server events, newest 16 s old, collector actively ingesting. A prod roll is item 11, never
automatic.*

*On item 1: external review rounds 7, 8 and 9 have now each landed fixes on `main` that are not
in the v0.4.4 tag, and one of them (the geo/device breakdown row cap) changes API responses.
Nothing here changes item 1's priority — but when you rotate, the same sitting authorises one
motion: **rotate, cut v0.4.5, submit against it**. That single cut also clears the stale prose
frozen inside the v0.4.4 tag, which is the only remaining thing an evaluator could catch.*

*Noticed while probing your AMS: its licence shows `type: trial`, `endDate 2026-07-27` —
expiring today. It affects nothing we ship, but it does affect future live validation against
that instance.*
