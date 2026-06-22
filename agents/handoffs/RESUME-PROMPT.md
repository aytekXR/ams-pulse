# Resume prompt — Pulse (next session)

> Updated 2026-06-22 (**session 8 → GO-LIVE DONE + D-032 completeness/test audit; next session = production-readiness**).
> **Pulse is LIVE on REAL AMS in production: https://beyondkaira.com (+ `www` → apex), real Let's Encrypt TLS,
> CORS/CSP/auth hardened, backed by `test.antmedia.io` (AMS 3.0.3 Enterprise) — the mock-ams demo is RETIRED.**
>
> ## ▶ NEXT SESSION = PRODUCTION-READINESS (read `agents/handoffs/PRODUCTION-READINESS.md` FIRST)
> The `pulse-completeness-and-test-audit` workflow (D-032) produced a full, evidence-based brief —
> **`agents/handoffs/PRODUCTION-READINESS.md`** is the paste-ready next-session prompt. Headline: the MVP is
> functionally substantial but **NOT "tested at every level"** (3 packages at **0.0%** in a normal run —
> `internal/query`, `internal/store/clickhouse`, `internal/config`; **0 Playwright**; no response-body↔OpenAPI
> validation; no enforced coverage gate; e2e = 4 assertions), and several features **stub silently** in prod
> (alert channel test-fire no-ops; 3 license gates unenforced; standalone node card blank; QoE/beacon needs a
> Pro+ license to flow). The brief sequences it: **Step A** golang:1.26→1.25 ✅ done; **Step B** merge
> `ams-integration`→`main`; **Step C** wire the Caddy `/webhook/*` route; then **Phase 1** (`pulse-p1-gaps`),
> **Phase 2** (`pulse-test-backfill` — TDD + coverage gate, every level), **Phase 3** (`pulse-prod-harden`),
> **Phase 4** (`pulse-feature-complete`). **Orchestrate each phase with a Workflow.** Honor Verify + Commit +
> Handoff. Operator follow-up still open: AMS-side **IP allow-list** for the VPS (8/16 apps 403 → user's 4
> active streams are in blocked apps).
>
> **D-032 quick win shipped this session:** `golang:1.26`→`golang:1.25` in
> `docker-compose.{hardened,ci,override}.yml` (1.26 is unreleased → would break mock-ams + CI on `docker pull`).
>
> ## ✅ REAL-AMS GO-LIVE — DONE (D-031, session 8)
> Operator gave GO (execute / wipe ClickHouse / bitrate target 600). The prod swap is COMPLETE + live-verified:
> `/api/v1/live/overview` → `total_publishers:1` (LiveApp `test123`), **`bitrate_kbps≈623.5`** (was 624016),
> **`health_score:100` (Good)**, ClickHouse = real data only (seeded rows wiped, 0 remain), migrate DSN masked
> (`clickhouse://pulse:xxxxx@…`). Procedure + rollback: **`deploy/runbooks/real-ams-go-live.md`** (the
> seeded-demo sidecar was stopped; restore steps for rollback are in `oguz-testing.md`, gitignored).
>
> **NEXT SESSION (agent) — the headline is now the D-031 POST-DEMO BACKLOG** (make the real dashboard richer
> for the founder), NOT a new deploy. In priority order: (1) **standalone node card** — on `cluster/nodes`
> 404, synthesize a single-node card from `/rest/v2/system-status` so Fleet + CPU/RAM aren't blank; (2)
> **`EventWebRTCClientStats` aggregator case** — wire viewer-side RTT/jitter/loss into the live snapshot
> (decide viewer-side vs ingest-side field ownership); (3) surface AMS **version** (3.0.3); (4) **merge
> `ams-integration` → `main`** + drop the vestigial `AMS_LOGIN_*` lines from `deploy/.env`; (5) QoE/beacon
> end-to-end (needs Pro+ license). Honor Verify + Commit + Handoff. Watch for AMS wire drift in pulse logs.
>
> **Go-live also surfaced + fixed (D-031 addendum):** the aggregator hardcoded the bitrate health target
> (2000), ignoring `PULSE_INGEST_TARGET_BITRATE_KBPS` — so the dashboard health badge ignored config. Fixed
> via `Aggregator.SetIngestTargets()` (wired from `serve.go`; passthrough added to `docker-compose.real-ams.yml`)
> + `TestAggregator_SetIngestTargets`. That's why test123 reads "Good" (623.5 kbps ≥ 600 target).
>
> **D-031 deploy-prep fixes (on `ams-integration`):** (1) **`maskDSN` was a no-op** → ClickHouse password
> leaked in plaintext to migrate/`pulse diag` logs; fixed with `url.URL.Redacted()` + `TestMaskDSN`. (2)
> **broken SDK-docs CTA** (`your-org` 404) → real repo URL (`web/src/features/qoe/QoePage.tsx`).
>
> **D-030 committed `fe321bf` (same session — all fixes from D-029v validation):** see bullet list below;
> pushed to `ams-integration`; full `go test ./... -race` GREEN; live-confirmed `bitrate_kbps=624.152,
> total_publishers=1`.
>
> **Session 8 shipped (D-029v → D-030 — real-AMS wire-correctness, on `ams-integration`, now pushed):** re-validated the
> D-029 integration LIVE against `test.antmedia.io` (isolated `pulse-realams` stack, loopback :18090,
> `pulse-prod` untouched) and ran an adversarial validation workflow (5 finders → refute pass) that
> diffed REAL captures against the decode/normalize code + fixtures. It found **15 confirmed wire
> bugs**; the high-value, code-confirmed ones are FIXED (scope: `server/pkg/amsclient` +
> `server/internal/collector`, no contract change):
> - **CRITICAL bitrate 1000×** — AMS `bitrate` is **bits/sec** (curl-verified: 624016 ≈ receivedBytes·8/duration
>   ≈ 624 kbps); `normalize.go` stored it raw into `bitrate_kbps`. Now `/1000`. **Verified live: API
>   `bitrate_kbps` = 624.152** (was 624016). Also REMOVED the bogus `speed`→bitrate fallback (`speed` is a
>   realtime RATIO ≈1.0, not a bitrate).
> - **HIGH fps→permanent Warning** — AMS 3.0.3 REST omits `currentFPS`, so every REST stream scored fps=0 →
>   health capped at 0.75 (Warning), "Good" unreachable. Fix: `fps` is emitted only when present; a `-1`
>   "unavailable" sentinel makes `ComputeHealthScore` redistribute the 0.25 FPS weight across the other 4.
>   (test123 now correctly shows **Warning for the HONEST reason** — 624 kbps < 2000 kbps target — not a
>   phantom fps.)
> - **MEDIUM dropped ingest QoE** — `packetLostRatio`/`jitterMs`/`rttMs` exist on the real broadcast object
>   but the DTO dropped them and `normalize` hardcoded loss/jitter=0 (health blind to real degradation).
>   Now decoded + wired (`packetLostRatio` treated as 0..1 fraction ×100 → pct; jitter/rtt already ms).
> - **MEDIUM `terminated_unexpectedly`** — real AMS crash status (seen in `meet`), was unhandled → crashed
>   streams stayed "live" ~3 min. Now emits `publish_end` next poll. **LOW** restpoller now logs non-404
>   cluster-poll errors; **MEDIUM** WebRTC per-peer rtt/jitter/loss no longer halved for single-track viewers.
> - **Tests:** real-wire regression tests added (sanitized `testdata/broadcasts_real_test123_v303.json` +
>   normalize/health/client cases pinning bps→kbps, fps-redistribution, QoE units, terminated_unexpectedly,
>   single-track WebRTC). Full `go test ./... -race` GREEN (repo-ROOT mount, GOFLAGS=-buildvcs=false).
> - **Still DEFERRED** (documented below, no current live impact): `webrtc_client_stats` event is normalized
>   but the aggregator has no `case` for it (viewer-side QoE never applied — needs a QoE-model decision);
>   standalone AMS has no cpu/mem (system-status lacks them → fleet shows none, needs an "N/A" UX);
>   AMS `version` (3.0.3) never surfaced; `speed_read_kbps` data-key name is a legacy misnomer (now carries
>   the ratio). B7 (per-source webhook secret, contract CR) + B3 (Docker secrets) unchanged.
>
> **Session 7 shipped (D-029 — real-AMS integration vs `test.antmedia.io`, committed locally on
> `ams-integration`, push pending):** connected Pulse to the **real AMS 3.0.3 Enterprise** server and
> proved the dashboard renders its live stream. The W2c `amsclient` (D-025), built from assumed wire
> shapes, was **wrong for real AMS** — fixed: (a) **cookie-session login+refresh** auth (AMS has no JWT:
> `jwtServerControlEnabled=false`; `Config.LoginEmail/LoginPassword` → `POST /rest/v2/users/authenticate`
> → JSESSIONID via a custom IP-safe cookie jar; re-login+single-retry on 401/403, throttled vs IP-block
> storms); (b) **per-app REST paths** (`/{app}/rest/v2/broadcasts/list/{offset}/{size}` etc. — root paths
> 404'd); (c) array-of-strings `applications` decode; (d) 404-tolerant cluster/system endpoints.
> **Live validation surfaced a 2nd, pre-existing multi-app bug** (D-029 addendum): the restpoller
> `detectEnded` + aggregator keyed streams by `node/streamID` (no app), so polling multiple apps falsely
> ended one app's live stream — now keyed `node/app/streamID`. **Result: `/api/v1/live/overview` →
> `total_publishers:1`** (LiveApp `test123`) on an isolated `pulse-realams` stack; `pulse-prod` untouched.
> Full `go test ./... -race` green (repo-ROOT mount). New env: `PULSE_AMS_LOGIN_EMAIL/PASSWORD`. Operator
> still must decide when to **swap `pulse-prod` to real-AMS** (see "▶ REAL-AMS — DONE" below).
>
> **Session 6 shipped (D-028 — pushed to `main`: `54e2d8f` + `f43a22e` + `63f702d`):** the three unblocked
> deferred hardening items — **B6** (source-test now decrypts the stored credential), **A2** (per-token
> rate-limit on the main-port `/ingest/beacon`), **A7** (per-IP rate-limit on `/metrics`). Server-only,
> no contract change. ⚠️ Process note worth reading: the authoring workflow returned a **false green** —
> A7 shipped *unwired* and the whole api test suite was silently *skipping* because the Docker gate
> mounted only `server/` (so `metaDDLPath`'s `../../../contracts` escaped the mount → `t.Skip`, and Go
> counts skip as pass). ORCH's faithful repro (**mount the repo ROOT**, workdir `/repo/server`,
> `GOFLAGS=-buildvcs=false`) caught both, wired A7, and re-verified: **api = 92 pass / 0 skip / 0 fail**,
> full `go test ./... -race` green. See D-028.
>
> Session 5 (prior) shipped + pushed to `main`: W2b TLS go-live (D-024), W2c `amsclient` real-wire
> hardening (D-025), **ALL 6 CI failures fixed** (D-026), and the security+AMS hardening suite +
> **live redeploy** (D-027). Commit SHAs + details below.
>
> **YOUR part (operator) — 3 things only you can do:**
> 1. **Confirm the live dashboard renders** in a browser (the CSP is browser-enforced; I verified the
>    SPA has no inline scripts but couldn't run a real browser — if the console flags a CSP violation,
>    tell me and I'll loosen it instantly).
> 2. **Confirm the latest GitHub Actions run is green** — the repo is private and `gh` isn't on the
>    VPS, so I cannot see it; paste any red job logs and I'll fix them (that's how the last 3 were found).
> 3. **GitHub-side admin TODOs** (need a repo admin): `branch-protection.sh`, push a `v*` tag — see
>    "USER GitHub-side TODO".
>
> ✅ **Real-AMS creds**: obtained + wired. D-029 + D-030 done + pushed on `ams-integration`.
>
> **NEXT session (agent) — real-AMS integration is DONE (D-029 + D-030). `ams-integration` is pushed.**
> Headline options: **(a)** merge `ams-integration` → `main` (run the full test suite first, confirm CI
> green); **(b)** prod swap: wire `pulse-prod` to real AMS (operator must approve — see "▶ REAL-AMS" below);
> **(c)** QoE/beacon end-to-end (Pro+ license lifts the ingest gate); **(d)** deferred items:
> `webrtc_client_stats` aggregator case, `speed_read_kbps` rename, fleet N/A UX, B7 (contract CR), B3
> (Docker secrets). B6/A2/A7 are DONE (D-028). Honor the **Verify + Commit + Handoff** flows below.
> Paste this file into a fresh Claude Code session at the repo root (`/home/aytek/repo/ams-pulse`, VPS).

