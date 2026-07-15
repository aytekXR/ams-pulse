# SESSION-39 — planned at S38 close (D-100)

> Written by SESSION-38 close (2026-07-15). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146`. Read `RESUME-PROMPT.md` + `ROADMAP-V2.md` §2 + the final-assessment §5
> roadmap before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-40

Before dispatching: re-read ROADMAP-V2 §2 and the final-assessment §5 roadmap and **revise this
plan if a higher-leverage move exists.** This file is a starting point, not a contract. Record any
revision in the D-101 open block.

**S35–S38 all exercised that clause and all were right** — S35 → ship-readiness audit; S36 → user-intake
audit; S37 → §2.16 was already built, pivoted to entitlement enforcement; S38 → team-management UI turned
out to be advisory (non-authoritative role, no password login) so it pivoted to API-correctness hardening.
**Verify candidate status AND product-viability against the code before committing to a goal.**

## ⛔ At open — verify, do not assume

**★★ STANDING RULE (D-095): a session claiming "DONE" is NOT evidence that it landed.**

- `git log --oneline origin/main -4` — S38 (D-100, PR #73) + its docs PR should be on `origin/main`.
- Prod should print the **S38 build** (rolled forward at S38 close — see D-100 evidence for the exact
  `vX.Y.Z-N-g<sha>`): `sg docker -c "docker exec pulse-prod-pulse-1 /usr/local/bin/pulse version"`.
- `/healthz` should still report `ams_env_configured: true` (S36 regression check).
- **Prove an S38 fix is live** (prod is Enterprise, admin token available in `oguz-testing.md`):
  create a user with an invalid role → expect **400**, and a duplicate username → expect **409**:
  ```sh
  curl -s -o /dev/null -w "%{http_code}\n" -X POST --resolve beyondkaira.com:443:161.97.172.146 \
    -H "Authorization: Bearer <admin>" -H "Content-Type: application/json" \
    https://beyondkaira.com/api/v1/admin/users -d '{"username":"s39probe","role":"root"}'   # want 400
  ```
  (Clean up any probe user you create.)
- Re-check the operator queue **live**: GHCR anon pull → 401; AMS licence expiry (now ~9 days at S39 open
  — **from ~07-25 this outranks GHCR**).

## 🔧 Environment gotchas — read BEFORE running any gate

- **Go cannot run bare-metal.** Repo-root mount + `-w /repo/server` (test helpers resolve the meta DDL via
  `runtime.Caller` → `<server>/../../../contracts/...`, so mounting `server/` alone breaks them):
  ```sh
  sg docker -c "docker run --rm -v /home/aytek/repo/ams-pulse:/repo \
    -v pulse-gomod:/go/pkg/mod -v pulse-gocache2:/root/.cache/go-build \
    -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./..."   # S38 census: 24/24
  ```
- **Contract change? Regenerate `schema.d.ts`.** After editing `contracts/openapi/pulse-api.yaml`, run
  `cd web && npm run gen:api` and commit the regenerated `web/src/lib/api/schema.d.ts`, or the `contracts`
  CI check drifts. (S38: adding a 409 response added two lines to schema.d.ts.)
- **Playwright tests built `dist/`** — `npm run build` before it; skip only if zero web files changed.
  Node 20 + npm are on PATH locally, so `npm run typecheck|test|build` run bare-metal.
- **`sleep` in the foreground is BLOCKED.** Poll background docker runs with a `python3 -c` `time.sleep`
  loop, not the shell `sleep`. Pin an ABSOLUTE repo path in `-v` (a stray `cd server` drifts `$(pwd)`).

## Mission

> ### The two operator items still outrank everything a session can do.
> **GHCR is private** (anon pull → 401; one click). **The AMS licence expires 2026-07-27T13:45Z** —
> from ~07-25 a lapse + the next `antmedia` restart = total ingest death. Surface both, every time.

## S39 candidates — pick by leverage (verify against the ledger AND product-viability first)

1. **Out-of-band licence-expiry alerting** [S] — **strongest unblocked candidate.** The alert evaluator
   has no `license_expiry` metric; the only warning is a UI banner (≤14 days). A customer who never opens
   the dashboard gets no warning before downgrade — and it's directly relevant to the operator's OWN
   07-27 expiry. Self-contained, unambiguous. **Start here unless the evidence says otherwise.** Scope:
   emit a `license_expiry_days` gauge/metric the alert engine can evaluate + a default rule; verify the
   Manager exposes `ExpiresAt`.
2. **Team-management UI** — **BLOCKED on operator product ruling (operator-expected item 10).** Do NOT
   build until the operator decides the model (SSO-group-driven / add password login / stored-role-authoritative).
   The API is now correct (S38/D-100), so it's ready to build once ruled.
3. **Audit trail** [M–L] — no actor recorded on writes; gates SOC 2 / ISO 27001 buyers. Larger.
4. **The e2e gaps still open from S34's audit** [S] — Reports **Schedules** tab never activated by a test;
   Probes **create** happy-path never driven. Real holes; small; good if you want a bounded confidence win.
5. **§2.7 CI job promotions** — date-gated ≥ 2026-07-23. At S39 open (if ≥ 07-23) this MAY be unlocked;
   `web-e2e`/`csp-e2e` carry `continue-on-error: true`. Check the date.
6. **Reconcile the dead `PULSE_LICENSE_OFFLINE_FILE` path** [XS] — read by `internal/config/config.go` but
   `config.Load()` never runs (`main.go` uses `loadEnvConfig()`; `HOOK(BE-02)`). Wire or delete the ghost.

**Not candidates:** §2.16 (BUILT S25/S26), §2.19 (uipro), §2.20 (S35), §2.21 (S36), §2.22 (S37), §2.23 (S38).

## ⚠️ Binding lessons — carry into every wave

1. **★ Verify product-viability, not just existence, before building on a feature.** S38 found `/admin/users`
   CRUD exists — but the role it edits is non-authoritative and there's no password login, so the UI would
   manage a field that governs nothing. Trace the actual auth/data path before investing in UI.
2. **★ A gate with no test is not a gate; a green suite is not a working feature.** Mutation-prove every
   guard: remove it, watch its test go RED, restore. (S37's callback gate + S38's role/required checks all
   proven this way.)
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
2. `decisions.md` **D-101** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-40; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-40.md` (carry the standing-directive header).
6. **Roll prod forward** if server/web code changed, per `deploy/runbooks/upgrade-rollback.md` — STAMPED
   build (`--build-arg` on `build`, then `up -d` WITHOUT `--build`); smoke with **evidence**.

