# SESSION-45 — planned at S44 close (D-106)

> Written by SESSION-44 close (2026-07-15). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> Read `RESUME-PROMPT.md` + `ROADMAP-V2.md` §2 + the S44 audit backlog before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-46

Before dispatching: re-read ROADMAP-V2 §2 and the final-assessment §5 roadmap and **revise this plan if a
higher-leverage move exists.** This file is a starting point, not a contract. Record any revision in the
D-107 open block. **Verify candidate status AND product-viability against the code before committing** —
S38 and S43 overturned their own plans; S39–S42 and S44 confirmed theirs. The clause cuts both ways.

## ★★ READ FIRST — S44 flipped the "backlog is thinning" story

S43/S44 handoffs said clean-autonomous work was thinning. **That was wrong** — an 8-finder adversarial
audit at S44 open surfaced **13 CONFIRMED, mutation-checkable defects (0 refuted)**: a blocker, several
majors (2 security), and correctness bugs across families. S44 shipped the **security cluster** (3 fixes,
PR #85, D-106). **The remaining 10 are real, verified, and autonomous** — this is the S45–S47 backlog, and
it is genuine high-value engineering, not hygiene. Full detail with file:line + failure scenario:
`decisions.md` D-106 (and the S44 audit table). **Re-verify each against the code before building** (the
audit is a strong signal, not a licence to skip verification — S38/S43).

## S45 candidates — the audit backlog, ranked by severity/coherence

### PRIMARY — Reports scheduler correctness [M] (the single highest-severity finding + its sibling)
1. **BLOCKER — `PUT /reports/schedules/{id}` silently silences the schedule.** `reports_wave2.go:~177`
   (`reportScheduleFromAPI` returns `NextRunAt=nil, LastRunAt=nil`; the update handler copies only `ID` +
   `CreatedAt` from the existing row, never `NextRunAt`/`LastRunAt`) → editing a schedule's cron/format
   NULLs `next_run_at`, so the scheduler never fires it again. A paying Business customer edits their weekly
   report and it silently stops. **Fix**: carry `existing.NextRunAt`/`LastRunAt` (and recompute `NextRunAt`
   when the cron changed). Mutation-prove: edit → assert `next_run_at` preserved/recomputed, not NULL.
2. **MAJOR — `nextCronTime` ignores day-of-month for 5-field cron** → the UI "Monthly (1st, 6 AM)" preset
   (`0 6 1 * *`) fires **every day**. `reports/cron.go:~39` (`parseCronFieldsInternal` 5-field branch drops
   `fields[2]` DOM). **Fix**: honor the DOM field. Mutation-prove with `0 6 1 * *` → next-fire is the 1st,
   not tomorrow.

### S46 — entitlement + WS auth [M]
3. **MAJOR — probe runner executes probes after Pro→Free downgrade** (`prober/prober.go:~101`): CRUD gates
   `CheckProbes()` but the background `Runner.Run()` refresh tick does not, so a downgraded tenant keeps
   probing (S37 "enforced, not decorative" class). **Fix**: gate the runner tick on the live entitlement.
4. **MAJOR — `handleLiveWS` ignores cookie auth already validated by middleware** (`server.go:~1091`): an
   OIDC cookie-session user (no `Authorization` header) gets rejected from `GET /api/v1/live/ws` because the
   handler re-extracts from the header/`?token=` and ignores `ctxTokenKey`. **Fix**: read the validated
   token from context.

### S47 — audit integrity + hardening [S, several XS]
5. **MAJOR — `handleDeleteUser` returns 204 + false audit entry for a non-existent id** (`server.go:~2180`):
   `DeleteUser` ignores `RowsAffected`, so deleting a made-up UUID emits a fabricated `user.delete` audit
   row and 204 instead of 404. Corrupts the compliance trail (S38 missing-id class).
6. **MAJOR — `handleRevokeToken` same shape** (`server.go:~2045`): non-existent token id → false
   `token.revoke` audit + 204 (one verifier split CONFIRMED/REFUTED — re-verify: check whether
   `DeleteToken` surfaces rows-affected). **Fix 5+6**: check rows-affected → 404 + no audit when nothing
   was deleted.
7. **MINOR — `handleCreateUser` / `handleCreateToken` audit AFTER the re-fetch guard** (`server.go:~2115`,
   `~2031`): a committed create can go unrecorded if the re-fetch nils (the S40 bug class, fixed for
   *update* but missed for *create*). **Fix**: pre-assign the UUID and audit before the re-fetch.
8. **MINOR — `handleCreateToken` accepts an arbitrary `kind`** (`server.go:~2010`): no allowlist → a
   `kind:"superadmin"` token is stored but can never authenticate (dead row). **Fix**: allowlist
   `api`/`ingest`; 422 otherwise (positive-allowlist, D-098).
9. **MINOR — anomaly boundary `>` vs `>=`** (`alert/wave3.go:~250` eval vs `anomaly.go:~532` detect): the
   eval path uses strict `>` while the tick/detect path uses `>=`, so a z exactly at the sigma threshold is
   flagged on one path and not the other. **Fix**: make both `>=` (or both `>`), pick the detect-path
   semantics as canonical.

**Verify-first note (S43 lesson):** re-read each finding's cited code before building — confirm the bug still
holds and the fix has no compat/contract impact. Finding 6 had a split verdict; resolve it first.

## ⛔ At open — verify, do not assume (D-095 standing rule)

- `git log --oneline origin/main -4` — S44 (D-106, PR #85 `a280b56`) + the S44 docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-29-ga280b56`** (S44 roll-forward; rollback tag `pre-d106`). `/healthz` all-ok,
  `ams_env_configured: true`.
- Operator queue **live**: GHCR anon → 401; **AMS licence expiry still the 07-12 vs 07-27 doc discrepancy**
  (operator-only). CI promotions **date-gated ≥ 2026-07-23** — CHECK THE DATE; if eligible it is a clean win.

## 🔧 Environment gotchas (unchanged from S44 — read before any gate)

- **Go runs only in docker**: `sg docker -c "docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gomod:/go/pkg/mod -v pulse-gocache2:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./..."` (census 24/24).
- **Mutation-prove on a COPY** to keep the real tree clean: `cp -a /repo/server /tmp/mut && cp -a /repo/contracts /tmp/contracts` (tests resolve the meta DDL at `../contracts`), mutate `/tmp/mut`, test there. Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` (unstage only) is fine.
- **Contract change? `cd web && npm run gen:api`** even for description-only edits. **New migration? FIVE places** (incl. `meta_pg_integration_test.go sqliteSchemaVersions`); **0004 (audit_log) is the last shipped → next is 0005.**
- **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` AFTER `npm run build`; mutation-prove every e2e.
- **Do NOT overlap gate runs with heavy jobs.** **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy is LOCAL** — `deploy/runbooks/upgrade-rollback.md`: validate → tag `pre-dNNN` → backup → STAMPED build (`--build-arg`) → `up -d` WITHOUT `--build` → assert stamp ≠ dev/unknown → evidence smoke. Roll forward ONLY if server/web *source* changed.

## Binding lessons (carry into every wave)

1. **Verify product-viability AND candidate-status before building** (S38/S43). The audit is a signal; re-read the code.
2. **A gate with no test is not a gate; a green suite is not a working feature.** Mutation-prove every guard AND e2e. Audit on the committed-write path, before any re-fetch (S40).
3. **Independent adversarial review before merge for non-trivial code** (S40 found a real defect; S42/S44 confirmed clean). For a test-only change already mutation-proven, the mutation proof suffices (S43).
4. **Positive allowlists over blocklists** for authz (D-098) — applies directly to finding 8.
5. **No silent scope caps — and its inverse: don't invent scope.** When the highest-leverage work is operator-gated, say so; but S44 proved there IS real autonomous work when you go looking with an audit.

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — and VERIFY the merge landed on `origin/main`.**
2. `decisions.md` **D-107** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-46; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-46.md` (carry the standing-directive header + the remaining audit findings).
6. **Roll prod forward** if server/web *source* changed (the scheduler + entitlement fixes DO change source).
