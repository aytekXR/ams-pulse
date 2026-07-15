// Package reports_test — S37 (D-099) scheduler license-enforcement tests.
//
// The HTTP CRUD handlers gate report-schedule create/update by tier, but a
// schedule created while licensed keeps firing on the timer after a downgrade.
// runSchedule now re-checks the license on every fire:
//   - CheckReports() != nil  → skip the whole run (no artifact, not marked ran).
//   - CheckWhiteLabel() != nil → run, but drop the white-label header (plain output).
//
// These tests drive RunOnce with a fake LicenseChecker and assert on the on-disk
// artifact + the schedule's last_run_at.
package reports_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/reports"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// fakeSchedLic is a reports.LicenseChecker whose gate results are set per-test.
type fakeSchedLic struct {
	reportsErr    error
	whitelabelErr error
}

func (f fakeSchedLic) CheckReports() error    { return f.reportsErr }
func (f fakeSchedLic) CheckWhiteLabel() error { return f.whitelabelErr }

// newSchedTestStore spins up an in-memory meta store with the embedded DDL.
func newSchedTestStore(t *testing.T) *meta.Store {
	t.Helper()
	ctx := context.Background()
	ms, err := meta.New(ctx, "sqlite", ":memory:", "testsecret123456")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	t.Cleanup(func() { ms.Close() })
	if err := ms.MigrateEmbedded(ctx, meta.EmbeddedDDL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return ms
}

// createDueSchedule inserts a schedule that was due one second ago.
func createDueSchedule(t *testing.T, ms *meta.Store, wlHeader string) meta.ReportScheduleRow {
	t.Helper()
	pastDue := time.Now().Add(-1 * time.Second).UnixMilli()
	sched := meta.ReportScheduleRow{
		Cron:      "0 0 *",
		Format:    "csv",
		ScopeJSON: "{}",
		NextRunAt: &pastDue,
	}
	if wlHeader != "" {
		sched.WhitelabelHeader = sql.NullString{String: wlHeader, Valid: true}
	}
	created, err := ms.CreateReportSchedule(context.Background(), sched)
	if err != nil {
		t.Fatalf("CreateReportSchedule: %v", err)
	}
	return created
}

// TestScheduler_DowngradedReports_SkipsRun proves the timer-path CheckReports gate:
// a schedule whose tier no longer licenses reports must NOT fire — no artifact is
// written and last_run_at stays nil.
//
// Mutation proof: deleting the CheckReports gate in runSchedule lets the schedule
// fire → last_run_at is set and a CSV lands in the dir → this test fails.
func TestScheduler_DowngradedReports_SkipsRun(t *testing.T) {
	ctx := context.Background()
	ms := newSchedTestStore(t)
	created := createDueSchedule(t, ms, "")

	acct := reports.NewAccountant(nil, ms)
	tmpDir := t.TempDir()
	svc := reports.NewScheduler(reports.SchedulerConfig{
		ArtifactsDir: tmpDir,
		TickInterval: time.Hour,
	}, acct, ms, newTestLogger(t))
	svc.SetLicense(fakeSchedLic{reportsErr: errors.New("reports require Business tier")})

	svc.RunOnce(ctx)

	updated, err := ms.GetReportSchedule(ctx, created.ID)
	if err != nil || updated == nil {
		t.Fatalf("GetReportSchedule: %v (row=%v)", err, updated)
	}
	if updated.LastRunAt != nil && *updated.LastRunAt != 0 {
		t.Errorf("downgraded schedule fired: last_run_at=%v, want nil", updated.LastRunAt)
	}
	entries, _ := os.ReadDir(tmpDir)
	if len(entries) != 0 {
		t.Errorf("downgraded schedule wrote %d artifact(s): %v; want none", len(entries), entries)
	}
}

// TestScheduler_DowngradedWhiteLabel_DropsHeader proves the timer-path
// CheckWhiteLabel gate: when reports are still licensed but white-label is not,
// the schedule fires but the generated CSV must NOT carry the white-label header.
//
// Mutation proof: removing the CheckWhiteLabel guard in runSchedule keeps the
// "# ACME Corp" header line in the CSV → this test fails.
func TestScheduler_DowngradedWhiteLabel_DropsHeader(t *testing.T) {
	ctx := context.Background()
	ms := newSchedTestStore(t)
	createDueSchedule(t, ms, `{"name":"ACME Corp"}`)

	acct := reports.NewAccountant(nil, ms)
	tmpDir := t.TempDir()
	svc := reports.NewScheduler(reports.SchedulerConfig{
		ArtifactsDir: tmpDir,
		TickInterval: time.Hour,
	}, acct, ms, newTestLogger(t))
	svc.SetLicense(fakeSchedLic{whitelabelErr: errors.New("white-label requires Enterprise")})

	svc.RunOnce(ctx)

	body := readSoleArtifact(t, tmpDir)
	if strings.Contains(body, "ACME Corp") {
		t.Errorf("white-label header leaked despite downgrade:\n%s", body)
	}
}

// TestScheduler_WhiteLabelLicensed_KeepsHeader is the positive control for the
// test above: with white-label licensed, the same schedule DOES carry the header.
// Without it, the "drops header" test could pass for the wrong reason (e.g. CSV
// never emits the header at all).
func TestScheduler_WhiteLabelLicensed_KeepsHeader(t *testing.T) {
	ctx := context.Background()
	ms := newSchedTestStore(t)
	createDueSchedule(t, ms, `{"name":"ACME Corp"}`)

	acct := reports.NewAccountant(nil, ms)
	tmpDir := t.TempDir()
	svc := reports.NewScheduler(reports.SchedulerConfig{
		ArtifactsDir: tmpDir,
		TickInterval: time.Hour,
	}, acct, ms, newTestLogger(t))
	svc.SetLicense(fakeSchedLic{}) // both gates pass

	svc.RunOnce(ctx)

	body := readSoleArtifact(t, tmpDir)
	if !strings.Contains(body, "ACME Corp") {
		t.Errorf("white-label header missing when licensed:\n%s", body)
	}
}

// readSoleArtifact reads the single file written to dir, failing if the count != 1.
func readSoleArtifact(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read artifacts dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 artifact, got %d: %v", len(entries), entries)
	}
	b, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	return string(b)
}
