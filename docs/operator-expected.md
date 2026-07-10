# Operator TODO — the items only YOU can do (updated at S13 close, post-D-073, 2026-07-10)

> **Audience: the human operator.** Ledger of record: `ROADMAP.md` §5 + `ROADMAP-V2.md` §4; this
> file is the actionable view, refreshed at every session close. When you finish an item, just
> tell the agent (or do nothing — every session start re-verifies each item automatically).
> **Never commit secret VALUES anywhere; `deploy/.env` and `oguz-testing.md` are gitignored.**

## ⚡ TL;DR — expected from you right now (2026-07-10, S13 closed)

> **NOTHING is required from you.** S13 finished autonomously (RTMP + DASH probes,
> retention fix; the WebRTC media-path work was honestly re-gated to S14 with evidence).
> Your four standing questions are still open — answer any of them whenever; the next
> session (S14) checks them first and adapts.

**Your open items, all non-blocking:**

| Priority | Item | What to say/do |
|---|---|---|
| 🟠 **unlocks prod rollout** | **Ship v0.3.0?** Prod still runs v0.2.0; main now additionally carries S13's RTMP+DASH probes and the probe-retention fix on top of the O(N²) fix, S11 features (PDF logo, anomaly rules, OIDC), Postgres backend, WebRTC probe, and **your brandkit UI re-theme** | Reply "ship v0.3.0" — the session tags, rolls out with smoke + rollback ready, then pings you for the browser look |
| 🟠 answer by ~07-23 | **CodeQL as a required merge check?** (CI promotions land at S14, date gate opens 07-23) | Reply "CodeQL required: yes/no" |
| 🟠 whenever | **PR-first cadence?** (drives enforce_admins re-arm) | Reply "PR-first" or "keep direct pushes" |
| 🟠 whenever | **Mobile SDKs — do you have (or plan) native iOS/Android apps?** S14 has an iOS-SDK work order that fires ONLY on your explicit yes. | Reply "need mobile SDKs: yes/no" (no = the item is cut from the roadmap) |
| 🟡 feature unlock | **U3 — Pro+ license in prod** (QoE/beacon data doesn't flow until then) | See §U3 below (self-serve minting) |
| 🟢 optional | Enable DASH muxing on an AMS app (NEW, see below) · O7 GHCR-public · D-V2-1 unsigned-webhook call · O11 rotation · `gh auth refresh -s workflow` | See §Optional below |
| 👀 later (after v0.3.0) | **Browser-accept the re-branded UI** (prod renders the OLD UI until v0.3.0 rolls out) | You'll be pinged with URLs to eyeball |

## ✅ What SESSION-13 did (2026-07-10, D-073 — nothing was needed from you)

| Area | Result |
|---|---|
| **RTMP probe** | RTMP streams now get a real probe (TCP handshake, connect time, precise error codes) instead of `not_probed`. Zero new dependencies. The handshake was verified live against YOUR real AMS (succeeded in 40 ms). |
| **DASH probe** | DASH streams now get a full probe: manifest parse + first segment + bitrate — same measurements as HLS. |
| **Probe data retention** | `probe_results` previously ignored your configured retention (hardcoded 90 days); now honors `PULSE_RETENTION_DAYS` (new migration ships with the next rollout). |
| **WebRTC media QoE** | Honestly re-gated to S14 with evidence (needs a new dependency in two modules + substantial mock rework + a new fixture capture) — rather than landing something half-tested. |
| **Recovery** | Your terminal crash at the end of S12 lost nothing: the interrupted close was reconstructed and committed at S13 open. |
| **Quality net** | 3 adversarial verifiers incl. a live cross-pair test; findings fixed same-session (a DASH URL-resolution gap + a documentation sweep). |

### 🟢 NEW optional: enable DASH muxing on an AMS app
Your AMS has DASH muxing disabled (verified read-only: `.mpd` → 404), so the DASH probe's
test fixtures are spec-derived rather than captured from your server. Purely optional: if
you enable DASH muxing on any AMS app (AMS panel → app settings → muxing), tell a session
and it will capture a real MPD fixture to pin the parser against your server's exact
output. Nothing is broken without it.

## ✅ AMS trial license expiry (2026-07-12) — operator says handled (2026-07-10)

You said "don't worry about AMS" — recorded as operator-handled/accepted. S13 verified the
real AMS still answers (RTMP handshake + HLS manifest both live-confirmed today). Sessions
keep observing + reporting only.

## 🟠 Standing questions (answer whenever, not blocking — FOUR, see TL;DR table)

1. **Ship v0.3.0?** — gates the prod rollout work order; release pipeline proven (v0.1.0,
   v0.2.0); rollback tags stand. Until then prod stays on v0.2.0 and your brandkit UI is
   not visible in prod.
2. **CodeQL as a REQUIRED merge context** when the CI promotions land (≥2026-07-23, S14)?
3. **PR-first cadence?** Today sessions push directly to main; `enforce_admins` stays off
   for that (rationale re-verified every session). Say "PR-first" to flip.
4. **Mobile SDKs needed?** iOS/Android beacon SDKs — S14 WO-H fires only on explicit yes.

## 🟡 When you're ready (feature unlock, not a blocker)

### U3 — Activate a Pro+ Pulse license in prod (minting is self-serve)
- Mint your own key: `docs/licensing.md` §3 (offline vendor keypair → vault → pubkey-only
  deploy), e.g. `go run . -tier pro -privkey /secure/vendor.priv -expires 365` in
  `qa/licensegen/`.
- Then set `PULSE_LICENSE_KEY=<key>` (+ `PULSE_LICENSE_PUBKEY=<your pubkey>`) in `deploy/.env`
  and tell a session; it restarts pulse and live-verifies the beacon → QoE chain.
- Until then QoE/beacon data does NOT flow in prod (CI covers it with a mock license).

## 🟢 Optional / your policy call

- **DASH muxing** — see the NEW note above (real-MPD fixture capture).
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
*Status snapshot (2026-07-10, S13 close): GA v0.2.0 live + healthy; main is ahead with
D-068 + D-070 + D-072 + D-073 — all await your "ship v0.3.0". Dependabot queue zero. Go
coverage 74.0% (floor 70.2); web 62.68/58.78/51.54 (gates 59/54/45); sdk 3.52 KB. All four
probe protocols now have real probes (HLS + DASH full; WebRTC signaling; RTMP handshake);
WebRTC media path is S14. Plan of record: `ROADMAP-V2.md` (S14 next: pion media path, OIDC
phase-2 SPA login, CI promotions ≥07-23, conditional v0.3.0). Your list: 4 questions
(v0.3.0, CodeQL, PR-first, mobile SDKs) + U3 + optionals (DASH-muxing fixture, O7,
D-V2-1, O11, workflow-scope).*
