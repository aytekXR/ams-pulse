# Pulse — Production-Readiness ROADMAP (session-divided, TDD-enforced)

> **Plan of record** as of 2026-07-08 (D-057). Supersedes `PRODUCTION-READINESS.md` (2026-06-22 —
> keep for provenance, do not execute from it). Built on a 9-scout verified audit (coverage, CI,
> dockerization, stubs, contracts, web, docs, git/GitHub, live prod) — every claim below has
> file:line or command-output evidence recorded in D-057; nothing is inherited from stale docs.
>
> **Operator directive (binding):** production-readiness with TDD; dockerization/release path
> FIRST ("ready as soon as ready with dockerization"); work divided into sessions; **each
> session's prompt is written BEFORE the session starts, and every session ends by writing the
> next session's prompt** from this roadmap + what actually happened (§6 protocol).

---

## 0. How to use this file

1. Next session = the lowest-numbered session in §3 not marked ✅. Its ready-to-run prompt is
   `agents/handoffs/sessions/SESSION-NN.md` — start there, not here.
2. This file owns the SEQUENCE and exit criteria. Session prompts own the per-work-order detail.
3. When a session completes: mark its §3 entry ✅ with the D-0NN + commit refs, update §4/§5
   ledgers, then write the next `SESSION-NN.md` (template: `sessions/TEMPLATE.md`). A session
   that hasn't written its successor prompt is NOT done.
4. Scope changes (new gaps discovered, priorities shifted) are edited HERE first, then reflected
   in the next session prompt — never the other way around.

## 1. Verified current state (2026-07-08 audit)

| Dimension | Verdict (evidence in D-057) |
|---|---|
| Go tests | 24 pkgs, 0 FAIL, 0 races, **total 59.5%** vs CI floor 58.0. Weak: `cmd/pulse` 13.5, `internal/query` 18.5, `internal/api` 55.9, `collector/webhook` 58.1, `reports` 58.8, `domain` 0.0, `store/clickhouse/migrations` 0.0 (no test files). **RESUME §6's priority table is stale**: license is 91.5 (not 37), channels 74.1, config 74.5, meta 61.9, clickhouse unit 61.8, logtail 92.1 — those targets are already met. |
| CI | 8 ci.yml jobs green at HEAD `175095a`. Gaps: e2e.yml not on main pushes; `web-e2e` advisory (`continue-on-error`, node 20 vs 22); no dependabot/CodeQL. |
| Release pipeline | **Weakest GA dimension.** release.yml publishes `ghcr.io/aytekxr/ams-pulse` single-arch, ungated by CI, no Trivy, no SBOM/provenance, no cosign. No `v*` tag has ever been pushed — the pipeline has never run. Binaries self-report `dev/unknown` (no `-ldflags` anywhere: Dockerfile:24, Makefile:30, release.yml). |
| Dockerization (compose) | Production-shaped: non-root USER, HEALTHCHECK, digest-pinned CH, `_FILE` secrets, limits, backup sidecar. Gaps: golang builder stage (Dockerfile:18) + caddy (hardened.yml:36) float on tags; `.env.example` missing ~8 consumed vars; `Caddyfile.prod:92` hard-codes the VPS IP. |
| Helm | **Not installable**: values.yaml:13 references `ghcr.io/pulse-analytics/pulse:0.1.0` (never published; release publishes to a different org/repo). Missing: CH auth, webhook port 8092, backup equivalent; `PULSE_SECRET_KEY` secretKeyRef `optional: true` crashes at boot when absent. |
| Contracts | 52 operations in `pulse-api.yaml`; only **14 response-body-validated**. Harness traps: `openAPISpec()` `t.Skipf` on missing spec (api_test.go:83-85); `conformCheck` FindRoute failure only `t.Logf` (api_test.go:183-188). |
| Web/SDK | 14 vitest files, thresholds lines 57/branches 71 (achieved 61.7/75.4); **functions ungated at 48.3%**; `App/Layout/SettingsPage/OnboardingWizard/AnalyticsPage` at 0% lines; CSP spec skipped (Caddy-fronted); SDK has no coverage config. |
| Stubs still open | rebuffer_ratio/error_rate proxy from HealthScore (`alert/wave2.go:57-71`); B7 single shared webhook secret (`serve.go:214-220`); logtail collector implemented but wiring commented out (`serve.go:200-204`); probes non-HLS = `not_probed`; Postgres meta / SSO / PDF logo / mobile SDKs absent. RequestID is CLOSED (server.go:326). Zero `TODO()` markers remain. |
| Docs | 2 P0-stale: productionize.md teaches a 3-overlay prod start (reality: 5 overlays + `--env-file`) and a loopback-only HTTPS step then curls the public URL. Missing for GA: LICENSE, SECURITY.md, CHANGELOG, upgrade/rollback runbook, monitoring-Pulse runbook. Stale claims in alerting.md (unbounded history — wrong since D-052), ARCHITECTURE.md §6 (bcrypt "roadmap" — shipped), install.md (3-tier table, "planned for Wave 3"). |
| Git/GitHub | Clean, synced, CI green. `main` **unprotected** (API 404); no tags; stale `ams-integration` ref on local+origin. `gh` authed as owner `aytekXR` → protection + tag are now agent-runnable (U4 unblocked). |
| Live prod | `/healthz` all ok. **Runs `v0.1.0-25-gbc15d43` since 2026-07-09 (D-062)** — rule→channel alert delivery live-proven; beacon chain live (403 LICENSE_REQUIRED until U3). Rollback tags `pre-d061`/`pre-d058`. WATCH: intermittent CH "Memory limit exceeded 1.80 GiB" on server_events inserts seen pre-swap (did not recur post-swap). O4 `webhook: invalid signature` WARN still awaits AMS-side config (O3). |

