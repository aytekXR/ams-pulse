# Operator expected — Ant Media Marketplace submission

Only what needs a human: decisions, legal or business facts, credentials, and actions outside
the repository. Everything else in this package is finished and verified (2026-10-01).

**Blocking** = Ant Media cannot build or publish the page without it.
Placeholders in the package use the marker names shown here — search for them to fill them in.

---

## 1. Required from the operator (you)

### 1.1 [BLOCKING] Developer / legal entity name — `PENDING_DEVELOPER_LEGAL_ENTITY`

- **What is missing:** the exact name to credit as developer. Ant Media's draft says "Only an
  individual developer is named so far; the page needs a 'Developed by ___' credit."
- **What the repository says today:** `Copyright (c) 2026 Aytek Erdoğan (beyondkaira.com)` in
  `LICENSE` and `docs/licensing-public.md`. No company name appears anywhere in the repository.
  The task brief mentions "Beyond Technologies"; that name is not in the repository, so it was
  **not** used.
- **Where it is used:** the page intro ("developed by …") and footer credit;
  `marketplace-copy.md`; `answers-for-ant-media.md`. If it is a company, `LICENSE`, the
  licensing doc and the website footer should probably match it too.
- **Format:** the exact registered name including its legal form, as on official documents
  (for example `<Name> Ltd.`, `<Name> A.Ş.`, `<Name> GmbH`), with the country of registration —
  or explicit confirmation that the individual name above is the credit.
- **Blocks submission:** yes.

### 1.2 [BLOCKING] Pricing confirmation — `PENDING_OPERATOR_PRICE_CONFIRMATION`

- **What is missing:** your confirmation of the prices to publish. A launch price list exists
  (`docs/licensing-public.md` §2 and §4, decision D-169). It was set on your behalf and marked
  "subject to operator override": Free $0 (noncommercial use only), Pro $99/month, Business
  $299/month, Enterprise from $799/month, annual = 10× monthly, a "Founding Operators" first-year
  offer (Pro $9, Business $29, Enterprise 90-day pilot + 25% off), and a 14-day Pro trial.
  The public website deliberately shows no prices ("contact for current rates").
- **Why:** Ant Media's draft says "ams-pulse's own pricing hasn't been shared".
- **Where it is used:** the pricing section of `marketplace-copy.md`; the `Get Pulse` page
  (`website/get/index.html`, comment `PRICING_PENDING_OPERATOR_INPUT`); optionally the landing
  page.
- **Format:** per tier — price, currency, billing period, and node/retention limits if they
  change; whether the launch offer and the trial are on; whether prices may be **public** (the
  page and site) or are "contact us".
- **Blocks submission:** yes, for the pricing section.

### 1.3 [BLOCKING] Purchase link for the "Get ams-pulse" button — `PENDING_PURCHASE_URL`

- **What is missing:** a URL. No checkout or purchase page exists; licenses and trial keys are
  issued by e-mail today (support@beyondkaira.com).
- **Options (pick one):**
  a. `https://aytekxr.github.io/ams-pulse/get/` — the "Get Pulse" page added in this package
     (install steps, tiers, trial/license by e-mail). Goes live when this work merges to `main`.
  b. A checkout page you create (Stripe, Paddle, Lemon Squeezy, …).
  c. Ant Media's own marketplace checkout — depends on their commission terms (§2.4).
- **Where it is used:** hero and closing-CTA buttons on Ant Media's page.
- **Format:** one HTTPS URL that is already live.
- **Blocks submission:** yes.

### 1.4 [STRONGLY RECOMMENDED before going live] Decide on the analytics accuracy defects D1–D4

- **What:** the audit found that audience analytics shows zeros (D1). Rollups would overstate
  views and watch time (D2). Usage-report viewer-minutes are overstated about 10× for SDK
  traffic (D3). QoE rebuffer and error ratios are understated by a similar factor (D4).
  Evidence and root causes: `marketplace/submission-notes.md` §4.
- **Why it matters:** an Ant Media reviewer who installs Pulse and adds the player SDK will
  see an empty Analytics page and wrong QoE/billing figures. The current listing copy also
  claims "viewer-minutes reconciled to ±1%".
- **Decision needed:** (a) fix before the listing goes live — recommended. It is one data-model
  change (count distinct sessions, aggregate per-heartbeat deltas, re-backfill rollups) plus
  tests, then a release. Or (b) list now: keep the analytics, QoE and usage pages out of
  marketing (already done in this package) and disclose them as known limitations.
