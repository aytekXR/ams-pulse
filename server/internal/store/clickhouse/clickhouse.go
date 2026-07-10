// Package clickhouse implements the event store: batched inserts of normalized
// events and query helpers over raw tables and rollups.
//
// Schema is owned by contracts/db/clickhouse/ migrations — this package never
// issues DDL outside `pulse migrate`. Performance budgets from the PRD:
// 13-month rollup queries < 3s (F2); ~1–2 GB per million viewer-sessions at
// default sampling (§7.10).
//
// Write path: all inserts go through async batching (flush at 1000 events OR
// 2 s, whichever comes first). This tolerates AMS Kafka/REST bursts without
// dropping events and keeps ClickHouse insert pressure low for small deployments.
package clickhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// Config holds ClickHouse store configuration.
type Config struct {
	// DSN is the ClickHouse native protocol DSN, e.g.
	// "clickhouse://localhost:9000/pulse"
	DSN string

	// Database name (default: "pulse").
	Database string

	// BatchSize triggers a flush when the pending batch reaches this count.
	BatchSize int

	// FlushInterval triggers a periodic flush regardless of batch size.
	FlushInterval time.Duration

	// MaxRetries for initial connect (compose race mitigation).
	MaxRetries int

	// RetryDelay between connect retries.
	RetryDelay time.Duration
}

// Store is the ClickHouse-backed event store.
// Implements collector.Consumer so it can be wired directly into the fanout.
type Store struct {
	cfg    Config
	conn   clickhouse.Conn
	db     string
	logger *slog.Logger

	// Async batch queues.
	serverEventCh chan domain.ServerEvent
	beaconEventCh chan domain.BeaconEvent
	viewerSessCh  chan domain.ViewerSession

	// Metrics.
	inserted atomic.Int64
	dropped  atomic.Int64

	done chan struct{}
	once sync.Once

	// wg tracks the three flusher goroutines started by Start().
	// Close() waits on wg before closing the connection, guaranteeing that
	// all in-flight and buffered events are inserted before the connection
	// is torn down.
	wg sync.WaitGroup
}

// Conn is the read accessor for BE-02's query plane.
// BE-02 imports this to run analytical queries against ClickHouse.
type Conn = clickhouse.Conn

// New creates and returns a Store, establishing the ClickHouse connection with
// retry.
func New(ctx context.Context, cfg Config, logger *slog.Logger) (*Store, error) {
	if cfg.Database == "" {
		cfg.Database = "pulse"
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 1000
	}
	if cfg.FlushInterval == 0 {
		cfg.FlushInterval = 2 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 10
	}
	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = 2 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}

	var conn clickhouse.Conn
	var lastErr error
	for i := 0; i < cfg.MaxRetries; i++ {
		opts, err := clickhouse.ParseDSN(cfg.DSN)
		if err != nil {
			return nil, fmt.Errorf("clickhouse: parse DSN: %w", err)
		}
		// Ensure we connect to the configured database (not the DSN default which
		// may differ from cfg.Database). The database must already exist before
		// New is called (run `pulse migrate` first).
		if cfg.Database != "" {
			opts.Auth.Database = cfg.Database
		}
		conn, err = clickhouse.Open(opts)
		if err != nil {
			lastErr = err
			logger.Warn("clickhouse: connect failed, retrying",
				"attempt", i+1,
				"max", cfg.MaxRetries,
				"error", err,
			)
		} else {
			// Ping to confirm connection is live.
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = conn.Ping(pingCtx)
			cancel()
			if err == nil {
				break
			}
			lastErr = err
			_ = conn.Close()
			conn = nil
			logger.Warn("clickhouse: ping failed, retrying",
				"attempt", i+1,
				"max", cfg.MaxRetries,
				"error", err,
			)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(cfg.RetryDelay):
		}
	}
	if conn == nil {
		return nil, fmt.Errorf("clickhouse: failed to connect after %d retries: %w", cfg.MaxRetries, lastErr)
	}

	s := &Store{
		cfg:           cfg,
		conn:          conn,
		db:            cfg.Database,
		logger:        logger,
		serverEventCh: make(chan domain.ServerEvent, cfg.BatchSize*2),
		beaconEventCh: make(chan domain.BeaconEvent, cfg.BatchSize*2),
		viewerSessCh:  make(chan domain.ViewerSession, cfg.BatchSize*2),
		done:          make(chan struct{}),
	}
	return s, nil
}