## ✅ REAL-AMS INTEGRATION — DONE (D-029, session 7) — what's left is the prod SWAP

**Status:** DONE + validated live against `https://test.antmedia.io/` (AMS 3.0.3 Enterprise). The
dashboard renders the real live stream (`/api/v1/live/overview` → `total_publishers:1`, LiveApp
`test123`) on an **isolated** `pulse-realams` stack (loopback :18090). D-029 (`4ce3a76`) + D-030
(`fe321bf`) committed and **pushed to `ams-integration`** (2026-06-21). The section below is retained as
the integration reference + the remaining operator step (prod swap).

**Remaining (operator decision):** swap the LIVE `pulse-prod` demo from mock-ams to real-AMS. Mechanics:
add `PULSE_AMS_LOGIN_EMAIL/PASSWORD` + `PULSE_AMS_APPLICATIONS` (already in `deploy/.env`) and layer
`-f deploy/docker-compose.real-ams.yml` onto the `pulse-prod` stack, then
`docker compose $DC up -d --build pulse`. ⚠️ Only after the operator says go (it changes what the Ant
Media founder sees). The dashboard will then show real `test.antmedia.io` streams instead of seeded demo
data. To VALIDATE without touching prod, bring up the isolated stack:
`DC="-p pulse-realams -f deploy/docker-compose.yml -f deploy/docker-compose.real-ams.yml -f deploy/docker-compose.realams-test.yml --env-file deploy/.env"; sg docker -c "docker compose $DC up -d --build"`
then `curl -s http://127.0.0.1:18090/api/v1/live/overview -H "Authorization: Bearer $(sg docker -c "docker compose $DC logs pulse"|grep -oE 'plt_[A-Za-z0-9]+'|head -1)"`.

