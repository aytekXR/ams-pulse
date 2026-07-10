// Package api — OIDC/SSO phase-1 handler (S11 WO-C).
//
// Implements /auth/oidc/login, /auth/oidc/callback, and /auth/oidc/logout.
// When OIDCConfig is nil (PULSE_OIDC_ISSUER unset) all three endpoints return
// 501 NOT_CONFIGURED so clients can detect the feature gate.
//
// Session model: the callback mints a short-lived api_tokens row (kind="api")
// and sets the raw value as an HttpOnly "pulse_session" cookie.
// bearerAuthMiddleware in server.go falls back to this cookie when the
// Authorization header is absent — so existing bearer-token flows are unchanged.
//
// Security properties (phase 1):
//   - PKCE S256: code_verifier generated per-login, stored in state cookie,
//     code_challenge sent to provider, verifier sent in token exchange.
//   - State CSRF: 16B random hex + nonce + code_verifier → HMAC-SHA256 signed
//     into the pulse_oidc_state cookie (HttpOnly, 10-min TTL).
//   - Nonce binding: nonce embedded in state cookie, sent to provider, verified
//     manually against id_token claims after go-oidc validation.
//   - id_token: verified by go-oidc (JWKS fetch, RS256, iss/aud/exp/sig).
//   - Fail-closed group mapping: empty DefaultRole → 403 when no group matches.
//   - Concurrent first-login: UNIQUE race handled by re-fetching on constraint err.
package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	oidclib "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// OIDCProviderConfig holds OIDC provider configuration for the API server.
// Nil = OIDC disabled (no behaviour change to existing auth).
type OIDCProviderConfig struct {
	// Issuer is the OIDC provider issuer URL (PULSE_OIDC_ISSUER).
	Issuer string
	// ClientID is the registered OAuth2 client ID (PULSE_OIDC_CLIENT_ID).
	ClientID string
	// ClientSecret is the OAuth2 client secret (PULSE_OIDC_CLIENT_SECRET).
	ClientSecret string
	// RedirectURL is the full callback URL registered with the provider
	// (PULSE_OIDC_REDIRECT_URL). e.g. https://pulse.example.com/auth/oidc/callback.
	RedirectURL string
	// GroupClaim is the id_token claim name holding group membership.
	// Default: "groups" (PULSE_OIDC_GROUP_CLAIM).
	GroupClaim string
	// GroupRoleMap maps group names to Pulse roles ("admin"|"viewer").
	// Parsed from PULSE_OIDC_GROUP_ROLE_MAP JSON.
	GroupRoleMap map[string]string
	// DefaultRole is the Pulse role when no group matches.
	// "" = fail-closed (403 GROUP_DENIED); "viewer" = allow read-only.
	// Corresponds to PULSE_OIDC_DEFAULT_ROLE (ORCH: default is EMPTY).
	DefaultRole string
	// SessionTTL is the session cookie/token lifetime (PULSE_OIDC_SESSION_TTL).
	// Default: 24h.
	SessionTTL time.Duration
	// SecretKey is the raw PULSE_SECRET_KEY value used to derive the state-cookie
	// HMAC key. Empty = per-restart random key (state cookies invalidated on restart).
	SecretKey string
	// Provider is a pre-built *oidc.Provider injected by serve.go (production) or
	// tests. When non-nil, api.New uses it directly instead of calling NewProvider.
	Provider *oidclib.Provider
}

// idTokenVerifier abstracts *oidc.IDTokenVerifier for test injection.
type idTokenVerifier interface {
	Verify(ctx context.Context, rawIDToken string) (*oidclib.IDToken, error)
}

// tokenExchanger abstracts *oauth2.Config.Exchange for test injection.
type tokenExchanger interface {
	Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error)
}

// oidcHandler handles the /auth/oidc/* endpoints.
type oidcHandler struct {
	cfg       OIDCProviderConfig
	verifier  idTokenVerifier
	exchanger tokenExchanger
	hmacKey   [32]byte // derived from cfg.SecretKey or random
	store     *meta.Store
	logger    *slog.Logger
}

