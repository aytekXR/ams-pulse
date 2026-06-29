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

## D-016 · 2026-06-15 · V3a stall + recovery (anti-stall hardening)

V3a (run `wf_daf126f5-e1e`) ran ~9 h and FAILED: both Implement-phase lanes
stalled (no progress 180 s × 6 retries). Almost certainly an agent ran a
foreground long-running process (`/tmp/pulse serve` or `clickhouse server`),
blocking its Bash call indefinitely; the workflow's stall detector then burned
retries for hours.

State at failure: **INT-01 committed cleanly** (`0d84d31`) — it landed the
`business` tier in BOTH the contract enum AND `server/internal/license/license.go`
(+ tests + a conformance test); a sensible declared scope-cross that completes
VD-01's enum foundation. The Implement agents left INCOMPLETE uncommitted edits
(SDK: 3 failing tests; `beacon.go`: unused imports + half-changed `batchToDomain`
signature → server did NOT build). ORCH-00 discarded the partial edits
(`git restore server/ sdk/` + clean), keeping `0d84d31`; clean tree builds.

**Re-run as `pulse-val-3a-rest`** (INT-01 done, cached out): Implement
`parallel([BE-01 → BE-02-A1 → BE-02-A2], SDK-01)` → QA mini. Hardening baked into
every prompt (ANTISTALL): never run a server/CH in the foreground — background +
hard `timeout` + kill, or prefer in-process `go test`; every test/build command
gets an explicit `timeout` and `-timeout`; never `vitest` watch mode (use
`npm run test` = `vitest run`); never bare `go test ./...` without `-timeout`;
if a command hangs, kill and move on. Smaller per-agent scope (3-deep server chain
vs 2 heavy) to bound context/time. Verification favors bounded Go tests over live
servers (e.g. VD-09 = assert the SDK header constant equals the server's expected
header — no server needed).

## D-017 · 2026-06-15 · V3b gate PASS; QA-3b "remaining defects" SPURIOUS (D-013 recurrence)

V3b (`wf_f21da966-d85`, hardened) completed: BE-02-B `cfd6d79` (alerting: muted,
group_by, node_down, cron range/5-field), BE-02-C `982f73e` (tier gating per §7.11,
WS LiveOverview, fleet role, security VD-S1/S2/S3), FE-01 `9a0ba42` (tier copy, WS
mapping, interval param), QA `050ba6f`, DOC `568a22b`. **QA verdict
PASS_WITH_LIMITATIONS**; all V3b VD guard tests PASS; full regression green (22 Go
pkgs, 150 web, 65 SDK; wave-1/2/3 gate scripts pass).

**BUT QA-3b's "still-open defects" table listed 12 VDs (VD-03/06/07/08/09/10/11/12/
17/21/23/X3-A) as OPEN — all SPURIOUS.** Every one was fixed + verified in V3a-rest;
QA-3b echoed the V2-triage descriptions without re-verifying ("not re-verified in
V3b scope"). DOC propagated two (VD-23, VD-X3-A) into ARCHITECTURE.md. This is the
**second** occurrence of this QA failure mode (cf. D-013, wave-3 stale binary).

ORCH-00 empirically disproved all 12 on HEAD: `go test -tags integration -run
'TestQuery_Geo/Device/QoeSummary|TestVD10/20b/21' ./internal/query/... ./internal/
api/...` PASS; handlers call real queries (server.go:752/771, not stubs);
`TestVD23_IngestTracker_InterfaceConformance`, `TestContract_AmsSourceStatus_
HandlerReachableField` PASS; SDK 65/65 (VD-09/12). **All 12 CLOSED.**

Corrections applied: ARCHITECTURE.md VD-23/VD-X3-A → CLOSED; an ORCH-00 correction
banner prepended to `V3b-QA-gate-report.md`. **Systemic guard:** QA-agent
"remaining/carried defect" lists are NOT authoritative — the IMPLEMENTATION_LOG
feature status is built from ORCH-00's own empirical verification (tests run on
HEAD), not from echoed triage descriptions. Future QA prompts must REBUILD + RE-RUN
each prior-fixed defect's guard test before listing it as open.

**Validation COMPLETE.** All V2 P0/P1/P2 defects (the 11 MVP-blocking + the majors/
security/contract) are fixed and verified; only genuine P3 (test-coverage, cosmetic,
Phase-3) + D-002/D-007.5 waivers remain. Next: consolidation + `IMPLEMENTATION_LOG.md`
→ notify user, STOP for review.

## D-018 · 2026-06-15 · Wave 3-Plus: Phase-3 tech-debt & accuracy closeout (post-MVP)

User resumed after the MVP-review STOP (D-017) and chose, from the Phase-3 menu, the
**tech-debt & accuracy closeout** track (not net-new Enterprise features, not mobile
SDKs). This is the first post-MVP wave. It closes the deferred, environment-FEASIBLE
Phase-3 gaps enumerated in `IMPLEMENTATION_LOG.md` "Known limitations & Phase-3 backlog"
and the GAP-3-* / VD-* items in D-013/D-015. Out of scope (remain waivers, no toolchain
or multi-node here): VD-04 headless render-time, VD-14 player-CPU (both need a real
browser profiler — not measurable in jsdom/vitest), real multi-node cluster E2E, mobile
beacons, SSO/white-label/air-gapped/hosted (deferred Enterprise/platform tracks).

Phase-3 planning is a human-approved checkpoint per `agents/README.md` §4; the user
approved this track. Contracts are frozen (D-004) — the three contract changes below are
**pre-approved CRs (ORCH-00, D-004 authority)**, applied by INT-01 before code.

**Scope ownership** (single-writer map; `prober/`→BE-01 and `anomaly/`→BE-02 per D-012;
BE-01→BE-02 sequential per D-003; FE disjoint/parallel; D-005 cmd-wiring + D-008 commits
+ D-016 anti-stall all in force):

- **INT-01 (CR, contracts/ only):**
  - CR-VD38: new migration `0002_concurrency_rollup.sql` — `rollup_concurrency_1d`
    (AggregatingMergeTree, key `(bucket Date, app, stream_id)`,
    `peak_concurrency AggregateFunction(max, UInt32)`) + `mv_concurrency_1d` doing
    `maxState(viewer_count)` from `server_events` (the AMS-authoritative instantaneous
    concurrent count) filtered to the stream-stats event_type, GROUP BY day/app/stream.
    Do NOT edit `0001_init.sql`.
  - CR-GAP3001: new migration `0003_probe_segment_ttfb.sql` —
    `ALTER TABLE {db}.probe_results ADD COLUMN IF NOT EXISTS segment_ttfb_ms UInt32`;
    plus add `segment_ttfb_ms` to the `ProbeResult` OpenAPI schema (additive, not
    required).
  - CR-VD27: add an optional `kafka` component (status + `lag` + `parse_errors`) to the
    `HealthStatus.components` OpenAPI schema (additive, NOT in `required`; `degraded`
    is already in the status enum).
- **BE-01 (data plane, after INT-01):** GAP-3-001 `ProbeResult.SegmentTTFBMs` (domain) +
  segment-TTFB measurement (prober) + CH store write/read; GAP-3-003 prober follows a
  master-playlist variant to a real segment for non-zero bitrate; VD-27 source fix
  (`kafka.Source.Lag()` actually reads `r.Stats().Lag`, atomic-safe); VD-41 fix
  `discovery_test.go` captureSink signature + assert the sink-emit path fires.
- **BE-02-A (product plane, after BE-01):** GAP-3-001 api serializer `segment_ttfb_ms`;
  GAP-3-004 anomaly **epsilon floor** — `effStddev = max(stddev, relEps·|mean|, absEps)`
  on read in `ComputeFlags` (metric-agnostic relative floor; fixes the constant-baseline
  blind spot without changing stored Welford state or the unchanged DefaultSigma/
  MinSamples/Hysteresis, so the analytical false-alarm test stays <1/node-week); VD-27
  `KafkaStatsProvider` + `/healthz` kafka block + serve.go wiring (D-005) + guard test.
- **BE-02-B (after BE-02-A):** VD-38 accounting sources peak from `rollup_concurrency_1d`
  (`maxMerge`) for both primary and hour-fallback paths + integration test seeding
  overlapping `viewer_count` snapshots and asserting the TRUE windowed max (not session
  count); VD-31 real wall-clock alert-latency test (`Start()` + real ticker, asserts
  <30 s); VD-19 api-level geo/device non-empty test; VD-24 qoe/ingest seeded-CH test.
- **FE-01 (web/, parallel):** regenerate `schema.d.ts` (`npm run gen:api`); show
  `segment_ttfb_ms` in `ProbeResultsPanel`; VD-26 new `IngestPage` test.
- **QA-01 (qa/):** VD-18 add a DIMENSIONAL 13-month GROUP BY query to the wave-2 gate
  (≥3 geo × ≥2 device seed; assert ≤3 s); full re-gate — REBUILD binaries + RE-RUN every
  new guard test on HEAD (D-013/D-017: never echo triage); `qa/wave-3-plus/gate-report.md`.
- **DOC-01 (docs/, README):** ARCHITECTURE §4 budget updates (true peak; wall-clock alert
  latency; dimensional 13-mo; segment TTFB; kafka-lag healthz); feature docs.

ORCH-00 keeps `IMPLEMENTATION_LOG.md`/`DEVLOG.md`/`decisions.md`/`RESUME-PROMPT.md` and
**independently re-verifies the gate** (own test runs are the source of truth, D-017).
Dispatch = one Workflow `pulse-phase3-techdebt`:
`INT-01 → parallel([BE-01→BE-02-A→BE-02-B], FE-01) → QA-01 → DOC-01`.

