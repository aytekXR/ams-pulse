# SESSION-73 — planned at S72 close (D-134) — ★ first POST-S62-AUDIT session

> ## ✅ CLOSED (2026-07-17, D-135) — OPENED the third subsystem audit → 8 findings
> Ran the third fresh adversarial audit over the still-un-swept subsystems (`store/meta`, `query`, `config`,
> `cmd/pulse`, `web/` — deduplicated against S48 + S62). 5 finder lenses (high effort) + refute-by-default verifiers,
> 17 agents → **8 CONFIRMED (3 HIGH, 5 MEDIUM), 4 refuted.** HIGHs: [1] `IngestTimeseries` cross-tenant leak (same
> class as S48/D-110), [2] `server.Stop()` doesn't drain the HTTP server on SIGTERM, [3] `PULSE_ANONYMIZE_IP=1`
> silently leaves IPs un-anonymized. Ledger `agents/handoffs/S73-AUDIT-FINDINGS.md`; tracker ROADMAP §2.32; decision
> D-135. **No operator action.** Next: work the clusters — SESSION-74 leads with the `cmd/pulse` config-startup cluster
> [2]+[3]+[6]. Everything below is the original pre-session plan (historical).


> Written by SESSION-72 close (2026-07-17). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE first.** The S62 audit (§2.31) is COMPLETE — the 25-finding
> backlog is empty. This session opens a new arc.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-74

Re-read ROADMAP-V2 §2 / `docs/assessment/` §5 and **choose the next-highest-leverage move.** Verify candidate status
AND product-viability against the code before committing. Take the verified CORE. Do NOT stop after one session — at
close, update all docs, regenerate this plan, record progress + operator-needs, continue until the roadmap is complete
or a human/operator is genuinely required. **Ultracode is on** — use the adversarial-review workflow on
security/state-machine/semantics surface; token cost is not a constraint. **Workflow-script gotcha (S72):** never put
backticks in workflow prompt prose — they close the JS template literal and break parsing. `gofmt -l` before every push.

## Where things stand (post-audit)

Two adversarial subsystem audits are now complete: **§2.30 (S48→S61, 16 findings, 14 shipped)** covered one cluster;
**§2.31 (S62→S72, 25 findings, 24 shipped + [20] deferred)** covered alert/evaluator+channels, license, prober, anomaly,
and api handlers. Prod is at **v0.4.0-82-g8355127**, all HIGH+MEDIUM security findings resolved.

**Remaining roadmap items are mostly gated or operator-directed** (verify each at open):
- **§2.7 CI job promotions** — date-gated, unlocks ≥ 2026-07-23 (today 2026-07-17 → still locked).
- **§2.1 branch protection** `enforce_admins=true` — operator-gated (needs a 2nd approver or review-count 0).
- **§2.6 unsigned-webhook ingest mode** — DECISION-first (operator).
- **§2.18 item 6, GHCR-public flip, licence ceremony** — operator-gated (see operator-expected.md).
- **§2.15 brand adoption phase 2** (light theme / density / motion) and **§2.19 full UI/UX refactor via uipro** —
  OPERATOR-DIRECTED design work; best with operator input on direction. Actionable but not clearly autonomous.
- **§2.12 Mobile SDKs** — [L per platform], large.

## Lead — a THIRD fresh subsystem adversarial audit of the still-UN-swept subsystems

The two prior audits found 41 findings (many HIGH/security) in the subsystems they swept. The **un-swept** subsystems
have never had a dedicated adversarial pass and are the highest-leverage autonomous move:
- **collector pipeline:** `collector/kafka`, `collector/beacon`, `collector/webhook`, `collector/restpoller`,
  `collector/ingest`, `collector/sessions`, `collector/aggregator` (ingest hardening, backpressure, parsing of
  untrusted AMS/beacon input, concurrency).
- **storage:** `store/clickhouse` (+ migrations), `store/meta` (SQLite/PG) — batching, retries, TTL, injection, tx.
- **query / reports:** `query`, `reports` (accounting, schedules, PDF) — query construction, decimal/rounding, cron.
- **config / cluster / amsclient:** `config`, `cluster` (discovery/eviction), `pkg/amsclient` (AMS wire parsing, auth).
- (stretch) **web/** frontend — a separate lens (CSP, auth-gate, API-schema drift, XSS) if backend is thin.

**Pipeline:** run the S62-style audit workflow (multiple finder lenses over the subsystem clusters → refute-by-default
verifiers, schema-validated) → produce a NEW confirmed-findings ledger `S73-AUDIT-FINDINGS.md` (+ a ROADMAP §2.32
tracker header) → then work the findings in coherent clusters exactly like S62 (one cluster per session: verify-at-open →
re-verify vs code / take CORE → fix → mutation-prove on `/tmp/pulsemut` → full suite → adversarial review →
PR → CI → merge → roll prod if server/web source → 5-check smoke → close docs → re-arm loop). Scale finder count to
subsystem size; dedup vs the two prior ledgers so already-fixed issues aren't re-filed.

**Alternative (if the operator prefers):** pick up the operator-directed UI work (§2.15 phase 2 or §2.19) — but that
benefits from operator design direction, so surface it as a choice rather than starting unilaterally.

## Pipeline for the audit-open (this session)

1. **Verify-at-open:** `git log --oneline -3`, HEAD == origin/main, `git status` shows ONLY `deploy/config/Caddyfile.prod`
   dirty. Record **D-135 IN PROGRESS** in `decisions.md` (append EARLY). Branch `s73-audit` (or per-cluster later).
   **CHECK THE DATE** (§2.7 gate ≥ 2026-07-23).
2. **Scope + dedup:** list the un-swept packages, confirm against the §2.30 + §2.31 ledgers what was already covered,
   and skip re-auditing swept code. Decide finder lenses (by-subsystem + by-vulnerability-class).
3. **Run the audit workflow** (finders READ actual files; NO backticks in JS prose). Schema-validated findings →
   refute-by-default verifiers → write `agents/handoffs/S73-AUDIT-FINDINGS.md` with per-finding loc/mechanism/scenario/
   mutation-test/fix-sketch/verifier-reasoning (mirror the S62 ledger format) + a ROADMAP §2.32 tracker.
4. **Close docs** for the audit-open (the ledger IS the deliverable): decisions.md D-135, ROADMAP §2.32, RESUME-PROMPT
   ▶ START HERE → SESSION-74 (first fix cluster), operator-expected (no new action expected), SESSION-73 CLOSED. Re-arm loop.
5. Subsequent sessions work the clusters (S62 loop).

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`
  (25 pkgs). `gofmt`/`go` NOT on host PATH. Node at `/home/aytek/.local/bin/node` (npx) for web/redocly.
- **Mutation copy:** `/mut` at root is NOT writable — use `/tmp/pulsemut` (`cp -a /repo/server /tmp/pulsemut/server`;
  the `preprocessed_configs`/`access` artifact dirs error on cp but are non-fatal). `go test -vet=off` for unreachable mutants.
- **CodeQL** flags `InsecureSkipVerify` in PRODUCTION code only (`go/disabled-certificate-check`, HIGH — fails the
  required "CodeQL" check); test files are exempt (S72/D-134). Prefer keeping security controls ON.
- **Prod deploy is LOCAL** (this host IS prod): 5-overlay compose `DC_ARGS="-p pulse-prod -f deploy/docker-compose.yml
  -f deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. Prod at **v0.4.0-82-g8355127**.
- **Admin token** (side-effect-free GET only, never commit): gitignored `oguz-testing.md`. Prod API base
  `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). Commit trailer `Co-Authored-By: Claude Opus 4.8 (1M context)
  <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
