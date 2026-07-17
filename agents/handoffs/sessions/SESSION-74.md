# SESSION-74 — planned at S73 close (D-135) — first S73-audit fix cluster

> Written by SESSION-73 close (2026-07-17). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE + `agents/handoffs/S73-AUDIT-FINDINGS.md`** (the 8-finding ledger).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-75

Re-verify each finding against the code before building — take the verified CORE, not the audit's literal scope (the
S73 audit already flagged [5]'s fix sketch as wrong). Do NOT stop after one session — at close, update all docs,
regenerate this plan, record progress + operator-needs, continue until the roadmap is complete or a human/operator is
genuinely required. **Ultracode is on** — adversarial-review the security/state-machine surface; token cost is not a
constraint. **Workflow-script gotcha:** no backticks in workflow prompt prose. `gofmt -l` before every push.

## Goal — the config-startup cluster [2] + [3] + [6] (all in `server/cmd/pulse/`, coherent one-package PR)

`S73-AUDIT-FINDINGS.md`: 8 remain (3 HIGH, 5 MEDIUM). Lead cluster = the three `cmd/pulse` findings (2 HIGH + 1 MEDIUM),
self-contained, no cross-package threading:

### [2] HIGH — `server.Stop()` never calls `apiServer.Stop()` → HTTP not drained on SIGTERM
- **loc:** `server/cmd/pulse/serve.go:709` (server.Stop). `api.Server.Stop()` (api/server.go:390) exists and does the
  right thing (httpSrv.Shutdown 10 s + WS/eviction goroutine stops) but is never called.
- **Fix:** add `s.apiServer.Stop()` inside `server.Stop()` (~:726, before `s.store.Close()`). **Re-verify** the field
  name (`s.apiServer`?) and that Stop() is idempotent / safe to call in that order. **Test:** a Start→Stop lifecycle
  asserting the HTTP server is shut down and the goroutines exit (or that a second request after Stop fails). Mutation:
  remove the added call → test reddens.

### [3] HIGH — `PULSE_ANONYMIZE_IP=1` silently leaves IPs un-anonymized (exact `== "true"` compare)
- **loc:** `server/cmd/pulse/config.go:302` (AnonymizeIP) + `:248` (WebhookRequireTimestamp). Live path `loadEnvConfig`.
- **Fix:** add a shared `envBool(name string) bool` (`v=="1" || strings.EqualFold(v,"true")`) and use it at both sites
  (+ any sibling boolean env — audit `loadEnvConfig` for other `== "true"` compares). **Re-verify** the dead
  `internal/config` path isn't wired (it wasn't at audit time) — reconcile or leave. **Test:** table of `1`/`true`/
  `True`/`TRUE`/`0`/`false`/`""` → expected bool. Mutation: revert to `== "true"` → the `1`/`True` cases redden.

### [6] MEDIUM — `pulse diag` / `checkAMS` print the raw AMS URL (userinfo creds) unredacted
- **loc:** `server/cmd/pulse/main.go:253` (runDiag) + `server/cmd/pulse/migrate.go:131` (checkAMS). `runServe` already
  uses `url.Parse(...).Redacted()` (B10).
- **Fix:** apply the same `.Redacted()` at both sites (consider a tiny shared `redactURL(s string) string` helper).
  **Test:** a URL with `user:pass@` → printed output has no password. Mutation: drop the redaction → test reddens.

**These are `cmd/pulse` (the `main` package + `serve`/`config`/`migrate`).** NOTE: `cmd/pulse` is the `main`-ish package;
confirm the test package convention (there may be few existing tests there — check `server/cmd/pulse/*_test.go`). If the
functions are hard to unit-test (e.g. `runDiag` prints to stdout), extract the redaction/env-bool into testable helpers.

## Alternatives / later clusters
- **[1] HIGH query cross-tenant** (IngestTimeseries) — self-contained (add Tenant param + `AND tenant=?` + handler
  `q.Get("tenant")`), a strong standalone HIGH. Could even lead instead of the cmd cluster.
- **[4] MEDIUM** PruneAlertHistory race (store/meta, single-statement DELETE).
- **[5] MEDIUM** QoEForStream tenant — WIDER than filed (thread tenant through the live pipeline); scope carefully.
- **[7] MEDIUM** WS-token-in-URL (web + a new `/auth/ws-ticket` server endpoint; avoid the do-not-commit Caddyfile).
- **[8] MEDIUM** web SettingsPage silent error handlers (try/catch + toast; web-only CI).

## Pipeline (the S62→S72 loop)
1. **Verify-at-open:** `git log --oneline -3`, HEAD == origin/main, `git status` shows ONLY Caddyfile.prod dirty. Record
   **D-136 IN PROGRESS** in `decisions.md`. Branch `s74-d136`. **CHECK THE DATE** (§2.7 gate ≥ 2026-07-23).
2. **Re-verify each finding vs the code** (`mcp__codegraph__codegraph_explore`); take CORE.
3. **Fix → mutation-prove** on `/tmp/pulsemut` (NOT `/mut` — not writable; `cp -a /repo/server /tmp/pulsemut/server`,
   artifact-dir cp errors non-fatal). `go test -vet=off` for unreachable mutants; value-forcing mutations.
4. **Full Go suite** (25 pkgs) in docker; **`gofmt -l`** on changed files.
5. **Adversarial review** (shutdown-ordering + config-security surface): finder lenses → refute-by-default verifiers.
6. **PR → CI poll** (bounded background monitor; `csp-e2e`/`e2e` may flake — re-run) → **squash-merge --delete-branch**.
7. **Roll prod forward** (server source changed): `config -q` → tag `pulse-prod-pulse:pre-d136` → backup rc0 → STAMPED
   build backgrounded → assert stamp ≠ dev → `up -d pulse` no --build → 5-check smoke. **Extra smoke for [2]:** confirm
   graceful shutdown works (e.g. the container stops cleanly within the drain window — but do NOT disrupt prod
   unnecessarily; a lifecycle unit test is the primary proof).
8. **Close docs:** `decisions.md` D-136 SHIPPED, CHANGELOG, `S73-AUDIT-FINDINGS.md` [2]/[3]/[6] ✅ DONE, ROADMAP §2.32
   count, RESUME-PROMPT rotation, `operator-expected.md`, SESSION-74 CLOSED, SESSION-75 written. Re-arm the `/loop`.

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`
  (25 pkgs). `gofmt`/`go` NOT on host PATH. Node at `/home/aytek/.local/bin/node` (npx). Mutation copy `/tmp/pulsemut`.
- **CodeQL** flags `InsecureSkipVerify` in PRODUCTION Go only (S72); prefer keeping security controls on.
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC_ARGS="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. Prod at **v0.4.0-82-g8355127**. 5-check smoke: startup
  version stamp, healthz 200, signed webhook 200 (HMAC from PULSE_WEBHOOK_SECRET), limits 512M/0.5cpu, 0 error lines.
- **Admin token** (side-effect-free GET only, never commit): gitignored `oguz-testing.md`. Prod API base
  `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). Commit trailer `Co-Authored-By: Claude Opus 4.8 (1M context)
  <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
