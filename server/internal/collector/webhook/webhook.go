// Package webhook receives AMS lifecycle webhooks (publish start/stop,
// recording ready) for instant stream state changes — lower latency than the
// REST poll for F1's 10-second publish-visibility criterion and F5's
// 30-second detection-to-notification criterion.
package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// Config holds webhook handler configuration.
type Config struct {
	// NodeID is stamped on all emitted events.
	NodeID string

	// SharedSecret is the HMAC-SHA256 secret used to validate AMS webhook
	// requests on the legacy /webhook/ams route. If empty, all requests on
	// that route are rejected (fail-closed).
	SharedSecret string

	// SourceSecrets maps source name → decrypted per-source HMAC secret (B7).
	// Used exclusively by /webhook/ams/{name}: if a name is present, its secret
	// is validated with NO SharedSecret fallback (cross-source isolation). If the
	// name is absent from the map, SharedSecret is used as fallback.
	// Populated at startup from the meta store; secret rotation requires restart.
	SourceSecrets map[string]string

	// ListenAddr is the address for the webhook HTTP server (e.g. ":8091").
	// If empty, the handler is embedded rather than standalone.
	ListenAddr string

	// RequireTimestamp, when true, enables webhook replay protection (audit
	// finding [8], D-123): every request must carry an X-Ams-Timestamp header
	// (Unix seconds) within ±TimestampSkew of now, and the HMAC is computed over
	// "<timestamp>.<body>" instead of the bare body — so the timestamp is bound
	// into the signature and cannot be stripped or forged, and a captured request
	// cannot be replayed once it falls outside the window.
	//
	// Default false preserves the original body-only contract. AMS lifecycle
	// webhooks are UNSIGNED (docs/AMS-INTEGRATION.md §4.5); the signed path is for
	// a signing proxy / custom middleware. This flag is GLOBAL — it applies to the
	// legacy /webhook/ams route AND every per-source /webhook/ams/{name} route
	// (B7). Enable it ONLY once ALL webhook senders — including every per-source
	// sender in SourceSecrets — send + sign the timestamp; a single sender that
	// does not will 401 on every request (fail-closed, same as an empty secret).
	// A per-source override is intentionally not provided yet (see D-123).
	RequireTimestamp bool

	// TimestampSkew is the ± acceptance window for X-Ams-Timestamp when
	// RequireTimestamp is true. Zero is replaced by the New() default (5 minutes).
	TimestampSkew time.Duration
}

// Handler implements collector.Source as an HTTP server receiving AMS webhooks.
type Handler struct {
	cfg    Config
	sink   domain.EventSink
	logger *slog.Logger
	server *http.Server
	now    func() time.Time // injectable clock (tests); defaults to time.Now
}

// New creates a webhook Handler.
func New(cfg Config, sink domain.EventSink, logger *slog.Logger) *Handler {
	if cfg.NodeID == "" {
		cfg.NodeID = "standalone"
	}
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.TimestampSkew <= 0 {
		cfg.TimestampSkew = 5 * time.Minute
	}
	h := &Handler{cfg: cfg, sink: sink, logger: logger, now: time.Now}
	mux := http.NewServeMux()
	// Legacy route: validates against SharedSecret ONLY; per-source secrets
	// never apply here, preserving backward compatibility.
	mux.HandleFunc("/webhook/ams", h.handleWebhook)
	// Per-source route (B7): dispatches using SourceSecrets[name] with
	// SharedSecret fallback for unknown source names.
	mux.HandleFunc("/webhook/ams/", h.handleWebhookPerSource)
	h.server = &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	return h
}

// Name implements collector.Source.
func (h *Handler) Name() string {
	return fmt.Sprintf("webhook(%s)", h.cfg.ListenAddr)
}

