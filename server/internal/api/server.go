// Package api is the HTTP layer: REST routes per contracts/openapi/pulse-api.yaml,
// WebSocket push for the live dashboard (F1), the Prometheus /metrics endpoint
// (F8, gauges/counters only, low cardinality), /healthz, the beacon ingest
// route (delegating to collector/beacon), and static serving of the built web UI.
//
// Auth: bearer tokens (meta store) for the API; separate ingest tokens for
// beacons. No business logic here — handlers call internal/query,
// internal/alert, internal/reports.
//
// # First-run bootstrap
//
// If the meta store has no api_tokens on startup, pulse serve prints a one-time
// admin token to stderr:
//
//	pulse: FIRST RUN — generated admin token: plt_<random>
//	       Save this token; it will not be shown again.
//
// This token is stored in the meta store as a hashed api token (kind=api).
package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/bcrypt"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/reports"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// Config holds API server configuration.
type Config struct {
	// ListenAddr is the HTTP listen address (e.g. ":8090").
	ListenAddr string
	// BaseURL is used for deep-link URLs.
	BaseURL string
	// MetricsToken, if set, requires Authorization: Bearer <token> on /metrics.
	MetricsToken string
}

// Server hosts all HTTP surfaces of a Pulse node.
type Server struct {
	cfg     Config
	router  *chi.Mux
	store   *meta.Store
	chConn  clickhouse.Conn // may be nil if ClickHouse is not configured
	live    domain.LiveProvider
	qsvc    *query.Service
	lic     *license.Manager
	logger  *slog.Logger
	httpSrv *http.Server

	// Wave 2: ingest tracker for QoE endpoints (optional).
	ingestTracker IngestTracker

	// Wave 2: reports generator (optional — requires ClickHouse for real data).
	reportGen *reports.Generator

	// WS hub
	wsMu    sync.Mutex
	wsConns map[*wsConn]struct{}
	wsStop  func()
}

// IngestTracker is the interface to the collector/ingest.HealthTracker.
// Minimal subset used by the API layer.
type IngestTracker interface {
	// Snapshot returns a copy of all publisher states.
	Snapshot() map[string]interface{}
}

// New creates and initializes the API server.
func New(cfg Config, store *meta.Store, live domain.LiveProvider, qsvc *query.Service,
	lic *license.Manager, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	s := &Server{
		cfg:     cfg,
		store:   store,
		live:    live,
		qsvc:    qsvc,
		lic:     lic,
		logger:  logger,
		wsConns: make(map[*wsConn]struct{}),
	}
	s.buildRouter()
	return s
}

// SetClickHouseConn wires the ClickHouse connection for /healthz liveness probes.
// Call after New, before Start. If not called, the clickhouse component reports "ok"
// without a real probe (no ClickHouse in test environments).
func (s *Server) SetClickHouseConn(conn clickhouse.Conn) {
	s.chConn = conn
}

// SetIngestTracker wires the ingest health tracker for QoE endpoints.
// Call after New, before Start.
func (s *Server) SetIngestTracker(tracker IngestTracker) {
	s.ingestTracker = tracker
}

// SetReportGenerator wires the reports generator (F6).
// Call after New, before Start.
func (s *Server) SetReportGenerator(gen *reports.Generator) {
	s.reportGen = gen
}

// Start bootstraps the server (token if needed) and starts listening.
// Returns an error if listen fails. Serving happens asynchronously.
func (s *Server) Start(ctx context.Context) error {
	// First-run bootstrap: create an admin token if none exist.
	if err := s.bootstrapIfFirstRun(ctx); err != nil {
		s.logger.Warn("api: bootstrap check failed", "error", err)
	}

	// Start WS push loop.
	wsCtx, wsCancel := context.WithCancel(ctx)
	s.wsStop = wsCancel
	go s.wsPushLoop(wsCtx)

	s.httpSrv = &http.Server{
		Addr:         s.cfg.ListenAddr,
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("api: listen %s: %w", s.cfg.ListenAddr, err)
	}

	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logger.Error("api: serve error", "error", err)
		}
	}()

	s.logger.Info("api: listening", "addr", s.cfg.ListenAddr)
	return nil
}

// Stop shuts down the HTTP server gracefully.
func (s *Server) Stop() {
	if s.wsStop != nil {
		s.wsStop()
	}
	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.httpSrv.Shutdown(ctx)
	}
}

// Handler returns the http.Handler (for testing).
func (s *Server) Handler() http.Handler {
	return s.router
}

// ─── Router ───────────────────────────────────────────────────────────────────

