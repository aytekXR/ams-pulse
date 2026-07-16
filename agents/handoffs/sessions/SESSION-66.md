# SESSION-66 — planned at S65 close (D-127)

> Written by SESSION-65 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE + `agents/handoffs/S62-AUDIT-FINDINGS.md`** (the 25-finding ledger).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-67

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. **Each S62 finding is an AGENT finding —
re-verify against the code; take the verified CORE, not the audit's literal suggested scope** (S65's adversarial
review even found a sink the finding didn't name — the review is part of "verify"). One coherent scope per PR.

## ★ Context — the S62 audit backlog (9 shipped, 16 remain — ALL HIGH done)

SESSION-62 audited the un-swept subsystems → **25 CONFIRMED (6 HIGH, 15 MEDIUM, 4 LOW), 1 refuted**. Shipped:
S63 alert-channels ([1]/[2]/[10]/[11]); S64 reports_wave2 ([5]/[6]/[19]); S65 prober DASH ([3]/[4]). **★ All 6 HIGH
are now shipped.** **16 remain: 0 HIGH, 12 MEDIUM, 4 LOW.** Continue in coherent clusters, one scope per PR.

## ★ SESSION-66 scope: prober RTMP DoS ([13] MEDIUM, `probe_rtmp.go`)

Completes the prober subsystem's untrusted-input hardening (same "hostile probed server → OOM" threat model as the
S65 DASH work). One finding, one file (`server/internal/prober/probe_rtmp.go`), with a coupled off-by-one:

- **[13] MEDIUM `probe_rtmp.go:437`** — `readAMF0Command` allocates a new `*rtmpCSIDState` + map entry for every
  unseen CSID (`:438-440`). The RTMP 3-byte-form basic header (`csidRaw==1`) admits **65,536 unique CSID values**
  (64..65,599), and each state accumulates up to 65,536 bytes of payload → **~4.3 GB heap** within the probe deadline.
  There's no cap on the CSID-state count or aggregate buffer bytes. A malicious server that passes the RTMP handshake
  then streams interleaved chunks across all 65,536 CSIDs OOM-kills the prober (aborting monitoring for ALL probes).
  **Fix:** a package-level `maxCSIDStates` (e.g. 256 — real RTMP uses a handful of chunk streams) checked immediately
  before the new-state allocation at `:438`:
  `if len(states) >= maxCSIDStates { return "", fmt.Errorf("rtmp: too many CSID streams (%d)", len(states)) }`.
- **Coupled off-by-one `probe_rtmp.go:506`** — the oversized-message guard is `st.length > rtmpMaxMsgSize` (strict
  `>`, `rtmpMaxMsgSize = 65536`), so a message declaring **exactly** 65,536 bytes is NOT rejected and its payload
  accumulates. **Fix:** change to `st.length >= rtmpMaxMsgSize`. **Re-verify at open** that `rtmpMaxMsgSize` is
  intended as an exclusive bound (i.e. 65,536 should be rejected) and that `>=` doesn't wrongly reject a legitimate
  max-size message — trace how `st.length` is set and what a real AMS RTMP message max is.

### Plan
1. **Verify-at-open:** open `probe_rtmp.go`; confirm `:437-440` (new-state alloc, unbounded `states` map), the CSID
   value space (`:432` `csid = 64 + x[0] + 256*x[1]`), and `:506` (`>` guard). Trace `states` lifetime + `st.length`
   set site. Confirm `maxCSIDStates=256` is safely above any real RTMP usage (check for existing constants/tests).
2. **Fix:** add the `maxCSIDStates` cap at the new-state branch; change `>` → `>=` at the message-size guard.
3. **Test (mutation-proven, on a `/mut` copy — prober tests use httptest, no DDL, so `cp -a /repo/server /mut`
   suffices):**
   - Cap: simulate 257 unique 3-byte-form CSID chunks → assert an `rtmp`-class error, not silent state growth.
     Mutation: revert the cap → the map grows past 256 (test asserts the error, reddens on revert).
   - Off-by-one: a message declaring exactly `rtmpMaxMsgSize` bytes → assert it is rejected. Mutation: revert `>=`→`>`
     → the exact-size message is accepted (test reddens). Positive control: a message just UNDER the cap is accepted.
   - **Reuse the existing RTMP probe test harness** (trace it first — S49). If the AMF0/chunk demuxer is only
     reachable via a full fake RTMP server, consider a focused unit test on `readAMF0Command` with a synthetic byte
     stream (mirror how S65 unit-tested `expandSegmentTemplate` directly).
4. **Full Go suite** in docker (24/24, `-buildvcs=false`). `prober` package is slow (~27s).
5. **`gofmt -l .`** before push (CI gofmt gate).
6. **Review:** untrusted-input DoS hardening on the prober's network-facing RTMP demuxer — a security surface. Lean
   toward the **multi-lens adversarial review workflow** (S65 proved its value — it found a sibling sink). At minimum,
   have the review check for OTHER unbounded allocations in the RTMP path (aggregate buffer bytes, handshake buffers).
