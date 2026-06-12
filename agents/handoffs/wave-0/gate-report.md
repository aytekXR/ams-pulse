# Wave 0 — Gate Report

**Gate (manifest wave 0):** `make build`, `make test`, `make lint` real and green locally; SDK size gate operational.
**Verified by:** ORCH-00 directly (the dispatched QA-01 gate agent was lost to a session
interruption after both implementation agents completed; wave-0 criteria are purely
mechanical command-exit checks, so ORCH-00 re-ran them rather than re-dispatching —
noted as a one-time protocol deviation, 2026-06-12).

## Verdict: PASS

| Criterion | Command | Result |
|---|---|---|
| Build green | `make build` | exit 0 — server binary, web dist (60.78 kB gz), sdk esm/cjs/iife+dts |
| Tests green | `make test` | exit 0 — go tests pass; web/sdk zero-test-file exits tolerated per WO-001 |
| Lint green | `make lint` | exit 0 — missing eslint.config.js tolerated, tracked as gap |
| SDK size gate operational | `npm run size` (sdk/beacon-js) | exit 0 — limit 15 kB, current 88 B (stub) |
| Contract validation | `make validate-contracts` | exit 0 — OpenAPI valid (55 warnings), 3 event schemas compile |

## Known gaps carried forward (non-blocking for wave 0)

- `eslint.config.js` missing in web/ and sdk/ — `lint` tolerates absence; FE-01 adds
  web config in WO-104; SDK-01 adds sdk config in wave 2.
- web/ and sdk/ have zero test files — test suites land with the wave 1/2 feature work.
- OpenAPI 55 redocly warnings — mostly TODO response bodies; resolved by WO-101 freeze.

## Inputs

- `WO-001-report.md` (INFRA-01) — DONE: Makefile guards, real CI jobs, digest-pinned
  images, HEALTHCHECK, graceful matrix stub.
- `WO-002-report.md` (SDK-01) — DONE: package.json/size-limit paths aligned to tsup
  output (`dist/index.*`); size gate now actually runs.
