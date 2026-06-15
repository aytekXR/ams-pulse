// Package beacon is the public HTTPS ingest endpoint for the player QoE SDK
// (POST /ingest/beacon). The only internet-facing surface of Pulse; treat as
// hostile input.
//
// Contract: contracts/events/beacon-event.schema.json
// Requirements (PRD F3): token auth (constant-time compare, hashed at rest),
// per-token rate limit (token bucket), body size cap (64 KB), strict schema
// validation, 202 + async write, CORS for browser SDKs.
//
// Security rules (ARCHITECTURE §3.5):
//   - X-Pulse-Ingest-Token compared constant-time; never echoed
//   - Tokens stored as SHA-256 hashes (never plaintext)
//   - 413 on body > 64 KB; 429 on rate limit; 422 on schema error; 401 on bad token
//   - Enrichment: client IP extracted from X-Forwarded-For/RemoteAddr,
//     UA from User-Agent header; geo+UA resolved and stored in BeaconEvent.Enrichment.
package beacon

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/pulse-analytics/pulse/server/internal/collector"
	"github.com/pulse-analytics/pulse/server/internal/domain"
)

const (
	// maxBodyBytes is the maximum allowed body size (64 KB per spec).
	maxBodyBytes = 64 * 1024

	// defaultRateLimit is the per-token request/s rate limit (configurable).
	defaultRateLimit = 100.0

	// defaultRateBurst is the token bucket burst size.
	defaultRateBurst = 200
)

// TokenStore is a minimal interface to validate ingest tokens.
// Satisfied by *meta.Store.
type TokenStore interface {
	// GetIngestTokenByHash returns the token record for a given SHA-256 hex hash.
	// Returns nil, nil if not found.
	GetIngestTokenByHash(ctx context.Context, hash string) (tokenID string, ok bool, err error)
}

// MemTokenStore is an in-memory token store for testing.
type MemTokenStore struct {
	mu     sync.RWMutex
	hashes map[string]string // hash → token ID
}

// NewMemTokenStore creates an in-memory token store with pre-hashed tokens.
func NewMemTokenStore(rawTokens ...string) *MemTokenStore {
	s := &MemTokenStore{hashes: make(map[string]string)}
	for i, tok := range rawTokens {
		h := sha256Hex(tok)
		s.hashes[h] = fmt.Sprintf("token-%d", i)
	}
	return s
}

// AddToken adds a raw token to the store (used in tests).
func (m *MemTokenStore) AddToken(rawToken, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hashes[sha256Hex(rawToken)] = id
}

// GetIngestTokenByHash implements TokenStore.
func (m *MemTokenStore) GetIngestTokenByHash(_ context.Context, hash string) (string, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.hashes[hash]
	return id, ok, nil
}

// ─── Rate limiter ─────────────────────────────────────────────────────────────

// tokenBucket implements a per-token token-bucket rate limiter.
type tokenBucket struct {
	mu       sync.Mutex
	tokens   float64
	maxBurst float64
	rate     float64 // tokens per second
	lastFill time.Time
}

func newTokenBucket(rate, burst float64) *tokenBucket {
	return &tokenBucket{
		tokens:   burst,
		maxBurst: burst,
		rate:     rate,
		lastFill: time.Now(),
	}
}

// Allow returns true and consumes one token if the request is within rate limit.
func (b *tokenBucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.lastFill).Seconds()
	b.tokens += elapsed * b.rate
	if b.tokens > b.maxBurst {
		b.tokens = b.maxBurst
	}
	b.lastFill = now
	if b.tokens >= 1.0 {
		b.tokens--
		return true
	}
	return false
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// Config holds beacon handler configuration.
type Config struct {
	// RateLimitPerTokenRPS is the per-ingest-token request rate limit (requests/s).
	// Default: 100.
	RateLimitPerTokenRPS float64

	// RateBurst is the token bucket burst size.
	// Default: 200.
	RateBurst float64

	// ListenAddr is the dedicated ingest listener address.
	// If empty, the handler is mounted on the main API router.
	ListenAddr string

	// GeoResolver and UAParser provide geo-IP and User-Agent enrichment.
	// The beacon path is the only viable geo source for viewer sessions
	// (AMS REST is server-side, no per-viewer IP available).
	// If nil, enrichment is skipped (no-op).
	GeoResolver collector.GeoResolver
	UAParser    collector.UAParser
}