## 2. GA definition — "production-ready" means ALL of:

- **G1 Release**: signed (cosign), Trivy-scanned (fail HIGH/CRITICAL), SBOM'd, multi-arch image at a
  canonical GHCR path, published by a CI-gated tag pipeline; `pulse version` reports the real
  version/commit/date; `v0.1.0` tag exists; `main` branch-protected.
- **G2 Prod currency**: production runs current `main` (≥ D-056); backup cycle N≥2 + keep-7 verified;
  §8 smoke green.
- **G3 Server tests**: total coverage ≥ 70% with the CI floor ratcheted to ≥ 68; no tested package
  below 60% except justified-in-writing; migrations runner tested incl. idempotent re-run (A11);
  0 FAIL / 0 unexpected SKIP under `-race`, repo-root mount.
- **G4 Contracts**: all 52 OpenAPI operations response-body-validated; conformance harness fails
  loud (no `t.Skipf`/`t.Logf` escape hatches).
- **G5 Web/E2E**: functions threshold gated; 0%-coverage pages have at least smoke tests; CSP
  asserted in a Caddy-fronted CI job; `web-e2e` required; delivery_failure e2e; 500-stream
  dashboard render budget measured (VD-04/A10 proxy).
- **G6 Features honest**: rebuffer_ratio/error_rate alerts read real `rollup_qoe_1h` data (CI-proven
  under the D-055 mock Pro license); B7 per-source webhook secrets; logtail wiring decided
  (wired+tested or removed).
- **G7 Docs**: no stale operator instruction (the §1 list fixed); LICENSE + SECURITY.md + CHANGELOG +
  upgrade/rollback + monitoring runbooks exist; Helm either installable or explicitly marked
  experimental in install.md.
- **G8 Operator** (§5): U3 license activated (QoE flows in prod), U5 browser/CSP check, AMS webhook
  configured in the AMS console.

Post-GA backlog (explicitly NOT gating): Postgres meta backend, SSO/OIDC, native
WebRTC/RTMP/DASH probes, mobile SDKs, white-label PDF logo, anomaly expansion, distributed probes.

## 3. Sessions

Sizing: one session ≈ one prior phase-sized effort (D-048..D-051 or D-055 scale) — a Workflow of
~10–20 agents + gates + handoff, survivable within a usage-limited session. Every session follows
§6 protocol and §7 standing rules. **TDD is binding**: red→green→refactor where a test can express
the requirement; for infra/CI work where it can't, every change needs a falsifiable scripted
verification (mutation-proof where practical, D-055 pattern).

### ✅ S1 — Release engineering + D-056 prod rollout — DONE 2026-07-08 (D-058)
**Result:** v0.1.0 released (run 28911789088: CI-gated, Trivy, multi-arch amd64+arm64, SBOM+provenance,
cosign tlog 2110636506); `pulse version` stamped everywhere (+ mutation-proof ci assert); Helm ref canonical;
caddy+golang digest-pinned; dependabot live (docker-compose ecosystem proven by its first caddy PR);
prod runs `1a701d6` (D-056 live; beacon public chain fixed end-to-end — 3 staging-found bugs: missing
PULSE_INGEST_LISTEN_ADDR, VD-15 fail-open on the dedicated listener, /beacon handle_path); backup cycle 2 +
keep-7 verified; main protected; ams-integration deleted. **G1 met except GHCR package visibility (→O7);
G2 met.** Evidence: D-058.

