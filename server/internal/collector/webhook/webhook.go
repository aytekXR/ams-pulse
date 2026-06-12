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
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// Config holds webhook handler configuration.
type Config struct {
	// NodeID is stamped on all emitted events.
	NodeID string

	// SharedSecret is the HMAC-SHA256 secret used to validate AMS webhook
	// requests. If empty, signature validation is skipped (warn on startup).
	SharedSecret string

	// ListenAddr is the address for the webhook HTTP server (e.g. ":8091").
	// If empty, the handler is embedded rather than standalone.
	ListenAddr string
}

// Handler implements collector.Source as an HTTP server receiving AMS webhooks.
type Handler struct {
	cfg    Config
	sink   domain.EventSink
	logger *slog.Logger
	server *http.Server
}

// New creates a webhook Handler.
func New(cfg Config, sink domain.EventSink, logger *slog.Logger) *Handler {
	if cfg.NodeID == "" {
		cfg.NodeID = "standalone"
	}
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{cfg: cfg, sink: sink, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook/ams", h.handleWebhook)
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
		h.logger.Warn("webhook: shared secret not configured — signature validation disabled")
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

// handleWebhook is the HTTP handler for AMS webhook POST requests.
func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB max
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	// Validate HMAC signature if configured.
	if h.cfg.SharedSecret != "" {
		sig := r.Header.Get("X-Ams-Signature")
		if !validateHMAC(body, sig, h.cfg.SharedSecret) {
			h.logger.Warn("webhook: invalid signature", "remote", r.RemoteAddr)
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
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
