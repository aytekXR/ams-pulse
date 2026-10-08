# Operator TODO — the items only YOU can do

*Open items only. No history, no "what changed", no loop-owned work — those live in
`agents/handoffs/decisions.md`, `agents/handoffs/sessions/` and
`agents/handoffs/RESUME-PROMPT.md`.*

> **v0.5.0 is released and in production (2026-10-07).** Pulse is free; your decisions (developer
> credit, no prices, no purchase link, the page title, D1–D4 to the roadmap) are applied
> everywhere, and the Ant Media package is ready to send. Prod runs v0.5.0 and ingests from your
> AMS 3.1.0 Enterprise; `https://pulse.beyondkaira.com` is live again (vhost re-enabled
> 2026-10-08). The Ant Media materials were sent on 2026-10-07. What is left for you is §0 and
> §B1; iOS (§A) is independent.

---

## 0. Do soon

1. **Renew the AMS license before 2026-10-16.** The `antmedia` container (AMS 3.1.0 Enterprise)
   runs on the trial key that expires **2026-10-16 09:29 UTC**. Apply the new key through the run
   command's `-l` argument — `start.sh` blanks the `LICENSE_KEY` environment variable. When the
   key lapses, prod's collector goes blind again.

## A. iOS TestFlight — the critical path

**Everything on our side is built and verified.** The app compiles for iOS 26 under Swift 6
strict concurrency on a real GitHub macOS runner, 296 Linux tests and 50 simulator tests pass,
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
| **A6** | **Trigger the build**: Actions → **ios** → Run workflow, or push a tag `ios-v0.5.0`. The job archives, signs, uploads, and prints where to look. Without the secrets it skips loudly rather than failing — so a run before A5 tells you nothing is broken, only that it is waiting. | github.com Actions | 15 min |
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

## B. Marketplace

1. **Read `/privacy/` and `/terms/`** on the website — legal statements published in your name.
2. **Ant Media's answers** — submission requirements, review timeline, load-test format and
   thresholds, their terms for a free listing (the package's `operator-expected.md` §2; the
   A-ledger in `docs/marketplace/submission-process.md` §2). Bring to the developer meeting:
   listing format and category (A2/A10), asset specs (A3), review flow and SLA (A5), AMS
   version-support expectations (A7), docs-linking policy (A8), load-evidence format (A9).
3. *Optional:*
   - **`GHCR_CLEANUP_TOKEN`** repo secret (a PAT with `delete:packages`), so a failed release
     run removes its own `candidate-<sha>` image instead of leaving it public (as v0.4.5's did —
     deleted by hand on 2026-10-08).
   - **`deploy/.env.bak.20260731T112701Z`** still holds the pre-rotation ClickHouse secret
     (mode 600): `shred -u` it when you are satisfied. The other chat-exposed credentials in
     `deploy/.env` and `oguz-testing.md` were never rotated — only ClickHouse was.
   - **Load lane on a PAYG AMS** — the real capacity number for the listing; with
     `server.kafka_brokers` set it also runs AV-15 (Kafka), and as a **2-node cluster** it lets
     LIM-10 be fixed or proven instead of disclosed.
   - **`NPM_TOKEN`** repo secret, so `ams-pulse-beacon` publishes to npm on release.
   - **Re-record the demo video** over the current UI (re-read `docs/marketplace/demo-video-script.md` first).
   - **A custom domain for the website** (GitHub Pages custom domain, or serve it from this VPS —
     the vhost is written at `deploy/nginx/pulse-website.conf` and needs your `sudo`). Decide
     before A3 if you care: the App Store URLs follow it.
   - **Ant Media's Documentation-V2 feedback form** (their 2026-09 e-mail, 2–3 minutes) — honest
     notes: the REST statistics endpoints need one consolidated reference page; the webhooks page
     documents no payload authentication (`docs/assessment/ams-3.1-docs-v2-assessment.md`).

## Decision-gated engineering (one word each unblocks a build)

- **§2.45** Pulse-native self-alert ("Pulse collector offline") — the **maintenance-window
  semantics** ruling. (The channels-per-tier half is moot since v0.5.0: every channel is free.)
  Prod has gone blind twice without paging anyone; this is the alert that would have.
- **§2.44 `[FO-1]`** firing-orphan behaviour for node/QoE alerts whose subject vanishes:
  auto-resolve-after-grace (the loop's lean) / stay-firing / leave-as-is.
- **Dependabot queue** (operator-held): confirm the hold, or authorise a batch-absorb session per
  `docs/dependabot-policy.md`. v0.5.0 absorbed only the CVE-relevant subset (Go 1.26.8,
  `golang.org/x/crypto` v0.55.0, Alpine 3.24.2).

---

*Prod (2026-10-08): **v0.5.0** (`8523b47`), every `/healthz` component `ok`, ingesting from AMS
3.1.0 Enterprise, public at `https://pulse.beyondkaira.com` (`/metrics` and the API need tokens;
unsigned webhooks get 401); rollback image `pulse-prod-pulse:pre-d194` (= v0.4.5-9).*