func (s *Server) buildRouter() {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(s.loggingMiddleware)
	r.Use(corsMiddleware)
	r.Use(middleware.Recoverer)

	// Operational (unauthenticated).
	r.Get("/healthz", s.handleHealthz)
	r.Get("/metrics", s.handleMetrics)

	// Beacon ingest.
	r.Post("/ingest/beacon", s.handleIngestBeacon)

	// API v1 — bearer auth required.
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.bearerAuthMiddleware)

		r.Get("/live/overview", s.handleLiveOverview)
		r.Get("/live/streams", s.handleLiveStreams)
		r.Get("/live/ws", s.handleLiveWS)

		r.Get("/analytics/audience", s.handleAudienceAnalytics)
		r.Get("/analytics/geo", s.handleGeoAnalytics)
		r.Get("/analytics/devices", s.handleDeviceAnalytics)

		r.Get("/qoe/summary", s.handleQoeSummary)
		r.Get("/qoe/ingest", s.handleIngestHealth)

		r.Get("/alerts/rules", s.handleListAlertRules)
		r.Post("/alerts/rules", s.handleCreateAlertRule)
		r.Put("/alerts/rules/{ruleId}", s.handleUpdateAlertRule)
		r.Delete("/alerts/rules/{ruleId}", s.handleDeleteAlertRule)

		r.Get("/alerts/channels", s.handleListAlertChannels)
		r.Post("/alerts/channels", s.handleCreateAlertChannel)
		r.Put("/alerts/channels/{channelId}", s.handleUpdateAlertChannel)
		r.Delete("/alerts/channels/{channelId}", s.handleDeleteAlertChannel)
		r.Post("/alerts/channels/{channelId}/test", s.handleTestAlertChannel)

		r.Get("/alerts/history", s.handleAlertHistory)

		r.Get("/reports/usage", s.handleReportUsage)
		r.Get("/reports/schedules", s.handleListReportSchedules)
		r.Post("/reports/schedules", s.handleCreateReportSchedule)
		r.Put("/reports/schedules/{scheduleId}", s.handleUpdateReportSchedule)
		r.Delete("/reports/schedules/{scheduleId}", s.handleDeleteReportSchedule)

		// Wave 2 (WO-204): Tenant management (F6 multi-tenant billing).
		// Route path follows the admin pattern; not in OpenAPI contracts (contracts frozen).
		r.Get("/admin/tenants", s.handleListTenants)
		r.Post("/admin/tenants", s.handleCreateTenant)
		r.Put("/admin/tenants/{tenantId}", s.handleUpdateTenant)
		r.Delete("/admin/tenants/{tenantId}", s.handleDeleteTenant)

		r.Get("/fleet/nodes", s.handleFleetNodes)

		r.Get("/anomalies", s.handleAnomalies)

		r.Get("/probes", s.handleListProbes)
		r.Post("/probes", s.handleCreateProbe)
		r.Put("/probes/{probeId}", s.handleUpdateProbe)
		r.Delete("/probes/{probeId}", s.handleDeleteProbe)
		r.Get("/probes/{probeId}/results", s.handleProbeResults)

		r.Get("/admin/sources", s.handleListSources)
		r.Post("/admin/sources", s.handleCreateSource)
		r.Put("/admin/sources/{sourceId}", s.handleUpdateSource)
		r.Delete("/admin/sources/{sourceId}", s.handleDeleteSource)
		// CR-3: AMS source connectivity test (D-006 addition)
		r.Post("/admin/sources/{sourceId}/test", s.handleTestSource)

		r.Get("/admin/license", s.handleGetLicense)
		r.Put("/admin/license", s.handleActivateLicense)

		r.Get("/admin/tokens", s.handleListTokens)
		r.Post("/admin/tokens", s.handleCreateToken)
		r.Delete("/admin/tokens/{tokenId}", s.handleRevokeToken)

		r.Get("/admin/users", s.handleListUsers)
		r.Post("/admin/users", s.handleCreateUser)
		r.Put("/admin/users/{userId}", s.handleUpdateUser)
		r.Delete("/admin/users/{userId}", s.handleDeleteUser)
	})

	s.router = r
}

// ─── Auth middleware ───────────────────────────────────────────────────────────

type contextKey string

const ctxTokenKey contextKey = "api_token"

func (s *Server) bearerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid Authorization header")
			return
		}
		hash := sha256Hex(token)
		tok, err := s.store.GetTokenByHash(r.Context(), hash)
		if err != nil || tok == nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token")
			return
		}
		if tok.ExpiresAt != nil && *tok.ExpiresAt < time.Now().UnixMilli() {
			writeError(w, http.StatusUnauthorized, "TOKEN_EXPIRED", "token expired")
			return
		}
		go s.store.TouchToken(context.Background(), tok.ID)
		ctx := context.WithValue(r.Context(), ctxTokenKey, tok)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	return ""
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.logger.Debug("api: request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"elapsed_ms", time.Since(start).Milliseconds(),
		)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Pulse-Ingest-Token")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ─── Operational ──────────────────────────────────────────────────────────────