**Original goal (achieved):** point Pulse at the real Ant Media Server and prove the dashboard renders
its real streams/QoE; validate the W2c `amsclient` fixtures (D-025) against real captures; swap into the
live demo only once proven AND the operator says go.

**Operator inputs (provided):**
- AMS console URL: `https://test.antmedia.io/` · Username: `test@antmedia.io`
- **Creds (provided):** `deploy/.env` (gitignored) now has the full real-AMS block set + UNCOMMENTED:
  `PULSE_AMS_URL=https://test.antmedia.io`, `PULSE_AMS_LOGIN_EMAIL/PASSWORD`, `PULSE_AMS_NODE_ID=test-antmedia`,
  `PULSE_AMS_APPLICATIONS=LiveApp,PetarTest2,demo,clipcreator,meet`. **These vars are interpolated ONLY by
  `docker-compose.real-ams.yml`** — the base + hardened overrides hardcode `PULSE_AMS_URL` to mock-ams, so
  the live `pulse-prod` demo KEEPS polling mock-ams regardless of `.env` until you deliberately layer the
  real-ams overlay onto `pulse-prod` (the prod swap).

**Recon already done (2026-06-21, ORCH):** root `/` is the AMS web console (HTTP/2 200). REST is
**gated** — `GET /rest/v2/version` and `/rest/v2/applications` return **HTTP 403 Forbidden** (Tomcat)
unauthenticated; default ports `:5080`/`:5443` also 403 via the domain. So REST needs auth and/or an
allow-listed IP.

