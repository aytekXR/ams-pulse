# SESSION-100 — the marketplace-wait gate caught a LIVE PROD OUTAGE (D-164)

> Opened 2026-07-23 ~18:30 local under `sessions/SESSION-99.md`'s wait protocol. Repo
> `/home/aytek/repo/ams-pulse` on the VPS (**this host IS prod**; no SSH).
> Prod now at **v0.4.0-139-gf9e9c69**, tier `enterprise`, collecting from the real AMS.

## What the gate found

The two-minute gate said "nothing new" on every documented axis — `gradle`/`java`/`xcodebuild`
absent, `main` unchanged at `f9e9c69`, no operator input, no new PRs from the operator. Under the
S99 protocol that means *re-arm and stop in one line*.

The standing prod health-read is what broke the tie. It found the collector dead:

| signal | reading |
|---|---|
| `server_events` last row | **2026-07-23 08:58:20 UTC**, after a steady ~2,160 rows/h |
| prod boot log | `version=dev`, `ams_url=http://mock-ams:9090`, `tier=free` |
| `restpoller` | one DNS failure every 5 s, for 7 h 46 m |
| `/healthz` | `collector: "ok"` — the entire time |

**Root cause: the un-swept delta, applied to infrastructure.** PR #199 shipped
`deploy/nginx/deployment.sh` — a deploy script the loop had never reviewed, because S99 swept #199's
*docs* and not its *code*. The script hardcoded `COMPOSE_FILES` to `docker-compose.prod.yml` alone,
so every run dropped `real-ams.yml` (which sets `PULSE_AMS_URL` **and** is the only file that maps
`PULSE_LICENSE_KEY`) and `backup.yml`. One omission, three silent failures: blind collector,
Enterprise→Free downgrade, and an unstamped `pulse dev (commit unknown)` binary.

## What shipped (D-164, branch `s100-d164-prod-restore-collector-health`)

1. **Prod restored** (operator-approved in-session): `pre-d164` tag → stamped build → stamp asserted
   before deploy → canonical 3-file `up -d`. Zero poll errors, 1,265,906 rows of history intact,
   public edge 200. ClickHouse volume / host nginx / certbot / `antmedia` untouched.
2. **`deployment.sh` fixed** — canonical overlay set, stamped builds, and a **new step 6 that
   asserts Pulse is actually COLLECTING** before declaring success. The deploy that caused this
   would now fail loudly and roll back. `COLLECTOR_GRACE=0` opts out during AMS maintenance.
3. **`/healthz` made honest** — the collector component was a liveness proxy that can never go false
   once the aggregator holds a snapshot. It now ages out (`domain.CollectorHealth`, 3 missed
   intervals, floor 30 s), names the cause, and reports never-succeeded-since-boot as degraded.
   Stays HTTP 200 when degraded so a transient AMS outage cannot trip liveness probes into a
   restart loop. 12 new tests; full server suite green under `-race`.

> **⚠ PRECISION — what is and is not live.** The prod restore (item 1) was a **configuration** fix and
> is fully live. Items 2 and 3 are **code**, committed on the branch and NOT yet in prod: the running
> image is `v0.4.0-139-gf9e9c69`, built from `main` before those commits. So prod today is collecting
> correctly but still carries the old liveness-only `/healthz`. The collector-freshness signal reaches
> prod on the **next roll after this PR merges** — which is why SESSION-101 verifies it in an isolated
> stack rather than by reading prod's `/healthz`.

## ⚠ Environment truths for whoever reads this cold

- **A SECOND SESSION IS WRITING THIS REPO.** ~20 files gained uncommitted edits during S100 that this
  session did not make (`CLAUDE.md`, `Makefile`, `web/package.json` + lockfile, `web/vite.config.ts`,
  `docs/AMS-INTEGRATION.md` −213 lines, several runbooks). S100 briefly stashed them, then **restored
  them in full** and committed only its own seven paths. **`git status` and re-read before every
  edit**, and never `git add -A` here. Two of those edits are wrong or risky — see operator-expected
  §4 (`@eslint/js` downgraded ^10→^9; `docs/product.md` F2 tier flipped to Pro+, which is factually
  wrong: analytics is retention-gated, Pro = 90 days, "13-month rollups" = Business+).
- **The canonical prod command is load-bearing.** `deploy/runbooks/upgrade-rollback.md` says "never
  omit an overlay" and means it: one missing overlay = blind collector + Free tier, with no crash and
  no alert. Prefer `bash deploy/nginx/deployment.sh` now that it is fixed.
