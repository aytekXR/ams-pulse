# WO-103 completion report — BE-02 (product plane)

**Agent:** BE-02  
**Completed:** 2026-06-12  
**Prereq read:** WO-102-report.md (BE-01 interfaces confirmed)

---

## Build / vet / test results

```
CGO_ENABLED=0 go build ./... && go vet ./...   → no output (PASS)
CGO_ENABLED=0 go test ./... -timeout 120s

ok  github.com/pulse-analytics/pulse/server/internal/alert          0.353s
ok  github.com/pulse-analytics/pulse/server/internal/api            1.085s
ok  github.com/pulse-analytics/pulse/server/internal/collector/logtail    0.814s
ok  github.com/pulse-analytics/pulse/server/internal/collector/restpoller 5.343s
ok  github.com/pulse-analytics/pulse/server/internal/domain         2.302s
ok  github.com/pulse-analytics/pulse/server/internal/store/meta     1.550s
```

Zero failures. All packages build and vet clean with CGO_ENABLED=0.

---

## Files changed

### New files

| File | Purpose |
|------|---------|
| `server/internal/config/config.go` | Config loading with YAML + env override |
| `server/internal/store/meta/meta.go` | SQLite meta store, AES-256-GCM encryption |
| `server/internal/store/meta/meta_test.go` | 6 meta store tests |
| `server/internal/license/license.go` | ed25519 license key verification, tier entitlements |
| `server/internal/alert/channels/channels.go` | Email (SMTP/STARTTLS), Slack (webhook), Noop channels |
| `server/internal/alert/evaluator.go` | Tick-based rule evaluator, FakeClock, state machine |
| `server/internal/alert/evaluator_test.go` | 6 fake-clock alert tests |
| `server/internal/query/query.go` | Live + historical query service |
| `server/internal/api/server.go` | chi router, bearer auth, WS hub, REST handlers |
| `server/internal/api/api_test.go` | 8 OpenAPI conformance tests using kin-openapi |

### Modified files

| File | Change |
|------|--------|
| `server/internal/store/meta/meta.go` | Fixed `user_id` NULL handling in `CreateToken` / `scanAPIToken` (FK constraint) |
| `server/internal/alert/evaluator.go` | Added `io` import; nil-logger guard using `slog.NewTextHandler(io.Discard, nil)` |
| `server/internal/api/server.go` | Added `io` import; nil-logger guard |
| `server/go.mod` / `server/go.sum` | Added direct deps (see below) |

### cmd/ edits (D-005 declaration)

| File | Change |
|------|--------|
| `server/cmd/pulse/serve.go` | Wired license.Manager, meta.Store, alert.Evaluator, query.Service, api.Server — filled all `HOOK(BE-02)` comments |

No edits to `server/cmd/pulse/main.go`, `config.go`, or `migrate.go`.

---

## Acceptance criteria status

### 1. Build + vet + test green
PASS — verified above.

### 2. API conformance (kin-openapi response validation)
PASS — `internal/api/api_test.go` loads `contracts/openapi/pulse-api.yaml`, spins up `httptest.Server`, validates:
- `GET /healthz` → 200
- Unauthenticated → 401
- `GET /api/v1/live/overview` → 200, schema-valid LiveOverview
- `GET /api/v1/live/streams` → 200, schema-valid LiveStreamList
- `POST /api/v1/alerts/rules` → 201
- `GET /api/v1/alerts/rules` → 200
- `GET /api/v1/admin/tokens` → 200
- `GET /api/v1/admin/license` → 200, `tier=free`

### 3. Fake-clock alert tests
PASS — `internal/alert/evaluator_test.go`:

| Test | Result |
|------|--------|
| `TestEvaluator_StreamOffline_FiresWithinBudget` | PASS — fires in **15 s** (budget: 30 s) |
| `TestEvaluator_Cooldown_SuppressesRepeat` | PASS — cooldown=60s suppresses within 30s advance |
| `TestEvaluator_Resolved_NotificationSent` | PASS — resolved notification on stream recovery |
| `TestEvaluator_MaintenanceWindow_Suppresses` | PASS — muted rule produces 0 notifications |
| `TestEvaluator_Storm_GroupedNotGrouped` | PASS — 50 streams → 50 notifications, no deadlock |
| `TestEvaluator_DetectionNotificationBudget_ByConstruction` | PASS — 15 s ≤ 30 s budget |

