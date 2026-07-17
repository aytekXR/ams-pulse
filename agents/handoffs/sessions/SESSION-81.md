# SESSION-81 — planned at S80 close (D-142) — the S80 review's one confirmed follow-up

> ## ✅ CLOSED (2026-07-17, D-143) — report-artifact retention prune SHIPPED → ★ §2.33 COMPLETE
> Shipped `PULSE_REPORT_ARTIFACT_RETENTION_DAYS` (default 90; `<=0` disables) + `Scheduler.pruneArtifacts()` — strictly
> bounded to regular `pulse-usage-*.{csv,pdf}` files in the reports dir (never the metastore/secret-key), runs each tick
> independent of schedule-listing. Also persisted artifacts on the volume in the BASE compose. **Its own adversarial
> review found 4 issues (HIGH: prune gated behind a schedule-listing error → decoupled via defer; MEDIUM: symlink guard
> via Type().IsRegular(); MEDIUM: envInt TrimSpace; LOW: base-compose persistence), ALL fixed pre-commit.** 8 mutations
> killed; full suite green. **Prod-verified** stamped rebuild `v0.4.0-98-g641b4e2` — 5-check smoke green, hardening
> persisted through the rebuild, reports scheduler at /var/lib/pulse/reports, 0 read-only/prune errors. PR #155
> squash-merged. **This completes ROADMAP §2.33.** **No operator action.** See `decisions.md` D-143 and
> `sessions/SESSION-82.md` for the next arc. Everything below is the original pre-session plan.


> Written by SESSION-80 close (2026-07-17). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-93-g8858b5f**; the pulse container now runs
> read-only-rootfs + cap_drop:[ALL] + no-new-privileges (D-142, verified live).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-82

Re-read ROADMAP-V2 §2 / `docs/assessment/` §5 and **choose the next-highest-leverage move.** Verify candidate status AND
product-viability against the code before committing. Take the verified CORE. Do NOT stop after one session — at close,
update all docs, regenerate this plan, record progress + operator-needs, continue until the roadmap is complete or a
human/operator is genuinely required. **Ultracode is on.** **Workflow-script gotcha:** no backticks in workflow prompt
prose. `gofmt -l` before every push; web gotchas (`vi.hoisted`, full `npm test` for coverage, binary embeds `web/dist`).

## Goal — report-artifact retention prune (the S80/§2.33 adversarial-review's 1 confirmed LOW finding)

The S80 hardening moved report artifacts from the **ephemeral container root** onto the **persistent pulse-data volume**
(`PULSE_REPORTS_DIR=/var/lib/pulse/reports`). That fixed a durability bug (artifacts were lost on every redeploy) but
**activated an accumulation path**: the reports scheduler writes artifacts and **never prunes** them, and
`PULSE_RETENTION_DAYS` governs only ClickHouse TTL, not files. The artifacts share the volume with the SQLite metastore
(`pulse_meta.db`, `pulse_secret.key`), so unbounded growth would eventually exhaust it. **Severity LOW** (monthly
CSV/PDF reports are KB-sized → decades to fill under realistic use), but it's a real gap and the S80 change is what made
it live, so close it.

### The fix (a scoped, well-tested prune)
- **loc:** `server/internal/reports/scheduler.go` — `storeArtifact()` (~:261-270) does `os.MkdirAll` + `os.WriteFile`
  with no cleanup; there is NO prune logic anywhere in the reports package. Config: `server/cmd/pulse/config.go:351`
  wires `PULSE_REPORTS_DIR` with no retention counterpart.
- **Add** `PULSE_REPORT_ARTIFACT_RETENTION_DAYS` (config.go, default e.g. 90 to mirror `PULSE_RETENTION_DAYS`; 0 =
  keep-forever/disabled). On each scheduler tick (or right after a successful write), prune artifacts older than the
  window.
