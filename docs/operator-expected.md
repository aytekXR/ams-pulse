# Operator TODO — the items only YOU can do (updated post-D-071 brandkit directive, 2026-07-10)

> **Audience: the human operator.** Ledger of record: `ROADMAP.md` §5 + `ROADMAP-V2.md` §4; this
> file is the actionable view, refreshed at every session close. When you finish an item, just
> tell the agent (or do nothing — every session start re-verifies each item automatically).
> **Never commit secret VALUES anywhere; `deploy/.env` and `oguz-testing.md` are gitignored.**

## ⚡ TL;DR — expected from you right now (2026-07-10)

**Nothing blocks the next session (S12).** Your open items, all non-blocking:

| Priority | Item | What to say/do |
|---|---|---|
| 🟠 answer by ~07-23 | **CodeQL as a required merge check?** (needed when CI promotions land, S12 WO-D) | Reply "CodeQL required: yes/no" |
| 🟠 whenever | **PR-first cadence?** (drives enforce_admins re-arm, S12 WO-F) | Reply "PR-first" or "keep direct pushes" |
| 🟡 feature unlock | **U3 — Pro+ license in prod** (QoE/beacon data doesn't flow until then) | See §U3 below (self-serve minting) |
| 🟢 optional | O7 GHCR-public · D-V2-1 unsigned-webhook call · O11 rotation · `gh auth refresh -s workflow` | See §Optional below |
| 👀 later (after S12) | **Browser-accept the re-branded UI** (your brandkit ships in S12 WO-G) | You'll be pinged with URLs to eyeball |

## ✅ Brandkit received (2026-07-10) — UI re-theme scheduled for the next session

Your `brandkit/` package is committed and scheduled: SESSION-12 carries a non-droppable
work order (WO-G, D-071) to re-theme the web UI from it — design tokens, self-hosted IBM
Plex, the new logo/favicon/PWA icons, and component/chart restyling per your hi-fi screens.
Nothing needed from you now; after it ships you'll be asked for a browser check (the U5
pattern) to visually accept it.

## ✅ AMS trial license expiry (2026-07-12) — operator says handled (2026-07-10)

You told the session "don't worry about AMS" — recorded as operator-handled/accepted. No
session action; S12 will simply observe whether real-AMS polling still returns data during
the release test and report what it sees.

## ✅ What SESSION-11 did (2026-07-09/10, D-070 — nothing was needed from you)

| Area | Result |
|---|---|
| **White-label PDF logo** | Set `PULSE_REPORT_LOGO_PATH=/path/to/logo.png` (PNG or JPEG) and scheduled PDF reports carry YOUR logo; unset = embedded Pulse mark; bad path/format = warning + default, never a crash. |
| **Anomaly alert rules** | New rule type in the alert UI: instead of a manual threshold, a rule fires when viewer_count / cpu_pct / mem_pct deviates > σ from its learned baseline. Proven end-to-end in CI (e2e step A5). |
| **SSO / OIDC (phase 1)** | Set `PULSE_OIDC_ISSUER/CLIENT_ID/CLIENT_SECRET/REDIRECT_URL` (see `deploy/.env.example`) and `/auth/oidc/login` does a full PKCE code flow against Okta/Entra/Google, maps IdP groups to admin/viewer (fail-closed), and issues a session cookie. ⚠ Phase-1 note: the web UI login screen doesn't use it yet (phase 2) — the cookie authenticates API calls. |
| **install.md accuracy** | 6 doc bugs fixed (the yaml-copy step was dead — config is env-var-only; missing AMS env vars; released-image instructions; port-80 collision warning; migration note). |
| **Interrupted + resumed** | The session hit the usage limit mid-run; work was committed per-scope and the workflow resumed cleanly — nothing lost. |

## ✅ GHCR — release test UNBLOCKED (2026-07-10, your `read:packages` refresh)

Verified live the moment you did it: authed `docker pull ghcr.io/aytekxr/ams-pulse:0.2.0` ✓
and keyless `cosign verify` ✓ (signed by release.yml @ v0.2.0, commit 4657512, Rekor log
2128354996). **S12 WO-E (the clean-install release test) can now run.** Bonus: this
immediately caught a doc bug — image tags have NO `v` prefix (git tag `v0.2.0` → image tag
`0.2.0`); docs fixed.

### O7 — package visibility (now OPTIONAL, outsiders-only)
The package is still **private**: outside users can't `docker pull` or `cosign verify` it.
That no longer blocks any session work — it only matters when you want the public to pull
your image. Click path when you do: github.com/aytekXR → Packages → `ams-pulse` → Package
settings → Danger zone → **Change visibility → Public** (UI-only, no API).

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
*Status snapshot (2026-07-10, post-D-071): GA v0.2.0 live + healthy; S11 features (PDF logo,
anomaly rules, OIDC phase 1) on main, ship with the next rollout; **brandkit committed +
UI re-theme scheduled (S12 WO-G, non-droppable)**; release test UNBLOCKED (read:packages ✓,
pull+cosign verified); AMS license operator-handled; dependabot queue zero; Go coverage
73.9% (floor 70.2). CI/e2e/codeql all GREEN at the last code-verified commit (`ae6b5ed`);
the D-071 commits are docs+assets-only and pushed `[skip ci]` per your instruction.
Plan-of-record completion: v1 roadmap → GA 100%; ROADMAP-V2 sessions 3/5 done (S9-S11);
v2 backlog ~6.5/15 items closed. Your list: 2 questions (CodeQL, PR-first) + optional
O7-public / U3 / logo / D-V2-1 / O11 / workflow-scope.*
