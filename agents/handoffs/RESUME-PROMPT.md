# Resume prompt — Pulse (next session)

> Updated 2026-06-15 (**session 3**). The functional MVP is now **DEPLOYED end-to-end on a
> real VPS via Docker Compose** against the built-in mock AMS — **closing the long-standing
> D-002 waiver** — and committed + pushed to `main`. The next phases are defined below **as
> Workflows**, each with a built-in **verification flow** and a **commit + handoff flow**.
> Paste this into a fresh Claude Code session started in the repo root
> (`/home/aytek/repo/ams-pulse` on the VPS).

## ✅ Status

**Prior (sessions 1–2):** MVP (F1–F10) + **Wave 3-Plus** (Phase-3 tech-debt & accuracy)
complete, independently verified, on `main`. Gate **CLOSED** (D-019, PASS_WITH_LIMITATIONS).
Authoritative artifacts: `IMPLEMENTATION_LOG.md`, `DEVLOG.md`, `agents/handoffs/decisions.md`
(**D-001…D-019** binding), `qa/wave-3-plus/gate-report.md`.

**This session (session 3) — D-002 waiver CLOSED.** Brought the FULL stack up on an Ubuntu
VPS via Docker Compose against the mock AMS and verified end-to-end; exposed a working
dashboard on the internet.
- Pipeline `mock-AMS → collector → ClickHouse → aggregator → API → web UI` verified **live**:
  559 viewers across cam1/cam2; real-time publish/unpublish/viewer-change propagation within
  the 5s poll; license-tier gating correct (anomalies=Enterprise, probes=Pro → 403 on Free).
- Getting a working stack required **5 deploy fixes + 2 server bugs** (commit `c5d882f`):
  npm `--legacy-peer-deps` (eslint@9 vs @eslint/js@10); builder `golang 1.24→1.25`
  (`go.mod` needs ≥1.25); `/var/lib/pulse` owned by `pulse` (SQLITE_CANTOPEN); ClickHouse
  `CLICKHOUSE_SKIP_USER_SETUP=1`; `PULSE_META_DDL_PATH` so serve creates meta tables + the
  first-run admin token; one-shot **`pulse migrate`** that creates the CH schema (the real
  D-002 gap — `pulse serve` does NOT run CH migrations); **web-UI static serving** registered
  in `buildRouter` (was 404; assets shipped but unserved); **`/api/v1/qoe/summary`** NaN→valid
  empty JSON (was an empty 200 body). `go test ./internal/api` ok (0 fail).
- **Live (DEMO-GRADE)** at `http://161.97.172.146/` — UI+API on host `:80 → pulse:8090`, plain
  HTTP, admin API public, CH auth relaxed. **Admin token** is printed once to
  `docker compose logs pulse` (first-run). `deploy/docker-compose.override.yml` carries the
  mock-AMS + migrate + exposure config; `deploy/.env` (gitignored) holds `PULSE_SECRET_KEY`.
- Committed + pushed to `main`: `c5d882f` (MVP) + the resume/handoff commit.

## Next session — run these Workflows (orchestrate with the Workflow tool)

Default order **W1 → W2**; the rest are optional (ask the user). **Every** workflow MUST end
with the Verify + Commit + Handoff flows defined in the next section.

### Workflow 1 — `pulse-cicd` — stand up always-on CI/CD (gates `main`)
Goal: every push/PR to `main` is built + linted + tested; broken changes cannot merge.
Phases:
1. **Plan/contracts** — ORCH writes the CI plan + branch-protection policy to `decisions.md` (new CR).
2. **Author** (parallel agents, disjoint files under `.github/workflows/`):
   - `server`: Go 1.25 → `cd server && go vet ./... && go build ./... && go test ./... -race`,
     with a ClickHouse **service container** so integration tests run (or isolate
     build-tagged integration tests into their own job).
   - `web`: `cd web && npm ci --legacy-peer-deps && npm run lint && npm run build && npm test`.
   - `sdk`: `cd sdk/beacon-js && npm ci && npm run build && npm run size` (fail if >15 KB).
   - `docker`: `docker build -f deploy/docker/pulse.Dockerfile .` (prove the image builds).
   - `e2e` (PR only): compose up (base + override), wait for health, drive mock-ams traffic,
     assert `/healthz` ok + `/api/v1/live/overview` shows published viewers + `pulse-migrate`
     exited 0 + `SHOW TABLES FROM pulse` non-empty; then `compose down -v`.
   - `release.yml` (on tag `v*`): build + push the pulse image to GHCR, tagged with the version.