**⚠️ THE CRUX — AMS auth is email/password, but `amsclient` only sends a STATIC `Authorization: Bearer`.**
Resolve this BEFORE coding anything:
1. **Preferred:** log into the console, in **Settings → Security/JWT** generate a **long-lived JWT / app
   token**. If available → put it in `PULSE_AMS_AUTH_TOKEN`; `amsclient` works unchanged.
2. **Else:** only `POST /rest/v2/users/authenticate {email,password}` (→ JWT/cookie that **expires**) is
   available → a static `.env` token will break on expiry, so `amsclient` needs a small **login+refresh
   extension** (scope `server/pkg/amsclient` + a token provider; **no contract change**). Decide
   token-vs-login first.
3. **IP allow-list:** a 403 can also mean the VPS IP `161.97.172.146` isn't allowed — check the console's
   REST/dashboard CIDR allow-list and add it if needed.

**⛔ DO NOT disturb the live Oğuz demo.** `pulse-prod` is serving the seeded demo to the Ant Media founder
(`oguz-testing.md`, gitignored; token `plt_c692…`; liveness sidecar container `pulse-demo-liveness`;
teardown steps in that doc). Bring real-AMS up on a **separate compose project** (e.g. `-p pulse-realams`
on alt host ports, or a local stack) and validate THERE. Swap `pulse-prod` to real-AMS only after it's
proven and the operator approves.

**Plan (run as an ORCH workflow; Verify + Commit + Handoff):**
1. Determine auth (above). Confirm from the VPS:
   `curl -H "Authorization: Bearer <token>" https://test.antmedia.io/rest/v2/version` → 200.
2. If login/refresh is required, extend `amsclient` (author + unit tests; no contract change).
3. Set `PULSE_AMS_URL` + token in `deploy/.env`; bring up a SEPARATE stack with
   `deploy/docker-compose.real-ams.yml` (AMS-INTEGRATION.md §3.2).
4. **Validate W2c fixtures vs real captures (D-025):** curl each endpoint `amsclient` polls
   (AMS-INTEGRATION.md §1.1 table) from inside the container; diff real JSON against
   `server/pkg/amsclient/testdata/*.json`; fix any field/envelope drift; `go test ./... -race` green
   (**repo-ROOT mount** + `GOFLAGS=-buildvcs=false` — D-028 lesson, else api tests silently skip).
5. Register the source (`POST /api/v1/admin/sources`), run `.../test` (B6 decrypt works now), confirm
   `/api/v1/live/overview` shows real `total_viewers`/`total_publishers` when streams are live.
6. Verify + commit by explicit path (`feat(real-ams) D-029: …`) + update THIS file + push when directed.

Full operator guide + a ready-to-paste task prompt: **`agents/handoffs/AMS-INTEGRATION.md`** (§3 + §8).

