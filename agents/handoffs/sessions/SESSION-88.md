# SESSION-88 — planned at S87 close (D-149) — F6 PHASE 3: [20] audit-log read model (the LAST F6 item; may be a product call)

> Written by SESSION-87 close (2026-07-18). Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`
> (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-114-ge295795** (F6 Phase 1+2 live).
> F6 is operator-directed ("start F6", D-148). Phase 1 (live tenant, BUG-009) ✅ and Phase 2 (tenant-scoped QoE alerts,
> [5]) ✅ are shipped. This is Phase 3 — the last F6 item — and it may be a **product ruling**, not a pure code change.

## ⚡ STANDING DIRECTIVE — still in force
Re-read ROADMAP-V2 §2 / `docs/assessment/` §5 and choose the next-highest-leverage move. **F6 is the active operator
priority** — finish it, but Phase 3 ([20]) intersects a long-pending operator decision, so **verify against the code and
do NOT guess a product ruling** (Lead B: if the operator names a different priority, do that). **Ultracode is on.**
**Workflow gotcha:** no backticks in workflow prompt prose. `gofmt -l` before every push.

## THE FIRST THING TO DO: adjudicate [20] against the code (it may not be a build)

**The gap ([20], S62 defer-by-ruling; D-130 / the S43 "reads-open" ruling):** `GET /admin/audit-log` is readable by ANY
authenticated token (a viewer-scoped token or viewer-role SSO user), exposing actor IDs/names, IPs, and every config
change. This is the DELIBERATE S43/D-105 "all authenticated reads are open; only writes need admin scope" model — it also
governs `GET /admin/users` and `GET /admin/tokens`. So [20] is **not obviously a bug**; it's a product call. It is also
not tenant-scoped (relevant to F6).

**At open, re-read against the code (verify-first — the last two phases proved the finding notes were stale/mis-scoped):**
1. `agents/handoffs/S62-AUDIT-FINDINGS.md` finding [20] + `decisions.md` D-130 (the S43 reads-open ruling) + the earlier
   operator-expected [20] write-ups.
2. The audit-log read handler (`server/internal/api/audit.go` + `server.go` route) + the meta audit-log query
   (`store/meta` audit-log list) — does it carry/accept a tenant? who can call it?
3. Decide the disposition — likely ONE of:
   - **(A) Product ruling (most likely):** keep reads open vs gate the whole admin-read surface behind `admin` scope
     (which would remove the audit page from viewer-role users) vs add an optional `?tenant=` filter. If it's a genuine
     either/or the operator must pick → write it up crisply in `operator-expected.md` with a recommendation and **do NOT
     change the access model unilaterally** (it's the same class as the S43 ruling). Mark [20] as adjudicated-to-operator.
   - **(B) Bounded code slice:** if the clean, non-controversial move is an OPTIONAL `?tenant=` filter on the audit read
     (mirroring Phase 1/2 — empty = all, backward-compatible, does NOT change who can read), implement it contracts-first
     (OpenAPI param + meta query `AND tenant=?` if the audit rows carry a tenant; if they DON'T, that's itself a finding —
     audit rows may need a tenant column, a bigger change → escalate) + param-conformance registry entry + mutation-prove
     + adversarial-review (data-isolation) + prod-roll.
4. **Whichever way:** record **D-150**; if F6 is then effectively complete (all of BUG-009/[5]/[20] dispositioned),
   **say so** and re-survey ROADMAP §2 for the next-highest-leverage move (the §2.7 CI-promotions gate unlocks 2026-07-23;
   the other checkpoint items remain operator-gated).

## Pipeline (if Phase 3 is a code slice — option B)
1. **Verify-at-open:** git clean (only Caddyfile). Record **D-150 IN PROGRESS**. Branch `s88-d150-f6p3`.
2. Contracts first (OpenAPI param + schema.d.ts if a `?tenant=` filter). 3. Implement + tenant-scope the audit query.
4. **Validate:** Go full 25-pkg suite via docker + **mutation-prove** the filter; web full `npm test`/build/typecheck/lint
   + param-conformance registry entry + floor bumps (D-147 pattern) if a new query param.
5. **Adversarial review (mandatory — this is the audit/access surface).**
6. **PR → CI → squash-merge --delete-branch → verify origin/main.** (Two-PR cadence: arc, then docs-close.)
7. **Roll prod forward** if server source changed (stamped rebuild + 5-check smoke). 8. **Close docs:** D-150, CHANGELOG
   (if user-facing), ROADMAP §2.37 (Phase 3), S62-AUDIT-FINDINGS [20], RESUME → SESSION-89, `operator-expected.md`,
   SESSION-88 CLOSED, SESSION-89 written. Re-arm the `/loop`.

## F6 phase map (track across sessions)
- **Phase 1 ✅ (D-148, S86):** server-side tenant resolution on live endpoints; `?tenant=` filter; BUG-009 closed.
- **Phase 2 ✅ (D-149, S87):** tenant-scoped QoE alert rules; S73 [5] closed.
- **Phase 3 (this, [20]):** audit-log read model — code slice OR operator product ruling.
- After Phase 3: F6 core complete; remaining multi-tenant polish is demand-driven.

## Environment gotchas (carried — unchanged from SESSION-87)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`.
  `gofmt`/`go` NOT on host PATH. Mutation copy `/tmp/pulsemut`; restore via `cp` (NEVER `git checkout`, D-096).
  Node at `/home/aytek/.local/bin/node`. Reusable: `internal/tenant` (Matcher + CachedResolver).
- **Web:** from `web/`, `npm test`; `npm run gen:api` regenerates `src/lib/api/schema.d.ts`. A new query param needs a
  `param_conformance_test.go` registry entry + floor bumps (`minSpecParams`, `minProbes` — D-147 pattern).
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. **D-058 stamped build:** `docker compose $DC build
  --build-arg VERSION=$(git describe --tags --always) --build-arg COMMIT=$(git rev-parse --short HEAD) --build-arg
  BUILD_DATE=$(date -u ...) pulse` THEN `docker compose $DC up -d pulse` (never mix `--build` into `up -d`). Prod
  **v0.4.0-114-ge295795**; rollback tags `pulse-prod-pulse:pre-d148` / `:pre-d148-fix` / `:pre-d149`. Read-only rootfs —
  new writes → `/var/lib/pulse` or `/tmp`. 5-check smoke: version stamp, healthz 200, signed webhook 200 (HMAC from
  PULSE_WEBHOOK_SECRET), limits 512M/0.5cpu, 0 error lines. Admin token (side-effect-free GET only, never commit):
  gitignored `oguz-testing.md`; API base `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). **Do-not-commit:** `deploy/config/Caddyfile.prod` stays modified/unstaged (verify
  `git diff --cached --name-only | grep -q Caddyfile` is empty before every commit). Commit trailer `Co-Authored-By:
  Claude Opus 4.8 (1M context) <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
