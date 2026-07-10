// Package api_test — OIDC/SSO phase-1 tests (S11 WO-C).
//
// TDD order: RED tests written before implementation; run to capture failures,
// then implementation added to get GREEN.
//
// The mock OIDC server (mockOIDC) uses stdlib crypto only (crypto/rsa,
// crypto/rand, crypto/sha256, encoding/base64) — no additional test deps.
// go-oidc/v3 is used as the real verifier pointed at the mock, exercising the
// full JWKS-fetch+RS256-verify path for happy-path and security tests.
package api_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	oidclib "github.com/coreos/go-oidc/v3/oidc"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── Mock OIDC server ────────────────────────────────────────────────────────

// mockOIDC is a minimal OIDC provider for tests. It uses real RSA signing so
// that go-oidc's IDTokenVerifier can exercise the full JWKS+RS256 path.
type mockOIDC struct {
	srv     *httptest.Server
	privKey *rsa.PrivateKey
	keyID   string

	mu sync.Mutex // guards mutable fields below

	// Configurable per-test (set before calling callback):
	sub         string          // default "test-sub-123"
	email       string          // default "user@example.com"
	groups      []string        // default nil
	expDelta    time.Duration   // relative to now; default +1h; negative = expired
	badSigKey   *rsa.PrivateKey // when non-nil, sign with this key (JWKS still has privKey's pubkey → sig fails)
	overrideIss string          // when non-empty, override iss claim (e.g. "https://evil.com")
	overrideAud []string        // when non-nil, override aud claim
	groupClaim  string          // claim name for groups; default "groups"

	// currentNonce is set by doLogin after extracting nonce from the auth URL.
	// The token endpoint includes it in the id_token so go-oidc's nonce check passes.
	currentNonce string
}

func newMockOIDC(t *testing.T) *mockOIDC {
	t.Helper()

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("mockOIDC: generate RSA key: %v", err)
	}

	m := &mockOIDC{
		privKey:    privKey,
		keyID:      "test-key-1",
		sub:        "test-sub-123",
		email:      "user@example.com",
		expDelta:   time.Hour,
		groupClaim: "groups",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", m.discoveryHandler)
	mux.HandleFunc("/.well-known/jwks.json", m.jwksHandler)
	mux.HandleFunc("/token", m.tokenHandler)
	mux.HandleFunc("/authorize", m.authorizeHandler)

	m.srv = httptest.NewServer(mux)
	t.Cleanup(func() { m.srv.Close() })
	return m
}

func (m *mockOIDC) issuer() string { return m.srv.URL }

func (m *mockOIDC) discoveryHandler(w http.ResponseWriter, r *http.Request) {
	doc := map[string]any{
		"issuer":                                m.issuer(),
		"authorization_endpoint":                m.issuer() + "/authorize",
		"token_endpoint":                        m.issuer() + "/token",
		"jwks_uri":                              m.issuer() + "/.well-known/jwks.json",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doc)
}

func (m *mockOIDC) jwksHandler(w http.ResponseWriter, r *http.Request) {
	pub := &m.privKey.PublicKey // always expose the ORIGINAL pubkey (even when badSigKey is set)
	nBytes := pub.N.Bytes()

	// Encode exponent as big-endian bytes, trimming leading zeros.
	eVal := pub.E
	var eBytes []byte
	for eVal > 0 {
		eBytes = append([]byte{byte(eVal & 0xff)}, eBytes...)
		eVal >>= 8
	}

	jwk := map[string]any{
		"kty": "RSA",
		"use": "sig",
		"kid": m.keyID,
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(nBytes),
		"e":   base64.RawURLEncoding.EncodeToString(eBytes),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{jwk}})
}

func (m *mockOIDC) tokenHandler(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	nonce := m.currentNonce
	sub := m.sub
	email := m.email
	groups := m.groups
	expDelta := m.expDelta
	badSigKey := m.badSigKey
	overrideIss := m.overrideIss
	overrideAud := m.overrideAud
	groupClaim := m.groupClaim
	m.mu.Unlock()

	iss := m.issuer()
	if overrideIss != "" {
		iss = overrideIss
	}
	aud := []string{"test-client-id"}
	if overrideAud != nil {
		aud = overrideAud
	}

	claims := map[string]any{
		"iss":   iss,
		"aud":   aud,
		"sub":   sub,
		"email": email,
		"exp":   time.Now().Add(expDelta).Unix(),
		"iat":   time.Now().Unix(),
		"nonce": nonce,
	}
	if len(groups) > 0 {
		claims[groupClaim] = groups
	}

	idToken := m.signJWT(claims, badSigKey)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": "dummy-access-token",
		"token_type":   "Bearer",
		"id_token":     idToken,
		"expires_in":   3600,
	})
}

