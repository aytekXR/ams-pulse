// Package meta_test — regression guard for the SHA-256 fallback warn.
//
// When PULSE_SECRET_KEY is absent, New() must emit a slog.Warn so that
// operators are not silently running plain SHA-256 token hashes in production.
package meta_test

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// warnCapture is a minimal slog.Handler that records Warn-level messages.
type warnCapture struct {
	mu   sync.Mutex
	msgs []string
}

func (h *warnCapture) Enabled(_ context.Context, l slog.Level) bool { return true }
func (h *warnCapture) Handle(_ context.Context, r slog.Record) error {
	if r.Level == slog.LevelWarn {
		h.mu.Lock()
		h.msgs = append(h.msgs, r.Message)
		h.mu.Unlock()
	}
	return nil
}
func (h *warnCapture) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *warnCapture) WithGroup(_ string) slog.Handler      { return h }

func (h *warnCapture) WarnMessages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.msgs))
	copy(out, h.msgs)
	return out
}

// TestNew_EmptySecretKey_WarnsSHA256Fallback asserts that constructing a meta
// Store with an empty secret key emits a slog.Warn mentioning SHA-256 and
// PULSE_SECRET_KEY, so operators are never silently running plain hashes.
func TestNew_EmptySecretKey_WarnsSHA256Fallback(t *testing.T) {
	cap := &warnCapture{}
	prev := slog.Default()
	slog.SetDefault(slog.New(cap))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ctx := context.Background()
	// Empty secretKey triggers the SHA-256 fallback warn.
	s, err := meta.New(ctx, "sqlite", ":memory:", "")
	if err != nil {
		t.Fatalf("meta.New (empty key): %v", err)
	}
	defer s.Close()

	msgs := cap.WarnMessages()
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "SHA-256") && strings.Contains(m, "PULSE_SECRET_KEY") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a Warn mentioning SHA-256 and PULSE_SECRET_KEY, got: %v", msgs)
	}
}

// TestNew_WithSecretKey_NoSHA256Warn asserts that a store opened with an
// explicit secret key does NOT emit the SHA-256 fallback warning.
func TestNew_WithSecretKey_NoSHA256Warn(t *testing.T) {
	cap := &warnCapture{}
	prev := slog.Default()
	slog.SetDefault(slog.New(cap))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ctx := context.Background()
	s, err := meta.New(ctx, "sqlite", ":memory:", "explicit-secret-for-warn-test")
	if err != nil {
		t.Fatalf("meta.New (explicit key): %v", err)
	}
	defer s.Close()

	for _, m := range cap.WarnMessages() {
		if strings.Contains(m, "SHA-256") {
			t.Errorf("unexpected SHA-256 warn when explicit key is set: %q", m)
		}
	}
}