- **`up --build` never stamps.** Always `compose build --build-arg VERSION/COMMIT/BUILD_DATE` first,
  then `up -d` with no `--build`, and assert the stamp before deploying (D-058 lesson b, re-learned).
- Still true: host nginx owns :80/:443 (never touch it or certbot without operator direction); CI
  keeps its own Caddy deliberately; `gofmt`/Go tests only via docker (`-buildvcs=false` is needed —
  the container trips Go's VCS stamping on this mount); never `docker compose down -v` on prod.
- **Attribution, corrected:** S100 first suspected its own read-only verification subagents of making
  those edits. They did not. The concurrent session identified itself — it is a **review/staleness
  session**, and it recorded S100 as *the* intruder in its own notes ("theirs superseded mine on
  `deployment.sh`" — it was independently converging on the same deploy-script fix). Two sessions were
  fixing the same file from opposite ends. The lesson is not "distrust subagents", it is **check
  `git status` before attributing any edit, and commit by explicit path so a collision cannot silently
  absorb someone else's work.**

## Open on the operator (see `docs/operator-expected.md` ★S100)

1. Nothing blocking. Prod is healthy.
2. A **permanent 7 h 46 m data hole** on 2026-07-23 (08:58–16:44 UTC) — affects any report,
   screenshot or demo recording covering today.
3. **★ Decision: the 18-PR Dependabot queue** (oldest 8 days, outside the policy's 1-week target,
   previously recorded as "deliberately operator-held") — confirm the hold, or authorize absorption.
4. Whether the concurrent session is intentional.
5. Carried, unchanged: the 6-step submission sequence, `[FO-1]`, `[20]`, Android JVM/Gradle (standing
   GO), AMS trial/PAYG, rotate chat-exposed creds.

---

# SESSION-101 — plan

**Goal: verify the D-164 fixes under real conditions, then return to the wait.** This is a
*verification* session, not a build session — D-164 changed a live health path and a deploy script,
and neither has yet been proven against the failure it was written for.

## At open — the standard gate (unchanged)
1. `command -v gradle && command -v java` → if present, the Kotlin SDK auto-starts (standing GO D-154).
2. `docs/operator-expected.md` top block — a Dependabot ruling, a capacity number, G-27 answers,
   D-081 approval, or an `[FO-1]`/`[20]` ruling outranks everything below.
3. **New standing check: `git status`.** If the concurrent session's ~20 uncommitted files are still
   there, do not touch them; if they have landed, re-verify the two flagged edits (§4).

## Lead A — prove D-164 works (the reason this session exists)
- **Prod still collecting?** `server_events` row count over the last hour must be non-zero and the
  rate comparable to the pre-outage ~2,160/h baseline.
- **Does the new health signal actually fire?** Exercise it **without touching prod**: bring up an
  isolated stack (`-p pulse-d164-verify`, never the prod `.env`) pointed at an unreachable AMS,
  confirm `/healthz` goes `collector: degraded` with the cause named after ~30 s, then point it at a
  reachable mock and confirm recovery to `ok`.
- **Does `deployment.sh` step 6 actually catch it?** Same isolated project: run the script against a
  dead AMS and confirm it fails and rolls back rather than reporting success. This is the assertion
  that would have prevented D-164 — it must be proven, not assumed.
- Then tear the verify stack down (`down -v` is safe ONLY for that throwaway project name).
- **Fix ROADMAP §2.46 — the backup sidecar startup race** (found at S100 close, fully autonomous). The
  daemon runs its first cycle with no readiness wait, so both of 2026-07-23's host reboots cost a
  ClickHouse backup (`Connection refused (clickhouse:9000)` → 24 h sleep). S100 ran a manual cycle to
  restore a current recovery point, but the race is untouched. Add a bounded readiness wait + retry;
  consider skipping retention pruning on a failed cycle so a broken path cannot erode an intact set.

## Lead B — operator-input driven (only if provided at open)
- **Dependabot ruling (b) = absorb** → run the policy's batch protocol (`docs/dependabot-policy.md`
  §2a–2c): digest/patch first with the staging smoke, then minors, then pre-verify the two majors
  (`typescript` 7, `@types/node` 26) **together** in scratch checkouts before landing either.
- Capacity number / G-27 answers / `[FO-1]` / `[20]` / D-081 approval / demo rough-cut GO — as
  pre-scoped in `sessions/SESSION-99.md` Lead B (unchanged).

## Lead C — nothing new
Re-arm at max interval and stop in one line. Do NOT re-sweep (S89/S91/S92 ×3 + S95 delta + S96–S99),
and do NOT manufacture an arc. **But keep reading prod health at every gate** — that read, not the
documented checklist, is what caught D-164.