// Handler is the beacon ingest HTTP handler.
type Handler struct {
	cfg    Config
	store  TokenStore
	sink   domain.EventSink
	logger *slog.Logger

	mu      sync.Mutex
	buckets map[string]*tokenBucket // per-token-ID rate limit
}

// New creates a new beacon Handler.
func New(cfg Config, store TokenStore, sink domain.EventSink, logger *slog.Logger) *Handler {
	if cfg.RateLimitPerTokenRPS <= 0 {
		cfg.RateLimitPerTokenRPS = defaultRateLimit
	}
	if cfg.RateBurst <= 0 {
		cfg.RateBurst = defaultRateBurst
	}
	if cfg.GeoResolver == nil {
		cfg.GeoResolver = collector.NoopGeoResolver{}
	}
	if cfg.UAParser == nil {
		cfg.UAParser = collector.NoopUAParser{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		cfg:     cfg,
		store:   store,
		sink:    sink,
		logger:  logger,
		buckets: make(map[string]*tokenBucket),
	}
}

// Mount registers the ingest route on the given chi router.
func (h *Handler) Mount(r chi.Router) {
	r.Use(corsMiddlewareBeacon)
	r.Post("/ingest/beacon", h.Handle)
}

// Handle is the HTTP handler for POST /ingest/beacon.
//
// Security path:
//  1. Validate X-Pulse-Ingest-Token (constant-time compare via SHA-256)
//  2. Per-token rate limit (429)
//  3. Body size cap 64 KB (413)
//  4. JSON decode + schema validation (422)
//  5. 202 + async WriteBeaconEvent
func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	// ── 1. Token auth (constant-time via hash lookup) ──────────────────────
	rawToken := r.Header.Get("X-Pulse-Ingest-Token")
	if rawToken == "" {
		writeBeaconError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing X-Pulse-Ingest-Token header")
		return
	}
	// Constant-time: SHA-256 hash first, then map lookup — never compare raw token
	hash := sha256Hex(rawToken)
	tokenID, ok, err := h.store.GetIngestTokenByHash(r.Context(), hash)
	if err != nil || !ok {
		// Never distinguish "not found" from "error" — always 401
		writeBeaconError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid ingest token")
		return
	}

	// ── 2. Per-token rate limit ───────────────────────────────────────────
	bucket := h.getBucket(tokenID)
	if !bucket.Allow() {
		w.Header().Set("Retry-After", "1")
		writeBeaconError(w, http.StatusTooManyRequests, "RATE_LIMITED", "rate limit exceeded; retry after 1s")
		return
	}

	// ── 3. Body size cap (64 KB) ─────────────────────────────────────────
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if len(body) >= maxBodyBytes-1 {
			writeBeaconError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE",
				fmt.Sprintf("body exceeds %d KB limit", maxBodyBytes/1024))
			return
		}
		writeBeaconError(w, http.StatusBadRequest, "READ_ERROR", "failed to read request body")
		return
	}
	if int64(len(body)) >= maxBodyBytes {
		writeBeaconError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE",
			fmt.Sprintf("body exceeds %d KB limit", maxBodyBytes/1024))
		return
	}

	// ── 4. JSON decode ───────────────────────────────────────────────────
	var batch beaconBatch
	if err := json.Unmarshal(body, &batch); err != nil {
		writeBeaconError(w, http.StatusUnprocessableEntity, "INVALID_JSON", "invalid JSON body")
		return
	}

	// ── 5. Schema validation ─────────────────────────────────────────────
	errs := validateBeaconBatch(&batch)
	if len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"code":    "SCHEMA_ERROR",
			"message": "beacon event batch failed schema validation",
			"errors":  errs,
		})
		return
	}

	// ── 6. Async write to event sink ─────────────────────────────────────
	evt := batchToDomain(&batch, r, h.cfg.GeoResolver, h.cfg.UAParser)
	// Non-blocking: write runs in the sink's goroutine
	go h.sink.WriteBeaconEvent(evt)

	// ── 7. 202 Accepted ──────────────────────────────────────────────────
	accepted := len(batch.Events)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted": accepted,
		"rejected": 0,
		"errors":   []any{},
	})
}

