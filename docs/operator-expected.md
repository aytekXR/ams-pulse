# Operator TODO — the items only YOU can do (updated at SESSION-17 close, D-079, 2026-07-11; rides S17's PR)

> **Audience: the human operator.** Ledger of record: `ROADMAP.md` §5 + `ROADMAP-V2.md` §4; this
> file is the actionable view, refreshed at every session close. When you finish an item, just
> tell the agent (or do nothing — every session start re-verifies each item automatically).
> **Never commit secret VALUES anywhere; `deploy/.env` and `oguz-testing.md` are gitignored.**

## ⚡ TL;DR — expected from you right now (2026-07-11, SESSION-17 closed)

> **Nothing is needed from you.** S17 executed your validation directive (D-078):
> the real-AMS test harness is BUILT (26 automated parity scenarios under
> `qa/realams/`, rerunnable any time via `make validate-realams-p0`) and the full
> P0 suite ran against your live AMS: **24 PASS / 2 SKIP / 0 FAIL.** Highlights:
> **Pulse saw stream start in 4 s and stream end in 7 s (the PRD promises 10 s)**;
> ingest bitrate parity within ±10%; WebRTC/RTMP/HLS probes green against your
> server; viewer counts matched AMS exactly. The 2 skips are honest "nothing to
> test against" cases, not failures (details in the S17 table below). The suite
> also caught and fixed its own false-green bug before trusting any result —
> every PASS is now backed by a fresh evidence file, never just an exit code.
>
> **One thing to confirm when you have a second (non-blocking):** your AMS's app
> inventory changed since S16 — 16 apps (8 IP-blocked) shrank to **4 apps, all
> open** (`LiveApp`, `WebRTCAppEE`, `live`, `pulse-test`), the old VoDs are gone,
> and `GET /rest/v2/applications/info` now returns HTTP 405. If YOU reset/
> recreated the AMS apps, all good — just say so; if not, tell a session and it
> will investigate. (The `antmedia` container shows a restart ~18 h before S17.)
>
> **FYI, two harmless test artifacts on your AMS:** S17 briefly enabled MP4
> recording on the `pulse-test` app to create ONE small test VoD (~20 s test
> pattern) as ground truth for the recording-gap validation — the setting was
> restored to off; the VoD stays as standing test evidence. Test streams named
> `val-*` may appear/disappear on LiveApp while validation suites run.

## 🔎 What SESSION-17 did (2026-07-11, closed — D-079)

