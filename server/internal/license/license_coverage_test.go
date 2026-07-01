// Package license_test extends coverage for signed-key paths, expiry handling,
// Refresh/ExpiresAt/Valid, and tier-gated checks (Pro, Business, Enterprise).
//
// These are characterisation tests: they pass against the current production
// code and are designed to fail on plausible regressions.
package license_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/license"
)

// ─── key fixture ──────────────────────────────────────────────────────────────

// testKeys holds a generated ed25519 key pair used to produce signed license keys.
type testKeys struct {
	pubHex  string
	privKey ed25519.PrivateKey
}

func generateKeys(t *testing.T) *testKeys {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	return &testKeys{
		pubHex:  hex.EncodeToString(pub),
		privKey: priv,
	}
}

// signKey returns a "<base64(claimsJSON)>.<base64(sig)>" license key string.
func (k *testKeys) signKey(t *testing.T, claimsMap map[string]interface{}) string {
	t.Helper()
	claimsJSON, err := json.Marshal(claimsMap)
	if err != nil {
		t.Fatalf("json.Marshal claims: %v", err)
	}
	sig := ed25519.Sign(k.privKey, claimsJSON)
	return base64.StdEncoding.EncodeToString(claimsJSON) + "." +
		base64.StdEncoding.EncodeToString(sig)
}

// install sets PULSE_LICENSE_PUBKEY to this fixture's public key for the
// duration of the test (restored on cleanup via t.Setenv).
func (k *testKeys) install(t *testing.T) {
	t.Helper()
	t.Setenv("PULSE_LICENSE_PUBKEY", k.pubHex)
}

// newMgr is a shorthand: generate a key pair, install it, sign claims, and
// return a Manager that accepts keys signed by that pair.
func newMgr(t *testing.T, claimsMap map[string]interface{}) *license.Manager {
	t.Helper()
	kf := generateKeys(t)
	kf.install(t)
	key := kf.signKey(t, claimsMap)
	mgr, err := license.New(key, "")
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}
	return mgr
}

// ─── New() with valid signed keys ─────────────────────────────────────────────

func TestNew_ValidProKey_TierAndEntitlements(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":           "pro",
		"data_api":       true,
		"white_label":    false,
		"max_nodes":      10,
		"retention_days": 90,
	})

	if mgr.Tier() != license.TierPro {
		t.Errorf("tier: want %q got %q", license.TierPro, mgr.Tier())
	}
	ent := mgr.Entitlements()
	if !ent.DataAPI {
		t.Error("pro tier: DataAPI must be true")
	}
	if ent.WhiteLabel {
		t.Error("pro tier: WhiteLabel must be false")
	}
	if ent.MaxNodes != 10 {
		t.Errorf("pro tier: MaxNodes want 10 got %d", ent.MaxNodes)
	}
	if ent.RetentionDays != 90 {
		t.Errorf("pro tier: RetentionDays want 90 got %d", ent.RetentionDays)
	}
	// Pro allows slack but NOT pagerduty.
	if err := mgr.CheckChannelAllowed("slack"); err != nil {
		t.Errorf("pro tier must allow slack: %v", err)
	}
	if err := mgr.CheckChannelAllowed("pagerduty"); err == nil {
		t.Error("pro tier must block pagerduty")
	}
}

func TestNew_ValidBusinessKey_TierAndEntitlements(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":      "business",
		"data_api":  true,
		"max_nodes": 5,
	})

	if mgr.Tier() != license.TierBusiness {
		t.Errorf("tier: want %q got %q", license.TierBusiness, mgr.Tier())
	}
	ent := mgr.Entitlements()
	if !ent.DataAPI {
		t.Error("business tier: DataAPI must be true")
	}
	// Business tier includes pagerduty and webhook.
	channelSet := make(map[string]bool, len(ent.Channels))
	for _, ch := range ent.Channels {
		channelSet[ch] = true
	}
	for _, required := range []string{"pagerduty", "webhook", "email", "slack", "telegram"} {
		if !channelSet[required] {
			t.Errorf("business tier: missing channel %q", required)
		}
	}
	if ent.MaxNodes != 5 {
		t.Errorf("business tier: MaxNodes want 5 got %d", ent.MaxNodes)
	}
}

