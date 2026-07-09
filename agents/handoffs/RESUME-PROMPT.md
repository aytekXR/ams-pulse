# Pulse — Resume / handoff prompt (SINGLE source of truth)

> **This is the one handoff doc.** It supersedes the previous separate "next-session" prompt (merged 2026-06-29,
> D-037); don't recreate a second handoff file — update THIS one + `decisions.md` each session.
> Pulse = self-hosted analytics/QoE/alerting for Ant Media Server. Repo: `/home/aytek/repo/ams-pulse`
> on VPS `161.97.172.146`. Full decision log: `agents/handoffs/decisions.md` (D-001…D-057 + session notes, binding).
> **Plan of record: `agents/handoffs/ROADMAP.md`** (D-057; supersedes `PRODUCTION-READINESS.md`,
> which is kept for provenance only). Session prompts: `agents/handoffs/sessions/`. AMS operator guide:
> `agents/handoffs/AMS-INTEGRATION.md`. Go-live runbook + rollback: `deploy/runbooks/real-ams-go-live.md`.
> Operator creds/keys (gitignored, never commit): `oguz-testing.md`.

---

## ▶ START HERE (next session — execute `sessions/SESSION-07.md`)

**Session 2026-07-09(c) result: D-063 — S6 DONE: docs + Helm GA batch; G7 MET except LICENSE
(O5, operator legal call). Nothing in docs lies to an operator** (every claim command-verified
by its author AND adversarially re-derived). `pulse-s6-docs-helm` (19 agents: 4 scout→author→
verify/fix pipelines + WO-5 promotion auditor + cross-doc critic) → `pulse-s6-tail` (6 agents;
survived an operator rate-limit stop via journal-cache resume).
- **Docs truth pass:** productionize.md + real-ams-go-live.md → 5-overlay / stamped-build /
  `_FILE` truth (PULSE_LICENSE_KEY has NO _FILE); alerting.md → honest-QoE **3-case** semantics
  (nil reader → skip+WARN; reader error → stream skip+WARN; **no data → evaluates vs 0.0, NOT
  skipped, NOT silent** — a verifier killed the "silently skipped" claim), factory.go channel-key
  tables; AMS-INTEGRATION §4.5 B7 per-source URLs (+its §3.2 DC was 4-overlay + `up -d --build` —
  the cross-doc critic caught it; also 4 FAKE metric names in productionize.md).
- **NEW docs:** SECURITY.md, CHANGELOG.md (Keep-a-Changelog: v0.1.0 + Unreleased), deploy/runbooks/
  upgrade-rollback.md + monitoring.md (CH memory-WATCH signature greppable). LICENSE NOT drafted (O5).
- **WO-6 stale batch:** ARCHITECTURE §6 bcrypt/HMAC truth; install.md 4-tier table cell-by-cell
  from license.go (**business MaxNodes=5 < pro 10 is PRD §7.11 BY-DESIGN — do not "fix"**) +
  env-only config truth (YAML parser exists but UNWIRED — main.go HOOK(BE-02); pulse.yaml is
  silently ignored); beacon-sdk.md re-MEASURED (3.52 KB gzip, 65 tests).
- **Helm parity (`bcdd3b8`):** ghcr.io/aytekxr/ams-pulse ref, CH auth via Secret, backup CronJob
  (compose-sidecar mirror), `optional: false`, NOTES.txt; goldens ×3 regenerated red-diff-first;
  chart stays EXPERIMENTAL (D-002 waiver — no cluster).
- **Promotion (WO-5): NOT DUE** (2026-07-09 < 2026-07-23), recorded w/ job-level evidence:
  web-e2e streak broke ONCE at `ba56c6e` run 28984417114 — deterministic D-061 spec-gating gap
  (csp/render-500 specs ran ungated in plain web-e2e), fixed `ecfc25c`, NOT a flake → green
  streak restarts 2026-07-09; csp-e2e 7/7; CodeQL first green `5dacb7d`, bake day 0. **Both
  clocks end ~2026-07-23 → the session running on/after that date promotes web-e2e + csp-e2e
  (FULL-LIST PUT; drop csp continue-on-error); CodeQL only ≥1wk green + operator OK.**
- **⚠️ Process incident → NEW §12 rule:** the wo6 fixer `git restore`d two files carrying
  OTHER agents' uncommitted verified work (false-positive "out-of-scope" flag on a shared dirty
  tree) — recovered BYTE-EXACT by replaying all 19 Edit calls from agent transcripts. Subagents
  never revert shared-tree files; ORCH commits early per scope.