// Run implements collector.Source. It starts the HTTP server and blocks until
// ctx is cancelled.
func (h *Handler) Run(ctx context.Context) error {
	if h.cfg.SharedSecret == "" {
		h.logger.Error("webhook: shared secret not configured — ALL requests will be REJECTED (fail-closed)")
	}

	errCh := make(chan error, 1)
	go func() {
		h.logger.Info("webhook: listening", "addr", h.cfg.ListenAddr)
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return h.server.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

// HTTPHandler returns the http.Handler for use when embedding in the main server
// rather than running standalone.
func (h *Handler) HTTPHandler() http.Handler {
	return h.server.Handler
}

// handleWebhook is the HTTP handler for the legacy /webhook/ams route.
// It validates exclusively against SharedSecret; per-source secrets never apply.
// Behavior is byte-for-byte identical to the pre-B7 implementation.
func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	h.handleWebhookWithSecret(w, r, h.cfg.SharedSecret)
}

// handleWebhookPerSource is the HTTP handler for /webhook/ams/{name}.
// Secret selection (B7 design):
//   - If SourceSecrets[name] exists → validate against it ONLY (no SharedSecret
//     fallback — cross-source isolation is the security goal of B7).
//   - If no entry for name → fall back to SharedSecret if non-empty, else 401
//     (fail-closed; NOT 404 — do not leak which source names exist).
func (h *Handler) handleWebhookPerSource(w http.ResponseWriter, r *http.Request) {
	// Extract {name} from the URL path ("/webhook/ams/" prefix is 13 chars).
	name := r.URL.Path[len("/webhook/ams/"):]

	var secret string
	if s, ok := h.cfg.SourceSecrets[name]; ok {
		// Per-source secret exists — use it exclusively.
		secret = s
	} else {
		// Unknown source name — fall back to SharedSecret (may be empty →
		// validateHMAC returns false → 401).
		secret = h.cfg.SharedSecret
	}
	h.handleWebhookWithSecret(w, r, secret)
}

// handleWebhookWithSecret reads the body, validates the HMAC against secret,
// parses the payload and forwards events to the sink.
// Both /webhook/ams and /webhook/ams/{name} delegate here.
func (h *Handler) handleWebhookWithSecret(w http.ResponseWriter, r *http.Request, secret string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB max
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	// Validate HMAC signature. Fail-closed: an empty secret makes validateHMAC
	// reject every request (defense-in-depth).
	sig := r.Header.Get("X-Ams-Signature")

	// Replay protection (audit finding [8], D-123). Opt-in via RequireTimestamp:
	// when off, signedPayload is the bare body — byte-for-byte the original
	// contract. When on, require a fresh X-Ams-Timestamp and fold it into the
	// signed payload ("<ts>.<body>") so a captured request cannot be replayed
	// outside the ±TimestampSkew window and the timestamp cannot be stripped.
	signedPayload := body
	if h.cfg.RequireTimestamp {
		tsHeader := r.Header.Get("X-Ams-Timestamp")
		ts, perr := strconv.ParseInt(tsHeader, 10, 64)
		if perr != nil {
			h.logger.Warn("webhook: missing or invalid X-Ams-Timestamp", "remote", r.RemoteAddr)
			http.Error(w, "invalid timestamp", http.StatusUnauthorized)
			return
		}
		skew := int64(h.cfg.TimestampSkew / time.Second)
		now := h.now().Unix()
		if ts < now-skew || ts > now+skew {
			// Log ts and now (not a signed delta) so the direction of the skew is
			// unambiguous to an operator reading the structured log.
			h.logger.Warn("webhook: X-Ams-Timestamp outside acceptance window",
				"remote", r.RemoteAddr, "ts", ts, "now", now, "skew_limit_s", skew)
			http.Error(w, "stale timestamp", http.StatusUnauthorized)
			return
		}
		// Bind the timestamp into the HMAC input using its CANONICAL decimal form
		// (FormatInt of the parsed value) so a sender whose header is non-canonical
		// (leading '+' or zeros, both accepted by ParseInt) still verifies. The
		// sender signs "<decimal-unix-seconds>.<raw-body>" with the same secret.
		signedPayload = append([]byte(strconv.FormatInt(ts, 10)+"."), body...)
	}

	if !validateHMAC(signedPayload, sig, secret) {
		h.logger.Warn("webhook: invalid signature", "remote", r.RemoteAddr)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	events, err := h.parseWebhook(body)
	if err != nil {
		h.logger.Warn("webhook: parse error", "error", err)
		// Return 200 anyway — AMS may retry if we return non-2xx.
		w.WriteHeader(http.StatusOK)
		return
	}

	for _, ev := range events {
		h.sink.WriteServerEvent(ev)
	}

	w.WriteHeader(http.StatusOK)
}

// parseWebhook parses the AMS webhook body into domain events.
// AMS sends a JSON object; we handle multiple payload shapes across versions.
func (h *Handler) parseWebhook(body []byte) ([]domain.ServerEvent, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		// Try array form.
		var arr []map[string]json.RawMessage
		if err2 := json.Unmarshal(body, &arr); err2 != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		var events []domain.ServerEvent
		for _, item := range arr {
			evs := h.translateWebhook(item)
			events = append(events, evs...)
		}
		return events, nil
	}
	return h.translateWebhook(raw), nil
}

// translateWebhook maps one AMS webhook payload to domain events.
func (h *Handler) translateWebhook(raw map[string]json.RawMessage) []domain.ServerEvent {
	action := jsonString(raw["action"])
	if action == "" {
		action = jsonString(raw["event"])
	}
	if action == "" {
		action = jsonString(raw["type"])
	}

	streamID := jsonString(raw["streamId"])
	app := jsonString(raw["app"])
	if app == "" {
		app = jsonString(raw["appName"])
	}
	now := time.Now().UnixMilli()

	ev := domain.ServerEvent{
		Version:  1,
		TS:       now,
		Source:   domain.SourceWebhook,
		NodeID:   h.cfg.NodeID,
		App:      app,
		StreamID: streamID,
	}

	switch action {
	case "liveStreamStarted", "startBroadcast", "publish_started":
		ev.Type = domain.EventStreamPublishStart
		pt := jsonString(raw["publishType"])
		if pt == "" {
			pt = "other"
		}
		ev.Data = map[string]any{"publish_type": normalizePublishType(pt)}

	case "liveStreamEnded", "stopBroadcast", "publish_ended":
		ev.Type = domain.EventStreamPublishEnd
		ev.Data = map[string]any{
			"reason":     "webhook",
			"duration_s": jsonInt(raw["duration"]),
		}

	case "vodReady", "recording_ready":
		ev.Type = domain.EventRecordingReady
		ev.Data = map[string]any{
			"path":       jsonString(raw["vodName"]),
			"size_bytes": jsonInt64(raw["vodSize"]),
		}

	default:
		h.logger.Debug("webhook: unknown action, skipping", "action", action)
		return nil
	}

	return []domain.ServerEvent{ev}
}

// ─── HMAC validation ──────────────────────────────────────────────────────────

func validateHMAC(body []byte, signature, secret string) bool {
	if secret == "" {
		return false // fail-closed: never accept when no shared secret is configured
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}

// ─── JSON helpers ─────────────────────────────────────────────────────────────

func jsonString(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func jsonInt(raw json.RawMessage) int {
	if raw == nil {
		return 0
	}
	var n int
	_ = json.Unmarshal(raw, &n)
	return n
}

func jsonInt64(raw json.RawMessage) int64 {
	if raw == nil {
		return 0
	}
	var n int64
	_ = json.Unmarshal(raw, &n)
	return n
}

func normalizePublishType(t string) string {
	switch t {
	case "webrtc", "WebRTC", "WEBRTC":
		return "webrtc"
	case "rtmp", "RTMP":
		return "rtmp"
	case "hls", "HLS":
		return "hls"
	default:
		return "other"
	}
}