// GetConn returns the underlying ClickHouse connection for BE-02's query plane.
// BE-02 should use this via a thin accessor in the store package.
func (s *Store) GetConn() Conn {
	return s.conn
}

// Start launches the background flush goroutines. Call once after New.
func (s *Store) Start(ctx context.Context) {
	s.wg.Add(3)
	go s.runServerEventFlusher(ctx)
	go s.runBeaconEventFlusher(ctx)
	go s.runViewerSessionFlusher(ctx)
}

// Close shuts down the store gracefully.
//
// # Drain contract
//
// Close() signals the three flusher goroutines to stop (via the done channel),
// waits for each of them to drain its channel buffer completely and flush the
// final partial batch, then closes the underlying ClickHouse connection.
// Callers are guaranteed that every event queued before Close() returns has
// been handed to the ClickHouse driver.
//
// Context-cancel vs Close()
//
// The ctx passed to Start() provides a *fast exit* for normal operation: each
// flusher flushes its current in-memory batch and exits WITHOUT draining the
// channel. This keeps the stop path lean for restarts where a fresh process
// will reprocess events. The full drain guarantee holds only when the shutdown
// path calls Close().
//
// IMPORTANT — serve.go gap: Start is called with the signal-aware ctx
// (cancelled by SIGTERM). When SIGTERM fires, flushers exit via ctx.Done()
// before Stop/Close is called, defeating the drain. Fix: pass
// context.Background() (or a dedicated, non-signal context) to store.Start()
// so only Close() controls flusher lifetime. This is a one-line change in
// serve.go (s.store.Start(context.Background())), reported here rather than
// applied because the broader context-wiring pattern affects all sources.
//
// Safety
//
//   - Idempotent: subsequent calls are no-ops (once.Do).
//   - Safe when Start was never called: wg counter is zero, wg.Wait returns
//     immediately, no hang.
//   - Safe when ctx was already cancelled before Close: flushers exited via
//     ctx.Done() first, wg counter is already zero, wg.Wait returns promptly.
//   - Events sent after Close begins may be dropped (non-blocking send with
//     default case) but will never panic.
func (s *Store) Close() {
	s.once.Do(func() {
		close(s.done)
		// Wait for all flusher goroutines to drain their channels and exit.
		// If Start was never called the counter is zero and Wait returns
		// immediately. If flushers already exited via ctx.Done(), the counter
		// is also zero and Wait returns immediately.
		s.wg.Wait()
		_ = s.conn.Close()
	})
}

// ─── collector.Consumer implementation ──────────────────────────────────────

// OnServerEvent queues a ServerEvent for batched insert.
func (s *Store) OnServerEvent(ev domain.ServerEvent) {
	select {
	case s.serverEventCh <- ev:
	default:
		s.dropped.Add(1)
		s.logger.Warn("clickhouse: server event channel full, dropping event",
			"event_type", ev.Type,
		)
	}
}

// OnBeaconEvent queues a BeaconEvent for batched insert.
func (s *Store) OnBeaconEvent(ev domain.BeaconEvent) {
	select {
	case s.beaconEventCh <- ev:
	default:
		s.dropped.Add(1)
	}
}

// OnViewerSession queues a ViewerSession for batched upsert.
func (s *Store) OnViewerSession(sess domain.ViewerSession) {
	select {
	case s.viewerSessCh <- sess:
	default:
		s.dropped.Add(1)
	}
}

// ─── Flush goroutines ─────────────────────────────────────────────────────────

