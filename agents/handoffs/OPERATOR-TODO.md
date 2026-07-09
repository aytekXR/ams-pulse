# Operator TODO — the items only YOU can do (updated by SESSION-09 close, D-067, 2026-07-09)

> **Audience: the human operator.** Ledger of record: `ROADMAP.md` §5 + `ROADMAP-V2.md` §4; this
> file is the actionable view, refreshed at every session close. When you finish an item, just
> tell the agent (or do nothing — every session start re-verifies each item automatically).
> **Never commit secret VALUES anywhere; `deploy/.env` and `oguz-testing.md` are gitignored.**

## ✅ What SESSION-09 did (2026-07-09, D-067 — nothing was needed from you)

| Area | Result |
|---|---|
| **Dependabot queue** | **CLOSED — 0 open PRs** (was 20 + 1 trailing). Actions bumps proven by a green release-pipeline dry-run; image-digest bumps staged then rolled to prod (caddy/clickhouse/backup containers refreshed; the pulse app container untouched, still v0.2.0); web/sdk major upgrades (vite 8, vitest 4, eslint 10, size-limit 12, plugin-react 6) landed as verified co-upgrade units. |
| **Coverage gates** | Re-baselined for vitest 4's new instrumentation (reads lower on identical code): web 59/54/45, sdk 63/43/67 (sdk lines actually ratcheted UP). Enforcement proven. Go floor 70.2 unchanged. |
| **Prod** | `pulse v0.2.0`, healthy, smoke-green after the digest refresh (healthz ok, dashboard 200, live overview returns real data). |
| **Post-GA plan** | `ROADMAP-V2.md` seeded (S10–S13: enforce_admins, O(N²) fix, license-key minting flags, SSO/OIDC, Postgres meta, probes, mobile SDKs). Next session prompt ready: `sessions/SESSION-10.md`. |

## 🔴 The ONE remaining click

### O7 — Make the GHCR package public
- Click path: github.com/aytekXR → Packages → `ams-pulse` → Package settings →
  Danger zone → **Change visibility → Public**.
- There is NO API for this (verified — UI-only), so it stays with you.
- Until then: nobody can `docker pull ghcr.io/aytekxr/ams-pulse:v0.2.0` anonymously and
  `cosign verify` fails for outsiders. Re-verified still private today (anon pull → 403).

## 🟠 One standing question (answer whenever, not blocking)

- **May CodeQL become a REQUIRED merge context** when the web-e2e/csp-e2e promotions land
  (~2026-07-23, SESSION-10 WO-F)? Reply "CodeQL required: yes/no" to any session.

## 🟡 When you're ready (feature unlock, not a blocker)

### U3 — Activate a Pro+ Pulse license in prod
- `deploy/.env` line 32 has the commented placeholder — set `PULSE_LICENSE_KEY=<key>`
  and tell a session; it restarts pulse and live-verifies the beacon → QoE chain.
- **Minting your own keys:** documented in `docs/licensing.md` (ed25519 keypair offline;
  signed claims; deployments carry `PULSE_LICENSE_PUBKEY`). The `licensegen -privkey/-expires`
  minting flags land in SESSION-10.

## 🟢 Optional / your policy call

- **gh `workflow` scope (NEW, found in S9):** the gh CLI token can't update PR branches that
  touch `.github/workflows/*` (sessions detour via `@dependabot rebase`). One-time fix:
  type `! gh auth refresh -s workflow` in a session (interactive, ~1 min). Pure convenience.
- **O11 rotation** (if policy demands): api.slack.com/apps → regenerate webhook →
  `gh secret set SLACK_WEBHOOK_URL`. (Exposure was never public; risk-accepted D-066.)

---
*Status snapshot (2026-07-09, post-D-067): **GA SHIPPED as v0.2.0; dependabot queue ZERO.**
G1 ✅ except O7 · G2 ✅ (v0.2.0 + refreshed digests) · G3 ✅ (Go 73.2/70.2; web+sdk re-baselined
under vitest 4) · G4 ✅ · G5 promotions land ~2026-07-23 (agent-owned; CodeQL needs your yes/no) ·
G6 ✅ · G7 ✅ · G8: U3 optional-unlock. Your list: O7 (one click) + the CodeQL question +
optional U3 / O11 / workflow-scope.*
