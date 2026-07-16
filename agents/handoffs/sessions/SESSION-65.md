> ## ✅ SESSION-65 CLOSED (2026-07-16, D-127) — PR #124 merged `2a122fd`, prod `v0.4.0-68-g2a122fd` (smoke green)
> **Shipped:** prober DASH untrusted-input cluster ([3]+[4], the last 2 S62 HIGH) — [3] MPD manifest body now
> `io.LimitReader`-capped (16 MiB) before xml.Decode (segment body was already capped; manifest was the gap); [4]
> `$Number%<spec>$` printf format positive-allowlisted (`^%0?\d{0,3}d$`) so a hostile `%999999999d` degrades to plain
> decimal. **A 4-lens adversarial review (10 agents, refute-by-default) found a sibling sink my fix missed — now also
> fixed:** `$RepresentationID$` `strings.ReplaceAll` was unbounded (count×len(id) → TB-scale within the 16 MiB body
> cap), now bounded by `maxExpandedTemplateBytes` (64 KiB). Review also widened the width bound 2→3 digits (spec-legal
> %100d) and documented the LOW large-archive-manifest tradeoff honestly; 1 finding refuted correctly. New
> `probe_dash_s65_test.go` (mutation-proven ×4). Full suite 24/24; gofmt + vet clean. **Ledger:** [3]/[4] ✅ DONE;
> **★ ALL 6 S62 HIGH now shipped; 16 remain (0 HIGH, 12 MEDIUM, 4 LOW).** **No operator action.** **Next (SESSION-66):**
> prober RTMP DoS ([13] MEDIUM — unbounded CSID state map + off-by-one guard, `probe_rtmp.go`) — see `sessions/SESSION-66.md`.
> **★ Lesson reinforced:** the adversarial workflow earned its cost — the RepresentationID sink would have left the
> prober OOM-able despite the [3]/[4] fixes. Security-surface changes get the workflow, not self-review.

# SESSION-65 — planned at S64 close (D-126)

> Written by SESSION-64 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE + `agents/handoffs/S62-AUDIT-FINDINGS.md`** (the 25-finding ledger).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-66

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. **Each S62 finding is an AGENT finding —
re-verify against the code; take the verified CORE, not the audit's literal suggested scope** (the S48 arc + S63/S64
proved this: NARROWER, BROADER, DEFER, or downgrade-severity). One coherent scope per PR.

## ★ Context — the S62 audit backlog (7 shipped, 18 remain)

SESSION-62 audited the un-swept subsystems → **25 CONFIRMED (6 HIGH, 15 MEDIUM, 4 LOW), 1 refuted** in
`S62-AUDIT-FINDINGS.md`. Shipped so far: S63 alert-channels ([1]/[2]/[10]/[11]); S64 reports_wave2 re-fetch
([5]/[6]/[19]). **18 remain: 2 HIGH, 12 MEDIUM, 4 LOW.** Work HIGH-first, in coherent clusters, one scope per PR.
**The two remaining HIGH ([3],[4]) are both in the DASH prober** — this session's scope.

## ★ SESSION-65 scope: prober untrusted-input cluster ([3]+[4] HIGH, both in `probe_dash.go`)

Both HIGH findings are in the **same file, same probe path** (`server/internal/prober/probe_dash.go`), and share one
threat model: **a malicious/hostile probed server returns a crafted MPD manifest that drives the prober into a
gigabyte-scale allocation → OOM.** A single probe against one hostile endpoint suffices; no concurrency needed. The
in-repo model for the fix already exists in the same package (the segment-body `io.LimitReader` cap).

- **[3] HIGH `probe_dash.go:151`** — the MPD manifest body is passed to `parseMPD`/`xml.NewDecoder` **unbounded**
  (`parseMPD(resp.Body, p.URL)`), so a manifest with millions of `<SegmentURL>` elements materialises a 2–4 GB tree
  in one `DecodeElement` call. The HTTP client (`prober.go:147-151`) has no transport body limit. **Note the
  asymmetry:** the *segment* body at `:196` is already capped with `io.LimitReader(segResp.Body, segBodyCapBytes+1)`
  — the manifest fetch is the one gap. **Fix:** define `maxMPDBodyBytes` (e.g. `4 << 20`, matching typical CDN
  manifest ceilings — re-verify a sane value) and wrap: `parseMPD(io.LimitReader(resp.Body, maxMPDBodyBytes), p.URL)`.
