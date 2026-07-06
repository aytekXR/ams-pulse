package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// fakeSink collects events written by the handler.
type fakeSink struct {
	mu     sync.Mutex
	events []domain.ServerEvent
}

func (f *fakeSink) WriteServerEvent(ev domain.ServerEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, ev)
}

func (f *fakeSink) WriteBeaconEvent(_ domain.BeaconEvent)     {}
func (f *fakeSink) WriteViewerSession(_ domain.ViewerSession) {}

func (f *fakeSink) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.events)
}

// hmacSign produces the "sha256=<hex>" signature expected by validateHMAC.
func hmacSign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

const testSecret = "super-secret-key-for-tests"

// newTestHandler builds a Handler whose HTTP mux can be exercised via httptest.
func newTestHandler(t *testing.T, secret string) (*Handler, *fakeSink) {
	t.Helper()
	sink := &fakeSink{}
	h := New(Config{
		NodeID:       "test-node",
		SharedSecret: secret,
		ListenAddr:   ":0", // not actually bound in unit tests
	}, sink, nil)
	return h, sink
}

// post sends a POST to /webhook/ams through the handler's embedded mux.
func post(t *testing.T, h *Handler, body []byte, sigHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhook/ams", bytes.NewReader(body))
	if sigHeader != "" {
		req.Header.Set("X-Ams-Signature", sigHeader)
	}
	rr := httptest.NewRecorder()
	h.HTTPHandler().ServeHTTP(rr, req)
	return rr
}

// TestHMACAccepted verifies that a correctly-signed request is accepted (200)
// and the resulting event is forwarded to the sink.
func TestHMACAccepted(t *testing.T) {
	h, sink := newTestHandler(t, testSecret)

	body := []byte(`{"action":"liveStreamStarted","streamId":"s1","app":"live"}`)
	sig := hmacSign(body, testSecret)

	rr := post(t, h, body, sig)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if sink.Len() != 1 {
		t.Fatalf("expected 1 event in sink, got %d", sink.Len())
	}
}

// TestHMACRejectedBadSignature verifies that a request with a wrong signature
// is rejected with 401 and no event is forwarded.
func TestHMACRejectedBadSignature(t *testing.T) {
	h, sink := newTestHandler(t, testSecret)

	body := []byte(`{"action":"liveStreamStarted","streamId":"s1","app":"live"}`)
	badSig := hmacSign(body, "wrong-secret")

	rr := post(t, h, body, badSig)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if sink.Len() != 0 {
		t.Fatalf("expected 0 events in sink, got %d", sink.Len())
	}
}

// TestHMACRejectedMissingSignature verifies that a request with no signature
// header is rejected with 401 when a secret is configured (fail-closed).
func TestHMACRejectedMissingSignature(t *testing.T) {
	h, sink := newTestHandler(t, testSecret)

	body := []byte(`{"action":"liveStreamStarted","streamId":"s1","app":"live"}`)

	// Pass empty string as sigHeader — header not set at all.
	rr := post(t, h, body, "")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing signature, got %d", rr.Code)
	}
	if sink.Len() != 0 {
		t.Fatalf("expected 0 events in sink, got %d", sink.Len())
	}
}

// TestValidateHMACConstantTime exercises validateHMAC directly to confirm the
// constant-time comparison holds: identical bodies with same secret match, and
// bodies/secrets that differ do not.
func TestValidateHMACConstantTime(t *testing.T) {
	body := []byte(`{"action":"test"}`)
	goodSig := hmacSign(body, testSecret)

	if !validateHMAC(body, goodSig, testSecret) {
		t.Fatal("expected valid HMAC to pass")
	}
	if validateHMAC(body, "sha256=badhex", testSecret) {
		t.Fatal("expected invalid hex signature to fail")
	}
	if validateHMAC(body, goodSig, "other-secret") {
		t.Fatal("expected signature from wrong secret to fail")
	}
	if validateHMAC([]byte(`{"action":"tampered"}`), goodSig, testSecret) {
		t.Fatal("expected tampered body to fail")
	}
}

