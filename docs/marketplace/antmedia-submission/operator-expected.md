# Operator expected — Ant Media Marketplace submission

Only what still needs a human: actions outside the repository, and answers only Ant Media can
give. Everything else in this package is finished and verified (2026-10-07, Pulse v0.5.0).
Decided on 2026-10-07 and already applied everywhere: developer credit **Aytekin Erdogan**
(individual), **free** (every feature, no license key, commercial use included — PolyForm
Shield 1.0.0, SDKs MIT), **no purchase link** (the main button points at
`https://aytekxr.github.io/ams-pulse/get/`), page title **"Pulse for Ant Media Server"**, the
analytics accuracy defects D1–D4 disclosed as LIM-30 and scheduled next (ROADMAP-V2 §2.49).

---

## 1. Required from you

### 1.1 Send the materials to Ant Media

Send `dist/pulse-for-ant-media-server-marketplace-materials.zip` (build it with
`bash qa/marketplace/build-submission-zip.sh ant-media`; it contains only the shareable files —
start with its `README.md`). It answers their five open items and corrects their draft, whose
install command fails as written.

### 1.2 Confirm the spelling of your name

The package, `LICENSE` and the SDK licenses credit **Aytekin Erdogan**, as you wrote it (the
website names no developer). Earlier versions of `LICENSE` read "Aytek Erdoğan". If you want the Turkish "ğ" (or
the short first name) on the page, say so before sending — it is a one-line change in each place.

### 1.3 Renew the AMS license before 2026-10-16

The AMS 3.1.0 Enterprise instance on this VPS runs on a trial key that expires on
**2026-10-16**. Production Pulse polls that instance, and live demos or re-validation need it.
Renew or replace the key (it is applied through the AMS run command's `-l` argument, not the
`LICENSE_KEY` environment variable).

### 1.4 Delete one vulnerable image version on GHCR

`ghcr.io/aytekxr/ams-pulse:candidate-5c561bc4` (package version id `1080500729`) is left over
from the v0.4.5 release run that Trivy blocked; it predates the CVE-2026-56852 fix. The
repository token cannot delete packages (it lacks `delete:packages`; verified 2026-10-07).
Either delete it in the web UI (GitHub → Packages → ams-pulse → that version → Delete — only
that version; other `candidate-*` tags share digests with released versions), or run
`gh auth refresh -h github.com -s delete:packages` once and ask the next session to delete it.

### 1.5 Read `/privacy/` and `/terms/` on the website

They are legal statements published in your name. Read them before marketing traffic arrives.

---

## 2. Required from Ant Media

| # | Item | Why we need it | Format expected | Blocks submission? | Status |
|---|---|---|---|---|---|
| 2.1 | Marketplace submission requirements / qualification checklist | Defines "done"; no public checklist exists | Their document or e-mail | **Yes** | Pending — they are checking |
| 2.2 | Review timeline | Plan the D1–D4 release (§2.49) around it | Dates or SLA | No | Pending — they are checking |
| 2.3 | Load-testing expectations: tool, scenario, thresholds, evidence format | Our load harness (`qa/realams/load/`) needs a target and a dedicated, licensed AMS | Written spec (N publishers / M viewers, pass criteria, report format) | **Likely**, if load evidence is a qualification step | Pending — they are checking |
| 2.4 | Their terms for a free listing | Pulse is free, so commission no longer drives pricing; the vendor agreement may still need it | Written terms | No | Pending — they are checking |
| 2.5 | Image specs (sizes, aspect, format, count); SVG wanted? | We delivered 1920×1080 PNGs, a 4K hero, 1200×630 social cards, SVG + PNG logos | Pixel sizes / formats | No (defaults supplied) | Not yet asked |
| 2.6 | Is linking to GitHub docs acceptable, or must documents be uploaded? | We provided PDFs as well | Policy answer | No | Not yet asked |
| 2.7 | Who builds and publishes the page; can we review it before it goes live? | Keep the draft's inaccuracies off the live page | Process answer | No, but strongly advised | Not yet asked |

---

## 3. Optional improvements (not blocking)

| # | Improvement | Effort |
|---|---|---|
| 3.1 | Fix the analytics accuracy defects D1–D4 (LIM-30) — one data-model change plus a backfill; scheduled next (ROADMAP-V2 §2.49) | M |
| 3.2 | Alerting UX: the affected stream and a dashboard link in e-mails (D7); keep SMTP settings when a channel is edited in the UI (D6); rule names and streams in History (D7); keep wildcard *stream offline* alerts firing until the stream returns (D8) | S–M each |
| 3.3 | Cosmetics: round sub-1 Mbps bitrates (D9), show env-configured AMS on Settings → Sources (D12), re-tune the health score (D10) | S |
| 3.4 | Validate the Helm chart on a real cluster before advertising Kubernetes on the page | M |
| 3.5 | Light-theme screenshot variants, if Ant Media's page uses light sections (`--theme light`) | S |
| 3.6 | Re-record the demo video over the current UI | M |
| 3.7 | `NPM_TOKEN` repository secret, so `ams-pulse-beacon` publishes to npm on release | XS |