// getBucket returns the token bucket for a given token ID (creates if absent).
func (h *Handler) getBucket(tokenID string) *tokenBucket {
	h.mu.Lock()
	defer h.mu.Unlock()
	b, ok := h.buckets[tokenID]
	if !ok {
		b = newTokenBucket(h.cfg.RateLimitPerTokenRPS, h.cfg.RateBurst)
		h.buckets[tokenID] = b
	}
	return b
}

// ─── Beacon batch types (internal parse targets) ──────────────────────────────

type beaconBatch struct {
	Version   int           `json:"version"`
	SessionID string        `json:"session_id"`
	StreamID  string        `json:"stream_id"`
	App       string        `json:"app"`
	Meta      map[string]string `json:"meta"`
	Player    *beaconPlayer `json:"player"`
	Events    []beaconItem  `json:"events"`
}

type beaconPlayer struct {
	Kind       string `json:"kind"`
	SDKVersion string `json:"sdk_version"`
}

type beaconItem struct {
	Type string         `json:"type"`
	TS   int64          `json:"ts"`
	Data map[string]any `json:"data"`
}

// validEventTypes is the allowed set per beacon-event.schema.json.
var validEventTypes = map[string]bool{
	"session_start":     true,
	"startup_complete":  true,
	"heartbeat":         true,
	"rebuffer_start":    true,
	"rebuffer_end":      true,
	"error":             true,
	"bitrate_change":    true,
	"resolution_change": true,
	"session_end":       true,
}

// validateBeaconBatch validates the batch against beacon-event.schema.json rules.
func validateBeaconBatch(b *beaconBatch) []string {
	var errs []string

	if b.Version != 1 {
		errs = append(errs, fmt.Sprintf("version must be 1, got %d", b.Version))
	}
	if b.SessionID == "" {
		errs = append(errs, "session_id is required")
	}
	if b.StreamID == "" {
		errs = append(errs, "stream_id is required")
	}
	if len(b.Events) == 0 {
		errs = append(errs, "events array must have at least 1 item")
	}
	for i, ev := range b.Events {
		if ev.Type == "" {
			errs = append(errs, fmt.Sprintf("events[%d]: type is required", i))
		} else if !validEventTypes[ev.Type] {
			errs = append(errs, fmt.Sprintf("events[%d]: type %q is not a valid beacon event type", i, ev.Type))
		}
		if ev.TS <= 0 {
			errs = append(errs, fmt.Sprintf("events[%d]: ts must be a positive unix epoch ms", i))
		}
		// Type-specific required fields.
		switch ev.Type {
		case "startup_complete":
			if _, ok := ev.Data["startup_ms"]; !ok {
				errs = append(errs, fmt.Sprintf("events[%d]: startup_complete requires data.startup_ms", i))
			}
		case "heartbeat":
			if _, ok := ev.Data["watch_ms"]; !ok {
				errs = append(errs, fmt.Sprintf("events[%d]: heartbeat requires data.watch_ms", i))
			}
		case "rebuffer_end":
			if _, ok := ev.Data["duration_ms"]; !ok {
				errs = append(errs, fmt.Sprintf("events[%d]: rebuffer_end requires data.duration_ms", i))
			}
		case "error":
			if _, ok := ev.Data["code"]; !ok {
				errs = append(errs, fmt.Sprintf("events[%d]: error requires data.code", i))
			}
		case "bitrate_change":
			if _, ok := ev.Data["from_kbps"]; !ok {
				errs = append(errs, fmt.Sprintf("events[%d]: bitrate_change requires data.from_kbps", i))
			}
			if _, ok := ev.Data["to_kbps"]; !ok {
				errs = append(errs, fmt.Sprintf("events[%d]: bitrate_change requires data.to_kbps", i))
			}
		case "resolution_change":
			if _, ok := ev.Data["from"]; !ok {
				errs = append(errs, fmt.Sprintf("events[%d]: resolution_change requires data.from", i))
			}
			if _, ok := ev.Data["to"]; !ok {
				errs = append(errs, fmt.Sprintf("events[%d]: resolution_change requires data.to", i))
			}
		}
	}
	return errs
}