- **SAFETY (this is the crux — deletion has blast radius):** the prune MUST be strictly bounded to `ArtifactsDir` AND
  match only the report filename pattern (e.g. the `pulse-*-YYYY-MM-DD-to-YYYY-MM-DD.{csv,pdf}` shape the scheduler
  emits). It must NEVER `os.Remove` `pulse_meta.db` / `pulse_secret.key` / the WAL/shm sidecars / any non-artifact file,
  even though (post-D-142) `ArtifactsDir=/var/lib/pulse/reports` is a SUBDIR of the metastore dir — so operate only
  inside the reports subdir, and additionally filename-gate. Age from the file's own mtime, not a parsed filename.
- **Wire it into the compose** hardened overlay if a non-default value is wanted (optional — the code default suffices).

### Pipeline (the standard loop)
1. **Verify-at-open:** git state clean (only Caddyfile + any carried docs). Record **D-143 IN PROGRESS** in `decisions.md`.
   Branch `s81-d143`. **CHECK THE DATE** — §2.7 CI-promotion gate unlocks **≥ 2026-07-23** (if today ≥ 07-23, strongly
   consider flipping the soft CI jobs to required instead of / in addition to this; it's a bounded high-signal move).
2. **Implement** the prune (config + scheduler). Keep the reports-package public surface stable.
3. **Mutation-prove** the prune predicate (Go: `/tmp/pulsemut`): prove the age cutoff AND the filename/dir guard are
   load-bearing — a mutant that widens the glob to non-artifact files, or flips `>` to `>=`/`<`, or drops the dir scope,
   must be caught by a test. Add a test that a metastore-shaped file in the dir is NEVER deleted.
4. **Full Go suite** (25 pkgs) + web if touched (web isn't expected to change).
5. **Adversarial review** (deletion logic is security-relevant — a finder lens specifically on "can this delete a
   non-artifact / the metastore / traverse out of ArtifactsDir").
6. **PR → CI poll** → **squash-merge --delete-branch** → verify origin/main.
7. **Roll prod forward** (server source changed → REBUILD needed this time — stamped build + version assert + `up -d
   pulse` + 5-check smoke; the report prune runs server-side so it needs the new binary).
8. **Close docs:** D-143, CHANGELOG, ROADMAP §2.33 (flip the follow-up ⏳→✅), RESUME rotation (→ SESSION-82),
   `operator-expected.md`, SESSION-81 CLOSED, SESSION-82 written. Re-arm the `/loop`.

## Alternatives (weigh at open)
- **§2.7 CI-promotions** if the date has unlocked (≥ 2026-07-23) — bounded, autonomous, high-signal; may outrank the
  prune. Check first.
- **§2.15 light-theme** from brandkit `tokens.json` — user-facing, tokens define it (autonomous-ish); a clean web arc.
- **Operator checkpoint** — if the autonomous backlog is genuinely thin after this, produce a crisp summary of the gated
  items ([20] audit-read, [5] per-tenant QoE alerting, §2.6 unsigned-webhook, §2.1 branch protection, GHCR-public,
  §2.15/§2.19 UI direction) and recommend, so the operator can unblock the next wave.

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`
  (25 pkgs). `gofmt`/`go` NOT on host PATH. Mutation copy `/tmp/pulsemut` (not `/mut`); copy `contracts` if the meta
  harness reads the DDL. **CodeQL** flags production `InsecureSkipVerify` only. `govulncheck` via `go run
  golang.org/x/vuln/cmd/govulncheck@latest` in the container (network works).
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC_ARGS="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. **The pulse container now runs read-only rootfs** — any new
  server-side file write MUST target `/var/lib/pulse` (the volume) or `/tmp` (tmpfs), never the root fs. Rollback tag
  `pulse-prod-pulse:pre-d142` exists. 5-check smoke: version stamp, healthz 200, signed webhook 200 (HMAC from
  PULSE_WEBHOOK_SECRET), limits 512M/0.5cpu, 0 error lines.
- **Admin token** (side-effect-free GET only, never commit): gitignored `oguz-testing.md`. Prod API base
  `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). Commit trailer `Co-Authored-By: Claude Opus 4.8 (1M context)
  <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
- **Do-not-commit:** `deploy/config/Caddyfile.prod` stays modified/unstaged (a clean git status is a FAILURE signal;
  D-082/D-096). Verify `git diff --cached --name-only | grep -q Caddyfile` is empty before every commit.