3. **VERIFY (adversarial)** — open a draft PR; confirm all jobs go green; push a *deliberately
   broken* change (failing test) and confirm the merge is **BLOCKED**; confirm required status
   checks + branch protection on `main` (provide a `gh` script).
4. **GATE** — ORCH independently inspects the PR checks / re-runs locally (QA not authoritative
   alone — D-013/D-017). Only then accept.
5. **Commit + Handoff** (see protocol below).

### Workflow 2 — `pulse-productionize` — TLS + real AMS + hardening
Goal: production-ready, real-AMS deployment. Current live exposure is DEMO-GRADE.
Phases:
1. **Plan/contracts.**
2. **Author** (parallel where disjoint):
   - **Real AMS**: add `deploy/docker-compose.real-ams.yml` (no mock) wiring `PULSE_AMS_URL` +
     `PULSE_AMS_AUTH_TOKEN` + `PULSE_AMS_NODE_ID` + `PULSE_AMS_APPLICATIONS`; harden
     `pkg/amsclient` for real wire-format variance (extra/missing fields, status values,
     pagination, cluster vs standalone, AMS version differences) with captured fixtures + tests.
   - **Security**: TLS-terminating reverse proxy (Caddy/nginx) + real domain; move UI/API off
     public plain HTTP back to internal + proxy; restore CH auth (CLICKHOUSE_USER/PASSWORD,
     thread into DSNs, drop SKIP_USER_SETUP); secrets manager for PULSE_SECRET_KEY + AMS token;
     review CORS / WS allowed origins.
   - **QoE/beacon**: integrate `sdk/beacon-js` into AMS player pages; validate end-to-end
     (needs Pro+ license to lift the ingest gate so `beacon_events` populate `qoe/summary`).
   - **Ops**: backups + retention (ClickHouse + SQLite meta); resource limits; metrics scraping
     (`PULSE_METRICS_TOKEN`); a short runbook.
3. **VERIFY** — against a running stack: `POST /api/v1/admin/sources/{id}/test` for the real AMS;
   re-run the e2e smoke; **adversarially verify each goal** (try to refute "it works" before
   accepting); confirm the public surface is locked down + TLS valid.
4. **GATE** — ORCH independent re-gate on HEAD.
5. **Commit + Handoff.**

### Optional follow-on workflows (ask the user first)
- `pulse-enterprise` — SSO (OIDC), white-label (brand config + branded PDF), air-gapped
  licensing (signed offline license). Maps to PRD "2 Enterprise logos".
- `pulse-portability-spike` — protocol-level beacon against a non-AMS HLS source
  (Wowza/Red5/Flussonic). PRD "one non-AMS pilot".
- `pulse-mobile-sdks` — Android/iOS/Flutter beacons (native toolchains may be absent → author +
  unit-test, execution waived).
- `pulse-techdebt` — VD-04 headless render-time + VD-14 player-CPU via Playwright/CDP; long-run
  anomaly false-alarm simulation.

## Every workflow MUST include these flows (binding user directive)

- **Verification flow** — an independent/adversarial re-check of *every* claim against a running
  stack or a fresh build. **QA is NOT authoritative alone** (D-013/D-017/D-019): rebuild and
  re-run the guard, default to "refuted" until reproduced. Make it a dedicated workflow phase
  (e.g. `parallel` skeptics per finding), not an afterthought.
- **Commit flow** — commit the work by **EXPLICIT path**, per scope (never `git add -A`/`-u`/`.`;
  parallel agents share the tree — D-008/D-011). Message `<scope> <id>: <summary>` + evidence.
  **Push to `main` when the user directs** (this session pushed `c5d882f` + handoff to `main`).
  Set a local git identity if the box has none (`git config user.email …`).