## ✅ Status

**This session (session 5) — W2b production TLS go-live DONE (D-024).** `https://beyondkaira.com`
(+ `www`) is live on real Let's Encrypt certs; stack = project **`pulse-prod`**
(`base+hardened+prod-tls`, mock-ams, fresh authed volumes). Verified by Workflow
`pulse-golive-verify` (`wf_9d503e84-e0e`, 8 adversarial verifiers, 7/8 PASS + 1 accepted). Demo
torn down; `brier-db` (unrelated project) untouched. Operating commands are in the W2b section below.

**Also this session — ALL CI failures fixed (D-026), security+AMS hardening shipped + LIVE (D-027),
and W2c amsclient hardening DONE (D-025).** CI: **6 real failures total**, all fixed + pushed —
3 from my repro (helm goldens `6c7666c`; server build-from-root + dead ClickHouse URL `3a0a489`) and
**3 the user's actual GitHub logs revealed that my repro had masked** (`22dfd4d` compose `:?`-secret +
web `git diff` path; `b1304da` de-flaked the ~20% flaky `TestQuery_QoeSummary` rollup race). Every
job now passes a faithful local repro (query `-count=20` → 0 fail); **GitHub confirmation of a fresh
green run is still pending — paste it if anything is red.** Hardening (D-027, workflow
`pulse-security-ams-hardening`): CORS allowlist, token-in-URL, SSRF, rate-limiter eviction, beacon
caps, amsclient body limit, webhook wiring (fail-closed), CSP — `efe8578`/`89ace7e`; **redeployed
LIVE** (CORS+CSP confirmed on https://beyondkaira.com). **Session 6 (D-028) then shipped B6/A2/A7**
(`54e2d8f`, push pending) — the deferred backlog is now just **B7** (frozen-contract CR) + **B3**
(Docker secrets). See **`agents/handoffs/AMS-INTEGRATION.md`** for the real-AMS operator guide. W2c
(Workflow `pulse-amsclient-hardening`) mapped `amsclient` +
`collector`, fixed **3 latent bugs** (node-version drop/VD-40, v2.10 speed-only bitrate, empty
`StreamID` corruption) + a Kafka dash-viewer parity gap, and added `amsclient`'s **first** tests
(11 + 10 fixtures) — full `go test ./... -race` green.

**Side note (different repo):** also fixed + pushed the **`brier-claude`** CI (the `brier-db` on :5432
is that project). Its `make check` failed because 7 DB tests asserted a seeded video/transcript but
`seed.py` seeds analysts only; the `test_dedup`/`test_contradictions` helpers now create their own
video+transcript like the other DB tests (verified on a fresh DB → 40 passed; commit `2633dc1` in
`brier-claude`). Not part of Pulse — noted so the next session isn't surprised by cross-repo work.

**Prior (sessions 1–3):** MVP (F1–F10) + **Wave 3-Plus** + a **functional MVP DEPLOYED on the
VPS via Docker Compose** against the mock AMS (closed the D-002 waiver). Gate **CLOSED**
(D-019). Authoritative artifacts: `IMPLEMENTATION_LOG.md`, `DEVLOG.md`,
`agents/handoffs/decisions.md` (**D-001…D-027** binding), `qa/wave-3-plus/gate-report.md`.

**This session (session 4) — W1 `pulse-cicd` CLOSED (D-020).** The skeleton CI was broken vs.
the shipped MVP (Go 1.24 not 1.25; `npm ci` without `--legacy-peer-deps`; a malformed
`CGO_ENABLED=0 cd server`; soft-fail lint; no docker-build/e2e/release). Workflow `pulse-cicd`
(`wf_ca6228d5-6cf`, 18 agents: parallel author → adversarial verify + 2-round fix-loop →
independent gate) fixed + hardened + extended it. Shipped (commits `cc3e008` impl, `6254d90`
docs — both on `main`):
- `.github/workflows/ci.yml` — 7 jobs: **contracts**, **server** (go 1.25; `CGO_ENABLED=0`
  vet/build; `go test ./... -race`; **ClickHouse 24.8 service-container** `pulse migrate` smoke
  + integration), **web** (`npm ci --legacy-peer-deps`, `gen:api` drift guard, **HARD** lint/test),
  **sdk** (15 KB size gate, HARD lint/test), **docker-build** (GHCR lowercase), **helm**, **compose**.
- `.github/workflows/e2e.yml` + `deploy/docker-compose.ci.yml` (CI-safe override, loopback
  ports, ephemeral secret) — compose-up smoke: extract admin token, wait mock-ams, seed via
  `/control/publish`, assert healthz 200 + migrate exit 0 + CH tables + **authed** `/live/overview`
  viewers>0; `down -v` always. PR + dispatch only.
- `.github/workflows/release.yml` — build+push `ghcr.io/aytekxr/ams-pulse` on tag `v*`.
- `.github/workflows/ams-version-matrix.yml` — Go 1.24→1.25.
- `.github/branch-protection.sh` — `gh` script (required checks = the 7 ci jobs).
- A behaviour-preserving compose refactor (base `ports:`→`expose:`, drop `!override`).
- **Verification:** all 7 ci jobs reproduced locally inside `golang:1.25` / `node:22-alpine`;
  ORCH re-ran the full e2e chain directly → authed `/live/overview` = **viewers=13, publishers=1**.

### ⚠️ USER GitHub-side TODO (carry forward — these CANNOT be done from the VPS as-is)
1. The workflows are pushed. Open a PR (or let push trigger `ci`) so **GitHub Actions actually
   runs the jobs green** the first time.
2. Run `bash .github/branch-protection.sh` to make `main` gated — **needs `gh` installed +
   authed as a repo admin**. `gh` is NOT installed on the VPS (install needs the sudo password:
   `! sudo apt-get install -y gh`, or run the script from another admin machine).
3. Push a `v*` tag (e.g. `git tag v0.1.0 && git push origin v0.1.0`) to exercise the GHCR
   release once. `e2e` is intentionally **not** a required status check (slow full-stack bring-up).

**CI failing? (user reported red jobs, session 5).** The repo is **private** and `gh` isn't on the
VPS, so Actions can't be inspected from here. Some first-run red is *expected* (branch protection not
set; the slow `e2e`; GHCR/auth steps that local verification can't exercise). To diagnose: **(A)**
reproduce each `ci.yml` job locally in its image on the VPS (catches logic failures, not GitHub-only
auth ones); **(B)** paste the failing job names/logs (`gh run list` from an admin machine); or **(C)**
install `gh` here (`! sudo apt-get install -y gh && gh auth login`) and query runs directly.

### ✅ D-021 — live-dashboard deadlock fixed, demo restored (commits `70b4b14`, `69a8a1e`)
The demo's "unhealthy" pulse was a genuine **AB→BA deadlock** (root-caused from a SIGQUIT
goroutine dump — 486 HTTP handlers wedged on the aggregator RWMutex): `cluster.Discovery.poll`
and `aggregator.EvictStale` emitted to the fan-out sink **while holding their own lock**, and
each re-entered the other. Fix (rule: never hold a state lock across a sink call): collect events
under the lock, emit after releasing it; + regression tests that **deadlock on the un-fixed
source** and pass on the fix. Image rebuilt + redeployed → `/healthz` 200 on :8090 AND :80.
Demo live again at `http://161.97.172.146/`.

