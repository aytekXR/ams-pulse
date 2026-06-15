# Pulse — Next-Session Prompts (post functional-MVP deployment)

## Where we are (2026-06-15)

Pulse reached a **functional MVP**: the full
`mock-AMS → collector → ClickHouse → aggregator → API → web UI` pipeline works,
deployed via Docker Compose and verified live (correct viewer counts, real-time
updates on publish/unpublish/viewer changes, correct license-tier gating).

- Committed as `feat: functional MVP …` (see the commit on `main`).
- Live demo (MVP/demo-grade) was exposed at `http://161.97.172.146/` — UI+API on
  host `:80 → pulse:8090`, plain HTTP, admin API public, ClickHouse auth relaxed.
  This is **demo-grade only** (see the override comments + commit message).
- Getting a working stack required deploy-layer fixes (npm `--legacy-peer-deps`,
  builder `golang 1.24→1.25`, `/var/lib/pulse` ownership, `CLICKHOUSE_SKIP_USER_SETUP`,
  `PULSE_META_DDL_PATH`, one-shot `pulse migrate` for the CH schema — the D-002 gap)
  and two server fixes (web-UI static serving was never registered in `buildRouter`;
  `/api/v1/qoe/summary` returned an empty 200 body due to NaN-in-JSON).

Two follow-up sessions are planned. Paste the relevant prompt to start one.

---

## Prompt A — Stand up CI/CD (always-on, gates `main`)

```
Set up CI/CD for the Pulse repo so every push and pull request is built, linted, and
tested automatically, and broken changes cannot merge to main.

Repo: monorepo with three components —
- server/  : Go >= 1.25 (`go vet/build/test ./...`); has ClickHouse integration tests.
- web/     : Node + Vite/React. IMPORTANT: `npm ci` needs `--legacy-peer-deps`
             (eslint@9 vs @eslint/js@10 peer conflict). Scripts: lint, build, test.
- sdk/beacon-js/ : TS SDK with a hard 15 KB size gate (`npm run size`).
The all-in-one image is deploy/docker/pulse.Dockerfile (builder pinned golang:1.25-alpine).
`make build|test|lint` delegate per component.

Deliver GitHub Actions under .github/workflows/:
1) ci.yml on pull_request + push to main, jobs (fail-fast: false, with go/npm caching,
   concurrency cancel-in-progress):
   - server : Go 1.25, `cd server && go vet ./... && go build ./... && go test ./... -race`,
              with a ClickHouse service container so integration tests run (or isolate
              build-tagged integration tests into their own job).
   - web    : `cd web && npm ci --legacy-peer-deps && npm run lint && npm run build && npm test`.
   - sdk    : `cd sdk/beacon-js && npm ci && npm run build && npm run size` (fail if >15 KB).
   - docker : `docker build -f deploy/docker/pulse.Dockerfile .` (prove the image builds).
   - e2e (PR only): bring up deploy/docker-compose.yml + docker-compose.override.yml,
              wait for health, drive mock-ams traffic, assert /healthz ok,
              /api/v1/live/overview returns the published viewers, pulse-migrate exited 0,
              and `SHOW TABLES FROM pulse` is non-empty; then `compose down -v`.
2) Make these checks REQUIRED for merging to main (branch protection); provide a `gh` script
   to enable required status checks + require 1 review.
3) release.yml on tag v*: build and push the pulse image to GHCR tagged with the version.
4) No secrets in CI; deploy/.env stays gitignored.

Verify by opening a draft PR: confirm all jobs run green, and that a deliberately broken
change (failing test) blocks the merge. Report the workflow files and branch-protection setup.
```

---

## Prompt B — The next state (productionize + real AMS)

```
Take the functional MVP to a production-ready, real-AMS deployment.

Current state: full pipeline works against the mock AMS and is verified; but the live
exposure is MVP/demo-grade — UI+API on public plain HTTP (:80), ClickHouse auth relaxed
(CLICKHOUSE_SKIP_USER_SETUP), admin API reachable without TLS.

Goals (change contracts before code; keep deploy/docker-compose.yml clean and put
environment-specific settings in overrides; verify each step against a running stack):

1. Real AMS integration
   - Add deploy/docker-compose.real-ams.yml (no mock): PULSE_AMS_URL + PULSE_AMS_AUTH_TOKEN
     + PULSE_AMS_NODE_ID + PULSE_AMS_APPLICATIONS for a real AMS 2.x.
   - Validate read-only polling end-to-end; use POST /api/v1/admin/sources/{id}/test first.
   - Harden pkg/amsclient for real wire-format variance (extra/missing fields, status values,
     pagination, cluster vs standalone, AMS version differences). Capture real responses as
     fixtures and add parser tests.

2. Security / production hardening
   - TLS-terminating reverse proxy (Caddy/nginx) + a real domain; move UI/API off public
     plain HTTP back to internal + proxy.
   - Restore ClickHouse auth (CLICKHOUSE_USER/PASSWORD, thread creds into the DSNs, drop
     SKIP_USER_SETUP). Manage PULSE_SECRET_KEY + AMS token via a secrets manager.
   - Review CORS / WS allowed origins for the real domain.

3. QoE / beacon
   - Integrate sdk/beacon-js into AMS player pages; validate the QoE pipeline end-to-end
     (needs a Pro+ license to lift the ingest gate so beacon_events populate qoe/summary).

4. Ops
   - Backups + retention for ClickHouse and the SQLite meta store; container resource limits;
     metrics scraping (PULSE_METRICS_TOKEN); a short runbook.

Report PASS/FAIL per goal with evidence against the running stack.
```
