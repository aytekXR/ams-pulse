# Operator TODO — the items only YOU can do (updated by SESSION-08 close, D-066, 2026-07-09)

> **Audience: the human operator.** The ledger of record is `ROADMAP.md` §5; this file is the
> actionable view, refreshed at every session close. When you finish an item, just tell the
> agent (or do nothing — every session start re-verifies each item automatically).
> **Never commit secret VALUES anywhere; `deploy/.env` and `oguz-testing.md` are gitignored.**

## ✅ What your 2026-07-09 decisions triggered (all executed same-session)

| Decision | Result |
|---|---|
| **O13 tag = v0.2.0** | Tag pushed; release pipeline run `29023647495` (CI-gated, Trivy, SBOM, cosign) — prod rolled onto the tag after it went green. GA is shipped. |
| **O5 license = noncommercial** | **PolyForm Noncommercial 1.0.0** at root `LICENSE` (SDK stays MIT); README + CHANGELOG + `docs/licensing.md` updated. Rationale: purpose-built noncommercial code license; you retain full commercial/dual-licensing rights — matches the paid-tier model. |
| **O12 secret-scanning** | ENABLED (+ push-protection) via API, verified. |
| **O3 AMS webhook** | **Closed as not-applicable:** live inspection of AMS 3.0.3 settings (182 fields) shows NO HMAC/signature-header support — unsigned hooks would only 401 against Pulse's fail-closed listener. REST polling (5 s) remains the supported ingest and meets the ≤10 s budget. `AMS-INTEGRATION.md` §4.5 corrected. O4 is moot. |
| **U5 browser/CSP** | Closed via real headless-Chromium check from the VPS: both `beyondkaira.com` and `pulse.beyondkaira.com` → HTTP 200, SPA rendered, **0 console errors / 0 CSP violations**. (Optional: one human glance whenever convenient.) |
| **O11 Slack webhook** | Risk ACCEPTED + recorded (exposure was never public: unpushed commit + local transcripts). Stale local branch `backup/slack-notify-original` deleted. Rotate later only if your policy demands — 2-min task, instructions in ROADMAP §5. |
| **O8 dependabot (21 PRs)** | Verdict: PR **#4 CLOSED** (golang 1.26 — violates the D-032 pin; dependabot now ignores golang version bumps). The remaining 20 are deferred to a SESSION-09 absorption work order with real verification — merging blind before the release would have risked the pipeline. |

## 🔴 The ONE remaining click

### O7 — Make the GHCR package public
- Click path: github.com/aytekXR → Packages → `ams-pulse` → Package settings →
  Danger zone → **Change visibility → Public**.
- There is NO API for this (verified — visibility change is UI-only), so it stays with you.
- Until then: nobody can `docker pull ghcr.io/aytekxr/ams-pulse:v0.2.0` anonymously and
  `cosign verify` fails for outsiders. Everything else about the release is done.

## 🟡 When you're ready (feature unlock, not a blocker)

### U3 — Activate a Pro+ Pulse license in prod
- `deploy/.env` line 32 has the commented placeholder — set `PULSE_LICENSE_KEY=<key>`
  and tell a session; it restarts pulse and live-verifies the beacon → QoE chain.
- **Minting your own keys:** the vendor-key ceremony + minting + distribution procedure is
  now documented in `docs/licensing.md` (you generate an ed25519 keypair offline; keys are
  signed claims; deployments carry `PULSE_LICENSE_PUBKEY`).

## 🟢 Optional / your policy call

- **O11 rotation** (if policy demands): api.slack.com/apps → regenerate webhook →
  `gh secret set SLACK_WEBHOOK_URL`.
- **O8 majors**: a future session absorbs vite 8 / vitest 4 / eslint 10 / size-limit 12 /
  plugin-react 6 with full verification (SESSION-09 WO).

---
*Status snapshot (2026-07-09, post-D-066): **GA SHIPPED as v0.2.0.** G1 ✅ except O7 ·
G2 ✅ · G3 ✅ (73.2%, floor 70.2) · G4 ✅ · G5 promotions land ~2026-07-23 (agent-owned) ·
G6 ✅ · G7 ✅ **fully** (LICENSE landed) · G8: U3 optional-unlock, U5 ✅, O3 N/A.
Your list is down to: O7 (one click) + U3 (when you want QoE data flowing).*
