# SESSION-12 — Postgres meta backend + WebRTC probe phase 1 + S11 carries (ROADMAP-V2 S12)

> Written by SESSION-11 close (D-070, 2026-07-10). Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP-V2.md`
> (plan of record) + `RESUME-PROMPT.md` §7/§8/§12 before dispatching. Prod: **pulse v0.2.0**
> (commit 4657512, pre-D-068/pre-D-070 image) + D-067 digests, healthy. The next prod rollout
> carries BOTH the O(N²) fix (D-068) AND S11 features (D-070) — consider cutting v0.3.0 first
> (release pipeline proven D-058/D-067) so the clean-install test (WO-E) exercises a tag that
> matches main.
> ⚠ CHECK `docs/operator-expected.md` ANSWERS FIRST: CodeQL yes/no (WO-D), PR-first yes/no
> (WO-F). ALREADY RESOLVED post-D-070 (2026-07-10): gh token has `read:packages` — WO-E is
> UNBLOCKED and pre-staged (image `ghcr.io/aytekxr/ams-pulse:0.2.0` pulled + cosign-verified:
> Rekor logIndex 2128354996, commit 4657512). ⚠ Image tags have NO v prefix (git v0.2.0 →
> image 0.2.0) — docs fixed, remember it in the WO-E override file. AMS license expiry:
> operator says handled ("don't worry") — observe + report during WO-E, don't block on it.

## Mission

Execute ROADMAP-V2 §3 S12. Exit = (a) `PULSE_META_BACKEND=postgres` boots + passes migration
parity in CI (SQLite default untouched); (b) WebRTC probe returns a real result (not
`not_probed`) for a WebRTC stream in CI; (c) keep-7 cycle-8 pruning verified (boundary hit
~07-10 — oldest zip `pulse-20260707-073113` should be gone, count ≤7, restore-verify green);
(d) CI promotions IF date ≥2026-07-23 (JOB-level streak re-measure first; FULL-LIST PUT;
GET-diff proof; CodeQL only with explicit operator OK); (e) WO-F clean-install RELEASE test
IF operator unblocked GHCR (full runnable step list: D-070 + S11 scout report — pull+cosign,
pristine-copy install per install.md Path A, image-pin override, -p pulse-s12install,
real-AMS verify ≤15 min budget, down -v teardown, doc bugs fixed on divergence); (f)
enforce_admins re-arm (flip if operator said PR-first, else re-record rationale, D-V2-3).

## Work orders (sizes from ROADMAP-V2 §3 S12)

1. **WO-A [L]** Postgres meta backend (§2.13) — new `store/meta/postgres` implementing the
   same interface as sqlite; migration runner parity (0001+0002 + applySchemaUpgrades
   equivalents); connection pool config (`PULSE_META_BACKEND=postgres`, `PULSE_META_DSN`);
   TDD with a Postgres testcontainer under `-tags integration` in CI. SQLite default is NOT
   deprecated. Contract CR only if config surface needs OpenAPI exposure (unlikely).
2. **WO-B [L]** WebRTC probe phase 1 (§2.11) — headless-browser (or native) WebRTC probe;
   CI fixture from `real-ams-captures/`; contract CR for the extended probe result schema
   (INT-01 single writer). On this VPS run browser bits via
   `mcr.microsoft.com/playwright:v1.61.1-noble` (host lacks chromium libs, no sudo — D-055).
3. **WO-C [XS, carry]** keep-7 cycle-8 verification (§2.2) — list the
   `pulse-prod_pulse-backups` volume (alpine bind-mount pattern), confirm prune + count ≤7 +
   restore-verify; record in decisions.md D-071.
4. **WO-D [S, date-gated ≥2026-07-23]** CI promotions (§2.7) — spec in Mission (d).
5. **WO-E [M, UNBLOCKED 2026-07-10]** clean-install RELEASE test (S11 WO-F carry) — spec in
   Mission (e). GHCR access verified (pull + cosign green, image pre-staged locally as
   `ghcr.io/aytekxr/ams-pulse:0.2.0`). AMS side: operator-handled per their statement —
   verify live behavior during the test and report. Do NOT substitute a local build.
6. **WO-F [XS]** enforce_admins re-arm (§2.1, D-V2-3) — operator-answer-dependent.

Backlog seeded but NOT S12 (pick up only if the session runs light): §2.14 anomaly Detector
metric expansion (needs manifest owner for `internal/anomaly` first).

## Preconditions (re-verify cheaply; note drift in decisions.md)

- Tree clean; ci+e2e+codeql GREEN at HEAD (e2e now includes A5 anomaly; `gh run list`).
- Dependabot queue: triage per `docs/dependabot-policy.md`.
- Standings (D-070): Go **73.9%** (floor 70.2); web 79.69/76.25/47.33 (gates 59/54/45);
  sdk 66.06/45.79/70.42 (gates 63/43/67; 3.52 KB). Never compare across instrumentation.
- CH-startup flake watch stands: occurrence #1 (D-067); 2nd ⇒ 60→180s in ALL 4 harness
  copies, one TDD-gated commit.
- U3: if `PULSE_LICENSE_KEY` appeared in `deploy/.env`, restart pulse + live-verify
  beacon→QoE, record.
- OIDC phase-1 limitation is DOCUMENTED (SPA AuthGate still token-based; cookie auths API
  only) — phase 2 is S13+, do not "fix" it ad hoc.
- Binding rules unchanged: Go ONLY in docker golang:1.25 REPO-ROOT mount (D-028); gofmt gate
  on OUTPUT EMPTINESS; `sg docker -c`; pristine-copy compose staging (D-061), unique `-p`;
  commit by explicit path; no subagent reverts (D-063); contracts frozen — CR via INT-01
  (D-004); concurrent-session hazard (§14); authors NEVER touch `cmd/pulse` concurrently —
  serial wiring author pattern (D-070) worked, reuse it.

## Gates (ORCH, before any commit)

- Contract CR touched → `redocly lint` + `ajv` + `npm run gen:api` drift check (§8.6).
- Go touched → full `-race` repo-root mount, floor 70.2, 0 FAIL/0 unexpected SKIP
  (grep -v output for SKIP — D-070 caught a silent-skip false green in a NEW test file);
  gofmt emptiness; CGO_ENABLED=0 build (postgres driver must be pure-Go).
- Web touched → lint + typecheck + full coverage gates + build.
- Prod untouched unless U3 fires or a v0.3.0 rollout is explicitly decided (then §8.8 smoke
  + runbook; rollback tags stand).

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope; push; `gh run watch` ci AND e2e AND codeql green.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-071 (per-WO evidence incl. skipped/blocked-trigger records).
3. RESUME-PROMPT ▶ START HERE → SESSION-13; ROADMAP-V2 §3/§4/§5 ledgers updated.
4. REFRESH `docs/operator-expected.md` + PushNotification at completion.
5. Write `sessions/SESSION-13.md` from ROADMAP-V2 §3 S13.