7. **PR → CI → squash-merge → verify `origin/main`.**
8. **Roll prod forward** (server *source* changed) — full deploy sequence + smoke.
9. **Docs at close:** D-128 in `decisions.md` (append EARLY); mark [13] ✅ in `S62-AUDIT-FINDINGS.md`; ROADMAP-V2
   §2.31 (10 shipped / 15 remain — recount MED/LOW); RESUME-PROMPT ▶ START HERE → SESSION-67; CHANGELOG Security;
   `operator-expected.md` (no operator action); SESSION-66 CLOSED; write SESSION-67.

## ★ Then (subsequent sessions), the remaining MEDIUM/LOW clusters

- **alert-evaluator** — [7] `evalNodeMetric` missing D-088 presence guards (false threshold alerts on AMS 3.x nodes),
  stream_offline compare bypass, license_expiry stuck-firing.
- **anomaly** — [18] `scopeJSON` builds JSON by raw concatenation without escaping the ID fields (wrong stream
  attribution for IDs with `"`); hysteresis.
- **license** / **prober-core** / **api** — [21] SSRF probe-URL scheme/host validation; **[20] audit-log admin-scope
  gate — RE-VERIFY vs D-105 "reads-open" ruling FIRST; likely DEFER** (any authenticated user reading the audit log
  may be the same product ruling S43 made — do NOT "fix" a deliberate decision).

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S65 (D-127, PR #124 `2a122fd`) + the S65 close-docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-68-g2a122fd`** (S65 shipped code — prober DASH). `/healthz` all-ok. Signed-webhook smoke
  200 (replay check default-off). Email STARTTLS fail-closed (D-125) — expected.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If ≥ 07-23, bundle the `web-e2e`/`csp-e2e` promotion.
- Operator queue: GHCR 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1`.
- **★ RUN `gofmt -l .` BEFORE EVERY PUSH** — CI's `server` job has a gofmt gate the local `go build && go vet` misses (S54). (Memory: `ci-gofmt-gate`.)
- **Mutation-prove on a COPY**: `cp -a /repo/server /mut && cd /mut`; mutate; test there. **If the test uses the DDL-backed HTTP harness (`setupEnterpriseServer`), also copy `contracts` — `cp -a /repo/server /mutrepo/server && cp -a /repo/contracts /mutrepo/contracts`, run from `/mutrepo/server` — else `metaDDLPath` SKIPs (S64).** Prober tests use httptest only (no DDL). `perl -0pi -e 's/\Q<lit>\E/<repl>/'` handles metachars; for `$`-heavy Go, put the perl in a mounted `.pl` file so bash doesn't interpolate `$` (S65). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check.** **Contract change? `cd web && npm run gen:api`** (types-drift guard; node 22, `npm ci --legacy-peer-deps`, S55). **New CH migration? lineage at 0010** (next = 0011; do NOT edit 0001 — S60). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: 5-overlay `DC_ARGS`; **rollback = a DOCKER image tag** `docker tag pulse-prod-pulse:latest pulse-prod-pulse:pre-dNNN` → backup (`exec -T backup … once`, rc 0) → STAMPED `build --build-arg VERSION=$(git describe --tags --always) …` (backgrounded, >2 min) → assert stamp ≠ dev → `up -d pulse` (no `--build`) → smoke (healthz, version, **signed webhook 200** via `X-Ams-Signature` HMAC, limits 512M/0.5cpu, logs clean). Roll forward ONLY if server/web *source* changed. Admin token in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND mechanism before building; take the verified CORE — NARROWER, BROADER (S64 dropped a
   re-fetch; S65's review widened scope to a 3rd sink), DEFER (dead/vestigial or duplicate-of-a-ruling — S59/S60, and
   **[20] vs D-105**), SHIP-OPT-IN after a contract check (S61), or DOWNGRADE-SEVERITY (S63/[11]). Trace an existing
   test before trusting it (S49). An audit claim can be stale/overstated/INCOMPLETE (S65) — re-verify.
2. Mutation-prove every guard; positive control so the harness can't be vacuous. Cover BOUNDARY conditions (S61; S66's
   off-by-one is exactly a boundary — test the exact-size message). When a concrete dep gives no injection seam, find
   a deterministic trigger (S64: pre-canceled ctx) or unit-test the pure function directly (S65: expandSegmentTemplate).
3. Independent review before merge: a genuine SEMANTIC/security/auth/contract change warrants the multi-lens
   adversarial workflow — **S65 proved it finds real sinks the finding didn't name** (untrusted-input DoS qualifies);
   a purely mechanical, mutation-proven robustness fix can take a careful self-review (S64).
4. Positive allowlists over blocklists (D-098; S65's printf allowlist). Respect documented contract/design even when
   an audit disagrees (S59/S60; **[20]**). Migrations forward-only (lineage 0010; never edit 0001).
5. No silent scope caps; persist verified findings to the ledger; state latency/impact honestly (S65 documented the
   LOW manifest-cap tradeoff rather than over-claiming). Default-off / backward-compatible ships safely (S61).
6. **Run `gofmt -l` before every push** (S54).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-128** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-67; ROADMAP-V2 §2.31 ledger; mark [13] ✅ in `S62-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md` (no operator action expected for this cluster — internal hardening).
5. Write `sessions/SESSION-67.md`.
6. **Roll prod forward** (server source changed) + smoke.