Gates: helm lint + 3 goldens no-drift (alpine/helm:3.17.0, CI-faithful); full `-race` 24 pkgs
0 FAIL / 2 domain npx skips, total **73.2%** (floor 70, no Go touched); link-check 8 docs 0 dead;
secret scans clean. Commits `bcdd3b8` `f1a624b` `58e318f` `8627f05` `cc6b71c` `fff3315` `352b7d7`
+ handoff. ci 28993029934 + e2e 28993029982 + codeql 28993029935 on `352b7d7`: all GREEN
(`gh run watch --exit-status` ×3 = 0). Full evidence: D-063.

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-07.md` and execute it** (S7 GA gate:
9-scout re-audit vs G1–G8, A10 load smoke, promotion duty if ≥2026-07-23, GA declaration or
punch list; tag v1.0.0-vs-v0.2.0 is an operator call).

**Standing numbers (2026-07-09 post-S6):** Go total **73.2%** (floor **70.0**, unchanged — no Go
touched); web 76/72/45 + guard; SDK 62/73/70 (size 3.52 KB re-measured, webrtc.ts 20.1% still the
known gap); conformance 51/52 + 1 waived; only 2 skips (domain npx). Prod **`v0.1.0-25-gbc15d43`**
healthy — docs/Helm session, NO rollout needed. G-status: G1 ✅(−O7) G2 ✅ G3 ✅ G4 ✅ G5 partial
(promotion clocks) G6 ✅ **G7 ✅ except LICENSE (O5)** G8 operator. Operator queue: U3 license,
U5 browser/CSP check, O3 AMS-side webhook config (→O4), O5 LICENSE pick, O7 GHCR visibility
(re-verified still private 2026-07-09), O8 21 dependabot PRs, O11 Slack-webhook rotation + other
session's local reset.

---

## ▶ prior session context (2026-07-07(c) — e2e backfill, superseded by ROADMAP)

**Session 2026-07-07(c) result: `pulse-e2e-backfill` is COMPLETE (D-055 + D-056).** Two workflows
(13 + 7 agents), all verifiers green. Verify with `git log --oneline -6`:
- **D-055 `001bcbe`+`3882952`+`a3cb351`** — e2e.yml now asserts A1 alert→history (fires in ~4s), A3
  health_score 100→50 transition (new mock-ams `/control/set_bitrate`; equality assert, never unpublish),
  A2 ephemeral-Pro-license beacon→`/qoe/summary` (`qa/licensegen`, ≤120s bounded poll, real ~10s);
  Playwright skeleton `web/e2e/` (5 specs; CSP spec skipped → Caddy-fronted phase 2) + non-required
  `web-e2e` ci job. ⚠️ Plan correction that MUST survive: normalize.go:79 divides wire bitrate by 1000 —
  mock wire 2000000→health 100, 400000→50. On this VPS run Playwright via
  `mcr.microsoft.com/playwright:v1.61.1-noble` (host lacks chromium libs, no sudo).
- **D-056 `0240a29`** — the e2e's faithful repro EXPOSED two pre-existing bugs, both fixed: (1) beacon
  ingest always-401 post-D-052 (adapter used plain-SHA-256 `GetTokenByHash`; now raw-token
  `LookupIngestToken` → HMAC-aware `meta.LookupToken` + kind + NEW expiry guard, 6 TDD adapter tests);
  (2) mock-ams still served pre-D-029 un-prefixed broadcast paths → every poll 404'd (even the OLD e2e
  overview assert was silently broken; e2e only runs on PRs). ⚠️ **Prod runs the pre-D-056 image** — no live
  impact (beacon is Pro+-gated, U3 pending); ship with the next prod rollout.
Coverage 59.4% → **59.5%**; full -race suite 24 pkgs, 0 FAIL / 0 SKIP. Detail: `decisions.md` D-055/D-056.
Do NOT re-do any of this. E2E-TEST-PLAN.md phase-2 leftovers: caddy-fronted CSP/Playwright job,
delivery_failure e2e, promote web-e2e to required after ~2 weeks green.

~~FIRST ACTION: pulse-test-backfill~~ **SUPERSEDED by D-057** — test backfill is ROADMAP S2/S3
(with CORRECTED per-package numbers; the debt list that stood here was stale). B7 → S5; backup
cycle-2 watch + the D-056-carrying prod rollout → SESSION-01 (WO-5).

### Operator-only actions (surface every session)
- **U3 — activate a Pro+ Pulse license.** Until then QoE/beacon data does NOT flow in prod; rebuffer/error-rate alerts
  run off the HealthScore proxy. (The e2e plan's mock license only covers CI.)
- **U4 — branch protection + a `v*` tag** (repo-admin; also retire the stale `ams-integration` ref).
- **U5 — open `beyondkaira.com` + `pulse.beyondkaira.com`**, confirm no CSP console errors.
- **point AMS at the webhook** — configure the AMS app(s) to POST lifecycle webhooks to
  `https://beyondkaira.com/webhook/ams` with the HMAC secret from `deploy/.env`. **The Pulse side is LIVE as of
  D-054** (smoke-verified: signed → 200, bad-sig → 401); only the AMS-console configuration remains.

**Binding (unchanged, hard-won):** Go ONLY in Docker `golang:1.25`, **mount the repo ROOT** (`-v <repo>:/repo -w /repo/server
-e GOFLAGS=-buildvcs=false`) or ~90 api tests silently `t.Skip` → false green (D-028). Api integration tests need
`-tags integration` + `/tmp/clickhouse` (the unit `-race` gate skips them). **No false-green:** a "flake" that never resolves
with more waiting is a deterministic bug — read the code, don't bump the timeout (D-042); verify adversarially; reproduce CI
faithfully via `gh`. Commit by **explicit path** only, never `git add -A`. `Verify → Commit → Handoff` (§11); update THIS
file + `decisions.md` (new D-0NN) each session. AMS web login is RESOLVED (D-036). The `brier` project is DROPPED (D-046) —
`Caddyfile.prod` is now plain committable Pulse config.

---

## 0. VERIFIED CURRENT STATE (facts, not assumptions)

- **Production is LIVE on a SELF-HOSTED AMS (D-034).** `https://beyondkaira.com` (apex) + subdomains
  `https://pulse.beyondkaira.com` (app) and `https://ams.beyondkaira.com` (AMS panel) — all real Let's Encrypt
  TLS via Caddy. Backend = operator-owned `antmedia` container (AMS Enterprise 3.0.3, `--network host`,
  `http://161.97.172.146:5080`), **NOT** test.antmedia.io. `/healthz` = ok (clickhouse/collector/meta_store);
  `/api/v1/live/overview` → `total_publishers:2` on LiveApp as of 2026-07-07(b) (one is the synthetic 2 Mbps
  `ams-teststream` container — `docker rm -f ams-teststream` once real streams suffice). The mock-ams seeded demo
  is **retired**. [re-verified by authed curl post-D-054 rollout].
