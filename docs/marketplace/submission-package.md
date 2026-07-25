<!--
  DRAFT — INTERNAL. External use gated on operator review of
  docs/assessment/final-assessment.md (D-081).
-->

# Ant Media Marketplace — Submission Package Index

**Product:** Pulse — Analytics & QoE Monitoring for Ant Media Server
**Version:** v0.4.0 · **Prepared:** S97 / D-161 (2026-07-22)

This is the single page to hand Ant Media when the listing process starts: every
submission artifact, where it lives, and its state. Statuses: **READY** (accurate,
reviewable now) · **DRAFT-OP** (content complete; blocked only on an operator decision
or the D-081 external-use review) · **TBD-EXT** (needs an external step).

## Listing artifacts

| Artifact | Location | Status |
|---|---|---|
| Listing copy (title, tagline, description, bullets, tiers, pricing) | [`listing-draft.md`](listing-draft.md) | DRAFT-OP (prices still PROPOSED — need sign-off; MaxNodes ladder now settled D-166; D-081) |
| Screenshots — 6 listing shots, 1920×1080 live-app | [`screenshot-list.md`](screenshot-list.md) + `screenshots/` (regenerate: `node qa/marketplace/capture-live-screenshots.mjs`) | READY (regenerable; commit/upload choice at submission) |
| Logo / media kit | `brandkit/logo/` (SVG + PNG variants), OG banner `brandkit/assets/png/og-1200x630.png` | READY (final specs = meeting A3) |
| Demo video | [`demo-video-script.md`](demo-video-script.md) | TBD-EXT (operator records) |
| Release notes ("what's new in 0.4") | [`release-notes.md`](release-notes.md) | DRAFT-OP (D-081) |

## Documentation set (linkable as the product docs)

| Doc | Location | Status |
|---|---|---|
| Product overview + architecture (diagrams) | [`../overview.md`](../overview.md) | READY |
| Install guide (quickstart / Compose / binary / Helm) | [`../runbooks/install.md`](../runbooks/install.md) | READY (GHCR public + anonymous quickstart verified end-to-end, D-168) |
| User guide (per-screen) | [`../user-guide.md`](../user-guide.md) | READY |
| Administrator guide (full config reference) | [`../admin-guide.md`](../admin-guide.md) | READY |
| API guide + rendered OpenAPI reference | [`../api-guide.md`](../api-guide.md) + [`../api/index.html`](../api/index.html) | READY |
| Beacon SDK integration (player-side QoE) | [`../beacon-sdk.md`](../beacon-sdk.md) + [`../../sdk/beacon-js/README.md`](../../sdk/beacon-js/README.md) | READY |
| Compatibility matrix (AMS versions, G-27, capacity) | [`../compatibility.md`](../compatibility.md) | READY except capacity row (load lane pending) |
| Known limitations (26 honest disclosures) | [`../known-limitations.md`](../known-limitations.md) | READY |
| Troubleshooting | [`../troubleshooting.md`](../troubleshooting.md) | READY |
| FAQ | [`../faq.md`](../faq.md) | READY |
| Upgrade & rollback | [`../../deploy/runbooks/upgrade-rollback.md`](../../deploy/runbooks/upgrade-rollback.md) | READY |
| Security policy | [`../../SECURITY.md`](../../SECURITY.md) | READY |
| Licensing explained (public terms + tiers + trial) | [`../licensing-public.md`](../licensing-public.md) | DRAFT-OP (trial mechanics; D-081) |
| Support policy | [`../support.md`](../support.md) | DRAFT-OP (channel + SLA decisions) |
| Changelog | [`../../CHANGELOG.md`](../../CHANGELOG.md) | READY |

## Process documents (internal)

| Doc | Location | Status |
|---|---|---|
| Submission process (facts vs assumptions A1–A10) | [`submission-process.md`](submission-process.md) | READY (internal) |
| Developer-meeting brief & agenda | [`developer-meeting-brief.md`](developer-meeting-brief.md) | READY (internal) |
| Readiness checklist (17 rows) | [`../assessment/final-assessment.md`](../assessment/final-assessment.md) §3 | Rows 7–11 operator-gated |
| Fact ledger (claims verified against code) | [`../../agents/handoffs/validation/S97-fact-ledger.md`](../../agents/handoffs/validation/S97-fact-ledger.md) | Evidence record |

## Validation evidence

46/50 live scenarios vs AMS 3.0.3 Enterprise · CI (Go `-race` + coverage gate, web,
full-stack e2e, CSP e2e, docker-build stamp gate, Helm golden, SDK 15 KB gate, nightly
AMS version-matrix, CodeQL) · cosign-signed multi-arch images + SBOM/provenance +
Trivy-gated releases · load-lane budgets L-1…L-9 (**capacity number pending the
operator's dedicated PAYG AMS run** — `bash qa/realams/run-load-suite.sh`).

## Blocking items before external submission

**✅ DONE (2026-07-25):**
- ~~**GHCR public** — reviewers must `docker pull` anonymously.~~ **DONE (D-168):** package is
  public; the anonymous clean-room install (`docker pull …:0.4.1` → quickstart → live dashboard,
  collector `ok`, events flowing) was verified end-to-end with zero credentials.
- ~~**MaxNodes reconcile** (Pro 10 vs Business 5 inversion)~~ **DONE (D-166):** Business is now 50;
  ladder is monotonic (Free 1 / Pro 10 / Business 50 / Enterprise ∞) with a regression test. The
  *prices* still need sign-off (item 2 below) but the node ladder is settled.

**Still needed — all operator-owned (no engineering left):**
1. **D-081 review** — operator reviews `docs/assessment/final-assessment.md`; clears the
   DRAFT-INTERNAL headers.
2. **Pricing sign-off** — the tier/node ladder is fixed in code; the dollar figures are still
   "(PROPOSED)". Confirm them.
3. **Support channel + SLA** — fill `docs/support.md` decision boxes.
4. **Trial mechanics** — confirm the 14-day Pro proposal + vault key-mint ceremony.
5. **Capacity number** — run the load lane on a dedicated PAYG AMS; fill `docs/compatibility.md`.
6. **Demo video** — record per the script (or authorize a Playwright rough cut).
7. Then: reply to Ankush Banyal — documentation ready, request the developer meeting
   ([`developer-meeting-brief.md`](developer-meeting-brief.md)).