- **Blocks submission:** not the page build; it does block an honest "billing-grade" claim.

### 1.5 [BLOCKING for screenshot fidelity] Merge this work and release v0.4.6

- **Why:** the installer fixes go live for `curl | bash` users as soon as `install.sh` is on
  `main`. The sign-in fix (a new user no longer sees "Session expired") only reaches users in a
  new image. The quickstart pins `0.4.5`, so the step-4 screenshot shows behaviour v0.4.5 lacks
  until v0.4.6 ships.
- **What:** review and merge the branch/PR containing this session's changes (list in
  `marketplace/submission-notes.md` §3), then tag `v0.4.6` (the release workflow builds, signs,
  scans and publishes the image and chart). Bump the quickstart/Helm pins as usual.
- **Blocks submission:** blocks the walkthrough screenshots from matching what users install.

### 1.6 Send the answers and corrections to Ant Media

- `marketplace/answers-for-ant-media.md` is ready to send (fill in §1.1–1.3 first), with
  `assets/`. Correct one statement from the July e-mail to Ankush: it said viewer IPs are
  "SHA-256 hashed". In fact Pulse **never stores** them, which is a stronger guarantee.
- **Blocks submission:** yes — Ant Media's draft currently contains a failing install command.

---

## 2. Required from Ant Media

| # | Item | Why we need it | Where it is used | Format expected | Blocks submission? | Status |
|---|---|---|---|---|---|---|
| 2.1 | Marketplace submission requirements / qualification checklist | Defines "done"; no public checklist exists | Our checklist (`marketplace/submission-checklist.md`) | Their document or e-mail | **Yes** — we cannot prove readiness without it | Pending — they are checking |
| 2.2 | Review timeline | Plan the v0.4.6 release and the D1–D4 fix around it | Release planning | Dates or SLA | No | Pending — they are checking |
| 2.3 | Load-testing expectations: tool, scenario, thresholds, evidence format | Our load harness (`qa/realams/load/`, supports their official tools) needs a target; it also needs a dedicated, licensed AMS instance | Capacity claim on the page; qualification | Written spec (e.g. N publishers / M viewers, pass criteria, report format) | **Likely** — if load evidence is a qualification step | Pending — they are checking |
| 2.4 | Commission terms, **in writing**, including after year one | Pricing (§1.2) and purchase link (§1.3) depend on it | Pricing; vendor agreement | Written terms / agreement | **Yes** for paid listing | Pending — they are checking. Our July research found "first year: 100% to vendor"; post-year-one unknown |
| 2.5 | Whether their checkout sells Pulse (vs a link to us) | Decides §1.3 | "Get ams-pulse" button | Yes/no + checkout process | **Yes** (decides the button target) | Not yet asked |
| 2.6 | Image specs (sizes, aspect, format, count) and whether they want SVG | We delivered 1920×1080 PNGs, a 4K hero, 1200×630 social cards, SVG + PNG logos | Page build | Pixel sizes / formats | No (defaults supplied) | Not yet asked |
| 2.7 | Whether linking to GitHub docs is acceptable or documents must be uploaded | We provided PDFs as well | "View Documentation" button | Policy answer | No | Not yet asked |
| 2.8 | Who builds and publishes the page, and whether we review it before it goes live | Keep the draft's inaccuracies off the live page | Page publication | Process answer | No, but strongly advised | Not yet asked |

---

## 3. Optional improvements (not blocking)

| # | Improvement | Effort |
|---|---|---|
| 3.1 | Alerting UX: put the affected stream and a dashboard link in e-mails (D7); merge channel config on update so UI edits keep SMTP settings, or add SMTP fields to the form (D6); show rule names and streams in History (D7); keep wildcard *stream offline* alerts firing until the stream returns (D8) | S–M each |
| 3.2 | Cosmetic fixes: round sub-1 Mbps bitrates (D9), "Unlimited" instead of `null` on the License tab (D11), show env-configured AMS on Settings → Sources (D12), re-tune the health score (D10), refresh the sidebar tier label and trial banner right after the first sign-in (D14) | S |
| 3.3 | Validate the Helm chart on a real cluster before advertising Kubernetes | M |
| 3.4 | Live re-validation on AMS **3.1.0** (`antmedia/enterprise:3.1.0` needs a license; `antmedia/community` is pullable but only 2.14.0) | M |
| 3.5 | Light-theme screenshot variants, if Ant Media's page uses light sections | S (script supports `--theme light`) |
| 3.6 | Re-record the demo video over the current UI | M |
| 3.7 | Custom domain for the site (e.g. `pulse.beyondkaira.com` or `get.…`) and self-hosted IBM Plex fonts for the website | S |
| 3.8 | `NPM_TOKEN` repository secret so `ams-pulse-beacon` publishes to npm on release | XS |