- **AMS web-console login RESOLVED (D-036, 2026-06-29).** The AMS console MD5-hashes the password client-side, but
  both admin accounts were REST-provisioned (D-034) with the plaintext password, so the browser's hashed submission
  never matched. Fixed by re-provisioning `aytek@` + `admin@` with `MD5(realpassword)`; both now web-login, Pulse
  (plaintext) unaffected. Brute-force lockout = **2 tries → 5-min block, per-EMAIL not IP**. AMS is the **latest
  stable** (3.0.3 == Docker Hub `latest`); trial license valid to 2026-07-12. Opened the newly-created `pulse-test`
  app's `remoteAllowedCIDR` 127.0.0.1→0.0.0.0/0 (logs clean — every new AMS app defaults to 127.0.0.1). Values in
  `oguz-testing.md`.
- **Branch state (D-058, 2026-07-08): `main` is PROTECTED** (contexts contracts/server/web/sdk/docker-build/
  helm/compose, strict, 1 review, enforce_admins=false — owner direct pushes work; keep it that way while
  sessions push to main). `ams-integration` is DELETED (local+origin). Tag **v0.1.0** exists @ `1a701d6`;
  release pipeline proven (D-058). U4 is fully resolved.
- **Go suite green / coverage 73.2%** as of 2026-07-08 (full `-race` + coverage, **repo-root mount**,
  golang:1.25, after D-052…D-060; was 47.5% on 2026-06-28). Working tree is CLEAN — everything is committed and
  pushed; CI additionally enforces a `gofmt -l` gate, a **66%** coverage floor (D-053, ratcheted D-059/D-060)
  and a stamped-version docker-build assert (D-058). **Prod runs `v0.1.0-25-gbc15d43` (≥D-061/D-062)
  since 2026-07-09** — rule→channel alert delivery LIVE-PROVEN (D-062 WO-1 smoke), beacon public
  chain live (403 LICENSE_REQUIRED until U3), rollback tags `pulse-prod-pulse:pre-d061` (1a701d6)
  and `:pre-d058`.
- **The prod image embeds the web UI** (multi-stage `deploy/docker/pulse.Dockerfile`: `npm ci && npm run build` →
  embedded in the Go binary), so a passing go-live build implies the web build passed.

---

## 1. PENDING USER ACTIONS (only the operator can do these — persist every session)

