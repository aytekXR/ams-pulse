> ## ✅ SESSION-64 CLOSED (2026-07-16, D-126) — PR #122 merged `fede961`, prod `v0.4.0-66-gfede961` (smoke green)
> **Shipped:** reports_wave2 post-mutation re-fetch cluster — [6] HIGH `handleUpdateReportSchedule` (DROPPED the
> redundant re-fetch: row is authoritative, no `updated_at` in the response → structurally eliminates the nil-deref +
> a DB round-trip), [5] HIGH `handleUpdateTenant` (KEPT the re-fetch — `updated_at` is stamped inside the store and
> not returned in row — but GUARDED it, mirroring `handleUpdateProbe`), [19] MEDIUM (SPLIT transient-error→500 from
> missing-row→404 in the three existence checks). **Verify-at-open confirmed all line numbers + the store behavior**
> that decided drop-vs-guard per handler. New `reports_wave2_s64_internal_test.go` (deterministic [19] proof via a
> pre-canceled request ctx — 500 not 404, mutation-proven) + `reports_wave2_s64_test.go` (update responses render
> fresh values; genuine missing row still 404; [6] non-vacuity checked). Full suite 24/24; gofmt + vet clean;
> self-review (no auth/contract/semantic surface). **Ledger:** [5]/[6]/[19] ✅ DONE; **18 S62 findings remain**
> (2 HIGH, 12 MEDIUM, 4 LOW). **No operator action.** **Next (SESSION-65):** prober untrusted-input cluster (the 2
> remaining HIGH — [3] MPD `io.LimitReader`, [4] printf-format) — see `sessions/SESSION-65.md`.

# SESSION-64 — planned at S63 close (D-125)

> Written by SESSION-63 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE + `agents/handoffs/S62-AUDIT-FINDINGS.md`** (the 25-finding ledger).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-65

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. **Each S62 finding is an AGENT finding —
re-verify against the code; take the verified CORE, not the audit's literal suggested scope** (the S48 arc + S63
proved this: NARROWER, BROADER, DEFER, or downgrade-severity). One coherent scope per PR.

## ★ Context — the S62 audit backlog (4 shipped, 21 remain)

SESSION-62 audited the un-swept subsystems → **25 CONFIRMED (6 HIGH, 15 MEDIUM, 4 LOW), 1 refuted** in
`S62-AUDIT-FINDINGS.md`. S63 shipped the alert-channels cluster ([1]/[2]/[10]/[11]). **21 remain: 4 HIGH, 13
MEDIUM, 4 LOW.** Work HIGH-first, in coherent clusters, one scope per PR, exactly as S49→S61 worked the S48 backlog.
**The ledger numbering is by severity (1-25); [3]-[6] are the remaining HIGH findings.**

## ★ SESSION-64 scope: reports_wave2 re-fetch nil-deref cluster ([5]+[6] HIGH, [19] MEDIUM)

Three findings, **one file** (`server/internal/api/reports_wave2.go`), **one root pattern** — the post-mutation
re-fetch that swallows the store error with `_` and then unconditionally dereferences a possibly-nil pointer, PLUS
the sibling `err != nil || row == nil` collapse that maps a transient store error to a 404. This mirrors the
**S40/D-102** re-fetch fix and the already-correct `handleUpdateProbe` guard (`wave3.go:429-430`) — that handler is
the in-repo reference for the shape of the fix.

- **[5] HIGH** `reports_wave2.go:345` — `handleUpdateTenant`: `updated, _ := s.store.GetTenant(...)` discards the
  error, then `tenantToAPI(*updated)` derefs nil when a concurrent DELETE (or transient DB error) makes GetTenant
  return `(nil, nil)`/`(nil, err)`. `scanTenant` (meta.go:1403-1412) returns `(nil, nil)` on `sql.ErrNoRows`. Chi's
  Recoverer turns the panic into a bare 500 **after the UPDATE already committed** — phantom failure of a succeeded
  mutation. **Fix:** `updated, err := …; if err != nil || updated == nil { writeError(500, INTERNAL_ERROR, "failed
  to fetch updated tenant"); return }`.
