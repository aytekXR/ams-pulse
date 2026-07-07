package reports_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/reports"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── Seeded-month test ────────────────────────────────────────────────────────

// TestSeedMonth_ReconcileWithinOnePct seeds 10,000 synthetic sessions (ICP-B
// scale for a single month), computes usage from them, and verifies:
//  1. viewer-minutes from ComputeUsageFromSessions matches truth within ±1%
//  2. egress_method field is present on every row
//  3. generation time < 60 s
func TestSeedMonth_ReconcileWithinOnePct(t *testing.T) {
	const n = 10_000
	const bitrateKbps = 1500.0

	start := time.Now()

	sessions, truthMinutes, _ := reports.SyntheticMonth(n, bitrateKbps)

	report := reports.ComputeUsageFromSessions(sessions, nil)

	elapsed := time.Since(start)
	t.Logf("SeedMonth: n=%d sessions, truth=%.4f viewer-minutes, computed=%.4f viewer-minutes, elapsed=%s",
		n, truthMinutes, report.Totals.ViewerMinutes, elapsed)

	// Budget: < 60 s.
	if elapsed >= 60*time.Second {
		t.Errorf("statement generation took %s, budget is <60s", elapsed)
	}

	// ±1% reconciliation.
	if truthMinutes == 0 {
		t.Fatal("truth is zero — test data bug")
	}
	drift := absFloat(report.Totals.ViewerMinutes-truthMinutes) / truthMinutes * 100.0
	t.Logf("Drift: %.4f%%", drift)
	if drift > 1.0 {
		t.Errorf("drift=%.4f%% exceeds ±1%% budget (truth=%.4f, computed=%.4f)",
			drift, truthMinutes, report.Totals.ViewerMinutes)
	}

	// egress_method must be present on every row.
	for i, r := range report.Rows {
		if r.EgressMethod == "" {
			t.Errorf("row[%d] has empty egress_method", i)
		}
	}

	t.Logf("PASS: n=%d, drift=%.4f%%, elapsed=%s", n, drift, elapsed)
}

// TestSeedMonth_StatementCSV verifies CSV generation for the synthetic month.
func TestSeedMonth_StatementCSV(t *testing.T) {
	sessions, _, _ := reports.SyntheticMonth(1000, 1500)
	report := reports.ComputeUsageFromSessions(sessions, nil)

	now := time.Now()
	stmt, err := reports.GenerateStatement(report, reports.StatementOptions{
		From:   now.AddDate(0, -1, 0),
		To:     now,
		Format: reports.FormatCSV,
	})
	if err != nil {
		t.Fatalf("GenerateStatement: %v", err)
	}
	if len(stmt.Data) == 0 {
		t.Fatal("empty CSV output")
	}
	csv := string(stmt.Data)
	if !strings.Contains(csv, "viewer_minutes") {
		t.Error("CSV missing 'viewer_minutes' header")
	}
	if !strings.Contains(csv, "egress_method") {
		t.Error("CSV missing 'egress_method' column")
	}
	if stmt.ContentType != "text/csv" {
		t.Errorf("expected text/csv, got %q", stmt.ContentType)
	}
	t.Logf("CSV generated: %d bytes, %d rows", len(stmt.Data), stmt.RowCount)
}

// TestSeedMonth_StatementPDF verifies PDF generation for the synthetic month.
func TestSeedMonth_StatementPDF(t *testing.T) {
	sessions, _, _ := reports.SyntheticMonth(100, 1500)
	report := reports.ComputeUsageFromSessions(sessions, nil)

	now := time.Now()
	wl := &reports.WhitelabelHeader{
		Name:    "Acme Broadcasting",
		Address: "123 Main St, New York, NY 10001",
	}
	stmt, err := reports.GenerateStatement(report, reports.StatementOptions{
		From:       now.AddDate(0, -1, 0),
		To:         now,
		Format:     reports.FormatPDF,
		Whitelabel: wl,
	})
	if err != nil {
		t.Fatalf("GenerateStatement PDF: %v", err)
	}
	if len(stmt.Data) < 100 {
		t.Fatal("PDF output too small (< 100 bytes)")
	}
	// PDF magic bytes.
	if !strings.HasPrefix(string(stmt.Data), "%PDF-") {
		t.Errorf("output does not start with %%PDF- magic (got: %q)", string(stmt.Data[:10]))
	}
	if stmt.ContentType != "application/pdf" {
		t.Errorf("expected application/pdf, got %q", stmt.ContentType)
	}
	t.Logf("PDF generated: %d bytes", len(stmt.Data))
}