func (m *mockOIDC) authorizeHandler(w http.ResponseWriter, r *http.Request) {
	// Just record the nonce for use in the next token response.
	nonce := r.URL.Query().Get("nonce")
	if nonce != "" {
		m.mu.Lock()
		m.currentNonce = nonce
		m.mu.Unlock()
	}
	// In tests we never actually follow the redirect to the provider,
	// so this just returns 200 for inspection.
	w.WriteHeader(http.StatusOK)
}

// signJWT signs a JWT with the given claims. Uses sigKey if non-nil,
// otherwise uses m.privKey (the key whose pubkey is in JWKS).
func (m *mockOIDC) signJWT(claims map[string]any, sigKey *rsa.PrivateKey) string {
	header := map[string]string{
		"alg": "RS256",
		"kid": m.keyID,
		"typ": "JWT",
	}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	h := base64.RawURLEncoding.EncodeToString(headerJSON)
	p := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := h + "." + p

	key := m.privKey
	if sigKey != nil {
		key = sigKey
	}

	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		panic("mockOIDC.signJWT: " + err.Error())
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

// oidcTestEnv bundles a running test server with its mock OIDC provider.
type oidcTestEnv struct {
	mock  *mockOIDC
	store *meta.Store
	srv   *httptest.Server
}

// setupOIDCTestServer starts an API server with OIDC configured against the mock.
// defaultRole is "" for fail-closed, "viewer" for permissive default.
func setupOIDCTestServer(t *testing.T, defaultRole string, groupRoleMap map[string]string) *oidcTestEnv {
	t.Helper()
	return setupOIDCTestServerRedirect(t, defaultRole, groupRoleMap, "http://example.com/auth/oidc/callback")
}

// setupOIDCTestServerRedirect is setupOIDCTestServer with an explicit redirect
// URL — used to test the Secure cookie flag under an https redirect URL.
func setupOIDCTestServerRedirect(t *testing.T, defaultRole string, groupRoleMap map[string]string, redirectURL string) *oidcTestEnv {
	t.Helper()
	ctx := context.Background()

	mock := newMockOIDC(t)

	// Real go-oidc provider pointing to the mock — exercises full JWKS+RS256 path.
	provider, err := oidclib.NewProvider(ctx, mock.issuer())
	if err != nil {
		t.Fatalf("setupOIDCTestServer: oidc.NewProvider: %v", err)
	}

	// Meta store (in-memory SQLite).
	ddlPath := oidcMetaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		t.Fatalf("setupOIDCTestServer: read DDL (need repo-root mount): %v", err)
	}
	store, err := meta.New(ctx, "sqlite", ":memory:", "oidc-test-secret")
	if err != nil {
		t.Fatalf("setupOIDCTestServer: meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("setupOIDCTestServer: migrate: %v", err)
	}

	// Also create a bearer test token for regression tests.
	bearerToken := "plt_oidc_bearer_test"
	bHash := oidcHashToken(bearerToken)
	_ = store.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "oidc-regression-bearer",
		TokenHash: bHash,
		Scopes:    []string{"admin"},
	})

	lic, _ := license.New("", "")
	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)

	apiCfg := api.Config{
		OIDCConfig: &api.OIDCProviderConfig{
			Issuer:       mock.issuer(),
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURL:  redirectURL,
			GroupClaim:   "groups",
			GroupRoleMap: groupRoleMap,
			DefaultRole:  defaultRole,
			SessionTTL:   24 * time.Hour,
			SecretKey:    "oidc-unit-test-hmac-key",
			Provider:     provider,
		},
	}

	srv := api.New(apiCfg, store, live, qsvc, lic, nil)
	ts := httptest.NewServer(srv.Handler())

	t.Cleanup(func() {
		ts.Close()
		store.Close()
	})

	return &oidcTestEnv{mock: mock, store: store, srv: ts}
}

// doLogin calls GET /auth/oidc/login and returns the state cookie value and the
// state parameter from the Location redirect URL.  It also extracts the nonce
// and stores it on the mock so that the token endpoint can include it.
func (env *oidcTestEnv) doLogin(t *testing.T) (stateCookieVal, stateParam string) {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(env.srv.URL + "/auth/oidc/login")
	if err != nil {
		t.Fatalf("doLogin: GET /auth/oidc/login: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("doLogin: expected 302, got %d: %s", resp.StatusCode, body)
	}

	loc := resp.Header.Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("doLogin: parse Location %q: %v", loc, err)
	}
	stateParam = u.Query().Get("state")

	// Extract nonce from auth URL and store on mock.
	nonce := u.Query().Get("nonce")
	if nonce != "" {
		env.mock.mu.Lock()
		env.mock.currentNonce = nonce
		env.mock.mu.Unlock()
	}

	for _, c := range resp.Cookies() {
		if c.Name == "pulse_oidc_state" {
			stateCookieVal = c.Value
			break
		}
	}
	if stateCookieVal == "" {
		t.Fatal("doLogin: no pulse_oidc_state cookie in login response")
	}
	return
}

