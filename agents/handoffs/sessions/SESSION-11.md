# SESSION-11 — polish + anomaly expansion + SSO/OIDC phase 1 (ROADMAP-V2 S11) + date-gated carry-overs

> Written by SESSION-10 close (D-068, 2026-07-09). Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP-V2.md`
> (plan of record) + `RESUME-PROMPT.md` §7/§8/§12 before dispatching. Prod: **pulse v0.2.0**
> (commit 4657512, pre-D-068 image) + D-067 digest-refreshed caddy/clickhouse/backup, healthy.
> ⚠ D-068 reverted the CPU cap to 0.5 in compose+helm — safe at prod's current ~2 streams, but do
> NOT run a high-stream-count prod on the v0.2.0 image; the O(N²) fix ships with the next rollout.

## Mission

Execute ROADMAP-V2 §3 S11 + the two date-gated S10 carry-overs. Exit = (a) `PULSE_REPORT_LOGO_PATH`
white-label PDF logo TDD-green + boot-validated; (b) anomaly rule type e2e-proven in CI under the
CH mock (contract CR → CH aggregation → alert engine → UI rule builder → TDD; define the PRD
numeric target at scoping and add to ARCHITECTURE.md §4); (c) OIDC login round-trip proven in CI
with a mock OIDC server (phase 1: provider config + callback handler + session issuance; UI login
flow change = phase 2); (d) WO-B carry-over: keep-7 backup cycle-8 pruning observed + recorded
(date ≥2026-07-16 — first S11 run after that date; if still early, skip + record again);
(e) WO-F carry-over: CI promotions if date ≥2026-07-23 (JOB-level streak re-measure FIRST via
`gh api .../runs/<id>/jobs` — continue-on-error makes workflow-level lie; FULL-LIST PUT: contracts,
server, web, sdk, docker-build, helm, compose + web-e2e + csp-e2e; GET-diff proof; drop
continue-on-error; actionlint + faithful step repro; CodeQL required ONLY with explicit operator
OK — check OPERATOR-TODO for the answer).

## Work orders (sizes from ROADMAP-V2 §3 S11)

1. **WO-A [XS]** White-label PDF logo (§2.9) — `PULSE_REPORT_LOGO_PATH` env; reports package reads
   at PDF-generation time, fallback to embedded default; boot validation (WARN if set+unreadable,
   never crash). TDD: fallback bytes, override bytes, missing-file no-crash.
2. **WO-B [M]** Anomaly expansion (§2.8) — CONTRACT CR FIRST (new rule type `anomaly` in
   contracts/openapi + event schemas via INT-01 pattern), then rolling baseline (mean+σ,
   configurable lookback) in CH query layer, alert engine extension, UI rule builder, TDD at every
   level (§7 table). Scope the PRD numeric latency target explicitly.
3. **WO-C [L]** SSO/OIDC phase 1 (§2.10) — issuer/client-id/secret env config; `/auth/oidc/callback`
   handler; session issuance reusing existing session machinery; group→role mapping; TDD with mock
   OIDC server. Contract CR for new auth endpoints. UI deferred to phase 2.
4. **WO-D [XS, date-gated ≥2026-07-16]** keep-7 backup cycle-8 verification (S10 WO-B carry-over,
   ROADMAP-V2 §2.2) — list `/backups/pulse/` (backup sidecar volume), confirm oldest pruned at
   cycle 8, count ≤7, restore-verify still green. Record in decisions.md.
5. **WO-E [S, date-gated ≥2026-07-23]** CI promotions carry-over (S10 WO-F, ROADMAP-V2 §2.7) —
   spec in Mission (e) above.

## Preconditions (re-verify cheaply; note drift in decisions.md)

- Tree clean; ci+e2e+codeql GREEN at HEAD (`gh run list --branch main`; 0-step cancelled jobs =
  GitHub capacity blip → `gh run rerun <id> --failed`).
- Dependabot queue: triage any new PRs per `docs/dependabot-policy.md` (NEW, D-068 — the policy is
  now binding; co-upgrade clusters listed in its §6).
- Coverage standings (D-068): Go total per RESUME-PROMPT standing numbers / floor 70.2; web
  62.13/57.6/51 (gates 59/54/45); sdk 66.06/45.79/70.42 (gates 63/43/67; 3.52 KB). Never compare
  across instrumentation engines.
- CH-startup flake watch: occurrence #1 recorded (D-067, `TestQuery_GeoBreakdown_NonEmptyRows`
  60s budget). **2nd occurrence ⇒ bump 60→180s in ALL 4 harness copies, one TDD-gated commit.**
- U3: if the operator has set `PULSE_LICENSE_KEY` (minting unblocked by D-068 WO-C —
  docs/licensing.md §3), restart pulse and live-verify the beacon→QoE chain, then record.
- Binding rules unchanged: Go ONLY in docker golang:1.25 REPO-ROOT mount (D-028); gofmt gate on
  OUTPUT EMPTINESS; `sg docker -c`; pristine-copy compose staging (D-061), unique `-p`; commit by
  explicit path; no subagent reverts (D-063); contracts frozen — CR via INT-01 only (D-004);
  concurrent-session hazard (§14); LICENSE = PolyForm NC 1.0.0 root + MIT sdk.

## Gates (ORCH, before any commit)

- Contract CR touched → `redocly lint` + `ajv` + `npm run gen:api` drift check (§8.6).
- Go touched → full `-race` repo-root mount, floor 70.2, 0 FAIL/0 unexpected SKIP; gofmt emptiness.
- Web touched → lint + typecheck + full coverage gates at current baselines + build.
- Prod untouched this session unless U3 fires (then §8.8 smoke).

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope; push; `gh run watch` ci AND e2e AND codeql green.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-069 (per-WO evidence incl. skipped-trigger records).
3. RESUME-PROMPT ▶ START HERE → SESSION-12; ROADMAP-V2 §3/§4/§5 ledgers updated.
4. REFRESH `agents/handoffs/OPERATOR-TODO.md` + PushNotification at completion.
5. Write `sessions/SESSION-12.md` from ROADMAP-V2 §3 S12.
