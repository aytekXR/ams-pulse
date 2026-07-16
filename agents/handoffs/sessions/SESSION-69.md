# SESSION-69 — planned at S68 close (D-130)

> Written by SESSION-68 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE + `agents/handoffs/S62-AUDIT-FINDINGS.md`** (the 25-finding ledger).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-70

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. **Each S62 finding is an AGENT finding —
re-verify against the code; take the verified CORE, not the audit's literal suggested scope** (S66 declined the
ledger's off-by-one + a NIT; S67's review overturned two first-pass implementations; **S68 narrowed [21] from "deny all
RFC-1918" to "deny link-local/metadata, allow private" per the B4/A6 ruling, and DEFERRED [20] as a product call**). Do
NOT stop after one session — at close, update all docs, regenerate this plan for the next session, record progress +
operator-needs, and continue until the roadmap is complete or a human/operator is genuinely required. **Ultracode is
on** — use the adversarial-review workflow on anything with security or state-machine/semantics surface (it caught two
MAJOR metadata-reachable SSRF bypasses in S68 that a green test suite missed); token cost is not a constraint.

## Goal — continue the S62 backlog (10 remain: 0 HIGH, 6 MEDIUM, 4 LOW)

`S62-AUDIT-FINDINGS.md` after S68: **14 shipped + [20] deferred-by-ruling (15 resolved); 10 remain — MEDIUM
[12]/[14]/[15]/[16]/[17]/[18] + LOW [22]/[23]/[24]/[25] (verified against the ledger's status markers).** Re-read ROADMAP §2.31 and the ledger, then pick the highest-leverage cluster. Candidates (re-verify each at
open — the ledger's line refs may have drifted, and the disposition may be NARROWER/BROADER/DEFER):

### Lead candidate — prober HLS correctness [14] + [15] (coherent, bounded, subsystem just swept)
- **[14] MEDIUM — HLS zero-duration `#EXTINF` → divide-by-zero / bogus bitrate.** `server/internal/prober/prober.go`
  probeHLS bitrate computation. Verify: what happens when a segment's EXTINF duration is 0 or missing? `bitrate = bytes*8
  / durationS` → Inf/NaN or panic? Fix sketch: guard duration <= 0 → emit an honest error_code (mirror the existing
  `segment_too_large` honest-failure pattern), don't fabricate a bitrate.
- **[15] MEDIUM — protocol-relative segment URI (`//host/seg.ts`) mis-resolved.** probeHLS segment-URI resolution.
  Verify how a `//host/…` or absolute-path segment URI is joined against the manifest base; a protocol-relative URI
  should inherit the manifest scheme, not be treated as a path. **NOTE:** any fix that makes probeHLS fetch a
  new host now flows through the S68 `ssrfguard` dial guard automatically — good, but confirm the resolved URL is still
  built correctly.
- Same file/subsystem as S66/S68 → one coherent PR. Untrusted-server-input parsing → adversarial-review candidate.

### Alternative A — anomaly flags cluster [16] + [17] + [18]
- **[16]/[17]** anomaly-flag hysteresis (flag flapping at the sigma boundary) — state-machine surface, re-verify the
  flag-transition logic; **[18]** scopeJSON escaping in the anomaly flag store. Coherent if the lead is deferred.

### Alternative B — licence cluster [12] + [23] + [24]
- Licence tier/error-handling findings. Re-verify against `server/internal/license/`.

### Remaining LOW — [22] cert_expiry `lt 0` never-fires, [25]
- **[22]** (already detailed in the ledger, lines ~362): `CertChecker.DaysUntilExpiry` returns 0 (not -1) for an
  already-expired cert, so a `cert_expiry lt 0` rule never fires. Small, self-contained. Good low-risk filler.

## Pipeline (unchanged — the S64→S68 loop)

1. **Verify-at-open:** `git log --oneline -3`, confirm HEAD == origin/main (concurrent-session check), `git status`
   shows ONLY `deploy/config/Caddyfile.prod` dirty (do-not-commit, D-082/D-096 — a clean status is a FAILURE signal).
   Record OPEN facts in `decisions.md` (append EARLY, newest-at-bottom) as **D-131 IN PROGRESS**. Branch `s69-d131`.
   **CHECK THE DATE** — §2.7 CI-promotion gate unlocks ≥ 2026-07-23 (today 2026-07-16 → still locked).
2. **Re-verify each finding vs the actual code** via `mcp__codegraph__codegraph_explore` (take verified CORE —
   NARROWER/BROADER/DEFER). Trace existing tests first (S49).
3. **Fix → mutation-prove** on a throwaway `/mut` copy (`cp -a /repo/server /mut/server && cp -a /repo/contracts
   /mut/contracts`, run from `/mut/server` — the api harness needs `contracts/` for `metaDDLPath` or it SKIPs). For a
   mutant that would leave a var unused, prefer a value-forcing mutation over `if false`, and run mutation with
   `go test -vet=off` so an intentionally-unreachable mutant still compiles (S68 pattern).
4. **Full Go suite** in docker; **`gofmt -l`** on changed files (CI gofmt gate — run before EVERY push). If a change
   touches `contracts/openapi/pulse-api.yaml`, ALSO regen `web/src/lib/api/schema.d.ts` via `cd web && npm run gen:api`
   and commit it — the `web` CI job runs an openapi-typescript drift check (`git diff --exit-code`) that failed S68's
   first CI run. Validate the spec with `npx @redocly/cli lint --skip-rule=path-parameters-defined`.
5. **Adversarial review workflow** (security / untrusted-input / state-machine surface → mandatory): finder lenses →
   refute-by-default verifiers, schema-validated. Fix anything CONFIRMED before merge (S65/S66/S67/S68 each had
   review-found in-scope issues — S68 had TWO MAJOR).
6. **PR → CI poll** (bounded background monitor) → **squash-merge --delete-branch** → verify origin/main.
7. **Roll prod forward ONLY if `server/`|`web/` source changed** (deploy sequence: `config -q` → tag
   `pulse-prod-pulse:pre-d131` → backup rc0 → STAMPED build backgrounded → assert stamp ≠ dev → `up -d pulse` no
   --build → 5-check smoke).
8. **Close docs:** `decisions.md` D-131 SHIPPED, CHANGELOG, `S62-AUDIT-FINDINGS.md` marks, ROADMAP §2.31 count,
   RESUME-PROMPT rotation, `operator-expected.md` (top block), SESSION-69 CLOSED, SESSION-70 written. Re-arm the
   `/loop` (ScheduleWakeup, `<<autonomous-loop-dynamic>>`).

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`
  (25 pkgs now — `internal/ssrfguard` was added in S68). `gofmt`/`go` are NOT on the host PATH.
- **ssrfguard (new, S68/D-130):** `server/internal/ssrfguard` is the shared SSRF policy (scheme allowlist + resolved-IP
  dial guard). Any new outbound-fetch path should route through it; deny link-local/metadata/NAT64/unspecified, ALLOW
  loopback + RFC-1918. Node available at `/home/aytek/.local/bin/node` (npx) for web codegen / redocly.
- **Admin token** (side-effect-free GET only, never commit): gitignored `oguz-testing.md`. Prod API base
  `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). Commit trailer `Co-Authored-By: Claude Opus 4.8 (1M context)
  <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
