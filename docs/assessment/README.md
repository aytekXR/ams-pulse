# Pulse × AMS Validation & Product Fit Program

**Status:** Plan produced — S16 close. Execution begins S17.
**Owner:** ORCH-00 + operator (Aytek)
**Last updated:** 2026-07-11

---

## Purpose

This directory contains the complete validation and product-fit assessment
program for Pulse v0.3.0 running against the real Ant Media Server (AMS)
3.0.3 Enterprise deployment at `http://161.97.172.146:5080`.

The goal is not a smoke test — it is a systematic, evidence-backed answer to:

> *Does Pulse deliver on its PRD promises against a real AMS deployment, and
> is it ready for the Ant Media marketplace?*

Every finding is pinned to a timestamped artifact (AMS API snapshot, Pulse
API response, screenshot). A discrepancy between AMS ground truth and Pulse
output is a **FAIL** with a filed bug document. No UI-only assertions are
accepted; every viewer count or metric claim is cross-checked against the AMS
REST API directly.

---

## The 8 Phases

| Phase | Name | Primary Deliverable | Target Session |
|-------|------|---------------------|----------------|
| 1 | Product Understanding | `capability-map.md` (this set) | S16 (done) |
| 2 | Test Environment | `validation-environment.md` + harness scripts | S17 |
| 3 | E2E User Scenarios | Scenario runs + evidence packages | S17–S18 |
| 4 | Automated Validation Scripts | `qa/realams/` suite, CI-optional | S17–S18 |
| 5 | Bug Investigation | Bug docs in `docs/assessment/bugs/` | Rolling |
| 6 | Documentation Program | Docs gap list + authored docs | S18–S19 |
| 7 | PRD Validation Matrix | `prd-validation-matrix.md` | S19 |
| 8 | Final Assessment | `final-assessment.md` + exec summary | S19+ |

Phases 3 and 4 run concurrently — automated scripts capture evidence for
manual scenario walkthroughs.

---

## File Map

```
docs/assessment/
  README.md                   — this file; program overview and working rules
  capability-map.md           — Phase 1: AMS capabilities × Pulse coverage
  validation-environment.md   — Phase 2: harness design, publisher/viewer control
  scenario-matrix.md          — Phase 3+4: full scenario table with AMS+Pulse assertions
  session-plan.md             — Phase scheduling across S17–S19+
  bugs/                       — Bug investigation docs (Phase 5), one file per bug
  prd-validation-matrix.md    — Phase 7 (produced at S19)
  final-assessment.md         — Phase 8 (produced at S19+)
```

Harness scripts live at `qa/realams/` (proposed S17, consistent with
`qa/wave-*/` layout already in the repo). Evidence packages (JSON snapshots,
screenshots) land in `qa/realams/evidence/` with timestamped subdirectories,
never committed to main unless they are small (<50 KB) reference fixtures.

---

## How to Resume (any session)

1. Read `session-plan.md` to find the current phase and open tasks.
2. Read `scenario-matrix.md` for the specific scenario you are executing.
3. Confirm the real-AMS stack is reachable:
   ```bash
   curl -s http://161.97.172.146:5080/rest/v2/version | jq .versionName
   curl -s https://beyondkaira.com/api/v1/healthz
   ```
4. Confirm `ams-teststream` publisher status:
   ```bash
   docker ps --filter name=ams-teststream --format '{{.Status}}'
   ```
5. Authenticate to Pulse API using the token from `oguz-testing.md` line 159
   (`plt_0352...`). Token lives in `deploy/.env` as well.
6. Follow the scenario, capture AMS snapshot first, then Pulse snapshot,
   compare, record in `qa/realams/evidence/<date>-<scenario-id>/`.

---

## Working Rules

### Cross-Check Philosophy

**AMS API says N == Pulse API says N**, or it is a FAIL.

- Never trust the Pulse UI alone. Always curl the Pulse API endpoint and
  the AMS REST endpoint, compare the numeric values.
- Eventual consistency is real: AMS poll interval is 5 s. Allow up to
  **15 s** for counts to converge after a state change. Document the exact
  delay observed.
