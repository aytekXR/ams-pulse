# SESSION-79 — planned at S78 close (D-140) — the LAST S73 finding ([5]); closes the third audit

> Written by SESSION-78 close (2026-07-17). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE + `agents/handoffs/S73-AUDIT-FINDINGS.md`** (8-finding ledger; 7 shipped, ALL HIGH done).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-80

Re-verify each finding against the code before building — **take the verified CORE**, and for [5] specifically ADJUDICATE
scope (the finder's fix sketch was already flagged WRONG). Do NOT stop after one session — at close, update all docs,
regenerate this plan, record progress + operator-needs, continue until the roadmap is complete or a human/operator is
genuinely required. **Ultracode is on.** **Workflow-script gotcha:** no backticks in workflow prompt prose. Web gotchas:
`vi.hoisted` for vi.mock; `ApiError(status,{code,message})`; full `npm test` for coverage; binary embeds `web/dist`.

## Goal — [5] MEDIUM: `QoEForStream` cross-tenant QoE (the LAST S73 finding). ★ Closing it completes §2.32.

`S73-AUDIT-FINDINGS.md`: 1 MEDIUM remains ([5]). This is the WIDER one — **re-verify AND re-scope at open.**

### [5] MEDIUM — `QoEForStream` omits tenant → the alert evaluator reads cross-tenant QoE
- **loc:** `server/internal/query/query.go:898` (QoEForStream) → `QoeSummary` :794; caller `server/internal/alert/wave2.go:93`.
- **The problem is WIDER than the finder claimed** (already flagged in the ledger): `QoEForStream(streamID, app)` builds
  `QoeParams` with an empty `Tenant`, so `QoeSummary`'s `AND tenant = ?` is skipped and `rollup_qoe_1h` aggregates across
  every tenant sharing an (app, stream_id). BUT the alert evaluator (`wave2.go:93`) has NO tenant to pass —
  `domain.AlertScope` / `meta.AlertRuleRow` / `domain.LiveStream` have NO `Tenant` field. So the real fix must first make
  the tenant available at the evaluation point.
- **Re-verify + ADJUDICATE at open (this is the key decision):**
  1. Does the live pipeline know the tenant per stream? Trace: does the aggregator / `domain.LiveStream` / the source
     carry tenant anywhere (grep `Tenant` in `domain/`, `collector/aggregator`, `LiveSnapshot`)? Multi-tenancy is a
     Business+ tier feature (F6) — how is a stream associated with a tenant today (via the source? the beacon? the AMS
     node)? If the aggregator has no per-stream tenant, threading it is a real data-model change.
  2. **Impact:** the verifier downgraded high→medium because the PRIMARY self-hosted single-AMS-cluster model uses ONE
     tenant (or the empty default), so contamination does NOT occur in practice — only a multi-tenant deployment where
     distinct tenants share overlapping (app, stream_id) is affected.
  3. **Decide the CORE:** (a) FULL FIX — thread `Tenant` from the source/aggregator → `domain.LiveStream` → `AlertScope`,
     then add a `tenant` param to `QoEForStream` + the `QoEReader` interface, then `QoeSummary`'s filter fires. Complete
     but wide (multi-file: domain, aggregator, alert). (b) NARROWER — if a stream's tenant is derivable at the eval point
     from existing data (e.g. the source→tenant map), pass it without a full struct change. (c) DEFER-BY-RULING (like
     [20]) — if the full thread is disproportionate for a multi-tenant-only edge and the primary model is unaffected,
     document the limitation + the fix path and escalate as an adjudicated product call, closing §2.32 with 24 shipped +
     [20] + [5] dispositioned. **Prefer (a) if the live pipeline already carries (or trivially can carry) tenant; else
     weigh (c) honestly** — do NOT force a wide, risky change for a low-real-world-impact edge without clear value.
- **If building:** mutation-prove the tenant reaches `QoeParams` at the eval path (mirror the S48/D-137 tenant tests);
  adversarial-review (tenant-isolation surface). **Adversarial-review mandatory** whichever path (correctness OR the
  adjudication rationale).

## After [5] — §2.32 COMPLETE → re-survey the roadmap
Once [5] is dispositioned, the third audit is done (8/8). Flip ROADMAP §2.32 to ✅ COMPLETE. Then **re-read ROADMAP-V2 §2
/ `docs/assessment/` §5** and pick the next arc. Note the landscape (from S73 open): §2.7 CI-promotion gate unlocks
**≥ 2026-07-23** (check the date — if unlocked, that's a candidate); the rest is largely operator-gated (§2.1 branch
protection, §2.6 unsigned-webhook, §2.18 item 6 / GHCR / licence ceremony) or operator-directed UI (§2.15 phase 2 /
§2.19). A FOURTH audit is possible but the un-swept surface is now thin (three audits done: S44, S48, S62, S73). Consider
whether the highest-leverage move is now operator-gated → surface to the operator, OR a smaller hardening/polish pass.

## Pipeline (the S62→S78 loop)
1. **Verify-at-open:** git state clean (only Caddyfile). Record **D-141 IN PROGRESS** in `decisions.md`. Branch `s79-d141`.
   **CHECK THE DATE** (§2.7 gate ≥ 2026-07-23).
2. **Re-verify + adjudicate [5] scope** (`mcp__codegraph__codegraph_explore` the live pipeline for tenant).
3. **Fix → mutation-prove** (Go: `/tmp/pulsemut`; if a struct/interface change, run the full 25-pkg suite for fallout).
   If OpenAPI/contract changes, regen `web/src/lib/api/schema.d.ts`.
4. **Full Go suite** + web if touched.
5. **Adversarial review (mandatory).**
6. **PR → CI poll** → **squash-merge --delete-branch** → verify origin/main.
7. **Roll prod forward** (if server/web source): the standard sequence + 5-check smoke.
8. **Close docs:** D-141 SHIPPED (or DEFER-BY-RULING), CHANGELOG, ledger [5] mark, **ROADMAP §2.32 → ✅ COMPLETE**, RESUME
   rotation (→ SESSION-80, first post-S73 arc), `operator-expected.md`, SESSION-79 CLOSED, SESSION-80 written. Re-arm `/loop`.

## Environment gotchas (carried)
- **Go only in docker** (25 pkgs); mutation copy `/tmp/pulsemut` (not `/mut`); copy `contracts` if the meta/api harness
  reads the DDL. **CodeQL** flags `InsecureSkipVerify` in PRODUCTION Go only. Any OpenAPI change → regen schema.d.ts
  (committed) + a param-conformance registry entry for a new documented param.
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC_ARGS="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. Prod at **v0.4.0-93-g8858b5f**. 5-check smoke: startup
  version stamp, healthz 200, signed webhook 200 (HMAC from PULSE_WEBHOOK_SECRET), limits 512M/0.5cpu, 0 error lines.
- **Admin token** (side-effect-free GET only, never commit): gitignored `oguz-testing.md`. Prod API base
  `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). Commit trailer `Co-Authored-By: Claude Opus 4.8 (1M context)
  <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