func TestNew_ValidEnterpriseKey_TierAndEntitlements(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":        "enterprise",
		"data_api":    true,
		"white_label": true,
		// max_nodes omitted → unlimited (-1)
	})

	if mgr.Tier() != license.TierEnterprise {
		t.Errorf("tier: want %q got %q", license.TierEnterprise, mgr.Tier())
	}
	ent := mgr.Entitlements()
	if !ent.DataAPI {
		t.Error("enterprise tier: DataAPI must be true")
	}
	if !ent.WhiteLabel {
		t.Error("enterprise tier: WhiteLabel must be true")
	}
	// max_nodes omitted in claims means unlimited.
	if ent.MaxNodes != -1 {
		t.Errorf("enterprise tier: MaxNodes want -1 (unlimited) got %d", ent.MaxNodes)
	}
}

// ─── Valid() ──────────────────────────────────────────────────────────────────

func TestValid_FreeTierIsValid(t *testing.T) {
	mgr, err := license.New("", "")
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}
	if !mgr.Valid() {
		t.Error("free tier must report valid")
	}
}

func TestValid_SignedKeyIsValid(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "pro",
		"data_api": true,
	})
	if !mgr.Valid() {
		t.Error("manager with valid signed key must report valid")
	}
}

// ─── Expired key → fail-open → Free tier ──────────────────────────────────────

func TestNew_ExpiredKey_FallsBackToFree(t *testing.T) {
	pastMs := time.Now().Add(-24 * time.Hour).UnixMilli()
	kf := generateKeys(t)
	kf.install(t)
	key := kf.signKey(t, map[string]interface{}{
		"tier":       "pro",
		"data_api":   true,
		"expires_at": pastMs,
	})

	mgr, err := license.New(key, "")
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}
	// activate() returns an error for expired keys; New() fails open → Free tier.
	if mgr.Tier() != license.TierFree {
		t.Errorf("expired key: want free tier (fail-open) got %q", mgr.Tier())
	}
}

// ─── ExpiresAt() ──────────────────────────────────────────────────────────────

func TestExpiresAt_NilForFree(t *testing.T) {
	mgr, _ := license.New("", "")
	if mgr.ExpiresAt() != nil {
		t.Errorf("free tier ExpiresAt: want nil got %v", mgr.ExpiresAt())
	}
}

func TestExpiresAt_NilForPerpetualKey(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "pro",
		"data_api": true,
		// expires_at omitted → perpetual
	})
	if mgr.ExpiresAt() != nil {
		t.Errorf("perpetual key ExpiresAt: want nil got %v", mgr.ExpiresAt())
	}
}

func TestExpiresAt_SetForKeyWithFutureExpiry(t *testing.T) {
	future := time.Now().Add(30 * 24 * time.Hour)
	futureMs := future.UnixMilli()

	mgr := newMgr(t, map[string]interface{}{
		"tier":       "pro",
		"data_api":   true,
		"expires_at": futureMs,
	})

	exp := mgr.ExpiresAt()
	if exp == nil {
		t.Fatal("ExpiresAt: want non-nil for key with expires_at")
	}
	// Allow 1-second tolerance for millisecond round-trip.
	diff := exp.Sub(time.UnixMilli(futureMs).UTC())
	if diff < -time.Second || diff > time.Second {
		t.Errorf("ExpiresAt: want %v got %v (diff %v)", time.UnixMilli(futureMs).UTC(), *exp, diff)
	}
}

// ─── activate() error paths → fail-open ───────────────────────────────────────

func TestNew_InvalidKeyFormat_FallsBackToFree(t *testing.T) {
	// No "." separator — activate splits into 1 part and errors.
	mgr, err := license.New("thishasnodot", "")
	if err != nil {
		t.Fatalf("license.New returned unexpected error: %v", err)
	}
	if mgr.Tier() != license.TierFree {
		t.Errorf("invalid key format: want free tier got %q", mgr.Tier())
	}
}

func TestNew_BadBase64Claims_FallsBackToFree(t *testing.T) {
	// "!!!" is not valid base64.
	mgr, err := license.New("!!!.abc", "")
	if err != nil {
		t.Fatalf("license.New returned unexpected error: %v", err)
	}
	if mgr.Tier() != license.TierFree {
		t.Errorf("bad base64 claims: want free tier got %q", mgr.Tier())
	}
}

