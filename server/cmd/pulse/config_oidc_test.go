// config_oidc_test.go — TDD tests for PULSE_OIDC_* env var parsing (S11 WO-C).
// Package main (same package as config.go) to call loadEnvConfig directly.
//
// RED pass: these tests FAIL before the OIDC fields / env-parsing are added to
// config.go because loadEnvConfig returns an EnvConfig that lacks the OIDC fields.
// GREEN pass: all assertions pass after WO-C wiring is applied.
package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadEnvConfig_OIDC_Disabled_WhenNoIssuer(t *testing.T) {
	orig := os.Getenv("PULSE_OIDC_ISSUER")
	os.Setenv("PULSE_OIDC_ISSUER", "")
	defer os.Setenv("PULSE_OIDC_ISSUER", orig)
	cfg, err := loadEnvConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OIDCIssuer != "" {
		t.Errorf("expected OIDCIssuer empty, got %q", cfg.OIDCIssuer)
	}
}

func TestLoadEnvConfig_OIDC_Error_MissingClientID(t *testing.T) {
	os.Setenv("PULSE_OIDC_ISSUER", "https://provider.example.com")
	os.Setenv("PULSE_OIDC_CLIENT_ID", "")
	defer func() {
		os.Unsetenv("PULSE_OIDC_ISSUER")
		os.Unsetenv("PULSE_OIDC_CLIENT_ID")
	}()
	_, err := loadEnvConfig()
	if err == nil || !strings.Contains(err.Error(), "PULSE_OIDC_CLIENT_ID") {
		t.Errorf("expected PULSE_OIDC_CLIENT_ID error, got: %v", err)
	}
}

func TestLoadEnvConfig_OIDC_Error_MissingSecret(t *testing.T) {
	os.Setenv("PULSE_OIDC_ISSUER", "https://provider.example.com")
	os.Setenv("PULSE_OIDC_CLIENT_ID", "client-id")
	os.Setenv("PULSE_OIDC_CLIENT_SECRET", "")
	defer func() {
		os.Unsetenv("PULSE_OIDC_ISSUER")
		os.Unsetenv("PULSE_OIDC_CLIENT_ID")
		os.Unsetenv("PULSE_OIDC_CLIENT_SECRET")
	}()
	_, err := loadEnvConfig()
	if err == nil || !strings.Contains(err.Error(), "PULSE_OIDC_CLIENT_SECRET") {
		t.Errorf("expected PULSE_OIDC_CLIENT_SECRET error, got: %v", err)
	}
}

func TestLoadEnvConfig_OIDC_SessionTTL_Custom(t *testing.T) {
	os.Setenv("PULSE_OIDC_SESSION_TTL", "48h")
	defer os.Unsetenv("PULSE_OIDC_SESSION_TTL")
	cfg, err := loadEnvConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OIDCSessionTTL != 48*time.Hour {
		t.Errorf("expected 48h, got %v", cfg.OIDCSessionTTL)
	}
}

func TestLoadEnvConfig_OIDC_SessionTTL_Default(t *testing.T) {
	os.Unsetenv("PULSE_OIDC_SESSION_TTL")
	cfg, err := loadEnvConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OIDCSessionTTL != 24*time.Hour {
		t.Errorf("expected default 24h, got %v", cfg.OIDCSessionTTL)
	}
}

func TestLoadEnvConfig_OIDC_DefaultRole_Empty(t *testing.T) {
	// ORCH ruling: default is EMPTY = fail-closed.
	os.Unsetenv("PULSE_OIDC_DEFAULT_ROLE")
	cfg, err := loadEnvConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OIDCDefaultRole != "" {
		t.Errorf("expected OIDCDefaultRole empty (fail-closed), got %q", cfg.OIDCDefaultRole)
	}
}

func TestLoadEnvConfig_OIDC_GroupClaim_Default(t *testing.T) {
	os.Unsetenv("PULSE_OIDC_GROUP_CLAIM")
	cfg, err := loadEnvConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OIDCGroupClaim != "groups" {
		t.Errorf("expected OIDCGroupClaim=groups, got %q", cfg.OIDCGroupClaim)
	}
}

func TestLoadEnvConfig_OIDC_Secret_FileSuffix(t *testing.T) {
	f, err := os.CreateTemp("", "pulse-oidc-secret-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("test-client-secret")
	f.Close()
	os.Setenv("PULSE_OIDC_CLIENT_SECRET_FILE", f.Name())
	os.Unsetenv("PULSE_OIDC_CLIENT_SECRET")
	defer func() {
		os.Unsetenv("PULSE_OIDC_CLIENT_SECRET_FILE")
	}()
	cfg, err := loadEnvConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OIDCClientSecret != "test-client-secret" {
		t.Errorf("expected secret from file, got %q", cfg.OIDCClientSecret)
	}
}

func TestLoadEnvConfig_OIDC_Error_MissingRedirectURL(t *testing.T) {
	os.Setenv("PULSE_OIDC_ISSUER", "https://provider.example.com")
	os.Setenv("PULSE_OIDC_CLIENT_ID", "client-id")
	os.Setenv("PULSE_OIDC_CLIENT_SECRET", "secret")
	os.Unsetenv("PULSE_OIDC_REDIRECT_URL")
	defer func() {
		os.Unsetenv("PULSE_OIDC_ISSUER")
		os.Unsetenv("PULSE_OIDC_CLIENT_ID")
		os.Unsetenv("PULSE_OIDC_CLIENT_SECRET")
	}()
	_, err := loadEnvConfig()
	if err == nil || !strings.Contains(err.Error(), "PULSE_OIDC_REDIRECT_URL") {
		t.Errorf("expected PULSE_OIDC_REDIRECT_URL error, got: %v", err)
	}
}

func TestLoadEnvConfig_OIDC_SessionTTL_InvalidDuration(t *testing.T) {
	os.Setenv("PULSE_OIDC_SESSION_TTL", "not-a-duration")
	defer os.Unsetenv("PULSE_OIDC_SESSION_TTL")
	_, err := loadEnvConfig()
	if err == nil || !strings.Contains(err.Error(), "PULSE_OIDC_SESSION_TTL") {
		t.Errorf("expected PULSE_OIDC_SESSION_TTL parse error, got: %v", err)
	}
}
