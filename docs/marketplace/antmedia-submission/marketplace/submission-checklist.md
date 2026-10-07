# Ant Media Marketplace — submission checklist

Status on 2026-10-07. Owner: **Dev** = the developer (operator), **AM** = Ant Media,
**Done** = completed and verified in this package.

## A. What Ant Media asked for

| # | Item | Status | Owner | Where |
|---|---|---|---|---|
| 1 | Logo / brand mark | **Done** | — | `assets/branding/` |
| 2 | Dashboard screenshot | **Done** | — | `assets/screenshots/01-live-dashboard.png`, `assets/hero/` |
| 2 | Alert-in-action screenshot | **Done** | — | `assets/hero/pulse-hero-alert-1920x1080.png`, `screenshots/02–04, 08` |
| 2 | Hero image | **Done** | — | `assets/hero/` |
| 2 | Install walkthrough screenshots | **Done** | — | `assets/walkthrough/` |
| 3 | Developer credit for "Developed by" | **Done** | — | Aytekin Erdogan (individual developer, beyondkaira.com) |
| 4 | Pricing | **Done** | — | Free (every feature on every install, no license key) |
| 4 | Page title | **Done** | — | "Pulse for Ant Media Server" |
| 4 | Main button link | **Done** | — | https://aytekxr.github.io/ams-pulse/get/ (no purchase link — Pulse is free) |
| 5 | Dashboard URL / port | **Done** | — | `answers-for-ant-media.md` §5, `dashboard-access.md` |

## B. Draft page corrections

| Item | Status | Where |
|---|---|---|
| Claim-by-claim review of the draft | **Done** | `draft-review.md` |
| Corrected page copy in the draft's structure | **Done** | `marketplace-copy.md` |
| Install step 1 command (missing `--password`) | **Corrected** | `marketplace-copy.md` step 1 |
| Helm step (experimental; incomplete command) | **Recommend removal** | `draft-review.md` S2 |
| Send the corrections to Ant Media | **Pending** | Dev — `answers-for-ant-media.md` is ready to send |

## C. Things Ant Media is checking

| Item | Status | Owner |
|---|---|---|
| Marketplace submission requirements / qualification steps | Pending | AM |
| Review timeline | Pending | AM |
| Load-testing expectations (format, thresholds) | Pending — harness ready to run once known | AM → Dev |
| Their terms for a free listing | Pending | AM |
| Whether the listing links to our docs or needs uploads | Pending | AM |

## D. Product readiness

| Item | Status | Owner |
|---|---|---|
| Build, tests and lint green on the final tree | See `submission-notes.md` §1 | — |
| Published installer works end to end | **Done** (exit 0, 73 s) | — |
| Alert delivery (e-mail + signed webhook) end to end | **Done** | — |
| Defects D1–D4 (analytics, usage, QoE accuracy) | **Decided:** list now, disclosed as LIM-30; fix scheduled next (ROADMAP-V2 §2.49) | Dev |
| Defects D6–D8 (alerting UX) | Open | Dev |
| Release **v0.5.0** with the free-tier changes so install = screenshots | **Done** — 2026-10-07: multi-arch image signed (cosign verify passes), Trivy clean, chart 0.4.0; walkthrough retaken from it | — |
| Live re-validation against a real AMS | **Done** — AMS 3.1.0 Enterprise, 2026-10-07 (`docs/compatibility.md`) | — |
| Capacity number from the load lane | Pending (needs a dedicated AMS) | Dev |

## E. Publishing and hygiene

| Item | Status | Owner |
|---|---|---|
| Product site live (`https://aytekxr.github.io/ams-pulse/`) | **Done** (HTTP 200) | — |
| "Get Pulse" page + website fixes | **Done** — live at `https://aytekxr.github.io/ams-pulse/get/` (HTTP 200, 2026-10-07) | — |
| No secrets in the package | **Done** — scanned, see submission notes | — |
| Vulnerable `candidate-5c561bc4` GHCR image deleted | Pending — needs the `delete:packages` scope or the web UI | Operator |
| Review `/privacy/` and `/terms/` (legal statements in your name) | Pending | Operator |