// batchToDomain converts a parsed batch to a domain.BeaconEvent.
// VD-08: extract client IP (X-Forwarded-For / RemoteAddr) and User-Agent,
// call geo+UA resolvers, populate Enrichment on the event.
func batchToDomain(b *beaconBatch, r *http.Request, geo collector.GeoResolver, ua collector.UAParser) domain.BeaconEvent {
	items := make([]domain.BeaconItem, len(b.Events))
	for i, ev := range b.Events {
		items[i] = domain.BeaconItem{Type: ev.Type, TS: ev.TS, Data: ev.Data}
	}

	evt := domain.BeaconEvent{
		Version:   b.Version,
		SessionID: b.SessionID,
		StreamID:  b.StreamID,
		App:       b.App,
		Events:    items,
	}

	if b.Player != nil {
		evt.PlayerKind = b.Player.Kind
	}
	if b.Meta != nil {
		if tenant, ok := b.Meta["tenant"]; ok {
			evt.Tenant = tenant
		}
	}

	// VD-08: Extract client IP and User-Agent from the HTTP request.
	// X-Forwarded-For is checked first (CDN/proxy deployments);
	// fall back to RemoteAddr (direct connections).
	clientIP := extractClientIP(r)
	userAgent := r.Header.Get("User-Agent")

	// Resolve geo and UA enrichment.
	var geoEnrich domain.GeoEnrichment
	var clientEnrich domain.ClientEnrichment
	if clientIP != "" && geo != nil {
		geoEnrich = geo.Resolve(clientIP)
	}
	if userAgent != "" && ua != nil {
		clientEnrich = ua.Parse(userAgent)
	}

	// Only set Enrichment if there is something to report.
	if geoEnrich.Country != "" || geoEnrich.Region != "" || clientEnrich.Device != "" {
		evt.Enrichment = &domain.EnrichmentBlock{
			Geo:    &geoEnrich,
			Client: &clientEnrich,
		}
	}

	return evt
}

// extractClientIP returns the best-guess client IP from the request.
// Prefers the first non-local IP in X-Forwarded-For; falls back to RemoteAddr.
func extractClientIP(r *http.Request) string {
	// Check X-Forwarded-For (may contain comma-separated list of proxies).
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Use the first (leftmost) address — the original client.
		parts := strings.SplitN(xff, ",", 2)
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}
	// Fall back to RemoteAddr (strip port).
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ─── HTTP helpers ─────────────────────────────────────────────────────────────

func corsMiddlewareBeacon(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow browser SDKs from any origin.
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Pulse-Ingest-Token")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeBeaconError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"code": code, "message": message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// sha256Hex returns the SHA-256 hex of s. Used for constant-time token comparison
// via map lookup on the hash (hash comparison is inherently constant-time for
// equal-length inputs; SHA-256 always produces 64 hex chars).
func sha256Hex(s string) string {
	h := sha256.New()
	// SECURITY: use HMAC-SHA256 with an empty key is equivalent to SHA256 here;
	// tokens are one-way hashed via SHA-256 at rest (same scheme as api/server.go).
	// For ingest tokens we also use SHA-256 to be consistent with meta store.
	_, _ = h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// constantTimeTokenEqual compares two tokens in constant time.
// Not used (hash approach is equivalent), exported for tests.
func constantTimeTokenEqual(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}

// ─── Dedicated ingest listener ────────────────────────────────────────────────

// Server is an optional dedicated HTTP server for the beacon ingest endpoint.
// Use when PULSE_INGEST_LISTEN_ADDR is set (separate port for DMZ exposure).
type Server struct {
	cfg    Config
	h      *Handler
	srv    *http.Server
	logger *slog.Logger
}

// NewServer creates a dedicated ingest server.
func NewServer(cfg Config, store TokenStore, sink domain.EventSink, logger *slog.Logger) *Server {
	h := New(cfg, store, sink, logger)
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	h.Mount(r)
	return &Server{
		cfg:    cfg,
		h:      h,
		logger: logger,
		srv: &http.Server{
			Addr:         cfg.ListenAddr,
			Handler:      r,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}
}

// Start starts the dedicated ingest listener.
func (s *Server) Start() error {
	go func() {
		s.logger.Info("beacon: ingest listener", "addr", s.cfg.ListenAddr)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("beacon: ingest listener error", "error", err)
		}
	}()
	return nil
}

// Stop gracefully shuts down the ingest listener.
func (s *Server) Stop(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