---

## 4. Questions / unresolved items

1. **Is "Beyond Technologies" your registered entity?** It is in the task brief but nowhere in
   the repository. If yes, give the exact legal form (§1.1).
2. **Page title:** "Pulse" (the brand on the logo and in the UI) or "ams-pulse" (the repository
   and package name, used throughout Ant Media's draft)? Marker: `PENDING_PRODUCT_NAME_CHOICE`.
   Suggestion: "Pulse for Ant Media Server", with `ams-pulse` in commands.
3. **Free tier on a commercial marketplace:** Free is noncommercial-only (PolyForm Noncommercial).
   Most AMS customers are commercial, so their path is trial → paid. Should the page lead with the
   14-day trial rather than "start free"?
4. **Trial delivery:** keys are issued manually by e-mail (about one business day). Acceptable
   for marketplace traffic, or do you want self-serve trial keys?
5. **Support promises:** the copy repeats your published response targets (Pro 2 business days,
   Business 1, Enterprise 4 hours). Confirm you can honour them as listed.
6. **Show Helm on the page?** Recommendation: no, until it has been validated on a cluster (§3.3).
7. **Launch offer:** should Ant Media's page mention the "Founding Operators" first-year prices?

---

## 5. Exact files or assets you must provide

| # | What | Format | Goes into |
|---|---|---|---|
| 5.1 | Legal entity name (§1.1) | Plain text, exact legal form + country | `marketplace-copy.md`, `answers-for-ant-media.md`, possibly `LICENSE` |
| 5.2 | Confirmed price table (§1.2) | Table: tier, price, currency, period; launch offer y/n; public y/n | `marketplace-copy.md` pricing, `website/get/index.html` |
| 5.3 | Purchase URL (§1.3) | One live HTTPS URL | Hero + closing CTA |
| 5.4 | **Company logo — only if** the "Developed by" credit should show one (Ant Media's draft notes the Scotty page credits its developer with a logo). The product logo is supplied; no company logo exists in the repository and none was invented. | SVG preferred, plus PNG with transparent background | Ant Media page footer credit |
| 5.5 | Optional: contact name/title for partner communications | Text | Ant Media partner records |

---

## 6. Exact actions you must perform

1. **Review** `marketplace/marketplace-copy.md`, `answers-for-ant-media.md`, `draft-review.md`
   and the `assets/` folders.
2. **Decide and fill in** §1.1, §1.2, §1.3 and §4.2. Search the package for `PENDING_` and
   replace every marker, or send the answers back for them to be applied.
3. **Merge** the branch with this session's changes (repository, `main`); GitHub Pages then
   publishes `/get/` and the website corrections automatically.
4. **Tag `v0.4.6`** after merging (§1.5); confirm the release workflow is green (image signed,
   Trivy clean, chart published).
5. **Send** Ant Media `answers-for-ant-media.md` + the `assets/` folder (or this ZIP without
   `internal/` and `operator-expected.md`), including the corrections to their draft.
6. **Decide on D1–D4** (§1.4) and tell Ant Media whether analytics features will be presented at
   launch.
7. **Delete the vulnerable GHCR image** `candidate-5c561bc4` (package version id `1080500729` —
   still published on 2026-10-01; it predates the CVE-2026-56852 fix): GitHub → Packages →
   ams-pulse → that version → Delete. Delete **only** that version — the other `candidate-*` tags
   share digests with released versions.
8. **Restore a reachable, licensed AMS** if Ant Media expects a live demo or re-validation. Your
   `antmedia` container has been unreachable from the host since 2026-08-12 (bridge mode, no
   published ports), and its trial license expired on 2026-07-28. The public dashboard vhost
   `pulse.beyondkaira.com` is also disabled. Details: `docs/operator-expected.md` §0 in the
   repository.
9. **Read `/privacy/` and `/terms/`** on the website before marketing traffic arrives. They are
   legal statements published in your name.