// doCallback calls GET /auth/oidc/callback with the given state cookie and params,
// does NOT follow redirects, and returns the raw response.
func (env *oidcTestEnv) doCallback(t *testing.T, stateCookieVal, stateParam string) *http.Response {
	t.Helper()
	return doCallbackRaw(t, env.srv.URL, stateCookieVal, stateParam, "test-code")
}

// doCallbackRaw is the low-level helper that lets tests control all parameters.
func doCallbackRaw(t *testing.T, baseURL, stateCookieVal, stateParam, code string) *http.Response {
	t.Helper()
	callbackURL := baseURL + "/auth/oidc/callback"
	if code != "" {
		callbackURL += "?code=" + url.QueryEscape(code)
	}
	if stateParam != "" {
		if strings.Contains(callbackURL, "?") {
			callbackURL += "&state=" + url.QueryEscape(stateParam)
		} else {
			callbackURL += "?state=" + url.QueryEscape(stateParam)
		}
	}

	req, err := http.NewRequest(http.MethodGet, callbackURL, nil)
	if err != nil {
		t.Fatalf("doCallbackRaw: new request: %v", err)
	}
	if stateCookieVal != "" {
		req.AddCookie(&http.Cookie{Name: "pulse_oidc_state", Value: stateCookieVal})
	}

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("doCallbackRaw: do request: %v", err)
	}
	return resp
}

// sessionCookieFrom returns the value of the pulse_session cookie, or "".
func sessionCookieFrom(resp *http.Response) string {
	for _, c := range resp.Cookies() {
		if c.Name == "pulse_session" {
			return c.Value
		}
	}
	return ""
}

func oidcMetaDDLPath(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file),
		"..", "..", "..", "contracts", "db", "meta", "0001_init.sql"))
}

func oidcHashToken(tok string) string {
	h := sha256.New()
	h.Write([]byte(tok))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// assertErrorCode reads the JSON body and checks the "code" field.
func assertErrorCode(t *testing.T, resp *http.Response, wantCode string) {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("assertErrorCode: read body: %v", err)
	}
	var errResp struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("assertErrorCode: parse JSON %q: %v", string(body), err)
	}
	if errResp.Code != wantCode {
		t.Errorf("assertErrorCode: got code=%q, want %q (body: %s)", errResp.Code, wantCode, string(body))
	}
}

// ─── Test: OIDC disabled ─────────────────────────────────────────────────────

func TestOIDC_Disabled_Returns501(t *testing.T) {
	// Standard server with no OIDCConfig → all OIDC routes return 501.
	ts, _, cleanup := setupTestServer(t)
	defer cleanup()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	for _, path := range []string{"/auth/oidc/login", "/auth/oidc/callback"} {
		resp, err := client.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotImplemented {
			t.Errorf("GET %s: expected 501, got %d", path, resp.StatusCode)
		}
		assertErrorCode(t, resp, "NOT_CONFIGURED")
	}

	// POST /auth/oidc/logout
	resp, err := client.Post(ts.URL+"/auth/oidc/logout", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /auth/oidc/logout: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("POST /auth/oidc/logout: expected 501, got %d", resp.StatusCode)
	}
}

// ─── Test: Login redirect ─────────────────────────────────────────────────────

func TestOIDC_Login_Redirect302(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(env.srv.URL + "/auth/oidc/login")
	if err != nil {
		t.Fatalf("GET /auth/oidc/login: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302, got %d: %s", resp.StatusCode, body)
	}

	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, env.mock.issuer()+"/authorize") {
		t.Errorf("Location %q does not start at mock authorize endpoint", loc)
	}
	u, _ := url.Parse(loc)
	if u.Query().Get("state") == "" {
		t.Error("Location URL missing state parameter")
	}
	if u.Query().Get("code_challenge") == "" {
		t.Error("Location URL missing code_challenge (PKCE S256 required)")
	}
	if u.Query().Get("code_challenge_method") != "S256" {
		t.Errorf("expected code_challenge_method=S256, got %q", u.Query().Get("code_challenge_method"))
	}

	// Must have pulse_oidc_state cookie.
	found := false
	for _, c := range resp.Cookies() {
		if c.Name == "pulse_oidc_state" {
			found = true
		}
	}
	if !found {
		t.Error("expected pulse_oidc_state cookie in Set-Cookie")
	}
}

