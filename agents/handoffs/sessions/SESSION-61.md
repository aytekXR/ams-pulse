# SESSION-61 — planned at S60 close (D-122)

> Written by SESSION-60 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE for the full ranked candidate list** + `S48-AUDIT-FINDINGS.md`.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-62

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. **Each S48 finding is an AGENT finding —
re-verify against the code; take the verified CORE, not the audit's literal suggested scope.** The tail proved this
repeatedly: S55 [10] BROADENED; S50/S51/S58 narrowed; **S59 [11] DEFERRED** (dead code, D-087); **S60 [12] DEFERRED**
(vestigial column, impact refuted, D-018 CR-VD38). One scope per PR.

## ★ Context — the LAST S48-audit finding ([8]); the rest are shipped or deferred

Shipped (13): [6] D-110, [1]+[2] D-111, [3] D-112, [4]+[15] D-113, [5] D-114, [7] D-115, [9] D-116, [10] D-117,
[13] D-118, [16] D-119, [14] D-120. **Deferred (2):** [11] D-121 (dead-code dup of D-087), [12] D-122 (vestigial
`rollup_usage_1d.peak_concurrency`, impact refuted — real peak comes from `rollup_concurrency_1d` per D-018 CR-VD38).
**1 remains:**

- **⚠ [8] MEDIUM (product/contract-gated)** `collector/webhook/webhook.go:160` webhook replay — `validateHMAC` proves
  the body was signed with the correct key but performs **no freshness check** (no timestamp window, no nonce), so
  any captured valid signed webhook can be replayed indefinitely → duplicate stream-start/end/recording-ready events
  injected into the analytics pipeline. Textbook fix: a new `X-Ams-Timestamp` header + a ±5-min window check, with
  the timestamp folded into the HMAC input so it can't be stripped/forged.

## ⛑ [8] is a CONTRACT change — VERIFY PRODUCT-VIABILITY FIRST (do NOT ship a half-measure)

The fix only works if the SENDER (AMS, or the deployed signing proxy in front of Pulse) actually emits a signed
timestamp. **Before writing any code:**

1. **Establish who signs the webhook today.** Read `docs/AMS-INTEGRATION.md` + the webhook handler
   (`collector/webhook/webhook.go`, esp. `validateHMAC` and `handleWebhookWithSecret`) + any signing-proxy config in
   `deploy/` (grep `X-Ams-Signature`, `PULSE_WEBHOOK_SECRET`, `webhook`, Caddy/nginx snippets). AMS's **native**
   webhook (`webhookCallbackURL`) posts a JSON body and, in this deployment, is HMAC-signed — determine WHERE the
   signature is applied (AMS itself vs. a proxy) and whether a timestamp header is available at that point.