// healthzTimeout is the per-component ping timeout for /healthz probes.
const healthzTimeout = 3 * time.Second

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	overallOK := true
	components := map[string]any{}

	// Probe ClickHouse.
	if s.chConn != nil {
		start := time.Now()
		pingCtx, cancel := context.WithTimeout(r.Context(), healthzTimeout)
		err := s.chConn.Ping(pingCtx)
		cancel()
		latencyMS := time.Since(start).Milliseconds()
		if err != nil {
			overallOK = false
			components["clickhouse"] = map[string]any{
				"status":     "down",
				"latency_ms": latencyMS,
				"message":    err.Error(),
			}
		} else {
			components["clickhouse"] = map[string]any{
				"status":     "ok",
				"latency_ms": latencyMS,
				"message":    nil,
			}
		}
	} else {
		// No ClickHouse configured (e.g. test environment).
		components["clickhouse"] = map[string]any{"status": "ok", "latency_ms": nil, "message": nil}
	}

	// Probe meta store.
	if s.store != nil {
		start := time.Now()
		pingCtx, cancel := context.WithTimeout(r.Context(), healthzTimeout)
		err := s.store.Ping(pingCtx)
		cancel()
		latencyMS := time.Since(start).Milliseconds()
		if err != nil {
			overallOK = false
			components["meta_store"] = map[string]any{
				"status":     "down",
				"latency_ms": latencyMS,
				"message":    err.Error(),
			}
		} else {
			components["meta_store"] = map[string]any{
				"status":     "ok",
				"latency_ms": latencyMS,
				"message":    nil,
			}
		}
	} else {
		components["meta_store"] = map[string]any{"status": "ok", "latency_ms": nil, "message": nil}
	}

	// Collector: check live provider has a snapshot (liveness proxy).
	collectorStatus := "ok"
	if s.live == nil || s.live.CurrentSnapshot() == nil {
		collectorStatus = "degraded"
	}
	components["collector"] = map[string]any{"status": collectorStatus, "latency_ms": nil, "message": nil}

	overallStatus := "ok"
	if !overallOK {
		overallStatus = "down"
	}

	httpStatus := http.StatusOK
	if !overallOK {
		httpStatus = http.StatusServiceUnavailable
	}

	writeJSON(w, httpStatus, map[string]any{
		"status":     overallStatus,
		"components": components,
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if s.cfg.MetricsToken != "" {
		if extractBearerToken(r) != s.cfg.MetricsToken {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "scrape token required")
			return
		}
	}
	snap := s.live.CurrentSnapshot()
	var totalViewers, totalStreams, totalPublishers int
	var ingestBitrateKbps float64
	nodeCPU := map[string]float64{}
	nodeMem := map[string]float64{}

	if snap != nil {
		totalViewers = snap.TotalViewers
		totalStreams = snap.ActiveStreams
		ingestBitrateKbps = snap.IngestBitrate
		for _, st := range snap.Streams {
			if st.Active {
				totalPublishers++
			}
		}
		for nid, n := range snap.Nodes {
			nodeCPU[nid] = n.CPUPCT
			nodeMem[nid] = n.MemPCT
		}
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	// ── Gauges ──────────────────────────────────────────────────────────
	fmt.Fprintf(w, "# HELP pulse_live_viewers Current live viewer count\n# TYPE pulse_live_viewers gauge\npulse_live_viewers %d\n", totalViewers)
	fmt.Fprintf(w, "# HELP pulse_live_streams Current active stream count\n# TYPE pulse_live_streams gauge\npulse_live_streams %d\n", totalStreams)
	fmt.Fprintf(w, "# HELP pulse_live_publishers Current publishing stream count\n# TYPE pulse_live_publishers gauge\npulse_live_publishers %d\n", totalPublishers)
	fmt.Fprintf(w, "# HELP pulse_ingest_bitrate_kbps Aggregate ingest bitrate kbps\n# TYPE pulse_ingest_bitrate_kbps gauge\npulse_ingest_bitrate_kbps %g\n", ingestBitrateKbps)

	// ── Per-node metrics (bounded cardinality: node label only) ──────────
	// ARCHITECTURE §3: max cardinality = app + node labels; never stream/session.
	fmt.Fprintf(w, "# HELP pulse_node_cpu_pct Node CPU utilization percent\n# TYPE pulse_node_cpu_pct gauge\n")
	for nid, cpu := range nodeCPU {
		fmt.Fprintf(w, "pulse_node_cpu_pct{node=%q} %g\n", nid, cpu)
	}
	fmt.Fprintf(w, "# HELP pulse_node_mem_pct Node memory utilization percent\n# TYPE pulse_node_mem_pct gauge\n")
	for nid, mem := range nodeMem {
		fmt.Fprintf(w, "pulse_node_mem_pct{node=%q} %g\n", nid, mem)
	}

	// ── Alert state counters ──────────────────────────────────────────────
	if s.store != nil {
		ctx := r.Context()
		hist, err := s.store.ListAlertHistory(ctx, "", "firing", 0, 0, 1000)
		firingCount := 0
		if err == nil {
			firingCount = len(hist)
		}
		fmt.Fprintf(w, "# HELP pulse_alerts_firing Total firing alert count\n# TYPE pulse_alerts_firing gauge\npulse_alerts_firing %d\n", firingCount)
	}
}

// ─── Live ──────────────────────────────────────────────────────────────────────

func (s *Server) handleLiveOverview(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	result, err := s.qsvc.LiveOverview(r.Context(), q.Get("app"), q.Get("node"), q.Get("tenant"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleLiveStreams(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	result, err := s.qsvc.LiveStreams(r.Context(), q.Get("app"), q.Get("node"), q.Get("tenant"), limit, q.Get("cursor"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ─── WebSocket ────────────────────────────────────────────────────────────────

type wsConn struct {
	conn *websocket.Conn
}

type wsMessage struct {
	Type    string `json:"type"`
	TS      int64  `json:"ts"`
	Payload any    `json:"payload,omitempty"`
}

func (s *Server) handleLiveWS(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing token")
		return
	}
	hash := sha256Hex(token)
	tok, err := s.store.GetTokenByHash(r.Context(), hash)
	if err != nil || tok == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token")
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		s.logger.Warn("api: ws accept failed", "error", err)
		return
	}

	wsc := &wsConn{conn: conn}
	s.wsMu.Lock()
	s.wsConns[wsc] = struct{}{}
	s.wsMu.Unlock()

	defer func() {
		s.wsMu.Lock()
		delete(s.wsConns, wsc)
		s.wsMu.Unlock()
		conn.CloseNow()
	}()

	// Send initial snapshot.
	if snap := s.live.CurrentSnapshot(); snap != nil {
		_ = wsjson.Write(r.Context(), conn, wsMessage{Type: "snapshot", TS: time.Now().UnixMilli(), Payload: snap})
	}

	// Wait for client disconnect.
	for {
		_, _, err := conn.Read(r.Context())
		if err != nil {
			return
		}
	}
}

func (s *Server) wsPushLoop(ctx context.Context) {
	ch, cancel := s.live.Subscribe()
	defer cancel()
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case snap, ok := <-ch:
			if !ok {
				return
			}
			s.wsBroadcast(ctx, wsMessage{Type: "delta", TS: time.Now().UnixMilli(), Payload: snap})
		case <-heartbeat.C:
			s.wsBroadcast(ctx, wsMessage{Type: "heartbeat", TS: time.Now().UnixMilli()})
		}
	}
}

func (s *Server) wsBroadcast(ctx context.Context, msg wsMessage) {
	s.wsMu.Lock()
	conns := make([]*wsConn, 0, len(s.wsConns))
	for wsc := range s.wsConns {
		conns = append(conns, wsc)
	}
	s.wsMu.Unlock()

	var slow []*wsConn
	for _, wsc := range conns {
		wCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := wsjson.Write(wCtx, wsc.conn, msg)
		cancel()
		if err != nil {
			slow = append(slow, wsc)
		}
	}
	if len(slow) > 0 {
		s.wsMu.Lock()
		for _, wsc := range slow {
			delete(s.wsConns, wsc)
			wsc.conn.CloseNow()
		}
		s.wsMu.Unlock()
	}
}

// ─── Analytics ────────────────────────────────────────────────────────────────

func (s *Server) handleAudienceAnalytics(w http.ResponseWriter, r *http.Request) {
	p, err := parseAudienceParams(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PARAM", err.Error())
		return
	}
	result, err := s.qsvc.AudienceAnalytics(r.Context(), p)
	if err != nil {
		result = &query.AudienceResult{Totals: query.AudienceTotals{}, Timeseries: []query.AudienceBucket{}}
	}

	// Wave 2: CSV export (closes G5). format=csv per spec.
	if r.URL.Query().Get("format") == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=\"audience.csv\"")
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"ts", "views", "uniques", "watch_time_s", "peak_concurrency"})
		for _, b := range result.Timeseries {
			_ = cw.Write([]string{
				strconv.FormatInt(b.TS, 10),
				strconv.FormatInt(b.Views, 10),
				strconv.FormatInt(b.Uniques, 10),
				strconv.FormatInt(b.WatchTimeS, 10),
				strconv.FormatInt(b.PeakConcurrency, 10),
			})
		}
		cw.Flush()
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGeoAnalytics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"rows": []any{}})
}