func (s *Store) runServerEventFlusher(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]domain.ServerEvent, 0, s.cfg.BatchSize)

	flush := func(flushCtx context.Context) {
		if len(batch) == 0 {
			return
		}
		if err := s.insertServerEvents(flushCtx, batch); err != nil {
			s.logger.Error("clickhouse: insert server_events failed", "error", err, "count", len(batch))
		} else {
			s.inserted.Add(int64(len(batch)))
		}
		batch = batch[:0]
	}

	// drain consumes all events remaining in the channel and flushes them.
	// Uses context.Background() so inserts succeed even if Start's ctx is cancelled.
	drain := func() {
		drainCtx := context.Background()
		for {
			select {
			case ev := <-s.serverEventCh:
				batch = append(batch, ev)
				if len(batch) >= s.cfg.BatchSize {
					flush(drainCtx)
				}
			default:
				flush(drainCtx)
				return
			}
		}
	}

	for {
		// Priority check: if Close() has been called, enter drain regardless of
		// whether ctx is also cancelled. This ensures the graceful-drain guarantee
		// holds even when both done and ctx.Done() fire simultaneously.
		select {
		case <-s.done:
			drain()
			return
		default:
		}

		select {
		case <-s.done:
			drain()
			return
		case <-ctx.Done():
			// Fast exit: flush the current in-memory batch only; do not drain
			// the channel (events buffered there may be re-queued by a new
			// process or are acceptable to lose on a crash restart).
			flush(ctx)
			return
		case ev := <-s.serverEventCh:
			batch = append(batch, ev)
			if len(batch) >= s.cfg.BatchSize {
				flush(ctx)
			}
		case <-ticker.C:
			flush(ctx)
		}
	}
}

func (s *Store) runBeaconEventFlusher(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]domain.BeaconEvent, 0, s.cfg.BatchSize)

	flush := func(flushCtx context.Context) {
		if len(batch) == 0 {
			return
		}
		if err := s.insertBeaconEvents(flushCtx, batch); err != nil {
			s.logger.Error("clickhouse: insert beacon_events failed", "error", err, "count", len(batch))
		} else {
			s.inserted.Add(int64(len(batch)))
		}
		batch = batch[:0]
	}

	drain := func() {
		drainCtx := context.Background()
		for {
			select {
			case ev := <-s.beaconEventCh:
				batch = append(batch, ev)
				if len(batch) >= s.cfg.BatchSize {
					flush(drainCtx)
				}
			default:
				flush(drainCtx)
				return
			}
		}
	}

	for {
		select {
		case <-s.done:
			drain()
			return
		default:
		}

		select {
		case <-s.done:
			drain()
			return
		case <-ctx.Done():
			flush(ctx)
			return
		case ev := <-s.beaconEventCh:
			batch = append(batch, ev)
			if len(batch) >= s.cfg.BatchSize {
				flush(ctx)
			}
		case <-ticker.C:
			flush(ctx)
		}
	}
}

func (s *Store) runViewerSessionFlusher(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]domain.ViewerSession, 0, s.cfg.BatchSize)

	flush := func(flushCtx context.Context) {
		if len(batch) == 0 {
			return
		}
		if err := s.insertViewerSessions(flushCtx, batch); err != nil {
			s.logger.Error("clickhouse: insert viewer_sessions failed", "error", err, "count", len(batch))
		} else {
			s.inserted.Add(int64(len(batch)))
		}
		batch = batch[:0]
	}

	drain := func() {
		drainCtx := context.Background()
		for {
			select {
			case sess := <-s.viewerSessCh:
				batch = append(batch, sess)
				if len(batch) >= s.cfg.BatchSize {
					flush(drainCtx)
				}
			default:
				flush(drainCtx)
				return
			}
		}
	}

	for {
		select {
		case <-s.done:
			drain()
			return
		default:
		}

		select {
		case <-s.done:
			drain()
			return
		case <-ctx.Done():
			flush(ctx)
			return
		case sess := <-s.viewerSessCh:
			batch = append(batch, sess)
			if len(batch) >= s.cfg.BatchSize {
				flush(ctx)
			}
		case <-ticker.C:
			flush(ctx)
		}
	}
}

// ─── Insert helpers ───────────────────────────────────────────────────────────