- **[6] HIGH** `reports_wave2.go:191` — `handleUpdateReportSchedule`: identical pattern with
  `GetReportSchedule`/`reportScheduleToAPI(*updated)`; `scanReportSchedule` (meta.go:1538-1539) returns `(nil, nil)`
  on `ErrNoRows`. **Fix:** same nil+err guard after line 191. **⚠ Consider the ledger's alt:** the create handler
  (line 141) avoids the re-fetch entirely by using the value `UpdateReportSchedule`/`CreateReportSchedule` already
  returns. **RE-VERIFY at open** whether `UpdateReportSchedule` returns the fully-populated updated row (incl.
  recomputed `next_run_at`); if so, dropping the re-fetch closes the race window AND removes a DB round-trip — the
  stronger fix. If the update does NOT return the row, use the guard. Take the verified CORE.
- **[19] MEDIUM** `reports_wave2.go:154` — transient store error collapsed into 404 in **three** handlers:
  `handleUpdateReportSchedule` (154), `handleGetTenant` (297), `handleUpdateTenant` (313). Each does
  `if err != nil || row == nil { writeError(404, NOT_FOUND, "… not found") }`, so a DB-down error is reported to
  clients as a definitive 404 — SDK/UI cache invalidators mark an existing resource permanently absent. **Fix:**
  split into two guards per handler — `if err != nil { writeError(500, INTERNAL_ERROR, …); return }` then
  `if row == nil { writeError(404, NOT_FOUND, …); return }`.

**Why bundle:** all three live in one file, share the store-error-swallowing anti-pattern, and are fixed by the same
two-guard discipline. One PR, one coherent scope. **Note [19] overlaps [5]/[6]'s edit sites** (lines 154/313/345 are
adjacent to the re-fetch guards) — do them together to avoid a second pass over the same lines.