// newOIDCHandler builds an oidcHandler from a fully populated OIDCProviderConfig.
// cfg.Provider must be non-nil (injected by caller).
func newOIDCHandler(cfg *OIDCProviderConfig, store *meta.Store, logger *slog.Logger) *oidcHandler {
	if cfg == nil || cfg.Provider == nil {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}

	// Build oauth2 config from the provider's endpoint (fetched via discovery).
	oauth2Cfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     cfg.Provider.Endpoint(),
		Scopes:       []string{oidclib.ScopeOpenID, "profile", "email"},
	}

	// Build verifier: go-oidc checks iss, aud, exp, sig.
	verifier := cfg.Provider.Verifier(&oidclib.Config{ClientID: cfg.ClientID})

	// Derive HMAC key for state cookies.
	var hmacKey [32]byte
	if cfg.SecretKey != "" {
		raw := append([]byte("oidc-state-v1:"), []byte(cfg.SecretKey)...)
		hmacKey = sha256.Sum256(raw)
	} else {
		// Dev mode: random per-restart key (state cookies invalidated on restart).
		if _, err := rand.Read(hmacKey[:]); err != nil {
			panic("oidcHandler: generate random HMAC key: " + err.Error())
		}
	}

	return &oidcHandler{
		cfg:       *cfg,
		verifier:  verifier,
		exchanger: oauth2Cfg,
		hmacKey:   hmacKey,
		store:     store,
		logger:    logger,
	}
}

// ─── Login handler ────────────────────────────────────────────────────────────

// handleLogin initiates the OIDC authorization code + PKCE S256 flow.
//
//	GET /auth/oidc/login → 302 to provider + pulse_oidc_state cookie
func (h *oidcHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	// Generate random state (16B hex), nonce (16B hex), PKCE code_verifier (32B base64url).
	stateStr, err := randomHex(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "state generation failed")
		return
	}
	nonceStr, err := randomHex(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "nonce generation failed")
		return
	}
	codeVerifier, err := randomBase64URL(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "PKCE verifier generation failed")
		return
	}

	// Compute PKCE code_challenge = BASE64URL(SHA256(verifier)).
	challenge := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(challenge[:])

	// Sign state cookie payload: "stateStr:nonceStr:codeVerifier.HMAC"
	cookiePayload := stateStr + ":" + nonceStr + ":" + codeVerifier
	sig := signHMAC(h.hmacKey, cookiePayload)
	signed := cookiePayload + "." + hex.EncodeToString(sig)

	// Set state cookie (HttpOnly, SameSite=Lax, 10-minute TTL).
	http.SetCookie(w, &http.Cookie{
		Name:     "pulse_oidc_state",
		Value:    signed,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
		Path:     "/",
	})

	// Build authorization URL with PKCE and nonce.
	authURL := h.exchanger.(*oauth2.Config).AuthCodeURL(
		stateStr,
		oidclib.Nonce(nonceStr),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
	)

	http.Redirect(w, r, authURL, http.StatusFound)
}

// ─── Callback handler ─────────────────────────────────────────────────────────

