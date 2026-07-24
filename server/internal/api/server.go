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
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/alert/channels"
	"github.com/pulse-analytics/pulse/server/internal/collector/beacon"
	"github.com/pulse-analytics/pulse/server/internal/collector/ingest"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/reports"
	"github.com/pulse-analytics/pulse/server/internal/ssrfguard"
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
	// AllowedWSOrigins is the list of allowed WebSocket origin patterns.
	// Patterns follow nhooyr.io/websocket glob syntax (e.g. "https://*.example.com").
	// Empty slice means same-origin only (most restrictive).
	// Set to []string{"*"} for development only.
	// VD-S2: replaces the removed InsecureSkipVerify=true.
	AllowedWSOrigins []string
	// WebDir is the directory of the built web UI (index.html + assets/).
	// When set and present, the server serves the SPA and its static assets,
	// falling back to index.html for client-side routes. Empty (or an absent
	// dir) disables static serving so API-only and test deployments keep 404s.
	WebDir string
	// CORSAllowedOrigins is the list of origins permitted on /api/v1/* routes.
	// When set, corsMiddleware echoes the matching request Origin and adds
	// Vary: Origin. When empty, no Access-Control-Allow-Origin header is emitted
	// for API/admin routes (same-origin browser requests still work without CORS).
	// The beacon ingest route (/ingest/beacon) is always permissive regardless.
	// Corresponds to PULSE_CORS_ALLOWED_ORIGINS (comma-separated).
	CORSAllowedOrigins []string

	// BeaconRateRPSOverride and BeaconBurstOverride, when both non-zero, replace
	// the production beacon rate-limit defaults (100 rps / 200 burst) with the
	// provided values. Intended exclusively for tests that need a tiny burst so
	// that a small number of requests reliably exhausts the bucket without relying
	// on wall-clock timing. Never set in production (serve.go leaves them zero).
	BeaconRateRPSOverride float64
	BeaconBurstOverride   float64

	// OIDCConfig, when non-nil, enables the /auth/oidc/* endpoints.
	// OIDCConfig.Provider must be pre-built by the caller (serve.go or test).
	// Nil = OIDC disabled (no behaviour change to existing auth).
	// S11 WO-C: SSO/OIDC phase 1.
	OIDCConfig *OIDCProviderConfig

	// AMSEnvConfigured reports whether PULSE_AMS_URL was explicitly set (i.e. the
	// operator configured an AMS connection via the environment rather than the
	// ams_sources table). Surfaced on /healthz so the web UI can tell an
	// env-configured-but-source-empty deployment apart from a truly fresh install
	// and not push a running operator into the onboarding wizard.
	AMSEnvConfigured bool
}

// KafkaStatsProvider is the interface to the Kafka source for health reporting.
// Implemented by *kafkasrc.Source (see server/internal/collector/kafka).
// The API layer uses this to populate the kafka component in /healthz.
type KafkaStatsProvider interface {
	// Lag returns the last observed consumer lag across all topic-partitions.
	Lag() int64
	// ParseErrors returns the count of malformed messages since start.
	ParseErrors() int64
}

// AnomalyDetector is the interface to the anomaly detection service.
// Implemented by *anomaly.Detector.
type AnomalyDetector interface {
	// ComputeFlags returns current anomaly flags above sigmaThreshold.
	// If sigmaThreshold <= 0, the configured default is used.
	ComputeFlags(ctx context.Context, sigmaThreshold float64) ([]AnomalyFlagAPI, error)
}

// ErrBadCursor is a sentinel returned (possibly wrapped) by FlagHistoryQuerier
// implementations when the cursor parameter cannot be decoded.
// handleAnomalies maps this to HTTP 400 BAD_REQUEST rather than 500.
// ADR-0009 §6: base64("<detected_at_ms>:<id>") cursor namespace.
var ErrBadCursor = errors.New("bad cursor")

// FlagHistoryQuerier queries the persisted anomaly flag-event store.
// Separate from AnomalyDetector to preserve the single-method interface
// and avoid breaking existing test fakes (blast-radius argument; see ADR-0009 §B).
// ADR-0009 §6.
type FlagHistoryQuerier interface {
	// QueryFlagHistory returns a page of anomaly flag events in [from, to].
	// Zero from/to = unbounded side. cursor = "" means first page.
	// ADR AMENDMENT (D-086): carries metric + minSigma so /anomalies ?metric
	// and ?min_sigma remain honest on the history path.
	QueryFlagHistory(ctx context.Context,
		from, to time.Time,
		metric, app, stream string,
		minSigma float64,
		limit int,
		cursor string,
	) (FlagHistoryPage, error)
}

// FlagHistoryPage is a single page of QueryFlagHistory results.
// NextCursor "" = last page (serialize as JSON null per spec [string,null]).
type FlagHistoryPage struct {
	Items      []AnomalyFlagAPI
	NextCursor string // empty on last page
}

// AnomalyFlagAPI is the API representation of an anomaly flag.
// Mirrors the AnomalyFlag schema in contracts/openapi/pulse-api.yaml.
type AnomalyFlagAPI struct {
	ID       string            `json:"id"`
	Metric   string            `json:"metric"`
	Scope    domain.AlertScope `json:"scope"`
	Observed float64           `json:"observed"`
	Expected float64           `json:"expected"`
	Sigma    float64           `json:"sigma"`
	TS       int64             `json:"ts"`
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

	// sourceMu serializes the count→CheckNodeLimit→create critical section in
	// handleCreateSource so concurrent requests cannot race past the node-limit
	// gate (D-041). Sufficient for the single-binary deployment; a multi-instance
	// deployment would additionally need a DB-level constraint.
	sourceMu sync.Mutex

	// VD-10: event sink for main-port /ingest/beacon persistence (optional).
	// When set, beacon events POSTed to the main API port are written to the sink
	// (same fanout as the dedicated beacon handler). Without this, beacon events
	// posted to the main port would be silently discarded.
	eventSink domain.EventSink

	// Wave 2: ingest tracker for QoE endpoints (optional).
	ingestTracker IngestTracker

	// D-164: upstream poll-freshness source for /healthz (optional).
	// nil keeps the pre-D-164 liveness-only collector semantics, so every
	// constructor and test that does not wire a collector still compiles and
	// reports "ok".
	collectorHealth domain.CollectorHealth

	// BUG-004: ingest querier used exclusively by handleIngestHealth.
	// Defaults to qsvc; tests may override via SetIngestQuerier to inject a
	// capture double without replacing the concrete *query.Service everywhere.
	iqsvc IngestQuerier

	// Wave 2: reports generator (optional — requires ClickHouse for real data).
	reportGen *reports.Generator

	// Wave 3: anomaly detector (optional — wired in serve.go).
	anomalyDetector AnomalyDetector

	// BUG-008 phase 2 (ADR-0009): flag-event store querier for GET /anomalies ?from/?to.
	// Nil until wired; when nil, ?from/?to returns 400 FLAG_STORE_NOT_CONFIGURED.
	flagHistoryQuerier FlagHistoryQuerier

	// Wave 3: kafka stats provider for /healthz (optional — wired in serve.go).
	kafkaStats KafkaStatsProvider

	// Rate limiters (A2, A7).
	beaconLimiter  *keyedLimiter // per ingest-token; A2
	metricsLimiter *keyedLimiter // per client IP;   A7

	// eviction stop functions for the limiter background goroutines.
	// Started in Start(), stopped in Stop().
	stopBeaconEvict  func()
	stopMetricsEvict func()

	// WS hub
	wsMu    sync.Mutex
	wsConns map[*wsConn]struct{}
	wsStop  func()

	// S11 WO-C: OIDC handler (nil when OIDC is not configured).
	oidc *oidcHandler
}

// IngestTracker is the interface to the collector/ingest.HealthTracker.
// Minimal subset used by the API layer.
// VD-23: return type matches ingest.HealthTracker.Snapshot() → map[string]PublisherState.
type IngestTracker interface {
	// Snapshot returns a copy of all publisher states keyed by
	// "nodeID/app/streamID". Return type must match ingest.HealthTracker.Snapshot().
	Snapshot() map[string]ingest.PublisherState
}