### S1 (original plan, kept for provenance) — prompt: `sessions/SESSION-01.md`
**Goal:** a stranger can pull a versioned, signed, scanned image; prod runs current main.
1. Version stamping end-to-end: `-ldflags -X main.Version/GitCommit/BuildDate` in
   `deploy/docker/pulse.Dockerfile` (ARG-fed), Makefile, ci.yml docker-build, release.yml;
   unit test on the version formatter + a CI assert that the built image does NOT report `dev`.
2. release.yml hardening: gate on the tagged commit's green ci run; multi-arch
   (amd64+arm64, qemu+buildx); Trivy image scan failing HIGH/CRITICAL pre-push; SBOM+provenance;
   cosign keyless signing (`id-token: write`); `workflow_dispatch` dry-run input (build+scan, no push).
3. Canonical image path decision → fix Helm `values.yaml:13` to match release.yml (full Helm
   parity waits for S6; only the P0 ref + an "experimental" warning now).
4. Digest-pin golang builder stage + caddy; add `.github/dependabot.yml` (gomod, npm×2, docker,
   github-actions).
5. **Prod rollout carrying D-056** — §8.7 staging-verify on an isolated compose project FIRST
   (D-054 lesson), then the standing 5-overlay combo; §8.8 smoke incl. beacon-401-fix spot-check;
   confirm backup cycle 2 + keep-7; verify stamped version in prod.
6. U4 (now agent-runnable, `gh` = owner): run `.github/branch-protection.sh`, verify via API;
   delete stale `ams-integration` (local+origin) after a 0-unique-commits diff; push `v0.1.0`
   AFTER items 1–4 land on main → watch the release run → `cosign verify` + pull the published image.
**Exit:** G1 + G2 fully met.

### ✅ S2 — Test backfill A: highest blast radius (Go core) — DONE 2026-07-08 (D-059)
**Result:** total 59.4→**69.7%** (exit bar was ≥64), FLOOR 58→62 (mutation-checked); query 18.5→88.5,
migrations 0→65.6 unit + **A11 retired** (double-migrate idempotency integration-proven), cmd/pulse
13.6→43.0 (beaconListenerConfig() extraction; VD-15 License-non-nil + listen-addr pins), api 55.9→74.3 +
**conformance harness honest** (t.Fatalf/t.Errorf, no drift found → no CR, 0 SKIP), domain 0→100,
discovery budget derived 3→5 cycles. 12-agent workflow, WO-4 fixed after 1 refute (3 t.Skip hatches).
ci.yml server+docker-build steps reproduced locally; CI run 28922883994 green. Commits `d3f697c`…`c80badf`.

### S2 (original plan, kept for provenance) — prompt: `sessions/SESSION-02.md`
**Goal:** kill the biggest coverage holes; make the conformance harness honest.
1. `internal/query` 18.5→≥70 via mock-Conn unit tests (AudienceAnalytics, Geo/DeviceBreakdown,
   QoeSummary, IngestTimeseries, QueryProbeResults, applyRetention, FleetNodes — all 0% today).
2. `store/clickhouse/migrations` 0→≥60: runner unit tests (splitStatements, stripLeadingComments,
   substitute pure fns; Run against the integration harness) **incl. re-run idempotency (A11)**.
3. `cmd/pulse` 13.5→≥40: serve/migrate/diag wiring smoke (in-process, `:memory:` meta, mock CH/AMS).
4. `internal/api` 55.9→≥65: the 15 uncovered handlers (update/delete alert rules+channels,
   sources, users, license activate, bootstrapIfFirstRun, checkPassword, wsPushLoop/wsBroadcast).
5. `internal/domain` 0→covered (trivial; Time method table test).
6. Harness honesty: `openAPISpec()` `t.Skipf`→`t.Fatalf`; `conformCheck` FindRoute `t.Logf`→`t.Errorf`.
7. De-flake `TestDiscovery_NewNodeVisible` latency budget (RESUME §6.7, observed 68.8ms vs 60ms).
**Exit:** total ≥ 64%, ci.yml FLOOR → 62.0; all new tests red→green documented; 0 SKIP.

