# Operator TODO — the items only YOU can do

*Open items only. No history, no "what changed", no loop-owned work — those live in
`agents/handoffs/decisions.md`, `agents/handoffs/sessions/` and
`agents/handoffs/RESUME-PROMPT.md`.*

> **Two independent tracks, one blocker each.**
> **Marketplace** is blocked by item 1 (rotate `CLICKHOUSE_PASSWORD`) and nothing else.
> **iOS TestFlight** is blocked by item A (Apple Developer Program enrolment) and nothing else.
> Neither blocks the other; do them in whichever order suits you.
> Prod is healthy and untouched.

---

## A. iOS TestFlight — the critical path

**Everything on our side is built and verified.** The app compiles for iOS 26 under Swift 6
strict concurrency on a real GitHub macOS runner, 259 Linux tests and 50 simulator tests pass,
the archive-and-upload pipeline is written, and the tester-facing website is live. What is left
is the part that legally requires your Apple identity. **Nothing below can be automated away —
each step needs a human with your Apple ID.**

Full runbook with screenshots-worth-of-detail: **`docs/mobile/ios-testflight.md`**.

| # | Step | Where | Time |
|---|---|---|---|
| **A1** | **Enrol in the Apple Developer Program.** ~$99/yr. *Individual* is instant-ish and lists you personally as the seller; *Organization* shows the company name but needs a D-U-N-S number and can take days-to-weeks. Given `beyondkaira.com` is a company domain, Organization is the better long-term answer — but Individual gets testers onto the app this week and can be migrated later. **Your call; it is the only genuinely irreversible-ish choice here.** | developer.apple.com | 30 min + wait |
| **A2** | **Create the App ID** `com.beyondkaira.pulse` (Certificates, Identifiers & Profiles → Identifiers). No special capabilities needed. | developer.apple.com | 5 min |
| **A3** | **Create the App Store Connect app record.** ⚠ The App Store *name* must be globally unique and plain "Pulse" is certainly taken. Suggestions: "Pulse for Ant Media", "Pulse Stream Monitor", "Pulse AMS". The on-device name stays "Pulse" regardless. | appstoreconnect.apple.com | 10 min |
| **A4** | **Create an App Store Connect API key** — Users and Access → Integrations → App Store Connect API → **App Manager** role. You get an Issuer ID, a Key ID, and a `.p8` file **that can only be downloaded once.** | appstoreconnect.apple.com | 5 min |
| **A5** | **Add three repository secrets** (Settings → Secrets and variables → Actions): `APP_STORE_CONNECT_ISSUER_ID`, `APP_STORE_CONNECT_KEY_ID`, `APP_STORE_CONNECT_PRIVATE_KEY`. ⚠ The private key is the **`.p8` contents verbatim**, BEGIN/END lines included — *not* base64. Optionally `APPLE_TEAM_ID` (10 chars); add it if A6 fails with "requires a development team". | github.com repo settings | 5 min |
| **A6** | **Trigger the build**: Actions → **ios** → Run workflow, or push a tag `ios-v0.4.4`. The job archives, signs, uploads, and prints where to look. Without the secrets it skips loudly rather than failing — so a run before A5 tells you nothing is broken, only that it is waiting. | github.com Actions | 15 min |
| **A7** | **Invite testers.** *Internal* (up to 100 App Store Connect users, **no review**, available minutes after processing) is the fast path — use it first. *External* (up to 10,000, needs a one-time Beta App Review, gives you a public link) is the one that produces a shareable URL. | appstoreconnect.apple.com | 10 min |
| **A8** | **Publish the public link.** Once external testing is approved, App Store Connect gives you a `testflight.apple.com/join/…` URL. Search the repo for **`TESTFLIGHT_PUBLIC_LINK_PLACEHOLDER`** — it appears once, in `website/beta/index.html`, currently rendered as a disabled button. Replace that block with a real link (the exact replacement is in the HTML comment beside it) and the site redeploys on merge. Or hand the loop the URL and it will do it. | repo | 5 min |

**What is already true, so you do not need to arrange it:** the tester-facing site is built and
GitHub Pages is enabled — it publishes to **https://aytekxr.github.io/ams-pulse/** the moment this
work merges to `main`. `/privacy/` and `/support/` there are the two URLs App Store Connect will
demand in A3, and `/beta/` is where you point testers. Until A8 that page shows a disabled button
plus a **working manual-invitation route** (email / GitHub issue), so it is honest and usable today.

**Two things worth knowing before A6.** Apple has required **Xcode 26 / iOS 26 SDK** for every
App Store Connect upload since 2026-04-28; CI pins and asserts that, so a build that uploads is a
build Apple accepts. And the build number comes from the CI run number, because App Store Connect
rejects a duplicate build number at upload time — a slow, confusing failure we designed out.

**One judgement call is yours:** `/privacy/` and `/terms/` are legal statements published in your
name. They were written from what the code actually does, not from a template, but **read them
before the site goes public.** Both carry an `OPERATOR REVIEW REQUIRED` marker in the HTML source.

---

## B. Marketplace queue (leverage order)