func TestOIDC_Login_StateIsRandom(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	states := make(map[string]bool)
	for i := 0; i < 3; i++ {
		resp, err := client.Get(env.srv.URL + "/auth/oidc/login")
		if err != nil {
			t.Fatalf("GET /auth/oidc/login: %v", err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		u, _ := url.Parse(resp.Header.Get("Location"))
		state := u.Query().Get("state")
		if states[state] {
			t.Errorf("state value %q repeated across login calls", state)
		}
		states[state] = true
	}
}

func TestOIDC_Login_StateCookie_HttpOnly(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(env.srv.URL + "/auth/oidc/login")
	if err != nil {
		t.Fatalf("GET /auth/oidc/login: %v", err)
	}
	defer resp.Body.Close()

	// Parse the raw Set-Cookie header to check HttpOnly and Max-Age.
	setCookie := resp.Header.Get("Set-Cookie")
	if !strings.Contains(strings.ToLower(setCookie), "httponly") {
		t.Errorf("pulse_oidc_state cookie missing HttpOnly; Set-Cookie: %q", setCookie)
	}
	for _, c := range resp.Cookies() {
		if c.Name == "pulse_oidc_state" {
			if c.MaxAge <= 0 || c.MaxAge > 600 {
				t.Errorf("pulse_oidc_state Max-Age should be 1–600, got %d", c.MaxAge)
			}
		}
	}
}

// ─── Callback failure paths ───────────────────────────────────────────────────

func TestOIDC_Callback_MissingCode_400(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)
	stateCookieVal, stateParam := env.doLogin(t)

	// Omit code param.
	resp := doCallbackRaw(t, env.srv.URL, stateCookieVal, stateParam, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	assertErrorCode(t, resp, "MISSING_CODE")
}

func TestOIDC_Callback_MissingState_400(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	// No state cookie + no state param.
	resp := doCallbackRaw(t, env.srv.URL, "", "", "test-code")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	assertErrorCode(t, resp, "MISSING_STATE")
}

func TestOIDC_Callback_StateMismatch_400(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)
	stateCookieVal, _ := env.doLogin(t)

	// Pass wrong state param (doesn't match cookie).
	resp := doCallbackRaw(t, env.srv.URL, stateCookieVal, "bad-state-value", "test-code")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	assertErrorCode(t, resp, "STATE_MISMATCH")
}

func TestOIDC_Callback_ExpiredToken_401(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)
	env.mock.mu.Lock()
	env.mock.expDelta = -2 * time.Hour // expired
	env.mock.mu.Unlock()

	stateCookieVal, stateParam := env.doLogin(t)
	resp := env.doCallback(t, stateCookieVal, stateParam)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 401, got %d: %s", resp.StatusCode, body)
	}
}

func TestOIDC_Callback_BadSignature_401(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	// Generate a different key for signing. JWKS still has original pubkey.
	badKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate bad key: %v", err)
	}
	env.mock.mu.Lock()
	env.mock.badSigKey = badKey
	env.mock.mu.Unlock()

	stateCookieVal, stateParam := env.doLogin(t)
	resp := env.doCallback(t, stateCookieVal, stateParam)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 401, got %d: %s", resp.StatusCode, body)
	}
}

func TestOIDC_Callback_WrongIssuer_401(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)
	env.mock.mu.Lock()
	env.mock.overrideIss = "https://evil.example.com"
	env.mock.mu.Unlock()

	stateCookieVal, stateParam := env.doLogin(t)
	resp := env.doCallback(t, stateCookieVal, stateParam)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 401, got %d: %s", resp.StatusCode, body)
	}
}

func TestOIDC_Callback_WrongAudience_401(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)
	env.mock.mu.Lock()
	env.mock.overrideAud = []string{"wrong-client-id"}
	env.mock.mu.Unlock()

	stateCookieVal, stateParam := env.doLogin(t)
	resp := env.doCallback(t, stateCookieVal, stateParam)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 401, got %d: %s", resp.StatusCode, body)
	}
}

// ─── Group/role mapping ───────────────────────────────────────────────────────

func TestOIDC_Callback_GroupAdmin_ScopesAdmin(t *testing.T) {
	env := setupOIDCTestServer(t, "", map[string]string{"ops-admins": "admin"})
	env.mock.mu.Lock()
	env.mock.groups = []string{"ops-admins"}
	env.mock.mu.Unlock()

	stateCookieVal, stateParam := env.doLogin(t)
	resp := env.doCallback(t, stateCookieVal, stateParam)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302, got %d: %s", resp.StatusCode, body)
	}

	sessionToken := sessionCookieFrom(resp)
	if sessionToken == "" {
		t.Fatal("expected pulse_session cookie")
	}

	tok, err := env.store.LookupToken(context.Background(), sessionToken)
	if err != nil || tok == nil {
		t.Fatalf("LookupToken: err=%v tok=%v", err, tok)
	}
	scopes := tok.Scopes
	if len(scopes) == 0 || scopes[0] != "admin" {
		t.Errorf("expected scopes=[admin], got %v", scopes)
	}
}