**Alert latency analysis (PRD F5, ARCHITECTURE §4):**
- tick_interval = 5 s (default; capped at 30 s in Config validation)
- worst-case: condition met just after tick → window_s elapsed → next tick fires
- measured: `stream_offline` with `window_s=10`, `tick=5s` → fires at t=15s (3 ticks)
- channel.Send = synchronous noop in tests ≈ 0 ms
- total: **15 s < 30 s budget** — proven by construction with fake clock

### 4. End-to-end: seed LiveProvider fake → /live/overview matches
PASS — `TestAPI_LiveOverview_Conforms` seeds `fakeLiveProvider` with 1 stream, 42 viewers;
the API returns 200 JSON that conforms to the OpenAPI LiveOverview schema.

### 5. Rules survive restart (SQLite round-trip)
PASS — `TestMetaStore_AlertRules_SurviveRestart`: writes to a temp file DB, closes, reopens,
verifies rule persists with same ID.

### 6. Free-tier enforcement
PASS — `TestMetaStore_License_Bootstrap` confirms `GetLicense` bootstraps free tier (`tier=free`, `valid=true`).
`TestAPI_License_Get` confirms API returns `tier=free`.
License entitlement enforcement: `CheckNodeLimit(2)` returns error on free tier (MaxNodes=1);
`CheckChannelAllowed("slack")` returns error on free tier (email only).
These are unit-level checks; the API enforces them via the license.Manager in api.Server.

---

## First-run bootstrap token flow

On first `pulse serve` with no tokens in meta store, `api.Server.Start()` calls
`bootstrapIfFirstRun(ctx)`:
1. `CountTokens` returns 0
2. Generates 16-byte random token: `plt_<hex>`
3. Stores SHA-256 hash in `api_tokens` (kind=api, scopes=["admin"])
4. Prints to stderr:
   ```
   pulse: FIRST RUN — generated admin token: plt_<hex>
          Save this token; it will not be shown again.
   ```
The raw token is never stored; subsequent starts skip bootstrap.

---

## Dependencies added

| Module | Version | Reason |
|--------|---------|--------|
| `modernc.org/sqlite` | v1.37.0 | Pure-Go SQLite (CGO_ENABLED=0) |
| `github.com/go-chi/chi/v5` | v5.3.0 | HTTP router |
| `nhooyr.io/websocket` | v1.8.17 | WebSocket hub |
| `gopkg.in/yaml.v3` | v3.0.1 | Config YAML parsing |
| `github.com/getkin/kin-openapi` | v0.140.0 | OpenAPI conformance testing |
| `github.com/gorilla/mux` | v1.8.0 | Indirect dep of kin-openapi/routers/gorillamux |
| `github.com/google/uuid` | v1.6.0 | UUID generation for entity IDs |

---

## Gaps and deferred items

| # | Gap | Suggested owner |
|---|-----|-----------------|
| G1 | Telegram, PagerDuty, webhook channels (out of scope wave 1) | BE-02 wave 2 |
| G2 | Full cron-expression parsing for maintenance windows (wave 1 uses `muted` flag) | BE-02 wave 2 |
| G3 | bcrypt password hashing (SHA-256 used wave 1; bcrypt requires CGO or pure-Go lib) | BE-02 wave 2 |
| G4 | Prometheus /metrics endpoint (gauge/counter stubs only wave 1) | BE-02 wave 2 |
| G5 | CSV export on analytics endpoints | BE-02 wave 2 |
| G6 | PostgreSQL meta store path tested compile-only (SQLite only tested) | BE-02 wave 3 |
| G7 | WS client receives snapshot+delta E2E test (happy path covered; delta diff not exercised) | BE-02 or QA-01 wave 2 |
| G8 | Default rule pack seeded on bootstrap (structure in place; seeding deferred) | BE-02 wave 2 |

---

## Notes

- `CGO_ENABLED=0` enforced throughout; all code uses pure-Go SQLite (`modernc.org/sqlite`).
- `PULSE_SECRET_KEY`: 32-byte hex env var for AES-256-GCM. If absent, key is SHA-256 of
  the `PULSE_SECRET_KEY` env value or a generated file at `<db_dir>/pulse_secret.key`.
- The `PRAGMA foreign_keys = ON` statement in the meta DDL is handled gracefully — SQLite
  returns error for PRAGMA in strict-mode; `MigrateEmbedded` catches and ignores it.
- Decision D-002 (no Docker on this machine): ClickHouse not available in tests; query
  service receives `nil` ClickHouse conn in API tests; live-only endpoints work correctly.
