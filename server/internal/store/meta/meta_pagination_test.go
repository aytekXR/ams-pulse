package meta_test

// meta_pagination_test.go — TDD RED tests for BUG-006 / BUG-007 keyset
// pagination (S22 / D-084). These tests call the NEW method signatures
// (limit int, cursor string) which do not yet exist — they compile-fail,
// establishing RED before implementation.
//
// Each test:
//  1. Creates an in-memory store + applies DDL migrations.
//  2. Inserts 3 rows.
//  3. Calls ListXxx(ctx, 2, "") — expects exactly 2 rows.
//  4. Derives cursor from the last row of page 1.
//  5. Calls ListXxx(ctx, 2, cursor) — expects exactly 1 row (third row).

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

func TestListAlertRulesPagination(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := s.CreateAlertRule(ctx, meta.AlertRuleRow{
			Name:      fmt.Sprintf("pg-rule-%d", i),
			Metric:    "cpu",
			Operator:  "gt",
			Threshold: 90,
			WindowS:   60,
			Severity:  "warning",
			Enabled:   true,
		})
		if err != nil {
			t.Fatalf("CreateAlertRule %d: %v", i, err)
		}
	}

	page1, err := s.ListAlertRules(ctx, 2, "")
	if err != nil {
		t.Fatalf("ListAlertRules page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1: want 2 rows, got %d", len(page1))
	}
	cursor := fmt.Sprintf("%d:%s", page1[1].CreatedAt, page1[1].ID)

	page2, err := s.ListAlertRules(ctx, 2, cursor)
	if err != nil {
		t.Fatalf("ListAlertRules page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2: want 1 row, got %d", len(page2))
	}
}

func TestListAlertChannelsPagination(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := s.CreateAlertChannel(ctx, meta.AlertChannelRow{
			Type:      "webhook",
			Name:      fmt.Sprintf("pg-chan-%d", i),
			ConfigEnc: "{}",
		})
		if err != nil {
			t.Fatalf("CreateAlertChannel %d: %v", i, err)
		}
	}

	page1, err := s.ListAlertChannels(ctx, 2, "")
	if err != nil {
		t.Fatalf("ListAlertChannels page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1: want 2 rows, got %d", len(page1))
	}
	cursor := fmt.Sprintf("%d:%s", page1[1].CreatedAt, page1[1].ID)

	page2, err := s.ListAlertChannels(ctx, 2, cursor)
	if err != nil {
		t.Fatalf("ListAlertChannels page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2: want 1 row, got %d", len(page2))
	}
}

func TestListAMSSourcesPagination(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := s.CreateAMSSource(ctx, meta.AMSSourceRow{
			Name:       fmt.Sprintf("pg-src-%d", i),
			SourceType: "rest",
			RestURL:    sql.NullString{String: fmt.Sprintf("http://10.0.0.%d:5080", i+1), Valid: true},
			RestUser:   sql.NullString{String: "admin", Valid: true},
			Enabled:    true,
		})
		if err != nil {
			t.Fatalf("CreateAMSSource %d: %v", i, err)
		}
	}

	page1, err := s.ListAMSSources(ctx, 2, "")
	if err != nil {
		t.Fatalf("ListAMSSources page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1: want 2 rows, got %d", len(page1))
	}
	cursor := fmt.Sprintf("%d:%s", page1[1].CreatedAt, page1[1].ID)

	page2, err := s.ListAMSSources(ctx, 2, cursor)
	if err != nil {
		t.Fatalf("ListAMSSources page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2: want 1 row, got %d", len(page2))
	}
}

func TestListTokensPagination(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		err := s.CreateToken(ctx, meta.APIToken{
			Kind:      "api",
			Name:      fmt.Sprintf("pg-tok-%d", i),
			TokenHash: fmt.Sprintf("sha256:pg-tok-hash-%d", i),
			Scopes:    []string{"read"},
		})
		if err != nil {
			t.Fatalf("CreateToken %d: %v", i, err)
		}
	}

	// ListTokens is DESC — first page returns the 2 most recently created.
	page1, err := s.ListTokens(ctx, "", 2, "")
	if err != nil {
		t.Fatalf("ListTokens page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1: want 2 rows, got %d", len(page1))
	}
	cursor := fmt.Sprintf("%d:%s", page1[1].CreatedAt, page1[1].ID)

	page2, err := s.ListTokens(ctx, "", 2, cursor)
	if err != nil {
		t.Fatalf("ListTokens page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2: want 1 row, got %d", len(page2))
	}
}