- **[4] HIGH `probe_dash.go:372`** — `expandSegmentTemplate` extracts a printf format from the server's MPD
  `SegmentTemplate media` attribute via `reNumberFmt` (`\$Number%[^$]+\$`) and passes it **verbatim** to
  `fmt.Sprintf(spec, number)`. A width like `%999999999d` makes `fmt.Sprintf` pad to ~1 GB. **Fix:** don't feed an
  attacker string as the format — either validate the width is small (bounded digits), or reconstruct a safe format
  (`%0<N>d` with a capped N) instead of trusting the captured spec. Re-verify the exact sink + the regex capture.

**Why bundle:** one file, one probe path, one threat model (hostile-manifest resource exhaustion), and the fixes are
independent small guards — clean single PR. **Verify-at-open:** confirm `:151`/`:196`/`:372` read as the ledger
states; confirm `segBodyCapBytes` is the reference pattern; check whether any existing prober test serves a synthetic
hostile manifest (reuse the harness — S49).

### Plan
1. **Verify-at-open:** open `probe_dash.go`; confirm the unbounded `parseMPD(resp.Body, …)` at :151, the capped
   segment read at :196 (reference), and the `fmt.Sprintf(spec, number)` sink at :372 + the `reNumberFmt` capture.
2. **Fix:** add the manifest `io.LimitReader` cap ([3]); make the segment-template expansion reject/neutralise an
   oversized or attacker-controlled width ([4]). Positive allowlist over blocklist where possible (D-098).