| # | Action | Why it's blocked / needed |
|---|---|---|
| U1 | ✅ **RESOLVED (D-034).** Self-hosted AMS on this VPS; per-app `remoteAllowedCIDR=0.0.0.0/0` so Pulse polls cleanly (200). No external allow-list dependency. | (was: 8/16 apps 403'd the VPS on test.antmedia.io). |
| U2 | ✅ **RESOLVED (D-039, 2026-06-30).** `ci` workflow is GREEN (de-flaked `TestQuery_QoeSummary_RealStartupP50`, 15s→90s poll); verified via `gh` (run 28429722100, 7/7 jobs). | — |
| U3 | **Activate a Pro+ Pulse license** on `beyondkaira.com` (`PULSE_LICENSE_KEY`, see §5). | QoE/beacon ingest (F3) is gated to Pro+ (`CheckBeaconIngest` 403 on Free). Without it `beacon_events` stays empty; QoE features/alerts can't be exercised in prod. *(This is a Pulse license — separate from the AMS license.)* |
| U4 | ✅ **RESOLVED (D-058, 2026-07-08).** Branch protection live (API 200) + v0.1.0 released (run 28911789088, cosign tlog 2110636506). NEW follow-ups: **O7** make the GHCR package public (or `gh auth refresh -s read:packages`) so pulls + `cosign verify` work; **O8** review the first dependabot PRs. | — |
| U5 | **Open `https://beyondkaira.com` AND `https://pulse.beyondkaira.com` in a browser; confirm the SPA renders with no CSP console errors on each** (Caddy serves both — apex via the catch-all, subdomain via its own block, so they can fail independently). | The agent can't run a real browser; CSP is browser-enforced. Report any violation → instant fix. |
| U6 | ✅ **DONE (2026-06-30).** `gh` is installed + authed (account `aytekXR`, ssh). The CI blind spot is gone — the agent now reads Actions directly (so it can also do U4). | — |

---

## 2. DONE (verified) vs MISSING (backlog) — no "done" without verification

**DONE — verified live or by green test:** real-AMS go-live (D-031); real-AMS wire correctness — bitrate
bps→kbps, FPS-redistribution, QoE fields, `terminated_unexpectedly`, WebRTC single-track (D-029v/D-030);
`maskDSN` password-leak fix (D-031); aggregator honors configured bitrate target (D-031); cookie-session auth +
per-app paths + multi-app keying (D-029); `golang:1.26`→`1.25` (D-032); subdomains + Caddy TLS (D-034/D-035);
AMS web-console login (D-036); `ams-integration` is now contained in `main` (branch divergence resolved).

**MISSING / NOT DONE (actionable backlog — detail in `PRODUCTION-READINESS.md`):**
- ✅ **Silently-stubbed features — DONE (D-041):** alert test-fire now delivers (real `Send` via `buildChannelFromRow`,
  contract keys, `200 {accepted,message}`, sanitized error body); 3 license gates enforced (+`/qoe/ingest`, +TOCTOU
  mutex); standalone node card shows real identity (os/cores/java/version — AMS 3.x exposes **no** standalone cpu/mem via
  REST, a documented AMS limit, A9); WebRTC viewer QoE captured **and** surfaced as `viewer_*` on `/live/streams`.
  *(Still open: the `rebuffer_ratio`/`error_rate` alerts proxy from HealthScore, not real beacon data — needs actual
  beacon data → blocked on U3; tracked under QoE/beacon e2e in phase 4 (§4).)*
- ✅ **Webhook path — DONE (D-046 route + D-048 config/test).** Prod rollout + AMS-side webhook URL config pending.
- **Branch cleanup [P2]:** retire the stale `ams-integration` pointer; branch protection + `v*` tag (U4).
- ✅ **Reliability gaps — DONE + DEPLOYED (D-049…D-054):** alert retry + delivery_failure; backups w/ verified
  restore (sidecar live in prod); CH graceful drain; resource limits (bound, inspected); `alert_history`
  auto-prune (cap 1000).
- **Security:** ✅ B3 secrets `_FILE` + opt-in overlay (D-052); ✅ API tokens HMAC-SHA256 w/ legacy back-compat
  (D-052). Remaining [P3]: B7 per-source webhook secret (contract CR).
- **Feature completion (PRD) [P3]:** QoE/beacon e2e (needs U3); Postgres meta backend (HA); SSO/OIDC; mobile SDKs;
  native WebRTC/RTMP/DASH probes; white-label PDF logo.
- **Testing [P0 for prod-readiness]:** `query` + `store/clickhouse` unit still ~0%, no response-body contract
  tests. ✅ e2e deepened (D-055: alert→history, health transition, beacon→QoE) + Playwright skeleton +
  coverage floor (D-053). Remaining breakdown in §6.

---

## 3. IMMEDIATE NEXT STEPS (do in order — each with verification)

- **Step A — `golang:1.26`→`1.25`** ✅ DONE (D-032). Verify: `grep -rn golang:1.26 deploy/ .github/` → empty.
- **Step B — Merge `ams-integration` → `main`** ✅ EFFECTIVELY DONE (2026-06-29): `main` now contains `ams-integration`
  (`git log main..ams-integration` empty). Remaining: **delete the stale `ams-integration` branch** (local + origin
  after a final diff confirms 0 unique commits), drop vestigial `AMS_LOGIN_*` from `deploy/.env.example`, add commented
  `PULSE_AMS_APPLICATIONS=` + `PULSE_INGEST_TARGET_BITRATE_KBPS=`.
- **Step C — Caddy `/webhook/*` route** ✅ DONE (D-046 route + D-048 config + D-054 live smoke: signed POST → 200).
  §3 is now fully retired — current next steps live in ▶ START HERE above.

---

## 4. BACKLOG = WORKFLOW-DRIVEN PHASES (orchestrate EACH phase as a Workflow)

> **D-057: this phase list is superseded by `ROADMAP.md` §3 (sessions S1–S7)** — kept for history.
> Mapping: phase 2 → S2/S3, phase 4 → S5 + post-GA backlog; release/dockerization work (new) = S1;
> e2e/CI hardening = S4; docs/Helm = S6; GA gate = S7.
1. ✅ **`pulse-p1-gaps`** — DONE (D-041): alert test-fire real delivery, 3 license gates enforced (+`/qoe/ingest`, +TOCTOU
   mutex), standalone node honest identity (AMS 3.x has no standalone cpu/mem via REST), WebRTC viewer QoE surfaced as
   `viewer_*`, `PULSE_ALLOWED_WS_ORIGINS` wired. Two adversarial-verify rounds.
2. **`pulse-test-backfill`** — TDD coverage to every level + enforced gate (3 sub-workflows: Go unit, web coverage
   gate, e2e+contract). See §6/§7.
3. ✅ **`pulse-prod-harden`** — DONE + DEPLOYED (D-048…D-054): webhook path, alert retry, backups, CH drain,
   B3 secrets `_FILE`, token HMAC, `alert_history` pruning, resource limits, SecretKey fail-closed. Still open
   from the original list: Trivy/SBOM, request-ID middleware (fold into phase 2/4 as convenient).
4. **`pulse-feature-complete`** — QoE/beacon e2e (after U3), AMS version surfacing, anomaly expansion, native probes,
   white-label PDF, B7 (contract CR), SSO/OIDC, mobile SDKs, backup sidecar, Postgres backend.

---

## 4a. `pulse-p1-gaps` — ✅ EXECUTED & VERIFIED (D-041, 2026-06-30)

> **DONE.** All 4 items below were implemented TDD + closed through **two adversarial-verify rounds**. The verify rounds
> overturned several of the round-1 "green" results (false-positive tests): item 1 read internal keys not contract keys
> (`webhook_url`/`email_to`/`telegram_chat_id`) and leaked secrets in the 502 body; item 3's premise was wrong — real AMS
> 3.x `/rest/v2/system-status` has **no cpu/mem**, so it now reports honest node identity (os/cores/java/`GetVersion`)
> instead; item 2 missed the `/qoe/ingest` gate + had a TOCTOU race (now mutex-guarded); item 4 was dead data (now exposed
> as `viewer_*` on `/live/streams`). The original scouted plan is kept below for provenance. **Do not re-run this workflow.**


Scouted by a read-only fan-out (4 agents); file:line below were read, not guessed. **Treat the approach as the plan,
not verified code — each item is TDD red→green (write the failing test FIRST, watch it fail, implement, watch it pass)
and re-confirmed against the live tree during implementation.** Launch as the `pulse-p1-gaps` workflow: one
disjoint-scope author per item (scopes are non-overlapping → safe to run in parallel), then ORCH gates (full `-race`
repo-root mount, §8) + commits by explicit path, then re-confirm CI green via `gh run watch`.

1. **Alert test-fire actually delivers** · scope `server/internal/api`
   - Now: `handleTestAlertChannel` (`server.go:1234-1243`) returns 202 and **never calls `Send()`**; the ready helper
     `alert.TestFireChannel` (`alert/evaluator.go:652-680`) is unused; no `buildChannelFromRow` exists.
   - Fix: add `buildChannelFromRow(store,row)` (decrypt `ConfigEnc`, switch `row.Type` → `channels.New{Slack,Webhook,
     Telegram,PagerDuty,Email}Channel`) + call `alert.TestFireChannel` in the handler; 200 on delivery, 5xx on failure.
     Channel impls + `Send` signatures in `alert/channels/*.go`.
   - Red test (`api/wave2_test.go`): POST `/alerts/channels/{id}/test` at an `httptest` webhook sink → assert the sink
     RECEIVED a body (fails today). Verify: `go test ./internal/api/... -run TestHandleTestAlertChannel`.

2. **Enforce the 3 license gates** · scope `server/internal/api/server.go` + new `license_gates_test.go`
   - Now: `CheckDataAPI`/`CheckNodeLimit`/`CheckPrometheus` (`license.go:288/250/347`) are **defined but never called** →
     Free tier 200s on `/analytics/{audience,geo,devices}`+`/qoe/summary`, registers unlimited sources, scrapes `/metrics`.
   - Fix: `if err := s.lic.CheckX(); err != nil { writeError(403,"LICENSE_REQUIRED",…); return }` at the top of
     `handleAudienceAnalytics(908)/handleGeoAnalytics(941)/handleDeviceAnalytics(961)/handleQoeSummary(982)` [DataAPI];
     `handleCreateSource(1316)` count `ListAMSSources+1` vs `CheckNodeLimit`; `handleMetrics(672)` `CheckPrometheus`.
     Pattern: `handleReportUsage` (`reports_wave2.go:26-29`).
   - Red test (`api/license_gates_test.go`, pattern `v3b_guard_test.go`): Free-tier request that should 403 (200s today).

3. **Standalone node card (`SystemStats`)** · scope `server/internal/collector` (BE-01)
   - Now: `SystemStats()` (`amsclient/client.go:532-541`, GET `/rest/v2/system-status`) has **0 callers**; for a
     standalone AMS, `ClusterNodes()` 404→nil → 0 `node_stats` → `snap.Nodes` empty → `FleetNodes()`=`[]` → blank card.
   - Fix: in `restpoller.poll()` (`restpoller.go:123-153`), when `ClusterNodes` returns nil, call `SystemStats()` + a new
     `NormalizeSystemStats` (`normalize.go`) → emit a `node_stats` event. `aggregator.onNodeStats` + `query.FleetNodes`
     already consume it (CPU/Mem wired).
   - Red test (`restpoller/standalone_node_stats_test.go`): mock AMS 404 on `/cluster/nodes` + `{cpuUsage,…}` on
     `/system-status` → assert an `EventNodeStats` with `cpu_pct` is emitted.

4. **WebRTC viewer QoE (`EventWebRTCClientStats`)** · scope `collector/aggregator` + `domain/types.go` + `cmd/pulse`
   - Now: aggregator `OnServerEvent` switch (`aggregator.go:115-134`) has **no case** for `EventWebRTCClientStats` → every
     `webrtc_client_stats` event (`restpoller.go:185-195`, `NormalizeWebRTCStats` `normalize.go:163-190`) is dropped;
     `domain.LiveStream` (`types.go:279-299`) has no viewer-QoE fields.
   - Fix: add `ViewerRTTMS/ViewerJitterMS/ViewerLossPct` to `LiveStream` + a `case domain.EventWebRTCClientStats:
     a.onWebRTCClientStats(ev)` handler that writes rtt/jitter/loss into the stream snapshot. **`PULSE_ALLOWED_WS_ORIGINS`:**
     `api Config.AllowedWSOrigins` (`server.go:70`) is consumed but never set — add the field to `EnvConfig` (`config.go`)
     + wire in `serve.go` `apiCfg` (~295-300).
   - Red test (`aggregator/aggregator_test.go`): feed publish-start + `webrtc_client_stats` → assert snapshot has `ViewerRTTMS` etc.

Full per-item detail (current behavior, fix, red test, verify cmd) was captured by the scout — re-scout cheaply with the
same fan-out if stale. Cross-check scopes against `agents/manifest.yaml` single-writer map before launching.

---

## 5. INTEGRATION KEYS (operator provides any subset; agent wires + verifies each on staging first, then prod)

Agent stores in `deploy/.env` (gitignored), wires, and verifies **real** behavior end-to-end. **Never commit keys.**
⚠️ Wire each alongside fixing the **stub the key would otherwise hide** (alert test-fire no-op; the 3 unenforced
license gates) — TDD each.

| Capability | Provide | Unlocks |
|---|---|---|
| **Pulse license** (Pro+/Business/Ent) | `PULSE_LICENSE_KEY` (or signed file + `PULSE_LICENSE_PUBKEY`) | QoE/beacon ingest (U3), anomalies, data API, probes, reports, Prometheus, multi-tenant — today gated to Free |
| **Email alerts** | SMTP host/port/user/pass (or SES/SendGrid key) | email alert delivery |
| **Slack alerts** | Slack incoming-webhook URL | Slack alert delivery |
| **PagerDuty** | routing/integration key | PagerDuty alert delivery |
| **Telegram** | bot token + chat id | Telegram alert delivery |
| **Generic webhook** | target URL + shared secret | webhook alert delivery |
| **S3 report export** | `PULSE_S3_ACCESS_KEY_ID`/`_SECRET_ACCESS_KEY`/`_BUCKET`/`_REGION`(/`_ENDPOINT`) | CSV/PDF report storage |
| **Geo enrichment** | MaxMind license key → GeoLite2-City.mmdb (`PULSE_GEO_MMDB_PATH`) | viewer country/region |
| **Prometheus** | `PULSE_METRICS_TOKEN` (self-generate) | authed `/metrics` |

Implemented alert channels: **email, slack, pagerduty, telegram, webhook**.

---

## 6. TEST & CI HARDENING (so breakage is caught in CI) — orchestrate as workflows, TDD red→green

> ⚠️ **D-057: the per-package numbers below are the 2026-07-07 baseline and several are now WRONG**
> (license 91.5, channels 74.1, config 74.5, meta 61.9, clickhouse unit 61.8, logtail 92.1 as of the
> 2026-07-08 audit). Use **ROADMAP §1/§4** as the current table; S2/S3 own this section's work.

Baseline coverage: total **59.5%** as of 2026-07-08 (was 47.5% on 2026-06-28); ci.yml enforces a 58% floor +
gofmt gate (D-053) — ratchet the floor as coverage climbs.

**ZERO unit coverage (write tests FIRST):**
- `internal/query` **0%** — powers every dashboard chart + API read (highest blast radius). Unit-test with a mock Conn.
- ~~`internal/config` 0%~~ ✅ covered by D-052 (secrets + validation tests); keep extending failure paths.
- `internal/store/clickhouse` **0% unit** (integration covers only ~3/12 query methods) + `.../migrations` **0%**.
- `cmd/pulse` **1.2%** — serve/migrate/diag wiring.

**LOW + critical:** `internal/license` **36.9%** (billing/tier gates = revenue), `store/meta` **29.7%**,
`collector/logtail` **37.5%**, `internal/api` **52.2%**, `alert/channels` **56.8%**.
**STRONG (keep ratcheting):** collector/ingest 85, cluster 89, sessions 81, anomaly 76, amsclient 76, restpoller 72,
alert 72.

**Priority (critical-business-logic-first):**
1. `license` 37→≥85 **and ENFORCE** the 3 gates + alert test-fire real `Send()`.
2. `query` 0→≥70 (mock-Conn unit) — analytics behind every chart.
3. alert firing→delivery (`channels` 57→≥80). ✅ The alert→history e2e gap is CLOSED (D-055, exactly the
   snapshot-present-metric approach: `ingest_bitrate_floor` lt 99999 → firing history row ≤30s). Still open:
   delivery_failure e2e (webhook channel at a dead URL → history row; E2E-TEST-PLAN phase 2) + channels unit depth.
4. `config` 0→≥80 — all env vars + failure paths.
5. `store/clickhouse` + `meta` — unit + expand integration to all query methods.
6. AMS wire **fixture-replay regression** pinning D-029/D-031 (bps→kbps, FPS-redistribution, `terminated_unexpectedly`,
   WebRTC single-track).
7. **De-flake `TestDiscovery_NewNodeVisible`** (`internal/cluster/discovery_test.go:116`, observed D-041): 60ms (3×20ms)
   latency budget is too tight on a CPU-contended/2-vCPU runner (measured 68.8ms once under whole-suite `-race`; 3/3 pass
   unloaded). Loosen the budget like D-039 did — a real future CI-red risk.

**CI gaps to close (`.github/workflows`) — the "see breakage in CI" asks:**
- ✅ **Coverage gate** — DONE (D-053): floor 58, ratchet as totals climb. Per-package regression check still optional.
- ✅ **Playwright browser e2e** — SKELETON DONE (D-055): `web/e2e/` 5 specs (auth gate in-place, dashboard zero
  console errors, 500-row virtualization, 401→gate; CSP spec skipped). Phase 2: caddy-fronted CSP job, promote
  `web-e2e` to required after ~2 weeks green.
- **ADD response-body contract tests** (kin-openapi) in `internal/api`: assert real responses conform to
  `contracts/openapi/pulse-api.yaml` (CI only lints the spec today, never the responses).
- **ADD web coverage threshold** (`vitest --coverage` gate).
- ✅ **e2e.yml DEEPENED** (D-055): alert fires→history, health 100→50 transition, beacon→QoE under an ephemeral
  Pro license. Still open: delivery_failure e2e, real-AMS fixture replay.

---

## 7. TDD ENFORCEMENT (BINDING — bias toward test coverage over implementation speed)

**Every change follows red→green→refactor: write the failing test FIRST, watch it fail, implement, watch it pass.**
For each unit of work produce tests at ALL applicable levels (do not stop at "unit"):

| Level | What it asserts | Where |
|---|---|---|
| **Unit** | pure logic, table-driven, both branches | `*_test.go`, `*.test.ts(x)` |
| **Integration** | real ClickHouse/sqlite via the Go harness (`-tags integration`, `/tmp/clickhouse`) | `*_integration_test.go` |
| **Contract** | HTTP response bodies validated against `contracts/openapi/pulse-api.yaml` (kin-openapi) | `internal/api/*_contract_test.go` |
| **Functional** | a feature's user-visible behavior end-to-end through the API (publish→visible, alert→history) | `e2e.yml` steps + api tests |
| **E2E (browser)** | dashboard render, auth redirect, CSP header, large-table virtualization | `web/e2e/*.spec.ts` (Playwright — NEW) |
| **Regression** | a fixed bug stays fixed (every D-0NN fix gets a pinning test) | co-located with the fix |
| **Edge-case** | empty/zero/max/null/unicode/pagination boundaries | per package |
| **Failure-path** | timeouts, 4xx/5xx, drop-on-full, retry exhaustion, decode errors | per package |

**Coverage gate (must not regress; the three 0.0% packages must reach ≥60%):**
```
sg docker -c 'docker run --rm -v /home/aytek/repo/ams-pulse:/repo -w /repo/server -e GOFLAGS=-buildvcs=false -e CGO_ENABLED=1 golang:1.25 sh -c "go test -race -coverprofile=cover.out -covermode=atomic ./... && go tool cover -func=cover.out | grep -E \"^total|0.0%\""'
```
**Prioritize critical business logic first:** (1) license/tier enforcement, (2) alert firing + delivery, (3) ingest
health scoring, (4) AMS wire decode/normalize, (5) the query layer. Report coverage in every handoff.

---

## 8. VERIFICATION WORKFLOW (BINDING — every implementation runs ALL of these before "done")

1. **Build:** `go build ./...` (CGO_ENABLED=0) + `cd web && npm run build`.
2. **Lint:** `cd web && npm run lint`; Go `gofmt -l` (must be empty) + `go vet ./...`.
3. **Type-check:** `cd web && npm run typecheck` (or `tsc --noEmit`).
4. **Test (race):** `go test ./... -race -count=1` **repo-root mount** (D-028: server-only mount silently skips ~90 api
   tests → false green). Confirm **0 FAIL, 0 unexpected SKIP**.
5. **Coverage:** the gate command in §7; attach numbers to the handoff.
6. **Contract drift:** `cd web && npm run gen:api` then `git diff --exit-code` (generated types match spec);
   `redocly lint` + `ajv` on event schemas.
7. **Staging verify:** bring the change up on an **isolated compose project** (NOT pulse-prod) and curl the affected
   endpoints. Never verify on prod first.
8. **Deploy smoke (after a prod change):** `/healthz` ok via `--resolve`; affected endpoint returns expected real
   data; `pulse logs` shows no 401/403/decode/login errors; for migrate, DSN masked (`:xxxxx@`).
9. **Independent/adversarial re-check:** default to "refuted" until reproduced on a fresh build (D-013/017/019). A
   verify harness that silently skips == no verify (D-028).

---

## 9. WORKFLOW SUGGESTIONS (prefer workflows; break large tasks into small verifiable ones)

- **Feature:** `pulse-feature-<name>` — fan out disjoint-scope authors → TDD tests → adversarial verify → ORCH gate →
  ORCH commit by explicit path.
- **Testing:** `pulse-test-backfill` — per-package finder measures coverage, authors the missing unit/edge/failure
  tests TDD-style, re-measures; a completeness critic asks "which exported fn has no test?".
- **Deployment:** `pulse-deploy-<target>` — pre-flight (config -q + login) → isolated staging verify → prod swap →
  post-swap smoke → handoff. (Pattern: `deploy/runbooks/real-ams-go-live.md`.)
- **Monitoring:** `pulse-monitor` — periodic poll of `/healthz` + `/live/overview` + `pulse logs` for AMS wire drift /
  403 storms / decode errors; surface regressions.
- **Rollback:** `pulse-rollback` — re-point pulse to the prior image/overlay (no `-v`), restore the prior state,
  smoke-verify. (Real-AMS rollback steps: runbook §5.)
- **Verification/audit:** `pulse-<x>-audit` — adversarial finders + refute pass (pattern proven in D-029v/D-031/D-032).

---

## 10. ASSUMPTIONS TO ELIMINATE (replace each with a verified fact; bias toward verification)

| # | Assumption (currently unverified or known-false) | How to eliminate |
|---|---|---|
| A1 | ✅ Resolved (2026-06-29): `main` now **contains** `ams-integration` (`main..ams-integration` empty). | Retire the stale `ams-integration` ref + branch protection (U4). |
| A2 | ✅ **VERIFIED GREEN (2026-06-30, D-039)** — `ci` all-green (run 28429722100) after de-flaking the QoE rollup test (15s→90s); readable via `gh` (U6 ✅), no longer an assumption. | Keep green: `gh run watch` after pushes. |
| A3 | ✅ Resolved: test-fire delivers (D-041); delivery retry (D-049); alert-fires→history **e2e in CI** (D-055, fired in ~4s live). Still open: delivery_failure e2e (phase 2). | Keep green via e2e.yml. |
| A4 | "Coverage is adequate." **FALSE** — 3 pkgs 0%, no gate. | `pulse-test-backfill` + coverage gate (§7). |
| A5 | "The 0.0% pkgs are covered by integration tests." Partially — only ~3 of ~12 query methods. | Add unit tests with a mock Conn (§6). |
| A6 | "QoE/beacon works in prod." **CI-VERIFIED under a mock Pro license** (D-055 beacon→rollup→qoe/summary e2e) and the always-401 bug it exposed is FIXED (D-056) — but prod still runs the pre-D-056 image AND has no license. | U3 + next prod rollout (carries D-056), then a live beacon smoke. |
| A7 | "The SPA renders / CSP is correct." **HALF-VERIFIED**: render/zero-console-errors/virtualization/auth now asserted by Playwright (D-055, route-mocked). CSP still unverified (Caddy-served; not reachable from `vite preview`). | U5 manual check + caddy-fronted Playwright CSP job (phase 2). |
| A8 | "Response bodies match the OpenAPI contract." **UNVERIFIED** — only spec-linting. | Response-body contract tests (kin-openapi). |
| A9 | "The real-AMS wire format is fully characterized." Partial — fixtures from one capture. | Watch pulse logs for decode errors; add a fixture-replay contract test; re-capture periodically. |
| A10 | "The teststream represents production load." **FALSE** — 1 low-bitrate publisher, 0 viewers. | Load/perf test (many streams/apps/viewers); VD-04 render-time at scale. |
| A11 | ✅ **RETIRED (D-059):** `TestIntegration_Migrations_IdempotentRun` applies all 4 migrations twice — second `Run` is a nil-error no-op, `schema_migrations` count unchanged. In CI on every push. | — |
| A12 | "ClickHouse shutdown loses no events." **FALSE** — 100ms sleep, not drain. | Drain-on-close + a no-loss test. |
| A13 | ✅ Moot (D-034): self-hosted AMS; `remoteAllowedCIDR=0.0.0.0/0` lets Pulse poll all apps (200). New apps default to 127.0.0.1 — open them. | — |

---

## 11. BINDING FLOWS — every workflow MUST end with these (user directive)

- **Verify** — independent/adversarial re-check of *every* claim against a running stack or fresh build; default to
  "refuted" until reproduced; **repo-root mount** or api tests silently skip (D-028). QA alone is not authoritative
  (D-013/017/019).
- **Commit** — by **EXPLICIT path**, per scope; never `git add -A/-u/.` (parallel agents share the tree — D-008/D-011).
  In a workflow, agents AUTHOR only; ORCH commits centrally (avoids `.git/index.lock` races). Message
  `<scope> D-0NN: <summary>` + evidence. Push when the user directs.
- **Handoff** — update **THIS `RESUME-PROMPT.md`** + `decisions.md` (new D-0NN) every session, then commit + push.

## 12. OPERATING PROTOCOL (binding — learned the hard way)

- **Orchestrate with the Workflow tool.** One phase = one Workflow: ORCH writes the plan + pre-approved CRs to
  `decisions.md`, fans out to disjoint-scope agents, then **independently gates**. Background work is harness-tracked —
  you're re-invoked on completion; don't poll-spin.
- **CodeGraph (operator-installed 2026-07-09, D-061).** Local index `.codegraph/` + CLI `~/.local/bin/codegraph`.
  Scouts/authors query the graph BEFORE grep/file sweeps: `codegraph explore "<question>"`,
  `codegraph node <sym>`, `codegraph callers <sym>` (blast radius). Put this in every agent work order
  (subagents use the CLI via Bash). **Closing protocol: `codegraph sync` after the last commit** (+
  `codegraph status` to confirm; stale lock → `codegraph unlock`).
- **Local compose stacks NEVER run from the real repo** — compose auto-loads `deploy/.env` (prod secrets) from
  the `-f` dir. Use a pristine working-tree copy:
  `git ls-files -co --exclude-standard -z | tar --null -T - -cf - | tar -C <scratch> -xf -` + unique `-p` name (D-061).
- **Anti-stall (D-016):** NEVER run `pulse serve`/`clickhouse server` in the foreground inside an agent. Use
  `docker compose up -d` (detached) + health polling; CH unit work via the integration harness. `timeout` on builds,
  `-timeout` on `go test`, vitest `run` not watch, `curl -m`. Long local repros: Bash `run_in_background: true`.
- **Single-writer scope map** in `agents/manifest.yaml`. **Contracts frozen (D-004)** — changes only via an
  ORCH-approved CR applied by INT-01 (OpenAPI + event schemas + migrations).
- **⚠️ Workflow/fork agents have Write+commit access** — a reviewer fork once auto-committed during a concurrent ORCH
  edit (D-030 process note). Scope reviewer agents read-only when ORCH is editing the same files.
- **⚠️ Subagents NEVER revert shared-tree files (D-063):** no `git restore` / `git checkout --` /
  `git stash` inside workflow agents — concurrent agents' UNCOMMITTED work shares the tree, and a
  verifier reading `git status` cannot tell foreign work from scope violations. Violations are
  REPORTED; ORCH decides and reverts. ORCH also commits early per scope to shrink the window.
  (A wo6 fixer once destroyed two files of verified work; recovered only via transcript-replay.)

## 13. HARD RULES (CLAUDE.md / ARCHITECTURE §3)

- AMS wire formats ONLY in `server/pkg/amsclient` + `server/internal/collector`; metrics in ClickHouse, config in the
  meta store, never crossed; web UI consumes ONLY generated public-API types; beacon ingest is hostile input.
- `CGO_ENABLED=0` for the shipping build (pure-Go sqlite); single binary `pulse serve|migrate|diag`; React 19 + RR7 +
  Vite + TS strict; recharts; no external fonts/CDNs. `go test -race` needs `CGO_ENABLED=1` + gcc.
- **4 tiers** (free/pro/**business**/enterprise) in the contract enum + `internal/license/license.go` (D-014).
- Deploy fixes live in `deploy/`. Base `docker-compose.yml` stays clean (`expose:`, no host ports); exposure in
  overrides. Prod stack = `base + hardened + prod-tls + real-ams + backup` (5 overlays since D-054 — see §14).

## 14. ENVIRONMENT (VPS)

- **Ubuntu 24.04 VPS `161.97.172.146`**, Docker 29 + Compose v5. **`go` is NOT on PATH** — run Go only in Docker
  (`golang:1.25`). node 20 + npm 10 on PATH. **`gh` IS installed + authed as owner `aytekXR`** (U6, 2026-06-30 —
  the old "`gh` NOT installed" note was stale, corrected D-057).
- **⚠️ For `go test` mount the REPO ROOT** (`-v /home/aytek/repo/ams-pulse:/repo -w /repo/server -e
  GOFLAGS=-buildvcs=false`): a `server/`-only mount makes `metaDDLPath` escape the mount → `t.Skip` →
  skip-counts-as-pass false green (~90 api tests). Confirm **0 SKIP** for api.
- **Docker:** user `aytek` is in `docker` group but stale in non-login shells → prefix `sg docker -c "…"`. `sudo` needs
  a password → ask the user via the `! <cmd>` prompt for privileged ops. For host-root debugging without sudo, run a
  privileged container in the host netns (e.g. `docker run --rm --net=host --cap-add=NET_RAW corfr/tcpdump …`, D-036).
- **Real-AMS prod ops** (run from repo root): `DC="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml
  -f deploy/docker-compose.backup.yml --env-file deploy/.env"` (backup overlay is part of the standing combo
  since D-054 — omitting it on `up -d` would REMOVE the backup sidecar). Status: `sg docker -c "docker compose $DC ps"`. Admin token: in `oguz-testing.md`
  (gitignored) — persisted in the `pulse-prod_pulse-data` volume; **never `down -v` that volume.** TLS check: always
  `--resolve beyondkaira.com:443:161.97.172.146` (VPS DNS is stale). Rollback: runbook §5.
- `deploy/.env`, `*.db*`, `oguz-testing.md`, `web/pulse_secret.key` are gitignored — never commit.
- ~~brier Caddyfile warning~~ RETIRED (D-062 verified): D-046 removed the brier block + `.bak-brier`
  file; `deploy/config/Caddyfile.prod` is clean, tracked, and uses `{$AMS_UPSTREAM}` since D-062.
- ⚠️ **Concurrent-session hazard (learned D-062):** the operator may run a second Claude session in
  this repo. If HEAD moves or the tree dirties mid-session with work you didn't do, STOP and inspect
  before committing/pushing — a foreign unpushed commit once carried a hardcoded live secret (O11).