func (s *Store) insertServerEvents(ctx context.Context, batch []domain.ServerEvent) error {
	b, err := s.conn.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s.server_events", s.db))
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}

	for _, ev := range batch {
		// Extract typed fields from the Data map.
		dataJSON, _ := json.Marshal(ev.Data)
		_ = dataJSON

		// Unpack data fields (zero-safe).
		d := ev.Data
		if d == nil {
			d = map[string]any{}
		}

		var geoCountry, geoRegion, clientDevice, clientOS, clientBrowser string
		if ev.Enrichment != nil {
			if ev.Enrichment.Geo != nil {
				geoCountry = ev.Enrichment.Geo.Country
				geoRegion = ev.Enrichment.Geo.Region
			}
			if ev.Enrichment.Client != nil {
				clientDevice = ev.Enrichment.Client.Device
				clientOS = ev.Enrichment.Client.OS
				clientBrowser = ev.Enrichment.Client.Browser
			}
		}

		ts := time.UnixMilli(ev.TS).UTC()

		if err := b.Append(
			uint8(ev.Version),
			ev.Type,
			ts,
			ev.Source,
			ev.NodeID,
			ev.App,
			ev.StreamID,
			// publish_type
			strFromData(d, "publish_type"),
			// stream stats
			uint32(intFromData(d, "viewer_count")),
			uint32(intFromProtocol(d, "webrtc")),
			uint32(intFromProtocol(d, "hls")),
			uint32(intFromProtocol(d, "rtmp")),
			uint32(intFromProtocol(d, "dash")),
			uint32(intFromProtocol(d, "other")),
			float32(floatFromData(d, "bitrate_kbps")),
			// webrtc client
			strFromData(d, "client_id"),
			float32(floatFromData(d, "rtt_ms")),
			float32(floatFromData(d, "jitter_ms")),
			float32(floatFromData(d, "packet_loss_pct")),
			// ingest
			float32(floatFromData(d, "fps")),
			float32(floatFromData(d, "keyframe_interval_s")),
			// node
			float32(floatFromData(d, "cpu_pct")),
			float32(floatFromData(d, "mem_pct")),
			float32(floatFromData(d, "disk_pct")),
			float32(floatFromData(d, "net_in_mbps")),
			float32(floatFromData(d, "net_out_mbps")),
			float32(floatFromData(d, "jvm_heap_used_mb")),
			// recording
			strFromData(d, "path"),
			uint64(int64FromData(d, "size_bytes")),
			uint32(intFromData(d, "duration_s")),
			// viewer
			strFromData(d, "viewer_id"),
			strFromData(d, "protocol"),
			strFromData(d, "ip_hash"),
			strFromData(d, "user_agent"),
			uint32(intFromData(d, "watch_time_s")),
			// enrichment
			geoCountry,
			geoRegion,
			clientDevice,
			clientOS,
			clientBrowser,
		); err != nil {
			return fmt.Errorf("append row: %w", err)
		}
	}
	return b.Send()
}