| Area | Result |
|---|---|
| **Validation harness (your D-078 program)** | LANDED: `qa/realams/` — 26 automated scenarios that drive your AMS (real ffmpeg publishers, HLS viewers, headless-browser WebRTC viewers, probes) and cross-check every Pulse number against the AMS REST API. Rerunnable forever: `make validate-realams-p0`. |
| **P0 parity results** | **24 PASS / 2 SKIP / 0 FAIL.** Stream start visible in Pulse in 4 s, stream end in 7 s (PRD: ≤10 s). Bitrate parity ±10%. Viewer counts exact. Probes (WebRTC incl. rtt/jitter/loss, RTMP, HLS, DASH-404) all green live. Fleet card honest (no fake zeros for CPU/mem your AMS doesn't report). |
| **The 2 SKIPs (honest, not failures)** | (1) "IP-blocked app handling" — your AMS currently has no blocked app to test against; (2) "WebRTC viewer count" — the headless browser loaded the player page but playback never started; next session debugs it (needed for deeper WebRTC stats anyway). |
| **Trust hardening** | The suite's first run claimed 21 passes in under 4 minutes — impossible. Root cause found (a script exiting early made "didn't run" look like "passed"); now every PASS requires fresh evidence on disk. Three reusable shell pitfalls saved to agent memory. |
| **Your AMS drifted since last week (please confirm)** | 16 apps → 4 apps (all open), old VoDs gone, `applications/info` endpoint now rejects GET (405), HLS path form changed. All validation docs corrected to match reality. **If this reset wasn't you, tell a session.** |
| **Bugs filed (for the assessment)** | BUG-001: dead code around an AMS statistics endpoint (low). BUG-002: `recording_gb` always 0 because AMS 3.0.3 can't sign the `vodReady` webhook — the recording/billing gap; suggested fix (VoD REST polling) is on the program roadmap. |
| **UI polish (S16 leftovers)** | Info-blue text now theme-aware-ready (6 hardcoded colors → variables), 21 new unit tests (360 total). The light-mode blue + link-color need two tiny brandkit token additions — **proposal awaiting your/designer sign-off** (non-blocking): `agents/handoffs/proposals/D-079-linkbody-token-proposal.md`. |
| **Ship vehicle** | Everything rides ONE PR; prod untouched all session. Gates: lint/types clean, 360/360 unit, coverage up, browser suite 15/15. |

## (superseded) TL;DR at SESSION-16 close

> **Nothing new is needed from you.** S16 landed everything it promised — the **light
> theme + density modes**, the **WebRTC stats columns** on the Probes page, and the
> login-gate bug fix (your prod was never affected) — all gates green (339/339 unit
> tests, 15/15 browser tests, coverage up across the board), one PR, prod untouched.
> **PR #28 MERGED with all 15 CI checks green — including `web-e2e`'s first green in
> 13 runs, the in-CI proof of the login-gate fix.** (This merge-confirmation line rides
> S17's first PR — push budget.)
> **Your new validation directive was received and is now the plan of record (D-078):**
> the full 8-phase Pulse × AMS real-validation & product-fit program is planned under
> `docs/assessment/` and EXECUTION STARTS next session (S17) — building the real test
> environment that drives your AMS (streams up/down, viewers ramping) and
> auto-cross-checks every Pulse number against the AMS APIs. It will need your AMS
> instance healthy and reachable; nothing else from you to start.

**Your open items (unchanged — one):**

| Priority | Item | What to do |
|---|---|---|
| 👀 **finish the eyeball** | Browser-accept the re-branded UI (icon now fixed) | Hard-refresh (Ctrl+Shift+R) https://pulse.beyondkaira.com — log in with the fresh `plt_0352…` token at the BOTTOM of `oguz-testing.md` — check the look + console, then tell a session "UI accepted". **Tip: after S16's PR merges and a future rollout, you'll also get a theme toggle (light/dark) + density switch in the sidebar.** |

## ✅ Key hygiene — DONE (2026-07-11, S16 open, D-077)

You confirmed the private key is stored on your side ("I have stored the file for
myself") → `deploy/.env.bak-d076` was **securely shredded** at S16 open. The private
signing key now exists only in your vault; `deploy/.env` (live prod config with the
minted enterprise LICENSE — not the private key) is untouched and stays gitignored.

## 🔎 What SESSION-16 did (2026-07-11, closed — D-077)

| Area | Result |
|---|---|
| **Key hygiene (your say-so)** | Backup shredded at open — done (above). |
| **CI promotion audit** | Date gate still closed (opens 07-23) — but the audit found the `web-e2e` browser-test job **red for 12 straight runs**, silently (it's non-blocking during its bake period). Root-caused to a real bug, not flakiness. |
| **Real bug found & FIXED** | The SSO login change (S14) made the app treat *any* "200 OK" reply on its session-check as a valid login — even an HTML error/fallback page. In the wrong topology (stale server, misconfigured proxy) you'd see a broken dashboard instead of the login screen. Fixed with tests, **proven in the browser gate: all 15 end-to-end tests green**, including the 3 that had been failing for 12 runs. Your prod was never affected. |
| **Light theme + density (brandkit phase 2)** | LANDED: light/dark toggle (remembers your choice, follows OS preference by default), compact & wall-screen density modes, motion tokens + reduced-motion support. Every light-theme color verified EXACTLY against your brandkit tokens.json; link color follows the WCAG note (§2). |
| **Probes page** | LANDED: WebRTC network-quality columns (ICE state badge + RTT / jitter / loss) — a dash means "not measured", 0 means genuinely measured zero. |
| **Quality net** | 2 implementation workflows + 3 adversarial verifiers (they found 3 real issues — all fixed same-session) + the docker browser gate (found 3 more test bugs — fixed, 15/15). Coverage rose to 65.8/61.1/54.9 (gates 59/54/45). The session also survived a terminal crash mid-work with zero loss (recovered from persisted workflow state). |
| **Ship vehicle** | Everything rides ONE PR (PR-first, ≤2 pushes/session) and reaches prod with the next rollout you approve — **prod is untouched this session.** |

## 🚀 NEW — your validation directive is now the plan of record (D-078, starts S17)

You asked for a **real validation environment**: simulate a real customer using AMS +
Pulse — control AMS (create/start/stop broadcasts, ramp concurrent streams and
viewers, force failures/reconnects) and verify the effects in Pulse, cross-checking
every number against the AMS APIs automatically (mismatch = test FAILS with evidence),
plus the full product-fit ladder up to a marketplace-readiness report for the Ant
Media team. **Planned this session** under `docs/assessment/` (program README,
AMS-capability × Pulse-coverage map, test-environment design, scenario matrix,
session plan). **Execution starts S17.** From you to start: nothing — just keep your
AMS reachable. Heads-up: publisher/viewer load runs will exercise your AMS instance;
sessions will tell you before any load beyond a handful of test streams/viewers.

**New (non-blocking) items the program will eventually want from you:**

| When | Item |
|---|---|
| before S19 | **Ant Media marketplace contact** — the Phase-8 assessment needs the real listing requirements + revenue-share terms (PRD's 20–30% is unverified). When you have a contact/thread with the Ant Media team, tell a session. |
| whenever | **Kafka broker: available or planned?** Standalone AMS 3.0.3 exposes no CPU/mem/disk via REST — without Kafka, Fleet resource gauges stay empty forever. A yes/no shapes the roadmap priority. |
| whenever | **AMS trial license after 07-12** — you said "handled"; if any Enterprise feature does lapse, the validation surface shrinks (sessions will observe + report what 403s). |

**Everything else: nothing needed.** Optionals whenever: D-V2-1 ("build"/"wontfix"),
O7 GHCR-public, O11 rotation, `gh auth refresh -s workflow`.

## ✅ What SESSION-15c did (2026-07-11, D-076b — your two mid-accept reports)

| Area | Result |
|---|---|
| **Broken icon (your report)** | Root cause: the server only served `/assets/*` — every other asset (favicon, icons, manifest, logo) got the HTML page instead. Fixed with a regression test, merged via PR #27 (all 9 required checks), redeployed; `/favicon.svg` → `image/svg+xml` verified live. |
| **Login token (your question)** | Fresh admin token minted and appended to `oguz-testing.md` (bottom); an earlier mint lost to a shell-quoting slip was revoked, not orphaned. Login placeholder corrected `pulse_tok_…` → `plt_…`. |
| **Prod now** | `v0.3.0-4-ge8f8f5f`, healthy, `tier=enterprise`; rollback tags + backup stand. |

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
*Status snapshot (2026-07-11, S16 close): **prod = v0.3.0-4-ge8f8f5f + ENTERPRISE
license, live + healthy**; QoE/beacon, all four probe protocols (WebRTC through
rtt/jitter/loss), data API and anomaly detection all active in prod. CodeQL required;
PR-first active (9 contexts, enforce_admins). Dependabot queue zero. Go coverage 74.5%
(floor 70.2); web 65.80/61.13/54.85 (gates 59/54/45); sdk 3.52 KB. Plan of record:
`ROADMAP-V2.md` + **`docs/assessment/` (D-078 validation program — S17's primary
track)**; CI-promotion gate opens 07-23 (csp-e2e candidate; web-e2e ~07-25). Your
list: 👀 UI browser-accept + optionals (O7, D-V2-1, O11, workflow-scope).*