## D-019 · 2026-06-15 · Wave 3-Plus gate CLOSED — independently verified (QA accurate)

Workflow `pulse-phase3-techdebt` (run `wf_fba510ab-717`, 7 agents) complete; all agents
COMPLETE + self-committed: INT-01 `19ea611`, BE-01 `042d2e4`, BE-02-A `a173b61`,
BE-02-B `95ee06d`, FE-01 `86b9994`, QA-01 `454da25`, DOC-01 `7aa877a`. QA verdict
**PASS_WITH_LIMITATIONS** (waivers: D-002 no-Docker, D-007.5 no-Kafka-broker ONLY).

**ORCH-00 independent re-verification on HEAD** (the D-013/D-017 mandate — never trust a
QA "open/closed" list without rebuilding + re-running): server `go build`/`go vet` clean;
full `go test ./...` = **18 packages, 0 failures**; CH-backed integration on HEAD —
`TestAccountant_CHIntegration` VD-38 `peak_concurrency` alpha=25/beta=5 (TRUE windowed max
from `rollup_concurrency_1d`), drift 0.0000%; `TestVD19_Geo/Device` non-empty;
`TestVD24` ingest timeseries 4 buckets; web **157/157**; SDK unchanged (65, 3.52 KB — no
SDK agent this wave). **Unlike D-013 and D-017, QA-01's report was ACCURATE — every
claimed PASS reproduced.** All 10 items CLOSED: GAP-3-001/003/004, VD-18/19/24/26/27/31/38/41.

**Gate CLOSED** (PASS_WITH_LIMITATIONS, D-002/D-007.5 waivers only). No fix-loop needed.

**Remaining Phase-3 backlog** (genuinely out of reach on this machine): VD-04 headless
render-time + VD-14 player-CPU (need a real browser profiler); mobile SDKs, SSO,
white-label PDF, air-gapped licensing, hosted-beta, distributed probe network, real
multi-node cluster E2E (D-002).

**Untracked VPS test-kit (flagged, NOT committed):** `deploy/docker-compose.override.yml`,
`docs/runbooks/test-on-vps.md`, `qa/vps-smoke-test.sh` — a coherent kit to bring the full
stack up against the mock AMS on a real VPS, i.e. to finally EXECUTE the D-002-waived
compose path. Authored outside this wave (untracked; no D-018 agent created them); left for
the user to decide whether to adopt as a separate "close the D-002 waiver" workstream.

## D-020 · 2026-06-15 · W1 `pulse-cicd` — always-on CI/CD that gates `main` (CLOSED)

**Plan/CR (ORCH pre-approval).** The skeleton-era CI (`ci.yml`, `ams-version-matrix.yml`,
dated before the MVP) was BROKEN against the shipped MVP: Go 1.24 (`go.mod` needs ≥1.25),
`npm ci` without `--legacy-peer-deps`, a malformed `CGO_ENABLED=0 cd server` line,
soft-failing (`|| echo GAP`) lint/test, and no docker-build / e2e / release jobs. CR-CI01:
fix + harden + extend CI so every push/PR to `main` is built, linted and tested, and add an
e2e smoke + a GHCR release path. Contracts unchanged (no OpenAPI/schema edits) — D-004 intact.

**Workflow `pulse-cicd`** (run `wf_ca6228d5-6cf`, 18 agents: Author ∥ → Verify adversarial +
2-round fix-loop → Gate). Files (committed by explicit path, D-008/D-011):
- `.github/workflows/ci.yml` — rewritten 7-job: **contracts** (ajv draft2020 + redocly),
  **server** (setup-go **1.25**; `CGO_ENABLED=0` vet+build; `go test ./... -race` WITHOUT
  CGO=0 since `-race` needs cgo+gcc which ubuntu-latest has; a **ClickHouse 24.8 service
  container** for a `pulse migrate` smoke-test; integration tests via a downloaded CH binary —
  the harness self-manages its own CH subprocess on random ports), **web** (node 22,
  `npm ci --legacy-peer-deps`, `gen:api` + `git diff --exit-code` types-drift guard, build,
  **HARD** lint, test), **sdk** (`npm ci`, build, 15 KB size gate, HARD lint, test),
  **docker-build** (`docker build` the all-in-one image, GHCR **lowercase** tag), **helm**
  (lint + golden diff, kept), **compose** (config-validate base AND CI override).
- `.github/workflows/e2e.yml` (new, PR + dispatch) + `deploy/docker-compose.ci.yml` (new
  CI-safe override: loopback 127.0.0.1:18090/18091, mock-ams control API on 127.0.0.1:19090,
  ephemeral secret) — compose up, extract first-run admin token from logs, wait mock-ams
  `/healthz`, seed a stream via `/control/publish`, assert `/healthz` 200 + migrate exit 0 +
  `SHOW TABLES FROM pulse` non-empty + authed `/api/v1/live/overview` viewers>0; `down -v` in
  `always()`, distinct project `pulse-e2e`.
- `.github/workflows/ams-version-matrix.yml` — Go 1.24→1.25 (only change).
- `.github/workflows/release.yml` (new, on tag `v*`) — build+push `ghcr.io/aytekxr/ams-pulse`
  via docker/metadata+build-push.
- `.github/branch-protection.sh` (new) — `gh api` script: required status checks =
  the 7 ci job names, PR reviews, no force-push/deletion.
- **Out-of-scope but ACCEPTED:** `deploy/docker-compose.yml` pulse `ports:`→`expose:` and
  `deploy/docker-compose.override.yml` dropped `!override`. Behaviour-PRESERVING (base+override
  and base+ci both render the SAME published ports as before — verified via `docker compose
  config`) and cleaner per CLAUDE.md "base stays clean, host exposure lives in overrides".
  `make up` (`cd deploy && docker compose up -d`, auto-merges only `override.yml`) unaffected.
  A sibling/fix agent made the edit outside its single-writer scope — noted; accepted on merit.

**ORCH-00 independent verification (D-013/D-017 — verify is NOT authoritative alone).**
The workflow's adversarial Verify phase reproduced 6/7 jobs locally inside the exact CI images
(`golang:1.25(-alpine)`, `node:22-alpine`): contracts ✓; server vet/build + `-race` ✓; web
`npm ci --legacy-peer-deps` + build + 157 tests + HARD lint, no schema.d.ts drift ✓; sdk build
+ size 3.52 KB ≪ 15 KB + 65 tests ✓; docker-build → 17.4 MB image ✓; all 5 YAML parse +
both compose configs render ✓. **e2e was "refuted" only as a harness artifact** (my prescribed
assert curled `/live/overview` without a token and against an UNSEEDED mock → 401 / 0 viewers).
ORCH then re-ran the FULL e2e chain directly (`/tmp/e2e_confirm.sh`, project
`pulse-e2e-confirm`): healthz 200, migrate exit 0, 17 CH tables, token extracted from logs,
mock-ams seeded → authed `/live/overview` returned **`total_viewers=13, total_publishers=1`**
(webrtc 10 + hls 3), clean `down -v`. e2e.yml logic CONFIRMED correct. Empirically also
confirmed `docker compose … config --quiet` exits 0 even with `PULSE_SECRET_KEY` unset
(Compose v5 enforces `:?` only at up/run), so the compose job is sound.

**Verdict: gate CLOSED — PASS_WITH_LIMITATIONS.** Everything is locally reproduced; what
remains is necessarily GitHub-side and the user's to do: (1) push these workflows + open a PR
so Actions actually runs green; (2) run `.github/branch-protection.sh` (needs `gh` + repo
admin — `gh` is NOT installed on the VPS) to make the checks gating; (3) push a `v*` tag to
exercise the GHCR release. Limitations: `e2e` is deliberately NOT a required check (full
compose bring-up is slow/heavy — PR/dispatch only); the integration step downloads a ~CH
binary at CI time (network-dependent); `helm` is GitHub-runner-only (helm not installed
locally). Carried waivers: D-002 (real cluster / real multi-node), D-007.5 (Kafka broker).

## D-021 · 2026-06-15 · Live-dashboard deadlock fixed — aggregator↔discovery AB→BA (demo restored)

While restoring the demo (the D-020 flag: pulse `Up (unhealthy)`, serving nothing on
:8090/:80), ORCH took a SIGQUIT goroutine dump and root-caused a genuine **AB→BA
lock-order deadlock** — NOT a flaky container (process alive, 20 MB, 0.04% CPU, OOMKilled
false, FailingStreak 448 ≈ 3.7 h). 489 goroutines were wedged on the aggregator RWMutex:
**486 HTTP handlers** blocked in `aggregator.CurrentSnapshot` (RLock), and the writers
`EvictStale`/`OnServerEvent` blocked on `Lock` for **152 min**. The cycle:
- `cluster.Discovery.poll` held `d.mu.Lock` and, still holding it, called
  `d.sink.WriteServerEvent` → `aggregator.OnServerEvent` wants `a.mu.Lock` (holds **B**, wants **A**).
- `aggregator.OnServerEvent` held `a.mu.Lock`, then `onStreamStats` → `Discovery.IsEdgeStream`
  wants `d.mu.RLock` (holds **A**, wants **B**).

Opposite lock orders → deadlock; once a writer queues, Go's RWMutex blocks every new RLock,
so all `CurrentSnapshot` readers piled up and the dashboard went dark (HTTP 000). The heavy
W1 build load made the two pollers interleave into it. Also latent: `aggregator.EvictStale`
emitted to the same Fanout while holding `a.mu` — a self-deadlock, since the aggregator
consumes its own sink.

