# Decision log — append-only (ORCH-00)

Rulings on PRD ambiguities, scope, waivers, and contract-change approvals.
Newest at the bottom. Referenced by DEVLOG.md.

---

## D-001 · 2026-06-11 · Scope: all features in MVP form, including Phase 2/3

PRD §7.14 stages F1–F10 across three phases. The build directive is "build all
features in MVP form, do not skip any PRD-specified functionality." Ruling: waves
1 and 2 run in full per `agents/manifest.yaml`; F9 (anomaly detection) and F10
(synthetic probes) are added as a Wave 3-MVP in minimal-but-working form
(statistical baselines, single probe runner; no mobile SDKs, SSO, or hosted
option — those are explicitly post-MVP platform work, not features).

## D-002 · 2026-06-11 · Environment: no Docker, local process stack for verification

Docker is not installed on the build machine. Docker Compose bundle, Dockerfile
and Helm chart are authored and lint-validated but not executed here. End-to-end
verification uses: pulse binary + ClickHouse single binary (`/tmp/clickhouse`,
v26.6.1, `clickhouse server` mode) + a mock AMS server (QA-01 owns it) that
serves REST v2, the analytics log format, and webhooks. The compose-up gate is
therefore verified by analysis + local-stack equivalence, recorded as a known
limitation in the QA report.

## D-003 · 2026-06-11 · Git baseline + serialized server/ writes

Repo was not under version control; initialized git with the skeleton as the
baseline commit so wave work is checkpointed and recoverable. Because BE-01 and
BE-02 share `server/go.mod`/`go.sum` and there is no worktree-able remote
history to merge, BE-01 and BE-02 run **sequentially** within each wave (BE-01
first — it owns `internal/domain`), while FE-01/SDK-01/INFRA-01 run in parallel
with them (disjoint trees). This satisfies the single-writer rule without
worktree merge risk.

## D-004 · 2026-06-11 · Full contract freeze up front (waves 1+2 combined)

The manifest freezes contracts per wave. Because all features build in this one
session (D-001), INT-01 freezes the **entire** contract surface once — full
OpenAPI (all features, response schemas, params, error envelope, beacon ingest
path, /metrics, /healthz), finalized event `data` payloads per type, complete
ClickHouse and meta DDL. Mid-build changes still route through ORCH-00 as
contract-change requests. Rationale: eliminates a second freeze round and the
re-issue churn it causes; risk accepted for MVP scope.

## D-005 · 2026-06-11 · cmd/pulse assembly is sequentially shared

`server/cmd/pulse` is BE-01 scope, but final assembly needs BE-02's api/alert/
license constructors which do not exist when BE-01 runs. Ruling: BE-01 wires the
data plane and leaves clearly-marked assembly hooks; BE-02 (running strictly
after BE-01 per D-003) may extend `cmd/pulse` wiring, declaring the edit in its
completion report. Single-writer's intent (no concurrent writes) is preserved by
the serialization.

## D-006 · 2026-06-12 · Wave 1 gate ruling: fix-loop in-wave; contract CRs approved

Wave-1 QA gate verdict PASS_WITH_LIMITATIONS (sole waiver: Docker Compose
execution, per D-002). Ruling: run a fix-loop within wave 1 before declaring
the gate closed — the major defect D-W1-001 (node CPU/mem normalized 100×
too high) corrupts fleet health and alert calibration and must not carry
into wave 2.

**Fix-loop scope:** D-W1-001/003/005 (BE-01), D-W1-002/004 (BE-02), then
targeted QA re-verification (re-run `qa/wave-1/run-gate.sh` + per-defect
checks). D-W1-006 (AMS version-matrix Go tests) is deferred to wave 2 /
validation sweep — it needs real AMS containers; carried forward, owner QA-01.

**Contract change requests (first uses of the D-004 CR channel) — all approved:**

- CR-1 `AlertRule.name` (string) — added to OpenAPI + meta DDL; implemented
  in this fix-loop (BE-02 store/API, FE-01 consumes; removes group_by
  label workaround).