func TestNew_BadBase64Sig_FallsBackToFree(t *testing.T) {
	validClaims := base64.StdEncoding.EncodeToString([]byte(`{"tier":"pro"}`))
	mgr, err := license.New(validClaims+".!!!", "")
	if err != nil {
		t.Fatalf("license.New returned unexpected error: %v", err)
	}
	if mgr.Tier() != license.TierFree {
		t.Errorf("bad base64 sig: want free tier got %q", mgr.Tier())
	}
}

func TestNew_WrongSignature_FallsBackToFree(t *testing.T) {
	// Syntactically valid key but the signature is all-zeros (forged).
	claims := base64.StdEncoding.EncodeToString([]byte(`{"tier":"pro"}`))
	fakeSig := base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	mgr, err := license.New(claims+"."+fakeSig, "")
	if err != nil {
		t.Fatalf("license.New returned unexpected error: %v", err)
	}
	if mgr.Tier() != license.TierFree {
		t.Errorf("wrong signature: want free tier (fail-open) got %q", mgr.Tier())
	}
}

// ─── offline file path ────────────────────────────────────────────────────────

func TestNew_OfflineFile_LoadsSignedKey(t *testing.T) {
	kf := generateKeys(t)
	kf.install(t)
	key := kf.signKey(t, map[string]interface{}{
		"tier":     "pro",
		"data_api": true,
	})

	dir := t.TempDir()
	path := filepath.Join(dir, "license.key")
	// Write with trailing newline — activate uses TrimSpace so this must be handled.
	if err := os.WriteFile(path, []byte(key+"\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	mgr, err := license.New("", path)
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}
	if mgr.Tier() != license.TierPro {
		t.Errorf("offline file: want pro tier got %q", mgr.Tier())
	}
}

func TestNew_OfflineFile_NotFound_FallsBackToFree(t *testing.T) {
	mgr, err := license.New("", "/nonexistent/path/to/license.key")
	if err != nil {
		t.Fatalf("license.New returned unexpected error: %v", err)
	}
	if mgr.Tier() != license.TierFree {
		t.Errorf("missing offline file: want free tier got %q", mgr.Tier())
	}
}

// ─── Refresh() ────────────────────────────────────────────────────────────────

func TestRefresh_EmptyKey_DowngradesToFree(t *testing.T) {
	kf := generateKeys(t)
	kf.install(t)
	key := kf.signKey(t, map[string]interface{}{"tier": "pro", "data_api": true})
	mgr, _ := license.New(key, "")
	if mgr.Tier() != license.TierPro {
		t.Fatalf("pre-condition: want pro got %q", mgr.Tier())
	}

	if err := mgr.Refresh(context.Background(), ""); err != nil {
		t.Fatalf("Refresh empty: %v", err)
	}
	if mgr.Tier() != license.TierFree {
		t.Errorf("after Refresh(''): want free got %q", mgr.Tier())
	}
}

func TestRefresh_ValidKey_UpdatesTier(t *testing.T) {
	// The Manager uses the pubKey that was set at New() time; both keys must be
	// signed with the same key pair.
	kf := generateKeys(t)
	kf.install(t)
	businessKey := kf.signKey(t, map[string]interface{}{"tier": "business", "data_api": true})
	mgr, _ := license.New(businessKey, "")
	if mgr.Tier() != license.TierBusiness {
		t.Fatalf("pre-condition: want business got %q", mgr.Tier())
	}

	enterpriseKey := kf.signKey(t, map[string]interface{}{
		"tier": "enterprise", "data_api": true, "white_label": true,
	})
	if err := mgr.Refresh(context.Background(), enterpriseKey); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if mgr.Tier() != license.TierEnterprise {
		t.Errorf("after Refresh: want enterprise got %q", mgr.Tier())
	}
}

// ─── CheckNodeLimit ───────────────────────────────────────────────────────────

func TestCheckNodeLimit_ProTier_WithinLimit(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":      "pro",
		"data_api":  true,
		"max_nodes": 10,
	})
	if err := mgr.CheckNodeLimit(10); err != nil {
		t.Errorf("pro tier 10/10 nodes: want nil got %v", err)
	}
}