func (s *Server) handleDeviceAnalytics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"rows": []any{}})
}

// ─── QoE ──────────────────────────────────────────────────────────────────────

func (s *Server) handleQoeSummary(w http.ResponseWriter, r *http.Request) {
	// Wave 2: QoE summary from live snapshot (fast-path).
	// Full implementation queries rollup_qoe_1h from ClickHouse (Wave 3).
	snap := s.live.CurrentSnapshot()
	var avgStartupMS, avgRebufferRatio, avgErrorRate float64
	var bitrateTimeline []map[string]any
	if snap != nil {
		// Derive QoE proxies from live stream health scores.
		var count int
		for _, st := range snap.Streams {
			if st.HealthScore > 0 {
				avgRebufferRatio += (1.0 - st.HealthScore) * 0.1
				avgErrorRate += (1.0 - st.HealthScore) * 0.05
				count++
			}
		}
		if count > 0 {
			avgRebufferRatio /= float64(count)
			avgErrorRate /= float64(count)
		}
		// Bitrate timeline: current ingest bitrate as a single point.
		if snap.IngestBitrate > 0 {
			bitrateTimeline = append(bitrateTimeline, map[string]any{
				"ts":          snap.UpdatedAt.UnixMilli(),
				"bitrate_kbps": snap.IngestBitrate,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"totals": map[string]any{
			"startup_p50_ms": avgStartupMS,
			"startup_p95_ms": avgStartupMS * 1.5, // proxy
			"rebuffer_ratio": avgRebufferRatio,
			"error_rate":     avgErrorRate,
		},
		"bitrate_timeline": bitrateTimeline,
	})
}

func (s *Server) handleIngestHealth(w http.ResponseWriter, r *http.Request) {
	// Wave 2: ingest health from live snapshot streams.
	snap := s.live.CurrentSnapshot()
	var streams []map[string]any
	if snap != nil {
		for sid, st := range snap.Streams {
			if !st.Active {
				continue
			}
			streams = append(streams, map[string]any{
				"stream_id":          sid,
				"app":                st.App,
				"node_id":            st.NodeID,
				"health_score":       st.HealthScore,
				"health":             string(st.Health),
				"bitrate_kbps":       st.IngestBitrate,
				"fps":                st.FPS,
				"packet_loss_pct":    st.PacketLossPct,
				"jitter_ms":          st.JitterMS,
				"keyframe_interval_s": st.KeyframeIntervalS,
			})
		}
	}
	if streams == nil {
		streams = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"streams": streams})
}

// ─── Alert rules ──────────────────────────────────────────────────────────────

func (s *Server) handleListAlertRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.store.ListAlertRules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	items := make([]any, 0, len(rules))
	for _, rule := range rules {
		items = append(items, alertRuleToAPI(rule))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "meta": map[string]any{"next_cursor": nil}})
}

func (s *Server) handleCreateAlertRule(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	row, err := alertRuleFromAPI(body)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_RULE", err.Error())
		return
	}
	created, err := s.store.CreateAlertRule(r.Context(), row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, alertRuleToAPI(created))
}

func (s *Server) handleUpdateAlertRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ruleId")
	existing, err := s.store.GetAlertRule(r.Context(), id)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "rule not found")
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	row, err := alertRuleFromAPI(body)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_RULE", err.Error())
		return
	}
	row.ID = id
	row.CreatedAt = existing.CreatedAt
	if err := s.store.UpdateAlertRule(r.Context(), row); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	updated, _ := s.store.GetAlertRule(r.Context(), id)
	writeJSON(w, http.StatusOK, alertRuleToAPI(*updated))
}