func TestOIDC_Callback_GroupViewer_ScopesViewer(t *testing.T) {
	env := setupOIDCTestServer(t, "", map[string]string{"viewers": "viewer"})
	env.mock.mu.Lock()
	env.mock.groups = []string{"viewers"}
	env.mock.mu.Unlock()

	stateCookieVal, stateParam := env.doLogin(t)
	resp := env.doCallback(t, stateCookieVal, stateParam)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302, got %d: %s", resp.StatusCode, body)
	}

	sessionToken := sessionCookieFrom(resp)
	tok, err := env.store.LookupToken(context.Background(), sessionToken)
	if err != nil || tok == nil {
		t.Fatalf("LookupToken: %v %v", err, tok)
	}
	if len(tok.Scopes) == 0 || tok.Scopes[0] != "viewer" {
		t.Errorf("expected scopes=[viewer], got %v", tok.Scopes)
	}
}

func TestOIDC_Callback_NoGroupMatch_DefaultViewer(t *testing.T) {
	// groups = ["unknown"], default_role = "viewer" → success with viewer scope
	env := setupOIDCTestServer(t, "viewer", map[string]string{"ops-admins": "admin"})
	env.mock.mu.Lock()
	env.mock.groups = []string{"unknown-group"}
	env.mock.mu.Unlock()

	stateCookieVal, stateParam := env.doLogin(t)
	resp := env.doCallback(t, stateCookieVal, stateParam)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302, got %d: %s", resp.StatusCode, body)
	}

	sessionToken := sessionCookieFrom(resp)
	tok, err := env.store.LookupToken(context.Background(), sessionToken)
	if err != nil || tok == nil {
		t.Fatalf("LookupToken: %v %v", err, tok)
	}
	if len(tok.Scopes) == 0 || tok.Scopes[0] != "viewer" {
		t.Errorf("expected scopes=[viewer], got %v", tok.Scopes)
	}
}

func TestOIDC_Callback_NoGroupMatch_NoDefault_403(t *testing.T) {
	// ORCH ruling: default_role="" = FAIL-CLOSED
	env := setupOIDCTestServer(t, "", map[string]string{"ops-admins": "admin"})
	env.mock.mu.Lock()
	env.mock.groups = []string{"unknown-group"}
	env.mock.mu.Unlock()

	stateCookieVal, stateParam := env.doLogin(t)
	resp := env.doCallback(t, stateCookieVal, stateParam)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 403, got %d: %s", resp.StatusCode, body)
	}
	assertErrorCode(t, resp, "GROUP_DENIED")
}

func TestOIDC_Callback_NoGroupsClaim_DefaultRole(t *testing.T) {
	// id_token has no groups claim, default_role="viewer" → viewer
	env := setupOIDCTestServer(t, "viewer", nil)
	// groups stays nil → no groups claim in token

	stateCookieVal, stateParam := env.doLogin(t)
	resp := env.doCallback(t, stateCookieVal, stateParam)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302, got %d: %s", resp.StatusCode, body)
	}

	sessionToken := sessionCookieFrom(resp)
	tok, err := env.store.LookupToken(context.Background(), sessionToken)
	if err != nil || tok == nil {
		t.Fatalf("LookupToken: %v %v", err, tok)
	}
	if len(tok.Scopes) == 0 || tok.Scopes[0] != "viewer" {
		t.Errorf("expected scopes=[viewer], got %v", tok.Scopes)
	}
}

// ─── Happy path ───────────────────────────────────────────────────────────────

func TestOIDC_Callback_Success_SessionCookieAndRedirect(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	stateCookieVal, stateParam := env.doLogin(t)
	resp := env.doCallback(t, stateCookieVal, stateParam)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302, got %d: %s", resp.StatusCode, body)
	}

	// Must redirect to /
	loc := resp.Header.Get("Location")
	if loc != "/" {
		t.Errorf("expected Location=/, got %q", loc)
	}

	// Must have pulse_session cookie with HttpOnly and SameSite=Lax.
	found := false
	for _, c := range resp.Cookies() {
		if c.Name == "pulse_session" {
			found = true
			if !c.HttpOnly {
				t.Error("pulse_session cookie missing HttpOnly")
			}
			if c.SameSite != http.SameSiteLaxMode {
				t.Errorf("pulse_session SameSite: got %v, want Lax", c.SameSite)
			}
			if c.MaxAge <= 0 {
				t.Errorf("pulse_session MaxAge should be positive, got %d", c.MaxAge)
			}
		}
	}
	if !found {
		t.Error("expected pulse_session cookie in Set-Cookie")
	}
}