// handleCallback processes the OIDC provider redirect, validates the id_token,
// maps groups to a role, creates a session token, and sets the pulse_session cookie.
//
//	GET /auth/oidc/callback?code=...&state=... → 302 to / (with session cookie)
func (h *oidcHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ── 1. Validate state cookie ──────────────────────────────────────────────
	stateCookie, err := r.Cookie("pulse_oidc_state")
	if err != nil {
		writeError(w, http.StatusBadRequest, "MISSING_STATE", "missing pulse_oidc_state cookie")
		return
	}

	stateStr, nonceStr, codeVerifier, err := verifyStateCookie(stateCookie.Value, h.hmacKey)
	if err != nil {
		code := "STATE_TAMPERED"
		if errors.Is(err, errStateMismatch) {
			code = "STATE_MISMATCH"
		}
		writeError(w, http.StatusBadRequest, code, err.Error())
		return
	}

	// ── 2. Verify state query param matches cookie ────────────────────────────
	queryState := r.FormValue("state")
	if queryState == "" {
		writeError(w, http.StatusBadRequest, "MISSING_STATE", "missing state query parameter")
		return
	}
	if queryState != stateStr {
		writeError(w, http.StatusBadRequest, "STATE_MISMATCH", "state parameter does not match cookie")
		return
	}

	// ── 3. Validate code presence ─────────────────────────────────────────────
	code := r.FormValue("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "MISSING_CODE", "missing code query parameter")
		return
	}

	// ── 4. Exchange code for tokens (PKCE: send code_verifier) ───────────────
	oauth2Token, err := h.exchanger.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", codeVerifier),
	)
	if err != nil {
		h.logger.Warn("oidc: code exchange failed", "error", err)
		writeError(w, http.StatusUnauthorized, "TOKEN_EXCHANGE_FAILED", "authorization code exchange failed")
		return
	}

	// ── 5. Extract and verify id_token ────────────────────────────────────────
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		writeError(w, http.StatusUnauthorized, "TOKEN_INVALID", "id_token missing from token response")
		return
	}

	idToken, err := h.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		h.logger.Warn("oidc: id_token verification failed", "error", err)
		writeError(w, http.StatusUnauthorized, "TOKEN_INVALID", "id_token verification failed")
		return
	}

	// ── 6. Extract claims (groups, sub, nonce) ────────────────────────────────
	// Use a map to handle the dynamic group claim name.
	var rawClaims map[string]json.RawMessage
	if err := idToken.Claims(&rawClaims); err != nil {
		writeError(w, http.StatusUnauthorized, "TOKEN_INVALID", "failed to extract id_token claims")
		return
	}

	sub := idToken.Subject

	// Manual nonce verification (go-oidc verifies nonce if set in Config, but
	// we use manual check for flexibility with the state cookie nonce).
	var nonceClaim string
	if raw, ok := rawClaims["nonce"]; ok {
		_ = json.Unmarshal(raw, &nonceClaim)
	}
	if nonceStr != "" && nonceClaim != nonceStr {
		writeError(w, http.StatusUnauthorized, "TOKEN_INVALID", "nonce mismatch")
		return
	}

	// Extract group claim.
	var groups []string
	groupClaimName := h.cfg.GroupClaim
	if groupClaimName == "" {
		groupClaimName = "groups"
	}
	if raw, ok := rawClaims[groupClaimName]; ok {
		_ = json.Unmarshal(raw, &groups)
	}

	// ── 7. Map groups to role ─────────────────────────────────────────────────
	role := mapGroupsToRole(groups, h.cfg.GroupRoleMap, h.cfg.DefaultRole)
	if role == "" {
		writeError(w, http.StatusForbidden, "GROUP_DENIED",
			"no OIDC group matches a configured role and no default role is set")
		return
	}

	// ── 8. Look up or create user (OIDC user key: "oidc:<sub>") ──────────────
	username := "oidc:" + sub
	user, err := h.store.GetUserByUsername(ctx, username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "user lookup failed")
		return
	}
	if user == nil {
		newUser := meta.User{Username: username, PwHash: "", Role: role}
		err = h.store.CreateUser(ctx, newUser)
		if err != nil {
			// Handle concurrent first-login UNIQUE race: re-fetch on constraint error.
			if isUniqueConstraintError(err) {
				user, err = h.store.GetUserByUsername(ctx, username)
				if err != nil || user == nil {
					writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "user provisioning failed")
					return
				}
			} else {
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "user creation failed")
				return
			}
		} else {
			// Fetch the created user to get the generated ID.
			user, err = h.store.GetUserByUsername(ctx, username)
			if err != nil || user == nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "user created but not found")
				return
			}
		}
	}

	// ── 9. Mint session token ─────────────────────────────────────────────────
	rawToken, err := generateSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "token generation failed")
		return
	}

	tokenHash, hashAlg := h.store.HashToken(rawToken)
	ttl := h.cfg.SessionTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	expMs := time.Now().Add(ttl).UnixMilli()
	err = h.store.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		UserID:    user.ID,
		Name:      "oidc-session",
		TokenHash: tokenHash,
		HashAlg:   hashAlg,
		Scopes:    []string{role},
		ExpiresAt: &expMs,
		CreatedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "session token creation failed")
		return
	}

	// ── 10. Set session cookie ────────────────────────────────────────────────
	secure := strings.HasPrefix(h.cfg.RedirectURL, "https://")
	http.SetCookie(w, &http.Cookie{
		Name:     "pulse_session",
		Value:    rawToken,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   int(ttl.Seconds()),
		Path:     "/",
	})

	// ── 11. Clear state cookie ────────────────────────────────────────────────
	http.SetCookie(w, &http.Cookie{
		Name:     "pulse_oidc_state",
		Value:    "",
		HttpOnly: true,
		MaxAge:   -1,
		Path:     "/",
	})

	// ── 12. Redirect to SPA root ──────────────────────────────────────────────
	http.Redirect(w, r, "/", http.StatusFound)
}