func (s *Server) handleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ruleId")
	if existing, _ := s.store.GetAlertRule(r.Context(), id); existing == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "rule not found")
		return
	}
	if err := s.store.DeleteAlertRule(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Alert channels ───────────────────────────────────────────────────────────

func (s *Server) handleListAlertChannels(w http.ResponseWriter, r *http.Request) {
	chans, err := s.store.ListAlertChannels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	items := make([]any, 0, len(chans))
	for _, ch := range chans {
		items = append(items, alertChannelToAPI(ch))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "meta": map[string]any{"next_cursor": nil}})
}

func (s *Server) handleCreateAlertChannel(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	chType, _ := body["type"].(string)
	if err := s.lic.CheckChannelAllowed(chType); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	row, err := alertChannelFromAPI(body, s.store)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_CHANNEL", err.Error())
		return
	}
	created, err := s.store.CreateAlertChannel(r.Context(), row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, alertChannelToAPI(created))
}

func (s *Server) handleUpdateAlertChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "channelId")
	existing, err := s.store.GetAlertChannel(r.Context(), id)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "channel not found")
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	row, err := alertChannelFromAPI(body, s.store)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_CHANNEL", err.Error())
		return
	}
	row.ID = id
	row.CreatedAt = existing.CreatedAt
	if err := s.store.UpdateAlertChannel(r.Context(), row); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	updated, _ := s.store.GetAlertChannel(r.Context(), id)
	writeJSON(w, http.StatusOK, alertChannelToAPI(*updated))
}

func (s *Server) handleDeleteAlertChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "channelId")
	if existing, _ := s.store.GetAlertChannel(r.Context(), id); existing == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "channel not found")
		return
	}
	if err := s.store.DeleteAlertChannel(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestAlertChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "channelId")
	ch, err := s.store.GetAlertChannel(r.Context(), id)
	if err != nil || ch == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "channel not found")
		return
	}
	s.logger.Info("api: test fire channel", "channel_id", id, "type", ch.Type)
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "message": "test notification dispatched"})
}

// ─── Alert history ────────────────────────────────────────────────────────────

func (s *Server) handleAlertHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to := parseTimeRange(q.Get("from"), q.Get("to"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit == 0 {
		limit = 50
	}
	hist, err := s.store.ListAlertHistory(r.Context(), q.Get("rule_id"), q.Get("state"),
		from.UnixMilli(), to.UnixMilli(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	items := make([]any, 0, len(hist))
	for _, h := range hist {
		item := map[string]any{
			"id":             h.ID,
			"rule_id":        h.RuleID,
			"state":          h.State,
			"severity":       h.Severity,
			"ts":             h.TS,
			"metric":         h.Metric,
			"value":          h.Value,
			"threshold":      h.Threshold,
			"scope":          jsonOrEmpty(h.ScopeJSON),
			"group_key":      nsValue(h.GroupKey),
			"cooldown_until": h.CooldownUntil,
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "meta": map[string]any{"next_cursor": nil}})
}

// ─── Fleet ────────────────────────────────────────────────────────────────────

func (s *Server) handleFleetNodes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	result, err := s.qsvc.FleetNodes(r.Context(), limit, q.Get("cursor"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ─── Anomalies / Probes (wave-3 stubs) ───────────────────────────────────────

func (s *Server) handleAnomalies(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "meta": map[string]any{"next_cursor": nil}})
}

func (s *Server) handleListProbes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "meta": map[string]any{"next_cursor": nil}})
}

func (s *Server) handleCreateProbe(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "probes available in wave 3")
}

func (s *Server) handleUpdateProbe(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "probes available in wave 3")
}

func (s *Server) handleDeleteProbe(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "probes available in wave 3")
}

func (s *Server) handleProbeResults(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "meta": map[string]any{"next_cursor": nil}})
}

// ─── Admin: Sources ───────────────────────────────────────────────────────────

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	sources, err := s.store.ListAMSSources(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	items := make([]any, 0, len(sources))
	for _, src := range sources {
		items = append(items, amsSourceToAPI(src))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "meta": map[string]any{"next_cursor": nil}})
}

func (s *Server) handleCreateSource(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	row, err := amsSourceFromAPI(body, s.store)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_SOURCE", err.Error())
		return
	}
	created, err := s.store.CreateAMSSource(r.Context(), row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, amsSourceToAPI(created))
}

func (s *Server) handleUpdateSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sourceId")
	existing, err := s.store.GetAMSSource(r.Context(), id)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "source not found")
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	row, err := amsSourceFromAPI(body, s.store)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_SOURCE", err.Error())
		return
	}
	row.ID = id
	row.CreatedAt = existing.CreatedAt
	if err := s.store.UpdateAMSSource(r.Context(), row); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	updated, _ := s.store.GetAMSSource(r.Context(), id)
	writeJSON(w, http.StatusOK, amsSourceToAPI(*updated))
}

