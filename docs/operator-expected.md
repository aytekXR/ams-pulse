# Operator TODO — the items only YOU can do (updated at S13 open, post-D-072, 2026-07-10)

> **Audience: the human operator.** Ledger of record: `ROADMAP.md` §5 + `ROADMAP-V2.md` §4; this
> file is the actionable view, refreshed at every session close. When you finish an item, just
> tell the agent (or do nothing — every session start re-verifies each item automatically).
> **Never commit secret VALUES anywhere; `deploy/.env` and `oguz-testing.md` are gitignored.**

## ⚡ TL;DR — expected from you right now (2026-07-10)

> **S13 IS RUNNING (probe protocol completion: RTMP + DASH + WebRTC media path):
> NOTHING is required from you.** All four of your standing questions are unanswered —
> that's fine: the session skips/gates the dependent items and runs everything else
> autonomously. Answer any of them whenever you like.

**Nothing blocks the current session (S13).** Your open items, all non-blocking:

| Priority | Item | What to say/do |
|---|---|---|
| 🟠 **NEW — unlocks prod rollout** | **Ship v0.3.0?** Prod still runs v0.2.0; main carries the O(N²) fix (D-068), S11 features (PDF logo, anomaly rules, OIDC phase 1), and S12's Postgres backend + WebRTC probe + **your brandkit UI re-theme** — none of it reaches prod until this ships. | Reply "ship v0.3.0" — the session tags, rolls out with smoke + rollback ready, then pings you for the browser look |
| 🟠 answer by ~07-23 | **CodeQL as a required merge check?** (needed when CI promotions land, S13/S14 WO-A) | Reply "CodeQL required: yes/no" |
| 🟠 whenever | **PR-first cadence?** (drives enforce_admins re-arm) | Reply "PR-first" or "keep direct pushes" |
| 🟠 whenever | **Mobile SDKs — do you have (or plan) native iOS/Android apps?** S14's shape depends on it; no iOS work starts without an explicit yes. | Reply "need mobile SDKs: yes/no" (no = §2.12 is cut from the roadmap) |
| 🟡 feature unlock | **U3 — Pro+ license in prod** (QoE/beacon data doesn't flow until then) | See §U3 below (self-serve minting) |
| 🟢 optional | O7 GHCR-public · D-V2-1 unsigned-webhook call · O11 rotation · `gh auth refresh -s workflow` | See §Optional below |
| 👀 later (after v0.3.0) | **Browser-accept the re-branded UI** (shipped on main in S12; prod renders the OLD UI until v0.3.0 rolls out) | You'll be pinged with URLs to eyeball |

## ✅ What SESSION-12 did (2026-07-10, D-072 — nothing was needed from you)

| Area | Result |
|---|---|
| **Your brandkit → product UI (WO-G, D-071)** | Phase 1 shipped on main: design tokens → CSS vars, self-hosted IBM Plex (zero CDN), your favicons/PWA icons/manifest/logo marks, nav + buttons + badges + charts restyled per the design system, WCAG contrast table applied. Light theme = phase 2. **Prod still shows the old UI until v0.3.0 ships** — your browser-accept comes after that. |
| **Postgres meta backend (HA)** | `PULSE_META=postgres` + DSN now supported (SQLite stays the default; zero action needed from you). Proven by a 19-test parity suite against postgres:16 in CI. Note: the backup sidecar is SQLite-only — PG operators use pg_dump. |
| **WebRTC probe phase 1** | WebRTC streams now get a real signaling probe (connect time, precise error codes) instead of `not_probed`. Media-path QoE (rtt/jitter/loss) is S13's WO-D. |
| **Backup keep-7 retention** | First real prune observed live (cycle 8): oldest backup removed, 7/7 kept, and a full ClickHouse RESTORE was verified (613,939 rows) + meta integrity ok. |
| **Release install test** | Clean-install from the released 0.2.0 image vs your real AMS: healthy in 182 s (budget was 15 min). 7 more install.md bugs found and fixed in the process. |
| **PDF report logo** | The default embedded logo is now your brandkit "powered by pulse" badge (your own logo still wins via `PULSE_REPORT_LOGO_PATH`). |
| **Quality net** | 3 adversarial verifiers, 10 findings, all resolved same-session — including a CRITICAL e2e assertion bug caught before push. |

## ✅ AMS trial license expiry (2026-07-12) — operator says handled (2026-07-10)

You said "don't worry about AMS" — recorded as operator-handled/accepted. S12 observed the
real AMS still serving normally 2 days pre-expiry. Sessions keep observing + reporting only.

## 🟠 Standing questions (answer whenever, not blocking — now FOUR, see TL;DR table)

1. **Ship v0.3.0?** (NEW) — gates the prod rollout work order; everything is staged and the
   release pipeline is proven (v0.1.0, v0.2.0). Rollback tags stand.
2. **CodeQL as a REQUIRED merge context** when the web-e2e/csp-e2e promotions land
   (≥2026-07-23)? Reply "CodeQL required: yes/no".
3. **PR-first cadence?** Today sessions push directly to main; `enforce_admins` stays off
   for that (rationale re-recorded every session). Say "PR-first" to flip.
4. **Mobile SDKs needed?** iOS/Android beacon SDKs are roadmap §2.12, operator-gated at S14.

## 🟡 When you're ready (feature unlock, not a blocker)

### U3 — Activate a Pro+ Pulse license in prod (minting is self-serve)
- Mint your own key: `docs/licensing.md` §3 (offline vendor keypair → vault → pubkey-only
  deploy), e.g. `go run . -tier pro -privkey /secure/vendor.priv -expires 365` in
  `qa/licensegen/`.
- Then set `PULSE_LICENSE_KEY=<key>` (+ `PULSE_LICENSE_PUBKEY=<your pubkey>`) in `deploy/.env`
  and tell a session; it restarts pulse and live-verifies the beacon → QoE chain.
- Until then QoE/beacon data does NOT flow in prod (CI covers it with a mock license).

## 🟢 Optional / your policy call

- **O7 — GHCR package visibility** (outsiders-only): the package is private; outside users
  can't `docker pull` or `cosign verify`. Click path: github.com/aytekXR → Packages →
  `ams-pulse` → Package settings → Danger zone → **Change visibility → Public** (UI-only).
- **D-V2-1 — unsigned-webhook ingest mode** (AMS 3.0.3 can't sign hooks): build an optional
  IP-allowlisted unsigned mode, or keep REST-polling-only (current, meets the ≤10 s budget)?
  Reply "build" or "wontfix" whenever; no work happens until you decide.
- **gh `workflow` scope:** the gh token can't update PR branches touching `.github/workflows/*`
  (sessions detour via `@dependabot rebase`). One-time fix: type `! gh auth refresh -s workflow`
  in a session (interactive, ~1 min). Pure convenience.
- **O11 rotation** (if policy demands): api.slack.com/apps → regenerate webhook →
  `gh secret set SLACK_WEBHOOK_URL`. (Exposure was never public; risk-accepted D-066.)

---
*Status snapshot (2026-07-10, S13 open): GA v0.2.0 live + healthy; main is ahead with
D-068 O(N²) fix + S11 features + S12 Postgres/WebRTC-probe/brandkit UI — all await your
"ship v0.3.0". CI/e2e/codeql ALL GREEN at `c767ded`. Dependabot queue zero. Go coverage
73.9% (floor 70.2); web 62.68/58.78/51.54 (gates 59/54/45, vitest-4); sdk 3.52 KB.
Backup keep-7 restore-verified. Plan of record: `ROADMAP-V2.md` (S13 running: RTMP + DASH
probes, WebRTC media path, TTL fix, CI-promotion date gate ≥07-23). Your list: 4 questions
(v0.3.0, CodeQL, PR-first, mobile SDKs) + U3 + optional O7/D-V2-1/O11/workflow-scope.*
