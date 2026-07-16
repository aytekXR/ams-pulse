# SESSION-68 — planned at S67 close (D-129)

> ## ✅ CLOSED (2026-07-16, D-130) — SHIPPED [21], DEFERRED [20]
> **[21] probe-URL SSRF** shipped (PR #130, prod `v0.4.0-74-g2621c03`): new `internal/ssrfguard` — scheme allowlist +
> dial-time `net.Dialer.Control` guard on the resolved IP across HTTP/RTMP/WS paths, denying link-local/metadata/NAT64/
> unspecified while intentionally ALLOWING loopback + RFC-1918 (B4/A6 ruling — narrower than the ledger's literal fix).
> 11/11 mutants killed; suite 25/25; 5-lens adversarial review caught + fixed 4 defects pre-merge (2 MAJOR: proxy
> bypass, NAT64 prefix). **[20] audit-log admin gate DEFERRED** as the deliberate reads-open product ruling (D-105/S43)
> → escalated to the operator as an adjudicated product call (operator-expected.md). See `decisions.md` D-130 and
> `sessions/SESSION-69.md` for the next scope. Everything below is the original pre-session plan (historical).


> Written by SESSION-67 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE + `agents/handoffs/S62-AUDIT-FINDINGS.md`** (the 25-finding ledger).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-69

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. **Each S62 finding is an AGENT finding —
re-verify against the code; take the verified CORE, not the audit's literal suggested scope** (S66 declined the
ledger's off-by-one + a NIT; S67's review overturned two first-pass implementations — a bare-`continue` stuck-firing
regression and a `math.MaxFloat64` value overflow). Do NOT stop after one session — at close, update all docs,
regenerate this plan for the next session, record progress + operator-needs, and continue until the roadmap is complete
or a human/operator is genuinely required. **Ultracode is on** — use the adversarial-review workflow on anything with
security or state-machine/semantics surface; token cost is not a constraint.

## Goal — the two SECURITY MEDIUMs: [20] audit-log admin gate + [21] probe-URL SSRF

Security-relevant findings before the remaining correctness edge-cases. `S62-AUDIT-FINDINGS.md` after S67: **12 remain
(0 HIGH, 8 MEDIUM, 4 LOW).** This session takes the two with real confidentiality / SSRF surface. They are in DIFFERENT
subsystems (api/authz vs prober) — treat as a "security" theme, and if either balloons, split it into its own PR.

### ⚠ [20] MEDIUM — `GET /admin/audit-log` readable by a viewer-scoped token (missing admin gate) — **RE-VERIFY FIRST, likely DEFER or product call**
- **loc:** `server/internal/api/` audit-log route + its auth middleware. `S62-AUDIT-FINDINGS.md` [20] (line ~332).
- **DO THIS FIRST:** re-verify against the code AND against **D-105** (which shipped the audit-log UI ledger) and the
  **S43 "reads-open" ruling** (operator soft-decision that authenticated reads are intentionally open). Trace the route
  registration (grep `audit-log` / `audit_log` in `server.go`), the scope/role middleware on it, and what D-105 added.
  Three outcomes:
  1. **D-105 already gates it by admin scope** → mark [20] ✅ DONE-BY-D-105 (DEFER, documented), no code.
  2. **Intentionally open per S43 "reads-open"** → this is a PRODUCT CALL, not a bug. Record in `operator-expected.md`
     and surface to the operator; do NOT unilaterally tighten (it could break the operator's own tooling).
  3. **Genuinely an unintended gap** (audit log leaks tenant/security events to a low-priv viewer token, contradicting
     S43's own scope model) → then it's a real fix: add the admin-scope gate, mutation-prove (viewer token → 403,
     admin token → 200), adversarial-review the authz change.
- **Bias:** the ledger itself flagged this "likely DEFER"; two prior operator-expected headers list it as "pending
  re-check against S43 before any change." Resolve the ambiguity decisively this session — either close it (DEFER with
  reason) or escalate it as a product call — rather than leaving it perpetually pending.

### [21] MEDIUM — probe URL accepted without scheme/host validation → stored-SSRF
- **loc:** `server/internal/prober/` (probe creation/validation) + the probe-create API handler. `S62-AUDIT-FINDINGS.md`
  [21] (line ~340).
- **Verify at open:** what validation (if any) exists today on a stored probe URL, on BOTH the API create/update path
  (`handleCreateProbe`/`handleUpdateProbe`) and the prober's own fetch path. The concern: an operator-stored URL with
  no scheme/host allowlist can be pointed at `http://169.254.169.254/…` (cloud metadata), `http://localhost:…`,
  link-local / RFC-1918 internal addresses, or a non-http scheme → the prober makes the request from inside the trust
  boundary (SSRF), and probe results/latency can exfiltrate internal-service responses.
- **Fix sketch (take verified CORE):** validate scheme ∈ {http,https} and reject/deny-list internal hosts
  (loopback, link-local `169.254.0.0/16` incl. metadata `169.254.169.254`, RFC-1918, `::1`, `fc00::/7`, `.internal`,
  bare IPs if policy requires) at the API boundary AND defensively at fetch time (DNS-rebinding: resolve + re-check, or
  use a hardened dialer). Check whether the prober already has a `SafeDial`/allowlist hook to extend. **Consider:** is
  there an existing outbound-allowlist config (`PULSE_PROBE_ALLOW…`)? Mirror the pattern; don't invent a new one.
- **Untrusted-outbound → strong adversarial-review candidate** (finder lenses: bypass via redirect / DNS-rebind /
  IPv6-mapped / octal-IP / userinfo-in-authority / non-http scheme; refute-by-default verifiers).

## Pipeline (unchanged — the S64→S67 loop)

1. **Verify-at-open:** `git log --oneline -3`, confirm HEAD == origin/main (concurrent-session check), `git status`
   shows ONLY `deploy/config/Caddyfile.prod` dirty (do-not-commit, D-082/D-096 — a clean status is a FAILURE signal).
   Record OPEN facts in `decisions.md` (append EARLY, newest-at-bottom) as **D-130 IN PROGRESS**. Branch `s68-d130`.
   **CHECK THE DATE** — §2.7 CI-promotion gate unlocks ≥ 2026-07-23.
2. **Re-verify each finding vs the actual code** via `codegraph_explore` (take verified CORE — NARROWER/BROADER/DEFER;
   [20] especially may DEFER or escalate). Trace existing tests first (S49).
3. **Fix → mutation-prove** on a throwaway `/mut` copy (`cp -a /repo/server /mut/server && cp -a /repo/contracts
   /mut/contracts`, run from `/mut/server` — the api harness needs `contracts/` for `metaDDLPath` or it SKIPs).
4. **Full Go suite 24/24** in docker; **`gofmt -l`** on changed files (CI gofmt gate — run before EVERY push).
5. **Adversarial review workflow** (security surface → mandatory): finder lenses → refute-by-default verifiers,
   schema-validated. Fix anything CONFIRMED before merge (S65/S66/S67 each had review-found in-scope issues).
6. **PR → CI poll** (bounded background monitor) → **squash-merge --delete-branch** → verify origin/main.
7. **Roll prod forward ONLY if `server/`|`web/` source changed** (the deploy sequence: `config -q` → tag
   `pulse-prod-pulse:pre-d130` → backup rc0 → STAMPED build backgrounded → assert stamp ≠ dev → `up -d pulse` no
   --build → 5-check smoke). A validation-only or authz change still ships server source → roll + smoke.
8. **Close docs:** `decisions.md` D-130 SHIPPED, CHANGELOG, `S62-AUDIT-FINDINGS.md` marks, ROADMAP §2.31 count,
   RESUME-PROMPT rotation, `operator-expected.md` (top block; **[20] may add a real operator/product item** — flag it),
   SESSION-68 CLOSED, SESSION-69 written. Re-arm the `/loop` (ScheduleWakeup, `<<autonomous-loop-dynamic>>`).

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`
  (24 pkgs). `gofmt`/`go` are NOT on the host PATH.
- **Mutation on `/mut`:** for api handler tests, copy BOTH `/repo/server`→`/mut/server` AND `/repo/contracts`→
  `/mut/contracts`, run from `/mut/server` (else `metaDDLPath`/`readMetaDDL` SKIPs — `../../../contracts/db/meta/...`).
  For a mutant that would leave a var unused, prefer a value-forcing mutation (e.g. flip a source field) over
  `if false` so the mutant compiles and reddens BEHAVIORALLY.
- **Admin token** (side-effect-free GET only, never commit): gitignored `oguz-testing.md`. Prod API base
  `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`. Alert-rules route mounted at
  `/api/v1/...`. Useful for verifying prod state before/after a behavior change (S67 verified prod's stream_offline
  rules this way).
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). Commit trailer `Co-Authored-By: Claude Opus 4.8 (1M context)
  <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
