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

## D-009 · 2026-06-14 · Wave 2 gate ruling: focused fix-loop for the one wave-3 blocker

Wave-2 QA gate (WO-207) verdict **PASS_WITH_LIMITATIONS** (`qa/wave-2/gate-report.md`):
12/14 criteria PASS/WAIVED; all 6 implementation agents COMPLETE + committed;
DOC-01 done. Waivers limited to D-002 (no Docker) + D-007.5 (no Kafka broker).

Defects:
- **D-W2-002 (major, BE-02)** — the only wave-3 blocker. `accounting.go` queries
  the wrong rollup with non-existent columns (`bucket_ts`/`watch_s_state`/
  `peak_viewers_state` vs schema `bucket`/`watch_time_s`/`peak_concurrency`), so
  `GET /reports/usage` → 500 and `pulse diag --reconcile` → CH error 47 on the
  LIVE stack. The headline gate criterion "billing reconciles ±1%" passes only at
  the in-memory unit level (which bypasses ClickHouse). Deeper finding (ORCH-00):
  `ComputeUsage` sources from `rollup_audience_1d` (where `peak_concurrency =
  maxState(1) = 1` and egress is a 1000-kbps model) but WO-204 specified
  `rollup_usage_1d` (real `viewer_minutes`/`peak_concurrency`/`egress_bytes`/
  `recording_bytes`/`tenant`). The unit test masked both because it never touched CH.
- **D-W2-001 / D-W2-003 (minor, QA-01)** — `qa/wave-1/run-gate.sh` POST
  /alerts/rules now needs the wave-2-required `name` field; script exits nonzero.
  Test-code only; non-blocking.

**Ruling:** run a focused in-wave fix-loop (precedent D-006: do not carry a major
defect into the next wave). Scope = D-W2-002 (BE-02: source billing from
`rollup_usage_1d` per WO-204, correct all SQL-vs-schema drift in `accounting.go`,
and ADD a ClickHouse integration test that exercises `GET /reports/usage` +
`Reconcile` against a live seeded CH and asserts ±1% — closing the blind spot the
in-memory test left) + D-W2-001/003 (QA-01 fixes its own gate script). Then QA-01
re-gates the committed tree (live reconcile is the real check). Everything else is
non-blocking → D-010.

## D-010 · 2026-06-14 · CR ruling + non-blocking gap disposition (validation sweep)

CRs filed in wave 2:
- **APPROVED — `/admin/tenants` CRUD** (BE-02 CR-WO204-01 + FE-01 CR-1). F6 required
  "tenants ... CRUD per OpenAPI" but the D-004 freeze omitted the paths (the
  `tenants` meta table, `tenant` query param, and `tenant.go` matcher all exist —
  only the management paths are missing). This is a freeze oversight, not new
  scope. INT-01 will amend the OpenAPI (`/admin/tenants` GET/POST + `/{id}`
  GET/PUT/DELETE, `Tenant` schema aligned to the meta table: id, name,
  stream_pattern, meta_tag_key, meta_tag_value), then BE-02 implements routes and
  FE-01 builds the surface. **Scheduled into the validation-sweep defect-fix loop**
  (not the wave-2 fix-loop) to keep the wave-3 critical path tight — F6 management
  is a completeness item, not a wave-3 blocker.
- **DEFERRED — global white-label config endpoint** (FE-01 CR-2). Per-schedule
  `whitelabel_header` already exists; a global `/admin/whitelabel` brand-config
  endpoint is Phase-3 integrator polish per the WO-204 annotation. Post-MVP.
- **CLOSED — CR-3 source-test endpoint** (`POST /admin/sources/{id}/test`):
  implemented by BE-02 in wave 2 (closes the D-006 carry-over).

Non-blocking gaps carried to the validation sweep (owner in parens):
- GAP-2-003 Kafka `Lag()`/`ParseErrors()` not surfaced in /healthz (BE-02).
- GAP-2-004 beacon ingest not tier-gated on Free (fail-open) — hostile-surface
  hardening (BE-02).
- GAP-2-005 `/qoe/summary` uses a live health-score proxy, not `rollup_qoe_1h`
  CH queries (BE-02).
- GAP-2-001 `BuildTestMMDB` yields an invalid mmdb → `TestGeo_MMDBFixture` SKIP
  (anonymize/absent-path/interface ARE tested) (BE-01/QA-01).
- GAP-2-002 edge/origin viewer dedup `IsEdgeStream()` always false — needs
  multi-node clusters (BE-01, wave 3).
- INFRA GAP-206-01..05 (image publish, postgres Secret template, busybox digest
  pin, real-AMS-container matrix assertions, ingest-addr Helm value) — D-002/
  release-time class (INFRA-01).