1. **⚠ Rotate `CLICKHOUSE_PASSWORD` — the hard blocker.** A 32-hex prefix of the live
   48-character production password sits in the **public** repo's git history (since `98b011c`,
   via an old test input). The source is scrubbed, but history cannot be un-published — only
   rotation closes it. Not remotely exploitable today (ClickHouse is Docker-internal and never
   published to the host), but it is 128 bits of the secret in a public repo that anyone can
   find with `git log -S`. **Re-checked 2026-07-28: still un-rotated** (2 commits still carry
   the live prefix). **Rotate before the repo gets marketplace traffic.**

   *Mechanics — no volume surgery (verified against the live container 2026-07-27):* the
   ClickHouse user is defined in `users_xml` and the image entrypoint rewrites it from the
   environment on **every** container start. So: new value into `deploy/.env` → `up -d` with the
   standing five-overlay combo. The **backup sidecar reads the same variables**, so it must be
   recreated in that same `up -d` or it keeps the stale password. Afterwards rotate the
   remaining chat-exposed set (`deploy/.env`, `oguz-testing.md` — already mode 600).

   **Re-checked 2026-07-30: still un-rotated, still 2 commits.** This is now the *only* thing
   between the product and submission — `v0.4.5` is cut, so the "rotate, cut, submit" motion is
   down to "rotate, submit". Nothing in the v0.4.5 release claims or implies rotation happened.

2. **Review `docs/marketplace/listing.md`** — the submission copy. Free of placeholders and
   internal notes, so it is safe to paste verbatim. Override anything; the category and all
   price wording are yours.

3. **Submit the listing** to the Ant Media Marketplace (your account) — paste from
   `listing.md`, never from `listing-draft.md` (internal). **`v0.4.5` is cut and is the submission
   target** — it carries review rounds 7–11 plus the S122 corrections, so the tag an evaluator
   pulls now contains its own security fixes. Artifact index:
   `docs/marketplace/submission-package.md`.

   The three things a reviewer verifies by hand were each run end-to-end against the published
   artifacts rather than assumed, because none of them had ever actually been executed here:
   `cosign verify` with a v3 client **passes** (digest `81673359…`), `helm pull
   oci://ghcr.io/aytekxr/charts/pulse --version 0.3.3` **pulls anonymously**, and the
   `curl … | bash` quickstart URL **resolves 200**. Chart semver is **0.3.3** now, not 0.3.2 —
   the listing and install docs say so.

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

12. *Optional, iOS-adjacent:* **point a domain at the website.** It publishes to
    `aytekxr.github.io/ams-pulse/` with no action from you. If you would rather it lived at
    `beyondkaira.com` or a subdomain, there are two routes: a GitHub Pages custom domain (a DNS
    record plus one repo setting), or serving the same static files from this VPS — the nginx
    vhost is written and waiting at `deploy/nginx/pulse-website.conf` and needs your `sudo`.
    Either way the App Store URLs in A3 change, so decide before A3 if you care.

## Decision-gated engineering (one word each unblocks a build)

- **§2.45** Pulse-native self-alert paging half — maintenance-window semantics plus the
  tier/channels ruling. (The Prometheus half has already shipped.)
- **§2.44 `[FO-1]`** firing-orphan behaviour for node/QoE alerts whose subject vanishes:
  auto-resolve-after-grace (the loop's lean) / stay-firing / leave-as-is.
- ~~**`[ANOM-TIER]`**~~ **RESOLVED S122 — you ruled "advertise correctly", so anomaly detection is
  now Business+ everywhere.** Implemented in the grant-only direction: `CheckAnomalies` admits
  Business, so no tenant loses anything and e2e A5 keeps passing. Two things turned up while doing
  it. The web UI also gated on Enterprise, so an entitled Business tenant would have met an
  "upgrade to Enterprise" wall over data the API was already serving — fixed, with the Pro floor
  now pinned by tests on both sides. And the note above that said *"the new website deliberately
  does not state anomaly detection's tier at all"* **was wrong**: the site said `F9 - ENTERPRISE`
  in the feature card *and* listed anomaly detection under Enterprise in the pricing table. Both
  corrected. Nothing is outstanding for you here; the tier table in item 3 is safe to paste.
- **Dependabot queue** (17 PRs open, operator-held): confirm the hold, or authorise a
  batch-absorb session per `docs/dependabot-policy.md`.

## Standing meeting items (A-ledger)

Bring to the Ant Media developer meeting: listing format/category confirmation (A2/A10), asset
specs (A3), post-year-1 revenue terms **in writing** (A4), review flow/SLA (A5), trial
expectations (A6), AMS version-support expectations (A7), docs-linking policy (A8), and
load-evidence format (A9). Details: `docs/marketplace/submission-process.md`.

---

*Prod: healthy and untouched — v0.4.0-139, all three `/healthz` components `ok`, **1,355,548**
server events, newest 2 s old at the check, collector actively ingesting. A prod roll is item 11,
never automatic.*

*On item 1: external review rounds 7 through 11 have each landed fixes on `main` that are not in
the `v0.4.4` tag. Five are behavioural — the geo/device breakdown row cap, the three D-184
security/enforcement fixes, and D-185's AMS-poll SSRF guard plus the `/healthz` fix that stops an
AMS error body being republished to unauthenticated callers. When you rotate, the same sitting
authorises one motion: **rotate, cut `v0.4.5`, submit against it**.*

*Noticed while probing your AMS: its licence shows `type: trial`, `endDate 2026-07-27` — i.e.
**expired as of 2026-07-28**. It affects nothing we ship, but it does affect future live
validation against that instance, including item 6's load lane.*
