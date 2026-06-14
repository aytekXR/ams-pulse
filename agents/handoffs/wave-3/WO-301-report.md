# WO-301 Completion Report — Wave 3-MVP data plane: F10 probe runner + probe_results store

**Agent:** BE-01
**Date:** 2026-06-14
**Work order:** WO-301 (issued by ORCH-00 2026-06-14)

---

## Status: DONE

All acceptance criteria verified. All tests pass. Commit ready.

---

## Downstream Interface Signatures (authoritative — BE-02 builds against these)

### `domain.ProbeConfig` — probe configuration struct

```go
// Package: github.com/pulse-analytics/pulse/server/internal/domain

type ProbeConfig struct {
    ID        string // UUID primary key
    Name      string // human-readable label
    URL       string // stream URL to probe
    Protocol  string // hls | webrtc | rtmp | dash
    IntervalS int    // probe interval in seconds (default 60)
    TimeoutS  int    // per-check timeout in seconds (default 10)
    Enabled   bool   // only enabled probes are listed by ListEnabled
}
```

### `domain.ProbeResult` — probe execution result

```go
// Package: github.com/pulse-analytics/pulse/server/internal/domain

type ProbeResult struct {
    ID          string    // UUID for this result row (primary key in ClickHouse)
    ProbeID     string    // foreign key → ProbeConfig.ID
    TS          time.Time // when the probe ran (UTC)
    Success     bool      // true only on 2xx + parseable response
    TTFBMs      uint32    // time-to-first-byte in milliseconds
    ErrorCode   string    // "timeout" | "dns" | "http_4xx" | "http_5xx" | "parse" | "conn_refused" | "network" | "not_probed" | "read" | ""
    ErrorMsg    string    // human-readable detail; empty on success
    BitrateKbps float32   // estimated kbps = segment_bytes / segment_duration_s; 0 on failure
}
```

### `domain.ProbeConfigSource` — seam interface

```go
// Package: github.com/pulse-analytics/pulse/server/internal/domain

type ProbeConfigSource interface {
    // ListEnabled returns all probes where enabled = 1.
    // Called by the runner at the start of each scheduler tick (initially + every 60s).
    ListEnabled(ctx context.Context) ([]ProbeConfig, error)

    // RecordResult updates the probes.last_result_id, last_success, and
    // last_run_at denormalized fields after a probe check completes.
    // The full time-series result is written to ClickHouse by the runner itself.
    RecordResult(ctx context.Context, r ProbeResult) error
}
```

BE-02 implements `ProbeConfigSource` as `meta.MetaProbeConfigSource` (or similar) over the `probes` SQLite table. Wire it into the probe runner in `serve.go` per the HOOK(BE-02, WO-302) comment.

### `prober.ResultStore` — ClickHouse writer interface (internal to prober)

```go
// Package: github.com/pulse-analytics/pulse/server/internal/prober

type ResultStore interface {
    InsertProbeResult(ctx context.Context, r domain.ProbeResult) error
}
// Implemented by: *store/clickhouse.Store
```

### `store/clickhouse.Store` — new methods added

```go
// Package: github.com/pulse-analytics/pulse/server/internal/store/clickhouse

// InsertProbeResult writes a single probe result to ClickHouse probe_results.
// Called synchronously by the runner (one per probe per interval — no batching needed).
func (s *Store) InsertProbeResult(ctx context.Context, r domain.ProbeResult) error

// QueryProbeResults fetches probe results for a given probeID in the [from, to) time range,
// ordered by ts ASC, capped at limit rows. Used by BE-02's GET /probes/{id}/results handler.
func (s *Store) QueryProbeResults(ctx context.Context, probeID string, from, to time.Time, limit int) ([]domain.ProbeResult, error)
```

### `prober.Runner` — probe runner

