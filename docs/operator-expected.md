# Operator TODO — the items only YOU can do

*Open items only. No history, no "what changed", no loop-owned work — those live in
`agents/handoffs/decisions.md`, `agents/handoffs/sessions/` and
`agents/handoffs/RESUME-PROMPT.md`.*

> **✅ THE MARKETPLACE BLOCKER IS GONE.** `CLICKHOUSE_PASSWORD` was rotated on 2026-07-31 and
> verified: `git log -S` on the new value returns **0 commits across all refs**. The two commits
> that carried the old prefix now hold a dead password. **Nothing technical stands between
> Pulse and submission** — item 1 below is now the submission itself.
>
> **iOS TestFlight** is still blocked by item A (Apple Developer Program enrolment) and nothing
> else. The two tracks are independent.
>
> **Prod:** healthy, on the same pinned v0.4.0-139 build as before. ⚠ The rotation's stack
> recreate exposed a real landmine and caused a **5-minute ingest outage** (11:27–11:32 UTC,
> since recovered): `pulse-migrate` bind-mounts `contracts/` from the working tree, so **any
> `docker compose up -d` on prod applies migrations from the git checkout to whatever binary is
> deployed.** Details and the pre-flight check: `deploy/runbooks/upgrade-rollback.md` §1.
> This makes item 11 (roll prod forward) more attractive than it was — it would close the gap
> permanently.

---

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
| **A6** | **Trigger the build**: Actions → **ios** → Run workflow, or push a tag `ios-v0.4.5`. The job archives, signs, uploads, and prints where to look. Without the secrets it skips loudly rather than failing — so a run before A5 tells you nothing is broken, only that it is waiting. | github.com Actions | 15 min |
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

1. ~~**Rotate `CLICKHOUSE_PASSWORD`**~~ **✅ DONE 2026-07-31.** Rotated with
   `deploy/scripts/rotate-clickhouse-password.sh`, which backs up `deploy/.env`, recreates every
   consumer (including the backup sidecar — the classic miss), then verifies the new credential
   works, **the old one is REJECTED**, the row count did not go backwards, all three `/healthz`
   components are `ok`, and the sidecar carries the new value. It rolls back automatically on any
   failure.

   **Verification, without printing the secret:** `git log -S` on the new value's 32-char prefix
   returns **0 commits across all refs**. The old prefix still appears in 2 commits — history
   cannot be un-published, which is precisely why rotation was the only fix — but that value is
   now dead.

   **Two things you should still do:**
   - The plaintext backup of the previous env file is at `deploy/.env.bak.20260731T112701Z`
     (mode 600, gitignored). Once you are satisfied: `shred -u deploy/.env.bak.20260731T112701Z`.
   - The *other* chat-exposed credentials in `deploy/.env` and `oguz-testing.md` were **not**
     rotated — only ClickHouse was. Rotate the rest when convenient.

   ⚠ **What this rotation uncovered, which matters more than the rotation.** Recreating the
   stack applied `0011_server_events_ingest_error.sql` from the working tree to the pinned
   v0.4.0-139 binary, took `server_events` from 40 to 42 columns, and **dropped ingest for five
   minutes** (`expected 42 arguments, got 40`). Recovered by dropping the two columns and clearing
   the ledger row; prod is back on exactly the build and schema it had before. The cause is
   structural: **`pulse-migrate` bind-mounts `contracts/` from the host repo, so prod's schema
   follows the git checkout rather than the deployed image.** The rotation script now refuses when
   the tree holds migrations the deployed binary predates, and
   `deploy/runbooks/upgrade-rollback.md` §1 carries the pre-flight check for any other prod
   `up -d`. Rolling prod forward (item 11) closes the gap for good.

2. **Review `docs/marketplace/listing.md`** — the submission copy. Free of placeholders and
   internal notes, so it is safe to paste verbatim. Override anything; the category and all
   price wording are yours.

3. **Submit the listing** to the Ant Media Marketplace (your account) — paste from
   `listing.md`, never from `listing-draft.md` (internal). **`v0.4.5` is RELEASED and is the
   submission target** — it carries review rounds 7–11 plus the S122 corrections, so the tag an evaluator
   pulls now contains its own security fixes. Artifact index:
   `docs/marketplace/submission-package.md`.

   Everything a reviewer checks by hand was executed against the **published v0.4.5 artifacts**,
   not assumed: `cosign verify` with a v3 client **passes** (digest `542fead1…`), `helm pull
   oci://ghcr.io/aytekxr/charts/pulse --version 0.3.3` **pulls anonymously**, the
   `curl … | bash` quickstart URL **resolves 200**, and a Trivy scan of the released image returns
   **0 HIGH/CRITICAL** (Alpine 0, Go binary 0). Chart semver is **0.3.3** now, not 0.3.2 — the
   listing and install docs say so.

   One thing to have ready if their reviewer asks about CVEs: the release pipeline **blocked
   v0.4.5 once** on a HIGH finding (CVE-2026-56852 in `golang.org/x/text`, an indirect dependency)
   and the release only went out after it was patched. That is a good story, not a bad one — it
   demonstrates the gate is real — and `CHANGELOG.md` records it under 0.4.5 Security.

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