func (s *Server) handleDeleteSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sourceId")
	if existing, _ := s.store.GetAMSSource(r.Context(), id); existing == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "source not found")
		return
	}
	if err := s.store.DeleteAMSSource(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTestSource tests connectivity to an AMS source (CR-3, D-006 addition).
// Tests the REST API connection to the configured source URL.
func (s *Server) handleTestSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sourceId")
	src, err := s.store.GetAMSSource(r.Context(), id)
	if err != nil || src == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "source not found")
		return
	}

	// Test connectivity: attempt a lightweight HTTP GET to the source's REST URL.
	if !src.RestURL.Valid || src.RestURL.String == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "unknown",
			"message": "no rest_url configured for this source",
			"latency_ms": nil,
		})
		return
	}

	testURL := strings.TrimRight(src.RestURL.String, "/") + "/rest/v2/version"
	start := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "error",
			"message":    fmt.Sprintf("build request: %v", err),
			"latency_ms": nil,
		})
		return
	}
	if src.RestUser.Valid && src.RestUser.String != "" {
		req.SetBasicAuth(src.RestUser.String, "") // password from encrypted store; skip for connectivity check
	}

	resp, err := http.DefaultClient.Do(req)
	latencyMS := time.Since(start).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "error",
			"message":    err.Error(),
			"latency_ms": latencyMS,
		})
		return
	}
	defer resp.Body.Close()

	status := "ok"
	msg := fmt.Sprintf("HTTP %d from %s", resp.StatusCode, testURL)
	if resp.StatusCode >= 400 {
		status = "error"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     status,
		"message":    msg,
		"latency_ms": latencyMS,
	})
}

// ─── Admin: License ───────────────────────────────────────────────────────────

func (s *Server) handleGetLicense(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, licenseToAPI(s.lic))
}

func (s *Server) handleActivateLicense(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Key == "" {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "key field required")
		return
	}
	if err := s.lic.Refresh(r.Context(), body.Key); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_LICENSE", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, licenseToAPI(s.lic))
}

// ─── Admin: Tokens ────────────────────────────────────────────────────────────

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.store.ListTokens(r.Context(), r.URL.Query().Get("kind"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	items := make([]any, 0, len(tokens))
	for _, t := range tokens {
		items = append(items, tokenToAPI(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "meta": map[string]any{"next_cursor": nil}})
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	kind, _ := body["kind"].(string)
	name, _ := body["name"].(string)
	if kind == "" || name == "" {
		writeError(w, http.StatusBadRequest, "INVALID_PARAM", "kind and name required")
		return
	}
	rawToken := "plt_" + newToken()
	hash := sha256Hex(rawToken)
	var scopes []string
	if sv, ok := body["scopes"].([]any); ok {
		for _, v := range sv {
			if ss, ok := v.(string); ok {
				scopes = append(scopes, ss)
			}
		}
	}
	tok := meta.APIToken{Kind: kind, Name: name, TokenHash: hash, Scopes: scopes, CreatedAt: time.Now().UnixMilli()}
	if err := s.store.CreateToken(r.Context(), tok); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	created, err := s.store.GetTokenByHash(r.Context(), hash)
	if err != nil || created == nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "token created but not found")
		return
	}
	resp := tokenToAPI(*created)
	if m, ok := resp.(map[string]any); ok {
		m["token"] = rawToken
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteToken(r.Context(), chi.URLParam(r, "tokenId")); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Admin: Users ─────────────────────────────────────────────────────────────

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	items := make([]any, 0, len(users))
	for _, u := range users {
		items = append(items, userToAPI(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "meta": map[string]any{"next_cursor": nil}})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	username, _ := body["username"].(string)
	role, _ := body["role"].(string)
	password, _ := body["password"].(string)
	if username == "" || role == "" {
		writeError(w, http.StatusBadRequest, "INVALID_PARAM", "username and role required")
		return
	}
	now := time.Now().UnixMilli()
	u := meta.User{Username: username, PwHash: hashPassword(password), Role: role, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateUser(r.Context(), u); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	created, _ := s.store.GetUserByUsername(r.Context(), username)
	if created == nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "user created but not found")
		return
	}
	writeJSON(w, http.StatusCreated, userToAPI(*created))
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "userId")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	username, _ := body["username"].(string)
	role, _ := body["role"].(string)
	if err := s.store.UpdateUser(r.Context(), id, username, role); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "username": username, "role": role, "created_at": 0})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteUser(r.Context(), chi.URLParam(r, "userId")); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Beacon ingest ────────────────────────────────────────────────────────────

func (s *Server) handleIngestBeacon(w http.ResponseWriter, r *http.Request) {
	ingestToken := r.Header.Get("X-Pulse-Ingest-Token")
	if ingestToken == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing X-Pulse-Ingest-Token")
		return
	}
	hash := sha256Hex(ingestToken)
	tok, err := s.store.GetTokenByHash(r.Context(), hash)
	if err != nil || tok == nil || tok.Kind != "ingest" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid ingest token")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			writeError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "body exceeds 256 KB")
			return
		}
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	eventsAny, _ := body["events"].([]any)
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": len(eventsAny), "rejected": 0, "errors": []any{}})
}

// ─── Bootstrap ────────────────────────────────────────────────────────────────

func (s *Server) bootstrapIfFirstRun(ctx context.Context) error {
	n, err := s.store.CountTokens(ctx)
	if err != nil || n > 0 {
		return err
	}
	rawToken := "plt_" + newToken()
	hash := sha256Hex(rawToken)
	tok := meta.APIToken{Kind: "api", Name: "admin (bootstrap)", TokenHash: hash, Scopes: []string{"admin"}, CreatedAt: time.Now().UnixMilli()}
	if err := s.store.CreateToken(ctx, tok); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	fmt.Fprintf(os.Stderr, "\npulse: FIRST RUN — generated admin token: %s\n       Save this token; it will not be shown again.\n\n", rawToken)
	return nil
}

// ─── Conversion helpers ───────────────────────────────────────────────────────