### ✅ D-022 — W2 productionize SUBSET (deploy hardening) done (no-infra half)
New files (committed): `deploy/docker-compose.hardened.yml` (**Caddy TLS** + **ClickHouse auth** +
pulse off all host ports + secrets-from-env, self-contained `base+hardened`), `deploy/config/Caddyfile`,
`deploy/.env.example` (placeholder template), `deploy/docker-compose.real-ams.yml`,
`docs/runbooks/productionize.md`. Adversarially verified live: HTTPS 200 (TLSv1.3, Caddy local CA),
CH auth enforced (wrong-password → Code 516, `default` user removed), migrate exit 0 on the authed
DSN, pulse with zero host ports. Local verify: `sg docker -c "CLICKHOUSE_USER=… CLICKHOUSE_PASSWORD=…
PULSE_SECRET_KEY=… docker compose -p pulse-hardened -f deploy/docker-compose.yml -f deploy/docker-compose.hardened.yml up -d --build"`
then `curl -k --resolve localhost:8443:127.0.0.1 https://localhost:8443/healthz`.

## Next session — run these Workflows (orchestrate with the Workflow tool)

W1 (D-020) + the W2 SUBSET (D-022) are DONE. The remaining default work needs **your infra
inputs**. **Every** workflow MUST end with the Verify + Commit + Handoff flows in the next section.

### ✅ W2b — production TLS go-live — DONE (D-024, session 5)
**Pulse is LIVE: https://beyondkaira.com (+ `www` → apex), real Let's Encrypt certs.** Stack is
project **`pulse-prod`** = `base + hardened + prod-tls` (mock-ams). `deploy/.env` (gitignored) holds
`PULSE_DOMAIN=beyondkaira.com` + `CLICKHOUSE_USER/PASSWORD` + `PULSE_SECRET_KEY`. Operating commands
(define `DC="-p pulse-prod -f deploy/docker-compose.yml -f deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml --env-file deploy/.env"`, run from repo root):
- **Status / logs:** `sg docker -c "docker compose $DC ps"` (swap `ps`→`logs caddy`/`logs pulse`).
- **Verify TLS** (⚠️ the VPS *local* DNS resolver is stale — always `--resolve` or `openssl -connect`):
  `curl -sS --resolve beyondkaira.com:443:161.97.172.146 https://beyondkaira.com/healthz`.