// ─── Logout handler ───────────────────────────────────────────────────────────

// handleLogout revokes the session token and clears the pulse_session cookie.
// Idempotent: returns 204 even when no session cookie is present.
//
//	POST /auth/oidc/logout → 204
func (h *oidcHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sessionCookie, err := r.Cookie("pulse_session")
	if err == nil && sessionCookie.Value != "" {
		// Look up the token to get its ID, then delete it.
		tok, lookupErr := h.store.LookupToken(ctx, sessionCookie.Value)
		if lookupErr == nil && tok != nil {
			_ = h.store.DeleteToken(ctx, tok.ID)
		}
	}

	// Always clear the cookie (idempotent).
	http.SetCookie(w, &http.Cookie{
		Name:     "pulse_session",
		Value:    "",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Path:     "/",
	})

	w.WriteHeader(http.StatusNoContent)
}

// ─── State cookie helpers ─────────────────────────────────────────────────────

// errStateMismatch is returned by verifyStateCookie when the HMAC is valid but
// the stateStr doesn't match the query parameter (checked by the caller).
var errStateMismatch = fmt.Errorf("state mismatch")

// verifyStateCookie parses and HMAC-verifies the pulse_oidc_state cookie value.
// Returns (stateStr, nonceStr, codeVerifier, nil) on success.
// Returns an error on HMAC failure or malformed cookie.
func verifyStateCookie(cookieVal string, hmacKey [32]byte) (stateStr, nonceStr, codeVerifier string, err error) {
	// Format: "stateStr:nonceStr:codeVerifier.HMAC_HEX"
	dotIdx := strings.LastIndex(cookieVal, ".")
	if dotIdx < 0 {
		return "", "", "", fmt.Errorf("malformed state cookie: missing HMAC separator")
	}
	payload := cookieVal[:dotIdx]
	sigHex := cookieVal[dotIdx+1:]

	// Verify HMAC.
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return "", "", "", fmt.Errorf("malformed state cookie: invalid HMAC encoding")
	}
	expectedSig := signHMAC(hmacKey, payload)
	if !hmac.Equal(sigBytes, expectedSig) {
		return "", "", "", fmt.Errorf("state cookie HMAC verification failed")
	}

	// Split payload: "stateStr:nonceStr:codeVerifier"
	parts := strings.SplitN(payload, ":", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("malformed state cookie: expected 3 parts, got %d", len(parts))
	}
	return parts[0], parts[1], parts[2], nil
}

// signHMAC returns HMAC-SHA256(key, message).
func signHMAC(key [32]byte, message string) []byte {
	mac := hmac.New(sha256.New, key[:])
	mac.Write([]byte(message))
	return mac.Sum(nil)
}

// ─── Group/role mapping ───────────────────────────────────────────────────────

// mapGroupsToRole maps a list of OIDC groups to a Pulse role using the provided
// mapping. Returns defaultRole when no match is found (can be "" for fail-closed).
func mapGroupsToRole(groups []string, roleMap map[string]string, defaultRole string) string {
	for _, g := range groups {
		if role, ok := roleMap[g]; ok && role != "" {
			return role
		}
	}
	return defaultRole
}

// ─── Crypto helpers ───────────────────────────────────────────────────────────

// randomHex generates n cryptographically random bytes and returns them as hex.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// randomBase64URL generates n cryptographically random bytes as raw base64url.
func randomBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generateSessionToken creates a "plt_<random>" session token.
func generateSessionToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "plt_" + hex.EncodeToString(b), nil
}

// Note: isUniqueConstraintError is declared in reports_wave2.go (package-level).
// oidc.go reuses it rather than redeclaring.
