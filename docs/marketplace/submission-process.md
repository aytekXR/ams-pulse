<!--
  DRAFT — INTERNAL. External use gated on operator review of
  docs/assessment/final-assessment.md (D-081).
-->

# Ant Media Marketplace — Submission Process

**Product:** Pulse — Analytics & QoE Monitoring for AMS
**Prepared:** S97 / D-161 (2026-07-22)
**Contact thread:** Ankush Banyal (Ant Media)

This document records everything known about how Pulse gets listed on the Ant Media
Marketplace, separating **established facts** (from the email thread and public research)
from **assumptions** that must be verified at the developer meeting. Companion docs:
[`developer-meeting-brief.md`](developer-meeting-brief.md) (the meeting agenda),
[`submission-package.md`](submission-package.md) (the artifact index),
[`listing-draft.md`](listing-draft.md) (the listing copy).

---

## 1. The agreed process (established facts)

From the Ankush Banyal email thread (2026-07):

1. **Pulse is fully ready + "relevant documentation" is ready** ← this documentation pack.
2. **Operator replies to Ankush**: documentation is ready; request the developer meeting.
3. **Ant Media arranges a meeting with their developer** to continue the listing process.
4. **Qualification steps** are still being defined by Ant Media's development team
   ("consulting with our development team on the qualification steps") — expect them at
   or around the meeting. **No published submission checklist exists** (verified by
   research 2026-07-22: no public checklist, artifact spec, review SLA, security-review
   process, or qualification thresholds could be found).

Additional established context:

- A **live demo was already given** (2026-07): "the live dashboard looked good" — expect a
  deeper technical demo at the developer meeting.
- Ant Media pointed us at their **load-testing documentation** — read as: capacity/scale
  validation is part of qualification. Our answer is the opt-in load lane
  (`qa/realams/load/`, budgets L-1…L-9, four scenarios TC-S-10…13), which also supports
  Ant Media's official tools (`LOAD_GENERATOR=official` — WebRTC Load Test Tool,
  `hls_players.sh`). The measured capacity number goes into
  [`../compatibility.md`](../compatibility.md) and the listing.
- Our AMS trial expired; the sanctioned continued-testing path is the AMS
  **pay-as-you-go hourly subscription** (per Ankush's email; ≈$0.09/instance/hr per
  third-party pricing aggregators — confirm on the official page).
- Ant Media is revamping its web panel. Assessment (G-27): **PROCEED** — see
  [`../compatibility.md`](../compatibility.md) § Panel-revamp, now backed by public-repo
  evidence (`ant-media/Management-panel-reborn`): the new panel is built entirely on
  `/rest/v2` with the unchanged cookie auth flow.
- **Marketplace structure** (public research 2026-07-22): listings live at
  `antmedia.io/marketplace/[slug]/`; existing listing types are JAR plugins (Enterprise),
  **WAR applications** uploaded via the AMS dashboard (Bitmovin Analytics — the closest
  analytics precedent), **external-process integrations** (GST-Ant Fusion), and services
  (Raskenlund). A "become a marketplace vendor" form exists on the marketplace page
  (formal fallback channel; we already have direct contact).
- **First-year vendor terms publicly stated: 100% revenue to the vendor, no commission.**
  Post-year-1 terms are NOT published — get them in writing at the meeting. (This
  supersedes the PRD's old, unverified 20–30% revenue-share figure — do not repeat that
  figure anywhere.)
- Ant Media promotes listings via blog/case-study, a 40,000+ subscriber newsletter,
  social, joint webinars, and an Ecosystem Slack channel.

## 2. Assumptions — verify at the developer meeting

Each assumption below is tagged where used across the docs pack as
**⚠ ASSUMPTION — verify at the developer meeting**.