func TestListUsersPagination(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := s.CreateUser(ctx, meta.User{
			Username: fmt.Sprintf("pg-user-%d", i),
			PwHash:   "bcrypt:placeholder",
			Role:     "viewer",
		}); err != nil {
			t.Fatalf("CreateUser %d: %v", i, err)
		}
	}

	page1, err := s.ListUsers(ctx, 2, "")
	if err != nil {
		t.Fatalf("ListUsers page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1: want 2 rows, got %d", len(page1))
	}
	cursor := fmt.Sprintf("%d:%s", page1[1].CreatedAt, page1[1].ID)

	page2, err := s.ListUsers(ctx, 2, cursor)
	if err != nil {
		t.Fatalf("ListUsers page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2: want 1 row, got %d", len(page2))
	}
}

func TestListTenantsPagination(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := s.CreateTenant(ctx, meta.TenantRow{
			Name:          fmt.Sprintf("pg-tenant-%d", i),
			StreamPattern: fmt.Sprintf("live/%d/*", i),
		})
		if err != nil {
			t.Fatalf("CreateTenant %d: %v", i, err)
		}
	}

	page1, err := s.ListTenants(ctx, 2, "")
	if err != nil {
		t.Fatalf("ListTenants page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1: want 2 rows, got %d", len(page1))
	}
	cursor := fmt.Sprintf("%d:%s", page1[1].CreatedAt, page1[1].ID)

	page2, err := s.ListTenants(ctx, 2, cursor)
	if err != nil {
		t.Fatalf("ListTenants page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2: want 1 row, got %d", len(page2))
	}
}

func TestListReportSchedulesPagination(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := s.CreateReportSchedule(ctx, meta.ReportScheduleRow{
			Cron:   "0 9 * * 1",
			Format: "csv",
		})
		if err != nil {
			t.Fatalf("CreateReportSchedule %d: %v", i, err)
		}
	}

	page1, err := s.ListReportSchedules(ctx, 2, "")
	if err != nil {
		t.Fatalf("ListReportSchedules page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1: want 2 rows, got %d", len(page1))
	}
	cursor := fmt.Sprintf("%d:%s", page1[1].CreatedAt, page1[1].ID)

	page2, err := s.ListReportSchedules(ctx, 2, cursor)
	if err != nil {
		t.Fatalf("ListReportSchedules page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2: want 1 row, got %d", len(page2))
	}
}

func TestListProbesPagination(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := s.CreateProbe(ctx, meta.ProbeRow{
			Name:      fmt.Sprintf("pg-probe-%d", i),
			URL:       fmt.Sprintf("http://example.com/%d", i),
			IntervalS: 60,
			TimeoutS:  10,
			Enabled:   true,
		})
		if err != nil {
			t.Fatalf("CreateProbe %d: %v", i, err)
		}
	}

	page1, err := s.ListProbes(ctx, 2, "")
	if err != nil {
		t.Fatalf("ListProbes page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1: want 2 rows, got %d", len(page1))
	}
	cursor := fmt.Sprintf("%d:%s", page1[1].CreatedAt, page1[1].ID)

	page2, err := s.ListProbes(ctx, 2, cursor)
	if err != nil {
		t.Fatalf("ListProbes page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2: want 1 row, got %d", len(page2))
	}
}

func TestListAlertHistoryCursor(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	// Insert a rule so history rows can reference it.
	rule, err := s.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name:      "hist-rule",
		Metric:    "cpu",
		Operator:  "gt",
		Threshold: 80,
		WindowS:   60,
		Severity:  "warning",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	// Insert 3 history rows with distinct TS values (1001, 1002, 1003 ms).
	for i := 0; i < 3; i++ {
		if err := s.CreateAlertHistory(ctx, meta.AlertHistoryRow{
			RuleID:    rule.ID,
			State:     "firing",
			Severity:  "warning",
			TS:        int64(1001 + i),
			Metric:    "cpu",
			Value:     95,
			Threshold: 80,
		}); err != nil {
			t.Fatalf("CreateAlertHistory %d: %v", i, err)
		}
	}

	// Page 1 — DESC order: TS=1003 first, TS=1002 second.
	page1, err := s.ListAlertHistory(ctx, rule.ID, "", 0, 0, 2, "")
	if err != nil {
		t.Fatalf("ListAlertHistory page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1: want 2 rows, got %d", len(page1))
	}
	// Cursor is last item on page 1 (second newest: TS=1002).
	cursor := fmt.Sprintf("%d:%s", page1[1].TS, page1[1].ID)

	// Page 2 — should return TS=1001 (oldest).
	page2, err := s.ListAlertHistory(ctx, rule.ID, "", 0, 0, 2, cursor)
	if err != nil {
		t.Fatalf("ListAlertHistory page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2: want 1 row, got %d", len(page2))
	}
	if page2[0].TS != 1001 {
		t.Errorf("page2 row: want TS=1001, got TS=%d", page2[0].TS)
	}
}
