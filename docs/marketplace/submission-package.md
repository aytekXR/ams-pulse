# Ant Media Marketplace — Submission Package Index

**Product:** Pulse — Analytics & QoE Monitoring for Ant Media Server
**Version:** v0.4.1 (released, GHCR public) · **Prepared:** S97 / D-161; updated S103 / D-169 (2026-07-25)

This is the single page to hand Ant Media when the listing process starts: every
submission artifact, where it lives, and its state. Statuses: **READY** (accurate,
reviewable now) · **DRAFT-OP** (content complete; blocked only on an operator decision
or the D-081 external-use review) · **TBD-EXT** (needs an external step).

## Listing artifacts

| Artifact | Location | Status |
|---|---|---|
| Listing copy (title, tagline, description, bullets, tiers, pricing) | [`listing-draft.md`](listing-draft.md) | READY-OP (pricing + Founding Operators campaign set D-169; ladder settled D-166; only submit-gated) |
| Screenshots — 6 listing shots, 1920×1080 live-app | [`screenshot-list.md`](screenshot-list.md) + `screenshots/` (regenerate: `node qa/marketplace/capture-live-screenshots.mjs`) | READY (committed to `docs/marketplace/screenshots/`, S105/D-172; portable capture script; regenerable at any time) |
| Logo / media kit | `brandkit/logo/` (SVG + PNG variants), OG banner `brandkit/assets/png/og-1200x630.png` | READY (final specs = meeting A3) |
| Demo video | [`demo-video-script.md`](demo-video-script.md) + rough-cut `docs/marketplace/demo/pulse-demo-roughcut.webm` | DRAFT-OP (rough-cut rendered D-170, attached to [GitHub release v0.4.1](https://github.com/aytekXR/ams-pulse/releases/tag/v0.4.1); operator records final with voiceover — TBD-EXT for voiceover only) |
| Release notes ("what's new in 0.4") | [`release-notes.md`](release-notes.md) | READY (D-081 cleared D-169) |

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
| Licensing explained (public terms + tiers + trial) | [`../licensing-public.md`](../licensing-public.md) | READY (pricing + campaign + 14-day trial set D-169) |
| Support policy | [`../support.md`](../support.md) | READY (channel + SLA set D-169; mailbox live, D-171) |
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
  ladder is monotonic (Free 1 / Pro 10 / Business 50 / Enterprise ∞) with a regression test.
- ~~**Pricing sign-off**~~ **DECIDED (D-169, operator-delegated):** standard Free $0 / Pro $99 /
  Business $299 / Enterprise from $799 per month, + the "Founding Operators" near-free-year-1
  launch campaign (Pro $9 / Business $29 for the first 12 months). Written into `licensing-public.md`
  + `listing-draft.md`. Operator may override any figure.
- ~~**Support channel + SLA**~~ **DECIDED (D-169):** `support@beyondkaira.com`; Pro 2-day / Business
  1-day / Enterprise 4-business-hour targets; hours Mon–Fri 09:00–18:00 UTC. Written into `support.md`.
- ~~**Trial mechanics**~~ **DECIDED (D-169):** 14-day Pro trial (no credit card; key delivered by email via support@beyondkaira.com or marketplace listing), graceful revert to Free.
- ~~**D-081 review**~~ **CLEARED (D-169):** `final-assessment.md` reviewed on the operator's behalf;
  internal assessment docs stay internal (not part of the external listing).
- ~~**Provision `support@beyondkaira.com`**~~ **DONE (2026-07-25, operator, D-171):** the mailbox
  is created and live; `support.md` + `licensing-public.md` already publish it.

**Still needed — OUTBOUND / operator-infra only (the loop physically cannot do these):**
1. **Submit the listing** to the Ant Media Marketplace (operator account).
2. **Set up billing** for the tiers / campaign / trial in the marketplace's billing system.
3. **Capacity number** — run the load lane on a dedicated PAYG AMS → replaces the PROVISIONAL claim
   now in `docs/compatibility.md`.
4. **Demo video** — rough-cut rendered (D-170, `docs/marketplace/demo/pulse-demo-roughcut.webm`)
   and attached to [GitHub release v0.4.1](https://github.com/aytekXR/ams-pulse/releases/tag/v0.4.1);
   operator re-records the final with voiceover.
5. **Reply to Ankush Banyal** — draft ready (`ankush-reply-draft.md`, D-170); operator fills the
   [brackets] and sends.
6. Optional: **roll prod to v0.4.1** (prod is healthy on v0.4.0-139); **rotate** the exposed secrets.