| # | Assumption | Basis / risk |
|---|---|---|
| A1 | Pulse lists as a standalone **integration/solution** (own deploy, like GST-Ant Fusion), not a WAR/JAR inside AMS | The closest analytics precedent (Bitmovin) IS a WAR — ask explicitly whether an external self-hosted service is listable as-is, or whether they want a thin AMS-side artifact |
| A2 | Listing format: title ≤60 chars, short description ≤250 chars, 5–6 feature bullets, ~6 screenshots | Our own analysis of existing listings, not a published spec |
| A3 | Screenshot/logo/video specs | Unpublished; we prepared 1920×1080 PNGs, SVG + 256px logos, 1200×630 OG banner, a 2–3 min video script as defaults |
| A4 | Revenue: first-year 100%/no-commission (publicly stated); **post-year-1 unknown** | Get post-year-1 terms in writing |
| A5 | Review flow: functional install review + doc review + security/scale questions; security review possibly self-certified | No published SLA/timeline; our posture: SECURITY.md, zero phone-home, cosign/SBOM, IP hashing |
| A6 | A trial offer is expected; our mechanic (14-day Pro key on request) is OPERATOR-DECISION-PENDING | Official trial-key mint is operator-gated (vault privkey ceremony) |
| A7 | AMS-version-support requirement (N-1/N-2?) unknown | Our position: 3.0.3 (current latest stable) live-validated; older versions mock-profile only — honestly disclosed |
| A8 | Linking to our GitHub docs is acceptable | If uploads/PDFs required, the markdown pack converts cleanly |
| A9 | Load-test qualification thresholds: none published | We present budgets L-1…L-9 + the capacity number + "Pulse adds only read-only REST polling load to AMS"; ask what evidence format they want |
| A10 | Listing category (analytics/monitoring?) and marketplace integration with the new React panel | The public panel repo shows no marketplace UI; confirm at the meeting |

## 3. Prerequisites before submission (state → owner)

| Prerequisite | State | Owner |
|---|---|---|
| GHCR image public (reviewers must pull anonymously; today 401) | OPEN | Operator (§2.18/O7 — at the release reviewers should pull) |
| Support channel + SLA named (checklist row 7) | Skeleton in [`../support.md`](../support.md) | Operator decision |
| Public licensing/trial terms (row 8) | Draft in [`../licensing-public.md`](../licensing-public.md) | Operator approval + trial-key mint ceremony |
| Capacity number (load lane on dedicated PAYG AMS) | Lane built, NOT run | Operator (`bash qa/realams/run-load-suite.sh` after `cp qa/realams/harness/load-env.sh.example qa/realams/harness/load-env.sh`) |
| Pricing + tier sign-off (incl. Pro=10 vs Business=5 MaxNodes reconcile) | Flagged in [`listing-draft.md`](listing-draft.md) | Operator product call |
| 15-min staging-panel network-tab walkthrough (G-27 residual) | Checklist in `docs/operator-expected.md` §3 | Operator (largely pre-answered by public-repo evidence) |
| Final-assessment review (D-081 gate — un-gates every DRAFT-INTERNAL header) | OPEN | Operator |
| Demo video | Script in [`demo-video-script.md`](demo-video-script.md) | Operator records (or approves a capture-based rough cut) |
| Rotate chat-exposed credentials; AMS PAYG license for the demo instance | OPEN (carried) | Operator |

## 4. Validation evidence to attach

- **Live validation:** 46/50 scenario scripts PASS against AMS 3.0.3 Enterprise
  (`docs/assessment/final-assessment.md` §1); 84.5% weighted product-completeness score.
- **CI:** Go suite with `-race` + 70.2% coverage floor; web suite + Playwright e2e;
  full-stack compose e2e (license mint → beacon → alert fire); CSP e2e; docker-build
  version-stamp gate; Helm lint/template golden tests; SDK 15 KB size gate; nightly AMS
  version-matrix + CodeQL.
- **Load:** budgets L-1…L-9 + capacity number (pending operator run — see §3).
- **Supply chain:** cosign-signed multi-arch images, SBOM + provenance, Trivy-gated
  releases, Dependabot policy.
- **Security/privacy:** [`../../SECURITY.md`](../../SECURITY.md); zero phone-home;
  viewer IPs SHA-256-hashed (optional anonymization); GeoIP only via operator-supplied
  MMDB; secrets encrypted at rest (AES-256-GCM); audit log on admin writes.

## 5. Expected review workflow and post-submission follow-up

**Expected flow (A5 — verify):** operator email → developer meeting (demo + qualification
steps handed over) → we satisfy the steps (likely: install review by their developer, doc
review, load evidence) → listing copy + assets handed to their marketplace team → listing
goes live → co-marketing (blog/newsletter/webinar — their stated channels).

**Post-submission follow-ups:**

- Track the qualification-steps document Ant Media's dev team produces; fold its
  requirements into `submission-package.md` and close the A1–A10 assumptions.
- Iterate the listing per review feedback (copy, screenshots, category).
- Coordinate the co-marketing blog post (checklist row 11) at launch.
- Keep `qa/tools/ams-drift-watch.sh` + the nightly version matrix green on new AMS
  releases; update [`../compatibility.md`](../compatibility.md) per release and notify
  the marketplace listing if the support matrix changes.
- Honor the support SLA in [`../support.md`](../support.md) once published.
- Re-verify the G-27 exposure when the AMS build carrying the new panel ships
  (`ams-drift-watch.sh`, then re-run the 46-scenario harness before claiming support).