// IngestQuerier is the subset of query.Service used by handleIngestHealth.
// Keeping it narrow lets tests inject a capture double without replacing the
// full *query.Service. *query.Service satisfies this interface automatically.
type IngestQuerier interface {
	IngestTimeseries(ctx context.Context, p query.IngestTimeseriesParams) (*query.IngestTimeseriesResult, error)
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
		// A2: per-token limiter for main-port beacon ingest.
		// Allow test overrides so unit tests can use a tiny burst without relying
		// on wall-clock timing under the race detector.
		beaconLimiter: func() *keyedLimiter {
			rps, burst := mainBeaconRateRPS, mainBeaconBurst
			if cfg.BeaconRateRPSOverride > 0 && cfg.BeaconBurstOverride > 0 {
				rps, burst = cfg.BeaconRateRPSOverride, cfg.BeaconBurstOverride
			}
			return newKeyedLimiter(rps, burst)
		}(),
		// A7: per-IP limiter for /metrics scrape.
		metricsLimiter: newKeyedLimiter(metricsRateRPS, metricsBurst),
	}
	// BUG-004: default the ingest querier to qsvc.  Guard the nil case explicitly
	// so that a nil *query.Service does not become a non-nil interface value
	// (Go nil-interface vs nil-pointer distinction).
	if qsvc != nil {
		s.iqsvc = qsvc
	}
	// S11 WO-C: initialise OIDC handler if a pre-built provider was injected.
	if cfg.OIDCConfig != nil && cfg.OIDCConfig.Provider != nil {
		s.oidc = newOIDCHandler(cfg.OIDCConfig, store, logger)
	}
	// S101 (REVIEW-EXT-2026-07-24): /metrics is served unauthenticated when no
	// token is configured. That is deliberate (prod publishes loopback-only and
	// Prometheus scrapes unauthenticated by default), but it must not be silent.
	if cfg.MetricsToken == "" {
		logger.Warn("PULSE_METRICS_TOKEN is not set — /metrics is served without authentication; " +
			"set PULSE_METRICS_TOKEN to require a bearer token on scrapes")
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

// SetEventSink wires the event sink so that the main-port /ingest/beacon handler
// persists events to ClickHouse + aggregator (VD-10).
// Call after New, before Start. Without this call the main-port handler still
// validates and authenticates beacons but cannot write them; the dedicated
// beacon server (PULSE_INGEST_LISTEN_ADDR) always has its own sink.
func (s *Server) SetEventSink(sink domain.EventSink) {
	s.eventSink = sink
}

// SetIngestTracker wires the ingest health tracker for QoE endpoints.
// Call after New, before Start.
func (s *Server) SetIngestTracker(tracker IngestTracker) {
	s.ingestTracker = tracker
}

// SetCollectorHealth wires the upstream poll-freshness source used by /healthz
// (D-164). Call after New, before Start. Leaving it unset keeps the collector
// component on liveness-only semantics.
func (s *Server) SetCollectorHealth(h domain.CollectorHealth) {
	s.collectorHealth = h
}

// SetIngestQuerier overrides the IngestQuerier used by handleIngestHealth.
// Call after New, before Start.  Intended for tests that need to inject a
// capture double without replacing the full *query.Service.
func (s *Server) SetIngestQuerier(iq IngestQuerier) {
	s.iqsvc = iq
}

// SetReportGenerator wires the reports generator (F6).
// Call after New, before Start.
func (s *Server) SetReportGenerator(gen *reports.Generator) {
	s.reportGen = gen
}

// SetAnomalyDetector wires the anomaly detector (F9, Wave 3).
// Call after New, before Start.
func (s *Server) SetAnomalyDetector(det AnomalyDetector) {
	s.anomalyDetector = det
}

// SetFlagHistoryQuerier wires the flag-event store for GET /anomalies ?from/?to.
// Call after New, before Start. Follows the SetIngestQuerier precedent
// (server/internal/api/server.go:275). ADR-0009 §6.
func (s *Server) SetFlagHistoryQuerier(q FlagHistoryQuerier) {
	s.flagHistoryQuerier = q
}

// SetKafkaStats wires the Kafka stats provider for /healthz (VD-27).
// Call after New, before Start. When not set, /healthz omits the kafka component.
func (s *Server) SetKafkaStats(p KafkaStatsProvider) {
	s.kafkaStats = p
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

	// Start rate-limiter eviction goroutines (A2, A7).
	// 5-minute sweep, evict buckets idle for 10 minutes.
	s.stopBeaconEvict = s.beaconLimiter.startEviction(5*time.Minute, 10*time.Minute)
	s.stopMetricsEvict = s.metricsLimiter.startEviction(5*time.Minute, 10*time.Minute)

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
	// Stop rate-limiter eviction goroutines (A2, A7).
	if s.stopBeaconEvict != nil {
		s.stopBeaconEvict()
	}
	if s.stopMetricsEvict != nil {
		s.stopMetricsEvict()
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
	r.Use(s.corsMiddleware)
	r.Use(middleware.Recoverer)

	// Operational (unauthenticated).
	r.Get("/healthz", s.handleHealthz)
	r.Get("/metrics", s.handleMetrics)

	// Beacon ingest.
	r.Post("/ingest/beacon", s.handleIngestBeacon)

	// API v1 — bearer auth required.
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.bearerAuthMiddleware)
		r.Use(s.requireWriteScope)

		r.Get("/live/overview", s.handleLiveOverview)
		r.Get("/live/streams", s.handleLiveStreams)
		// /live/ws is registered under downloadAuthMiddleware (below) — browsers
		// cannot set an Authorization header on a WebSocket, so it needs the same
		// header/cookie/?token= auth as file downloads.

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

		// Tenant management (F6 multi-tenant billing): D-010 APPROVED CR, spec amended by INT-01.
		// Business-tier (Enterprise) gated. All 5 ops per contracts/openapi/pulse-api.yaml.
		r.Get("/admin/tenants", s.handleListTenants)
		r.Post("/admin/tenants", s.handleCreateTenant)
		r.Get("/admin/tenants/{tenantId}", s.handleGetTenant)
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

		// Audit trail (S40 / D-102): read-only view of who changed what, when.
		// Capture happens in the mutating handlers above via s.audit(...).
		r.Get("/admin/audit-log", s.handleListAuditLog)
	})

	// Browser-initiated routes that cannot attach an Authorization header: file
	// downloads (window.location.href) and the WebSocket upgrade. Both use
	// downloadAuthMiddleware, which accepts header, pulse_session cookie, or
	// ?token= and validates the token into ctxTokenKey. A4 security: ?token= is
	// intentionally NOT accepted on normal /api/v1/* routes (bearerAuthMiddleware
	// enforces this). WS is a read stream (GET), so no requireWriteScope is needed.
	r.Group(func(r chi.Router) {
		r.Use(s.downloadAuthMiddleware)
		r.Get("/api/v1/reports/export", s.handleReportExport)
		r.Get("/api/v1/live/ws", s.handleLiveWS)
	})

	// S11 WO-C: OIDC/SSO auth (non-versioned, no bearer auth required).
	// Routes are always registered; handlers return 501 when OIDC is not configured.
	r.Get("/auth/oidc/login", s.handleOIDCLogin)
	r.Get("/auth/oidc/callback", s.handleOIDCCallback)
	r.Post("/auth/oidc/logout", s.handleOIDCLogout)

	// S14 WO-C: OIDC phase-2 discovery + identity endpoints.
	// /auth/oidc/status is unauthenticated (SPA mount-time discovery).
	// /auth/me uses the standard bearer middleware (cookie fallback included).
	r.Get("/auth/oidc/status", s.handleOIDCStatus)
	r.Group(func(r chi.Router) {
		r.Use(s.bearerAuthMiddleware)
		r.Get("/auth/me", s.handleAuthMe)
	})

	// Static serving of the built web UI (SPA). Registered after the API routes
	// (so they take precedence) and gated on the assets being present, so
	// API-only and test builds keep clean 404s.
	s.mountWebUI(r)

	s.router = r
}

// mountWebUI serves the built React SPA from s.cfg.WebDir: hashed assets under
// /assets/*, and an index.html fallback for any unmatched non-API GET so deep
// links (e.g. /live, /dashboard) resolve via client-side routing. It is a no-op
// when WebDir is unset or its index.html is absent.
func (s *Server) mountWebUI(r chi.Router) {
	webDir := s.cfg.WebDir
	if webDir == "" {
		return
	}
	indexPath := webDir + "/index.html"
	if _, err := os.Stat(indexPath); err != nil {
		s.logger.Warn("api: web UI assets not found; static serving disabled", "dir", webDir, "error", err)
		return
	}

	fileServer := http.FileServer(http.Dir(webDir))
	// Hashed, immutable build assets (e.g. /assets/index-*.js).
	r.Handle("/assets/*", fileServer)
	// Root-level static files emitted by the build.
	r.Get("/favicon.ico", fileServer.ServeHTTP)

	// SPA fallback: serve index.html for unmatched GETs that are not API,
	// ingest, or operational paths (those keep their own handlers / 404s).
	// Real files under webDir (favicon.svg, /icons/*, /logo/*, site.webmanifest —
	// the brandkit build's root-level assets) are served as themselves FIRST:
	// before D-076b every such path returned index.html as text/html, so the
	// browser rendered a broken tab icon (found live at the v0.3.0 accept).
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		p := req.URL.Path
		if req.Method != http.MethodGet ||
			strings.HasPrefix(p, "/api/") ||
			strings.HasPrefix(p, "/ingest/") ||
			p == "/healthz" || p == "/metrics" {
			http.NotFound(w, req)
			return
		}
		if clean := path.Clean("/" + p); clean != "/" && !strings.Contains(clean, "..") {
			if fi, err := os.Stat(filepath.Join(webDir, filepath.FromSlash(clean))); err == nil && !fi.IsDir() {
				fileServer.ServeHTTP(w, req)
				return
			}
		}
		http.ServeFile(w, req, indexPath)
	})
	s.logger.Info("api: serving web UI", "dir", webDir)
}