3. **Test (mutation-proven, on a `/mut` copy — copy `server` + `contracts` so any DDL-backed harness doesn't SKIP):**
   - `[3]`: an `httptest.Server` serves a valid-XML-prefix-then-garbage body larger than `maxMPDBodyBytes`; assert a
     `parse MPD` error (truncated). Non-vacuity: without the LimitReader the same body decodes/OOMs. Also assert a
     normal small manifest still probes OK (positive control).
   - `[4]`: a `SegmentTemplate media` with `%999999999d` (or similar) must NOT trigger a giant allocation — assert an
     error / safe bounded output. Mutation: revert the guard → the crafted width expands hugely (bound the test so it
     fails fast, not OOMs the CI runner — e.g. assert on the returned error/URI, not on wall-clock).
4. **Full Go suite** in docker (24/24, `-buildvcs=false`). `prober` package is one of the slow ones (~27s).
5. **`gofmt -l .`** before push (CI gofmt gate).
6. **Review:** untrusted-input / DoS-hardening on the prober's network-facing parser — a genuine security surface.
   Lean toward the **multi-lens adversarial review workflow** (lesson 3) rather than self-review, since [4] in
   particular is a format-string sink and easy to get subtly wrong.
7. **PR → CI → squash-merge → verify `origin/main`.**
8. **Roll prod forward** (server *source* changed) — full deploy sequence + smoke.
9. **Docs at close:** D-127 in `decisions.md` (append EARLY); mark [3]/[4] ✅ in `S62-AUDIT-FINDINGS.md`; ROADMAP-V2
   §2.31 (9 shipped / 16 remain — recount HIGH/MED/LOW); RESUME-PROMPT ▶ START HERE → SESSION-66; CHANGELOG Security;
   `operator-expected.md` (no operator action expected — internal hardening); SESSION-65 CLOSED; write SESSION-66.

## ★ Then (subsequent sessions), the remaining clusters

- **prober RTMP DoS** — [13] MEDIUM `probe_rtmp.go:437` unbounded CSID state map (65,536 × 64 KB = 4 GB) + the
  off-by-one `>` vs `>=` guard at :506. Same "hostile probed server → OOM" theme; may pair with any residual prober
  work.
- **alert-evaluator** — [7] D-088 presence guards (false threshold alerts on AMS 3.x), stream_offline compare bypass,
  license_expiry stuck-firing.
- **anomaly** — hysteresis, `scopeJSON` escaping ([18] MEDIUM — JSON-escape the ID fields).
- **api** — [21] SSRF probe-URL scheme/host validation; **[20] audit-log admin-scope gate — RE-VERIFY vs D-105
  "reads-open" ruling FIRST; likely DEFER** (any authenticated user reading the audit log may be the same product
  ruling S43 made for other admin reads — do not "fix" a deliberate decision).

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S64 (D-126, PR #122 `fede961`) + the S64 close-docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-66-gfede961`** (S64 shipped code — reports_wave2). `/healthz` all-ok. Signed-webhook
  smoke 200 (replay check default-off). Email STARTTLS fail-closed (D-125) — expected.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If ≥ 07-23, bundle the `web-e2e`/`csp-e2e` promotion.
- Operator queue: GHCR 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1`.
- **★ RUN `gofmt -l .` BEFORE EVERY PUSH** — CI's `server` job has a gofmt gate the local `go build && go vet` misses (S54). (Memory: `ci-gofmt-gate`.)
- **Mutation-prove on a COPY**: `cp -a /repo/server /mut && cd /mut`; mutate; test there. **If the test uses the DDL-backed HTTP harness (`setupEnterpriseServer`), also `cp -a /repo/contracts /mutrepo/contracts` and run from `/mutrepo/server` — else `metaDDLPath` skips the test (S64 gotcha).** `perl -0pi -e 's/\Q<literal>\E/<replacement>/'` handles metachars (S61). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check.** **Contract change? `cd web && npm run gen:api`** (types-drift guard; node 22, `npm ci --legacy-peer-deps`, S55). **New CH migration? lineage at 0010** (next = 0011; do NOT edit 0001 — S60). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: 5-overlay `DC_ARGS`; **rollback = a DOCKER image tag** `docker tag pulse-prod-pulse:latest pulse-prod-pulse:pre-dNNN` → backup (`exec -T backup … once`, rc 0) → STAMPED `build --build-arg VERSION=$(git describe --tags --always) …` (backgrounded, >2 min) → assert stamp ≠ dev → `up -d pulse` (no `--build`) → smoke (healthz, version, **signed webhook 200** via `X-Ams-Signature` HMAC, limits 512M/0.5cpu, logs clean). Roll forward ONLY if server/web *source* changed. Admin token in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND mechanism before building; take the verified CORE — NARROWER, BROADER (S64 dropped a
   redundant re-fetch instead of only guarding it), DEFER (dead/vestigial or duplicate-of-a-ruling — S59/S60, and
   **[20] vs D-105**), SHIP-OPT-IN after a contract check (S61), or DOWNGRADE-SEVERITY when the exposure is narrower
   than claimed (S63/[11]). Trace an existing test before trusting it (S49). An audit claim can be stale or overstated
   (S37/S48) — re-verify.
2. Mutation-prove every guard; positive control so the harness can't be vacuous. Cover BOUNDARY conditions (S61).
   When a concrete store/dep gives no injection seam, find a deterministic trigger (S64: a pre-canceled ctx makes
   `database/sql` return `ctx.Err()`); if a branch is genuinely unreachable in-test, say so + match the in-repo
   precedent, don't fake a vacuous test.
3. Independent review before merge: a genuine SEMANTIC/security/auth/contract change warrants the multi-lens
   adversarial workflow (S55/S61 — untrusted-input parser hardening qualifies); a purely mechanical, mutation-proven
   robustness fix can take a careful self-review (S64).
4. Positive allowlists over blocklists (D-098). Respect documented contract/design even when an audit disagrees
   (S59/S60; **[20]**). Migrations forward-only (lineage 0010; never edit 0001).
5. No silent scope caps; persist verified findings to the ledger; state latency/impact honestly. Default-off /
   backward-compatible is the way to ship a security/contract change without breaking live traffic (S61).
6. **Run `gofmt -l` before every push** (S54).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-127** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-66; ROADMAP-V2 §2.31 ledger; mark [3]/[4] ✅ in `S62-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md` (no operator action expected for this cluster — internal hardening).
5. Write `sessions/SESSION-66.md`.
6. **Roll prod forward** (server source changed) + smoke.