```go
// Package: github.com/pulse-analytics/pulse/server/internal/prober

type Config struct {
    Workers           int     // concurrency pool size (default 4)
    MaxJitterFraction float64 // jitter fraction of interval (0 = no jitter; default -1 = 0.10)
    HTTPUserAgent     string  // User-Agent header (default "Pulse-Prober/1.0")
}

// Clock abstracts time for testing.
type Clock interface {
    Now() time.Time
    After(d time.Duration) <-chan time.Time
}

func New(cfg Config, source domain.ProbeConfigSource, store ResultStore, logger *slog.Logger, clock Clock) *Runner

// Run starts the probe scheduler. Blocks until ctx is cancelled.
// Returns ctx.Err() on normal shutdown.
func (r *Runner) Run(ctx context.Context) error
```

---

## Runner Design

### Architecture

```
Run(ctx)
├── ListEnabled() → probes
├── spawn per-probe scheduler goroutines (runProbeScheduler)
│   └── After(jitter) → After(interval+jitter) → ... → send probeExecRequest to execCh
├── worker pool goroutine (Workers=4 concurrently)
│   └── <-execCh → acquire sem → executeProbe(ctx, probe)
└── refreshTicker(60s) → ListEnabled() → add/cancel scheduler goroutines
```

### executeProbe flow

1. Obtain timeout context from `probe.TimeoutS` (default 10s).
2. Branch on `probe.Protocol`:
   - `hls` or empty → `probeHLS`: manifest + first segment.
   - `webrtc|rtmp|dash` → `probeReachability`: honest not-probed stub.
3. Call `store.InsertProbeResult` (ClickHouse time-series).
4. Call `source.RecordResult` (denorm last_* in meta probes table).

---

## Protocol Coverage Matrix

| Protocol | Coverage | Error Code on Limitation | Phase-3 Plan |
|----------|----------|--------------------------|--------------|
| HLS | **Full** — manifest parse + first segment fetch; TTFB, bitrate, parse errors, 4xx/5xx, timeout, DNS | n/a | — |
| RTMP | Minimal-honest — HTTP GET reachability, no playback | `not_probed` | Native RTMP client (librtmp equivalent) |
| WebRTC | Minimal-honest — HTTP GET reachability, no playback | `not_probed` | WHIP/WHEP HTTP signaling check + STUN reachability |
| DASH | Minimal-honest — HTTP GET reachability, no playback | `not_probed` | DASH manifest parse (similar to HLS) |

The `not_probed` error_code is documented in the error_msg: `"protocol=webrtc: full probing not yet implemented (Phase 3); HTTP 200 received"`. No faked success is ever emitted for these protocols.

---

## Sensitivity Math (N/A for WO-301)

F9 anomaly detection is BE-02's scope (WO-302). WO-301 does not implement anomaly detection. See decisions.md D-012.

---

## Measured Numbers

### Acceptance criteria

| Criterion | Command | Result | Verdict |
|-----------|---------|--------|---------|
| `CGO_ENABLED=0 go build ./...` green | `CGO_ENABLED=0 go build ./...` | exit 0, no output | PASS |
| `CGO_ENABLED=0 go vet ./...` green | `CGO_ENABLED=0 go vet ./...` | exit 0, no output | PASS |
| `CGO_ENABLED=0 go test ./...` green | `CGO_ENABLED=0 go test ./... -timeout 120s` | 16 packages, 0 FAIL | PASS |
| HLS happy path: success=true, TTFB>0, bitrate>0 | `TestHLSProbe_Success` | success=true, ttfb_ms=1, bitrate_kbps=66.7 | PASS |
| 500 origin: success=false, error_code=http_5xx | `TestHLSProbe_HTTP500` | success=false, error_code=http_5xx | PASS |
| Timeout origin: success=false, error_code=timeout | `TestHLSProbe_Timeout` | success=false, error_code=timeout (1s timeout fires) | PASS |
| not-probed: success=false, error_code=not_probed | `TestProbe_NotProbed` (webrtc/rtmp/dash) | 3/3 PASS | PASS |
| Interval honored: ≥3 firings in 2 intervals | `TestInterval_Honored` | 3 firings (initial + 2×60s advance) | PASS |
| Master HLS playlist: success=true | `TestHLSManifest_Parse/master_playlist_returns_empty_segment` | success=true, bitrate=0 | PASS |
| CH integration: insert N + QueryProbeResults returns time-ordered | `TestIntegration_ProbeResults` | 20 inserted, 20 queried, time-ordered, range-filtered, limited | PASS |

