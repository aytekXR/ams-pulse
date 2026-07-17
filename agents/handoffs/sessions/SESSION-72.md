# SESSION-72 — planned at S71 close (D-133)

> ## ✅ CLOSED (2026-07-17, D-134) — SHIPPED [22] + [25] — ★ S62 AUDIT COMPLETE
> The final two S62 LOW findings shipped (PR #138, prod `v0.4.0-82-g8355127`): [22] `cert_expiry lt 0` now fires for an
> expired cert — the adversarial review caught that the audit's literal 1-line fix was DEAD CODE in production (an
> expired cert fails the verifying handshake, so the expiry branch was never reached), so the real fix detects the
> `x509.Expired` verification error and returns -1 (keeping TLS verification ON — no `InsecureSkipVerify` in production,
> which had tripped CodeQL); [25] the WebRTC stats-hold uses `time.NewTimer`+`defer Stop` instead of a leaked
> `time.After`. 3/3 mutants killed; suite 25/25; two adversarial-review passes (pass 1 found the [22] dead-code gap;
> re-review clean). **This completes the entire 25-finding S62 audit** (24 shipped + [20] deferred). **No operator
> action.** See `decisions.md` D-134 and `sessions/SESSION-73.md` for the next arc (a third fresh subsystem audit of the
> un-swept collector/store/query/config subsystems). Everything below is the original pre-session plan (historical).


> Written by SESSION-71 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE + `agents/handoffs/S62-AUDIT-FINDINGS.md`** (the 25-finding ledger).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-73

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. **Each S62 finding is an AGENT finding —
re-verify against the code; take the verified CORE, not the audit's literal suggested scope** (S66 declined an
off-by-one; S67 overturned two impls; S68 narrowed [21] + DEFERRED [20]; S69 review caught a classify regression; S70
review caught a `WarmHysteresis` off-by-one + an alert scope-key mirror divergence; S71 review was clean). Do NOT stop
after one session — at close, update all docs, regenerate this plan, record progress + operator-needs, and continue
until the roadmap is complete or a human/operator is genuinely required. **Ultracode is on** — use the adversarial-review
workflow on security or state-machine/semantics surface; token cost is not a constraint.

## Goal — FINISH the S62 audit: the last 2 LOW ([22] cert_expiry, [25] webrtc timer)

`S62-AUDIT-FINDINGS.md` after S71: **22 shipped + [20] deferred (23 resolved); 2 remain — LOW [22] + [25].** Closing both
completes the entire second subsystem audit (25 findings). They are unrelated subsystems, so either one close-out PR or
one each. Re-verify each at open (line refs may have drifted).

### [22] LOW — `CertChecker.DaysUntilExpiry` returns 0 for an already-expired cert → `cert_expiry lt 0` never fires
- **loc:** see ledger (~line 407). The checker returns 0 (not a negative value) once a cert is past expiry, so an alert
  rule `{metric: cert_expiry, operator: lt, threshold: 0}` can never fire (0 < 0 is false).
- **Re-verify:** find `DaysUntilExpiry` (likely `server/internal/alert/` or a checks/cert helper) and the alert-rule
  evaluation path for `cert_expiry`. Decide the fix: return a negative day count for an expired cert (so `lt 0` fires),
  OR clamp semantics in the eval path. Prefer the option that keeps the metric's meaning honest (negative = expired-by-N-days).
  Verify no other rule/threshold relies on the current floor-at-0 behavior.
- **Test:** a cert expired N days ago → `DaysUntilExpiry` returns a negative value; a `cert_expiry lt 0` rule fires.

### [25] LOW — `continueWebRTCICE` leaks a `time.After(hold)` timer on ctx cancellation during the stats hold
- **loc:** see ledger (~line 448), `server/internal/prober` (WebRTC probe stats hold). `time.After(hold)` allocates a
  timer that is not stopped if the context is cancelled first, leaking the underlying runtime timer until it fires.
- **Re-verify + fix:** replace `time.After(hold)` with `t := time.NewTimer(hold); defer t.Stop()` and select on
  `t.C` vs `ctx.Done()`. Confirm no behavior change on the normal (non-cancelled) path. WebRTC/state surface →
  adversarial-review candidate.