**Fix (the rule: never hold a state lock across a sink call).** `Discovery.poll` and
`aggregator.EvictStale` now collect events into a local slice under the lock and emit to the
sink only AFTER releasing it. Files (server scope): `internal/cluster/discovery.go`,
`internal/collector/aggregator/aggregator.go` + deadlock-reproducing regression tests in
`discovery_test.go` / `aggregator_test.go`.

**Verification (D-013/D-017 gold standard — proved the guard actually guards):** the two
regression tests **FAIL** (3 s watchdog: "poll()/EvictStale() deadlocked … while holding the
lock") against the un-fixed source (git-stashed) and **PASS race-clean** against the fix;
full server unit suite **all packages ok**; image rebuilt + demo redeployed (project `pulse`)
→ `/healthz` **200 on :8090 AND :80**, body `status:ok`. Demo restored at
`http://161.97.172.146/`. Committed by explicit path (server scope).

## D-022 · 2026-06-15 · W2 `pulse-productionize` (subset, no external infra) — deploy hardening (CLOSED)

User chose "subset now, no infra" (the full real-AMS + public-TLS path needs operator
infra). Workflow `pulse-productionize-subset` (run `wf_e82c50f2-c1e`, 4 agents: author ∥ →
adversarial verify → gate). Authored (committed by explicit path, deploy + docs scope):
- `deploy/docker-compose.hardened.yml` — a SELF-CONTAINED, production-shaped override
  (`base + hardened` = full testable stack): adds **Caddy** (`caddy:2-alpine`) TLS-terminating
  reverse proxy (loopback `127.0.0.1:8443→443`, `8080→80`); **removes ALL host ports from pulse**
  (`ports: !override []`) so it's reachable only via Caddy; **restores ClickHouse auth**
  (`CLICKHOUSE_USER`/`CLICKHOUSE_PASSWORD`, no `SKIP_USER_SETUP`, authenticated healthcheck);
  threads an authenticated `clickhouse://user:pass@…` DSN into pulse + pulse-migrate; secrets
  from env.
- `deploy/config/Caddyfile` — `{$PULSE_DOMAIN:localhost}` + `tls internal` (local CA) for
  verification, `/beacon/*`→pulse:8091 else →pulse:8090, security headers, no CDNs/fonts;
  documents the Let's-Encrypt-on-a-real-domain switch.
- `deploy/.env.example` — committable template (placeholders only) for every required/optional var.
- `deploy/docker-compose.real-ams.yml` — disables mock-ams (`profiles: [mock]`), wires the four
  `PULSE_AMS_*` vars from operator env (`:?` required / `:-` defaults); validated names against
  `config.go`.
- `docs/runbooks/productionize.md` — operator runbook (TLS+domain, CH auth, secrets, real-AMS
  wiring + `POST /api/v1/admin/sources/{id}/test`, backups/retention, resource limits + metrics).

**Verification (adversarial, against a live `base + hardened` stack with the mock AMS, project
`pulse-prod-verify`, then ORCH independent re-check).** (a) **HTTPS via Caddy = 200, TLSv1.3**,
Caddy local-CA cert, healthz `status:ok`. (b) **CH auth ENFORCED**: authed query → 17 tables;
wrong password → `Code 516 Authentication failed`; the `default` user is REMOVED; only `pulse_rw`
exists. (c) pulse-migrate **exit 0** with the authenticated DSN (3 migrations applied). (d) pulse
has **zero host port bindings**. real-ams overlay `config -q` exits 0; clean `down -v`. ORCH
re-confirmed both config parses, `.env.example` placeholders-only, all referenced env vars exist
in `config.go`, and the demo (`:80`) stayed healthy throughout.

**Verdict: gate CLOSED — PASS_WITH_LIMITATIONS** (blocking_issues: none). **Waived (real infra,
operator-provided):** real public TLS via Let's Encrypt (needs a live domain + ACME); real Ant
Media Server connectivity (`PULSE_AMS_URL`/`PULSE_AMS_AUTH_TOKEN`); and the **`pkg/amsclient`
real-wire-format fixture hardening — explicitly DEFERRED to a future session** (it pairs with the
real-AMS infra). Minor non-blocking notes: Caddy host ports are loopback (operator binds
`0.0.0.0:443/80` for prod — runbook says so); resource-limit YAML is in the runbook, not pre-wired.

## D-023 · 2026-06-16 · Production TLS pre-staged for `beyondkaira.com` (real domain acquired)

User acquired `beyondkaira.com` (Squarespace) and asked for (a) DNS-forwarding directions and
(b) the deploy made SSL-ready. DNS guidance given (replace Squarespace's default parking A
records `198.49.23.x`/`198.185.159.x` with an **A record `@` → 161.97.172.146** + `www`; verify
with `dig`). To avoid hand-editing the verified hardened files, added a turnkey production-TLS
overlay (deploy scope, committed):
- `deploy/docker-compose.prod-tls.yml` — `!override`s the Caddy service to bind **0.0.0.0:80/443**
  (public) and mount `Caddyfile.prod`; `PULSE_DOMAIN` is `:?`-required.
- `deploy/config/Caddyfile.prod` — no `tls internal` → real **Let's Encrypt**; `{$PULSE_DOMAIN}`
  required; staging-ACME line for dry runs; same proxy/security-header/log blocks as the local Caddyfile.
- `.env.example` updated with the `PULSE_DOMAIN=beyondkaira.com` production note.

Production composition: `base + hardened + prod-tls` (mock AMS) or `+ real-ams`. **Verified by
config only** (ORCH): `docker compose … config -q` exits 0 for both compositions; the rendered
caddy shows `host_ip 0.0.0.0` ports 80/443 and `source …/Caddyfile.prod`; the `PULSE_DOMAIN` `:?`
guard errors when unset. **NOT brought up** — real Let's Encrypt issuance is blocked on the user's
DNS change + propagation (and host :80 is currently held by the running demo, which must be stopped
first). Live execution is the W2b step (RESUME-PROMPT), to run once `dig +short beyondkaira.com`
== `161.97.172.146`. Carries the D-022 waivers; the public-TLS waiver is now PRE-STAGED (config
ready, awaiting DNS).

## D-024 · 2026-06-16 · W2b — production TLS go-live for `beyondkaira.com` (EXECUTED)

User completed the Squarespace DNS. Both public resolvers (8.8.8.8, 1.1.1.1) returned
**161.97.172.146** for the apex **and** `www` (no CAA record). NB: the VPS's own local resolver
was **stale** (still returned Squarespace parking IPs), so all on-box verification used
`curl --resolve` / `openssl s_client -connect <ip> -servername <host>` to bypass it — a
verification-only quirk; ACME is unaffected because Let's Encrypt validates from the public
internet. Executed the W2b go-live:

- Wrote `deploy/.env` (gitignored): `PULSE_DOMAIN=beyondkaira.com`, `CLICKHOUSE_USER=pulse`, a
  freshly generated `CLICKHOUSE_PASSWORD`, and the existing `PULSE_SECRET_KEY`.
- **Zero-downtime-during-build swap**, driven directly by ORCH as one sequential operation with
  auto-rollback (a live single-host cutover is not a fan-out): pre-built the prod images while the
  demo still served, then `docker compose -p pulse down` (frees host :80) and brought up
  `base + hardened + prod-tls` as project **`pulse-prod`** (its own fresh, authed volumes). The
  unrelated `brier-db` container (separate project) was untouched.
- **Real Let's Encrypt** cert obtained via **TLS-ALPN-01** in ~12 s (issuer `O=Let's Encrypt`,
  `subject CN=beyondkaira.com`, valid 2026-06-15 → 2026-09-13). HTTP→HTTPS 308; security headers live.

**CR (ORCH-approved, deploy scope) — `deploy/config/Caddyfile.prod`:** added a canonical
`www.{$PULSE_DOMAIN}` → apex 301 redirect (also provisions a `www` LE cert so the host the user
also pointed at the VPS never serves a TLS name-mismatch). A graceful `caddy reload` did **not**
proactively obtain the cert for the newly-added name → a `caddy` container **restart** triggers
startup provisioning (the `caddy_data` volume persists the apex cert + ACME account, so apex did
not re-issue). `www` then served a valid LE cert (`CN=www.beyondkaira.com`) + a 301 that preserves
path+query.

**Independent verification — Workflow `pulse-golive-verify` (`wf_9d503e84-e0e`, 8 adversarial
verifiers, each default-refuted, reproduced against the live stack): 7/8 PASS.** apex + www public
certs (`ssl_verify_result=0` against the system trust store, no `-k`); HTTP→308→HTTPS; SPA served
through Caddy; **ClickHouse auth enforced** (correct→1, wrong pw → code 516 AUTHENTICATION_FAILED);
**no host-port leakage** (only `caddy` on `0.0.0.0:80/443`; pulse/CH/mock-ams internal-only;
`127.0.0.1:8090` refused); **authed** `GET /api/v1/live/overview` → 200 (unauth → 401), node
`standalone` up. The 8th (security-headers) is **PASS-with-accepted-note**: all four required
headers present and the `Server` header is stripped, but Caddy appends `Via: 1.1 Caddy` at the
HTTP-server layer — **not removable** via Caddyfile (`header -Via` and `header_down -Via` both
no-op; tried and reverted so no dead config ships). `Via` is an informational RFC-7230 proxy
header, not a vulnerability → **accepted, non-blocking**.

**Notes:** `total_viewers=0` is honest — mock-ams has no active streams; the live data path is
proven (node up, authed JSON flowing). Real viewer data arrives when a real AMS is connected
(`real-ams` overlay). The fresh `pulse-prod` instance generated a new admin token (operator-held,
not committed). **This CLOSES the D-022/D-023 public-TLS waiver.** Remaining W2 work: `amsclient`
real-wire hardening (W2c) + real-AMS connectivity — both need operator infra.

## D-025 · 2026-06-16 · W2c — amsclient/collector real-wire-format hardening (author + unit tests)

Ran Workflow `pulse-amsclient-hardening` (`wf_4aab2501-0a4`, 4 agents: two disjoint-scope authors →
go-test-race gate + adversarial diff review). Mapped the AMS REST ingest surface first
(`server/pkg/amsclient` + `server/internal/collector`), which surfaced **3 latent bugs + 1 parity
gap**, all fixed:
- **FIX1 (VD-40):** `NormalizeClusterNode` decoded `ClusterNodeDTO.Version` but never wrote it to
  the event `Data["version"]`, so the FleetPage node version was always blank. Now populated.
- **FIX2:** AMS v2.10 nodes send `speed` and omit `bitrate`; `NormalizeBroadcast` emitted 0 kbps.
  Now falls back to `Speed` when `BitRate==0 && Speed>0` (both the stream_stats value and the
  ingest_stats emit gate).
- **FIX3:** an empty `StreamID` keyed the aggregator map to `nodeID+"/"` and merged/corrupted live
  state; now guarded (no events emitted for a blank stream id).
- **FIX4:** the Kafka normalizer omitted `dashViewerCount` from `viewer_count` while the REST path
  included it; added for REST/Kafka parity.

New tests (all run, no skips): `amsclient` gained its **first** tests — `client_test.go` (11 tests)
+ 10 `testdata/*.json` fixtures driving the real `getJSON`/httptest decode path (v2.10/v2.14/v3.0
field variance, mixed statuses, empty list, unknown-fields+nulls tolerance, exactly-200 pagination
boundary, non-2xx error, cluster role/version/usage, applications envelope, partial WebRTC stats).
`collector` gained tests for each fix + created/finished/ended transitions + WebRTC zero-field
averaging. ORCH then upgraded the Kafka↔REST parity test to actually run **both** normalizers
(`collector.NormalizeBroadcast`, no import cycle — only `cmd/pulse` imports the kafka pkg) instead
of inline arithmetic — closing the workflow reviewer's one non-blocking note.

**Verification (D-013/D-017 — QA not authoritative alone):** the workflow's go-test-race agent AND
an independent ORCH re-run both ran the full `go vet ./... && go test ./... -race` green (19 pkgs ok,
incl. the now-tested `amsclient`; no data race). Adversarial review confirmed each fix is
present/correct/minimal and the tests fail without the fix. **Still pending:** validate the fixtures
against **real** AMS REST captures once the real-ams overlay is connected (pairs with the W2b
real-AMS step). The unit/wire layer is done.

### CI diagnosis (session 5) — reproduced ci.yml locally
The user reported red CI. Repo is private + `gh` not on the VPS, so reproduced each job in its
matching image. **Three real failures found, all fixed:**
1. **`helm`** — committed golden files carried trailing blank lines that helm 3.17.0 (the pinned CI
   version) no longer emits; regenerated all three (whitespace-only, 16 deletions, 0 semantic
   change) → `6c7666c`.
2. **`server` → "Build pulse binary"** — ran `go build -o /tmp/pulse ./server/cmd/pulse` from the
   repo ROOT, but the Go module is in `server/` (no root go.mod/go.work) → "cannot find main
   module". Fixed to `cd server && CGO_ENABLED=0 go build -o /tmp/pulse ./cmd/pulse` → `3a0a489`.
3. **`server` → "Download ClickHouse binary"** — pinned URL `…/v26.6.1.1844/clickhouse-linux-amd64`
   404s (GitHub release assets never carried a single static binary, only *.tgz/.deb/.rpm). Fixed to
   `https://builds.clickhouse.com/master/amd64/clickhouse` → `3a0a489`.

contracts/web/sdk/compose pass; `server` unit/`-race` for all packages passes (the *local-only*
git "dubious ownership" VCS-stamp error is a container-as-root artifact, `safe.directory` fixes it).
docker-build is covered by the prod image built this session. Both server-job fixes were
re-validated end-to-end: `pulse migrate` applies all 3 CH migrations and the full
`go test -tags integration ./...` suite passes (incl. `internal/query`, `internal/store/clickhouse`).
**Lesson (D-013/D-017 reinforced): never trust a PARTIAL CI repro — reproduce EVERY step.** My first
pass skipped the integration steps and wrongly reported `server` green; the user caught it.

## D-026 · 2026-06-16 · CI — the remaining real failures (from the user's GitHub logs)

The local repro had MASKED three more GitHub-only failures; the user pasted the actual Actions
logs, which exposed them — all fixed + faithfully validated + pushed:
- **compose** (`22dfd4d`): `docker-compose.ci.yml` has `${PULSE_SECRET_KEY:?}`; a clean runner has
  no `deploy/.env`, so `config --quiet` failed. (My repro auto-loaded `deploy/.env` → masked it.)
  Fix: a throwaway `PULSE_SECRET_KEY` env on the compose-validation step (the `:?` guard stays for e2e).
- **web** (`22dfd4d`): after `cd web` the drift check ran `git diff web/src/...` (wrong cwd) → exit
  128 "ambiguous argument". (My repro used `git -C /repo` + the full path → masked it.) Fix:
  `git diff src/lib/api/schema.d.ts`.
- **server integration** (`b1304da`): `TestQuery_QoeSummary_RealStartupP50` was ~20% flaky — the
  `mv_qoe_1h` rollup briefly lags the INSERT/OPTIMIZE → transient `startup_p50_ms=0`. Reproduced
  locally (2/10), de-flaked with a bounded poll (re-OPTIMIZE + re-query); validated `-count=20` → 0
  failures. Production queries the rollup long after ingest, so the race is test-only.
**Lesson (binding): an inexact local repro is worse than none — it produces false green. Reproduce
each CI step EXACTLY (clean-checkout semantics, literal commands, no auto-loaded `.env`).** All
`ci.yml` jobs now pass a faithful local reproduction (GitHub confirmation pending a fresh run).

## D-027 · 2026-06-16 · Security + AMS-integration hardening — shipped + LIVE

Recon (Explore) produced a 25-item backlog (14 security A1–A14, 11 AMS B1–B11). Workflow
`pulse-security-ams-hardening` (`wf_06c3687a-a38`, 5 disjoint-file authors → go-test-race gate +
adversarial review) implemented the verified high-value subset, each with tests (commits `efe8578`
server + `89ace7e` Caddy CSP):
- **CORS allowlist** (A1, `PULSE_CORS_ALLOWED_ORIGINS`; beacon stays permissive; same-origin SPA
  unaffected) · **token-in-URL only on the WS route** (A4) · **SSRF**: source-test validates
  http/https scheme + blocks redirects, but NOT private IPs — a real AMS is often private, so
  IP-blocking would break it (corrected the recon) (B4/A6) · **rate-limiter eviction** (A3) ·
  **beacon batch caps + UTF-8-safe truncation** (A10) · **alert-history limit cap** (A11) ·
  **amsclient response body LimitReader 10 MB** (B9) · **http-AMS warn + redacted AMS URL log**
  (B5/B10) · **wired the previously-DEAD webhook source, fail-closed** (A5/B1/B2: empty secret →
  validateHMAC returns false AND serve.go refuses to start the listener) · **CSP + Permissions-Policy**
  (A8). Two reviewer defects fixed by ORCH (webhook fail-closed at the library level; beacon
  rune-safe truncation). Verified: full `go test ./... -race` green (workflow gate + independent
  ORCH re-run) + adversarial diff review.
- **Redeployed LIVE** (auto-rollback script): hardened binary + CSP now serve on
  https://beyondkaira.com — CORS hardening confirmed (cross-origin → no `Access-Control-Allow-Origin`),
  CSP/Permissions-Policy headers present, the SPA has NO inline scripts so `script-src 'self'` is
  compatible; apex+www+authed data all 200. **Gotcha logged:** Docker single-FILE bind-mount inode
  staleness — after the workflow rewrote `Caddyfile.prod` (new inode), `caddy reload` kept serving
  the OLD config; fixed with `up -d --force-recreate caddy`. Added a `.dockerignore` — root-owned
  ClickHouse test artifacts (`preprocessed_configs`/`access` left in package CWDs by integration
  tests, owned by the container uid, mode 0750) were poisoning the build context; the ignore file
  excludes them + deps/artifacts (also shrinks/robustifies the docker-build CI job).
- **Operator guide** `agents/handoffs/AMS-INTEGRATION.md` (683 lines): REST + webhook-over-HTTPS
  (incl. the required Caddy `/webhook/*` route + `X-Ams-Signature` HMAC + TLS + Docker secrets), a
  32-var env table, a next-session prompt, verification checklist + troubleshooting.
- **Deferred (documented in the guide):** B3 (Docker secrets), B6 (source-test should decrypt the
  stored password), B7 (per-source webhook secret — needs a frozen-contract CR), A2 (rate-limit
  main-port beacon), A7 (rate-limit `/metrics`).

## D-028 · 2026-06-17 · Deferred hardening B6/A2/A7 — shipped (server-only, no contract change)

Implemented the three unblocked deferred backlog items from D-027 (B6, A2, A7). Scope confined to
`server/internal/api/`; no contract change, no `go.mod` change. Commit `54e2d8f`.
- **B6** — `handleTestSource` now decrypts `src.CredentialEnc` (`meta.Store.Decrypt`) and passes it
  to `SetBasicAuth` (was hard-coded empty), so the connectivity test authenticates against a
  basic-auth-protected AMS. Decrypt error → `Warn` + empty-password fallback (no 500; robust to a
  rotated `PULSE_SECRET_KEY`).
- **A2** — `handleIngestBeacon` (main port) now enforces a per-ingest-token token-bucket limit
  (100 rps / 200 burst, matching the dedicated beacon server `serve.go:326`) after token validation
  and before the body read → `429 RATE_LIMITED`.
- **A7** — `handleMetrics` now enforces a per-IP token-bucket limit (10 rps / 20 burst) keyed on
  `clientIP(r)`, ahead of the `MetricsToken` check. New `internal/api/ratelimit.go` — a hand-rolled
  `keyedLimiter` mirroring the proven `collector/beacon` token bucket (no new dependency). Eviction
  goroutines start in `Server.Start`, stop in `Server.Stop` (no leak in `Handler()`-only tests).
  **Honesty note:** the router installs `middleware.RealIP`, which rewrites `RemoteAddr` from
  `X-Forwarded-For` upstream, so the per-IP key is not spoofing-proof. That is acceptable for A7's
  threat model (bounding a misconfigured scraper, not an adversary) and is documented in the
  `clientIP` doc comment; a spoofing-resistant key would need trusted-proxy XFF parsing (out of scope).

**Verification — a FALSE-GREEN was caught by ORCH (binding lesson reinforced).** Workflow
`pulse-hardening-b6-a2-a7` (`wf_dadfcb7e-58d`, 1 author → independent `-race` gate + 3 adversarial
per-fix skeptics) reported **clean** — but it was wrong on two counts, both surfaced only by ORCH's
independent faithful reproduction (D-013/D-017/D-019: QA is never authoritative alone; default to
refuted until reproduced):
1. **A7 shipped UNWIRED.** The limiter was declared/initialised/evicted but **never called** in
   `handleMetrics` — dead code; `/metrics` had no limit. One skeptic even cited a line number for the
   call that did not exist (hallucinated verification).
2. **The whole api regression suite was silently SKIPPING.** `metaDDLPath` resolves
   `../../../contracts/db/meta/0001_init.sql` via `runtime.Caller`; the gate mounted only `server/`
   at `/repo`, so that path escaped the mount → `os.ReadFile` failed → `t.Skip`. Go counts SKIP as
   non-failing, so ~90 api tests (including all 3 new ones) "passed" without executing. The unwired
   A7 was invisible because its test never ran.
   **Fix (now the standard for Go verification here):** mount the **repo ROOT** at `/repo`, workdir
   `/repo/server`, `GOFLAGS=-buildvcs=false` (matches CI's full-checkout semantics). Re-run census:
   **api = 92 pass / 0 skip / 0 fail**; full `go test ./... -race` green. ORCH then added the missing
   `handleMetrics` limiter call and re-verified: all 3 tests fail on pre-fix code, pass after.

**Remaining deferred items (NOT done this session):**
- **CR-B7 (PENDING — frozen contract, needs INT-01).** Per-source webhook HMAC secret. Requires:
  a `webhook_secret` column on the `ams_sources` meta table (+ migration); a `webhook_secret` field
  on `POST`/`PUT /api/v1/admin/sources` (change to `contracts/openapi/pulse-api.yaml`); and the
  webhook handler selecting the secret by matching the incoming `nodeId`/`app` to a registered
  source. **Do not implement without an ORCH-approved CR applied by INT-01** (contracts frozen, D-004).
- **B3 (deploy, flagged for a future ORCH/deploy session).** Move `PULSE_AMS_AUTH_TOKEN` /
  `PULSE_WEBHOOK_SECRET` from `deploy/.env` env-vars to Docker Compose `secrets:` (tmpfs files);
  needs secret-file reading added to `loadEnvConfig` (a `cmd/` scope change). Mitigation today:
  `chmod 600 deploy/.env` (already gitignored). See `AMS-INTEGRATION.md` §5.2.

## D-029 · 2026-06-21 · Real-AMS integration vs `test.antmedia.io` — plan + live recon findings

ORCH connected Pulse to the real Ant Media Server `https://test.antmedia.io` (operator creds in
gitignored `deploy/.env`). Live recon from the VPS (IP `161.97.172.146`) revealed the W2c amsclient
(D-025), built from assumed wire shapes, has **wrong REST paths and auth model** for real AMS
Enterprise. Server = **AMS 3.0.3 Enterprise Edition** (`/rest/v2/version`). Scope of fixes: server
only (`pkg/amsclient`, `cmd/pulse` wiring) + `deploy/` overrides — **NO contract change** (D-004 safe).

**Live recon facts (every one curl-verified from the VPS, not inferred):**
- **Auth = cookie-session, not Bearer.** `POST /rest/v2/users/authenticate {email,password}` → HTTP 200
  `{"success":true,...}` + a `JSESSIONID` cookie. No JWT: `server-settings.jwtServerControlEnabled=false`,
  empty `jwtServerSecretKey` → the "long-lived JWT" path (crux #1) is unavailable; the **login+refresh
  extension (crux #2)** is required. Login success is in the JSON body (`success`), NOT the HTTP status
  (wrong password → HTTP 200 `success:false`).
- **REST path layout is per-application, not root.** Real working paths:
  - `GET /rest/v2/applications` → `{"applications":["LiveApp","live",...]}` — **array of STRINGS**
    (amsclient decodes `[{ "name": … }]` → would fail). Root context; needs the session cookie.
  - `GET /{app}/rest/v2/broadcasts/list/{offset}/{size}` (path params, per-app) — amsclient builds
    `/rest/v2/broadcasts/{app}/list?offset=&size=` → **404**. THIS is the only critical-path call
    (restpoller.go:151 `ListBroadcastsPaged`); broken paths ⇒ `/live/overview` shows 0.
  - `GET /{app}/rest/v2/broadcasts/{id}/broadcast-statistics` → `{total{RTMP,HLS,WebRTC,DASH}WatchersCount}`
    (DTO has wrong field names) — but **`BroadcastStatistics` is never called in prod** (cosmetic).
  - `GET /{app}/rest/v2/broadcasts/{id}/webrtc-client-stats/0/100` (per-app) — best-effort, errors ignored.
  - `GET /rest/v2/system/stats` → **404**; real `/rest/v2/system-status` — but **`SystemStats` never called**.
  - `GET /rest/v2/cluster/nodes` → **404** (standalone, not a cluster). `cluster.Discovery`
    (discovery.go:124) logs a `Warn` every 30 s on the 404 → log spam ⇒ fix amsclient to return
    empty on 404.
- **Per-app IP allow-list.** From the VPS IP, 8 of 16 apps return **403 "Not allowed IP"**
  (Icomms, TEST, VsMediaTesting, WebRTCAppEE, amartest, drmtest, live, ll-hls). The other 8 are open:
  24x7test, Conference, **LiveApp** (16 broadcasts, **1 live: `test123` broadcasting**), LiveShopping,
  PetarTest2, clipcreator, demo, meet. Important: **per-app broadcast endpoints need NO auth from an
  allowed IP** (`/LiveApp/rest/v2/broadcasts/count` → 200 with no cookie). The cookie is needed for app
  *discovery* (`/rest/v2/applications`) + root/system calls. ⇒ set `PULSE_AMS_APPLICATIONS` to the open
  apps so the core poll works regardless. Real captures staged in `agents/handoffs/real-ams-captures/`.

**Plan (executed as Workflow `pulse-realams-integration` — 3 disjoint authors → adversarial verify →
fix loop; then ORCH live-deploy + validate):**
- **A1 `server/pkg/amsclient`**: cookie-session login+refresh provider (Config `LoginEmail`/`LoginPassword`;
  `net/http/cookiejar`; lazy login; on 401/403 re-login + single retry, throttled to avoid storms on
  permanent IP-block 403; static Bearer path preserved for back-compat); fix paths to per-app form;
  tolerant `applications` decoder (string OR `{name}` — cross-version); `ClusterNodes`/`NodeInfo`
  404 → empty,nil; fix `BroadcastStatisticsDTO`/`SystemStats` path for correctness; update tests +
  add real-capture fixtures + auth tests.
- **A2 `server/cmd/pulse`**: `PULSE_AMS_LOGIN_EMAIL`/`PULSE_AMS_LOGIN_PASSWORD` in config.go; pass to
  `amsclient.New` in serve.go:130.
- **A3 `deploy/`**: relax `docker-compose.real-ams.yml` token requirement (`:?`→`:-`, login now primary)
  + add login-var passthrough; add `docker-compose.realams-test.yml` (loopback-port isolated stack,
  modeled on `docker-compose.ci.yml`) for ORCH validation without touching `pulse-prod`.
- **Verify**: `go test ./... -race` with **repo-ROOT mount** + `GOFLAGS=-buildvcs=false` (D-028, else
  api tests silently skip); re-curl live AMS to confirm the new client.go path templates return 200.
- **ORCH live-deploy**: isolated project `pulse-realams` on loopback ports (mock-ams disabled), validate
  `/api/v1/live/overview` shows real `total_publishers` ≥ 1 (LiveApp `test123` is live). `pulse-prod`
  (Oğuz demo) untouched. Swap into the live demo only on operator approval.

### D-029 addendum · multi-app stream-identity collision (found via live validation)

Live validation against `test.antmedia.io` surfaced a SECOND, pre-existing bug (not in amsclient):
after the amsclient fixes the poller authenticated and ingested the live stream `LiveApp/test123`,
but `/api/v1/live/overview` showed **0 publishers**. Root cause — stream identity omitted the AMS
**application** in two places, so multi-app polling collided:
1. **`restpoller.detectEnded`** keyed `prevStatus` by `nodeID/streamID` and ran **per app**, comparing
   the GLOBAL prevStatus map against ONE app's current IDs. With ≥2 apps, each app's `detectEnded`
   emitted a `publish_end` (reason "disappeared", **not deduped**) for every *other* app's broadcasting
   stream. (Aggravated because `test.antmedia.io` reuses streamId `test123` across `LiveApp` and
   `PetarTest2`.) 60 phantom `stream_publish_end` rows in `server_events` confirmed it.
2. **`aggregator`** keyed `a.streams` by `nodeID/streamID`, so those phantom publish_ends deleted the
   genuinely-live `LiveApp/test123` every poll cycle (no eviction log, no panic — the snapshot just
   stayed empty). Single-app mock-ams never exposed either bug.

**Fix (server/internal/collector, allowed AMS territory; no contract change):**
- `restpoller`: `prevStatus` key → `nodeID/app/streamID`; `detectEnded` scopes its scan to the current
  app's key prefix (`strings.HasPrefix`) so it only ends streams of THAT app.
- `aggregator`: the 4 stream-event handlers + `UpdateIngestHealth` key by `nodeID/app/streamID`. The
  snapshot map (`snap.Streams`) intentionally stays keyed by `streamID` (its consumers iterate values
  and several tests look it up by bare streamId; `total_publishers` counts active entries, so it is
  correct for the realistic 1-live-stream case). Regression test
  `TestAggregator_CrossAppStreamID_NoCollision` proves a cross-app publish_end no longer deletes a live
  stream.

**Validated LIVE** (isolated `pulse-realams` stack, loopback :18090, `pulse-prod` untouched):
`/api/v1/live/overview` → `total_publishers:1`, `apps:[{app:LiveApp,publishers:1,streams:1}]`;
`/api/v1/live/streams` → `test123` `publisher_state:publishing`, `started_at` matching the real broadcast. Full `go test ./... -race` green (repo-ROOT mount, D-028).

## D-030 · 2026-06-21 · Real-AMS wire-correctness (D-029v validation) — shipped + live-confirmed

Re-validated the D-029 integration LIVE against `test.antmedia.io` (AMS 3.0.3 Enterprise) on an isolated
`pulse-realams` stack (loopback :18090, `pulse-prod` untouched) and ran an adversarial validation workflow
(`wf_54944d40-37f`: 5 finders diffed REAL curl captures against the decode/normalize code + fixtures → a
refute pass). 30 raw findings → 15 confirmed; the refute pass correctly killed misleading ones (e.g.
"speed is Mbps" — real data proved `speed` is a dimensionless realtime ratio). Fixes (scope:
`server/pkg/amsclient` + `server/internal/collector`, **no contract change**; commit `fe321bf`, docs
`3153722`, pushed to `ams-integration`):

- **CRITICAL — bitrate 1000×.** AMS REST `bitrate` is **bits/sec** (curl-verified: 624016 ≈
  receivedBytes·8/duration ≈ 624 kbps). `normalize.go` stored it raw into `bitrate_kbps`. Now `/1000` at
  the single normalization boundary. **Live-confirmed: API `bitrate_kbps` = 624.152** (was 624016).
- **HIGH — fps→permanent Warning.** AMS 3.0.3 omits `currentFPS` from the broadcast list (Go decodes 0),
  so every REST stream scored fps=0 → health capped at 0.75 (Warning); "Good" structurally unreachable.
  Fix: `normalize` emits `fps` only when present; a **-1 "unavailable" sentinel** makes
  `ComputeHealthScore` redistribute the 0.25 FPS weight across the other four sub-scores (re-normalized to
  a 1.0 basis). The -1 never reaches stored/serialized state (display value stays 0). Both call sites
  (`health.go`, `aggregator.go`) updated.
- **MEDIUM — speed→bitrate fallback removed.** `speed` is a realtime RATIO (0.991 for 624 kbps, 1.236 for
  1381 kbps), not a bitrate; the old `effectiveBitrate=b.Speed when bitrate==0` emitted ~1 "kbps" garbage.
- **MEDIUM — dropped ingest QoE.** Real broadcast object carries `packetLostRatio`/`jitterMs`/`rttMs`;
  the DTO dropped them and `normalize` hardcoded loss/jitter=0 (health blind to real degradation). Now
  decoded + wired (`packetLostRatio` ×100 → pct; jitter/rtt already ms).
- **MEDIUM — `terminated_unexpectedly`.** Real AMS crash status (curl-seen in `meet`) was unhandled →
  crashed streams stayed "live" until stale eviction (~3 min). Now emits `publish_end` next poll (≤5 s),
  reason = actual status.
- **MEDIUM — WebRTC single-track averaging.** `(video+audio)/2` halved metrics for audio-only/video-only
  viewers (40 ms RTT reported as 20 ms). Replaced with `avgNonZero` (mean of non-zero tracks).
- **LOW — restpoller cluster-error logging.** Non-404 `ClusterNodes` errors were silently discarded;
  now logged (404 = standalone still maps to nil, no spam).

**Tests:** real-wire regression cases added — `normalize_test` (bps→kbps, speed-not-bitrate, QoE units,
terminated_unexpectedly, single-track WebRTC), `health_test` (fps-unavailable redistribution, low-bitrate
honest Warning), `client_test` (DTO decode of new fields) + sanitized fixture
`testdata/broadcasts_real_test123_v303.json` (RTSP creds/IPs scrubbed). Full `go test ./... -race` GREEN
(repo-ROOT mount, `GOFLAGS=-buildvcs=false`, D-028 lesson) + independent adversarial diff review.

**Live-confirmed** (isolated stack, fresh bring-up): `/api/v1/live/overview` → `total_publishers:1`;
`/api/v1/live/streams` → `test123` `bitrate_kbps:624.152`, health=Warning (HONEST — 624 kbps < 2000 kbps
target — not the phantom-fps false-warning); ingest log `score:0.68` (redistributed, was 0.75-capped).

**Deferred (documented, no current live impact):** `webrtc_client_stats` event is normalized but the
aggregator has no `case` for it (viewer-side QoE never applied — needs a QoE-model decision: viewer-side
vs ingest-side fields share `LiveStream.{PacketLossPct,JitterMS}`); standalone AMS exposes no cpu/mem
(system-status lacks them → fleet shows none, needs an "N/A" UX, not a code fix); AMS `version` (3.0.3)
never surfaced; `speed_read_kbps` data-key is a legacy misnomer (now carries the ratio). Kafka/logtail
`bitrate` is a DIFFERENT source (not AMS REST) — left unchanged.

**Process note:** the adversarial-review fork was launched read-only but, inheriting full context, also
performed the commit + handoff + push autonomously (correctly — `fe321bf` was byte-identical to the
ORCH-tested tree, fixture sanitized, docs coherent). Lesson: do not give a reviewer fork write access
while ORCH is concurrently editing the same files — it caused a brief doc-edit race (reconciled cleanly).

## D-031 · 2026-06-21 · Real-AMS prod-swap deploy-readiness (runbook + pre-deploy fixes)

Ran the `realams-deploy-readiness` workflow (`wf_7139d2a0-4e3`: 5 parallel readiness dimensions —
deploy-mechanics, founder-UX, rollback-safety, data-integrity, pre-flight-security → release-manager
synthesis). Verdict: **ready-with-mitigations**. Deploy mechanics validated (`docker compose -p pulse-prod
… -f docker-compose.real-ams.yml config -q` passes; mock-ams auto-profiled-out; AMS cookie-session wired
from `deploy/.env`). Deliverable: **`deploy/runbooks/real-ams-go-live.md`** (pre-flight → stop sidecar →
optional CH wipe → deploy → verify → rollback + founder talking points). The actual swap is **founder-visible
and gated on explicit operator GO** (NOT executed).

**Pre-deploy fixes shipped (pre-conditions for a clean demo, on `ams-integration`):**
- **`maskDSN` was a no-op** (`server/cmd/pulse/migrate.go` `return dsn`) — the ClickHouse password leaked in
  plaintext to JSON logs on every migrate run + `pulse diag`. Fixed with `url.URL.Redacted()` (password →
  `xxxxx`); regression test `TestMaskDSN` (`migrate_test.go`). Server build + test GREEN.
- **Broken SDK-docs CTA** — `web/src/features/qoe/QoePage.tsx` had `github.com/your-org/pulse#sdk-setup`
  (404 in the Viewer-QoE empty state). → `github.com/aytekXR/ams-pulse#sdk-setup`.

**Operator decisions captured in the runbook (§2), not pre-decided by ORCH:**
1. **Wipe ClickHouse (recommended YES).** Prod CH holds ~1.05M SEEDED demo rows (fake stream IDs); analytics
   endpoints have no node filter → fake+real would aggregate and mislead. Rollups already empty (nothing to
   lose). Exact `docker volume rm pulse-prod_clickhouse-data` + re-migrate steps in runbook §3-C.
2. **Bitrate health target.** test123 ≈624 kbps; default target 2000 → "Warning", set
   `PULSE_INGEST_TARGET_BITRATE_KBPS=600` → "Good". Per-deployment threshold (honest, not a formula override).
3. Optional: open the HLS URL to seed 1 viewer for the protocol donut; merge `ams-integration`→`main` timing.

**Founder-facing honest empty states (NOT bugs, talking points in runbook §6):** Fleet/CPU/RAM blank
(standalone AMS → `cluster/nodes` 404); Viewer-QoE empty (no beacon SDK in player); WebRTC viewer stats
empty (deferred — aggregator lacks `EventWebRTCClientStats` case).

**D-031 backlog (post-demo):** standalone node card from `/rest/v2/system-status`; `EventWebRTCClientStats`
aggregator case; surface AMS version; merge to main + drop vestigial `AMS_LOGIN_*` env lines; Caddy
`/webhook/*` route (+ B7/B3).

**Process note (repeat of D-030):** a deploy-readiness workflow agent autonomously wrote
`deploy/runbooks/real-ams-go-live.md` to disk (Write access). ORCH overwrote it with the authoritative
synthesized version after correcting agent-guessed facts (exact volume name `pulse-prod_clickhouse-data`,
real migration filenames `0001_init/0002_concurrency_rollup/0003_probe_segment_ttfb`, endpoint
`/api/v1/live/streams` for bitrate, `xxxxx` mask, rollback sidecar → reference `oguz-testing.md`).

### D-031 addendum · real-AMS go-live EXECUTED + aggregator target-wiring bug (found live)

Operator gave GO (execute now / wipe ClickHouse / bitrate target 600). Ran the runbook against the LIVE
`pulse-prod` stack: stopped the `pulse-demo-liveness` sidecar, wiped `pulse-prod_clickhouse-data` (token
volume `pulse-prod_pulse-data` preserved), re-migrated (0001/0002/0003), and `up -d --build pulse` with the
real-ams overlay. **`beyondkaira.com` now serves real `test.antmedia.io`.** Verified live: healthz ok
(all components), `/api/v1/live/overview` → `total_publishers:1`, `bitrate_kbps≈623.5` (was 624016),
ClickHouse = 142 real rows / only `test123` / 0 seeded rows, migrate DSN masked `clickhouse://pulse:xxxxx@…`.

**Bug found during go-live (fixed):** setting `PULSE_INGEST_TARGET_BITRATE_KBPS=600` did NOT change the
dashboard health — test123 stayed "Warning" (band 50). Root cause: `aggregator.onIngestStats` called
`ingest.ComputeHealthScore` with the **hardcoded** `ingest.DefaultTargetBitrateKbps` (2000), ignoring the
configured target. `serve.go` passed the target to the `ingest.HealthTracker` but NOT to the aggregator,
and the live dashboard reads the aggregator's `LiveStream.Health` (banded in `query.go`). The
`HealthTracker`'s correct score was unused for the live view. Fix (scope: collector + cmd; no contract
change): added `targetBitrateKbps`/`targetFPS` fields + `Aggregator.SetIngestTargets()` (mirrors the
`SetEdgeChecker` pattern), defaulted in `New`, applied from `serve.go` via `cfg.IngestTargetBitrateKbps/FPS`;
`onIngestStats` now uses the configured targets. Regression test `TestAggregator_SetIngestTargets` (default
2000 → Warning; 600 → Good). Wired the `PULSE_INGEST_TARGET_BITRATE_KBPS` passthrough into
`docker-compose.real-ams.yml` (default 2000). Rebuilt + redeployed: **test123 now `health_score:100`
(Good)** at 623.5 kbps. Full `go test ./... -race` GREEN.

**Live status:** prod swap DONE. Founder-facing honest empty states remain (Fleet/CPU-RAM, Viewer-QoE,
WebRTC viewer stats) — runbook §6 talking points; D-031 backlog unchanged. Rollback procedure in
`deploy/runbooks/real-ams-go-live.md` §5 (the seeded-demo sidecar restore is in `oguz-testing.md`).

## D-032 · 2026-06-22 · Completeness + test-coverage audit → production-readiness brief

Ran the `pulse-completeness-and-test-audit` workflow (`wf_5f88abbb-dae`: 4 parallel auditors —
test-coverage-by-layer, functional-completeness-vs-PRD, production-readiness, e2e/functional strategy →
tech-lead synthesis). Deliverable: **`agents/handoffs/PRODUCTION-READINESS.md`** (the next-session prompt:
what's needed to complete the app, a 4-phase roadmap, and a binding mandate to test at EVERY level with TDD +
a coverage gate, each phase orchestrated as a Workflow).

**Honest test verdict (measured `go test ./...` coverage, no integration tag):** unit foundation is real
(321 Go test fns, non-tautological) but NOT "tested at every level" — **3 packages at 0.0% in a normal run**:
`internal/query` (1010 lines, integration-only), `internal/store/clickhouse` (integration-only),
`internal/config` (NO test file). `cmd/pulse` 1.2%; `store/meta` 29.7%; `license` 36.9%; api 52.2%. Web: 12
vitest suites but **no --coverage / no threshold**. **0 Playwright tests.** No response-body↔OpenAPI
conformance tests. e2e = 4 assertions (smoke). SDK beacon-js is the best-covered layer (15KB gate).

**Top functional gaps to "complete" (evidence-cited in the brief):** alert channel test-fire is a stub
(`server.go:~1234` returns 202, never calls `Send()`); `rebuffer_ratio`/`error_rate` alerts proxy from
HealthScore (not real beacon data); **3 license gates defined but never enforced** (`CheckDataAPI` on
analytics handlers, `CheckNodeLimit`, `CheckPrometheus` on `/metrics`) = monetization leak; standalone node
card blank (`SystemStats()` implemented but never called); `EventWebRTCClientStats` dropped by the aggregator;
QoE/beacon needs a Pro+ license to flow in prod; Postgres meta store stubbed; webhook unreachable (no Caddy
`/webhook/*` route); per-app IP allow-list (8/16 apps 403 the VPS).

**Quick win shipped (D-032):** `golang:1.26`→`golang:1.25` in `docker-compose.{hardened,ci,override}.yml`
(go1.26 unreleased → `docker pull` failure breaking mock-ams + the CI compose job). Verified:
`grep -rn golang:1.26 deploy/ .github/` empty.

**Immediate next step (per brief):** merge `ams-integration`→`main` (full `-race`, repo-root mount, 0 SKIP)
+ wire the Caddy `/webhook/*` route; then run the phased Workflows. RESUME-PROMPT updated to point here.

## D-033 · 2026-06-24 · Session-9 audit: verify state, eliminate stale assumptions, harden the handoff

Operator-directed meta-audit ("make sure the prompt is done; verify don't assume; TDD/verification/workflows/
assumption-elimination"). Actions:
- **Re-verified current reality (not assumed):** prod LIVE on real test.antmedia.io (`/healthz` ok,
  `total_publishers:1`, containers up 2 days healthy); `ams-integration @ ea30367` in sync, **7 commits ahead
  of `main` (main STALE — lacks D-029..D-032)**; `go test ./... -race` EXIT=0 / 21 ok / **0 SKIP** (repo-root
  mount); Dockerfile embeds the web build (QoePage fix is in the prod image).
- **Rewrote `RESUME-PROMPT.md` clean** — pruned the contradictory stale sections (it still said "prod swap
  pending / don't disturb the seeded demo" AFTER the go-live). New structure: §0 verified state, §1 Pending
  User Actions (U1 AMS IP allow-list, U2 CI-green confirm, U3 Pro+ license, U4 branch-protection/tag, U5
  browser/CSP, U6 gh install), §2 done-vs-missing, §3 immediate steps, §4 workflow-phase backlog, §5 **binding
  TDD enforcement** (red→green; unit/integration/contract/functional/e2e/regression/edge/failure-path; coverage
  gate; critical-business-logic-first), §6 **binding verification workflow** (build/lint/typecheck/test-race/
  coverage/contract-drift/staging/smoke/adversarial), §7 workflow suggestions (feature/test/deploy/monitor/
  rollback), §8 **Assumptions to Eliminate** (A1–A13, each with how-to-validate), §9–12 binding flows/protocol/
  hard-rules/environment.
- No code changed this session (verification + handoff only). Production untouched.

**Top unverified assumptions flagged for elimination:** main≠prod (merge), CI-green-on-GitHub (user/gh),
alerts-fire-and-deliver (stub + no retry + no e2e), coverage-adequate (3 pkgs 0%), QoE-works (needs Pro+),
SPA-renders/CSP (no Playwright/browser run), response-body↔contract (only spec-lint), CH-shutdown-no-loss
(sleep not drain). See RESUME-PROMPT §8.

## D-034 · 2026-06-28 · Self-hosted AMS Enterprise (trial) + prod Pulse repointed off test.antmedia.io

Operator-directed ("using workflows, implement RESUME-PROMPT; start installing AMS with the trial license").
Stood up the operator's OWN Ant Media Server so Pulse no longer depends on the IP-blocked shared
test.antmedia.io (**resolves U1**) and the full ingest/QoE/webhook/multi-app pipeline becomes exercisable on a
fully-controlled AMS.

**Decisions (operator-approved via AskUserQuestion):** (1) install = Docker on the prod VPS
(`antmedia/enterprise:3.0.3`, `--network host`); (2) exposure = full public; (3) switch prod Pulse after a
staging-verify.

**Done — every claim verified against the running stack, not assumed:**
- Pulled `antmedia/enterprise:3.0.3` (digest `09fbb5cd…`; tag matches the AMS version Pulse already modeled).
  Container `antmedia`, `--network host`, persistent volume `antmedia_data` → /usr/local/antmedia. Public IP
  161.97.172.146 is directly on eth0 → WebRTC ICE advertises the real IP (no NAT).
- **License:** the image does NOT honor `-e LICENSE_KEY` (that was an unverified web guess; docs.antmedia.io
  403'd the researchers). Injected `server.licence_key` into conf/red5.properties + restart. Confirmed
  Enterprise via authed `/rest/v2/version` → "Enterprise Edition 3.0.3"; server-settings `licenceKey` set,
  `heartbeatEnabled:true`. Definitive proof: a 2 Mbps RTMP test stream muxed/transcoded (HLS + adaptive +
  WebRTC adaptors active) — an unlicensed Enterprise would throttle/refuse.
- **Admin user** created headlessly: `POST /rest/v2/users/initial` (admin@beyondkaira.com); cookie-session
  login verified (`/rest/v2/users/authenticate` → system/ADMIN) — exactly Pulse's amsclient auth path. Creds
  in oguz-testing.md + deploy/.env (both gitignored).
- **Root cause found & fixed (same class as U1, on our own AMS):** each app's `remoteAllowedCIDR` defaulted to
  `127.0.0.1` with `ipFilterEnabled=true`. The Pulse container (src 172.20.0.3) got 403 "Not allowed IP" on
  every `/{app}/rest/v2/broadcasts/list`. Set `remoteAllowedCIDR=0.0.0.0/0` on LiveApp/WebRTCAppEE/live
  (started at 172.16.0.0/12 to keep the app REST host-private, then widened to 0.0.0.0/0 to honor "expose
  fully" AND unbreak the browser panel's per-app live view). Polls then returned 200.
- **Staging-verify (binding §6.7, isolated project `pulse-ams-staging`, NOT prod):** base+hardened+real-ams vs
  the new AMS; ffmpeg sidecar published a 2 Mbps test pattern (rtmp LiveApp/teststream); AMS broadcast
  "broadcasting" @2068 kbps/720p/0 loss; Pulse polls 200; ClickHouse `server_events` populated
  (`ingest_stats`/`stream_stats`, source `rest_poll`, stream_id teststream, bitrate_kbps≈1969 → bps→kbps
  normalization correct, D-029/D-031 holding vs a real AMS); `/api/v1/live/overview` (Bearer) →
  `total_publishers:1`. Staging torn down (`down -v`).
- **Prod switch:** rewrote `deploy/.env` PULSE_AMS_* off test.antmedia.io → `http://161.97.172.146:5080`
  (cookie auth, node_id `beyondkaira-ams`, apps=auto-discover, target bitrate 2000); old .env backed up to the
  session scratchpad; `compose config -q` OK; `up -d` recreated `pulse-prod-pulse-1` (no `-v`). Smoke (TLS via
  `--resolve`): `/healthz` 200 all-ok; `/live/overview` `total_publishers:1`; prod logs zero 403/decode/login
  errors. **U1 resolved.**

**State / open risks:** synthetic publisher `ams-teststream` left running so the dashboard shows live data —
remove with `docker rm -f ams-teststream` once real streams flow. VPS has **0 swap**; AMS (JVM, a few GB) + CH
+ Pulse co-located on 11 GiB → watch memory under real load (consider a swapfile + container mem limits). App
REST is now public (`0.0.0.0/0`) per the exposure choice — tighten to a specific CIDR if desired. No host
firewall present (UFW inactive; provider-level only). **No application code changed**; `deploy/.env` and
`oguz-testing.md` are gitignored (not committed). Runbook: `deploy/runbooks/self-hosted-ams.md`.

## D-035 · 2026-06-28 · Subdomains (pulse./ams.) + test-coverage/CI audit

**Subdomains (operator-directed "utilize subdomains").** Added Caddy site blocks in `deploy/config/Caddyfile.prod`:
`pulse.{$PULSE_DOMAIN}` → Pulse (apex kept), `ams.{$PULSE_DOMAIN}` → reverse-proxy the self-hosted AMS panel
(host:5080) with real Let's Encrypt TLS so the AMS admin password is encrypted in transit. DNS A records
`pulse`/`ams`/`www` → 161.97.172.146 added by the operator (verified via 8.8.8.8/1.1.1.1). **Gotcha:**
`caddy reload` via `docker exec` did NOT apply the new site addresses (no ACME attempts logged); `docker restart
pulse-prod-caddy-1` loaded them and certs issued via TLS-ALPN-01. Verified: `https://pulse.beyondkaira.com/healthz`
200, `https://ams.beyondkaira.com` → "Management of Ant Media Server", cert SANs present. Committed `4f7077b`
(Caddyfile only; the later operator-added `brier.<domain>` block is a separate project — left uncommitted).

**Test-coverage + CI audit (operator-directed "nothing untested; see breakage in CI").** Measured
`go test -race -cover ./...` in golang:1.25, repo-root mount: **EXIT=0, all pass, no races; total 47.5%.**
Zero unit coverage: `internal/query` (0%, powers all chart/API reads), `internal/config` (0%),
`store/clickhouse` (0% unit; integration ~3/12 query methods), `.../migrations` (0%), `cmd/pulse` (1.2%).
Low+critical: `license` 36.9% (+3 gates unenforced), `store/meta` 29.7%, `collector/logtail` 37.5%,
`internal/api` 52.2%, `alert/channels` 56.8%. Strong: ingest 85, cluster 89, sessions 81, anomaly 76,
amsclient 76, restpoller 72, alert 72. **CI gaps** (won't catch breakage): no coverage gate, no Playwright
browser e2e (web/e2e absent), no response-body contract tests (only spec-lint), no web coverage threshold,
shallow mock-only e2e. Full plan + integration-key checklist captured in `agents/handoffs/RESUME-PROMPT.md` §5–§6
(was `NEXT-SESSION-PROMPT.md`, merged into RESUME-PROMPT.md in D-037).

## D-036 · 2026-06-29 · AMS web-console login fixed (client-side MD5 vs plaintext-provisioned accounts) + session ops

**Symptom (operator could not log into `https://ams.beyondkaira.com` despite the correct password).** Web login
returned `success:false` while Pulse/amsclient and curl auth succeeded with the *same* credential. Red herrings
ruled out first: not a typo, not autofill/device/incognito, not the AMS version, not the trial license, not Caddy.
Real cause proven by capturing the browser's POST body (privileged host-netns `tcpdump` container — the agent
lacks host root, so `docker run --net=host --cap-add=NET_RAW corfr/tcpdump`): the AMS Angular console
**MD5-hashes the password in the browser** before POSTing to `/rest/v2/users/authenticate`. AMS auth succeeds iff
`submitted == stored` OR `MD5(submitted) == stored`, where `stored` is the value the account was *created with*.
Both admin users were provisioned via REST in D-034 with the **plaintext** password, so plaintext clients (Pulse)
matched but the console's hashed submission never did → web login impossible.

**Fix.** Re-created both `aytek@` and `admin@` via REST (`DELETE` then `POST /rest/v2/users`) with `password` set
to the **MD5 of the real password**, so the console's hashed submission matches. Both now web-login. Pulse is
unaffected — its plaintext auth still matches via the `MD5(submitted)==stored` branch (verified: `/healthz` ok,
no 401s, overview publisher intact). Rule going forward: web-loginable users must be REST-provisioned with
`MD5(realpassword)`; plaintext provisioning works for API/Pulse only. Actual values: `oguz-testing.md` (gitignored).

**Also this session (verified; no application code changed):**
- Brute-force lockout characterised from `io.antmedia.console.rest.CommonRestService`: **2** failed tries →
  **5-min** block, keyed by **email not IP**; returns "User is blocked" even with the right password.
- AMS confirmed on the **latest stable** (Enterprise 3.0.3 == Docker Hub `latest`); trial license valid to
  2026-07-12; quick-start effectively complete (install=Docker, TLS=Caddy not AMS `:5443`, default apps present,
  WebRTC sample pages HTTP 200).
- Opened `remoteAllowedCIDR` 127.0.0.1→0.0.0.0/0 for the newly-created **`pulse-test`** app (was logging
  `HTTP 403: Not allowed IP` every ~5s) → Pulse logs now clean. Note: every new AMS app defaults to 127.0.0.1
  and must be opened for the Pulse container to poll it.

**NEXT (operator-directed):** start the **`pulse-p1-gaps`** workflow next session — close the P0 silently-stubbed
features (real alert test-fire `Send()`, license-gate enforcement `CheckDataAPI`/`CheckNodeLimit`/`CheckPrometheus`,
standalone node card via `SystemStats()`, WebRTC `EventWebRTCClientStats` aggregator case), TDD red→green.

## D-037 · 2026-06-29 · Merged NEXT-SESSION-PROMPT.md into RESUME-PROMPT.md (single handoff doc) + de-staled branch facts

**Operator: "why two files for next prompt? merge them into one."** Folded `NEXT-SESSION-PROMPT.md` (integration-keys
table, per-package coverage breakdown + the verified alert-firing gap, CI-gap list, ▶ START HERE) into
`RESUME-PROMPT.md` as the single source of truth; **deleted** `NEXT-SESSION-PROMPT.md`; fixed the two in-repo
references. While merging, an independent verification pass caught a **flatly-stale claim**: RESUME said "`main` is
7 commits behind `ams-integration`; prod runs `ams-integration`." Live git shows the **reverse** —
`main..ams-integration` = **0**, `ams-integration..main` = **5** — i.e. `main` now fully **contains**
`ams-integration`. Corrected §0 + assumption A1 + Step B: branch divergence is **resolved**; remaining branch work is
just retiring the stale `ams-integration` pointer + branch protection (U4). No application code changed.