8. **Delete exactly ONE GHCR tag — and do NOT delete the other four.** ⚠ The previous version of
   this item told you to delete "the four `candidate-*` tags by hand in the GHCR package UI".
   **Following that would have deleted the v0.4.5 release.** A GHCR *package version* is a
   manifest digest, not a tag, and the UI deletes versions. Four of the five `candidate-*` tags
   ride the **same digest as a release tag**, so deleting them deletes the release — along with
   its SBOM, provenance and cosign signature. `release.yml` has always known this (its cleanup
   step refuses to delete a multi-tag digest); only this doc was wrong.

   Verified against the live package on 2026-07-31:

   | Version id | Tags on that digest | Action |
   |---|---|---|
   | `1080500729` | `candidate-5c561bc4` **only** | **DELETE — this is the vulnerable one** |
   | `1080581868` | `0.4.5`, `latest`, `0.4`, `0`, `candidate-7d522596` | **DO NOT DELETE** |
   | `1069926970` | `0.4.4`, `candidate-34a25fc4` | **DO NOT DELETE** |
   | `1068860998` | `0.4.3`, `candidate-669952ed` | **DO NOT DELETE** |
   | `1068283272` | `0.4.2`, `candidate-e318a053` | **DO NOT DELETE** |

   Only `candidate-5c561bc4` is a standalone image, and it is the one that matters: it was built
   from commit `5c561bc4`, which `git merge-base --is-ancestor 5c561bc4 7d52259` confirms predates
   the CVE-2026-56852 fix. It is publicly pullable and carries the HIGH CVE. The other four are
   harmless aliases — pulling one yields byte-identically the released image.

   **Do it in the web UI** (GitHub → Packages → ams-pulse → versions → the version whose only tag
   is `candidate-5c561bc4` → Delete). This cannot be automated from here: the session token holds
   `read:packages`, not `delete:packages` (re-probed 2026-07-31), so the loop cannot do it for you.

   **To stop it recurring**, add a **`GHCR_CLEANUP_TOKEN`** repo secret (a PAT with
   `delete:packages`). `release.yml` already has the cleanup step wired and correctly guarded — it
   deletes a quarantine image only when the candidate tag is the *sole* tag on the digest, i.e.
   only on the failed-release path. Without the secret that step warns loudly and no-ops, which is
   exactly what happened here. The complementary loop-owned fix (round-6 H-09, buildx
   `push-by-digest=true`) remains deliberately deferred until after submission — it changes the
   publish mechanism and cannot be exercised by the dry-run path, only by a real tag.

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

13. **⚠ Four open HIGH CodeQL alerts are visible in your PUBLIC repo's Security tab** — and
    they have been open since **2026-07-09/10**, roughly three weeks, with every CI gate green
    the whole time. A marketplace security reviewer will open that tab. **None was introduced by
    the S123 work**; they are pre-existing on `main`.

    **Why nothing caught them:** the required contexts include `Analyze (go)` and
    `Analyze (javascript-typescript)`, which report whether the *scan ran*, not whether it
    *found anything*. The aggregate `CodeQL` check-run — the one that says "2 new alerts
    including 1 high severity" — is **not** a required context. Same class as the three gate
    gaps closed in S123: a guard that cannot fail.

    | Alert | Location | Assessment |
    |---|---|---|
    | `go/weak-sensitive-data-hashing` | `server/internal/store/meta/meta.go:1649` | **Conditionally real.** SHA-256 turns `PULSE_SECRET_KEY` into the AES key that encrypts stored AMS credentials. Fine for the documented `openssl rand -hex 32` value (and the hex path skips SHA-256 entirely), weak if an operator sets a short guessable string. ⚠ **Do NOT "fix" this by switching to a KDF without a migration** — it would change the derived key and make every already-encrypted credential undecryptable. |
    | `go/weak-sensitive-data-hashing` | `server/internal/api/oidc.go:124` | **Effectively a false positive.** Derives an HMAC key for OIDC state cookies from the same high-entropy secret. The "use a slow hash" rule targets low-entropy user passwords. Safe to switch to HKDF if you want it silenced — changing it only invalidates in-flight state cookies. |
    | `js/insecure-randomness` ×2 | `sdk/beacon-js/src/index.ts:35,54` | **Low but genuine.** `Math.random()` generates beacon session IDs. They are analytics correlation IDs, not auth tokens, and the ingest endpoint requires a token — but guessable IDs let someone attribute events to another session. `crypto.getRandomValues` with a `Math.random` fallback is a small, safe change; it needs an SDK re-release and a re-run of the 15 KB size gate. |

    **Your call, three options:** fix them (only the SDK one is risk-free), dismiss each in the
    Security tab with the rationale above (GitHub records the reason, which is a *better* look to
    a reviewer than four untriaged alerts), or leave them. **Doing nothing is the only option
    that looks bad**, because the tab shows "4 open" with no explanation.

    **Also worth doing either way:** add `CodeQL` to the required contexts in
    `.github/branch-protection.sh` so new alerts cannot sit unnoticed again. It was deliberately
    NOT added in S123 because it would have blocked that PR on these pre-existing alerts — triage
    them first, then require it.

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

*Prod: healthy — v0.4.0-139 (unchanged), all three `/healthz` components `ok`, **1,400,267**
server events, newest 5 s old at the check, collector actively ingesting, 0 errors in the
preceding 90 s. It survived a 5-minute ingest outage during the rotation (see item 1) and is back
on exactly the build and schema it had before. A prod roll is item 11, never automatic.*

*Noticed while probing your AMS: its licence shows `type: trial`, `endDate 2026-07-27` — i.e.
**expired as of 2026-07-28**. It affects nothing we ship, but it does affect future live
validation against that instance, including item 6's load lane.*

*Tier packaging ruling you gave on 2026-07-31, now enforced everywhere: **ingest health (F4) is
Pro+, not Free.** The server had gated it at Pro+ since a deliberate fix ("was leaking to Free"),
while `docs/product.md`, `docs/overview.md` and the website all advertised it as Free — wrong
together, so no cross-check caught it. The website's pricing table was selling both F4 **and**
historical analytics (F2) inside the Free plan; a Free user following it hit `403
LICENSE_REQUIRED` on both. All ten features now agree across code, docs and site.*