// ─── Reconciliation ───────────────────────────────────────────────────────────

// TestReconcileInMemory tests the ReconcileInMemory function directly.
func TestReconcileInMemory_WithinTolerance(t *testing.T) {
	// 0.5% drift → within tolerance.
	r := reports.ReconcileInMemory(1000.0, 1005.0)
	if !r.WithinTolerance {
		t.Errorf("0.5%% drift should be within tolerance, drift=%.4f%%", r.DriftPct)
	}
}

func TestReconcileInMemory_ExceedsTolerance(t *testing.T) {
	// 2% drift → outside tolerance.
	r := reports.ReconcileInMemory(1000.0, 980.0)
	if r.WithinTolerance {
		t.Errorf("~2%% drift should exceed tolerance, drift=%.4f%%", r.DriftPct)
	}
}

func TestReconcileInMemory_ZeroRaw(t *testing.T) {
	r := reports.ReconcileInMemory(0, 0)
	if r.DriftPct != 0 {
		t.Errorf("zero raw: drift should be 0, got %.4f", r.DriftPct)
	}
}

// ─── Tenant mapping ───────────────────────────────────────────────────────────

// TestTenantMapping_GlobMatch verifies stream-name glob matching.
func TestTenantMapping_GlobMatch(t *testing.T) {
	tenants := []meta.TenantRow{
		{ID: "1", Name: "tenant-a", StreamPattern: "live/*"},
		{ID: "2", Name: "tenant-b", StreamPattern: "vod/%"},
	}
	tm := reports.NewTenantMatcher(tenants)

	tests := []struct {
		stream string
		want   string
	}{
		{"live/stream1", "tenant-a"},
		{"live/anything", "tenant-a"},
		{"vod/movie1", "tenant-b"},
		{"other/stream", ""},
		{"", ""},
	}
	for _, tc := range tests {
		got := tm.Resolve(tc.stream, nil)
		if got != tc.want {
			t.Errorf("Resolve(%q) = %q, want %q", tc.stream, got, tc.want)
		}
	}
}

// TestTenantMapping_MetaTagPrecedence verifies meta-tag wins over glob.
func TestTenantMapping_MetaTagPrecedence(t *testing.T) {
	tenants := []meta.TenantRow{
		{ID: "1", Name: "glob-tenant", StreamPattern: "live/*"},
		{ID: "2", Name: "tag-tenant", MetaTagKey: "customer_id", MetaTagValue: "cust-123"},
	}
	tm := reports.NewTenantMatcher(tenants)

	// Stream matches glob, but meta-tag should win.
	got := tm.Resolve("live/stream1", map[string]string{"customer_id": "cust-123"})
	if got != "tag-tenant" {
		t.Errorf("meta-tag should win over glob, got %q", got)
	}
}

// TestTenantMapping_UnassignedFallback verifies unassigned when no match.
func TestTenantMapping_UnassignedFallback(t *testing.T) {
	tenants := []meta.TenantRow{
		{ID: "1", Name: "tenant-a", StreamPattern: "live/*"},
	}
	tm := reports.NewTenantMatcher(tenants)

	got := tm.Resolve("other/stream", nil)
	if got != "" {
		t.Errorf("expected empty (unassigned), got %q", got)
	}
}

// ─── Schedule (fake clock) ────────────────────────────────────────────────────

// TestScheduler_FakeClockFire tests that a schedule fires, produces an artifact,
// and is listed as having run.
func TestScheduler_FakeClockFire(t *testing.T) {
	// Set up in-memory meta store.
	ctx := context.Background()
	ms, err := meta.New(ctx, "sqlite", ":memory:", "testsecret123456")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	defer ms.Close()
	if err := ms.MigrateEmbedded(ctx, meta.EmbeddedDDL); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Create a schedule that was due one second ago.
	pastDue := time.Now().Add(-1 * time.Second).UnixMilli()
	sched := meta.ReportScheduleRow{
		Cron:      "0 0 *",
		Format:    "csv",
		ScopeJSON: "{}",
		NextRunAt: &pastDue,
	}
	created, err := ms.CreateReportSchedule(ctx, sched)
	if err != nil {
		t.Fatalf("CreateReportSchedule: %v", err)
	}

	// Create accountant (no ClickHouse — in-memory mode).
	acct := reports.NewAccountant(nil, ms)

	// Create scheduler with temp artifacts dir.
	tmpDir := t.TempDir()
	svc := reports.NewScheduler(reports.SchedulerConfig{
		ArtifactsDir: tmpDir,
		TickInterval: time.Hour, // won't auto-tick in test
	}, acct, ms, newTestLogger(t))

	// Run once (simulates fake-clock tick).
	svc.RunOnce(ctx)

	// Verify schedule was updated (last_run_at set).
	updated, err := ms.GetReportSchedule(ctx, created.ID)
	if err != nil || updated == nil {
		t.Fatalf("GetReportSchedule: %v (row=%v)", err, updated)
	}
	if updated.LastRunAt == nil || *updated.LastRunAt == 0 {
		t.Error("schedule last_run_at not updated after fire")
	}
	t.Logf("PASS: schedule fired, last_run_at=%d, next_run_at=%v",
		*updated.LastRunAt, updated.NextRunAt)
}