// TestOIDC_Callback_NonceMismatch_401: an id_token whose nonce claim does not
// match the nonce bound into the HMAC-signed state cookie must be rejected.
// Pins the manual nonce check in handleOIDCCallback — if that check were
// removed, this test fails (the rest of the flow is valid).
func TestOIDC_Callback_NonceMismatch_401(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	stateCookieVal, stateParam := env.doLogin(t)

	// doLogin stored the real nonce on the mock; forge a different one so the
	// id_token's nonce claim no longer matches the state-cookie nonce.
	env.mock.mu.Lock()
	env.mock.currentNonce = "forged-nonce-value"
	env.mock.mu.Unlock()

	resp := env.doCallback(t, stateCookieVal, stateParam)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 401 on nonce mismatch, got %d: %s", resp.StatusCode, body)
	}
	assertErrorCode(t, resp, "TOKEN_INVALID")
}

// TestOIDC_Callback_SecureCookie_HTTPSRedirectURL: when the configured redirect
// URL is https, the pulse_session cookie must carry Secure=true.
func TestOIDC_Callback_SecureCookie_HTTPSRedirectURL(t *testing.T) {
	env := setupOIDCTestServerRedirect(t, "viewer", nil, "https://pulse.example.com/auth/oidc/callback")

	stateCookieVal, stateParam := env.doLogin(t)
	resp := env.doCallback(t, stateCookieVal, stateParam)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302, got %d: %s", resp.StatusCode, body)
	}
	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "pulse_session" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("no pulse_session cookie set")
	}
	if !sessionCookie.Secure {
		t.Error("pulse_session cookie must have Secure=true when the OIDC redirect URL is https")
	}
}

func TestOIDC_Callback_Success_TokenInStore(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	stateCookieVal, stateParam := env.doLogin(t)
	resp := env.doCallback(t, stateCookieVal, stateParam)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302, got %d: %s", resp.StatusCode, body)
	}

	sessionToken := sessionCookieFrom(resp)
	if sessionToken == "" {
		t.Fatal("no pulse_session cookie")
	}

	tok, err := env.store.LookupToken(context.Background(), sessionToken)
	if err != nil {
		t.Fatalf("LookupToken: %v", err)
	}
	if tok == nil {
		t.Fatal("token not found in store")
	}
	if tok.Kind != "api" {
		t.Errorf("expected kind=api, got %q", tok.Kind)
	}
}

func TestOIDC_Callback_Success_SessionTTL(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	stateCookieVal, stateParam := env.doLogin(t)
	before := time.Now()
	resp := env.doCallback(t, stateCookieVal, stateParam)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302, got %d: %s", resp.StatusCode, body)
	}

	sessionToken := sessionCookieFrom(resp)
	tok, err := env.store.LookupToken(context.Background(), sessionToken)
	if err != nil || tok == nil {
		t.Fatalf("LookupToken: %v %v", err, tok)
	}
	if tok.ExpiresAt == nil {
		t.Fatal("token ExpiresAt should be set")
	}
	expMS := *tok.ExpiresAt
	expTime := time.UnixMilli(expMS)
	ttl := expTime.Sub(before)
	// Should be approx 24h (within 5s).
	if ttl < 24*time.Hour-5*time.Second || ttl > 24*time.Hour+5*time.Second {
		t.Errorf("expected TTL≈24h, got %v", ttl)
	}
}

// ─── Integration tests ────────────────────────────────────────────────────────

func TestOIDC_SessionCookie_AcceptedByAPI(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	stateCookieVal, stateParam := env.doLogin(t)
	cbResp := env.doCallback(t, stateCookieVal, stateParam)
	defer cbResp.Body.Close()

	if cbResp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(cbResp.Body)
		t.Fatalf("callback: expected 302, got %d: %s", cbResp.StatusCode, body)
	}

	sessionToken := sessionCookieFrom(cbResp)
	if sessionToken == "" {
		t.Fatal("no pulse_session cookie after callback")
	}

	// Use the session cookie to access /api/v1/live/overview.
	req, _ := http.NewRequest(http.MethodGet, env.srv.URL+"/api/v1/live/overview", nil)
	req.AddCookie(&http.Cookie{Name: "pulse_session", Value: sessionToken})
	apiResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/live/overview: %v", err)
	}
	defer apiResp.Body.Close()
	if apiResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(apiResp.Body)
		t.Errorf("expected 200, got %d: %s", apiResp.StatusCode, body)
	}
}