## D-011 · 2026-06-14 · D-008 adherence note: SDK-01 blanket-staging incident

SDK-01's commit `2d2910f` used blanket staging and absorbed FE-01's `web/` files
(FE-01's own commit `4be5549` carried only its report). Tree CONTENT is correct
and complete — nothing lost — but commit attribution is wrong, violating D-008.2
(explicit-path staging). No tree action needed. Reinforce explicit-path staging
(`git add <listed paths>` only) in every fix-loop/wave dispatch prompt; consider a
pre-commit scope check in a later infra pass.

## D-012 · 2026-06-14 · Wave 3-MVP structure (F9 anomaly detection + F10 synthetic probes)

Per D-001, F9 and F10 ship minimal-but-working in a Wave 3-MVP. Contracts are
already frozen (D-004) for both: `GET /anomalies` (AnomalyFlag), `/probes` CRUD +
`/probes/{id}/results`; meta tables `anomaly_baselines` + `probes`; ClickHouse
`probe_results`. No INT-01 step (freeze stands; CR channel only). Work orders
WO-301..305.

Ownership split (single-writer map respected; BE-01→BE-02 sequential per D-003):

- **F9 anomaly detection → BE-02 entirely (WO-302).** It is product-plane like the
  alert evaluator: maintain rolling baselines (mean/stddev/sample_count via Welford
  or EWMA) in the `anomaly_baselines` meta table over viewers/error-rate/rebuffer
  metrics per scope+window; compute AnomalyFlags on read (observed-vs-baseline,
  |z| ≥ sigma, with a min-sample-count gate + default sensitivity tuned for the
  PRD target <1 false alarm/node-week); serve `GET /anomalies`. No separate
  anomalies storage table exists by design — flags are derived. Metric access
  reuses the alert evaluator's live snapshot + query layer.
- **F10 probes → split BE-01 (WO-301) + BE-02 (WO-302).** The "single probe
  runner" (D-001) is a data producer, so BE-01 owns the runner engine (outbound
  playback check: HLS manifest+first-segment fetch measuring success/TTFB/bitrate
  at minimum; honest minimal handling or documented-stub for webrtc/rtmp/dash) and
  the `probe_results` ClickHouse store (writer + time-range reader). BE-01 defines
  a `ProbeConfigSource` interface in `internal/domain` (the EventSink pattern):
  `ListEnabled() []ProbeConfig` + `RecordResult(probeID, ProbeResult)` (so the
  runner updates `probes.last_*` denormalized fields). BE-02 (WO-302) implements
  `ProbeConfigSource` over the meta `probes` table, builds probe CRUD +
  `GET /probes/{id}/results` (reads BE-01's results store), and wires runner+source
  into `pulse serve` (declared cmd edit, D-005).
- **FE-01 (WO-303):** `/anomalies` surface (flag list with scope/sigma/observed-vs-
  expected) and `/probes` surface (CRUD + results shown ALONGSIDE organic QoE with
  clear "synthetic" labeling — the PRD F10 acceptance). Generated types only.
- **QA-01 (WO-304) gate:** probe round-trip (create probe → runner executes →
  `probe_results` row in CH → visible via API/UI, labeled); anomaly false-alarm
  proxy (drive a synthetic steady metric stream through the baseline updater at
  default sensitivity, assert flag rate maps to <1/node-week; inject a real
  deviation, assert it IS flagged). VERIFY, NEVER FIX.
- **DOC-01 (WO-305):** anomaly + probe feature docs/runbooks; flip F9/F10 status.

Dispatch (one Workflow): parallel([BE-01(301) → BE-02(302)], FE-01(303)) →
QA-01(304) → DOC-01(305). Validation sweep (F1–F10 adversarial + deferred D-010
items incl. tenant CRUD) follows the wave-3 gate.

## D-013 · 2026-06-14 · Wave 3-MVP gate ruling: CLOSED; two "carried" defects are spurious

Wave-3 workflow (`pulse-wave-3-mvp`, run `wf_4320e819-3b5`) complete: 3 impl
agents COMPLETE + self-committed (BE-01 `31e0a13`, BE-02 `e9e4a99`, FE-01
`d63a28b`/`844abbf`), QA gate `05e0fd6`, DOC `2b55235`. QA-01 verdict
**PASS_WITH_LIMITATIONS** (`qa/wave-3/gate-report.md`). Measured: F9 false-alarm
**0.2594/node-week** (sigma=4.0, MinSamples=30, HysteresisTicks=10; PRD target
<1/node-week, 3.8× margin) + true-positive 20σ→1 flag; F10 probe round-trip
success ttfb=1ms bitrate=66.7kbps, degraded→http_5xx, 4-level synthetic labeling;
tier gates (anomalies Enterprise, probes Pro+) live-verified; kin-openapi
conformance; regression 17 Go pkgs / 109 web / 56 SDK / SDK 3.44 KB. Waivers:
D-002 only (probe CH round-trip covered by BE-01 integration_test.go; live-stack
gates by unit sweep).

**The gate report lists D-W2-001 + D-W2-002 as "carried from wave-2" defects.
These are SPURIOUS — empirically disproven by ORCH-00:**
- `accounting.go` is UNTOUCHED since the wave-2 close `558377c` (`git diff
  558377c..HEAD` = only `query.go`, which keeps the correct `watch_time_s`
  columns); `qa/wave-1/run-gate.sh:380` still carries the `name` fix.
- The authoritative live-ClickHouse test `TestAccountant_CHIntegration` PASSES
  (4.2s) on a freshly built `/tmp/pulse` → reconcile path works (D-W2-002 closed).
- Root cause of the false report: QA-01-wave-3 ran a STALE `/tmp/pulse` (built
  before the wave-2 fix `77e32c3`) and copied the wave-2 defect table without
  re-checking. Both wave-2 defects remain CLOSED (re-gate `558377c`, 0 defects).

**Ruling:** Wave 3-MVP gate CLOSED (PASS_WITH_LIMITATIONS, D-002 waiver only). No
fix-loop — there are no real open defects. An ORCH-00 correction note is appended
to `qa/wave-3/gate-report.md`. Process reinforcement for future QA prompts: REBUILD
all binaries (`/tmp/pulse`, mock-ams) before gating, and re-verify (never carry)
prior-wave defects against the current tree.

**Accepted minor scope crossing:** BE-02 (WO-302) edited `internal/prober/
prober.go` (BE-01 scope) for a 1-line TTFB floor (localhost TTFB rounds to 0ms,
breaking BE-01's `TestHLSProbe_Success`). Declared in the report; BE-01 had already
finished (sequential, no concurrent write); semantically correct. Accepted like the
D-005 cmd sharing — no revert.

**All F1–F10 now implemented in MVP form.** Non-blocking wave-3 gaps → validation
sweep / Phase-3 backlog (owner BE-02 unless noted): GAP-3-001 HLS segment TTFB
(needs CR for `segment_ttfb_ms`; Phase 3), GAP-3-003 master-playlist bitrate=0
(follow variant; Phase 3, BE-01), GAP-3-004 anomaly zero-stddev blind spot
(epsilon floor; Phase 3), GAP-3-006 no Pro-tier end-to-end test, FE act() warning
(cosmetic). Next: validation sweep (adversarial F1–F10 vs PRD §7 + ARCHITECTURE §4
budgets) folding in the approved D-010 `/admin/tenants` CRUD CR, then consolidation
+ `IMPLEMENTATION_LOG.md`, then notify user.

## D-014 · 2026-06-14 · Validation finding: the "Business" tier is missing (4-tier PRD vs 3-tier impl)

V1 closed the F6 tenant CRUD (INT-01 `2323429`, BE-02 `3793b9c`, FE-01 `cd5c4d5`,
QA live-reconcile drift 0.0000%); the one blocker DEF-QA-001 (TenantsTab.test.tsx
wrong PaginatedMeta/tier types) was fixed by ORCH-00 directly (`38469bf`, a
QA-diagnosed 9-line test-mock fix, no FE agent running — tsc -b/build/lint/127 tests
green). While verifying it, ORCH-00 found a cross-cutting inconsistency:

**PRD §7.11 defines FOUR pricing tiers** — Free ($0), Pro ($99), **Business ($299:
up-to-5 nodes, 13-month retention, PagerDuty+webhooks, usage/billing reports,
multi-tenant, API+Prometheus)**, Enterprise ($799: +SSO, white-label, air-gapped,
anomaly detection). **The implementation has only THREE** (`License.tier` enum =
`free|pro|enterprise`; the D-004 freeze dropped Business). Consequences:
- Features the PRD assigns to **Business** are mis-gated to **Enterprise**: F5
  PagerDuty/webhook channels, F6 reports + multi-tenant (incl. the new tenant CRUD,
  gated `enterprise`), F8 API tokens + Prometheus. A real $299 "Business" customer
  cannot be represented or correctly billed.
- The UI upsell copy says "requires Business tier" (`web/.../ReportsPage.tsx`) while
  the gate checks `enterprise` — symptom of the same gap.

**Disposition:** this is a genuine PRD-vs-impl correctness gap, not a naming nit
(the monetization model is the product). It is a lead for the V2 adversarial sweep
(tier-gating verifier produces the full cross-feature impact) and will be FIXED in
the validation fix-loop (V3). **Pre-approved CR (ORCH-00, D-004 authority):** add
`business` to the `License.tier` enum (between `pro` and `enterprise`) and re-map
entitlements to the PRD §7.11 table — Free=email/1-node/7d; Pro=+Slack/Telegram/
QoE-beacons/90d/CSV; Business=+PagerDuty/webhook/reports/multi-tenant/API/Prometheus/
13mo/5-node; Enterprise=+SSO/white-label/air-gapped/anomaly/unlimited. Touches
contract enum, `internal/license` entitlement matrix + every gate call site, UI
upsell copy/logic, and the tier tests. Owner chain: INT-01 (enum) → BE-02 (license
+ gating) → FE-01 (copy/logic) → QA-01 (per-tier matrix). Do NOT pre-fix before V2
confirms the full call-site list.

## D-015 · 2026-06-14 · V2 result + V3 fix-loop plan (41 findings, 11 MVP-blocking)

V2 adversarial validation (run `wf_3bdbf61e-76d`, 14 verifiers) → 41 unique defects
(`agents/handoffs/validation/V2-triage-report.md`), 11 MVP-blocking. The wave gates
passed these because they used workarounds (manual ingest header, unit tests
bypassing the real path à la D-W2-002, tautological/never-fail assertions). Ruling:
the MVP is NOT yet "functional end-to-end" — run a comprehensive V3 fix-loop for all
P0/P1 + P2 (majors/security/contract); document P3 (test gaps, cosmetic, Phase-3) as
known limitations in IMPLEMENTATION_LOG. Structured as two sequential workflows
(bounded agent loads; server tree sequential per D-003; verify between them):

**V3a — make the data flow (run `pulse-val-3a`):**
- INT-01 (contract): VD-01 add `business` to `License.tier` enum + entitlement-matrix
  doc; VD-X3-A AmsSourceStatus `reachable`; VD-X3-D 403 on `/anomalies`; VD-X3-C
  delete-404 semantics (document idempotent 204); VD-S4 body-cap → 64 KB in spec.
- BE-01 (data plane): VD-07 wire geo/UA resolvers into restpoller; VD-08 enrich
  beacon events (client IP/UA); VD-03 implement `IsEdgeStream()` + dedup; VD-20a
  HealthTracker→aggregator bridge; VD-22 REST `EventIngestStats`; VD-40 node Version;
  VD-17 valid test mmdb; VD-16/VD-25 doc-accuracy.
- BE-02-A (pipelines/queries, after BE-01): VD-10 main-port ingest→EventSink; VD-06
  geo/device breakdown queries; VD-11 `/qoe/summary` from rollup_qoe_1h + bitrate
  field name; VD-20b propagate health to `/qoe/ingest`; VD-21 ingest timeseries +
  drop_events; VD-23 IngestTracker interface; VD-37/38 egress/peak labels;
  VD-X3-A/C handlers.
- SDK-01 (parallel, disjoint): VD-09 header `X-Pulse-Ingest-Token`; VD-12 HLS
  rebuffer_end; VD-13 HLS bitrate from levels.
- QA-01 mini-verify: REAL-SDK beacon round-trip (VD-09), geo/device non-empty
  (VD-06), health score >0 (VD-20), ingest timeseries (VD-21), build/vet/test.

**V3b — correctness: gating/alerting/security/UI (run after V3a):**
- BE-02-B (alerting): VD-28 muted suppression; VD-29 group_by storm grouping; VD-30
  node_down via eviction not CPU proxy; VD-32 rebuffer/error from rollup_qoe_1h;
  VD-33 cron range; VD-36 server 5-field cron.
- BE-02-C (gating+WS+fleet+security, after B): VD-01 gating per §7.11 matrix; VD-35
  reports gate; VD-15 beacon Pro+ gate; VD-02 WS LiveOverview shape; VD-39 fleet role;
  VD-S1 metrics-token constant-time; VD-S2 WS origin check; VD-S3 token-kind scope.
- FE-01 (parallel): VD-01 upsell copy/isGated; VD-36 cron presets; VD-02 WS field
  mapping; VD-X3-B granularity→interval.
- QA-01 full re-gate + add the missing tests (VD-05/18/19/24/26/34/41) + wave-1/2/3
  regression. DOC-01: reconcile docs + honest feature-status + remaining limitations.

After V3b passes: consolidation + `IMPLEMENTATION_LOG.md` (per F1–F10: done / issues /
resolutions / known limitations) → notify user, STOP for review.