// TestRunContextCancel verifies that Run exits cleanly when the context is
// cancelled (i.e., the server shuts down gracefully).
func TestRunContextCancel(t *testing.T) {
	sink := &fakeSink{}
	h := New(Config{
		NodeID:       "test-node",
		SharedSecret: testSecret,
		ListenAddr:   "127.0.0.1:0",
	}, sink, nil)

	// Override the server addr to use a random free port.
	h.server.Addr = "127.0.0.1:0"

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- h.Run(ctx)
	}()

	// Give the listener a moment to start.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not exit after context cancel")
	}
}

// TestEndToEndWebhookTCPListener exercises the webhook handler over a real TCP
// listener with a real net/http client (not httptest.ResponseRecorder).
//
// (a) A correctly HMAC-signed POST to /webhook/ams must return HTTP 200 and
//
//	deliver exactly one event to the fanout consumer within a deadline.
//
// (b) A POST with a bad/missing signature must return HTTP 401 and must NOT
//
//	deliver any event to the fanout.
func TestEndToEndWebhookTCPListener(t *testing.T) {
	t.Parallel()

	// Bind a real OS-assigned TCP port so there is no fixed-port collision risk.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	addr := ln.Addr().String()

	// Build the webhook handler (shared secret matches what the client will sign).
	sink := &fakeSink{}
	h := New(Config{
		NodeID:       "e2e-test-node",
		SharedSecret: testSecret,
		ListenAddr:   addr,
	}, sink, nil)

	// Start serving over the real TCP listener.  Use http.Server.Serve so that
	// we can pass the already-bound listener and know the exact port.
	srv := &http.Server{Handler: h.HTTPHandler()}
	srvDone := make(chan error, 1)
	go func() { srvDone <- srv.Serve(ln) }()
	t.Cleanup(func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		<-srvDone
	})

	client := &http.Client{Timeout: 5 * time.Second}
	url := "http://" + addr + "/webhook/ams"

	// ── (a) Good signature → HTTP 200 and event arrives at fanout ────────────
	bodyGood := []byte(`{"action":"liveStreamStarted","streamId":"s1","app":"live"}`)
	sigGood := hmacSign(bodyGood, testSecret)

	reqGood, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyGood))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	reqGood.Header.Set("X-Ams-Signature", sigGood)

	respGood, err := client.Do(reqGood)
	if err != nil {
		t.Fatalf("client.Do (good sig): %v", err)
	}
	_ = respGood.Body.Close()

	if respGood.StatusCode != http.StatusOK {
		t.Errorf("(a) expected 200, got %d", respGood.StatusCode)
	}

	// handleWebhook writes to the sink before returning 200, so the event must
	// already be present when we read the response.  Use a short polling loop
	// as a safety net against any future async refactoring.
	deadline := time.After(5 * time.Second)
	for sink.Len() < 1 {
		select {
		case <-deadline:
			t.Fatalf("(a) event did not arrive at fanout within deadline; sink len=%d", sink.Len())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if sink.Len() != 1 {
		t.Errorf("(a) expected exactly 1 event, got %d", sink.Len())
	}

	// ── (b) Bad signature → HTTP 401 and no event delivered ──────────────────
	bodyBad := []byte(`{"action":"liveStreamStarted","streamId":"s2","app":"live"}`)
	sigBad := hmacSign(bodyBad, "wrong-secret")

	reqBad, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBad))
	if err != nil {
		t.Fatalf("NewRequest (bad sig): %v", err)
	}
	reqBad.Header.Set("X-Ams-Signature", sigBad)

	respBad, err := client.Do(reqBad)
	if err != nil {
		t.Fatalf("client.Do (bad sig): %v", err)
	}
	_ = respBad.Body.Close()

	if respBad.StatusCode != http.StatusUnauthorized {
		t.Errorf("(b) expected 401, got %d", respBad.StatusCode)
	}

	// Allow a brief window for any spurious async write, then assert count is
	// still 1 (the one from the good-signature request above).
	time.Sleep(50 * time.Millisecond)
	if sink.Len() != 1 {
		t.Errorf("(b) expected sink to still have 1 event after bad-sig request, got %d", sink.Len())
	}
}