### Plan
1. **Verify-at-open:** open `reports_wave2.go`; confirm lines 154/191/297/313/345 read as the ledger states; confirm
   `handleUpdateProbe` (wave3.go:429-430) is the correct-guard reference; check whether `UpdateReportSchedule`
   already returns the populated row (decides [6]'s guard-vs-drop-refetch approach).
2. **Fix:** apply the nil+err guards ([5], [6]) and the 500/404 split ([19]) across the three handlers.
3. **Test (mutation-proven, on a `/mut` copy):**
   - `[5]`: stub `GetTenant` → `(nil, nil)` after a successful `UpdateTenant`; assert **500 with JSON error body**,
     not panic/empty. Reddens on revert (unguarded deref panics).
   - `[6]`: stub `GetReportSchedule` → `(nil, nil)` after a successful update; assert **500**, not panic.
   - `[19]`: stub `GetTenant`/`GetReportSchedule` → `(nil, errors.New("db down"))`; assert **500 INTERNAL_ERROR**,
     not 404. Positive control: `(nil, nil)` (true ErrNoRows) still yields **404**.
   - Use the existing api-package test fakes/harness (trace one first — S49); do NOT introduce a new store mock if a
     table-driven store fake already exists.
4. **Full Go suite** in docker (24/24, `-buildvcs=false`; `-count=1` if contracts touched — they are not here).
5. **`gofmt -l .`** before push (CI gofmt gate).
6. **Review:** this is a correctness/robustness fix with no auth/contract/semantic-surface change and clean mutation
   proofs → a careful **self-review** is defensible (lesson 3). If the re-fetch-drop path for [6] changes response
   *shape* in any way, escalate to the adversarial workflow.
7. **PR → CI → squash-merge → verify `origin/main`.**
8. **Roll prod forward** (server *source* changed) — full deploy sequence + smoke.
9. **Docs at close:** D-126 in `decisions.md` (append EARLY); mark [5]/[6]/[19] ✅ in `S62-AUDIT-FINDINGS.md`;
   ROADMAP-V2 §2.31 (7 shipped / 18 remain); RESUME-PROMPT ▶ START HERE → SESSION-65; CHANGELOG Fixed;
   `operator-expected.md` (no operator action expected — internal robustness); SESSION-64 CLOSED; write SESSION-65.

## ★ Then (subsequent sessions), the other HIGH + coherent clusters

- **[3] HIGH** MPD prober unbounded read — `io.LimitReader` cap. **[4] HIGH** printf-format injection in prober.
  (prober untrusted-input cluster; possibly + RTMP CSID map cap MEDIUM.)
- **alert-evaluator cluster** — [7] D-088 presence guards, stream_offline compare bypass, license_expiry stuck-firing.
- **anomaly cluster** — hysteresis, `scopeJSON` escaping ([18] MEDIUM — JSON-escape the ID fields).
- **api cluster** — [21] SSRF probe-URL scheme/host validation; **[20] audit-log admin-scope gate — RE-VERIFY vs
  D-105 "reads-open" ruling FIRST; likely DEFER** (any authenticated user reading the audit log may be the same
  product ruling S43 made for other admin reads — do not "fix" a deliberate decision).

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S63 (D-125, PR #120 `5172150`) + the S63 close-docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-64-g5172150`** (S63 shipped code — alert-channels). `/healthz` all-ok. Signed-webhook
  smoke 200 (replay check default-off). Email STARTTLS now fail-closed (D-125) — expected.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If ≥ 07-23, bundle the `web-e2e`/`csp-e2e` promotion.
- Operator queue: GHCR 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1`.
- **★ RUN `gofmt -l .` BEFORE EVERY PUSH** — CI's `server` job has a gofmt gate the local `go build && go vet` misses (S54). (Memory: `ci-gofmt-gate`.)
- **Mutation-prove on a COPY**: `cp -a /repo/server /mut && cd /mut`; mutate; test there. `perl -0pi -e 's/\Q<literal>\E/<replacement>/'` handles metachars (S61). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check.** **Contract change? `cd web && npm run gen:api`** (types-drift guard; node 22, `npm ci --legacy-peer-deps`, S55). **New CH migration? lineage at 0010** (next = 0011; do NOT edit 0001 — S60). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: 5-overlay `DC_ARGS`; **rollback = a DOCKER image tag** `docker tag pulse-prod-pulse:latest pulse-prod-pulse:pre-dNNN` → backup (`exec -T backup … once`, rc 0) → STAMPED `build --build-arg VERSION=$(git describe --tags --always) …` (backgrounded, >2 min) → assert stamp ≠ dev → `up -d` (no `--build`) → smoke (healthz, version, **signed webhook 200**, limits 512M/0.5cpu, logs clean). Roll forward ONLY if server/web *source* changed. Admin token in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND mechanism before building; take the verified CORE — NARROWER, BROADER, DEFER (dead/
   vestigial or duplicate-of-a-ruling — S59/S60, and **[20] vs D-105**), SHIP-OPT-IN after a contract check (S61), or
   DOWNGRADE-SEVERITY when the exposure is narrower than claimed (S63/[11]). Trace an existing test before trusting
   it (S49). A named goal / audit claim can be stale or overstated (S37/S48) — re-verify.
2. Mutation-prove every guard; positive control so the harness can't be vacuous. Cover BOUNDARY conditions (S61) —
   here: the `(nil, nil)`-still-404 positive control for [19].
3. Independent review before merge: a genuine SEMANTIC/security/auth/contract change warrants the multi-lens
   adversarial workflow (S55/S61); a purely mechanical, mutation-proven robustness fix can take a careful
   self-review (this cluster qualifies for self-review unless [6]'s response shape changes).
4. Positive allowlists over blocklists (D-098). Respect documented contract/design even when an audit disagrees
   (S59/S60; **[20]**). Migrations forward-only (lineage 0010; never edit 0001).
5. No silent scope caps; persist verified findings to the ledger; state latency/impact honestly. Default-off /
   backward-compatible is the way to ship a security/contract change without breaking live traffic (S61).
6. **Run `gofmt -l` before every push** (S54).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-126** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-65; ROADMAP-V2 §2.31 ledger; mark [5]/[6]/[19] ✅ in `S62-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md` (no operator action expected for this cluster — internal robustness).
5. Write `sessions/SESSION-65.md`.
6. **Roll prod forward** (server source changed) + smoke.
