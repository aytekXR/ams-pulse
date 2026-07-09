# SESSION-06 — Docs + Helm GA batch (+ promotion decisions)

> Written by SESSION-05 on 2026-07-09 per ROADMAP §6. Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP.md`
> (plan of record, §3.S6) + `RESUME-PROMPT.md` §7/§8 (TDD + verification, binding) before dispatching.

## Mission

Exit = ROADMAP S6 (G7): **nothing in docs lies to an operator** — fix every P0-stale doc, write
the missing runbooks/SECURITY/CHANGELOG, bring Helm to parity-or-explicitly-experimental.
**PLUS the two promotion clocks: web-e2e and csp-e2e streaks both end ~2026-07-23** — if this
session runs on/after that date and the streaks held, promote BOTH into required contexts
(full-list PUT — enumerate ALL existing contexts + the new ones; a partial list silently
de-requires the rest). CodeQL promotion is NOT automatic — first-week bake only ended if ≥1 week
green; otherwise defer to S7.

## Preconditions (re-verify cheaply — fix this prompt if stale, note drift in decisions.md)

- `git log --oneline -3` shows the D-062 docs commit (or later); tree clean; ci AND e2e AND
  codeql green on main (`gh run list --branch main -L 6`). codeql went green from `5dacb7d`
  (go/autobuild + js-ts/none — the first run proved Go rejects build-mode none).
- Prod runs `v0.1.0-25-gbc15d43` (D-062): healthz ok, alert delivery live-proven. Rollback tags
  `pulse-prod-pulse:pre-d061` + `:pre-d058`. WATCH: intermittent CH "Memory limit (total)
  exceeded 1.80 GiB" on server_events inserts (seen once pre-D-062-swap; if it recurs, a CH
  memory-limit tune or insert batching review becomes a work order).
- Standing numbers post-S5 (D-062, verified 2026-07-09 full `-race`): Go total **73.2%** (floor
  **70.0**); webhook 94.7, query 86.9, alert 73.8, api 76.0, meta 66.9, cmd/pulse 42.3;
  logtail no longer exists. Web 76/72/45 + guard; SDK 62/73/70. Only 2 skips (domain npx).
- **Concurrent-session hazard (D-062/O11):** the operator may run a second Claude session. If
  HEAD moves or the tree dirties mid-session with foreign work, STOP and inspect before any
  commit/push (a foreign unpushed commit once carried a hardcoded live Slack webhook). Check
  whether O11 (webhook rotation + other session's local reset) happened; re-surface if not.
- CodeGraph in use: scouts/authors query it BEFORE grep (`codegraph explore/node/callers`);
  closing runs `codegraph sync` + `status`.
- Binding env unchanged: Go ONLY in Docker golang:1.25 repo-root mount + pulse-gomod/pulse-gobuild
  volumes, `sg docker -c`; compose stacks ONLY from a pristine copy; commit by explicit path;
  agents author, ORCH commits.

## Work orders (one Workflow: disjoint-scope authors → adversarial verify → ORCH gate/commit)

> Docs are TDD-inverted: every doc claim an operator could act on must be VERIFIED against the
> live tree/stack by the author (command + output in the report) — a doc "fix" that itself lies
> is the failure mode this session exists to kill (D-057 audit found the current P0s).

### WO-1 — productionize.md P0s + deploy-docs truth pass · scope `docs/` + `deploy/runbooks/` · [L]
- productionize.md: 5-overlay reality (quick-ref + step 1e + upgrade section — base+hardened+
  prod-tls+real-ams+backup, D-054), secrets `_FILE` section (D-052; note PULSE_LICENSE_KEY has NO
  _FILE support — os.Getenv, D-062), stamped-build procedure (build w/ build-args THEN up -d
  WITHOUT --build — D-058 lesson b; the runbook §3-D still shows the stale `--build` pattern, fix
  it), AMS_UPSTREAM env var (D-062).
- real-ams-go-live.md: mark §1-3 historical (test.antmedia.io / ams-integration era), point §4/§5
  at the current 5-overlay commands; SESSION-05 found §8/§14 references that never existed.
### WO-2 — alerting.md + AMS-INTEGRATION.md honesty · scope `docs/` · [M]
- alerting.md: prune cap 1000 (D-054), retry/delivery_failure semantics (D-049/D-061 A4),
  registry sync-on-tick (D-061 — channels take effect within one 5s tick, no restart),
  HONEST rebuffer_ratio/error_rate (D-062: rollup_qoe_1h via QoEReader; needs beacon data →
  Pro+ license; nil-reader/no-CH behavior = rule skipped + WARN), per-channel config keys
  (webhook_url/email_to+smtp_addr/slack_webhook_url/telegram_*/pagerduty_* — factory.go is
  the source of truth).
- AMS-INTEGRATION.md §4.5: B7 per-source webhook URLs (`/webhook/ams/{source_name}` + per-source
  secret via SourceWrite.webhook_secret, stored encrypted, webhook_secret_set read flag,
  STARTUP-ONLY load — rotation needs restart, cross-source isolation semantics; legacy
  /webhook/ams + global secret unchanged).
### WO-3 — new docs: upgrade/rollback + monitoring runbooks, SECURITY.md, CHANGELOG.md · scope new files · [L]
- upgrade/rollback runbook: 5-overlay commands, stamped-build, rollback tags (pre-dNNN
  convention), CH DDL rollback stance (migrations frozen), never `down -v` on pulse-data.
- monitoring runbook: backup daemon (cycle cadence, keep-7), alert_history growth (cap 1000),
  disk, collector_errors_total, the CH memory-pressure WATCH (D-062), log WARN taxonomy.
- SECURITY.md: report channel, HMAC webhook auth (global + B7), token HMAC-SHA256 (D-052),
  secrets _FILE, CSP, fail-closed postures. CHANGELOG.md: backfill from decisions.md D-0NN
  (v0.1.0 era → now), Keep-a-Changelog format.
- LICENSE: still O5 (operator picks) — draft only if the operator has chosen.
### WO-4 — Helm parity batch · scope `deploy/helm/` · [M]
- Image ref (ghcr canonical + digest default), CH auth, webhook port/Service (incl. B7 note),
  backup CronJob, `optional: false` secret, NOTES.txt; regenerate ALL 3 goldens (red golden diff
  first, then commit regenerated); install.md Path C stays EXPERIMENTAL unless a cluster exists
  (D-002 waiver stands).
### WO-5 — promotion decisions (date-gated) · scope `.github/` + branch protection API · [S]
- If ≥2026-07-23 AND streaks held (job-level green per run: `gh api .../runs/<id>/jobs`):
  promote web-e2e + csp-e2e (drop continue-on-error on csp-e2e, PUT the FULL contexts list).
  If run before the date: explicitly record "not due yet" in decisions.md.
- CodeQL: record bake status; promote only if ≥1 week green AND the operator agrees (it's a
  linter-class gate, not a test).

## Gates (ORCH, before any commit)
- Docs: every operator-actionable claim verified (command transcript in the verifier report);
  link-check the touched docs (no dead anchors/files).
- Helm: `helm lint` + 3 golden diffs green on alpine/helm:3.17.0; if goldens change, the diff is
  the reviewed artifact.
- Full `-race` repo-root only if any Go file was touched (docs sessions usually don't) — but the
  suite MUST still be green at close (run once as the closing gate; floor 70).
- No secrets in diffs; commit per scope; push; `gh run watch` ci AND e2e AND codeql green.

## Closing protocol (ROADMAP §6)
1. Commits per scope; push; watch ci + e2e + codeql to green.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-063 (per-WO evidence; promotion decisions recorded either way).
3. RESUME-PROMPT ▶ START HERE → SESSION-07; ROADMAP §3.S6 ✅ + §4 ledger + §5 (O-items re-checked:
   O1/U3, O2/U5, O3, O4, O5, O7, O8, O11).
4. Write `sessions/SESSION-07.md` from ROADMAP §3.S7 (GA gate: re-run the 9-scout audit, diff
   against G1–G8, punch list or GA declaration; load/perf probe vs the 500-stream mock).
