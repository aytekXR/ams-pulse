package amsclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aytekXR/ams-pulse/server/pkg/amsclient"
)

// newAuthTokenTestClient returns a Client with static-token auth configured.
func newAuthTokenTestClient(srv *httptest.Server, token string) *amsclient.Client {
	return amsclient.New(amsclient.Config{
		BaseURL:   srv.URL,
		AuthToken: token,
	})
}

// TestDoGet_AuthToken_BothHeaders verifies that with AuthToken configured every
// GET carries both:
//   - Authorization: Bearer <token>   (app-scope REST, jwtControlEnabled)
//   - ProxyAuthorization: <token>     (management REST, server.jwtServerControlEnabled)
//
// The ProxyAuthorization value must NOT include the "Bearer " prefix.
func TestDoGet_AuthToken_BothHeaders(t *testing.T) {
	const token = "eyJhbGciOiJIUzI1NiJ9.test"

	var gotAuth, gotProxy string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotProxy = r.Header.Get("ProxyAuthorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"applications":["LiveApp"]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := newAuthTokenTestClient(srv, token)
	// ListApplications hits /rest/v2/applications — a management-scope endpoint.
	_, _ = c.ListApplications(context.Background())

	wantAuth := "Bearer " + token
	if gotAuth != wantAuth {
		t.Errorf("Authorization = %q, want %q", gotAuth, wantAuth)
	}
	if gotProxy != token {
		t.Errorf("ProxyAuthorization = %q, want %q (no 'Bearer ' prefix)", gotProxy, token)
	}
}

// TestDoGet_NoAuthToken_NeitherHeader verifies that with no AuthToken configured
// neither Authorization nor ProxyAuthorization is sent.
func TestDoGet_NoAuthToken_NeitherHeader(t *testing.T) {
	var gotAuth, gotProxy string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotProxy = r.Header.Get("ProxyAuthorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"applications":["LiveApp"]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	// newTestClient uses no AuthToken.
	c := newTestClient(srv)
	_, _ = c.ListApplications(context.Background())

	if gotAuth != "" {
		t.Errorf("Authorization = %q, want empty (no token configured)", gotAuth)
	}
	if gotProxy != "" {
		t.Errorf("ProxyAuthorization = %q, want empty (no token configured)", gotProxy)
	}
}

// TestDoGet_CookieSessionMode_NoTokenHeaders verifies that cookie-session mode
// (LoginEmail/Password only, no AuthToken) does not add Authorization or
// ProxyAuthorization headers on data requests.
func TestDoGet_CookieSessionMode_NoTokenHeaders(t *testing.T) {
	var gotAuth, gotProxy string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/v2/users/authenticate":
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "sess-xyz"})
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success":true}`)) //nolint:errcheck
		case "/rest/v2/applications":
			gotAuth = r.Header.Get("Authorization")
			gotProxy = r.Header.Get("ProxyAuthorization")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"applications":["LiveApp"]}`)) //nolint:errcheck
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newLoginTestClient(srv, "admin@example.com", "secret")
	_, _ = c.ListApplications(context.Background())

	if gotAuth != "" {
		t.Errorf("Authorization = %q, want empty in cookie-session mode", gotAuth)
	}
	if gotProxy != "" {
		t.Errorf("ProxyAuthorization = %q, want empty in cookie-session mode", gotProxy)
	}
}

// TestDoGet_ProxyAuthorization_StrippedOnCrossHostRedirect verifies the token
// does not leak to a foreign host: net/http strips Authorization on cross-host
// redirects itself, but the non-standard "ProxyAuthorization" header is not in
// its sensitive-header list — the client's CheckRedirect must remove it whenever
// the redirect target's host differs from the origin (stricter than net/http:
// a port change alone counts as a different host).
func TestDoGet_ProxyAuthorization_StrippedOnCrossHostRedirect(t *testing.T) {
	const token = "eyJhbGciOiJIUzI1NiJ9.redirect"

	var gotProxy, gotAuth string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProxy = r.Header.Get("ProxyAuthorization")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"applications":[]}`)) //nolint:errcheck
	}))
	defer target.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound)
	}))
	defer origin.Close()

	c := newAuthTokenTestClient(origin, token)
	_, _ = c.ListApplications(context.Background())

	if gotProxy != "" {
		t.Errorf("ProxyAuthorization leaked across hosts: %q", gotProxy)
	}
	// Both httptest servers share the 127.0.0.1 hostname (different ports), and
	// net/http's own Authorization stripping compares hostnames only — so
	// Authorization still arrives here. This documents the asymmetry; the custom
	// rule for ProxyAuthorization is deliberately stricter (host:port match).
	_ = gotAuth
}