func TestOIDC_ExistingBearerToken_StillWorks(t *testing.T) {
	// Existing bearer-token flow must be unchanged when OIDC is configured.
	env := setupOIDCTestServer(t, "viewer", nil)

	req, _ := http.NewRequest(http.MethodGet, env.srv.URL+"/api/v1/live/overview", nil)
	req.Header.Set("Authorization", "Bearer plt_oidc_bearer_test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/live/overview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200, got %d: %s", resp.StatusCode, body)
	}
}

func TestOIDC_ExpiredSession_401(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	// Create an already-expired token in the store.
	expiredToken := "plt_oidc_expired_test"
	expiredHash := oidcHashToken(expiredToken)
	expMs := time.Now().Add(-1 * time.Hour).UnixMilli()
	_ = env.store.CreateToken(context.Background(), meta.APIToken{
		Kind:      "api",
		Name:      "expired-session",
		TokenHash: expiredHash,
		Scopes:    []string{"viewer"},
		ExpiresAt: &expMs,
	})

	req, _ := http.NewRequest(http.MethodGet, env.srv.URL+"/api/v1/live/overview", nil)
	req.AddCookie(&http.Cookie{Name: "pulse_session", Value: expiredToken})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/live/overview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

// ─── Logout tests ─────────────────────────────────────────────────────────────

func TestOIDC_Logout_RevokesSession(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	// First do a full login to get a session.
	stateCookieVal, stateParam := env.doLogin(t)
	cbResp := env.doCallback(t, stateCookieVal, stateParam)
	defer cbResp.Body.Close()
	if cbResp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(cbResp.Body)
		t.Fatalf("callback: expected 302, got %d: %s", cbResp.StatusCode, body)
	}
	sessionToken := sessionCookieFrom(cbResp)
	if sessionToken == "" {
		t.Fatal("no session token after callback")
	}

	// Verify the session works before logout.
	req, _ := http.NewRequest(http.MethodGet, env.srv.URL+"/api/v1/live/overview", nil)
	req.AddCookie(&http.Cookie{Name: "pulse_session", Value: sessionToken})
	apiResp, _ := http.DefaultClient.Do(req)
	apiResp.Body.Close()
	if apiResp.StatusCode != http.StatusOK {
		t.Fatalf("pre-logout: expected 200, got %d", apiResp.StatusCode)
	}

	// Now logout.
	logoutReq, _ := http.NewRequest(http.MethodPost, env.srv.URL+"/auth/oidc/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: "pulse_session", Value: sessionToken})
	logoutResp, err := http.DefaultClient.Do(logoutReq)
	if err != nil {
		t.Fatalf("POST /auth/oidc/logout: %v", err)
	}
	io.Copy(io.Discard, logoutResp.Body)
	logoutResp.Body.Close()
	if logoutResp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", logoutResp.StatusCode)
	}

	// Session cookie should now be cleared (MaxAge=-1 or 0 in response).
	cookieCleared := false
	for _, c := range logoutResp.Cookies() {
		if c.Name == "pulse_session" && c.MaxAge < 0 {
			cookieCleared = true
		}
	}
	if !cookieCleared {
		t.Error("expected pulse_session cookie to be cleared after logout")
	}

	// Token should now be rejected by the API.
	req2, _ := http.NewRequest(http.MethodGet, env.srv.URL+"/api/v1/live/overview", nil)
	req2.AddCookie(&http.Cookie{Name: "pulse_session", Value: sessionToken})
	apiResp2, _ := http.DefaultClient.Do(req2)
	io.Copy(io.Discard, apiResp2.Body)
	apiResp2.Body.Close()
	if apiResp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 after logout, got %d", apiResp2.StatusCode)
	}
}

