# SESSION-40 — planned at S39 close (D-101)

> Written by SESSION-39 close (2026-07-15). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> Read `RESUME-PROMPT.md` + `ROADMAP-V2.md` §2 + the final-assessment §5 roadmap before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-41

Before dispatching: re-read ROADMAP-V2 §2 and the final-assessment §5 roadmap and **revise this plan if a
higher-leverage move exists.** This file is a starting point, not a contract. Record any revision in the
D-102 open block.

**S35–S38 all exercised that clause and all were right; S39 re-read it and confirmed candidate 1 unchanged
(no pivot needed — the first non-pivot in five sessions).** The lesson stands both ways: **verify candidate
status AND product-viability against the code before committing** — sometimes that confirms the plan, not
just overturns it.

## ⛔ At open — verify, do not assume

**★★ STANDING RULE (D-095): a session claiming "DONE" is NOT evidence that it landed.**

- `git log --oneline origin/main -4` — S39 (D-101, PR #75) + its docs PR should be on `origin/main`.
- Prod should print the **S39 build `v0.4.0-19-g38111c9`** (rolled forward at S39 close):
  `sg docker -c "docker exec pulse-prod-pulse-1 /usr/local/bin/pulse version"`.
- `/healthz` should still report `ams_env_configured: true` (S36 regression check).
- **Prove the S39 feature is wired live without a side effect:** the running stamp = `-19-g38111c9` is the
  standing proof (the metric is compiled in and `wireAlertLicenseExpiry` runs at boot). Do NOT create a
  prod `license_expiry` rule just to test — that is an operator action and writes a persistent row. If you
  want a live functional check, do it against a throwaway meta store, not prod.
- Re-check the operator queue **live**: GHCR anon pull → 401; **AMS licence expiry 2026-07-27T13:45Z — now
  ~10 days at S40 open; this is the top hard blocker** (a lapse + the next `antmedia` restart = total ingest
  death). It is NOT fixed by S39 — `license_expiry` covers the *Pulse* key, not the *AMS* key.

## 🔧 Environment gotchas — read BEFORE running any gate

- **Go cannot run bare-metal.** Repo-root mount + `-w /repo/server` (test helpers resolve the meta DDL via
  `runtime.Caller` → `<server>/../../../contracts/...`, so mounting `server/` alone breaks them):
  ```sh
  sg docker -c "docker run --rm -v /home/aytek/repo/ams-pulse:/repo \
    -v pulse-gomod:/go/pkg/mod -v pulse-gocache2:/root/.cache/go-build \
    -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./..."   # S39 census: 24/24
  ```
- **Contract change? Regenerate `schema.d.ts`.** After editing `contracts/openapi/pulse-api.yaml`, run
  `cd web && npm run gen:api` and commit `web/src/lib/api/schema.d.ts`, or the `contracts` CI check drifts.
- **Playwright tests need built `dist/`** — `npm run build` before it; skip only if zero web files changed.
- **`sleep` in the foreground is BLOCKED.** Poll background docker runs with a `python3 -c` `time.sleep`
  loop, not the shell `sleep`. Pin an ABSOLUTE repo path in `-v` (a stray `cd server` drifts `$(pwd)`).
- **Prod deploy is LOCAL** — follow `deploy/runbooks/upgrade-rollback.md`: validate config → tag `pre-dNNN`
  → backup (exit 0) → STAMPED `compose build --build-arg …` → assert stamp ≠ dev/unknown → `up -d` WITHOUT
  `--build` → evidence smoke. DC_ARGS = 5 overlays + `--env-file deploy/.env`. Rollback tag from S39:
  `pre-d101` = `v0.4.0-17-g34c2221`.

## Mission

> ### The AMS licence expiry (2026-07-27) still outranks everything a session can do.
> From ~07-25 a lapse + the next `antmedia` restart = total ingest death. Surface it, every time.
> GHCR is still private (anon pull → 401; one operator click). Both re-verified live each session.

## S40 candidates — pick by leverage (verify against the ledger AND product-viability first)

1. **Audit trail — actor on every write** [M–L] — **strongest unblocked product gap.** No actor/subject is
   recorded on mutating API calls (rule/channel/user/probe/schedule create-update-delete), so there is no
   "who changed what, when." This **gates SOC 2 / ISO 27001 buyers** and is the natural next
   product-completeness step after the S36–S39 auth/entitlement/correctness arc. Scope carefully at open:
   an append-only audit table in the meta store + a write-path hook that captures (actor from the session
   scope, action, target, timestamp) + a read endpoint; decide whether OIDC-session and bearer-token actors
   are both captured. **Verify the size against the code before committing — this may want to be phased.**
2. **The e2e gaps still open from S34's audit** [S] — Reports **Schedules** tab never activated by a test;
   Probes **create** happy-path never driven. Real holes; small; a bounded confidence win if you want a
   quick, self-contained session instead of the larger audit-trail build.
3. **Reconcile the dead `PULSE_LICENSE_OFFLINE_FILE` path** [XS] — read by `internal/config/config.go` but
   `config.Load()` never runs (`main.go` uses `loadEnvConfig()`; `HOOK(BE-02)`). Wire it or delete the ghost
   — either way, remove the dead-code trap. Good pairing with a larger item or a fast clean-up session.
4. **Seed a default `license_expiry` rule** [XS] — S39 shipped the metric but left rule-creation to the
   operator. A first-boot default rule (e.g. `lt 14`, wired to the existing default channel if one exists)
   would make the warning fire out-of-the-box. **Product ruling first:** decide the default threshold and
   whether to auto-create a channel; don't guess. Small once ruled.
5. **§2.7 CI job promotions** [S] — date-gated **≥ 2026-07-23**. Still gated at S40 open (07-15). When the
   date passes, `web-e2e`/`csp-e2e` `continue-on-error: true` can be promoted to required. Check the date.

### BLOCKED — do not start without the operator

- **Team-management UI (§2.23 follow-on)** — BLOCKED on the operator product ruling (operator-expected
  item 10): SSO-group-driven only / add password login / make stored role authoritative. The API is correct
  (S38); ready to build once ruled.
- **§2.19 uipro Wave 2+** — OPERATOR-DIRECTED and phased; `brandkit/` is the operator's (D-071, G7 not
  approved). Verify the wave's design inputs are approved before starting; do not touch `brandkit/`.

