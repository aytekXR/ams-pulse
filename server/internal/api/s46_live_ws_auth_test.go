// S46 (D-108) — the live WebSocket must authenticate the way a browser can
// actually reach it. A browser cannot set an Authorization header on a WS
// handshake, so it connects via `?token=` (web/src/api/client.ts) or rides its
// pulse_session cookie after an OIDC login. The route was under
// bearerAuthMiddleware (header/cookie only, ?token= intentionally rejected), so
// every browser WS connection 401'd. Fix: move /api/v1/live/ws to the
// downloadAuthMiddleware group (header / pulse_session cookie / ?token=) and let
// handleLiveWS read the validated token from ctxTokenKey.
//
// Auth failures return a clean 401 JSON response BEFORE the WS upgrade. When auth
// passes, websocket.Accept rejects the plain (non-upgrade) test client with a
// 4xx handshake error or a connection error — neither is 401. So "not 401" is the
// signal that auth passed, mirroring TestGuard_VDS2.
//
// Mutation proof:
//   - revert the route to bearerAuthMiddleware  → case "?token=" goes RED (401).
//   - revert handleLiveWS to header/?token= re-extraction → case "cookie" goes
//     RED (cookie-only request carries no header and no query token).
package api_test

import (
	"net/http"
	"testing"
)

func TestLiveWS_AuthViaTokenQueryAndCookie(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	// notUnauthorized performs the request and asserts auth PASSED: either a
	// non-401 handshake response, or a connection error from the non-WS client
	// (the WS upgrade, not auth, failed). A real auth failure is a clean 401.
	notUnauthorized := func(t *testing.T, req *http.Request) {
		t.Helper()
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Logf("auth passed: connection error from non-WS client (expected): %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			t.Errorf("auth rejected (401) but should have passed: %s", req.URL.String())
			return
		}
		t.Logf("auth passed: status=%d (WS upgrade attempted)", resp.StatusCode)
	}

	t.Run("valid ?token= query (browser WS path)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/live/ws?token="+token, nil)
		notUnauthorized(t, req)
	})

	t.Run("valid pulse_session cookie (OIDC session path)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/live/ws", nil)
		req.AddCookie(&http.Cookie{Name: "pulse_session", Value: token})
		notUnauthorized(t, req)
	})

	t.Run("invalid ?token= is rejected", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/live/ws?token=plt_not_a_real_token", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /live/ws (bad token): %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for invalid ?token=, got %d", resp.StatusCode)
		}
	})

	t.Run("no credentials is rejected", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/live/ws", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /live/ws (no auth): %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for no credentials, got %d", resp.StatusCode)
		}
	})
}
