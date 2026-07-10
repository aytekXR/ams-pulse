# Operator TODO — the items only YOU can do (updated at S15 close, post-D-075, 2026-07-10)

> **Audience: the human operator.** Ledger of record: `ROADMAP.md` §5 + `ROADMAP-V2.md` §4; this
> file is the actionable view, refreshed at every session close. When you finish an item, just
> tell the agent (or do nothing — every session start re-verifies each item automatically).
> **Never commit secret VALUES anywhere; `deploy/.env` and `oguz-testing.md` are gitignored.**

## ⚡ TL;DR — expected from you right now (2026-07-10, S15 closed)

> **NOTHING is required from you.** S15 finished autonomously — WebRTC probes now
> measure the actual network quality of the media path against your AMS: round-trip
> time, jitter, and packet loss (verified live against your server: rtt 0.47 ms,
> jitter 22.33 ms, loss 0% — measured from your real teststream's video). A flaky
> alert test that would eventually have caused false CI failures was also found and
> hardened. Your four standing questions are still open — answer whenever; the next
> session (S16) checks them first and adapts. Next session also opens the CI-promotions
> date gate (2026-07-23).

**Your open items, all non-blocking:**

| Priority | Item | What to say/do |
|---|---|---|
| 🟠 **unlocks prod rollout** | **Ship v0.3.0?** Prod still runs v0.2.0; main now carries SIX sessions of improvements: O(N²) fix, S11 features (PDF logo, anomaly rules, OIDC), Postgres backend, **your brandkit UI re-theme**, all four protocol probes (WebRTC now measures rtt/jitter/loss of the real media path), SSO login UI, bitrate/disk anomaly alerts | Reply "ship v0.3.0" — the session tags, rolls out with smoke + rollback ready, then pings you for the browser look |
| 🟠 answer by ~07-23 | **CodeQL as a required merge check?** (CI promotions land at S16; the date gate opens 07-23) | Reply "CodeQL required: yes/no" |
| 🟠 whenever | **PR-first cadence?** (drives enforce_admins re-arm) | Reply "PR-first" or "keep direct pushes" |
| 🟠 whenever | **Mobile SDKs — do you have (or plan) native iOS/Android apps?** S16 has an iOS-SDK work order that fires ONLY on your explicit yes. | Reply "need mobile SDKs: yes/no" (no = the item is cut from the roadmap) |
| 🟡 feature unlock | **U3 — Pro+ license in prod** (QoE/beacon data doesn't flow until then) | See §U3 below (self-serve minting) |
| 🟢 optional | Enable DASH muxing on an AMS app (NEW, see below) · O7 GHCR-public · D-V2-1 unsigned-webhook call · O11 rotation · `gh auth refresh -s workflow` | See §Optional below |
| 👀 later (after v0.3.0) | **Browser-accept the re-branded UI** (prod renders the OLD UI until v0.3.0 rolls out) | You'll be pinged with URLs to eyeball |

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

## 🟠 Standing questions (answer whenever, not blocking — FOUR, see TL;DR table)

1. **Ship v0.3.0?** — gates the prod rollout work order; release pipeline proven (v0.1.0,
   v0.2.0); rollback tags stand. Until then prod stays on v0.2.0 and your brandkit UI is
   not visible in prod.
2. **CodeQL as a REQUIRED merge context** when the CI promotions land (≥2026-07-23, S15)?
3. **PR-first cadence?** Today sessions push directly to main; `enforce_admins` stays off
   for that (rationale re-verified every session). Say "PR-first" to flip.
4. **Mobile SDKs needed?** iOS/Android beacon SDKs — S15 WO-F fires only on explicit yes.

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
*Status snapshot (2026-07-10, S15 close): GA v0.2.0 live + healthy; main is ahead with
D-068 + D-070 + D-072 + D-073 + D-074 + D-075 — all await your "ship v0.3.0". Dependabot
queue zero. Go coverage 74.5% (floor 70.2); web 62.96/59.04/52.05 (gates 59/54/45); sdk
3.52 KB. All four probe protocols have real probes; WebRTC is complete through network-
quality stats (rtt/jitter/loss, live-verified against your AMS). Plan of record:
`ROADMAP-V2.md` (S16 next: CI promotions — gate opens 07-23, brandkit light theme,
probe-stats UI, conditional v0.3.0). Your list: 4 questions (v0.3.0, CodeQL, PR-first,
mobile SDKs) + U3 + optionals (DASH-muxing fixture, O7, D-V2-1, O11, workflow-scope).*