- **Certs auto-renew** (Caddy; the `caddy_data` volume persists certs + the ACME account).
- **Admin token** (fresh prod instance): `sg docker -c "docker compose $DC logs pulse" | grep plt_` (operator-held; not in git).
- **After editing `Caddyfile.prod`:** `sg docker -c "docker compose $DC restart caddy"` — a graceful
  `reload` does NOT provision a *newly added* hostname (that's why `www` needed a restart).
- **Teardown / rollback to demo:** `sg docker -c "docker compose $DC down"` then
  `cd deploy && sg docker -c "docker compose -p pulse up -d"` (base+override demo on :80).

**Remaining real-AMS step** (do when you have a real Ant Media Server): set the `PULSE_AMS_*` vars in
`deploy/.env` and add `-f deploy/docker-compose.real-ams.yml` (disables mock-ams); then
`POST /api/v1/admin/sources/{id}/test`. Until then the dashboard shows **0 viewers** (honest —
mock-ams has no streams). Pair this with W2c below. Adversarially verify; ORCH gate; commit + handoff.

### ✅ W2c — `pulse-amsclient-hardening` — DONE (D-025, author + unit tests)
Workflow `pulse-amsclient-hardening` (`wf_4aab2501-0a4`) hardened `server/pkg/amsclient` +
`internal/collector` against real AMS wire variance. Added `amsclient`'s **first** tests:
`client_test.go` (11) + 10 `testdata/*.json` fixtures driving the real `getJSON`/httptest decode
path (v2.10/v2.14/v3.0 field variance, mixed statuses, empty list, unknown-fields+nulls, exactly-200
pagination, non-2xx error, cluster role/version, applications envelope, partial WebRTC). Fixed
3 latent bugs (node `version` dropped → `Data["version"]`; v2.10 `speed`→bitrate fallback; empty
`StreamID` guard) + Kafka `dashViewerCount` parity. Verified: full `go test ./... -race` green
(workflow gate + independent ORCH re-run) + adversarial diff review.

**Still pending (needs a real AMS):** validate these fixtures against **real** AMS REST captures
once the real-ams overlay is connected (see the W2b real-AMS step above). The unit/wire layer is done.

### Also open (carry-forward)
- **QoE/beacon end-to-end**: integrate `sdk/beacon-js` into AMS player pages (needs a Pro+ license to
  lift the ingest gate so `beacon_events` populate `qoe/summary`).
- The **USER GitHub-side TODO** above (Actions-green PR · `branch-protection.sh` · `v*` tag).

### Optional follow-on workflows (ask the user first)
- `pulse-enterprise` — SSO (OIDC), white-label (brand config + branded PDF), air-gapped licensing.
- `pulse-portability-spike` — protocol-level beacon vs. a non-AMS HLS source (Wowza/Red5/Flussonic).
- `pulse-mobile-sdks` — Android/iOS/Flutter beacons (native toolchains may be absent → author +
  unit-test, execution waived).
- `pulse-techdebt` — VD-04 headless render-time + VD-14 player-CPU via Playwright/CDP; long-run
  anomaly false-alarm simulation.

## Every workflow MUST include these flows (binding user directive)

- **Verification flow** — an independent/adversarial re-check of *every* claim against a running
  stack or a fresh build. **QA is NOT authoritative alone** (D-013/D-017/D-019): rebuild and
  re-run the guard, default to "refuted" until reproduced. Make it a dedicated workflow phase.
  *(W1 lesson: a verify harness whose own asserts are wrong will "refute" correct code — when a
  job stays refuted, re-run the REAL artifact's logic yourself before believing it. The e2e job
  was correct; my prescribed assert was missing auth + seeding.)*
- **Commit flow** — commit by **EXPLICIT path**, per scope (never `git add -A`/`-u`/`.`; parallel
  agents share the tree — D-008/D-011). In a single orchestrated workflow, prefer having agents
  AUTHOR only and let ORCH commit centrally by explicit path (avoids `.git/index.lock` races).
  Message `<scope> <id>: <summary>` + evidence. **Push to `main` when the user directs** (this
  session pushed `cc3e008` + `6254d90`).
- **Handoff flow** — UPDATE **this `RESUME-PROMPT.md`** with the new status + next workflows, then
  commit + push it. **Keep it current every session** (binding user directive).

## Operating protocol (binding — learned the hard way)

- **Orchestrate with the Workflow tool.** One phase of work = one Workflow; ORCH writes the plan +
  pre-approved CRs to `decisions.md`, fans out to disjoint-scope agents, then **independently gates**
  before accepting. Background work is harness-tracked — you're re-invoked on completion; don't poll-spin.
- **Per-scope commits (D-008/D-011):** verify acceptance THEN commit by EXPLICIT path; on
  `.git/index.lock` busy, bounded wait+retry, never delete. ORCH owns `DEVLOG`/`decisions`/
  `IMPLEMENTATION_LOG`/`RESUME-PROMPT`.
- **Anti-stall (D-016 — a run once hung 9 h):** NEVER run a server/ClickHouse in the foreground
  (`pulse serve`, `clickhouse server`) **inside an agent**. For deploys use `docker compose up -d`
  (detached) + health polling; for CH unit work use the Go integration harness
  (`go test -tags integration`). Put `timeout` on builds and `-timeout` on `go test`; vitest `run`
  not watch. **Never leave a foreground `curl` without `-m`.** For long local repros (e.g. an e2e
  bring-up that needs `sleep`), run the script with Bash `run_in_background: true` — its sleeps are
  fine there and you're notified on completion. If a command hangs, kill it (TaskStop / kill the PID).
- **Single-writer scope map** in `agents/manifest.yaml`. Contracts frozen (D-004) — changes only via
  ORCH-approved CRs applied by INT-01.

## Hard rules (CLAUDE.md / ARCHITECTURE §3)

- AMS wire formats ONLY in `server/pkg/amsclient` + `server/internal/collector`; metrics in
  ClickHouse, config in the meta store, never crossed; web UI consumes ONLY generated public-API
  types; beacon ingest is hostile-input territory.
- `CGO_ENABLED=0` for the shipping build (pure-Go modernc.org/sqlite); single binary
  `pulse serve|migrate|diag`; React 19 + RR7 + Vite 6 + TS strict; recharts; no external fonts/CDNs.
  **NB:** `go test -race` needs CGO_ENABLED=1 + gcc — keep it off the build steps but on for the
  race test (CI ubuntu-latest has gcc).
- **4 tiers** per PRD §7.11 (free / pro / **business** / enterprise) — `business` in the contract
  enum and `internal/license/license.go` (D-014).
- Deploy fixes live in `deploy/` (Dockerfile + overrides). Base `deploy/docker-compose.yml` stays
  clean (no host port bindings — uses `expose:`); host exposure lives in overrides
  (`docker-compose.override.yml` = demo `:80`; `docker-compose.ci.yml` = CI loopback). `make up`
  (`cd deploy && docker compose up -d`) auto-merges ONLY `override.yml`.

## Environment (VPS with Docker)

- **Ubuntu 24.04 VPS** (`161.97.172.146`), x86_64, Docker 29 + Compose v5. **`go` is NOT on PATH** —
  Go runs only inside Docker (`golang:1.25-alpine` build; `golang:1.25` debian for `-race`/gcc).
  node 20 + npm 10 are on PATH. `gh` is **NOT installed**.
  - ⚠️ **For `go test` mount the REPO ROOT, not just `server/`** (D-028 lesson): the api tests'
    `metaDDLPath` reads `../../../contracts/db/meta/0001_init.sql`, which escapes a `server/`-only
    mount → `t.Skip` → **skip-counts-as-pass false green** (~90 api tests silently skipped). Use
    `-v /home/aytek/repo/ams-pulse:/repo -w /repo/server -e GOFLAGS=-buildvcs=false` and confirm the
    census (`-v -count=1`): expect **0 SKIP** for the api package.
- **Docker access:** user `aytek` is in the `docker` group but stale in non-login shells — prefix
  with `sg docker -c "…"` (no password). **`sudo` requires a password** — ask the user via the
  `! <cmd>` prompt for privileged ops (Docker/gh install, `ufw`, etc.).
- **Run the stack:** `cd deploy && sg docker -c "docker compose up -d --build"` (base +
  `docker-compose.override.yml`). Admin token: `docker compose logs pulse | grep plt_`. `deploy/.env`
  holds `PULSE_SECRET_KEY` (gitignored). For a clean e2e: add `-f docker-compose.ci.yml -p pulse-e2e`
  and an ephemeral `PULSE_SECRET_KEY`.
- **CI verification locally:** reproduce a ci job by running its commands in the matching image,
  e.g. `sg docker -c 'docker run --rm -v $PWD:/repo -w /repo/web node:22-alpine sh -c "npm ci --legacy-peer-deps && npm run build && npm run lint && npm test"'`.
- **Firewall:** `ufw` active (default DROP) but **Docker-published ports bypass ufw** (DNAT/FORWARD).
- `*.db*`, `web/pulse_secret.key`, `deploy/.env` are gitignored — never commit. Work on `main`.