- CR-2 `AlertRule.enabled` (boolean) — same treatment as CR-1 (replaces the
  muted-toggle workaround; `muted` stays, distinct semantics: enabled=off
  means not evaluated, muted means evaluated-but-silenced per PRD F5).
- CR-3 `POST /admin/sources/{sourceId}/test` + `AmsSourceStatus` schema —
  contract added NOW; server implementation deferred to wave 2 (touches
  collector/amsclient wiring); FE keeps its generic workaround until then.
- CR-4 `contracts/README.md` codegen path `../../contracts/…` → `../contracts/…`
  — trivial doc fix, INT-01.

All four applied by an INT-01 fix agent (contract freeze is amended through
this approval, per D-004). ORCH-00 housekeeping done directly: .gitignore
entries for ClickHouse test artifacts + built binaries (BE-01 gap, owner
ORCH-00).

## D-007 · 2026-06-12 · Wave 2 structure: no INT-01 step; beacon ingest scope; BE-02 split

Rulings for the wave-2 work orders (`agents/handoffs/wave-2/WO-201..208`):

1. **No INT-01 wave step.** D-004's freeze already covers wave 2+3-MVP surface;
   contract changes route through ORCH-00 CRs (as exercised in D-006). The
   manifest's wave-2 `[[INT-01], …]` slot is satisfied by the standing freeze.
2. **Beacon ingest (`server/internal/collector/beacon/`) is BE-02 scope for
   wave 2**, confirming WO-102/103's working assumption. Rationale: it is an
   HTTP product surface (token auth, rate limits, schema validation — hostile
   input, ARCHITECTURE §3.5) not an AMS wire-format concern; BE-01/BE-02
   serialization (D-003) removes write-conflict risk. Manifest scope map is
   NOT edited; this WO-scoped exception is recorded here.
3. **BE-02's wave-2 load is split into two sequential work orders** run by the
   same agent role: WO-203 (ingest + QoE/fleet/Prometheus/channels/gating)
   then WO-204 (F6 reports/exports). One double-size order risks agent
   stalls/context exhaustion observed as retries in wave 1.
4. **Geo enrichment ships reader-only.** MaxMind-format `.mmdb` reader with a
   configurable path + anonymize-IP switch; no database bundled (GeoLite2
   licensing requires per-user registration). Absent DB ⇒ honest no-op +
   documented setup. Satisfies F2 "IP-derived, anonymizable" within
   redistribution constraints.
5. **Kafka source verification limitation.** No broker runs on this machine;
   the Kafka source is contract/unit-tested against an in-process fake.
   Recorded as a D-002-class gate waiver; AMS-side Kafka E2E goes to the
   version-matrix CI (with D-W1-006) once real AMS containers are available.

## D-008 · 2026-06-12 · Per-agent commits after self-verification (user directive)

From wave 2 onward, every implementation/docs agent COMMITS its own changes
once its work-order acceptance criteria pass (its "solution is verified"),
instead of leaving everything uncommitted for one ORCH-00 wave commit.

Rules (enforced in every work order and dispatch prompt):

1. **Verify first.** Run your WO acceptance criteria; commit only when they
   pass. Partial/blocked work is NOT committed — report it instead.
2. **Stage by explicit path, never `git add -A`/`-u`/`.`** — agents run in
   parallel in one working tree; blanket staging would swallow another
   agent's in-flight files. Stage only paths inside your charter scope (plus
   declared cmd/ edits per D-005).
3. **Message format:** `<AGENT-ID> WO-XXX: <summary>` body listing
   verification evidence (commands run, key measurements). No push.
4. **Index contention:** if `.git/index.lock` is busy, wait and retry
   (bounded); never delete the lock.
5. QA-01 commits its qa/ artifacts + gate report the same way. ORCH-00 still
   makes a small wave-close commit (decisions, DEVLOG, handoffs) at the gate
   and remains the only committer for orchestration files.
6. The wave gate itself is unchanged: QA verifies the integrated tree after
   all agent commits; a fix-loop produces follow-up commits the same way.

Rationale: user directive (2026-06-12) + recoverability — wave 1 held ~6
agents of work uncommitted for hours; a crash would have lost it. Single-writer
scopes (manifest) make per-scope staging safe.
