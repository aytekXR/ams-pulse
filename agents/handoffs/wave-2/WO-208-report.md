# WO-208 Completion Report — Wave 2 Documentation (DOC-01)

**Agent:** DOC-01
**Date:** 2026-06-14
**Work order:** WO-208 (issued by ORCH-00 2026-06-12)

---

## Status: DONE

All acceptance criteria verified. All documented commands tested against the wave-2 build.

---

## Acceptance criteria — verified

### Commands tested

| Command | Result |
|---|---|
| `CGO_ENABLED=0 go build -o /tmp/pulse ./cmd/pulse/` | BUILD OK |
| `CGO_ENABLED=0 go build ./...` | BUILD OK |
| `CGO_ENABLED=0 go test ./... -timeout 120s` | 15 packages PASS, 0 FAIL |
| `cd web && npm run lint` | 0 errors |
| `cd web && npm run test` | 58/58 PASS |
| `cd sdk/beacon-js && npm run size` | 3.44 kB (budget 15 kB) |
| `/tmp/pulse diag --reconcile` | Executes; exits nonzero on no CH (correct behavior) |
| `make helm-lint` | 1 chart linted, 0 failed |

### Doc defects from QA gate addressed

QA-01 gate report (WO-207) had no explicit doc defects filed. The gate report itself
was used as the source of truth for measured numbers and feature status.

### No documented-but-unimplemented behavior

All "Roadmap (Wave 2)" labels in the Wave-1 docs have been updated:
- Items shipped in Wave 2: labeled "Shipped (Wave 2)"
- Items not yet implemented: relabeled "Phase-3 roadmap"

### Feature status table correct

`README.md` and `docs/ARCHITECTURE.md` feature status tables flip F2 (full 13-month
query verified 126 ms), F3 (SDK 3.44 KB), F4 (ingest health 250.8 µs), F6 (reports
±1% drift 0.0000%), F7 (fleet ≤30 s), F8 (Prometheus 5 metrics) to **Shipped**.

---

## Work items — completion status

### 1. `docs/guides/beacon-sdk.md` (NEW)

Full integration walkthrough covering:
- Token setup via Settings UI + API
- AMS WebRTC (`attachWebRTC`)
- hls.js (`attachHls` + `attachVideoElement` dual-layer)
- video.js (via underlying HTMLVideoElement)
- Plain `<video>` element (native HLS, Safari)
- Full event mapping table (all 16 trigger→event→data mappings)
- Sampling (`sampleRate`), privacy/IP anonymization guidance
- Transport behavior table
- Configuration reference

### 2. `docs/guides/prometheus.md` (NEW)

Covers:
- Enabling the scrape endpoint via `PULSE_METRICS_TOKEN`
- Prometheus scrape config (unauthenticated + authorized + ServiceMonitor)
- Metric reference table: 5 metrics (`pulse_live_viewers`, `pulse_live_streams`,
  `pulse_live_publishers`, `pulse_ingest_bitrate_kbps`, `pulse_alerts_firing`)
- Grafana starter dashboard JSON (6 panels: 4 stat + 2 timeseries)
- Useful PromQL expressions
- Helm installation with scrape token (secretRef pattern)

### 3. `docs/runbooks/install.md` (UPDATED)

- Added Path C: Helm section with prerequisite, create-secret, install, migrate,
  UI access, and upgrade steps; geo enrichment (mmdb) walkthrough; D-002 label
- Documented GAP-206-02 (postgres Secret must be created manually)
- Updated env var table: added all wave-2 vars (PULSE_INGEST_LISTEN_ADDR,
  PULSE_METRICS_TOKEN, PULSE_ANONYMIZE_IP, PULSE_GEO_MMDB_PATH, PULSE_KAFKA_BROKERS,
  PULSE_KAFKA_GROUP_ID, PULSE_SESSION_IDLE_TIMEOUT, PULSE_CLUSTER_DISCOVERY_INTERVAL,
  PULSE_INGEST_TARGET_BITRATE_KBPS, PULSE_INGEST_TARGET_FPS, PULSE_REPORTS_DIR,
  PULSE_S3_* family)
- Updated YAML config schema snippet with beacon + geo + ingest_listen sections
- Updated tier table to Free/Pro/Enterprise (per license.go; corrected "Business")
- Added `pulse diag --reconcile` to diagnostic commands

### 4. `docs/runbooks/reports.md` (NEW)

Covers:
- Tenant mapping: meta-tag vs glob precedence, glob semantics, examples, API
- Egress estimation method disclosure: `bitrate_x_watch_time` formula
- Schedule setup: 3-field cron format, presets, API example
- S3 upload: direct env vars and indirect reference pattern
- S3-compatible endpoints (MinIO, DigitalOcean Spaces)
- White-label config (Phase-3 roadmap for the endpoint)
- Reconciliation: `pulse diag --reconcile`, formula, when to run
- On-demand report generation via API (CSV + PDF)
- Known limitations table (D-W2-002, tenant CRUD CR-WO204-01, etc.)

### 5. `docs/runbooks/alerting.md` (UPDATED)

- Updated supported metrics table: added `rebuffer_ratio`, `error_rate`,
  `ingest_bitrate_floor`, `node_down`, `node_degraded`, `cert_expiry`
- Added Telegram channel section (Pro+) with API config shape
- Added PagerDuty channel section (Enterprise) with API config shape
- Added Webhook channel section (Enterprise) with HMAC verification snippet
  (Python + Go examples with constant-time comparison)
