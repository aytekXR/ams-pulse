# Resume prompt — Pulse (next session)

> Updated 2026-06-15 (**session 4**). **W1 `pulse-cicd` is DONE** — always-on GitHub Actions
> CI/CD that gates `main` (build + lint + test + docker-build + e2e + GHCR release) is authored,
> **independently verified locally in the real CI images**, and committed + pushed to `main`
> (D-020, gate CLOSED, PASS_WITH_LIMITATIONS). The next default is **W2 `pulse-productionize`**.
> Each workflow below has a built-in **verification flow** and a **commit + handoff flow**.
> Paste this into a fresh Claude Code session started in the repo root
> (`/home/aytek/repo/ams-pulse` on the VPS).

## ✅ Status

**Prior (sessions 1–3):** MVP (F1–F10) + **Wave 3-Plus** + a **functional MVP DEPLOYED on the
VPS via Docker Compose** against the mock AMS (closed the D-002 waiver). Gate **CLOSED**
(D-019). Authoritative artifacts: `IMPLEMENTATION_LOG.md`, `DEVLOG.md`,
`agents/handoffs/decisions.md` (**D-001…D-020** binding), `qa/wave-3-plus/gate-report.md`.

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

### ⚠️ FLAG — the demo stack is currently UNHEALTHY (not a W1 item)
`docker compose ps` (project `pulse`) shows the `pulse` container `Up (unhealthy)` and not
serving on `:8090`/`:80` (curl → no response). Next session should diagnose + restore it:
`cd deploy && sg docker -c "docker compose up -d"`, then
`sg docker -c "docker compose logs pulse | tail -50"` (suspect meta DB / CH connection /
first-run token / OOM). The CI override (`docker-compose.ci.yml`) and a fresh `pulse-e2e` stack
both came up healthy this session, so the image + pipeline are fine — it's a runtime state issue.

## Next session — run these Workflows (orchestrate with the Workflow tool)

Default: **W2** next; the optional follow-ons need a user OK first. **Every** workflow MUST end
with the Verify + Commit + Handoff flows defined in the next section.

### Workflow 2 — `pulse-productionize` — TLS + real AMS + hardening
Goal: production-ready, real-AMS deployment. Current live exposure is DEMO-GRADE (plain HTTP,
admin API public, CH auth relaxed). **Split by what is verifiable on this box vs. what needs
external infra** — author both, but only the first half can be self-verified here:

**A) Locally-verifiable subset (no external infra — DO + verify against the mock stack):**
1. **TLS reverse proxy**: add a Caddy/nginx TLS-terminating proxy (compose override) in front of
   pulse; move UI/API off public plain HTTP to internal-only + proxy; for local verify use a
   self-signed / `caddy internal` cert and `curl -k https://localhost`.
2. **Restore CH auth**: set `CLICKHOUSE_USER`/`CLICKHOUSE_PASSWORD`, thread into all DSNs, drop
   `CLICKHOUSE_SKIP_USER_SETUP`; verify migrate + serve + e2e still pass with auth on.
3. **Secrets**: move `PULSE_SECRET_KEY` + AMS token to env/secret-files (not committed); review
   CORS / WS allowed origins.
4. **`pkg/amsclient` hardening**: capture REST fixtures (extra/missing fields, status variants,
   pagination, cluster vs standalone, AMS version diffs) + unit tests that actually RUN.
5. **`deploy/docker-compose.real-ams.yml`** (no mock) wiring `PULSE_AMS_URL` + `PULSE_AMS_AUTH_TOKEN`
   + `PULSE_AMS_NODE_ID` + `PULSE_AMS_APPLICATIONS`; `docker compose … config -q` it.
6. **Ops**: backups + retention (ClickHouse + SQLite meta), resource limits, metrics scraping
   (`PULSE_METRICS_TOKEN`), a short runbook.

**B) Real-infra (NEEDS user inputs — WAIVE execution until provided, record as a D-021 waiver):**
- A **real production AMS**: URL + auth token + node id + applications, to run
  `POST /api/v1/admin/sources/{id}/test` and validate `pkg/amsclient` against real wire formats.
- A **real DNS domain** + Let's Encrypt for valid PUBLIC TLS (vs. the self-signed local cert).
- **QoE/beacon end-to-end**: integrate `sdk/beacon-js` into AMS player pages (needs a Pro+
  license to lift the ingest gate so `beacon_events` populate `qoe/summary`).

Phases: Plan/contracts (ORCH writes the CR + waiver to `decisions.md`) → Author (parallel, disjoint)
→ **VERIFY** (adversarial, against a running stack; refute "it works" before accepting; confirm the
public surface is locked down + local TLS valid) → **GATE** (ORCH independent re-gate on HEAD) →
Commit + Handoff.

### Optional follow-on workflows (ask the user first)
- `pulse-fix-demo` — diagnose + restore the unhealthy live demo stack (see FLAG above).
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