### S3 — Test backfill B: contracts + web — ✅ DONE (D-060, 2026-07-08)
**Goal:** G4 + the web half of G5. **All exit criteria MET, verified adversarially.**
1. ✅ Response-body conformance: **51/52 operations validated + 1 waived** (GET /live/ws, WS 101).
   The D-057 "38 uncovered" list was stale — the real pre-S3 state was 25/52; S3 added 26. Error
   shapes (401 sweep ×49 + 403/404/422) validated for the first time. NO contract drift → no CR.
2. ✅ `collector/webhook` 58.1→**94.3**; `reports` 58.8→**90.9** (local fakeConn pattern).
3. ✅ Web: functions gate **45** (NEW) + lines 57→**76** / branches 71→**72**; all 0% pages
   smoke-tested (now 60-100% lines); `coverage-gate.test.ts` pins gates + exact exclude set.
4. ✅ SDK: baseline gated **62/73/70** (+@vitest/coverage-v8; size 3.52 KB green).
**Exit actuals:** total Go **73.2%** (≥68 target beaten; G3's ≥70 exceeded), FLOOR → **66.0**.
Bonus: pre-existing D-042-class flake `TestAlertHistory_PruneTimingAt2000` exposed by the
faithful CI repro and fixed (derived insert-baseline budget, load-immune).

### S4 — E2E phase 2 + CI hardening — ✅ DONE (D-061, 2026-07-09)
**Result:** items 1-5 done (csp-e2e job + CSP spec un-skipped → A7 CI half closed; A4
delivery_failure e2e — which EXPOSED+FIXED the P0 registry gap: rule→channel delivery never
worked in prod paths; e2e on main pushes + node 22 (promotion documented, clock ends
~2026-07-21); VD-04 measured+CLOSED 668/459 ms @ 500 streams; fixture-replay suite live).
Item 6 CodeQL = BLOCKED → operator item O9 (private repo, no GHAS). Floor 66→70.
⚠️ Prod rollout carrying the registry fix is DUE → S5 WO-1.
**Goal (original):** the rest of G5; CI catches everything it can.
1. Caddy-fronted Playwright job: full compose incl. caddy in CI → CSP spec un-skipped, header +
   zero-console-violation assert (closes A7).
2. delivery_failure e2e (webhook channel at a dead URL → history row; E2E-TEST-PLAN phase 2).
3. e2e.yml on push-to-main; promote `web-e2e` to required + node 20→22 (2-weeks-green clock
   started 2026-07-07 — promote if green streak holds, else document).
4. VD-04/A10: 500-stream render-time measurement via Playwright against mock-ams at scale
   (`/control/set_viewers`); record numbers in ARCHITECTURE §4.
5. AMS wire fixture-replay regression suite pinning D-029/D-031 (bps→kbps, FPS-redistribution,
   `terminated_unexpectedly`, WebRTC single-track) from `real-ams-captures/`.
6. CodeQL workflow (Go + JS/TS).
**Exit:** G5 met; e2e.yml green on main; branch-protection contexts updated.

### S5 — Honest features + security tail — ✅ DONE (D-062, 2026-07-09)
**G6 MET**: rebuffer_ratio/error_rate read rollup_qoe_1h (proxy removed, e2e-proven in CI);
B7 shipped (contract CR merged, types byte-stable); logtail DELETED with rationale. Plus:
prod rollout (delivery live-proven), CodeQL live (O9), Slack-webhook secret intercept (O11).
Item 5 (O4 invalid-signature WARN) still awaits AMS-side webhook config (O3).
**Goal:** G6; no silently-approximated metric.
1. rebuffer_ratio/error_rate alerts read `rollup_qoe_1h`/`viewer_sessions` instead of the
   HealthScore proxy (`alert/wave2.go:57-71`); e2e-provable TODAY under the D-055 mock Pro
   license — do not wait for U3.
2. B7 per-source webhook secret: contract CR via INT-01 (config already parses per-source
   `WebhookSecret`, `config.go:283` — plumb source-keyed secrets into the handler, TDD).
3. Logtail decision: wire the commented-out block (`serve.go:200-204`) + rotation e2e, or delete
   it with a D-0NN rationale.
4. `Caddyfile.prod` AMS upstream → env var (`{$AMS_UPSTREAM}`); `.env.example` completeness
   (the ~8 missing vars incl. PULSE_ALLOWED_WS_ORIGINS, PULSE_BASE_URL, PULSE_CORS_ALLOWED_ORIGINS).
5. Investigate the prod `webhook: invalid signature` WARN with the operator (O4) once AMS-side
   webhook is configured.
**Exit:** G6 met; contract CR merged + regenerated types byte-stable.

### S6 — Docs + Helm GA batch — prompt written by S5
**Goal:** G7; nothing in docs lies to an operator.
1. Fix the P0s: productionize.md 5-overlay reality (quick-ref + step 1e + upgrade section),
   secrets `_FILE` section; then the P1/P2 batch: alerting.md prune cap + retry/delivery_failure
   docs, ARCHITECTURE §6 bcrypt + "Last updated", install.md 4-tier table + Wave-3 note +
   Helm warning, beacon-sdk.md numbers, runbooks README dead refs.
2. New docs: upgrade/rollback runbook (incl. CH DDL rollback + 5-overlay commands),
   monitoring-Pulse runbook (backup daemon, alert_history growth, disk, collector_errors_total
   thresholds), SECURITY.md, CHANGELOG.md (backfill from decisions.md D-0NN), LICENSE (**operator
   picks the license — O5**).
3. Helm parity: image ref + CH auth + webhook port/Service + backup CronJob + `optional: false`
   secret + digest defaults + NOTES.txt; `helm template` golden-file tests in CI (cluster install
   stays waived per D-002 unless a cluster appears).
**Exit:** G7 met (Helm "installable-or-marked-experimental" decision recorded).

### S7 — GA gate + post-GA backlog seeding — prompt written by S6
**Goal:** adversarial GA audit; declare GA or produce the punch list.
1. Re-run the 9-scout audit (same dimensions as D-057); diff against §2 G1–G8; every unmet
   criterion becomes a work order.
2. Load smoke (A10): sustained multi-stream/multi-viewer soak vs mock-ams + the real AMS
   teststream; watch memory/CPU vs limits, CH ingest lag, WS fan-out.
3. Tag `v1.0.0` (or `v0.2.0` — operator call) via the S1 pipeline; CHANGELOG entry; prod rollout.
4. Seed the post-GA backlog (§2 list) as a fresh ROADMAP v2 if the operator wants to continue.
**Exit:** GA declared with evidence, or a scoped remainder roadmap.

## 4. Coverage ratchet ledger (update every session)

| When | Go total | ci FLOOR | Web lines/branches/functions | Notes |
|---|---|---|---|---|
| 2026-07-08 (audit) | 59.5% | 58.0 | 61.7 / 75.4 / 48.3 (fn ungated) | baseline |
| 2026-07-08 (after S1) | 59.4% | 58.0 | unchanged | infra session; −0.1 = 4 uncovered serve.go wiring lines (S2 covers) |
| 2026-07-08 (after S2) | **69.7%** | **62.0** | unchanged | D-059; S2 target ≥64 beaten; G3's ≥70 nearly met already |
| 2026-07-08 (after S3) | **73.2%** | **66.0** | gates **76/72/45** + guard test | D-060; G4 met 51/52+1 waived; sdk gated 62/73/70; G3's ≥70 EXCEEDED |
| 2026-07-09 (after S4) | **73.3%** | **70.0** | hold (76/72/45) | D-061; ratchet done; alert 73.3 (new sync source), api 75.9, collector 66.5 |
| 2026-07-09 (after S5) | **73.2%** | **70.0** | hold (76/72/45) | D-062; −0.1 = logtail (92.1%-covered pkg) deleted; webhook 94.7, query 86.9, alert 73.8; no ratchet (<74) |
| GA (G3) | ≥70% ✅ (73.2) | ≥68.0 | ratchet to achieved−3 | coverage half of G3 already met; floor ratchet remains |

## 5. Operator ledger (surface EVERY session — agent cannot do these)

| # | Action | Status |
|---|---|---|
| O1 (=U3) | Activate a Pro+ Pulse license in prod (`PULSE_LICENSE_KEY`) — until then QoE/beacon data does not flow in prod (CI covers it with the mock license) | OPEN |
| O2 (=U5) | Open `https://beyondkaira.com` + `https://pulse.beyondkaira.com`, confirm SPA renders, zero CSP console errors (S4 automates CSP in CI, but one human check of prod is still wanted) | OPEN |
| O3 | Configure the AMS console to POST lifecycle webhooks to `https://beyondkaira.com/webhook/ams` with the HMAC secret from `deploy/.env` (Pulse side live since D-054) | OPEN |
| O4 | After O3: confirm the `webhook: invalid signature` WARN does not recur (else the AMS-side secret is wrong) | OPEN |
| O5 | Choose the project LICENSE (legal decision; agent drafts once chosen) | OPEN |
| O6 | (was U4) Branch protection + `v*` tag | ✅ DONE by S1 (D-058): protection live (API 200), v0.1.0 released |
| O7 | **Make `ghcr.io/aytekxr/ams-pulse` public** (package settings → Change visibility) or `gh auth refresh -s read:packages` — until then nobody (incl. the agent) can pull v0.1.0 or run `cosign verify` (commands in release.yml header); this is the last G1 bit | OPEN (re-verified still blocked 2026-07-08(d), D-060) |
| O8 | Review the dependabot PRs — **21 open** (majors: vite 8, vitest 4, plugin-react 6, eslint 10, size-limit 12; grouped minor-and-patch for web, sdk AND server gomod; caddy digest bump was CI+e2e green — mergeable). Protection requires 1 approval — dependabot PRs need owner review; S5 can absorb the web-tooling majors if the operator asks | OPEN (unchanged, re-checked D-061) |
| O9 | ~~CodeQL blocked~~ ✅ CLOSED by S5 (D-062): operator made the repo PUBLIC → `codeql.yml` live (go/autobuild + js-ts/none). NOT a required context yet — promote after a bake period (S6/S7 call). NOTE: repo-level secret-scanning/push-protection still disabled — consider enabling now that the repo is public | ✅ DONE (D-062) |
| O10 | ~~Prod rollout DUE~~ ✅ CLOSED by S5 WO-1 (D-062): prod runs `v0.1.0-25-gbc15d43`; rule→channel delivery live-proven (email-channel smoke, firing row ≤2s + mail received) | ✅ DONE (D-062) |
| O11 | **Rotate the Slack CI-notification webhook** (it sat hardcoded in an unpushed commit + session transcripts — never public, but hygiene demands rotation; then `gh secret set SLACK_WEBHOOK_URL` with the new value) AND reset the concurrent session's local main onto origin (`git fetch && git reset --hard origin/main` — its `ee4fc00` content is contained in `bc15d43`) | OPEN (NEW, D-062) |

## 6. Session protocol (BINDING — the "prompts" contract)

Every session, in order:
1. **Open** `sessions/SESSION-NN.md` (it was written by the previous session). Re-verify its
   preconditions cheaply (git log, gh run list, the specific file:line claims it relies on) —
   if stale, fix the prompt first, note the drift in decisions.md.
2. **Execute** as Workflow(s): disjoint-scope authors, TDD red→green (watch it fail), adversarial
   verify rounds, ORCH gates centrally (reproduce EVERY ci.yml step — D-053/D-055 lesson).
3. **Verify** per RESUME-PROMPT §8 (build, lint, typecheck, -race repo-root, coverage, contract
   drift, staging, smoke, adversarial re-check).
4. **Commit** by explicit path per scope; push; `gh run watch` until green.
5. **Handoff**: decisions.md (new D-0NN) + RESUME-PROMPT ▶ START HERE + this file's §3 ✅ + §4/§5
   ledgers.
6. **Write `sessions/SESSION-(NN+1).md`** from this roadmap's next session + actuals (template
   `sessions/TEMPLATE.md`) — include: mission, preconditions, work orders w/ file:line evidence,
   TDD plans, gates, and this protocol's closing steps. **The session is not done until the next
   prompt exists and is committed.** If the session was cut short (usage limit), SESSION-(NN+1).md
   is instead a resume prompt for the remainder (D-052 precedent).

## 7. Standing rules (inherited, unchanged)

Go ONLY in Docker `golang:1.25` with the REPO-ROOT mount (D-028); no false-green — a "flake" that
never resolves is a bug (D-042); commit by explicit path, never `git add -A`; contracts frozen —
changes only via INT-01 CR (D-004); never commit secrets (`deploy/.env`, `oguz-testing.md`,
`web/pulse_secret.key`); anti-stall D-016 (no foreground servers, timeouts everywhere); prod ops
use the 5-overlay combo incl. backup (D-054); staging-verify before prod for any boot-behavior
change (D-054); single-writer scope map `agents/manifest.yaml`; root-owned ClickHouse debris dirs
(`server/internal/*/{preprocessed_configs,access}`) are gitignored — ignore or ask the operator to
remove with sudo.
