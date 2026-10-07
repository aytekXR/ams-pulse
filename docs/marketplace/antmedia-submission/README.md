# Pulse (ams-pulse) — Ant Media Marketplace submission package

Prepared 2026-10-01 for Ant Media's marketplace page draft (*ams-pulse-page-draft*). It
answers Ant Media's five open items, corrects their draft against the actual product, and
supplies the images and documents to build the page.

**Start here:**

1. [`marketplace/answers-for-ant-media.md`](marketplace/answers-for-ant-media.md) — ready-to-send
   reply to Ant Media's "still needed" table.
2. [`marketplace/marketplace-copy.md`](marketplace/marketplace-copy.md) — the corrected page text
   in Ant Media's structure.
3. [`operator-expected.md`](operator-expected.md) — operator actions outside the repository.

## What is in the package

| Folder / file | Contents | Share with Ant Media? |
|---|---|---|
| `marketplace/marketplace-copy.md` | Final page copy (hero, intro, features, install steps, trust, CTA, developer, support, links) | Yes |
| `marketplace/answers-for-ant-media.md` | Answers to the five asks + draft corrections | Yes |
| `marketplace/draft-review.md` | Claim-by-claim check of their draft | Yes (helps their editor) |
| `marketplace/product-overview.md`, `installation-guide.md`, `configuration-guide.md`, `dashboard-access.md`, `alerting-guide.md`, `troubleshooting.md`, `system-requirements.md`, `architecture.md` | Operator documentation | Yes |
| `marketplace/asset-inventory.md` | Every image with caption, size and provenance | Yes |
| `marketplace/submission-checklist.md` | What is done / pending and who owns it | Optional |
| `marketplace/submission-notes.md` | Build and test results, what was run, changes made, **defects found** | Internal — share selectively |
| `assets/branding/` | Product logo: SVG, outlined SVG, transparent PNGs | Yes |
| `assets/hero/` | Annotated "alert triggered" hero, clean hero, social cards | Yes |
| `assets/screenshots/` | 14 real-application screenshots (1920×1080) | Yes |
| `assets/walkthrough/` | Install step 1 output; step 4 sign-in and first dashboard | Yes |
| `assets/diagrams/` | Architecture diagram (SVG + PNG) | Yes |
| `documentation/pdf/` | PDF versions of the overview, installation guide, answers and an asset sheet (in the ZIP) | Yes |
| `website/` | Snapshot of the public website source, including the new `get/` page (in the ZIP) | For reference |
| `internal/evidence/` | Defect evidence and raw verification logs: installer runs, accuracy check, image-signature check (tokens redacted) | **No** — internal |
| `operator-expected.md` | Developer's to-do list | **No** — internal |

## How the images were made — and what is real

Every UI image is a capture of the **real Pulse application**: the web UI, API, collector,
ClickHouse, alert evaluator and notification delivery, all running. Nothing was route-mocked
or redrawn. Two inputs were simulated, so the scenes could show a busy service — eight streams,
over a thousand viewers and a staged incident — on demand:

- **the Ant Media Server** — Pulse's own AMS simulator (`qa/mock-ams`), speaking the AMS REST v2
  wire format with an application named `LiveApp`;
- **the viewers** — synthetic player sessions sent through Pulse's public beacon endpoint.

So stream names and numbers are demo data. Pulse itself was also tested live against a real,
licensed **AMS 3.1.0 Enterprise** on 2026-10-07 (`marketplace/submission-notes.md` §0). The install
walkthrough comes from real runs of the installer. Admin tokens and passwords are masked. Details:
`marketplace/submission-notes.md` §2 and §6. To reproduce:
`qa/marketplace/demo-stack/README.md` in the repository.

## Versions

- Product release: **v0.5.0** (`ghcr.io/aytekxr/ams-pulse:0.5.0`, the Helm chart published with v0.5.0).
- Screenshots: retaken on 2026-10-07 from the v0.5.0 release candidate (same file names); the
  installer walkthrough (step 1 and the first-run dashboard) is retaken from the released v0.5.0.

## Decided

- **Page title:** "Pulse for Ant Media Server" (package name `ams-pulse`).
- **Developer credit:** Aytekin Erdogan (individual developer, beyondkaira.com) — no company.
- **Pricing:** Pulse is free. Every feature on every install, no license key needed, commercial
  use included. No purchase link — the main button points at https://aytekxr.github.io/ams-pulse/get/.
- **License:** PolyForm Shield License 1.0.0 (server, web UI, deploy tooling); MIT (beacon SDKs).

## Pending from Ant Media

Submission requirements, review timeline, load-test expectations, their terms for a free
listing. Each is an item in `operator-expected.md` §2.

## Rebuilding this package

From the repository root:

```sh
bash qa/marketplace/build-submission-zip.sh    # → dist/ams-pulse-antmedia-marketplace-submission.zip
```