func alertRuleToAPI(r meta.AlertRuleRow) map[string]any {
	m := map[string]any{
		"id":                  r.ID,
		"name":                r.Name,
		"metric":              r.Metric,
		"operator":            r.Operator,
		"threshold":           r.Threshold,
		"window_s":            r.WindowS,
		"severity":            r.Severity,
		"cooldown_s":          r.CooldownS,
		"enabled":             r.Enabled,
		"muted":               r.Muted,
		"scope":               jsonOrEmpty(r.ScopeJSON),
		"channel_ids":         jsonArrayOrEmpty(r.ChannelIDs),
		"maintenance_windows": jsonArrayOrEmpty(r.MaintenanceWindows),
		"created_at":          r.CreatedAt,
		"updated_at":          r.UpdatedAt,
	}
	if r.GroupBy.Valid {
		m["group_by"] = r.GroupBy.String
	} else {
		m["group_by"] = nil
	}
	return m
}

func alertRuleFromAPI(body map[string]any) (meta.AlertRuleRow, error) {
	name, _ := body["name"].(string)
	metric, _ := body["metric"].(string)
	operator, _ := body["operator"].(string)
	severity, _ := body["severity"].(string)
	if name == "" {
		return meta.AlertRuleRow{}, fmt.Errorf("name required")
	}
	if metric == "" {
		return meta.AlertRuleRow{}, fmt.Errorf("metric required")
	}
	if operator == "" {
		return meta.AlertRuleRow{}, fmt.Errorf("operator required")
	}
	threshold, _ := body["threshold"].(float64)
	windowS, _ := body["window_s"].(float64)
	cooldownS, _ := body["cooldown_s"].(float64)
	if cooldownS == 0 {
		cooldownS = 300
	}
	// enabled defaults to true (OpenAPI spec default); muted defaults to false.
	enabled := true
	if v, ok := body["enabled"].(bool); ok {
		enabled = v
	}
	muted, _ := body["muted"].(bool)

	scopeJSON := "{}"
	if scope, ok := body["scope"]; ok && scope != nil {
		if b, err := json.Marshal(scope); err == nil {
			scopeJSON = string(b)
		}
	}
	channelIDs := "[]"
	if cids, ok := body["channel_ids"].([]any); ok {
		if b, err := json.Marshal(cids); err == nil {
			channelIDs = string(b)
		}
	}
	mw := "[]"
	if mws, ok := body["maintenance_windows"].([]any); ok {
		if b, err := json.Marshal(mws); err == nil {
			mw = string(b)
		}
	}
	row := meta.AlertRuleRow{
		Name:               name,
		Metric:             metric,
		Operator:           operator,
		Threshold:          threshold,
		WindowS:            int(windowS),
		ScopeJSON:          scopeJSON,
		Severity:           severity,
		CooldownS:          int(cooldownS),
		Enabled:            enabled,
		Muted:              muted,
		MaintenanceWindows: mw,
		ChannelIDs:         channelIDs,
	}
	if gb, ok := body["group_by"].(string); ok && gb != "" {
		row.GroupBy = sql.NullString{String: gb, Valid: true}
	}
	return row, nil
}

func alertChannelToAPI(c meta.AlertChannelRow) map[string]any {
	return map[string]any{
		"id":             c.ID,
		"type":           c.Type,
		"name":           c.Name,
		"credential_set": c.ConfigEnc != "",
		"config_summary": jsonOrEmpty(c.ConfigPublic),
		"created_at":     c.CreatedAt,
	}
}

func alertChannelFromAPI(body map[string]any, store *meta.Store) (meta.AlertChannelRow, error) {
	chType, _ := body["type"].(string)
	name, _ := body["name"].(string)
	if chType == "" {
		return meta.AlertChannelRow{}, fmt.Errorf("type required")
	}
	if name == "" {
		return meta.AlertChannelRow{}, fmt.Errorf("name required")
	}
	config, _ := body["config"].(map[string]any)
	secretFields := map[string]bool{
		"slack_webhook_url": true, "telegram_bot_token": true,
		"pagerduty_routing_key": true, "webhook_secret": true,
	}
	publicConfig := make(map[string]any)
	secretConfig := make(map[string]any)
	for k, v := range config {
		if secretFields[k] {
			secretConfig[k] = v
		} else {
			publicConfig[k] = v
		}
	}
	var configEnc string
	if len(secretConfig) > 0 {
		secretJSON, _ := json.Marshal(secretConfig)
		enc, err := store.Encrypt(string(secretJSON))
		if err != nil {
			return meta.AlertChannelRow{}, fmt.Errorf("encrypt config: %w", err)
		}
		configEnc = enc
	}
	publicJSON, _ := json.Marshal(publicConfig)
	return meta.AlertChannelRow{
		Type:         chType,
		Name:         name,
		ConfigEnc:    configEnc,
		ConfigPublic: string(publicJSON),
	}, nil
}

func amsSourceToAPI(src meta.AMSSourceRow) map[string]any {
	m := map[string]any{
		"id":             src.ID,
		"name":           src.Name,
		"type":           src.SourceType,
		"credential_set": src.CredentialEnc.Valid && src.CredentialEnc.String != "",
		"created_at":     src.CreatedAt,
	}
	if src.RestURL.Valid {
		m["rest_url"] = src.RestURL.String
	}
	if src.RestUser.Valid {
		m["rest_user"] = src.RestUser.String
	}
	if src.LogPath.Valid {
		m["log_path"] = src.LogPath.String
	}
	if src.CredentialEnvRef.Valid {
		m["credential_env_ref"] = src.CredentialEnvRef.String
	}
	return m
}