- **Test:** cancel the ctx during the hold and assert prompt return + no lingering timer (or a mutation that drops the
  `Stop()` is caught by a leak/timing assertion).

### After the audit closes
The S62 backlog is then **empty** (25/25 dispositioned: 23 shipped + [20] deferred + [22]/[25] this session — adjust the
tally to 24 shipped + 1 deferred at close). Re-read `ROADMAP-V2.md §2` and `docs/assessment/` §5 and pick the next
highest-leverage roadmap item (or surface to the operator if what remains is genuinely operator-gated — e.g. the vendor
key ceremony, GHCR publish, §2.7 CI-promotion which unlocks ≥ 2026-07-23).

## Pipeline (unchanged — the S64→S71 loop)

1. **Verify-at-open:** `git log --oneline -3`, HEAD == origin/main, `git status` shows ONLY `deploy/config/Caddyfile.prod`
   dirty (do-not-commit, D-082/D-096). Record **D-134 IN PROGRESS** in `decisions.md` (append EARLY). Branch `s72-d134`.
   **CHECK THE DATE** — §2.7 gate unlocks ≥ 2026-07-23.
2. **Re-verify each finding vs the code** via `mcp__codegraph__codegraph_explore` (take verified CORE). Trace existing
   tests first (cert-checker tests in `alert/`; prober WebRTC tests).
3. **Fix → mutation-prove** on a throwaway copy. NOTE: `/mut` at root is NOT writable this environment — use
   `/tmp/pulsemut` (`rm -rf /tmp/pulsemut && mkdir -p /tmp/pulsemut && cp -a /repo/server /tmp/pulsemut/server`; the
   `preprocessed_configs`/`access` artifact dirs error on cp but are non-fatal). `go test -vet=off` for unreachable mutants.
4. **Full Go suite** (25 pkgs) in docker; **`gofmt -l`** on changed files (CI gofmt gate — before EVERY push).
5. **Adversarial review workflow** ([25] WebRTC/state surface → recommended; [22] small → self-review may suffice):
   finder lenses → refute-by-default verifiers. Fix CONFIRMED before merge. (When writing the workflow script, keep
   BACKTICKS out of JS template-literal prose — they break the parser; use plain quotes. S72-noted gotcha.)
6. **PR → CI poll** (bounded background monitor; occasional `csp-e2e`/`e2e` Playwright flake — re-run the failed job) →
   **squash-merge --delete-branch** → verify origin/main.
7. **Roll prod forward** (only if `server/`|`web/` source changed — both [22]/[25] are server): `config -q` → tag
   `pulse-prod-pulse:pre-d134` → backup rc0 → STAMPED build backgrounded → assert stamp ≠ dev → `up -d pulse` no --build
   → 5-check smoke.
8. **Close docs:** `decisions.md` D-134 SHIPPED, CHANGELOG, `S62-AUDIT-FINDINGS.md` marks (+ **audit COMPLETE** note),
   ROADMAP §2.31 count (flip ⏳ IN PROGRESS → ✅ COMPLETE), RESUME-PROMPT rotation, `operator-expected.md` (carry [20]),
   SESSION-72 CLOSED, SESSION-73 written (first post-audit roadmap pick). Re-arm the `/loop`.

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`
  (25 pkgs). `gofmt`/`go` NOT on host PATH. Node at `/home/aytek/.local/bin/node` (npx) for web/redocly.
- **Prod deploy is LOCAL** (this host IS prod): 5-overlay compose, `DC_ARGS="-p pulse-prod -f deploy/docker-compose.yml
  -f deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. Prod now at **`v0.4.0-80-gc477660`**. 5-check smoke:
  version stamp from startup log, healthz 200, signed webhook 200 (HMAC from `PULSE_WEBHOOK_SECRET`), limits
  512M/0.5cpu, 0 error lines.
- **Admin token** (side-effect-free GET only, never commit): gitignored `oguz-testing.md`. Prod API base
  `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). Commit trailer `Co-Authored-By: Claude Opus 4.8 (1M context)
  <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
