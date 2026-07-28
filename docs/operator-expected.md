# Operator TODO — the items only YOU can do

*Open items only. No history, no "what changed", no loop-owned work — those live in
`agents/handoffs/decisions.md`, `agents/handoffs/sessions/` and
`agents/handoffs/RESUME-PROMPT.md`.*

> **Blocking submission: item 1, and nothing else.** The submission target is **`v0.4.5`, once
> you cut it** — `v0.4.4` is still the published Latest, and `main` has since accumulated the
> round-7 through round-11 fixes, five of them behavioural (see the note at the foot of this file).
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
   `listing.md`, never from `listing-draft.md` (internal). **Submit against the tag you cut in
   item 1's sitting — `v0.4.5` if you take the recommended motion, `v0.4.4` if you decide not to
   cut.** (Until D-184 this line read "submit against `v0.4.4`" while the same file's closing note
   recommended cutting `v0.4.5` first — the two now agree, and the choice is yours either way.)
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
- ~~**`[M-04]`** beacon identity-field length limits~~ — **CLOSED in D-185, no ruling needed.** The
  deferral rested on truncation corrupting `uniq(session_id)`, which is true, and on rejection
  needing a contract change, which turned out to be a *tightening* no conforming client can notice:
  `session_id`/`stream_id`/`app` are now capped at 256 bytes (~7× a UUIDv4) in the contract and
  rejected past it, and `player_kind` is truncated. What forced the call was arithmetic the
  original deferral did not have — those fields are copied onto every row a batch produces, so one
  64 KB request could write ~50× its own size. Override the cap or the failure mode if you disagree;
  both are one constant.
- **`[ANOM-TIER]` Is anomaly detection Enterprise-only, or Business-and-up?** Your pricing pages
  say Enterprise (`docs/overview.md` tier table: "Business + F9 anomaly detection";
  `docs/marketplace/listing.md`; PRD §7.11). The code says something narrower: only
  `GET /anomalies` checks the tier, so a **Business** tenant can create a `rule_type=anomaly`
  alert rule and receive anomaly alerts — and our own e2e scenario A5 mints a Business licence and
  **requires** exactly that to work, and has for many sessions. So this is not a bug that slipped
  past review; it is two sources of truth disagreeing. **Either answer is one line**, and the loop
  deliberately did not pick, because picking changes something real in both directions: enforcing
  it stops alerts that Business tenants get today, and not enforcing it means the listing
  advertises as Enterprise-exclusive something Business already has. One word: **"enforce"**
  (gate rule create/update + the evaluator, and A5 moves to an Enterprise licence) or **"advertise
  correctly"** (move F9 anomaly alerting down the tier table and leave the code alone). Relevant
  before you submit the listing — item 3 pastes that tier table.
- **Dependabot queue** (17 PRs open, operator-held): confirm the hold, or authorise a
  batch-absorb session per `docs/dependabot-policy.md`.

## Standing meeting items (A-ledger)

Bring to the Ant Media developer meeting: listing format/category confirmation (A2/A10), asset
specs (A3), post-year-1 revenue terms **in writing** (A4), review flow/SLA (A5), trial
expectations (A6), AMS version-support expectations (A7), docs-linking policy (A8), and
load-evidence format (A9). Details: `docs/marketplace/submission-process.md`.

---

*Prod: healthy and untouched — v0.4.0-139, all three `/healthz` components `ok`, 1,340,393
server events, ingest steady at 720/h, collector actively ingesting. A prod roll is item 11, never
automatic.*

*On item 1: external review rounds 7 through **11** have now each landed fixes on `main` that are
not in the v0.4.4 tag. Five of them are behavioural — the geo/device breakdown row cap (changes API
responses), the three D-184 security/enforcement fixes, and D-185's AMS-poll SSRF guard plus the
`/healthz` fix that stops an AMS error body being republished to unauthenticated callers. Nothing
here changes item 1's priority — but when you rotate, the same sitting authorises one motion:
**rotate, cut v0.4.5, submit against it**. That single cut also clears the stale prose frozen inside
the v0.4.4 tag. Round 11 reaches the same conclusion independently: "not blocked by review —
blocked by one rotation, one SSRF wire, and one tag." The SSRF wire is done; the rotation and the
tag are yours.*

*Two things D-185 changed that you may want to know about before the cut: the AMS poll client no
longer honours `HTTP(S)_PROXY` (an egress proxy defeats the SSRF guard — same ruling already applied
to the prober and the S3 uploader; no documented deployment routes AMS polling through a proxy), and
`main`'s required-status-check list grew from 13 to 15 (`shellcheck`, `doc-stamps` — they were
running but could not block a merge). The branch-protection change is already applied.*

*Noticed while probing your AMS: its licence shows `type: trial`, `endDate 2026-07-27` — i.e.
**expired as of 2026-07-28**. It affects nothing we ship, but it does affect future live validation
against that instance, including item 6's load lane.*