- Updated maintenance windows: added cron-expression maintenance window docs
  (Wave-2 `maintenance_window.cron_expr` + `duration_s` field)
- Updated default rule pack: seeded automatically on first run (Wave 2),
  all 4 rules listed with muted=true explanation
- Fixed known issues table: removed shipped items, kept only genuine gaps
- Relabeled all "Roadmap (Wave 2)" to "Phase-3 roadmap"

### 6. `docs/ARCHITECTURE.md` + `README.md` (UPDATED)

**ARCHITECTURE.md:**
- Component diagram updated: added ingest listener (:8091), session stitcher,
  ingest health tracker, cluster discovery (F7), scheduler
- Implementation status table replaced with wave-2 complete table (all components)
- Performance budgets table updated with wave-2 measured numbers
- Added Section 10: Ingest health score formula with complete formula, weights,
  classification thresholds, drop detection thresholds, budget reference
- Added wave-2 defect table (D-W2-001/002/003) and gap table (GAP-2-001..GAP-206-03)
- Removed stale "Stub; Wave 2" entries; all components now show shipped status

**README.md:**
- Feature status table flipped: F2 full (126 ms), F3 (3.44 KB), F4 (250 µs),
  F5 (Telegram/PD/Webhook channels), F6 (±1% reconciliation), F7 (≤30 s), F8 (5 metrics)
- System overview diagram updated with ingest listener, session stitcher, health tracker,
  fleet discovery, scheduler, S3 uploader
- Documentation table expanded: added reports.md, beacon-sdk.md, prometheus.md,
  Helm README link
- Wave-2 test count updated: 58 tests (was 21)
- Roadmap updated: Wave 2 marked complete with measured numbers

### 7. ADRs (NEW)

**ADR 0005** (`docs/adr/0005-beacon-ingest-hardening.md`):
Beacon ingest security posture — 7-layer defense: constant-time token auth,
SHA-256 at rest, per-token rate limit (100/s burst 200), 64 KB body cap,
schema validation, async write, CORS. Rationale for each layer documented.

**ADR 0006** (`docs/adr/0006-kafka-client-kafka-go.md`):
kafka-go chosen over franz-go for the Kafka consumer. Decision matrix: CGO-free
(both), API simplicity (kafka-go wins for simple consumer group), dependency surface
(kafka-go wins), AMS message format fit (JSON-only; franz-go features unused).

---

## Measured numbers

| Item | Measured | Budget | Source |
|---|---|---|---|
| Server build | PASS | required | `CGO_ENABLED=0 go build ./...` |
| Server tests | 15 pkgs PASS | required | `go test ./...` |
| Web tests | 58/58 PASS | required | `npm run test` |
| SDK size | 3.44 kB gzip | < 15 kB | `npm run size` |
| 13-month query | 126 ms | < 3 s | QA gate C-W2-08 |
| Ingest degradation | 250.8 µs | ≤ 15 s | QA gate C-W2-06 |
| Billing reconciliation | 0.0000% drift | ≤ 1.0% | QA gate C-W2-05 |
| Statement generation | 4.8 ms | < 60 s | WO-204 report |
| Node discovery | 24.4 ms | ≤ 2 min | QA gate C-W2-07 |

---

## Files authored

| File | Type | Description |
|---|---|---|
| `docs/guides/beacon-sdk.md` | New | SDK integration guide (all players, sampling, privacy) |
| `docs/guides/prometheus.md` | New | Prometheus scrape config, metric reference, Grafana panels |
| `docs/runbooks/reports.md` | New | Usage reports: tenant mapping, egress, schedules, S3, reconcile |
| `docs/adr/0005-beacon-ingest-hardening.md` | New | ADR: 7-layer beacon ingest security posture |
| `docs/adr/0006-kafka-client-kafka-go.md` | New | ADR: kafka-go vs franz-go decision |
| `docs/runbooks/install.md` | Updated | Helm path, wave-2 env vars, mmdb note, tier table, pulse diag --reconcile |
| `docs/runbooks/alerting.md` | Updated | New channels (TG/PD/Webhook), new metrics, maintenance windows, default rule pack |
| `docs/ARCHITECTURE.md` | Updated | Component diagram, status table, budgets, health formula, defects |
| `README.md` | Updated | Feature status, system diagram, doc index, test counts, roadmap |

---

## Gaps / Change Requests

### CR-WO208-01: Tenant CRUD not in OpenAPI spec (inherited from WO-204 CR-WO204-01)

`/api/v1/admin/tenants` routes are implemented (WO-204) but not in the frozen
`contracts/openapi/pulse-api.yaml` (D-004). Documented in `reports.md` with a note.
Recommend unfreezing contracts in Wave 3 to add tenant CRUD operations.

### CR-WO208-02: White-label `GET/PUT /api/v1/admin/whitelabel` endpoint deferred

The white-label PDF header config is a Phase-3 item (CR-2 from WO-205). Documented
as "Phase-3 roadmap" in `reports.md`. No action needed until Wave 3 contracts are updated.

### Defect note: D-W2-002

The live ClickHouse billing reconciliation path in `accounting.go` uses wrong column
names (QA gate defect D-W2-002, owner BE-02). Documented in `reports.md` Known
Limitations section. The reconciliation runbook correctly describes the expected output
for when the defect is fixed.
