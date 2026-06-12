package meta_test

import (
	"context"
	"os"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// readMetaDDL reads the meta DDL from the contracts directory.
func readMetaDDL(t *testing.T) string {
	t.Helper()
	// Path relative: server/internal/store/meta/ → contracts/db/meta/0001_init.sql
	data, err := os.ReadFile("../../../../contracts/db/meta/0001_init.sql")
	if err != nil {
		t.Skipf("meta DDL not found (expected at contracts/db/meta/0001_init.sql): %v", err)
	}
	return string(data)
}

func openStore(t *testing.T) *meta.Store {
	t.Helper()
	ctx := context.Background()
	s, err := meta.New(ctx, "sqlite", ":memory:", "test-secret-key-for-tests")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := s.MigrateEmbedded(ctx, readMetaDDL(t)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMetaStore_Users_RoundTrip(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	// Create user.
	u := meta.User{
		Username:  "alice",
		PwHash:    "sha256:deadbeef",
		Role:      "admin",
		CreatedAt: 1000,
		UpdatedAt: 1000,
	}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Fetch by username.
	got, err := s.GetUserByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if got == nil {
		t.Fatal("expected user, got nil")
	}
	if got.Username != "alice" || got.Role != "admin" {
		t.Errorf("user mismatch: got %+v", got)
	}
	if got.ID == "" {
		t.Error("expected non-empty ID")
	}

	// List.
	users, err := s.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 {
		t.Errorf("expected 1 user, got %d", len(users))
	}

	// Count.
	n, err := s.CountUsers(ctx)
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if n != 1 {
		t.Errorf("expected count=1, got %d", n)
	}

	// Update.
	if err := s.UpdateUser(ctx, got.ID, "alice_updated", "viewer"); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	// Delete.
	if err := s.DeleteUser(ctx, got.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	users2, _ := s.ListUsers(ctx)
	if len(users2) != 0 {
		t.Error("expected empty after delete")
	}
}

func TestMetaStore_AlertRules_RoundTrip(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	row := meta.AlertRuleRow{
		Name:               "test-viewer-count-rule",
		Metric:             "viewer_count",
		Operator:           "lt",
		Threshold:          10.0,
		WindowS:            60,
		ScopeJSON:          `{"app":"live"}`,
		Severity:           "warning",
		CooldownS:          300,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         "[]",
	}
	created, err := s.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if created.CreatedAt == 0 {
		t.Fatal("expected non-zero created_at")
	}

	// Get.
	got, err := s.GetAlertRule(ctx, created.ID)
	if err != nil || got == nil {
		t.Fatalf("GetAlertRule: %v (nil=%v)", err, got == nil)
	}
	if got.Metric != "viewer_count" || got.Operator != "lt" {
		t.Errorf("rule mismatch: got %+v", got)
	}

	// Update.
	got.Threshold = 20.0
	if err := s.UpdateAlertRule(ctx, *got); err != nil {
		t.Fatalf("UpdateAlertRule: %v", err)
	}
	updated, _ := s.GetAlertRule(ctx, created.ID)
	if updated.Threshold != 20.0 {
		t.Errorf("expected threshold=20, got %v", updated.Threshold)
	}

	// List.
	rules, err := s.ListAlertRules(ctx)
	if err != nil {
		t.Fatalf("ListAlertRules: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(rules))
	}

	// Delete.
	if err := s.DeleteAlertRule(ctx, created.ID); err != nil {
		t.Fatalf("DeleteAlertRule: %v", err)
	}
	rules2, _ := s.ListAlertRules(ctx)
	if len(rules2) != 0 {
		t.Error("expected empty after delete")
	}
}

func TestMetaStore_AlertRules_SurviveRestart(t *testing.T) {
	// Write rules to a temp file DB and reopen to verify persistence.
	tmpFile, err := os.CreateTemp("", "pulse_meta_*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())
	defer os.Remove(tmpFile.Name() + "-journal")

	ctx := context.Background()
	ddl := readMetaDDL(t)

	// First open: write.
	s1, err := meta.New(ctx, "sqlite", tmpFile.Name(), "test-key")
	if err != nil {
		t.Fatalf("open1: %v", err)
	}
	if err := s1.MigrateEmbedded(ctx, ddl); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	row := meta.AlertRuleRow{
		Name:      "test-stream-offline",
		Metric:    "stream_offline",
		Operator:  "eq",
		Threshold: 1,
		WindowS:   30,
		Severity:  "critical",
		CooldownS: 60,
		Enabled:   true,
	}
	created, err := s1.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}
	s1.Close()

	// Second open: read.
	s2, err := meta.New(ctx, "sqlite", tmpFile.Name(), "test-key")
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	defer s2.Close()
	if err := s2.MigrateEmbedded(ctx, ddl); err != nil {
		t.Fatalf("migrate2: %v", err)
	}

	rules, err := s2.ListAlertRules(ctx)
	if err != nil {
		t.Fatalf("ListAlertRules after restart: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule after restart, got %d", len(rules))
	}
	if rules[0].ID != created.ID {
		t.Errorf("rule ID mismatch: %s != %s", rules[0].ID, created.ID)
	}
	if rules[0].Metric != "stream_offline" {
		t.Errorf("rule metric mismatch: %s", rules[0].Metric)
	}
	t.Logf("PASS: alert rule survived restart (id=%s, metric=%s)", rules[0].ID, rules[0].Metric)
}

func TestMetaStore_Encryption_RoundTrip(t *testing.T) {
	s := openStore(t)

	plaintext := "my-secret-slack-webhook-url"
	ciphertext, err := s.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if ciphertext == "" || ciphertext == plaintext {
		t.Fatal("expected non-empty different ciphertext")
	}

	decrypted, err := s.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("decrypt mismatch: got %q, want %q", decrypted, plaintext)
	}
	t.Logf("PASS: AES-256-GCM encrypt/decrypt round-trip OK")
}

func TestMetaStore_License_Bootstrap(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	// GetLicense on empty store should bootstrap free tier.
	lic, err := s.GetLicense(ctx)
	if err != nil {
		t.Fatalf("GetLicense: %v", err)
	}
	if lic.Tier != "free" {
		t.Errorf("expected free tier, got %s", lic.Tier)
	}
	if !lic.Valid {
		t.Error("expected valid=true for free tier")
	}
	t.Logf("PASS: license bootstrap → tier=%s valid=%v", lic.Tier, lic.Valid)
}

func TestMetaStore_Tokens_RoundTrip(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	tok := meta.APIToken{
		Kind:      "api",
		Name:      "test-admin",
		TokenHash: "sha256:abcdef1234",
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}
	if err := s.CreateToken(ctx, tok); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	// Fetch by hash.
	got, err := s.GetTokenByHash(ctx, "sha256:abcdef1234")
	if err != nil {
		t.Fatalf("GetTokenByHash: %v", err)
	}
	if got == nil {
		t.Fatal("expected token, got nil")
	}
	if got.Kind != "api" || got.Name != "test-admin" {
		t.Errorf("token mismatch: %+v", got)
	}

	// Count.
	n, err := s.CountTokens(ctx)
	if err != nil {
		t.Fatalf("CountTokens: %v", err)
	}
	if n != 1 {
		t.Errorf("expected count=1, got %d", n)
	}

	// Delete.
	if err := s.DeleteToken(ctx, got.ID); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}
	n2, _ := s.CountTokens(ctx)
	if n2 != 0 {
		t.Errorf("expected count=0 after delete, got %d", n2)
	}
	t.Logf("PASS: token round-trip OK")
}
