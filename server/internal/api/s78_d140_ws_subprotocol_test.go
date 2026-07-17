// S73/D-140 [7] — the Live WebSocket must authenticate via the Sec-WebSocket-Protocol
// handshake header, so the browser no longer has to put the bearer token in the URL
// query (where reverse-proxy access logs would record it). The browser offers
// ["pulse.v1", <token>]; the server reads the token from the header and negotiates the
// "pulse.v1" marker. ?token= is retained only as a legacy fallback.
//
// Same "not 401 == auth passed" signal as TestLiveWS_AuthViaTokenQueryAndCookie (a
// plain non-WS test client can't complete the upgrade, but auth runs first).
//
// Mutation proof:
//   - remove `token = wsSubprotocolToken(r)` in downloadAuthMiddleware → the valid-token
//     subprotocol case goes RED (401).
//   - if wsSubprotocolToken returned the "pulse.v1" marker instead of skipping it, the
//     valid case would also go RED (LookupToken("pulse.v1") is invalid → 401).
package api_test

import (
	"net/http"
	"testing"
)

func TestLiveWS_AuthViaSubprotocol(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	notUnauthorized := func(t *testing.T, req *http.Request) {
		t.Helper()
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Logf("auth passed: connection error from non-WS client (expected): %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			t.Errorf("auth rejected (401) but should have passed via the Sec-WebSocket-Protocol token")
			return
		}
		t.Logf("auth passed: status=%d (WS upgrade attempted)", resp.StatusCode)
	}

	t.Run("valid token in Sec-WebSocket-Protocol header (no token in URL)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/live/ws", nil)
		req.Header.Set("Sec-WebSocket-Protocol", "pulse.v1, "+token)
		notUnauthorized(t, req)
	})

	t.Run("invalid token in subprotocol is rejected", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/live/ws", nil)
		req.Header.Set("Sec-WebSocket-Protocol", "pulse.v1, plt_not_a_real_token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /live/ws (bad subprotocol token): %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for an invalid subprotocol token, got %d", resp.StatusCode)
		}
	})

	t.Run("only the marker, no token, is rejected", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/live/ws", nil)
		req.Header.Set("Sec-WebSocket-Protocol", "pulse.v1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /live/ws (marker only): %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 when only the marker (no token) is sent, got %d", resp.StatusCode)
		}
	})
}