---

## ✅ RESULT — S39 DONE (D-101, PR #75, 2026-07-15)

**Chose candidate 1 (out-of-band licence-expiry alerting) — the standing-directive re-read confirmed it
was still the highest-leverage unblocked move.** Unlike S35–S38, this plan needed **no pivot**: the goal
was viable exactly as scoped (`Manager.ExpiresAt()` public, `cert_expiry` a clean precedent, no metric
allowlist to change).

**Delivered:** a `license_expiry` alert metric mirroring `cert_expiry` — `LicenseExpiryChecker` +
`evalLicenseExpiry` (`alert/license_expiry.go`), evaluator field/setter/dispatch case, a `serve.go`
adapter over `ExpiresAt()` wired through a **`wireAlertLicenseExpiry` seam**, plus the supported-metrics
runbook row. Rule shape `{metric:"license_expiry", operator:"lt", threshold:14}`; free/perpetual keys are
skipped (`ok=false`), expired keys fire. No API/schema/web change.

**At-open census (verified live):** `origin/main` = `d6e4a57`; prod = `v0.4.0-17-g34c2221` (S38) — the S38
fix was proven live at S38 close (invalid role → 400); `/healthz` `ams_env_configured:true`.

**Gates:** `gofmt` clean · `go build ./...` OK · full Go suite green (24 pkgs). **Two guards mutation-proven
RED** (perpetual-skip guard; the wiring pin) — the pin was added in response to the adversarial review,
which flagged that all three unit tests called the setter directly and thus never proved `serve.go` wires
the checker. Adversarial review otherwise clean (delivery path, dispatch mirror, all four key states, test
non-vacuity, no allowlist gap).

**Prod rolled forward:** rollback `pre-d101` = `v0.4.0-17-g34c2221`; backup exit 0; STAMPED build
**`v0.4.0-19-g38111c9`** deployed; evidence smoke green (healthz all-ok, running stamp = `-19-g38111c9`,
signed webhook 200, limits `512M/0.5cpu`, logs clean). Feature inert until an operator creates the rule +
a channel (side-effect-free smoke — no prod rule created).

**Operator action: NONE for the build.** Standing note: rule + channel are operator-created (same as
`cert_expiry`). Blockers re-verified live: GHCR anon → 401; AMS expiry 2026-07-27 (12 days). Full evidence:
`decisions.md` D-101.
