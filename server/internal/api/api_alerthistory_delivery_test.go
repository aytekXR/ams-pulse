// Package api_test — delivery_failure alert-history round-trip guard.
//
// Verifies that a delivery_failure row inserted via the meta store is returned
// by GET /alerts/history?state=delivery_failure with the correct state value.
// This guards the contract CR that adds "delivery_failure" to the state enum.
package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// setupAlertHistoryServer is a self-contained setup that returns the meta Store
// so tests can insert history rows before querying the API.
func setupAlertHistoryServer(t *testing.T) (ts *httptest.Server, token string, store *meta.Store, cleanup func()) {
	t.Helper()
	licKey, licCleanup := makeTestBusinessLicense(t)

	ctx := context.Background()
	ddlPath := metaDDLPath(t)
	ddlBytes, err := os.ReadFile(ddlPath)
	if err != nil {
		licCleanup()
		t.Skipf("meta DDL not found: %v", err)
	}

	store, err = meta.New(ctx, "sqlite", ":memory:", "ah-test-secret")
	if err != nil {
		licCleanup()
		t.Fatalf("meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddlBytes)); err != nil {
		store.Close()
		licCleanup()
		t.Fatalf("MigrateEmbedded: %v", err)
	}

	token = "plt_ah_test_token"
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "ah-admin",
		TokenHash: hashToken(token),
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}); err != nil {
		store.Close()
		licCleanup()
		t.Fatalf("CreateToken: %v", err)
	}

	lic, err := license.New(licKey, "")
	if err != nil {
		store.Close()
		licCleanup()
		t.Fatalf("license.New: %v", err)
	}

	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)
	srv := api.New(api.Config{ListenAddr: ":0"}, store, live, qsvc, lic, nil)
	ts = httptest.NewServer(srv.Handler())

	cleanup = func() {
		ts.Close()
		store.Close()
		licCleanup()
	}
	return ts, token, store, cleanup
}

// TestAlertHistory_DeliveryFailure_RoundTrip verifies the full round-trip:
//  1. Insert a delivery_failure row via the meta store.
//  2. GET /alerts/history?state=delivery_failure returns HTTP 200 with 1 item.
//  3. The returned item has state="delivery_failure".
//
// This test is the contract guard for the pre-approved CR that adds
// "delivery_failure" to the AlertHistoryEntry.state enum.
func TestAlertHistory_DeliveryFailure_RoundTrip(t *testing.T) {
	ts, token, store, cleanup := setupAlertHistoryServer(t)
	defer cleanup()

	ctx := context.Background()

	// alert_history.rule_id is a FK → must create a rule first.
	rule, err := store.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name:               "df-test-rule",
		Metric:             "stream_offline",
		Operator:           "eq",
		Threshold:          1,
		WindowS:            5,
		ScopeJSON:          `{}`,
		Severity:           "critical",
		CooldownS:          300,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         `[]`,
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	// Insert a delivery_failure row directly (mimics what the evaluator will do).
	dfRow := meta.AlertHistoryRow{
		AlertID:   "df-alert-001",
		RuleID:    rule.ID,
		State:     "delivery_failure",
		Severity:  "critical",
		TS:        time.Now().UnixMilli(),
		Metric:    "stream_offline",
		Value:     1.0,
		Threshold: 1.0,
		ScopeJSON: `{"channel_id": "ch-slack-01", "error": "connection refused", "stream_id": "s1"}`,
	}
	if err := store.CreateAlertHistory(ctx, dfRow); err != nil {
		t.Fatalf("CreateAlertHistory (delivery_failure): %v", err)
	}

	// Also insert a firing row to confirm it is NOT included in the filtered response.
	firingRow := meta.AlertHistoryRow{
		AlertID:   "firing-alert-001",
		RuleID:    rule.ID,
		State:     "firing",
		Severity:  "critical",
		TS:        time.Now().UnixMilli() - 60000,
		Metric:    "stream_offline",
		Value:     1.0,
		Threshold: 1.0,
		ScopeJSON: `{}`,
	}
	if err := store.CreateAlertHistory(ctx, firingRow); err != nil {
		t.Fatalf("CreateAlertHistory (firing): %v", err)
	}

	// GET /alerts/history?state=delivery_failure
	req, err := http.NewRequest(http.MethodGet,
		ts.URL+"/api/v1/alerts/history?state=delivery_failure", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /alerts/history: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode response: %v — body: %s", err, body)
	}

	// Exactly 1 delivery_failure item (the firing row must be filtered out).
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 delivery_failure item, got %d (body: %s)", len(result.Items), body)
	}

	state, _ := result.Items[0]["state"].(string)
	if state != "delivery_failure" {
		t.Errorf("expected state=delivery_failure, got %q", state)
	}

	ruleID, _ := result.Items[0]["rule_id"].(string)
	if ruleID != rule.ID {
		t.Errorf("expected rule_id=%q, got %q", rule.ID, ruleID)
	}

	t.Logf("PASS: delivery_failure row round-trips: state=%q rule_id=%q (firing row excluded from filtered response)",
		state, ruleID)
}