- **Handoff flow** — UPDATE **this `RESUME-PROMPT.md`** with the new status + the next workflows,
  then commit + push it. **Keep it current every session** (binding user directive). The old
  separate `NEXT-SESSION-PROMPTS.md` is retired — everything lives here.

## Operating protocol (binding — learned the hard way)

- **Orchestrate with the Workflow tool.** One phase of work = one Workflow; ORCH writes the plan
  + pre-approved CRs to `decisions.md`, fans out to disjoint-scope agents, then **independently
  gates** before accepting.
- **Per-scope commits (D-008):** verify acceptance THEN commit your own scope by EXPLICIT path
  (D-011); on `.git/index.lock` busy, bounded wait+retry, never delete. ORCH owns `DEVLOG`/
  `decisions`/`IMPLEMENTATION_LOG`/`RESUME-PROMPT`.
- **Anti-stall (D-016 — a run once hung 9 h):** NEVER run a server/ClickHouse in the foreground
  (`pulse serve`, `clickhouse server`) **inside an agent**. For deploys use `docker compose up -d`
  (detached) + health polling; for CH unit work use the Go integration harness
  (`go test -tags integration`). Put `timeout` on builds and `-timeout` on `go test`; vitest
  `run` not watch. **Never leave a foreground `curl` without `-m`** — that stranded 3 shells this
  session. If a command hangs, kill it (TaskStop / kill the PID).
- **Single-writer scope map** in `agents/manifest.yaml`. Contracts frozen (D-004) — changes only
  via ORCH-approved CRs applied by INT-01. Background work is harness-tracked: you're re-invoked
  on completion — don't poll-spin.

## Hard rules (CLAUDE.md / ARCHITECTURE §3)

- AMS wire formats ONLY in `server/pkg/amsclient` + `server/internal/collector`; metrics in
  ClickHouse, config in the meta store, never crossed; web UI consumes ONLY generated public-API
  types; beacon ingest is hostile-input territory.
- `CGO_ENABLED=0`; single binary `pulse serve|migrate|diag`; React 19 + RR7 + Vite 6 + TS strict;
  recharts; no external fonts/CDNs.
- **4 tiers** per PRD §7.11 (free / pro / **business** / enterprise) — `business` in the contract
  enum and `internal/license/license.go` (D-014).
- Deploy fixes live in `deploy/` (Dockerfile + overrides); keep `deploy/docker-compose.yml` (base)
  clean and put environment-specific config in overrides.

## Environment (UPDATED — VPS with Docker)

- **This session ran on an Ubuntu 24.04 VPS** (`161.97.172.146`), x86_64, **WITH Docker 29 +
  Compose v5**. The D-002 "no Docker" waiver is **CLOSED** for the mock-AMS path.
- **Docker access:** user `aytek` is in the `docker` group; if a shell's group set is stale,
  prefix commands with `sg docker -c "…"` (no password). **`sudo` requires a password** — ask the
  user (via the `! <cmd>` prompt) for privileged ops (Docker install, `ufw`, etc.).
- **Run the stack:** `cd deploy && docker compose up -d --build` (uses `docker-compose.yml` +
  `docker-compose.override.yml`). Admin token: `docker compose logs pulse | grep plt_`.
  `deploy/.env` holds `PULSE_SECRET_KEY` (gitignored).
- **Firewall:** `ufw` is active (default DROP) but **Docker-published ports bypass ufw** here
  (FORWARD/DNAT), which is why `:80`/`:8091` are internet-reachable without a ufw rule.
- **If a future session runs on a no-Docker dev box** (the old environment): Go/Node toolchains;
  ClickHouse via the Go integration harness only; re-download `/tmp/clickhouse` if wiped
  (`cd /tmp && curl -fsSL https://clickhouse.com/ | sh`).
- `*.db*`, `web/pulse_secret.key`, `deploy/.env` are gitignored — never commit. Work on `main`.