**Not candidates (DONE):** §2.16 (S25/26), §2.19 W0/W1 (S31/32), §2.20 (S35), §2.21 (S36), §2.22 (S37),
§2.23 (S38), §2.24 (S39).

## ⚠️ Binding lessons — carry into every wave

1. **★ Verify product-viability AND candidate-status before building** — the clause cuts both ways: S38
   overturned its plan, S39 confirmed it. Trace the actual data/auth path either way.
2. **★ A gate with no test is not a gate; a green suite is not a working feature.** Mutation-prove every
   guard: remove it, watch its test go RED, restore. **And pin the wiring seam** — S39's adversarial review
   showed three unit tests can all pass while `serve.go` never wires the feature into the real evaluator; a
   `wireX` seam + a pin test that fires through the real object closes that (D-062 / D-101 pattern).
3. **★ Contract changes need `schema.d.ts` regenerated** or the `contracts` CI check drifts silently.
4. **★ Audit the whole family.** When you gate/fix one handler, grep every sibling with the same shape.
5. **RUN the doc/path; do not read it** (D-097/D-098). Adversarially verify findings with refuters.
6. **Positive allowlists over blocklists** for authz (D-098).

## Gates (before any commit)

- Web: lint + `tsc --noEmit` + build + `npx vitest run` + **Playwright in docker AFTER `npm run build`**
  (skip only if zero web files changed — say so). Contract drift: `npm run gen:api` → check `schema.d.ts`.
- Any Go change: full suite in `golang:1.25` docker (24/24), `vet`, `gofmt`.
- **`brandkit/` byte-untouched unless the operator ruled (D-071).** `docs/marketplace/` DRAFT-INTERNAL (D-081).
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** — the only expected dirty file; a CLEAN `git status`
  is a FAILURE signal.
- **NEVER** `git reset --hard` / `git checkout -- .` / `git stash` / `git clean` / `git restore` (D-096).
- **Never restart or "fix" AMS** — observe-only.

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — and VERIFY the merge landed on `origin/main`.**
2. `decisions.md` **D-102** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-41; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-41.md` (carry the standing-directive header).
6. **Roll prod forward** if server/web code changed, per `deploy/runbooks/upgrade-rollback.md` — STAMPED
   build (`--build-arg` on `build`, then `up -d` WITHOUT `--build`); smoke with **evidence**.

---

## ✅ RESULT — S40 DONE (D-102, PR #77, 2026-07-15)

**Chose candidate 1 (audit trail) — confirmed by the standing-clause re-read + a read-only scout** (the gap
was real: zero existing audit infra, actor already in context). Built the whole thing: an append-only
`audit_log` table, `s.audit(...)` threaded into all **24** mutating admin/config handlers, and
`GET /admin/audit-log` (keyset, newest-first). Actor from the bearer token in `ctxTokenKey` — no new
middleware. `detail` is non-sensitive only. Migration 0004 (SQLite idempotent + PG embed); OpenAPI +
`schema.d.ts`. **Documented out-of-scope** (test-fires, logout, OIDC provisioning) — not silent.

**Gates:** full Go suite (24 pkgs) · `gofmt`/`vet` · web `tsc`+`vitest`+`build`; capture mutation-proven RED;
store DESC/cursor tests; 2 param-conformance probes (floors bumped). **Adversarial review → 1 real defect
fixed** (update handlers audited after the re-fetch guards → committed mutation could go unrecorded on a
failed re-read; moved the audit before the re-fetch) + bounded the audit ctx to 5 s. **No secret leakage**
(all 24 `detail` payloads verified).

**CI:** `server` first RED on `TestPG_MigrationParity` (PG embed records 0004, test's SQLite side applied
only 0001–0003) — fixed by adding 0004 to the SQLite combined migration; re-ran green. `csp-e2e` flaked
(known `Live Dashboard` `toBeVisible` timeout; required `web-e2e` green; backend-only change).

**Prod rolled forward:** rollback `pre-d102` = `v0.4.0-19-g38111c9`; backup exit 0; STAMPED build
**`v0.4.0-21-g0b7decc`** deployed; evidence smoke green (healthz all-ok, stamp `-21-g0b7decc`, webhook 200,
limits `512M/0.5cpu`, logs clean). **Migration 0004 proven live** — WAL-aware SQLite copy shows `audit_log`
with all 10 columns. Side-effect-free (no prod audit rows written).

**Operator action: NONE for the build.** NEW operator item surfaced: **AMS trial expiry is documented
inconsistently** — `deploy/runbooks/self-hosted-ams.md` says 2026-07-12, the ledger says 2026-07-27
(live-verified S37–S39). Could not re-verify live this session (AMS creds operator-only). If it's 07-12 it
has **already lapsed**; operator must confirm. GHCR anon → 401 (unchanged). Full evidence: `decisions.md` D-102.