func TestCheckNodeLimit_ProTier_ExceedsLimit(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":      "pro",
		"data_api":  true,
		"max_nodes": 10,
	})
	if err := mgr.CheckNodeLimit(11); err == nil {
		t.Error("pro tier 11/10 nodes: want error got nil")
	}
}

func TestCheckNodeLimit_OmittedMaxNodes_IsUnlimited(t *testing.T) {
	// max_nodes absent in claims → nil → buildEntitlements sets -1 → unlimited.
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "enterprise",
		"data_api": true,
	})
	if err := mgr.CheckNodeLimit(99999); err != nil {
		t.Errorf("unlimited nodes: want nil got %v", err)
	}
}

func TestCheckNodeLimit_ZeroClaimsIsUnlimited(t *testing.T) {
	// max_nodes=0 in claims → buildEntitlements converts to -1 → unlimited.
	mgr := newMgr(t, map[string]interface{}{
		"tier":      "enterprise",
		"data_api":  true,
		"max_nodes": 0,
	})
	if err := mgr.CheckNodeLimit(99999); err != nil {
		t.Errorf("max_nodes=0 must be unlimited: want nil got %v", err)
	}
}

// ─── CheckDataAPI ─────────────────────────────────────────────────────────────

func TestCheckDataAPI_AllowedOnPro(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "pro",
		"data_api": true,
	})
	if err := mgr.CheckDataAPI(); err != nil {
		t.Errorf("pro tier: CheckDataAPI want nil got %v", err)
	}
}

func TestCheckDataAPI_AllowedOnBusiness(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "business",
		"data_api": true,
	})
	if err := mgr.CheckDataAPI(); err != nil {
		t.Errorf("business tier: CheckDataAPI want nil got %v", err)
	}
}

func TestCheckDataAPI_AllowedOnEnterprise(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "enterprise",
		"data_api": true,
	})
	if err := mgr.CheckDataAPI(); err != nil {
		t.Errorf("enterprise tier: CheckDataAPI want nil got %v", err)
	}
}

// ─── CheckPrometheus ──────────────────────────────────────────────────────────

func TestCheckPrometheus_BlockedOnFree(t *testing.T) {
	mgr, _ := license.New("", "")
	if err := mgr.CheckPrometheus(); err == nil {
		t.Error("free tier: CheckPrometheus must return error")
	}
}

func TestCheckPrometheus_BlockedOnPro(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "pro",
		"data_api": true,
	})
	if err := mgr.CheckPrometheus(); err == nil {
		t.Error("pro tier: CheckPrometheus must return error (Business+ required)")
	}
}

func TestCheckPrometheus_AllowedOnBusiness(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "business",
		"data_api": true,
	})
	if err := mgr.CheckPrometheus(); err != nil {
		t.Errorf("business tier: CheckPrometheus want nil got %v", err)
	}
}

func TestCheckPrometheus_AllowedOnEnterprise(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":        "enterprise",
		"data_api":    true,
		"white_label": true,
	})
	if err := mgr.CheckPrometheus(); err != nil {
		t.Errorf("enterprise tier: CheckPrometheus want nil got %v", err)
	}
}

// ─── CheckProbes ──────────────────────────────────────────────────────────────

func TestCheckProbes_BlockedOnFree(t *testing.T) {
	mgr, _ := license.New("", "")
	if err := mgr.CheckProbes(); err == nil {
		t.Error("free tier: CheckProbes must return error")
	}
}

func TestCheckProbes_AllowedOnPro(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "pro",
		"data_api": true,
	})
	if err := mgr.CheckProbes(); err != nil {
		t.Errorf("pro tier: CheckProbes want nil got %v", err)
	}
}

func TestCheckProbes_AllowedOnBusiness(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "business",
		"data_api": true,
	})
	if err := mgr.CheckProbes(); err != nil {
		t.Errorf("business tier: CheckProbes want nil got %v", err)
	}
}

// ─── CheckAnomalies ───────────────────────────────────────────────────────────

