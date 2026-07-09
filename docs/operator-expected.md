# Operator TODO — the items only YOU can do (updated by SESSION-11 start, 2026-07-09)

> **Audience: the human operator.** Ledger of record: `ROADMAP.md` §5 + `ROADMAP-V2.md` §4; this
> file is the actionable view, refreshed at every session close. When you finish an item, just
> tell the agent (or do nothing — every session start re-verifies each item automatically).
> **Never commit secret VALUES anywhere; `deploy/.env` and `oguz-testing.md` are gitignored.**

## ✅ What SESSION-10 did (2026-07-09, D-068 — nothing was needed from you)

| Area | Result |
|---|---|
| **O(N²) hot path FIXED** | `rebuildSnapshot` replaced by O(1) incremental deltas — ~688× faster at 1k streams, linear growth proven by benchmarks, adversarially verified. The D-065 CPU-cap mitigation is reverted (0.5 vCPU again in compose+helm). The fix ships to prod with the next image rollout. |
| **License minting unblocked** | `qa/licensegen` now takes `-privkey <file>` (your production ed25519 key) and `-expires <days>`. Full vendor key ceremony documented in `docs/licensing.md` §3 — you can now mint real Pro+ keys (see U3 below). |
| **Dependabot policy** | `docs/dependabot-policy.md` (new) — steady-state cadence + merge mechanics per bump class; future PRs follow it. |
| **enforce_admins** | Stays `false`, rationale committed: with 1 required review and you as the only human, GitHub's no-self-approval rule means flipping it would deadlock all session pushes. See the question below. |
| **Date-gated skips** | Backup keep-7 cycle-8 check (≥07-16) and CI promotions (≥07-23) recorded + carried to SESSION-11. |

## 🔴 The ONE remaining click — now BLOCKS a work order (SESSION-11 WO-F)

### O7 — Make the GHCR package public
- Click path: github.com/aytekXR → Packages → `ams-pulse` → Package settings →
  Danger zone → **Change visibility → Public**.
- There is NO API for this (verified — UI-only), so it stays with you.
- Until then: nobody can `docker pull ghcr.io/aytekxr/ams-pulse:v0.2.0` anonymously and
  `cosign verify` fails for outsiders.
- **NEW (S11): this now blocks the clean-install RELEASE test (WO-F, your D-069 directive).**
  Verified 2026-07-09: anonymous GHCR manifest → 401, the agent's gh token has no
  `read:packages` scope, and no `ghcr.io/aytekxr/ams-pulse` image exists locally — so the
  released artifact cannot be pulled or cosign-verified at all. **Either** of these unblocks it:
  1. Click O7 (above) — also fixes it for outside users, OR
  2. Type `! gh auth refresh -s read:packages` in a session (interactive, ~1 min) — unblocks
     the agent only; outsiders still can't pull until O7.
  Until one happens, WO-F is recorded BLOCKED-on-operator and carried forward.

## 🟠 Two standing questions (answer whenever, not blocking)

1. **May CodeQL become a REQUIRED merge context** when the web-e2e/csp-e2e promotions land
   (~2026-07-23, SESSION-11 WO-E)? Reply "CodeQL required: yes/no" to any session.
2. **Do you want PR-first development cadence?** (NEW, from the enforce_admins revisit.)
   Today sessions push directly to main; `enforce_admins` must stay off for that. If you want
   everything through PRs instead, say "PR-first" — a session will drop the review requirement
   to 0 (or you add a second reviewer), flip `enforce_admins=true`, and adapt the workflow.
   Perfectly fine to leave as-is; revisit re-arms at S12 anyway.

## 🟡 When you're ready (feature unlock, not a blocker)

### U3 — Activate a Pro+ Pulse license in prod (minting is now self-serve)
- **NEW:** you can mint your own key now — follow `docs/licensing.md` §3 (generate the vendor
  keypair offline, keep the private key in a vault, deploy only the public key), e.g.:
  `go run . -tier pro -privkey /secure/vendor.priv -expires 365` in `qa/licensegen/`.
- Then set `PULSE_LICENSE_KEY=<key>` (+ `PULSE_LICENSE_PUBKEY=<your pubkey>`) in `deploy/.env`
  and tell a session; it restarts pulse and live-verifies the beacon → QoE chain.
- Until then QoE/beacon data does NOT flow in prod (CI covers it with a mock license).

## 🟢 Optional / your policy call

- **D-V2-1 — unsigned-webhook ingest mode** (AMS 3.0.3 can't sign hooks): build an optional
  IP-allowlisted unsigned mode, or keep REST-polling-only (current, meets the ≤10 s budget)?
  Reply "build" or "wontfix" whenever; no work happens until you decide.
- **gh `workflow` scope:** the gh token can't update PR branches touching `.github/workflows/*`
  (sessions detour via `@dependabot rebase`). One-time fix: type `! gh auth refresh -s workflow`
  in a session (interactive, ~1 min). Pure convenience.
- **O11 rotation** (if policy demands): api.slack.com/apps → regenerate webhook →
  `gh secret set SLACK_WEBHOOK_URL`. (Exposure was never public; risk-accepted D-066.)

---
*Status snapshot (2026-07-09, post-D-068): GA v0.2.0 live + healthy; dependabot queue zero;
O(N²) fixed + cap reverted (ships next rollout); license minting self-serve. Your list: O7 (one
click) + 2 questions (CodeQL, PR-first) + optional U3 / D-V2-1 / O11 / workflow-scope.*