// TestAlertHistory_NegativeLimitUsesDefault verifies that ?limit=-1 is treated
// as the default page size (50) and does NOT return all rows unboundedly.
//
// TDD RED: before the fix, limit=-1 is not caught by `if limit == 0`, so
// limit+1 = 0 is passed to ListAlertHistory → no SQL LIMIT → all 52 rows returned.
// TDD GREEN: after changing the guard to `<= 0`, limit is clamped to 50 →
// exactly 50 rows are returned and next_cursor is non-nil.
func TestAlertHistory_NegativeLimitUsesDefault(t *testing.T) {
	ts, token, store, cleanup := setupAlertHistoryServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create a rule for the FK constraint.
	rule, err := store.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name:               "neg-limit-test-rule",
		Metric:             "stream_offline",
		Operator:           "eq",
		Threshold:          1,
		WindowS:            5,
		ScopeJSON:          `{}`,
		Severity:           "critical",
		CooldownS:          300,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         `[]`,
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	// Insert 52 rows — more than the default limit of 50 — so an unbounded
	// query returns 52 items, proving the cap was not applied.
	for i := 0; i < 52; i++ {
		row := meta.AlertHistoryRow{
			AlertID:   fmt.Sprintf("neg-limit-alert-%03d", i),
			RuleID:    rule.ID,
			State:     "firing",
			Severity:  "critical",
			TS:        time.Now().UnixMilli() - int64(i)*1000,
			Metric:    "stream_offline",
			Value:     1.0,
			Threshold: 1.0,
			ScopeJSON: `{}`,
		}
		if err := store.CreateAlertHistory(ctx, row); err != nil {
			t.Fatalf("CreateAlertHistory[%d]: %v", i, err)
		}
	}

	// GET /alerts/history?limit=-1 must behave as the default (50 items).
	req, err := http.NewRequest(http.MethodGet,
		ts.URL+"/api/v1/alerts/history?limit=-1", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /alerts/history: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Items []map[string]any `json:"items"`
		Meta  struct {
			NextCursor *string `json:"next_cursor"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode response: %v — body: %s", err, body)
	}

	// Without the fix: all 52 rows returned (unbounded — limit+1=0 → no SQL LIMIT).
	// With the fix: exactly 50 rows returned (default limit applied).
	if len(result.Items) != 50 {
		t.Errorf("expected 50 items (default limit), got %d (body snippet: %.200s)", len(result.Items), body)
	}
	if result.Meta.NextCursor == nil {
		t.Error("expected non-nil next_cursor: 52 rows exist but only 50 should be returned")
	}
	t.Logf("PASS: limit=-1 returns %d items (default=50), next_cursor present", len(result.Items))
}