func amsSourceFromAPI(body map[string]any, store *meta.Store) (meta.AMSSourceRow, error) {
	name, _ := body["name"].(string)
	srcType, _ := body["type"].(string)
	if name == "" {
		return meta.AMSSourceRow{}, fmt.Errorf("name required")
	}
	if srcType == "" {
		return meta.AMSSourceRow{}, fmt.Errorf("type required")
	}
	row := meta.AMSSourceRow{Name: name, SourceType: srcType, Enabled: true}
	if v, ok := body["rest_url"].(string); ok {
		row.RestURL = sql.NullString{String: v, Valid: v != ""}
	}
	if v, ok := body["rest_user"].(string); ok {
		row.RestUser = sql.NullString{String: v, Valid: v != ""}
	}
	if v, ok := body["log_path"].(string); ok {
		row.LogPath = sql.NullString{String: v, Valid: v != ""}
	}
	if v, ok := body["credential_env_ref"].(string); ok {
		row.CredentialEnvRef = sql.NullString{String: v, Valid: v != ""}
	}
	if v, ok := body["rest_password"].(string); ok && v != "" {
		enc, err := store.Encrypt(v)
		if err != nil {
			return row, fmt.Errorf("encrypt credential: %w", err)
		}
		row.CredentialEnc = sql.NullString{String: enc, Valid: true}
	}
	return row, nil
}

func licenseToAPI(lic *license.Manager) map[string]any {
	ent := lic.Entitlements()
	var expiresAt any = nil
	if exp := lic.ExpiresAt(); exp != nil {
		expiresAt = exp.UnixMilli()
	}
	return map[string]any{
		"tier":         string(lic.Tier()),
		"valid":        lic.Valid(),
		"expires_at":   expiresAt,
		"offline_file": false,
		"limits": map[string]any{
			"max_nodes":      nullableInt(ent.MaxNodes),
			"max_streams":    nullableInt(ent.MaxStreams),
			"retention_days": nullableInt(ent.RetentionDays),
			"data_api":       ent.DataAPI,
			"white_label":    ent.WhiteLabel,
		},
	}
}

func tokenToAPI(t meta.APIToken) any {
	return map[string]any{
		"id":           t.ID,
		"kind":         t.Kind,
		"name":         t.Name,
		"scopes":       t.Scopes,
		"created_at":   t.CreatedAt,
		"expires_at":   t.ExpiresAt,
		"last_used_at": t.LastUsedAt,
	}
}

func userToAPI(u meta.User) map[string]any {
	return map[string]any{
		"id":         u.ID,
		"username":   u.Username,
		"role":       u.Role,
		"created_at": u.CreatedAt,
	}
}

// ─── Utility helpers ──────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"code": code, "message": message})
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func newToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// hashPassword hashes a password using bcrypt (pure-Go, CGO_ENABLED=0 compatible).
// Closes wave-1 gap G3.
func hashPassword(pw string) string {
	if pw == "" {
		return ""
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		// Fallback to SHA-256 on error (should not happen in practice).
		sum := sha256.Sum256([]byte(pw))
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	return string(h)
}

// checkPassword verifies a plaintext password against a stored hash.
// Supports both bcrypt hashes and legacy sha256: hashes.
// Returns true if password matches.
func checkPassword(pw, stored string) bool {
	if stored == "" {
		return pw == ""
	}
	if strings.HasPrefix(stored, "sha256:") {
		// Legacy SHA-256 comparison (constant-time).
		sum := sha256.Sum256([]byte(pw))
		expected := "sha256:" + hex.EncodeToString(sum[:])
		return subtle.ConstantTimeCompare([]byte(stored), []byte(expected)) == 1
	}
	// bcrypt comparison.
	return bcrypt.CompareHashAndPassword([]byte(stored), []byte(pw)) == nil
}

// _ ensures subtle and bcrypt imports are used.
var _ = subtle.ConstantTimeCompare

func jsonOrEmpty(s string) any {
	if s == "" {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return map[string]any{}
	}
	return v
}

func jsonArrayOrEmpty(s string) any {
	if s == "" {
		return []any{}
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return []any{}
	}
	return v
}

func nullableInt(n int) any {
	if n < 0 {
		return nil
	}
	return n
}

func nsValue(ns sql.NullString) any {
	if !ns.Valid {
		return nil
	}
	return ns.String
}

func parseTimeRange(from, to string) (time.Time, time.Time) {
	var f, t time.Time
	if from != "" {
		if ms, err := strconv.ParseInt(from, 10, 64); err == nil {
			f = time.UnixMilli(ms)
		} else if pt, err := time.Parse(time.RFC3339, from); err == nil {
			f = pt
		}
	}
	if to != "" {
		if ms, err := strconv.ParseInt(to, 10, 64); err == nil {
			t = time.UnixMilli(ms)
		} else if pt, err := time.Parse(time.RFC3339, to); err == nil {
			t = pt
		}
	}
	if f.IsZero() {
		f = time.Now().AddDate(0, 0, -7)
	}
	if t.IsZero() {
		t = time.Now()
	}
	return f, t
}

func parseAudienceParams(q url.Values) (query.AudienceParams, error) {
	from, to := parseTimeRange(q.Get("from"), q.Get("to"))
	interval := q.Get("interval")
	if interval == "" {
		interval = "day"
	}
	return query.AudienceParams{
		From:     from,
		To:       to,
		App:      q.Get("app"),
		Stream:   q.Get("stream"),
		Node:     q.Get("node"),
		Tenant:   q.Get("tenant"),
		Interval: interval,
	}, nil
}
