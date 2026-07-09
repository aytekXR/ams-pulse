// Package webhook — B7 per-source webhook secret tests (D-062).
//
// TDD: these tests were written FIRST against the pre-implementation source
// (SourceSecrets field absent from Config, /webhook/ams/{name} route absent)
// to produce a compile error / 404 red phase.
//
// Design under test (ORCH-decided):
//   - /webhook/ams (legacy): validates against SharedSecret ONLY; per-source
//     secrets NEVER apply. Empty SharedSecret → 401.
//   - /webhook/ams/{name}: if SourceSecrets[name] exists → validate against it
//     ONLY (no SharedSecret fallback — cross-source isolation). If no entry for
//     name → fall back to SharedSecret if non-empty, else 401 (fail-closed; NOT
//     404 — do not leak which source names exist).
package webhook

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestHandlerWithSources builds a Handler with both a shared secret and a
// per-source secret map — used for B7 tests.
func newTestHandlerWithSources(t *testing.T, sharedSecret string, sourceSecrets map[string]string) (*Handler, *fakeSink) {
	t.Helper()
	sink := &fakeSink{}
	h := New(Config{
		NodeID:        "test-node",
		SharedSecret:  sharedSecret,
		SourceSecrets: sourceSecrets,
		ListenAddr:    ":0",
	}, sink, nil)
	return h, sink
}

// postToSource sends a POST to /webhook/ams/{name} through the handler's embedded mux.
func postToSource(t *testing.T, h *Handler, name string, body []byte, sigHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhook/ams/"+name, bytes.NewReader(body))
	if sigHeader != "" {
		req.Header.Set("X-Ams-Signature", sigHeader)
	}
	rr := httptest.NewRecorder()
	h.HTTPHandler().ServeHTTP(rr, req)
	return rr
}

// ─── B7 per-source secret tests ──────────────────────────────────────────────

// TestPerSource_RightSecret_200 verifies that the correct per-source secret
// produces HTTP 200 and delivers an event (B7-1).
func TestPerSource_RightSecret_200(t *testing.T) {
	secrets := map[string]string{"src-a": "secret-a"}
	h, sink := newTestHandlerWithSources(t, "", secrets)
	body := []byte(`{"action":"liveStreamStarted","streamId":"s1","app":"live"}`)
	sig := hmacSign(body, "secret-a")
	rr := postToSource(t, h, "src-a", body, sig)
	if rr.Code != http.StatusOK {
		t.Fatalf("B7-1: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if sink.Len() != 1 {
		t.Fatalf("B7-1: expected 1 event in sink, got %d", sink.Len())
	}
}

// TestPerSource_WrongSecret_401 verifies that a wrong per-source secret
// produces HTTP 401 (B7-2).
func TestPerSource_WrongSecret_401(t *testing.T) {
	secrets := map[string]string{"src-a": "secret-a"}
	h, sink := newTestHandlerWithSources(t, "", secrets)
	body := []byte(`{"action":"liveStreamStarted","streamId":"s1","app":"live"}`)
	sig := hmacSign(body, "wrong-secret")
	rr := postToSource(t, h, "src-a", body, sig)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("B7-2: expected 401, got %d", rr.Code)
	}
	if sink.Len() != 0 {
		t.Fatalf("B7-2: expected 0 events, got %d", sink.Len())
	}
}

// TestPerSource_CrossSourceSecret_401 verifies that using src-b's secret to
// post to src-a's path is rejected — cross-source isolation (B7-3).
func TestPerSource_CrossSourceSecret_401(t *testing.T) {
	secrets := map[string]string{
		"src-a": "secret-a",
		"src-b": "secret-b",
	}
	h, sink := newTestHandlerWithSources(t, "", secrets)
	body := []byte(`{"action":"liveStreamStarted","streamId":"s1","app":"live"}`)
	// Sign with src-b's secret but target src-a's path.
	sig := hmacSign(body, "secret-b")
	rr := postToSource(t, h, "src-a", body, sig)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("B7-3: expected 401 for cross-source secret, got %d", rr.Code)
	}
	if sink.Len() != 0 {
		t.Fatalf("B7-3: expected 0 events, got %d", sink.Len())
	}
}

// TestPerSource_LegacyPath_SharedSecret_200 verifies that the legacy
// /webhook/ams path still accepts the SharedSecret (B7-4).
func TestPerSource_LegacyPath_SharedSecret_200(t *testing.T) {
	secrets := map[string]string{"src-a": "secret-a"}
	h, sink := newTestHandlerWithSources(t, testSecret, secrets)
	body := []byte(`{"action":"liveStreamStarted","streamId":"s1","app":"live"}`)
	sig := hmacSign(body, testSecret)
	rr := post(t, h, body, sig) // hits /webhook/ams (legacy)
	if rr.Code != http.StatusOK {
		t.Fatalf("B7-4: expected 200, got %d", rr.Code)
	}
	if sink.Len() != 1 {
		t.Fatalf("B7-4: expected 1 event in sink, got %d", sink.Len())
	}
}

// TestPerSource_LegacyPath_RejectsPerSourceSecret_401 verifies that signing
// with a per-source secret and posting to the legacy /webhook/ams path is
// rejected — the legacy path ONLY validates SharedSecret (B7-5).
func TestPerSource_LegacyPath_RejectsPerSourceSecret_401(t *testing.T) {
	secrets := map[string]string{"src-a": "secret-a"}
	h, sink := newTestHandlerWithSources(t, testSecret, secrets)
	body := []byte(`{"action":"liveStreamStarted","streamId":"s1","app":"live"}`)
	// Sign with per-source secret, not the shared secret.
	sig := hmacSign(body, "secret-a")
	rr := post(t, h, body, sig) // hits /webhook/ams (legacy)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("B7-5: expected 401, got %d", rr.Code)
	}
	if sink.Len() != 0 {
		t.Fatalf("B7-5: expected 0 events, got %d", sink.Len())
	}
}

// TestPerSource_UnknownName_EmptySharedSecret_401 verifies that an unknown
// source name with no SharedSecret fallback returns 401 fail-closed (B7-6).
func TestPerSource_UnknownName_EmptySharedSecret_401(t *testing.T) {
	secrets := map[string]string{"src-a": "secret-a"}
	h, sink := newTestHandlerWithSources(t, "" /* no shared secret */, secrets)
	body := []byte(`{"action":"liveStreamStarted","streamId":"s1","app":"live"}`)
	sig := hmacSign(body, "secret-a") // arbitrary
	rr := postToSource(t, h, "unknown-src", body, sig)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("B7-6: expected 401 for unknown source + empty SharedSecret, got %d", rr.Code)
	}
	if sink.Len() != 0 {
		t.Fatalf("B7-6: expected 0 events, got %d", sink.Len())
	}
}

// TestPerSource_UnknownName_SharedSecretFallback_200 verifies that an unknown
// source name falls back to SharedSecret when it is non-empty (B7-7).
func TestPerSource_UnknownName_SharedSecretFallback_200(t *testing.T) {
	secrets := map[string]string{"src-a": "secret-a"}
	h, sink := newTestHandlerWithSources(t, testSecret, secrets)
	body := []byte(`{"action":"liveStreamStarted","streamId":"s1","app":"live"}`)
	sig := hmacSign(body, testSecret)
	rr := postToSource(t, h, "unknown-src", body, sig)
	if rr.Code != http.StatusOK {
		t.Fatalf("B7-7: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if sink.Len() != 1 {
		t.Fatalf("B7-7: expected 1 event in sink, got %d", sink.Len())
	}
}