// ─── Auth middleware ───────────────────────────────────────────────────────────

type contextKey string

const ctxTokenKey contextKey = "api_token"

// ctxAuthMethodKey stashes how the request was authenticated: "bearer" when the
// Authorization header was used, "cookie" when the pulse_session cookie was used.
// Set by bearerAuthMiddleware; read by handleAuthMe.
const ctxAuthMethodKey contextKey = "api_auth_method"

// ─── OIDC route handlers ──────────────────────────────────────────────────────

// handleOIDCLogin is the server-level wrapper for the OIDC login handler.
// Returns 501 NOT_CONFIGURED when OIDC is not enabled.
func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		writeError(w, http.StatusNotImplemented, "NOT_CONFIGURED", "OIDC is not configured on this server")
		return
	}
	if err := s.lic.CheckSSO(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	s.oidc.handleLogin(w, r)
}

// handleOIDCCallback is the server-level wrapper for the OIDC callback handler.
// Returns 501 NOT_CONFIGURED when OIDC is not enabled.
func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		writeError(w, http.StatusNotImplemented, "NOT_CONFIGURED", "OIDC is not configured on this server")
		return
	}
	if err := s.lic.CheckSSO(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	s.oidc.handleCallback(w, r)
}

// handleOIDCLogout is the server-level wrapper for the OIDC logout handler.
// Returns 501 NOT_CONFIGURED when OIDC is not enabled.
func (s *Server) handleOIDCLogout(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		writeError(w, http.StatusNotImplemented, "NOT_CONFIGURED", "OIDC is not configured on this server")
		return
	}
	s.oidc.handleLogout(w, r)
}

func (s *Server) bearerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		// S14 WO-C: track how the token was supplied so handleAuthMe can reflect it.
		authMethod := "bearer"
		// S11 WO-C: fall back to pulse_session cookie when no Authorization header.
		// The cookie is set by the OIDC callback; existing bearer flows are unchanged.
		if token == "" {
			if c, err := r.Cookie("pulse_session"); err == nil {
				token = c.Value
				authMethod = "cookie"
			}
		}
		if token == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid Authorization header")
			return
		}
		tok, err := s.store.LookupToken(r.Context(), token)
		if err != nil || tok == nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token")
			return
		}
		if tok.ExpiresAt != nil && *tok.ExpiresAt < time.Now().UnixMilli() {
			writeError(w, http.StatusUnauthorized, "TOKEN_EXPIRED", "token expired")
			return
		}
		// VD-S3: enforce token kind — admin/API routes require kind='api'.
		// Ingest tokens (kind='ingest') must not be accepted on /api/v1/* routes.
		// The /ingest/beacon route validates kind='ingest' independently.
		if tok.Kind != "api" {
			writeError(w, http.StatusForbidden, "WRONG_TOKEN_KIND",
				fmt.Sprintf("this route requires an API token (kind=api); got kind=%q", tok.Kind))
			return
		}
		go s.store.TouchToken(context.Background(), tok.ID)
		ctx := context.WithValue(r.Context(), ctxTokenKey, tok)
		ctx = context.WithValue(ctx, ctxAuthMethodKey, authMethod)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireWriteScope denies mutating requests (POST, PUT, PATCH, DELETE) to any
// token that is not permitted to write. GET, HEAD and OPTIONS always pass, so a
// read-only token can still drive the entire UI.
//
// The rule is a positive one — a token writes only if canWrite says so. Stating
// it as a blocklist ("deny viewer") is what made the first version of this
// middleware useless: the Settings UI mints its tokens with scope "read", which
// no blocklist of role names anticipated, so every UI-minted token kept full
// write access — including POST /admin/tokens, which lets it mint itself an
// admin token. Enumerate what may write, never what may not.
func (s *Server) requireWriteScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		tok, _ := r.Context().Value(ctxTokenKey).(*meta.APIToken)
		if tok == nil {
			// bearerAuthMiddleware runs first and never forwards without a token;
			// if that ordering ever changes, fail closed rather than open.
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid Authorization header")
			return
		}
		if !canWrite(tok.Scopes) {
			writeError(w, http.StatusForbidden, "FORBIDDEN",
				`this token is read-only; the operation requires a token with the "admin" scope`)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// canWrite reports whether a token's scopes permit mutating requests.
//
// An empty scope list is grandfathered as admin: handleCreateToken leaves Scopes
// nil when a caller omits the field, so tokens minted before scopes were enforced
// carry none, and the store cannot distinguish "no scope recorded" from "no scope
// required". Rejecting them would lock an operator out of their own instance.
// Any explicit scope other than "admin" ("read", "viewer", …) is read-only.
// Since S101, handleCreateToken defaults omitted scopes on NEW api tokens to
// ["read"], so the scopeless-admin case below is reachable only for tokens that
// predate that change.
func canWrite(scopes []string) bool {
	if len(scopes) == 0 {
		return true
	}
	for _, s := range scopes {
		if s == "admin" {
			return true
		}
	}
	return false
}

// downloadAuthMiddleware is like bearerAuthMiddleware but additionally accepts a
// ?token= query parameter as a last resort. It is used only for file-download
// routes (e.g. GET /api/v1/reports/export) where the browser issues the request
// via window.location.href and cannot attach an Authorization header.
//
// A4 security: normal /api/v1/* routes continue to use bearerAuthMiddleware,
// which rejects ?token= (TestTokenInURL_Ignored guards this). Only routes
// explicitly opted into this middleware accept token-in-URL.
func (s *Server) downloadAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			if c, err := r.Cookie("pulse_session"); err == nil {
				token = c.Value
			}
		}
		if token == "" {
			// WebSocket clients cannot set an Authorization header, so the browser sends the
			// bearer token as a Sec-WebSocket-Protocol subprotocol — a handshake HEADER, not the
			// URL query — so it stays out of proxy access logs (S73/D-140 [7]).
			token = wsSubprotocolToken(r)
		}
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		if token == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid Authorization header")
			return
		}
		tok, err := s.store.LookupToken(r.Context(), token)
		if err != nil || tok == nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token")
			return
		}
		if tok.ExpiresAt != nil && *tok.ExpiresAt < time.Now().UnixMilli() {
			writeError(w, http.StatusUnauthorized, "TOKEN_EXPIRED", "token expired")
			return
		}
		if tok.Kind != "api" {
			writeError(w, http.StatusForbidden, "WRONG_TOKEN_KIND",
				fmt.Sprintf("this route requires an API token (kind=api); got kind=%q", tok.Kind))
			return
		}
		go s.store.TouchToken(context.Background(), tok.ID)
		ctx := context.WithValue(r.Context(), ctxTokenKey, tok)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// extractBearerToken reads the bearer token from the Authorization header only.
// Query-parameter token (?token=) is intentionally NOT supported here to prevent
// tokens from leaking into access logs, proxy caches, and browser history.
// The only exception is file-download routes that use downloadAuthMiddleware
// and the WebSocket upgrade handler (handleLiveWS), which reads ?token= directly.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

// wsBearerSubprotocol is the marker the browser sends as the FIRST Sec-WebSocket-Protocol
// value alongside the bearer token, so the token travels in the WebSocket handshake header
// rather than the URL (browsers cannot set an Authorization header on a WS upgrade, and a
// ?token= query lands in proxy access logs — S73/D-140 [7]). The server negotiates/echoes
// this marker; the token is the other offered subprotocol.
const wsBearerSubprotocol = "pulse.v1"