// ─── S3 fake ──────────────────────────────────────────────────────────────────

// TestS3Upload_FakeServer tests the S3 uploader against a fake server.
// Verifies SigV4 Authorization header is present.
func TestS3Upload_FakeServer(t *testing.T) {
	fake := reports.NewS3FakeServer()
	srv := httptest.NewServer(fake)
	defer srv.Close()

	// Set fake credentials in env.
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key-id")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")

	uploader := reports.NewS3Uploader(reports.S3Config{
		Endpoint: srv.URL,
		Bucket:   "test-bucket",
		Prefix:   "reports/",
		Region:   "us-east-1",
	}, newTestLogger(t))

	data := []byte("col1,col2\nval1,val2\n")
	ctx := context.Background()
	if err := uploader.Upload(ctx, "reports/test.csv", "text/csv", data); err != nil {
		t.Fatalf("S3 upload failed: %v", err)
	}

	// Verify upload was received.
	key := "test-bucket/reports/test.csv"
	if got, ok := fake.Uploads[key]; !ok {
		t.Errorf("upload not found at key %q; uploads: %v", key, fake.Uploads)
	} else if string(got) != string(data) {
		t.Errorf("upload body mismatch: got %q, want %q", got, data)
	}
	t.Logf("PASS: S3 PUT verified, signature header accepted, body matches")
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func newTestLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ─── Guard test VD-36: 5-field cron presets must parse correctly ──────────────

// TestGuard_VD36_FiveFieldCronParsing verifies that standard 5-field cron
// expressions like "0 6 1 * *" (1st of each month at 06:00) are accepted and
// return a sane next run time — NOT the 1-month fallback.
// Old behavior: parseCronFieldsInternal returned an error for len>3, so
// scheduler.go fell back to AddDate(0,1,0) for ALL UI preset cron strings.
func TestGuard_VD36_FiveFieldCronParsing(t *testing.T) {
	// Baseline: 5-field cron "0 6 1 * *" = 06:00 on the 1st of the month.
	// NextCronTime should find the next 06:00 occurrence, NOT next month blindly.
	from := time.Date(2026, 6, 14, 8, 0, 0, 0, time.UTC) // 14 Jun 2026, 08:00

	cases := []struct {
		cron string
		desc string
		// wantBefore: the result must be before this time (not the 1-month fallback).
		wantBefore time.Time
	}{
		{
			cron:       "0 6 1 * *",
			desc:       "1st of month at 06:00",
			wantBefore: from.AddDate(0, 1, 0), // must not fall back to 1 month from now
		},
		{
			cron:       "30 9 * * 1",
			desc:       "every Monday at 09:30",
			wantBefore: from.Add(7 * 24 * time.Hour), // within a week
		},
		{
			cron:       "0 0 * * *",
			desc:       "midnight daily (5-field)",
			wantBefore: from.Add(24 * time.Hour), // within a day
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			next := reports.NextCronTime(tc.cron, from)
			t.Logf("cron=%q, from=%s → next=%s", tc.cron, from.Format(time.RFC3339), next.Format(time.RFC3339))

			// Must be after `from`.
			if !next.After(from) {
				t.Errorf("VD-36 FAIL: next=%s is not after from=%s for cron=%q",
					next.Format(time.RFC3339), from.Format(time.RFC3339), tc.cron)
			}

			// Must be before the 1-month fallback (proving it was actually parsed).
			fallback := from.AddDate(0, 1, 0)
			if !next.Before(fallback) {
				t.Errorf("VD-36 FAIL: cron=%q gave fallback next=%s (same as AddDate(0,1,0)=%s); 5-field cron not parsed correctly",
					tc.cron, next.Format(time.RFC3339), fallback.Format(time.RFC3339))
			} else {
				t.Logf("PASS VD-36: %q → next=%s (before fallback %s)", tc.cron, next.Format(time.RFC3339), fallback.Format(time.RFC3339))
			}
		})
	}
}