- Viewer counts have known approximation semantics (see
  `capability-map.md` § Viewer Counts). Tolerance windows are defined
  per-scenario in `scenario-matrix.md`.
- When a Pulse value is persistently wrong (not transient skew), file a
  bug document in `docs/assessment/bugs/`.

### Evidence Requirements

Every scenario execution must capture:

1. **AMS snapshot** — the raw JSON from the relevant AMS REST endpoint with
   `curl -s -b cookie.txt <url>` and a timestamp header
   (`curl -v` or add `-o /dev/null -D -`).
2. **Pulse snapshot** — the raw JSON from the Pulse API endpoint with the
   bearer token.
3. **Delta** — computed difference (numeric or structural) between the two.
4. **Pass/Fail verdict** with the observed Pulse value, expected value from
   AMS, and the gap.
5. If fail: create `docs/assessment/bugs/BUG-NNN-<short-slug>.md` with
   the fields from the Bug Investigation Protocol (Phase 5 template).

### Bug Document Template (Phase 5)

File: `docs/assessment/bugs/BUG-NNN-<slug>.md`

```markdown
# BUG-NNN: <one-line title>

**Severity:** critical | high | medium | low
**Component:** amsclient | normalize | restpoller | beacon | api | ui | prober
**Status:** open | confirmed | fixed | wontfix

## Reproduction Steps
1. ...

## Expected (AMS Ground Truth)
AMS endpoint: `GET /...`
Response: `{...}`

## Actual (Pulse Output)
Pulse endpoint: `GET /api/v1/...`
Response: `{...}`

## Root Cause
...

## Fix Suggestion
...

## Evidence
- `qa/realams/evidence/<date>-<scenario-id>/ams-snapshot.json`
- `qa/realams/evidence/<date>-<scenario-id>/pulse-snapshot.json`
```

### Operator-Interaction Rules

- Do NOT retry AMS login more than once in any automated script. The
  brute-force lockout is 2 failed attempts → 5-min lock, keyed by email.
  Use the `admin@` account for Pulse machine access; use `aytek@` for
  human-console access. Do not share the lockout counter.
- The AMS trial license expires 2026-07-12T12:09Z. Operator has waived
  this; sessions observe and report only. If Enterprise features become
  unavailable, note which scenario is blocked and continue.
- Never push more than 2 times per session (operator directive, S16).
- Docker root artifact cleanup: if a container writes root-owned files to
  a mounted directory, remove with
  `docker run --rm -v <dir>:/s alpine rm -rf /s/<target>`.
- No `git restore`, `git checkout .`, or `git reset --hard` without
  explicit operator instruction.

---

## Key Contacts and Pointers

| Resource | Location |
|----------|----------|
| AMS credentials | `oguz-testing.md` lines 101–127 (gitignored) |
| Pulse API token | `oguz-testing.md` lines 153–159 + `deploy/.env` |
| Pulse license env | `deploy/.env` — `PULSE_LICENSE_KEY` / `PULSE_LICENSE_PUBKEY` |
| Compose stack start | See `local_env.start_command` in scout data |
| AMS real captures | `agents/handoffs/real-ams-captures/` |
| E2E CI plan | `.github/workflows/e2e.yml` + `docs/E2E-TEST-PLAN.md` |
| PRD feature list | `docs/prd-report.md` §7 (F1–F10) |
| Architecture rules | `docs/ARCHITECTURE.md` §3–§4 |

---

## Relationship to Existing CI

The existing CI E2E suite (`e2e.yml`) runs against `mock-ams` — a
deterministic Go binary, not the real AMS. The validation program in this
directory runs against the **production AMS at 161.97.172.146:5080** using
the isolated `pulse-realams` compose stack (base + real-ams + realams-test
overlays, Pulse on `127.0.0.1:18090`).

The two test surfaces are complementary:

- CI/mock: deterministic, fast, regression-safe, does not require network
  access.
- Real-AMS validation: evidence-grade, exercises actual AMS wire format,
  finds format drift and integration gaps that mocks cannot.

Scripts under `qa/realams/` are **not run in CI** by default. They are
gated behind a `make validate-realams` target (to be defined in S17) that
requires the `PULSE_AMS_URL` env var to point to the real AMS.