### Interval measurement (fake clock)

```
Probe interval_s = 60
Initial fire: immediate (After(0) fires at once with MaxJitterFraction=0)
Fire 1: immediate after runner start
Fire 2: Advance(60s) → immediate
Fire 3: Advance(60s) → immediate
Total: 3 firings in 2 interval advances — PASS (≥3 expected)
```

### ClickHouse probe_results integration

```
$ CGO_ENABLED=0 go test -tags integration -run TestIntegration_ProbeResults \
    ./internal/store/clickhouse/... -v -timeout 120s

=== RUN   TestIntegration_ProbeResults
    ClickHouse ready
    migrations applied
    inserting 20 probe_results for probe_id=probe-integration-001...
    probe_results inserted: 20 (expected 20)
    QueryProbeResults returned 20 results (expected 20)
    first-half range query: 10 results (expected ~10)
    limit=5 query returned 5 results
    PASS: probe_results=20 inserted+queried, time-ordered, range-filtered, limited
--- PASS: TestIntegration_ProbeResults (3.04s)
```

---

## Dependencies Added

No new dependencies. The prober uses only stdlib (`net/http`, `bufio`, `io`) + already-imported `github.com/google/uuid` (go.mod already has it from Wave 1 meta store).

---

## Cmd Edits Declared (D-005)

Files modified in `server/cmd/pulse/` (WO-301 scope per D-005):

- **`serve.go`** — Wave 3 additions:
  - Import: `"github.com/pulse-analytics/pulse/server/internal/domain"` (for ProbeConfigSource nil declaration)
  - Import: `"github.com/pulse-analytics/pulse/server/internal/prober"` (runner type)
  - Added `probeRunner *prober.Runner` field to the `server` struct
  - In `newServer`: HOOK(BE-02, WO-302) comment + `var probeSource domain.ProbeConfigSource` + `var probeRunnerInstance *prober.Runner` + conditional construction
  - In `Start`: probe runner goroutine launch (conditional on non-nil runner)

---

## Gaps / Change Requests

### GAP-3-001: HLS segment TTFB not separately measured

The current implementation measures TTFB of the manifest request only (`result.TTFBMs` = time-to-first-byte of the manifest HTTP response). The segment TTFB is implicitly included in the bitrate measurement timing. For Phase 3, consider storing manifest-TTFB and segment-TTFB separately. The frozen `probe_results` DDL has a single `ttfb_ms` column, so this is a schema CR if needed.

**Impact:** Non-blocking. TTFB as currently measured is the manifest TTFB, which is the most actionable metric for HLS delivery monitoring.

### GAP-3-002: Probe runner not yet wired to a real ProbeConfigSource

The probe runner is constructed but the `probeSource` is `nil` until BE-02 (WO-302) implements `domain.ProbeConfigSource` over the meta `probes` table. The HOOK comment in `serve.go` shows exactly where BE-02 should wire it.

**Impact:** Non-blocking. Runner is fully functional; only the source is missing. Tests prove the runner works against a fake source.

### GAP-3-003: HLS probe success semantics for manifest-only URLs

When the manifest URL points to a master playlist (no `#EXTINF` segments, only variant URLs), the probe returns `Success=true, BitrateKbps=0`. This is correct per the WO ("manifest + first media segment; success on 2xx + parseable") — a reachable master playlist IS a success, and bitrate requires a media segment. Phase 3: follow the first variant URL and fetch that media segment for a more complete probe.

**Impact:** Acceptable for MVP. The behavior is documented in `probeHLS` function comments.