func TestOIDC_Logout_IdempotentWithoutCookie(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	// Logout with no session cookie → must still return 204 (idempotent).
	req, _ := http.NewRequest(http.MethodPost, env.srv.URL+"/auth/oidc/logout", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /auth/oidc/logout: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

// ─── Conformance: OIDC disabled → 501 conforms to Error schema ───────────────

// TestConformance_OIDCLogin_501_WhenDisabled checks that the 501 response body
// from a disabled-OIDC server conforms to the Error schema defined in pulse-api.yaml.
// Note: the OIDC paths are at the API root (not under /api/v1), so we validate
// the body structure manually against the spec's Error schema rather than using
// the kin-openapi router (which resolves paths relative to the /api/v1 server URL).
func TestConformance_OIDCLogin_501_WhenDisabled(t *testing.T) {
	ts, _, cleanup := setupTestServer(t)
	defer cleanup()

	// Verify the spec has the Error schema and the /auth/oidc/login 501 response.
	doc := openAPISpec(t)
	if doc.Paths.Map()["/auth/oidc/login"] == nil {
		t.Fatal("conformance: /auth/oidc/login not found in spec — spec must define this path")
	}

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(ts.URL + "/auth/oidc/login")
	if err != nil {
		t.Fatalf("GET /auth/oidc/login: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", resp.StatusCode)
	}

	// Validate response body conforms to the Error schema: {code, message} both strings.
	body, _ := io.ReadAll(resp.Body)
	var errBody struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &errBody); err != nil {
		t.Errorf("501 body is not valid JSON: %v (body: %s)", err, body)
		return
	}
	if errBody.Code == "" {
		t.Errorf("Error body missing 'code' field (body: %s)", body)
	}
	if errBody.Message == "" {
		t.Errorf("Error body missing 'message' field (body: %s)", body)
	}
}

// ─── Phase-2: /auth/oidc/status + /auth/me tests ─────────────────────────────

// TestOIDCStatus_Enabled: OIDC configured → 200 {"enabled": true}
func TestOIDCStatus_Enabled(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	resp, err := http.Get(env.srv.URL + "/auth/oidc/status")
	if err != nil {
		t.Fatalf("GET /auth/oidc/status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var status struct {
		Enabled bool `json:"enabled"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &status); err != nil {
		t.Fatalf("parse JSON %q: %v", string(body), err)
	}
	if !status.Enabled {
		t.Errorf("expected enabled=true, got false (body: %s)", body)
	}
}

// TestOIDCStatus_Disabled: no OIDC → 200 {"enabled": false}
func TestOIDCStatus_Disabled(t *testing.T) {
	ts, _, cleanup := setupTestServer(t)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/auth/oidc/status")
	if err != nil {
		t.Fatalf("GET /auth/oidc/status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var status struct {
		Enabled bool `json:"enabled"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &status); err != nil {
		t.Fatalf("parse JSON %q: %v", string(body), err)
	}
	if status.Enabled {
		t.Errorf("expected enabled=false, got true (body: %s)", body)
	}
}

// TestAuthMe_Bearer: bearer token → 200 {name, role, auth_method: "bearer"}
func TestAuthMe_Bearer(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	req, _ := http.NewRequest(http.MethodGet, env.srv.URL+"/auth/me", nil)
	req.Header.Set("Authorization", "Bearer plt_oidc_bearer_test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /auth/me: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var me struct {
		Name       string `json:"name"`
		Role       string `json:"role"`
		AuthMethod string `json:"auth_method"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("parse JSON: %v (body: %s)", err, body)
	}
	if me.AuthMethod != "bearer" {
		t.Errorf("expected auth_method=bearer, got %q (body: %s)", me.AuthMethod, body)
	}
	if me.Name == "" {
		t.Errorf("expected non-empty name (body: %s)", body)
	}
	if me.Role == "" {
		t.Errorf("expected non-empty role (body: %s)", body)
	}
}

// TestAuthMe_Cookie: pulse_session cookie → 200 {auth_method: "cookie"}
func TestAuthMe_Cookie(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	// Complete a full OIDC login to obtain a session cookie.
	stateCookieVal, stateParam := env.doLogin(t)
	cbResp := env.doCallback(t, stateCookieVal, stateParam)
	defer cbResp.Body.Close()
	if cbResp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(cbResp.Body)
		t.Fatalf("callback: expected 302, got %d: %s", cbResp.StatusCode, body)
	}
	sessionToken := sessionCookieFrom(cbResp)
	if sessionToken == "" {
		t.Fatal("no pulse_session cookie after callback")
	}

	req, _ := http.NewRequest(http.MethodGet, env.srv.URL+"/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "pulse_session", Value: sessionToken})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /auth/me (cookie): %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var me struct {
		Name       string `json:"name"`
		Role       string `json:"role"`
		AuthMethod string `json:"auth_method"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("parse JSON: %v (body: %s)", err, body)
	}
	if me.AuthMethod != "cookie" {
		t.Errorf("expected auth_method=cookie, got %q (body: %s)", me.AuthMethod, body)
	}
	if me.Name == "" {
		t.Errorf("expected non-empty name (body: %s)", body)
	}
	if me.Role == "" {
		t.Errorf("expected non-empty role (body: %s)", body)
	}
}

// TestAuthMe_Unauthenticated: no token → 401
func TestAuthMe_Unauthenticated(t *testing.T) {
	env := setupOIDCTestServer(t, "viewer", nil)

	resp, err := http.Get(env.srv.URL + "/auth/me")
	if err != nil {
		t.Fatalf("GET /auth/me: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 401, got %d: %s", resp.StatusCode, body)
	}
}

// ─── Unused import guard (big.Int is used in JWKS exponent encoding) ─────────
var _ = big.NewInt