func (s *Store) insertBeaconEvents(ctx context.Context, batch []domain.BeaconEvent) error {
	for _, ev := range batch {
		for _, item := range ev.Events {
			b, err := s.conn.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s.beacon_events", s.db))
			if err != nil {
				return err
			}
			d := item.Data
			if d == nil {
				d = map[string]any{}
			}

			var geoCountry, geoRegion, clientDevice, clientOS, clientBrowser string
			if ev.Enrichment != nil {
				if ev.Enrichment.Geo != nil {
					geoCountry = ev.Enrichment.Geo.Country
					geoRegion = ev.Enrichment.Geo.Region
				}
				if ev.Enrichment.Client != nil {
					clientDevice = ev.Enrichment.Client.Device
					clientOS = ev.Enrichment.Client.OS
					clientBrowser = ev.Enrichment.Client.Browser
				}
			}

			ts := time.UnixMilli(item.TS).UTC()
			if err := b.Append(
				uint8(ev.Version),
				ev.SessionID,
				ev.StreamID,
				ev.App,
				ts,
				item.Type,
				uint32(intFromData(d, "startup_ms")),
				uint32(intFromData(d, "watch_ms")),
				float32(floatFromData(d, "bitrate_kbps")),
				uint32(intFromData(d, "buffer_ms")),
				uint32(intFromData(d, "dropped_frames")),
				uint32(intFromData(d, "duration_ms")), // rebuffer_ms
				strFromData(d, "code"),                // error_code
				boolToUint8(boolFromData(d, "fatal")),
				float32(floatFromData(d, "from_kbps")),
				float32(floatFromData(d, "to_kbps")),
				strFromData(d, "from"), // resolution_from
				strFromData(d, "to"),   // resolution_to
				ev.PlayerKind,
				ev.SDK,
				ev.Tenant,
				geoCountry,
				geoRegion,
				clientDevice,
				clientOS,
				clientBrowser,
			); err != nil {
				return err
			}
			if err := b.Send(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) insertViewerSessions(ctx context.Context, batch []domain.ViewerSession) error {
	b, err := s.conn.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s.viewer_sessions", s.db))
	if err != nil {
		return err
	}
	for _, sess := range batch {
		endedAt := sess.EndedAt
		if endedAt.IsZero() {
			endedAt = sess.StartedAt
		}
		if err := b.Append(
			sess.SessionID,
			sess.StreamID,
			sess.App,
			sess.NodeID,
			sess.StartedAt,
			endedAt,
			sess.UpdatedAt,
			sess.StartupMS,
			sess.WatchTimeS,
			sess.RebufferCount,
			sess.RebufferMS,
			sess.ErrorCount,
			sess.PeakBitrate,
			sess.Protocol,
			sess.GeoCountry,
			sess.GeoRegion,
			sess.ClientDevice,
			sess.ClientOS,
			sess.ClientBrowser,
			sess.Tenant,
		); err != nil {
			return err
		}
	}
	return b.Send()
}

// Metrics returns insert/drop counters for observability.
func (s *Store) Metrics() (inserted, dropped int64) {
	return s.inserted.Load(), s.dropped.Load()
}

// ─── F10 Probe result store ───────────────────────────────────────────────────

// InsertProbeResult writes a single probe result to ClickHouse probe_results.
// Called synchronously by the probe runner after each check (results are low
// frequency — one per probe per interval — so batching is not needed).
func (s *Store) InsertProbeResult(ctx context.Context, r domain.ProbeResult) error {
	// Explicit column list: the Append below is positional, and a bare INSERT
	// binds to the table's physical column order — a future ADD COLUMN ... AFTER
	// would silently misalign values (D-072 verifier finding).
	// Explicit column list guards against physical-order drift (D-072 positional
	// hazard): every column appended here must appear in the Append call below
	// at the same position, atomically.
	// CH migration 0007 adds ice_state LowCardinality(String) DEFAULT ''.
	// CH migration 0008 adds rtt_ms, jitter_ms, loss_pct Nullable(Float32) (D-075 WO-B).
	// clickhouse-go v2 maps nil *float32 → NULL for Nullable(Float32) columns via
	// the reflect-based AppendRow nil-pointer check in lib/column/nullable.go — verified
	// against clickhouse-go v2.47.0 source (nullable.go:AppendRow, ScanRow).
	b, err := s.conn.PrepareBatch(ctx, fmt.Sprintf(
		"INSERT INTO %s.probe_results (id, probe_id, ts, success, ttfb_ms, error_code, error_msg, bitrate_kbps, segment_ttfb_ms, connect_time_ms, signaling_state, ice_state, rtt_ms, jitter_ms, loss_pct)", s.db))
	if err != nil {
		return fmt.Errorf("probe_results: prepare batch: %w", err)
	}
	var successByte uint8
	if r.Success {
		successByte = 1
	}
	// connect_time_ms: ClickHouse column is UInt32 DEFAULT 0; use 0 when nil (non-WebRTC).
	var connectTimeMs uint32
	if r.ConnectTimeMs != nil {
		connectTimeMs = *r.ConnectTimeMs
	}
	if err := b.Append(
		r.ID,
		r.ProbeID,
		r.TS.UTC(),
		successByte,
		r.TTFBMs,
		r.ErrorCode,
		r.ErrorMsg,
		r.BitrateKbps,
		r.SegmentTTFBMs,
		connectTimeMs,
		r.SignalingState,
		r.IceState, // ice_state: "connected"|"failed"|"timeout"|"" (D-074 CH-0007)
		r.RttMs,    // rtt_ms:    Nullable(Float32); nil → NULL (D-075 CH-0008)
		r.JitterMs, // jitter_ms: Nullable(Float32); nil → NULL (D-075 CH-0008)
		r.LossPct,  // loss_pct:  Nullable(Float32); nil → NULL (D-075 CH-0008)
	); err != nil {
		return fmt.Errorf("probe_results: append: %w", err)
	}
	return b.Send()
}

// QueryProbeResults fetches probe results for a given probeID in the [from, to)
// time range, ordered by ts ASC, capped at limit rows.
// Used by BE-02's GET /probes/{id}/results handler.
func (s *Store) QueryProbeResults(ctx context.Context, probeID string, from, to time.Time, limit int) ([]domain.ProbeResult, error) {
	if limit <= 0 {
		limit = 100
	}
	query := fmt.Sprintf(
		`SELECT id, probe_id, ts, success, ttfb_ms, error_code, error_msg, bitrate_kbps, segment_ttfb_ms,
		        connect_time_ms, signaling_state, ice_state,
		        rtt_ms, jitter_ms, loss_pct
		 FROM %s.probe_results
		 WHERE probe_id = ?
		   AND ts >= ? AND ts < ?
		 ORDER BY ts ASC
		 LIMIT %d`,
		s.db, limit,
	)
	rows, err := s.conn.Query(ctx, query, probeID, from.UTC(), to.UTC())
	if err != nil {
		return nil, fmt.Errorf("probe_results: query: %w", err)
	}
	defer rows.Close()

	var results []domain.ProbeResult
	for rows.Next() {
		var (
			r              domain.ProbeResult
			successU8      uint8
			ts             time.Time
			connectTimeMs  uint32
			signalingState string
			iceState       string
			// Nullable(Float32) columns — nil when NULL in CH (D-075 CH-0008).
			rttMs    *float32
			jitterMs *float32
			lossPct  *float32
		)
		if err := rows.Scan(
			&r.ID, &r.ProbeID, &ts, &successU8,
			&r.TTFBMs, &r.ErrorCode, &r.ErrorMsg, &r.BitrateKbps, &r.SegmentTTFBMs,
			&connectTimeMs, &signalingState, &iceState,
			&rttMs, &jitterMs, &lossPct,
		); err != nil {
			return nil, fmt.Errorf("probe_results: scan: %w", err)
		}
		r.TS = ts.UTC()
		r.Success = successU8 != 0
		// Reconstruct nullable ConnectTimeMs: 0 in DB means nil (not applicable).
		if connectTimeMs > 0 {
			ct := connectTimeMs
			r.ConnectTimeMs = &ct
		}
		r.SignalingState = signalingState
		r.IceState = iceState // "" when not applicable (CH DEFAULT '')
		// True NULL → nil pointer (no sentinel reconstruction — Nullable is exact, D-075).
		r.RttMs = rttMs
		r.JitterMs = jitterMs
		r.LossPct = lossPct
		results = append(results, r)
	}
	return results, rows.Err()
}

// ─── Data helpers ─────────────────────────────────────────────────────────────

func strFromData(d map[string]any, key string) string {
	if v, ok := d[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func intFromData(d map[string]any, key string) int {
	if v, ok := d[key]; ok {
		switch x := v.(type) {
		case int:
			return x
		case float64:
			return int(x)
		case int64:
			return int(x)
		}
	}
	return 0
}

func int64FromData(d map[string]any, key string) int64 {
	if v, ok := d[key]; ok {
		switch x := v.(type) {
		case int64:
			return x
		case float64:
			return int64(x)
		case int:
			return int64(x)
		}
	}
	return 0
}

func floatFromData(d map[string]any, key string) float64 {
	if v, ok := d[key]; ok {
		switch x := v.(type) {
		case float64:
			return x
		case float32:
			return float64(x)
		case int:
			return float64(x)
		}
	}
	return 0
}

func boolFromData(d map[string]any, key string) bool {
	if v, ok := d[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func boolToUint8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

func intFromProtocol(d map[string]any, proto string) int {
	if pcMap, ok := d["viewer_count_by_protocol"].(map[string]any); ok {
		return intFromData(pcMap, proto)
	}
	return 0
}
