# Operator TODO — the items only YOU can do (updated by SESSION-11 close, D-070, 2026-07-10)

> **Audience: the human operator.** Ledger of record: `ROADMAP.md` §5 + `ROADMAP-V2.md` §4; this
> file is the actionable view, refreshed at every session close. When you finish an item, just
> tell the agent (or do nothing — every session start re-verifies each item automatically).
> **Never commit secret VALUES anywhere; `deploy/.env` and `oguz-testing.md` are gitignored.**

## ⏰ TIME-CRITICAL — AMS trial license expires 2026-07-12 (~2 days)

Your AMS Enterprise 3.0.3 trial license (see oguz-testing.md) is valid only to
**2026-07-12T12:09Z**. After that the real AMS may stop serving streams / reject REST calls —
which affects BOTH production Pulse polling AND the S12 clean-install live test.
**Action: renew/replace the AMS license before 07-12**, or expect prod `live/overview` to go
empty and poll errors in pulse logs. (Pulse itself is unaffected — this is the AMS-side license,
not the Pulse license U3.)

## ✅ What SESSION-11 did (2026-07-09/10, D-070 — nothing was needed from you)

| Area | Result |
|---|---|
| **White-label PDF logo** | Set `PULSE_REPORT_LOGO_PATH=/path/to/logo.png` (PNG or JPEG) and scheduled PDF reports carry YOUR logo; unset = embedded Pulse mark; bad path/format = warning + default, never a crash. |
| **Anomaly alert rules** | New rule type in the alert UI: instead of a manual threshold, a rule fires when viewer_count / cpu_pct / mem_pct deviates > σ from its learned baseline. Proven end-to-end in CI (e2e step A5). |
| **SSO / OIDC (phase 1)** | Set `PULSE_OIDC_ISSUER/CLIENT_ID/CLIENT_SECRET/REDIRECT_URL` (see `deploy/.env.example`) and `/auth/oidc/login` does a full PKCE code flow against Okta/Entra/Google, maps IdP groups to admin/viewer (fail-closed), and issues a session cookie. ⚠ Phase-1 note: the web UI login screen doesn't use it yet (phase 2) — the cookie authenticates API calls. |
| **install.md accuracy** | 6 doc bugs fixed (the yaml-copy step was dead — config is env-var-only; missing AMS env vars; released-image instructions; port-80 collision warning; migration note). |
| **Interrupted + resumed** | The session hit the usage limit mid-run; work was committed per-scope and the workflow resumed cleanly — nothing lost. |

## 🔴 The ONE remaining click — still blocks the release test (now S12 WO-E)

### O7 — Make the GHCR package public
- Click path: github.com/aytekXR → Packages → `ams-pulse` → Package settings →
  Danger zone → **Change visibility → Public**.
- There is NO API for this (verified — UI-only), so it stays with you.
- Until then: nobody can `docker pull ghcr.io/aytekxr/ams-pulse:v0.2.0` anonymously,
  `cosign verify` fails for outsiders, AND the clean-install RELEASE test you ordered
  (D-069) cannot run — verified 2026-07-09: anonymous pull 401, agent token lacks
  `read:packages`, no local copy of the ghcr image exists.
- **Alternative unblock (agent-only):** type `! gh auth refresh -s read:packages` in a
  session (interactive, ~1 min). Outsiders still can't pull until O7.

## 🟠 Two standing questions (answer whenever, not blocking)

1. **May CodeQL become a REQUIRED merge context** when the web-e2e/csp-e2e promotions land
   (≥2026-07-23, SESSION-12 WO-D)? Reply "CodeQL required: yes/no" to any session.
2. **Do you want PR-first development cadence?** Today sessions push directly to main;
   `enforce_admins` must stay off for that. If you want everything through PRs instead, say
   "PR-first" — a session will drop the review requirement to 0 (or you add a second
   reviewer), flip `enforce_admins=true`, and adapt the workflow. The revisit re-arms at S12
   (WO-F there) either way.

## 🟡 When you're ready (feature unlock, not a blocker)

### U3 — Activate a Pro+ Pulse license in prod (minting is self-serve)
- Mint your own key: `docs/licensing.md` §3 (offline vendor keypair → vault → pubkey-only
  deploy), e.g. `go run . -tier pro -privkey /secure/vendor.priv -expires 365` in
  `qa/licensegen/`.
- Then set `PULSE_LICENSE_KEY=<key>` (+ `PULSE_LICENSE_PUBKEY=<your pubkey>`) in `deploy/.env`
  and tell a session; it restarts pulse and live-verifies the beacon → QoE chain.
- Until then QoE/beacon data does NOT flow in prod (CI covers it with a mock license).

### Want your logo on PDF reports?
- Drop a PNG/JPEG on the VPS, set `PULSE_REPORT_LOGO_PATH=` in `deploy/.env`, and ask a
  session to roll it out (ships with the next prod image rollout, which also carries the
  O(N²) fix and OIDC/anomaly features — the agent may propose tagging v0.3.0).

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
*Status snapshot (2026-07-10, post-D-070): GA v0.2.0 live + healthy; S11 features (PDF logo,
anomaly rules, OIDC phase 1) on main, ship with the next rollout; dependabot queue zero;
Go coverage 73.9%. Your list: ⏰ AMS license (07-12!) + O7 (one click, blocks the release
test) + 2 questions (CodeQL, PR-first) + optional U3 / logo / D-V2-1 / O11 / workflow-scope.*