// wsSubprotocolToken extracts the bearer token a WebSocket client passed via the
// Sec-WebSocket-Protocol header (format: "pulse.v1, <token>"). Returns the first offered
// subprotocol that is not the wsBearerSubprotocol marker, or "" if none.
func wsSubprotocolToken(r *http.Request) string {
	for _, hdr := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, v := range strings.Split(hdr, ",") {
			if v = strings.TrimSpace(v); v != "" && v != wsBearerSubprotocol {
				return v
			}
		}
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

// corsMiddleware handles CORS headers for all routes.
//
// Beacon ingest (/ingest/beacon) is permissive: it always echoes the request
// Origin so third-party pages can POST beacons from any origin.
//
// API/admin routes (/api/v1/*) are allowlist-controlled:
//   - If CORSAllowedOrigins is non-empty and the request Origin matches, the
//     exact Origin is echoed and Vary: Origin is added.
//   - If the allowlist is empty, or the Origin does not match, no
//     Access-Control-Allow-Origin header is emitted. Same-origin browser
//     requests still work because they do not require CORS headers.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Beacon ingest: always permissive (browsers can't set custom headers on
		// cross-origin beacons without a permissive CORS response).
		if strings.HasPrefix(r.URL.Path, "/ingest/") {
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Pulse-Ingest-Token")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// All other routes (API, healthz, metrics, static): allowlist-controlled.
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Pulse-Ingest-Token")
		if origin != "" && len(s.cfg.CORSAllowedOrigins) > 0 {
			for _, allowed := range s.cfg.CORSAllowedOrigins {
				if allowed == origin {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
					break
				}
			}
		}
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

	// Collector: the live provider must have a snapshot AND that snapshot must
	// still be fed by a reachable AMS.
	//
	// D-164: existence alone is not health. The aggregator keeps its last
	// snapshot forever, so a collector whose every poll fails stayed "ok"
	// indefinitely — a 7 h 46 m production collection outage was reported
	// healthy end to end, and the deploy health-gate passed on it. When a
	// freshness source is wired, age out the component.
	collectorStatus := "ok"
	var collectorMsg any
	switch {
	case s.live == nil || s.live.CurrentSnapshot() == nil:
		collectorStatus = "degraded"
		collectorMsg = "no live snapshot yet"
	case s.collectorHealth != nil:
		h := s.collectorHealth.PollHealth()
		// Before the first success, age from StartedAt: a collector that has
		// NEVER reached AMS must not read as healthy.
		ref := h.LastSuccess
		if ref.IsZero() {
			ref = h.StartedAt
		}
		if h.StaleAfter > 0 && !ref.IsZero() {
			if age := time.Since(ref); age > h.StaleAfter {
				collectorStatus = "degraded"
				msg := fmt.Sprintf("no successful AMS poll for %s", age.Truncate(time.Second))
				if h.LastError != "" {
					msg += ": " + h.LastError
				}
				collectorMsg = msg
			}
		}
	}
	components["collector"] = map[string]any{"status": collectorStatus, "latency_ms": nil, "message": collectorMsg}

	// Kafka: report lag and parse_errors when a stats provider is wired (VD-27).
	hasDegradedNonCritical := collectorStatus == "degraded"
	if s.kafkaStats != nil {
		lag := s.kafkaStats.Lag()
		parseErrors := s.kafkaStats.ParseErrors()
		kafkaStatus := "ok"
		if parseErrors > 0 || lag > 10000 {
			kafkaStatus = "degraded"
		}
		if kafkaStatus == "degraded" {
			hasDegradedNonCritical = true
		}
		components["kafka"] = map[string]any{
			"status":       kafkaStatus,
			"lag":          lag,
			"parse_errors": parseErrors,
		}
	}

	overallStatus := "ok"
	if !overallOK {
		overallStatus = "down"
	} else if hasDegradedNonCritical {
		// Non-critical component degraded (collector or kafka) while ClickHouse + meta_store are ok.
		// Report degraded at overall level but keep HTTP 200 (only down -> 503).
		overallStatus = "degraded"
	}

	httpStatus := http.StatusOK
	if !overallOK {
		httpStatus = http.StatusServiceUnavailable
	}

	writeJSON(w, httpStatus, map[string]any{
		"status":             overallStatus,
		"components":         components,
		"ams_env_configured": s.cfg.AMSEnvConfigured,
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// A7: per-IP scrape rate limit (10 rps / burst 20). Applied before the token
	// check so an unauthenticated flood is throttled ahead of the constant-time
	// compare and the live-snapshot computation. Keyed on clientIP(r); see the
	// clientIP doc for the RealIP-middleware interaction.
	if !s.metricsLimiter.Allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "metrics scrape rate limit exceeded")
		return
	}
	if err := s.lic.CheckPrometheus(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	if s.cfg.MetricsToken != "" {
		// VD-S1: use constant-time compare to prevent timing oracle attacks.
		// ARCHITECTURE §6: all auth comparisons must be constant-time.
		provided := extractBearerToken(r)
		if subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.MetricsToken)) != 1 {
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
		hist, err := s.store.ListAlertHistory(ctx, "", "firing", 0, 0, 1000, "")
		firingCount := 0
		if err == nil {
			firingCount = len(hist)
		}
		fmt.Fprintf(w, "# HELP pulse_alerts_firing Total firing alert count\n# TYPE pulse_alerts_firing gauge\npulse_alerts_firing %d\n", firingCount)
	}

	// ── Collector freshness (ROADMAP §2.45) ───────────────────────────────
	// The D-164 outage passed unnoticed because nothing pages when the collector
	// goes blind: the alert engine evaluates metrics DERIVED from the collector,
	// so when it stops there is nothing to evaluate. Exposing the poll freshness
	// as a scrape metric lets a Prometheus user close that gap themselves —
	//   alert: time() - pulse_collector_last_success_timestamp > 180
	// — without Pulse having to page itself (the built-in self-alert is a separate,
	// decision-gated item). Emitted only when a collector-health source is wired
	// (nil in tests / a pure-beacon deployment), mirroring /healthz.
	if s.collectorHealth != nil {
		h := s.collectorHealth.PollHealth()
		var lastSuccessUnix int64 // 0 = no successful poll since boot
		if !h.LastSuccess.IsZero() {
			lastSuccessUnix = h.LastSuccess.Unix()
		}
		// up mirrors the /healthz collector decision exactly: fresh unless the age
		// (from LastSuccess, or StartedAt before the first success) exceeds
		// StaleAfter. A zero StaleAfter or an unstarted loop cannot be judged stale,
		// so it reads up (matches /healthz leaving status "ok" in those cases).
		up := 1
		ref := h.LastSuccess
		if ref.IsZero() {
			ref = h.StartedAt
		}
		if h.StaleAfter > 0 && !ref.IsZero() && time.Since(ref) > h.StaleAfter {
			up = 0
		}
		fmt.Fprintf(w, "# HELP pulse_collector_last_success_timestamp Unix time of the most recent successful AMS poll (0 = none since boot)\n# TYPE pulse_collector_last_success_timestamp gauge\npulse_collector_last_success_timestamp %d\n", lastSuccessUnix)
		fmt.Fprintf(w, "# HELP pulse_collector_up 1 when the collector's last successful poll is within its staleness window, else 0\n# TYPE pulse_collector_up gauge\npulse_collector_up %d\n", up)
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

// wsAllowedOrigins returns the origin patterns for WebSocket accept options.
// VD-S2: replaces InsecureSkipVerify=true with explicit origin enforcement.
// Uses cfg.AllowedWSOrigins when set; otherwise derives a same-origin pattern
// from the Host header so the web UI on the same host always works.
func (s *Server) wsAllowedOrigins(r *http.Request) []string {
	if len(s.cfg.AllowedWSOrigins) > 0 {
		return s.cfg.AllowedWSOrigins
	}
	// Default: allow the same host (http:// and https://).
	host := r.Host
	if host == "" {
		return []string{}
	}
	return []string{
		"https://" + host,
		"http://" + host,
	}
}

type wsConn struct {
	conn *websocket.Conn
}

type wsMessage struct {
	Type    string `json:"type"`
	TS      int64  `json:"ts"`
	Payload any    `json:"payload,omitempty"`
}

func (s *Server) handleLiveWS(w http.ResponseWriter, r *http.Request) {
	// Auth is handled by downloadAuthMiddleware (header / pulse_session cookie /
	// ?token=), which validated the token and stashed it in ctxTokenKey. Reading
	// it here — instead of re-extracting from the header/query — is what lets an
	// OIDC cookie-session user open the socket (the old header/?token=-only path
	// rejected them); it also removes a redundant second LookupToken.
	if tok, _ := r.Context().Value(ctxTokenKey).(*meta.APIToken); tok == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing token")
		return
	}

	// VD-S2: do NOT use InsecureSkipVerify=true — that disables origin enforcement.
	// Use OriginPatterns to allow specific origins; empty slice = same-origin only.
	// For development / API clients that use query-param token auth, allow all for now
	// but remove the insecure flag so the library's default rejection applies.
	// Production deployments should set PULSE_ALLOWED_WS_ORIGINS.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: s.wsAllowedOrigins(r),
		// Negotiate the bearer-subprotocol marker when the client offers it, so a
		// browser passing the token via Sec-WebSocket-Protocol gets a valid handshake
		// (the token itself is never selected/echoed — S73/D-140 [7]).
		Subprotocols: []string{wsBearerSubprotocol},
	})
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

	// VD-02: Send initial LiveOverview (not raw LiveSnapshot) to match the
	// OpenAPI LiveOverview schema (total_publishers, protocol_mix, apps fields).
	if overview, err := s.qsvc.LiveOverview(r.Context(), "", "", ""); err == nil && overview != nil {
		_ = wsjson.Write(r.Context(), conn, wsMessage{Type: "snapshot", TS: time.Now().UnixMilli(), Payload: overview})
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
		case _, ok := <-ch:
			if !ok {
				return
			}
			// VD-02: broadcast LiveOverview shape (total_publishers, protocol_mix, apps)
			// not the raw LiveSnapshot. The OpenAPI spec for /live/ws requires LiveOverview.
			if overview, err := s.qsvc.LiveOverview(ctx, "", "", ""); err == nil && overview != nil {
				s.wsBroadcast(ctx, wsMessage{Type: "delta", TS: time.Now().UnixMilli(), Payload: overview})
			}
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
	if err := s.lic.CheckDataAPI(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
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
	if err := s.lic.CheckDataAPI(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	q := r.URL.Query()
	from, to := parseTimeRange(q.Get("from"), q.Get("to"))
	includeRegion := q.Get("region") == "true" || q.Get("region") == "1"

	rows, err := s.qsvc.GeoBreakdown(r.Context(), query.GeoParams{
		From:   from,
		To:     to,
		App:    q.Get("app"),
		Stream: q.Get("stream"),
		Tenant: q.Get("tenant"),
		Region: includeRegion,
	})
	if err != nil {
		s.logger.Warn("geo breakdown query failed", "error", err)
		rows = []query.GeoRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

func (s *Server) handleDeviceAnalytics(w http.ResponseWriter, r *http.Request) {
	if err := s.lic.CheckDataAPI(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	q := r.URL.Query()
	from, to := parseTimeRange(q.Get("from"), q.Get("to"))

	rows, err := s.qsvc.DeviceBreakdown(r.Context(), query.DeviceParams{
		From:   from,
		To:     to,
		App:    q.Get("app"),
		Stream: q.Get("stream"),
		Tenant: q.Get("tenant"),
	})
	if err != nil {
		s.logger.Warn("device breakdown query failed", "error", err)
		rows = []query.DeviceRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

// ─── QoE ──────────────────────────────────────────────────────────────────────

func (s *Server) handleQoeSummary(w http.ResponseWriter, r *http.Request) {
	if err := s.lic.CheckDataAPI(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	// VD-11: Query rollup_qoe_1h for real viewer-side QoE metrics.
	// startup_p50_ms is now populated from quantile state; bitrate timeline
	// uses the correct field name bitrate_kbps_p50 per the OpenAPI spec.
	q := r.URL.Query()
	from, to := parseTimeRange(q.Get("from"), q.Get("to"))
	interval := q.Get("interval")
	if interval == "" {
		interval = "hour"
	}

	result, err := s.qsvc.QoeSummary(r.Context(), query.QoeParams{
		From:     from,
		To:       to,
		App:      q.Get("app"),
		Stream:   q.Get("stream"),
		Tenant:   q.Get("tenant"),
		Country:  q.Get("country"),
		Device:   q.Get("device"),
		Interval: interval,
	})
	if err != nil {
		s.logger.Warn("qoe summary query failed", "error", err)
		result = &query.QoeSummaryResult{
			Totals:          query.QoeTotals{},
			BitrateTimeline: []query.BitrateBucket{},
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"totals":           result.Totals,
		"bitrate_timeline": result.BitrateTimeline,
	})
}

func (s *Server) handleIngestHealth(w http.ResponseWriter, r *http.Request) {
	// License gate: Ingest health (F4) requires Pro tier or higher.
	if err := s.lic.CheckDataAPI(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	// VD-20b + VD-21: return health_score (non-zero when ingest_stats received)
	// and per-stream timeseries + drop_events per OpenAPI IngestStream schema.
	//
	// BUG-004 fix: honour the declared OpenAPI query parameters.
	//
	//   from / to  — parsed by parseTimeParam (zero when absent → no time filter
	//                in IngestTimeseries; back-compat when caller passes nothing).
	//   app / stream / node — stream-selection filters; absent = no filtering.
	//   interval   — parsed by parseBucketInterval; 0 when absent (F4 60-second default preserved; see parseBucketInterval godoc).
	//
	// Data sources:
	//   - health_score, health, and current raw metrics: live aggregator snapshot
	//     (populated by BE-01 VD-20a bridge: aggregator.onIngestStats calls
	//      ingest.ComputeHealthScore so LiveStream.HealthScore is non-zero).
	//   - timeseries + drop_events: server_events table via query.Service.IngestTimeseries.
	//
	// OpenAPI IngestStream schema requires: [stream_id, app, health_score, timeseries].
	// The health_score field is 0–100 per spec (minimum 0, maximum 100).
	q := r.URL.Query()
	from := parseTimeParam(q.Get("from"))
	to := parseTimeParam(q.Get("to"))
	filterApp := q.Get("app")
	filterStream := q.Get("stream")
	filterNode := q.Get("node")
	filterTenant := q.Get("tenant")
	bucketSecs := parseBucketInterval(q.Get("interval"))

	ctx := r.Context()
	snap := s.live.CurrentSnapshot()
	var streams []map[string]any
	if snap != nil {
		for sid, st := range snap.Streams {
			if !st.Active {
				continue
			}
			// Stream-selection filters — absent param means include all.
			if filterApp != "" && st.App != filterApp {
				continue
			}
			if filterStream != "" && sid != filterStream {
				continue
			}
			if filterNode != "" && st.NodeID != filterNode {
				continue
			}

			// Fetch ingest timeseries for this stream.
			// Non-blocking: falls back to empty slices when ClickHouse is unavailable.
			ts, dropEvents := []any{}, []any{}
			if s.iqsvc != nil {
				tsResult, err := s.iqsvc.IngestTimeseries(ctx, query.IngestTimeseriesParams{
					StreamID:      sid,
					App:           st.App,
					NodeID:        st.NodeID,
					Tenant:        filterTenant,
					From:          from,
					To:            to,
					BucketSeconds: bucketSecs,
				})
				if err == nil && tsResult != nil {
					for _, b := range tsResult.Timeseries {
						ts = append(ts, b)
					}
					for _, de := range tsResult.DropEvents {
						dropEvents = append(dropEvents, de)
					}
				}
			}

			// health_score is 0.0–1.0 internally (ComputeHealthScore); the OpenAPI
			// schema specifies minimum 0, maximum 100 — scale accordingly.
			healthScoreScaled := st.HealthScore * 100.0

			streams = append(streams, map[string]any{
				"stream_id":           sid,
				"app":                 st.App,
				"node_id":             st.NodeID,
				"health_score":        healthScoreScaled,
				"health":              string(st.Health),
				"bitrate_kbps":        st.IngestBitrate,
				"fps":                 st.FPS,
				"packet_loss_pct":     st.PacketLossPct,
				"jitter_ms":           st.JitterMS,
				"keyframe_interval_s": st.KeyframeIntervalS,
				"timeseries":          ts,
				"drop_events":         dropEvents,
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
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	cursor := q.Get("cursor")
	rules, err := s.store.ListAlertRules(r.Context(), limit+1, cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	var nextCursor *string
	if len(rules) > limit {
		rules = rules[:limit]
		last := rules[len(rules)-1]
		c := fmt.Sprintf("%d:%s", last.CreatedAt, last.ID)
		nextCursor = &c
	}
	items := make([]any, 0, len(rules))
	for _, rule := range rules {
		items = append(items, alertRuleToAPI(rule))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "meta": map[string]any{"next_cursor": nextCursor}})
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
	// Validate threshold-rule spec (metric, operator, severity, window_s, threshold).
	// Skipped for anomaly rules — those carry anomaly-only metric names not in
	// KnownMetricNames, and their separate constraints are checked by ValidateAnomalyRule.
	if row.RuleType != "anomaly" {
		if valErr := alert.ValidateRuleSpec(row.Metric, row.Operator, int64(row.WindowS), row.Severity, row.Threshold); valErr != nil {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_RULE", valErr.Error())
			return
		}
	}
	// S11 WO-B: validate anomaly-specific constraints (metric support, window_s=3600).
	if valErr := alert.ValidateAnomalyRule(row); valErr != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ANOMALY_RULE", valErr.Error())
		return
	}
	created, err := s.store.CreateAlertRule(r.Context(), row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.audit(r, "alert_rule.create", "alert_rule", created.ID, map[string]any{"name": created.Name, "metric": created.Metric})
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
	// Validate threshold-rule spec — same guard as in handleCreateAlertRule.
	if row.RuleType != "anomaly" {
		if valErr := alert.ValidateRuleSpec(row.Metric, row.Operator, int64(row.WindowS), row.Severity, row.Threshold); valErr != nil {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_RULE", valErr.Error())
			return
		}
	}
	// S11 WO-B: validate anomaly-specific constraints.
	if valErr := alert.ValidateAnomalyRule(row); valErr != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ANOMALY_RULE", valErr.Error())
		return
	}
	row.ID = id
	row.CreatedAt = existing.CreatedAt
	if err := s.store.UpdateAlertRule(r.Context(), row); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.audit(r, "alert_rule.update", "alert_rule", id, map[string]any{"name": row.Name, "metric": row.Metric})
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
	s.audit(r, "alert_rule.delete", "alert_rule", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

// ─── Alert channels ───────────────────────────────────────────────────────────

func (s *Server) handleListAlertChannels(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	cursor := q.Get("cursor")
	chans, err := s.store.ListAlertChannels(r.Context(), limit+1, cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	var nextCursor *string
	if len(chans) > limit {
		chans = chans[:limit]
		last := chans[len(chans)-1]
		c := fmt.Sprintf("%d:%s", last.CreatedAt, last.ID)
		nextCursor = &c
	}
	items := make([]any, 0, len(chans))
	for _, ch := range chans {
		items = append(items, alertChannelToAPI(ch))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "meta": map[string]any{"next_cursor": nextCursor}})
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
	s.audit(r, "alert_channel.create", "alert_channel", created.ID, map[string]any{"type": created.Type, "name": created.Name})
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
	// Gate the resolved channel type, mirroring create: without this a Free tenant
	// could upgrade a channel to a paid type (e.g. email → slack) unlicensed.
	if err := s.lic.CheckChannelAllowed(row.Type); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	row.ID = id
	row.CreatedAt = existing.CreatedAt
	if err := s.store.UpdateAlertChannel(r.Context(), row); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.audit(r, "alert_channel.update", "alert_channel", id, map[string]any{"type": row.Type, "name": row.Name})
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
	s.audit(r, "alert_channel.delete", "alert_channel", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestAlertChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "channelId")
	row, err := s.store.GetAlertChannel(r.Context(), id)
	if err != nil || row == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "channel not found")
		return
	}
	// Firing a test delivery to a paid channel type is a paid action: gate it so a
	// Free (or downgraded) tenant cannot send live Slack/PagerDuty/webhook tests.
	if err := s.lic.CheckChannelAllowed(row.Type); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	ch, err := buildChannelFromRow(s.store, row)
	if err != nil {
		// Do NOT echo decrypt internals — just log and return generic message.
		s.logger.Warn("test fire: channel config invalid", "channel_id", id, "error", err)
		writeJSON(w, http.StatusOK, map[string]any{"accepted": false, "message": "channel configuration invalid"})
		return
	}
	if err := alert.TestFireChannel(r.Context(), ch, "test", s.cfg.BaseURL); err != nil {
		// SECURITY: Do NOT put err.Error() in the body — *url.Error includes the channel
		// URL which may embed telegram bot tokens / slack webhook URLs (secret leak).
		s.logger.Warn("test fire failed", "channel_id", id, "type", row.Type, "error", err)
		msg := fmt.Sprintf("delivery failed for %s channel; check configuration and connectivity", row.Type)
		writeJSON(w, http.StatusOK, map[string]any{"accepted": false, "message": msg})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true, "message": "test notification delivered"})
}

// buildChannelFromRow delegates to the shared alert.BuildChannelFromRow factory.
// The factory lives in the alert package so the evaluator and the API handler
// share a single implementation (no duplication, no import cycle).
func buildChannelFromRow(store *meta.Store, row *meta.AlertChannelRow) (channels.Channel, error) {
	return alert.BuildChannelFromRow(store, row)
}

// ─── Alert history ────────────────────────────────────────────────────────────

func (s *Server) handleAlertHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to := parseTimeRange(q.Get("from"), q.Get("to"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	// A11: cap at 500 to prevent unbounded result sets.
	if limit > 500 {
		limit = 500
	}
	cursor := q.Get("cursor")
	hist, err := s.store.ListAlertHistory(r.Context(), q.Get("rule_id"), q.Get("state"),
		from.UnixMilli(), to.UnixMilli(), limit+1, cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	var nextCursor *string
	if len(hist) > limit {
		hist = hist[:limit]
		last := hist[len(hist)-1]
		c := fmt.Sprintf("%d:%s", last.TS, last.ID)
		nextCursor = &c
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
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "meta": map[string]any{"next_cursor": nextCursor}})
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

// ─── Anomalies / Probes (wave-3 — implemented in wave3.go) ──────────────────
// handleAnomalies, handleListProbes, handleCreateProbe, handleUpdateProbe,
// handleDeleteProbe, handleProbeResults are defined in wave3.go.

// ─── Admin: Sources ───────────────────────────────────────────────────────────

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	cursor := q.Get("cursor")
	sources, err := s.store.ListAMSSources(r.Context(), limit+1, cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	var nextCursor *string
	if len(sources) > limit {
		sources = sources[:limit]
		last := sources[len(sources)-1]
		c := fmt.Sprintf("%d:%s", last.CreatedAt, last.ID)
		nextCursor = &c
	}
	items := make([]any, 0, len(sources))
	for _, src := range sources {
		items = append(items, amsSourceToAPI(src))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "meta": map[string]any{"next_cursor": nextCursor}})
}

func (s *Server) handleCreateSource(w http.ResponseWriter, r *http.Request) {
	// Serialize the count→gate→create sequence so concurrent creates cannot all
	// observe the same pre-create count and race past CheckNodeLimit (D-041 TOCTOU).
	s.sourceMu.Lock()
	defer s.sourceMu.Unlock()

	// License gate: count existing sources; fail if adding one more would exceed MaxNodes.
	existing, err := s.store.ListAMSSources(r.Context(), 0, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if err := s.lic.CheckNodeLimit(len(existing) + 1); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
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
	created, err := s.store.CreateAMSSource(r.Context(), row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.audit(r, "ams_source.create", "ams_source", created.ID, map[string]any{"name": created.Name})
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
	s.audit(r, "ams_source.update", "ams_source", id, map[string]any{"name": row.Name})
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
	s.audit(r, "ams_source.delete", "ams_source", id, nil)
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
	// VD-X3-A: response must include `reachable` (bool) per AmsSourceStatus schema.
	if !src.RestURL.Valid || src.RestURL.String == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"reachable":  false,
			"status":     "unknown",
			"error":      "no rest_url configured for this source",
			"latency_ms": nil,
		})
		return
	}

	// B4/A6: validate the stored URL scheme before making an outbound request.
	// This is a defence-in-depth check; amsSourceFromAPI already validates on
	// write, but a row could have been created before this guard was added.
	testBase := src.RestURL.String
	parsedBase, perr := url.Parse(testBase)
	if perr != nil || (parsedBase.Scheme != "http" && parsedBase.Scheme != "https") {
		writeJSON(w, http.StatusOK, map[string]any{
			"reachable":  false,
			"status":     "error",
			"error":      "rest_url must use http or https scheme",
			"latency_ms": nil,
		})
		return
	}
	// S101 (REVIEW-EXT-2026-07-24 item 4): IP-literal boundary check. The
	// authoritative, rebinding-safe enforcement is the DialControl hook on the
	// transport below; rejecting the obvious literal here just gives a named
	// error instead of a generic dial refusal.
	if ip := net.ParseIP(parsedBase.Hostname()); ip != nil && ssrfguard.IsDenied(ip) {
		writeJSON(w, http.StatusOK, map[string]any{
			"reachable":  false,
			"status":     "error",
			"error":      "rest_url host is a restricted address (link-local/metadata ranges are refused)",
			"latency_ms": nil,
		})
		return
	}

	testURL := strings.TrimRight(testBase, "/") + "/rest/v2/version"
	start := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"reachable":  false,
			"status":     "error",
			"error":      fmt.Sprintf("build request: %v", err),
			"latency_ms": nil,
		})
		return
	}
	// B6: decrypt the stored credential so the connectivity test authenticates
	// correctly — previously the password was always empty, so protected AMS
	// nodes would return 401 even when a valid credential was stored.
	if src.RestUser.Valid && src.RestUser.String != "" {
		password := ""
		if src.CredentialEnc.Valid && src.CredentialEnc.String != "" {
			dec, derr := s.store.Decrypt(src.CredentialEnc.String)
			if derr != nil {
				s.logger.Warn("source-test: failed to decrypt stored credential", "source_id", id, "error", derr)
			} else {
				password = dec
			}
		}
		req.SetBasicAuth(src.RestUser.String, password)
	}

	// B4/A6 + S101: dedicated client with the ssrfguard policy enforced at dial
	// time — private/RFC-1918/loopback ALLOWED (real AMS nodes live on internal
	// networks), link-local/IMDS/NAT64-embedded/unspecified DENIED. The Control
	// hook sees the *resolved* peer, so a hostname that resolves (or rebinds) to
	// 169.254.169.254 is refused even though only the literal is checked above.
	// Redirects are additionally not followed at all (defence in depth; the
	// guard would re-run on any redirect dial anyway).
	testTransport := http.DefaultTransport.(*http.Transport).Clone()
	testTransport.DialContext = (&net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   ssrfguard.DialControl,
	}).DialContext
	// No proxy: with an egress proxy the transport dials the proxy (which passes
	// DialControl) and the proxy fetches the real target, bypassing the guard.
	testTransport.Proxy = nil
	testClient := &http.Client{
		Timeout:   5 * time.Second,
		Transport: testTransport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := testClient.Do(req)
	latencyMS := time.Since(start).Milliseconds()
	if err != nil {
		// Network error means unreachable.
		writeJSON(w, http.StatusOK, map[string]any{
			"reachable":  false,
			"status":     "error",
			"error":      err.Error(),
			"latency_ms": latencyMS,
		})
		return
	}
	defer resp.Body.Close()

	// reachable = true when an HTTP response was received (any status code, including 4xx/5xx).
	reachable := true
	status := "ok"
	if resp.StatusCode >= 400 {
		status = "error"
	}
	// Per the AmsSourceStatus contract, `error` is null on success (reachable=true);
	// the coarse ok/error signal is carried by `status`.
	writeJSON(w, http.StatusOK, map[string]any{
		"reachable":  reachable,
		"status":     status,
		"error":      nil,
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
	// Record the activation, but never the key itself.
	s.audit(r, "license.activate", "license", "", nil)
	writeJSON(w, http.StatusOK, licenseToAPI(s.lic))
}

// ─── Admin: Tokens ────────────────────────────────────────────────────────────

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	cursor := q.Get("cursor")
	tokens, err := s.store.ListTokens(r.Context(), q.Get("kind"), limit+1, cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	var nextCursor *string
	if len(tokens) > limit {
		tokens = tokens[:limit]
		last := tokens[len(tokens)-1]
		c := fmt.Sprintf("%d:%s", last.CreatedAt, last.ID)
		nextCursor = &c
	}
	items := make([]any, 0, len(tokens))
	for _, t := range tokens {
		items = append(items, tokenToAPI(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "meta": map[string]any{"next_cursor": nextCursor}})
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
	// Positive allowlist (D-098): only the two kinds the auth layer honors are
	// storable. An unrecognized kind (e.g. "superadmin") authenticates nowhere yet
	// would persist as a valid-looking token — reject it, don't store a dead row.
	if kind != "api" && kind != "ingest" {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_PARAM", `kind must be one of: "api", "ingest"`)
		return
	}
	rawToken := "plt_" + newToken()
	tokenHash, hashAlg := s.store.HashToken(rawToken)
	var scopes []string
	if sv, ok := body["scopes"].([]any); ok {
		for _, v := range sv {
			if ss, ok := v.(string); ok {
				scopes = append(scopes, ss)
			}
		}
	}
	// S101 (REVIEW-EXT-2026-07-24 item 6): an api-kind token created without
	// scopes used to persist scopeless — and canWrite treats no-scopes as full
	// admin (legacy compat). Default new api tokens to least privilege instead;
	// admin now requires an explicit scopes:["admin"]. Legacy tokens already in
	// the store keep their historical semantics. Ingest tokens never consult
	// scopes, so they are left as-is.
	if kind == "api" && len(scopes) == 0 {
		scopes = []string{"read"}
	}
	// Pre-assign the id so the create can be audited BEFORE the re-fetch below —
	// otherwise a nil re-fetch would leave the committed token unrecorded (S40 class).
	tok := meta.APIToken{ID: uuid.NewString(), Kind: kind, Name: name, TokenHash: tokenHash, HashAlg: hashAlg, Scopes: scopes, CreatedAt: time.Now().UnixMilli()}
	if err := s.store.CreateToken(r.Context(), tok); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	// Audit the committed create here — before the re-fetch — so a re-fetch that
	// nils cannot leave the mutation unrecorded. Never log the raw token or hash.
	s.audit(r, "token.create", "token", tok.ID, map[string]any{"name": tok.Name, "kind": tok.Kind, "scopes": tok.Scopes})
	created, err := s.store.LookupToken(r.Context(), rawToken)
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
	id := chi.URLParam(r, "tokenId")
	err := s.store.DeleteToken(r.Context(), id)
	if err != nil && !errors.Is(err, meta.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	// Idempotent per the OpenAPI contract (204 even for a missing id), but only
	// audit a revoke that actually removed a token — never a phantom token.revoke
	// for a bogus id (that would corrupt the compliance trail; S38 missing-id class).
	if err == nil {
		s.audit(r, "token.revoke", "token", id, nil)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Admin: Users ─────────────────────────────────────────────────────────────

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	cursor := q.Get("cursor")
	users, err := s.store.ListUsers(r.Context(), limit+1, cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	var nextCursor *string
	if len(users) > limit {
		users = users[:limit]
		last := users[len(users)-1]
		c := fmt.Sprintf("%d:%s", last.CreatedAt, last.ID)
		nextCursor = &c
	}
	items := make([]any, 0, len(users))
	for _, u := range users {
		items = append(items, userToAPI(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "meta": map[string]any{"next_cursor": nextCursor}})
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
	if !validUserRole(role) {
		writeError(w, http.StatusBadRequest, "INVALID_PARAM", "role must be 'admin' or 'viewer'")
		return
	}
	// bcrypt hashes at most 72 bytes; reject an over-long password with a clear
	// error rather than letting hashPassword fail closed to an unusable empty hash.
	if len(password) > 72 {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_PARAM", "password must be at most 72 bytes")
		return
	}
	now := time.Now().UnixMilli()
	// Pre-assign the id so the create can be audited BEFORE the re-fetch below —
	// otherwise a nil re-fetch would leave the committed user unrecorded (S40 class).
	u := meta.User{ID: uuid.NewString(), Username: username, PwHash: hashPassword(password), Role: role, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateUser(r.Context(), u); err != nil {
		// A duplicate username is a client error, not a 500 — the unique
		// constraint on users.username is the only expected failure here.
		if isUniqueConstraintError(err) {
			writeError(w, http.StatusConflict, "CONFLICT", "a user with that username already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	// Audit the committed create here — before the re-fetch — so a re-fetch that
	// nils cannot leave the mutation unrecorded.
	s.audit(r, "user.create", "user", u.ID, map[string]any{"username": u.Username, "role": u.Role})
	created, _ := s.store.GetUserByUsername(r.Context(), username)
	if created == nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "user created but not found")
		return
	}
	writeJSON(w, http.StatusCreated, userToAPI(*created))
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "userId")
	// Fail with 404 rather than silently UPDATE-ing zero rows and returning 200.
	existing, err := s.store.GetUserByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	// Full replace, matching the UserWrite contract (required: [username, role]).
	// Requiring both fields is what prevents the old blanking bug: a role-only
	// body is now a 400, not a silent SET username=''.
	username, _ := body["username"].(string)
	role, _ := body["role"].(string)
	if username == "" || role == "" {
		writeError(w, http.StatusBadRequest, "INVALID_PARAM", "username and role required")
		return
	}
	if !validUserRole(role) {
		writeError(w, http.StatusBadRequest, "INVALID_PARAM", "role must be 'admin' or 'viewer'")
		return
	}
	if err := s.store.UpdateUser(r.Context(), id, username, role); err != nil {
		if isUniqueConstraintError(err) {
			writeError(w, http.StatusConflict, "CONFLICT", "a user with that username already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	// Audit the committed change here — before the re-fetch — so a concurrent delete
	// that nils the re-read (below) cannot leave a durable mutation unrecorded.
	s.audit(r, "user.update", "user", id, map[string]any{"username": username, "role": role})
	// Return the actual stored row, not an echo of the request with a fabricated
	// created_at:0. A nil here means the row was deleted concurrently (between the
	// existence check and the update) — report that honestly as 404, not 500.
	updated, err := s.store.GetUserByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}
	writeJSON(w, http.StatusOK, userToAPI(*updated))
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "userId")
	err := s.store.DeleteUser(r.Context(), id)
	if err != nil && !errors.Is(err, meta.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	// Idempotent per the OpenAPI contract (204 even for a missing id), but only
	// audit a delete that actually removed a user — never a phantom user.delete
	// for a bogus id (that would corrupt the compliance trail; S38 missing-id class).
	if err == nil {
		s.audit(r, "user.delete", "user", id, nil)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Beacon ingest ────────────────────────────────────────────────────────────

// beaconMaxBodyBytes is the body size cap for the main-port beacon handler.
// Aligned to 64 KB per the OpenAPI spec (VD-10: was incorrectly 256 KB).
const beaconMaxBodyBytes = 64 * 1024

func (s *Server) handleIngestBeacon(w http.ResponseWriter, r *http.Request) {
	// VD-15: license gate — beacon ingest requires Pro tier or higher.
	if err := s.lic.CheckBeaconIngest(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}

	ingestToken := r.Header.Get("X-Pulse-Ingest-Token")
	if ingestToken == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing X-Pulse-Ingest-Token")
		return
	}
	tok, err := s.store.LookupToken(r.Context(), ingestToken)
	if err != nil || tok == nil || tok.Kind != "ingest" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid ingest token")
		return
	}

	// A2: per-token rate limit (100 rps / burst 200), matching the dedicated
	// beacon server (serve.go:326 RateLimitPerTokenRPS:100, burst 200).
	// Token 401 is checked first (above); 429 check comes second — same ordering
	// as the dedicated beacon handler.
	if !s.beaconLimiter.Allow(tok.ID) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "rate limit exceeded; retry after 1s")
		return
	}

	// Body size cap: 64 KB per spec (VD-10: aligned from 256 KB).
	// Use io.ReadAll on a MaxBytesReader so we can detect size exceeded vs
	// JSON parse errors (the dedicated beacon handler uses the same pattern).
	r.Body = http.MaxBytesReader(w, r.Body, beaconMaxBodyBytes)
	rawBody, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		if strings.Contains(readErr.Error(), "http: request body too large") ||
			int64(len(rawBody)) >= beaconMaxBodyBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE",
				fmt.Sprintf("body exceeds %d KB limit", beaconMaxBodyBytes/1024))
			return
		}
		writeError(w, http.StatusBadRequest, "READ_ERROR", "failed to read request body")
		return
	}
	if int64(len(rawBody)) >= beaconMaxBodyBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE",
			fmt.Sprintf("body exceeds %d KB limit", beaconMaxBodyBytes/1024))
		return
	}

	// S101 (REVIEW-EXT-2026-07-24, main-port validation skip): enforce the SAME
	// schema rules as the dedicated beacon port before accepting anything. The
	// beacon package owns the rules; this route previously accepted any
	// decodable batch, so the two ports disagreed on what a valid event is.
	if jsonErr, schemaErrs := beacon.ValidateRawBatch(rawBody); jsonErr != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	} else if len(schemaErrs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"code":    "SCHEMA_ERROR",
			"message": "beacon event batch failed schema validation",
			"errors":  schemaErrs,
		})
		return
	}

	// Parse the beacon batch JSON directly into domain types so we can write
	// to the event sink (VD-10: was discarding all events after decode).
	var batch struct {
		Version   int               `json:"version"`
		SessionID string            `json:"session_id"`
		StreamID  string            `json:"stream_id"`
		App       string            `json:"app"`
		Meta      map[string]string `json:"meta"`
		Player    *struct {
			Kind       string `json:"kind"`
			SDKVersion string `json:"sdk_version"`
		} `json:"player"`
		Events []struct {
			Type string         `json:"type"`
			TS   int64          `json:"ts"`
			Data map[string]any `json:"data"`
		} `json:"events"`
	}
	if err := json.Unmarshal(rawBody, &batch); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}

	// Build domain.BeaconEvent and write to event sink if wired.
	if s.eventSink != nil && len(batch.Events) > 0 {
		items := make([]domain.BeaconItem, len(batch.Events))
		for i, ev := range batch.Events {
			items[i] = domain.BeaconItem{Type: ev.Type, TS: ev.TS, Data: ev.Data}
		}
		evt := domain.BeaconEvent{
			Version:   batch.Version,
			SessionID: batch.SessionID,
			StreamID:  batch.StreamID,
			App:       batch.App,
			Events:    items,
		}
		if batch.Player != nil {
			evt.PlayerKind = batch.Player.Kind
		}
		if batch.Meta != nil {
			if tenant, ok := batch.Meta["tenant"]; ok {
				evt.Tenant = tenant
			}
		}
		// Non-blocking async write — matches the dedicated handler's pattern.
		go s.eventSink.WriteBeaconEvent(evt)
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted": len(batch.Events),
		"rejected": 0,
		"errors":   []any{},
	})
}

// ─── Bootstrap ────────────────────────────────────────────────────────────────

func (s *Server) bootstrapIfFirstRun(ctx context.Context) error {
	n, err := s.store.CountTokens(ctx)
	if err != nil || n > 0 {
		return err
	}
	rawToken := "plt_" + newToken()
	tokenHash, hashAlg := s.store.HashToken(rawToken)
	tok := meta.APIToken{Kind: "api", Name: "admin (bootstrap)", TokenHash: tokenHash, HashAlg: hashAlg, Scopes: []string{"admin"}, CreatedAt: time.Now().UnixMilli()}
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
	// S11 WO-B: anomaly rule fields.
	m["rule_type"] = r.RuleType
	if r.Sigma > 0 {
		m["sigma"] = r.Sigma
	}
	if r.MinSamples > 0 {
		m["min_samples"] = r.MinSamples
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
	// S11 WO-B: anomaly rule fields.
	if rt, ok := body["rule_type"].(string); ok && rt != "" {
		row.RuleType = rt
	}
	if s, ok := body["sigma"].(float64); ok {
		row.Sigma = s
	}
	if ms, ok := body["min_samples"].(float64); ok {
		row.MinSamples = int(ms)
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
		// Email/SMTP auth pair — encrypted at rest, not stored in config_public.
		// factory.BuildChannelFromRow merges public+decrypted config on read, so
		// existing channels keep working and new ones no longer leak credentials.
		"password": true, "username": true,
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
		"id":                 src.ID,
		"name":               src.Name,
		"type":               src.SourceType,
		"credential_set":     src.CredentialEnc.Valid && src.CredentialEnc.String != "",
		"webhook_secret_set": src.WebhookSecretEnc.Valid && src.WebhookSecretEnc.String != "",
		"created_at":         src.CreatedAt,
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
	if v, ok := body["rest_url"].(string); ok && v != "" {
		// B4/A6: reject schemes other than http/https to prevent SSRF via
		// file://, ftp://, gopher://, etc.
		parsed, err := url.Parse(v)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return meta.AMSSourceRow{}, fmt.Errorf("rest_url must use http or https scheme")
		}
		row.RestURL = sql.NullString{String: v, Valid: true}
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
	// B7: per-source webhook HMAC secret (write-only; stored encrypted).
	if v, ok := body["webhook_secret"].(string); ok && v != "" {
		enc, err := store.Encrypt(v)
		if err != nil {
			return row, fmt.Errorf("encrypt webhook_secret: %w", err)
		}
		row.WebhookSecretEnc = sql.NullString{String: enc, Valid: true}
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

// validUserRole reports whether role is one of the two roles the system defines
// (meta.User.Role: "admin" | "viewer"; the OIDC group mapping emits the same set).
func validUserRole(role string) bool {
	return role == "admin" || role == "viewer"
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
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Status is already committed; at least make the silent failure visible
		// (e.g. a NaN/Inf float that encoding/json refuses, truncating the body).
		slog.Error("api: writeJSON encode failed; response body truncated", "error", err, "status", status)
	}
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
		// bcrypt errors only when pw exceeds 72 bytes (handleCreateUser rejects
		// that with 422 first) or on a broken crypto setup. NEVER fall back to a
		// fast hash (SHA-256) for a password — that silently downgrades it to a
		// crackable digest (D-109 / CWE-916). Fail closed with an unusable empty
		// hash instead; checkPassword still verifies legacy sha256: rows on login.
		return ""
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

// parseTimeParam parses a single from/to query-parameter value.
// Returns zero time.Time when s is empty or cannot be parsed — intentionally
// no default is applied, so callers that need "no filter" semantics get a zero
// value that IngestTimeseries treats as "unbounded".
// Supports epoch-milliseconds (integer string) and RFC 3339.
func parseTimeParam(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.UnixMilli(ms)
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}

// parseBucketInterval maps the OpenAPI `interval` query-parameter to
// IngestTimeseriesParams.BucketSeconds.
//
//	"hour"  → 3600
//	"day"   → 86400
//	""      → 0  (see F4 deviation note below)
//	other   → 0  (lenient; consistent with parseTimeParam invalid-input handling)
//
// F4 deviation — deliberate default override:
// The OpenAPI spec declares default: "day" (86400 s buckets). Pulse deviates
// intentionally: when interval is absent the function returns 0, which makes
// IngestTimeseries keep its internal 60-second bucket default.
// Rationale: PRD F4 requires ingest degradation to be visible within 15 seconds
// of occurrence. A 24-hour bucket collapses an entire day into a single data
// point, hiding sub-minute degradations entirely. Callers that want daily
// granularity must pass interval=day explicitly; absence means "give me the
// finest grain available." This matches the intent of the F4 criterion and is
// consistent with how parseTimeParam returns zero (= no filter) rather than a
// default window when a time param is absent.
func parseBucketInterval(s string) int {
	switch s {
	case "hour":
		return 3600
	case "day":
		return 86400
	default:
		return 0
	}
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
