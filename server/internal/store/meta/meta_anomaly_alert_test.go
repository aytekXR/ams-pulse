package meta_test

// meta_anomaly_alert_test.go — TDD tests for S11 WO-B meta store changes.
// Tests AlertRuleRow with rule_type/sigma/min_samples fields.
// Written before meta.go was updated (RED phase).

import (
	"context"
	"os"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// openMetaTestStore creates an in-memory test store with both 0001 and 0002 migrations applied.
// applySchemaUpgrades (called by MigrateEmbedded) applies the 0002 columns idempotently.
func openMetaTestStore(t *testing.T) *meta.Store {
	t.Helper()
	ctx := context.Background()
	s, err := meta.New(ctx, "sqlite", ":memory:", "test-secret-key-32chars-long-xx!")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	data, err := os.ReadFile("../../../../contracts/db/meta/0001_init.sql")
	if err != nil {
		t.Skipf("meta DDL not found: %v", err)
	}
	if err := s.MigrateEmbedded(ctx, string(data)); err != nil {
		t.Fatalf("MigrateEmbedded 0001: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// ─── TestAlertRuleRow_RuleTypeRoundtrip ──────────────────────────────────────

func TestAlertRuleRow_RuleTypeRoundtrip(t *testing.T) {
	store := openMetaTestStore(t)
	ctx := context.Background()

	created, err := store.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name:               "anomaly-roundtrip",
		Metric:             "viewer_count",
		RuleType:           "anomaly",
		Sigma:              2.5,
		MinSamples:         5,
		Operator:           "gt",
		Threshold:          0,
		WindowS:            3600,
		Severity:           "warning",
		Enabled:            true,
		MaintenanceWindows: "[]",
		ChannelIDs:         "[]",
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	fetched, err := store.GetAlertRule(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetAlertRule: %v", err)
	}
	if fetched == nil {
		t.Fatal("GetAlertRule returned nil")
	}

	if fetched.RuleType != "anomaly" {
		t.Errorf("RuleType: got %q, want %q", fetched.RuleType, "anomaly")
	}
	if fetched.Sigma != 2.5 {
		t.Errorf("Sigma: got %g, want 2.5", fetched.Sigma)
	}
	if fetched.MinSamples != 5 {
		t.Errorf("MinSamples: got %d, want 5", fetched.MinSamples)
	}
}

// ─── TestAlertRuleRow_ThresholdDefaultRuleType ───────────────────────────────

// Inserting without explicit rule_type stores "threshold" (the column default).
func TestAlertRuleRow_ThresholdDefaultRuleType(t *testing.T) {
	store := openMetaTestStore(t)
	ctx := context.Background()

	// Insert with empty RuleType — should be normalized to "threshold" by CreateAlertRule.
	created, err := store.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name:               "legacy-rule",
		Metric:             "viewer_count",
		RuleType:           "", // empty → should become "threshold"
		Operator:           "gt",
		Threshold:          100,
		WindowS:            60,
		Severity:           "warning",
		Enabled:            true,
		MaintenanceWindows: "[]",
		ChannelIDs:         "[]",
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	fetched, err := store.GetAlertRule(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetAlertRule: %v", err)
	}
	if fetched == nil {
		t.Fatal("GetAlertRule returned nil")
	}

	if fetched.RuleType != "threshold" {
		t.Errorf("RuleType: got %q, want 'threshold' for empty/legacy rule", fetched.RuleType)
	}
}

// ─── TestListAlertRules_IncludesNewFields ────────────────────────────────────

func TestListAlertRules_IncludesNewFields(t *testing.T) {
	store := openMetaTestStore(t)
	ctx := context.Background()

	_, err := store.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name:               "list-test-anomaly",
		Metric:             "cpu_pct",
		RuleType:           "anomaly",
		Sigma:              3.0,
		MinSamples:         10,
		Operator:           "gt",
		Threshold:          0,
		WindowS:            3600,
		Severity:           "critical",
		Enabled:            true,
		MaintenanceWindows: "[]",
		ChannelIDs:         "[]",
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	rules, err := store.ListAlertRules(ctx, 0, "")
	if err != nil {
		t.Fatalf("ListAlertRules: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal("expected 1 rule, got 0")
	}

	r := rules[0]
	if r.RuleType != "anomaly" {
		t.Errorf("RuleType: got %q, want 'anomaly'", r.RuleType)
	}
	if r.Sigma != 3.0 {
		t.Errorf("Sigma: got %g, want 3.0", r.Sigma)
	}
	if r.MinSamples != 10 {
		t.Errorf("MinSamples: got %d, want 10", r.MinSamples)
	}
}

// ─── TestUpdateAlertRule_PreservesAnomalyFields ──────────────────────────────

func TestUpdateAlertRule_PreservesAnomalyFields(t *testing.T) {
	store := openMetaTestStore(t)
	ctx := context.Background()

	created, err := store.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name:               "update-test",
		Metric:             "mem_pct",
		RuleType:           "anomaly",
		Sigma:              4.0,
		MinSamples:         30,
		Operator:           "gt",
		Threshold:          0,
		WindowS:            3600,
		Severity:           "warning",
		Enabled:            true,
		MaintenanceWindows: "[]",
		ChannelIDs:         "[]",
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	created.Sigma = 6.0
	created.MinSamples = 15
	if err := store.UpdateAlertRule(ctx, created); err != nil {
		t.Fatalf("UpdateAlertRule: %v", err)
	}

	fetched, err := store.GetAlertRule(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetAlertRule: %v", err)
	}
	if fetched.Sigma != 6.0 {
		t.Errorf("Sigma after update: got %g, want 6.0", fetched.Sigma)
	}
	if fetched.MinSamples != 15 {
		t.Errorf("MinSamples after update: got %d, want 15", fetched.MinSamples)
	}
}

// ─── TestMigration0002_DoubleApplyIdempotent ─────────────────────────────────

// Applying 0002 twice (via applySchemaUpgrades which runs on each MigrateEmbedded)
// must not error. applySchemaUpgrades checks column existence before ALTER TABLE.
func TestMigration0002_DoubleApplyIdempotent(t *testing.T) {
	ctx := context.Background()
	s, err := meta.New(ctx, "sqlite", ":memory:", "test-secret-key-32chars-long-xx!")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	defer s.Close()

	data, err := os.ReadFile("../../../../contracts/db/meta/0001_init.sql")
	if err != nil {
		t.Skipf("meta DDL not found: %v", err)
	}
	ddl := string(data)

	// First apply — creates schema + adds 0002 columns via applySchemaUpgrades.
	if err := s.MigrateEmbedded(ctx, ddl); err != nil {
		t.Fatalf("first MigrateEmbedded: %v", err)
	}

	// Second apply — must be idempotent (applySchemaUpgrades must not re-run ALTER TABLE).
	if err := s.MigrateEmbedded(ctx, ddl); err != nil {
		t.Fatalf("second MigrateEmbedded (should be idempotent): %v", err)
	}

	// Verify the columns are accessible after double-apply.
	rule, err := s.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name:               "idempotent-test",
		Metric:             "viewer_count",
		RuleType:           "anomaly",
		Sigma:              2.0,
		MinSamples:         5,
		Operator:           "gt",
		Threshold:          0,
		WindowS:            3600,
		Severity:           "warning",
		Enabled:            true,
		MaintenanceWindows: "[]",
		ChannelIDs:         "[]",
	})
	if err != nil {
		t.Fatalf("CreateAlertRule after double-apply: %v", err)
	}
	if rule.RuleType != "anomaly" {
		t.Errorf("expected RuleType='anomaly', got %q", rule.RuleType)
	}
}