func TestCheckAnomalies_AllowedOnEnterprise(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":        "enterprise",
		"data_api":    true,
		"white_label": true,
	})
	if err := mgr.CheckAnomalies(); err != nil {
		t.Errorf("enterprise tier: CheckAnomalies want nil got %v", err)
	}
}

func TestCheckAnomalies_BlockedOnBusiness(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "business",
		"data_api": true,
	})
	if err := mgr.CheckAnomalies(); err == nil {
		t.Error("business tier: CheckAnomalies must return error (Enterprise-only)")
	}
}

// ─── CheckMultiTenant ─────────────────────────────────────────────────────────

func TestCheckMultiTenant_AllowedOnBusiness(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "business",
		"data_api": true,
	})
	if err := mgr.CheckMultiTenant(); err != nil {
		t.Errorf("business tier: CheckMultiTenant want nil got %v", err)
	}
}

func TestCheckMultiTenant_AllowedOnEnterprise(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":        "enterprise",
		"data_api":    true,
		"white_label": true,
	})
	if err := mgr.CheckMultiTenant(); err != nil {
		t.Errorf("enterprise tier: CheckMultiTenant want nil got %v", err)
	}
}

func TestCheckMultiTenant_BlockedOnPro(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "pro",
		"data_api": true,
	})
	if err := mgr.CheckMultiTenant(); err == nil {
		t.Error("pro tier: CheckMultiTenant must return error")
	}
}

// ─── CheckReports ─────────────────────────────────────────────────────────────

func TestCheckReports_AllowedOnBusiness(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "business",
		"data_api": true,
	})
	if err := mgr.CheckReports(); err != nil {
		t.Errorf("business tier: CheckReports want nil got %v", err)
	}
}

func TestCheckReports_AllowedOnEnterprise(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":        "enterprise",
		"data_api":    true,
		"white_label": true,
	})
	if err := mgr.CheckReports(); err != nil {
		t.Errorf("enterprise tier: CheckReports want nil got %v", err)
	}
}

func TestCheckReports_BlockedOnPro(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "pro",
		"data_api": true,
	})
	if err := mgr.CheckReports(); err == nil {
		t.Error("pro tier: CheckReports must return error (Business+ required)")
	}
}

// ─── CheckBeaconIngest ────────────────────────────────────────────────────────

func TestCheckBeaconIngest_AllowedOnPro(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "pro",
		"data_api": true,
	})
	if err := mgr.CheckBeaconIngest(); err != nil {
		t.Errorf("pro tier: CheckBeaconIngest want nil got %v", err)
	}
}

func TestCheckBeaconIngest_AllowedOnBusiness(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "business",
		"data_api": true,
	})
	if err := mgr.CheckBeaconIngest(); err != nil {
		t.Errorf("business tier: CheckBeaconIngest want nil got %v", err)
	}
}

// ─── CheckRetention ───────────────────────────────────────────────────────────

func TestCheckRetention_Unlimited(t *testing.T) {
	// retention_days omitted → nil → buildEntitlements sets -1 → unlimited.
	mgr := newMgr(t, map[string]interface{}{
		"tier":     "enterprise",
		"data_api": true,
	})
	got := mgr.CheckRetention(3650)
	if got != 3650 {
		t.Errorf("unlimited retention: want 3650 got %d", got)
	}
}

func TestCheckRetention_WithinLimit_ReturnsRequested(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":           "pro",
		"data_api":       true,
		"retention_days": 90,
	})
	got := mgr.CheckRetention(45)
	if got != 45 {
		t.Errorf("within-limit retention: want 45 got %d", got)
	}
}

func TestCheckRetention_OverLimit_CapsAtLimit(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":           "pro",
		"data_api":       true,
		"retention_days": 90,
	})
	got := mgr.CheckRetention(180)
	if got != 90 {
		t.Errorf("over-limit retention: want 90 (capped) got %d", got)
	}
}

func TestCheckRetention_ZeroRequest_CapsToLimit(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":           "pro",
		"data_api":       true,
		"retention_days": 90,
	})
	// requestedDays=0 → treated as over limit → returns tier limit.
	got := mgr.CheckRetention(0)
	if got != 90 {
		t.Errorf("zero-request retention: want 90 got %d", got)
	}
}
