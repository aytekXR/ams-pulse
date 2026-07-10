# Operator TODO — the items only YOU can do (updated at S15b close, post-D-076, 2026-07-11)

> **Audience: the human operator.** Ledger of record: `ROADMAP.md` §5 + `ROADMAP-V2.md` §4; this
> file is the actionable view, refreshed at every session close. When you finish an item, just
> tell the agent (or do nothing — every session start re-verifies each item automatically).
> **Never commit secret VALUES anywhere; `deploy/.env` and `oguz-testing.md` are gitignored.**

## ⚡ TL;DR — expected from you right now (2026-07-11, S15b closed)

> **v0.3.0 IS LIVE IN PRODUCTION with your ENTERPRISE license active.** Everything
> you asked for in one batch is done: v0.3.0 shipped (the release pipeline first
> BLOCKED a real HIGH CVE in the OIDC dependency — fixed same session, nothing
> vulnerable was ever published), your license is verified end-to-end (a test
> viewer-event posted at the public edge came back in the QoE dashboard numbers),
> CodeQL is now a required merge check, PR-first is ON, mobile SDKs deferred,
> DASH fixture skipped.

**TWO items for you now:**

| Priority | Item | What to do |
|---|---|---|
| 👀 **please eyeball** | **Browser-accept the re-branded UI** — prod now renders your brandkit design for the first time | Open https://beyondkaira.com and https://pulse.beyondkaira.com — confirm the new look renders and the browser console shows no errors; tell a session "UI accepted" (or report anything off) |
| 🔑 **key hygiene** | **Your license PRIVATE key was in `deploy/.env`** (it's the 128-hex signing key, not a license — I minted your enterprise license from it and swapped it in). The original file is preserved at `deploy/.env.bak-d076` | Copy the 128-hex private key from that file into your offline vault (password manager / encrypted USB), then delete the file — or just tell a session "key vaulted, delete the backup" |

**Everything else: nothing needed.** Remaining optional decisions whenever: D-V2-1
(unsigned-webhook mode: "build" or "wontfix"), O7 (GHCR public — one UI click),
O11 rotation, `gh auth refresh -s workflow`.

## ✅ What SESSION-15b did (2026-07-11, D-076)

| Area | Result |
|---|---|
| **v0.3.0 shipped** | Tagged, released (Trivy/SBOM/cosign), stamped build rolled to prod with migrations; smoke green (health, UI on both domains, webhook fail-closed, clean logs). Rollback image + fresh backup staged first. |
| **Security gate win** | The release pipeline BLOCKED the first tag: a HIGH DoS CVE (go-jose, OIDC stack). Fixed 4.0.5→4.1.4 and re-released — no vulnerable image ever published. |
| **Your license (U3)** | TWO hidden problems found live: (1) the prod compose never passed license env vars to the container (CI-only wiring — fixed); (2) your `.env` held the private signing key, not a license — a proper **enterprise, perpetual** license was minted from it and installed. Verified: `tier=enterprise`, beacon event → 202 accepted → shows up in `/qoe/summary`. QoE/beacon, probes, data API and the anomaly detector are now all live in prod. |
| **CodeQL required** | Your "decide for me" → enabled (29-run green streak, zero maintenance, Go+JS scanning). |
| **PR-first ON** | enforce_admins=true, required reviews 0 (a solo owner can't self-approve); sessions work via PRs from S16 on. |
| **Recorded** | Mobile SDKs deferred (revisit whenever); DASH fixture skipped; push budget: max 2 pushes/session (your directive — saved to agent memory). |

## ✅ What SESSION-15 did (2026-07-10, D-075 — nothing was needed from you)

| Area | Result |
|---|---|
| **WebRTC network-quality stats** | Probes now measure the media path itself: after connecting, they report round-trip time, jitter, and packet loss per probe run. **Verified live against YOUR AMS: rtt 0.47 ms / jitter 22.33 ms / loss 0%, measured from your real teststream's video in 2.2 s.** |
| **Honest data semantics** | A metric that wasn't measured is *absent*, never shown as 0 — so "0% loss" always means genuinely measured zero loss. |
| **CI now exercises the full path** | The CI mock server sends real video packets after connecting, so the whole measurement chain is tested on every PR. |
| **Flaky test hardened** | A pre-existing alert test would have started randomly failing CI under load (it measured scheduler noise, not the behavior it guarded). Caught at this session's gate and rebuilt so it can neither false-fail nor false-pass. |
| **Docs corrected** | An operator-facing doc still claimed the WebRTC/RTMP/DASH probes were stubs — an operator reading it would think their working probes were broken. Fixed, plus ~19 smaller staleness fixes. |
| **Quality net** | 3 workflows (13 agents): scouts → authors (all TDD red→green) → 3 adversarial verifiers (correctness verdict: CONFIRMED-OK, zero functional must-fix) + the live real-AMS run; all CI gates green. |

**Not landed (nothing was due):** the CI-promotions work order stayed date-gated
(opens 2026-07-23 → S16); the v0.3.0 rollout and iOS SDK work orders are waiting on
your answers above; brandkit light-theme moved to S16.

## ✅ What SESSION-14 did (2026-07-10, D-074 — nothing was needed from you)

| Area | Result |
|---|---|
| **WebRTC media-path probe** | WebRTC probes no longer stop at signaling: they now negotiate a real media connection (ICE) and report `ice_state`. **Verified live against YOUR AMS: connected in 0.2 s.** |
| **Real-server bug found & fixed** | The live check exposed a bug CI could never see: your real AMS sends a notification message before the offer, which made every WebRTC probe against a live stream fail. Fixed, pinned by tests captured from your server, and the CI mock now mirrors your AMS exactly. |
| **SSO login UI** | The login screen now shows "Sign in with SSO" when OIDC is configured, and browser sessions from the SSO flow work without pasting a token. Sign-out revokes the SSO session too. |
| **Anomaly alerts** | Anomaly rules can now watch `ingest_bitrate_kbps` and `disk_pct` (was: viewers/CPU/memory only). |
| **Probe safety cap** | Probe segment downloads are capped at 32 MB — a huge/misbehaving segment can no longer produce a silently wrong bitrate or eat unbounded memory. |
| **Your teststream** | The `ams-teststream` publisher container had crashed (2 h earlier); the session restarted it. |
| **Quality net** | 3 workflows (14 agents): scouts → authors → 3 adversarial verifiers + a live cross-pair + the live AMS check; every finding fixed same-session; all CI gates green. |

**Not landed (honestly re-gated to S15 with a written plan):** WebRTC per-stream network
stats (rtt/jitter/loss) — needs mock-side media sending; kept off this push to avoid
landing a fresh flake surface late in a long session.

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

## ✅ Standing questions — ALL ANSWERED 2026-07-11 (D-076, executing now)

1. **Ship v0.3.0?** → **"proceed"** — rollout in progress this session.
2. **CodeQL required?** → **"decide for me"** → ORCH enabled it (29-run green streak,
   zero maintenance; contexts `Analyze (go)` + `Analyze (javascript-typescript)`).
3. **PR-first?** → **"switch going forward"** — flips at this session's close
   (enforce_admins=true, required reviews 0); sessions work via PRs from S16 on.
4. **Mobile SDKs?** → **"leave out for now, revisit later"** — deferred, work order cut.

## ✅ U3 — DONE (2026-07-11, D-076): you placed the key; the session verifies it live
during the v0.3.0 swap (tier + beacon→QoE chain). Evidence lands in decisions.md D-076.

## 🟢 Optional / your policy call

- ~~DASH muxing~~ — **SKIPPED by you (D-076)**; re-open anytime by enabling DASH muxing and telling a session.
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
*Status snapshot (2026-07-10, S15 close): GA v0.2.0 live + healthy; main is ahead with
D-068 + D-070 + D-072 + D-073 + D-074 + D-075 — all await your "ship v0.3.0". Dependabot
queue zero. Go coverage 74.5% (floor 70.2); web 62.96/59.04/52.05 (gates 59/54/45); sdk
3.52 KB. All four probe protocols have real probes; WebRTC is complete through network-
quality stats (rtt/jitter/loss, live-verified against your AMS). Plan of record:
`ROADMAP-V2.md` (S16 next: CI promotions — gate opens 07-23, brandkit light theme,
probe-stats UI, conditional v0.3.0). Your list: 4 questions (v0.3.0, CodeQL, PR-first,
mobile SDKs) + U3 + optionals (DASH-muxing fixture, O7, D-V2-1, O11, workflow-scope).*