2. **Decision gate:**
   - **If NO timestamp is sent** (most likely — AMS's native webhook has no timestamp header): a strict check would
     **401 every real webhook → live ingest breakage.** This is **operator/contract-gated.** Record it precisely in
     `docs/operator-expected.md` (what must change: the signing proxy must add+sign an `X-Ams-Timestamp`, or AMS must
     be configured to send one) and in the session log, and STOP on this finding — a legitimate human dependency per
     the standing directive. Do NOT ship a timestamp check that rejects production traffic.
   - **If a viable path exists** (e.g. the signing proxy is ours and can add+sign a timestamp): design it
     **backward-compatible and config-gated** — e.g. `PULSE_WEBHOOK_REQUIRE_TIMESTAMP` (default OFF) so existing
     deployments keep working until the proxy is updated; when ON, require + window-check + HMAC-bind the timestamp.
     Mutation-prove (replay a 10-min-old signed request → 401 when ON; valid fresh → 200; OFF → legacy behavior).
     Run the multi-lens adversarial workflow (security + contract lenses — this is a genuine auth-surface change).

**Most likely outcome: operator/contract-gated STOP.** That is a correct, directive-compliant result — not a failure.

## ★ After [8]: the S48 backlog is fully triaged — pick the next move

Once [8] is shipped or operator-gated, **13 shipped + 2 deferred + 1 shipped-or-gated = all 16 findings triaged.**
Re-read the standing directive + ROADMAP-V2 §2 and choose the next-highest-leverage move. Likely candidates:
- **A FRESH adversarial audit** of an un-swept subsystem (as S48 itself was — S44 audited handlers, S48 audited
  collector/amsclient/reports/cluster/clickhouse; candidates not yet deep-audited: `server/internal/api` handler
  families not in S44, `alert/evaluator`, `license`, `probe/prober`, the web SPA data layer, the SDK).
- **§2.7 CI-promotion win** IF today ≥ **2026-07-23** — promote `web-e2e`/`csp-e2e` off `continue-on-error` (both
  green through the bake). **CHECK THE DATE at open.**
- Operator-gated items stay operator-gated (GHCR, AMS licence, item 10, S43 rulings) — do not spin on them.

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S60 (D-122, PR #NNN — docs-only) should be on `origin/main`.
- Prod should print **`v0.4.0-57-g36c16ed`** (UNCHANGED — S59 + S60 were both no-code/no-DDL deferrals). `/healthz`
  all-ok, `ams_env_configured:true`. **[8] rolls prod forward ONLY if it ships as a code change** (not if gated).
- Operator queue: GHCR anon → 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.**

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1`.
- **★ RUN `gofmt -l .` BEFORE EVERY PUSH** — CI's `server` job has a gofmt gate the local `go build && go vet` misses (S54). Gate: `sh -c 'D=$(gofmt -l .); [ -z "$D" ] && go build ./... && go vet ./... && go test ./... || { echo DIRTY: $D; exit 1; }'`. (Memory: `ci-gofmt-gate`.)
- **Mutation-prove on a COPY** mounted read-only at `/repo`: `cp -a /repo/server /mut/server && cp -a /repo/contracts /mut/contracts`; mutate `/mut`; test there. **Target precisely**; replacement ending in `{` breaks perl `{}` → use `#`. Prefer compiling mutations (RED test). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check.** **Contract change? `cd web && npm run gen:api`** — CI's `web` job has a **types-drift guard**; regenerate with `npm ci --legacy-peer-deps` (node 22) so `schema.d.ts` matches CI byte-for-byte (S55). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: 5-overlay `DC_ARGS`; **rollback point is a DOCKER image tag** `docker tag pulse-prod-pulse:latest pulse-prod-pulse:pre-dNNN` → backup (`exec -T backup … once`, rc 0) → STAMPED `build --build-arg VERSION=$(git describe --tags --always) …` (backgrounded, >2 min) → assert stamp ≠ dev → `up -d` (no `--build`) → smoke (healthz, version, **signed webhook 200** — [8] touches this path, so smoke it carefully, limits 512M/0.5cpu, logs clean). Roll forward ONLY if server/web *source* changed. Admin token in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND mechanism before building; take the verified CORE — NARROWER, BROADER, or **DEFER**
   (S59 [11] dead code; S60 [12] impact refuted). **[8] is the archetype: verify the SENDER contract before coding —
   a signed-timestamp check is worthless (and breaks ingest) if nothing sends the timestamp.**
2. Mutation-prove every guard; positive control so the harness can't be vacuous. A config-gated auth change ([8])
   must prove BOTH states (gate OFF = legacy 200; gate ON = replay 401, fresh 200).
3. Independent review before merge; a genuine auth-surface/contract change ([8]) warrants the multi-lens adversarial
   workflow (security + contract), not a self-review.
4. Positive allowlists over blocklists (D-098). Respect documented contract/design even when an audit disagrees
   (S59/S60). Migrations are forward-only (never edit 0001; lineage is at 0010).
5. No silent scope caps; persist verified findings to the ledger; state latency/impact honestly. An
   operator/contract dependency is recorded in `operator-expected.md` + the log, not worked around with a half-measure.
6. **Run `gofmt -l` before every push** (S54).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.** (Docs-only if [8] is gated.)
2. `decisions.md` **D-123** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-62; ROADMAP-V2 §2.30 ledger (mark [8] shipped ✅ OR gated); mark in `S48-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md` (esp. if [8] is operator/contract-gated — spell out exactly what must change).
5. Write `sessions/SESSION-62.md` (with the next-highest-leverage move — likely a fresh audit or the CI-promotion win).
6. **Roll prod forward** ONLY if [8] shipped as a server-source change.
