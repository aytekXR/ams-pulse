// Package reports_test — unit tests using a mock ClickHouse driver.Conn.
//
// Covers: ComputeUsage (day+hour paths), fetchConcurrencyPeaks (via ComputeUsage),
// Reconcile, AggregateByTenant, SetTenantMatcher/resolveTenantMatcher, Scheduler
// lifecycle (Start/Stop/SetAlertStore/writeFailureAlert/runDue branches),
// ParseWhitelabelHeader, and cheap-win branches (escapePDFString, resolveEnv,
// likeMatch).
//
// TDD: each block was first written with a deliberately wrong expectation (RED),
// confirmed to fail, then corrected (GREEN). Red evidence recorded in work-order
// completion report.
package reports_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/column"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/pulse-analytics/pulse/server/internal/reports"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── Mock driver.Rows ─────────────────────────────────────────────────────────

type acctFakeRows struct {
	data    [][]any
	idx     int
	rowsErr error
	scanErr error
}

func acctNewFakeRows(rows ...[]any) *acctFakeRows {
	return &acctFakeRows{data: rows, idx: -1}
}

func acctNewErrRows(err error) *acctFakeRows {
	return &acctFakeRows{rowsErr: err, idx: -1}
}

func (r *acctFakeRows) Next() bool {
	r.idx++
	return r.idx < len(r.data)
}

func (r *acctFakeRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	if r.idx < 0 || r.idx >= len(r.data) {
		return fmt.Errorf("acctFakeRows: no current row at index %d", r.idx)
	}
	return acctScanDests(dest, r.data[r.idx])
}

func (r *acctFakeRows) Err() error                       { return r.rowsErr }
func (r *acctFakeRows) Close() error                     { return nil }
func (r *acctFakeRows) ScanStruct(_ any) error           { return nil }
func (r *acctFakeRows) ColumnTypes() []driver.ColumnType { return nil }
func (r *acctFakeRows) Totals(_ ...any) error            { return nil }
func (r *acctFakeRows) Columns() []string                { return nil }
func (r *acctFakeRows) HasData() bool                    { return len(r.data) > 0 }

// ─── Mock driver.Row ──────────────────────────────────────────────────────────

type acctFakeRow struct {
	data    []any
	scanErr error
}

func acctNewFakeRow(vals ...any) *acctFakeRow { return &acctFakeRow{data: vals} }
func acctNewErrRow(err error) *acctFakeRow    { return &acctFakeRow{scanErr: err} }

func (r *acctFakeRow) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	return acctScanDests(dest, r.data)
}

func (r *acctFakeRow) Err() error             { return r.scanErr }
func (r *acctFakeRow) ScanStruct(_ any) error { return nil }

// ─── scanDests: reflection-based column assignment ────────────────────────────

func acctScanDests(dest []any, src []any) error {
	if len(dest) != len(src) {
		return fmt.Errorf("acctScanDests: mismatch dest=%d src=%d", len(dest), len(src))
	}
	for i, d := range dest {
		dv := reflect.ValueOf(d)
		if dv.Kind() != reflect.Ptr {
			return fmt.Errorf("acctScanDests: dest[%d] not ptr (%T)", i, d)
		}
		sv := reflect.ValueOf(src[i])
		if !sv.IsValid() {
			continue
		}
		dv.Elem().Set(sv)
	}
	return nil
}

// ─── Mock clickhouse.Conn ─────────────────────────────────────────────────────

type acctFakeConn struct {
	queryQueue []acctQueryResp
	qi         int
	rowQueue   []acctRowResp
	ri         int
}

type acctQueryResp struct {
	rows driver.Rows
	err  error
}
type acctRowResp struct {
	row driver.Row
}

func (c *acctFakeConn) withQuery(rows driver.Rows, err error) *acctFakeConn {
	c.queryQueue = append(c.queryQueue, acctQueryResp{rows: rows, err: err})
	return c
}

func (c *acctFakeConn) withRow(row driver.Row) *acctFakeConn {
	c.rowQueue = append(c.rowQueue, acctRowResp{row: row})
	return c
}

func newAcctFakeConn() *acctFakeConn { return &acctFakeConn{} }

func (c *acctFakeConn) Query(_ context.Context, _ string, _ ...any) (driver.Rows, error) {
	if c.qi < len(c.queryQueue) {
		r := c.queryQueue[c.qi]
		c.qi++
		return r.rows, r.err
	}
	return acctNewFakeRows(), nil // default: empty rows
}

func (c *acctFakeConn) QueryRow(_ context.Context, _ string, _ ...any) driver.Row {
	if c.ri < len(c.rowQueue) {
		r := c.rowQueue[c.ri]
		c.ri++
		return r.row
	}
	return acctNewErrRow(fmt.Errorf("acctFakeConn: no QueryRow queued"))
}

func (c *acctFakeConn) PrepareBatch(_ context.Context, _ string, _ ...driver.PrepareBatchOption) (driver.Batch, error) {
	return nil, nil
}
func (c *acctFakeConn) Contributors() []string                                    { return nil }
func (c *acctFakeConn) ServerVersion() (*driver.ServerVersion, error)             { return nil, nil }
func (c *acctFakeConn) Select(_ context.Context, _ any, _ string, _ ...any) error { return nil }
func (c *acctFakeConn) Exec(_ context.Context, _ string, _ ...any) error          { return nil }
func (c *acctFakeConn) AsyncInsert(_ context.Context, _ string, _ bool, _ ...any) error {
	return nil
}
func (c *acctFakeConn) Ping(_ context.Context) error { return nil }
func (c *acctFakeConn) Stats() driver.Stats          { return driver.Stats{} }
func (c *acctFakeConn) Close() error                 { return nil }

// fakeBatch satisfies driver.Batch (unused but needed for interface).
type acctFakeBatch struct{}

func (b *acctFakeBatch) Abort() error                    { return nil }
func (b *acctFakeBatch) Append(_ ...any) error           { return nil }
func (b *acctFakeBatch) AppendStruct(_ any) error        { return nil }
func (b *acctFakeBatch) Column(_ int) driver.BatchColumn { return nil }
func (b *acctFakeBatch) Flush() error                    { return nil }
func (b *acctFakeBatch) Send() error                     { return nil }
func (b *acctFakeBatch) IsSent() bool                    { return false }
func (b *acctFakeBatch) Rows() int                       { return 0 }
func (b *acctFakeBatch) Columns() []column.Interface     { return nil }
func (b *acctFakeBatch) Close() error                    { return nil }

// ─── fakeHistoryWriter ────────────────────────────────────────────────────────

type fakeHistoryWriter struct {
	mu    sync.Mutex
	calls []meta.AlertHistoryRow
	err   error
}

func (f *fakeHistoryWriter) CreateAlertHistory(_ context.Context, h meta.AlertHistoryRow) error {
	f.mu.Lock()
	f.calls = append(f.calls, h)
	f.mu.Unlock()
	return f.err
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func acctLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func acctMetaStore(t *testing.T) *meta.Store {
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

// ─── ComputeUsage — NilConn ──────────────────────────────────────────────────

func TestAcctConn_ComputeUsage_NilConn_Empty(t *testing.T) {
	acct := reports.NewAccountant(nil, nil)
	rep, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From: time.Now().AddDate(0, -1, 0),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rep == nil {
		t.Fatal("result is nil")
	}
	if len(rep.Rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rep.Rows))
	}
	if rep.EgressMethod != reports.EgressMethodBitrateXWatchTime {
		t.Errorf("EgressMethod: got %q, want %q", rep.EgressMethod, reports.EgressMethodBitrateXWatchTime)
	}
}

// ─── ComputeUsage — day mode happy path ──────────────────────────────────────

func TestAcctConn_ComputeUsage_DayMode_HappyPath(t *testing.T) {
	// Queue 1: fetchConcurrencyPeaks → 1 row: app=live, stream=s1, peak=5
	concRow := []any{"live", "s1", int64(5)}
	// Queue 2: main usage (day) → 1 row: app=live, stream_id=s1, viewer_minutes=100,
	//   egress_bytes=0, recording_bytes=0
	usageRow := []any{"live", "s1", float64(100.0), uint64(0), uint64(0)}

	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(concRow), nil).
		withQuery(acctNewFakeRows(usageRow), nil)

	acct := reports.NewAccountant(conn, nil)
	rep, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From: time.Now().AddDate(0, -1, 0),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rep.Rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rep.Rows))
	}
	r := rep.Rows[0]
	if r.App != "live" {
		t.Errorf("App: got %q, want live", r.App)
	}
	if r.ViewerMinutes != 100.0 {
		t.Errorf("ViewerMinutes: got %f, want 100.0", r.ViewerMinutes)
	}
	if r.PeakConcurrency != 5 {
		t.Errorf("PeakConcurrency: got %d, want 5", r.PeakConcurrency)
	}
	// EgressGB: kbpsToGBPerMinute(100, 1000) = 100 * 1000 * 60 * 1000 / 8 / 1e9 = 0.75
	const wantEgressGB = 0.75
	if r.EgressGB != wantEgressGB {
		t.Errorf("EgressGB: got %f, want %f", r.EgressGB, wantEgressGB)
	}
	if r.EgressMethod != reports.EgressMethodBitrateXWatchTime {
		t.Errorf("EgressMethod: got %q, want %q", r.EgressMethod, reports.EgressMethodBitrateXWatchTime)
	}
	// VD-41 (audit [10]): every row used the bitrate model → report-level = bitrate.
	if rep.EgressMethod != reports.EgressMethodBitrateXWatchTime {
		t.Errorf("report EgressMethod: got %q, want %q", rep.EgressMethod, reports.EgressMethodBitrateXWatchTime)
	}
}

// ─── ComputeUsage — egress_bytes > 0 → AMSRestStats method (VD-37) ───────────

func TestAcctConn_ComputeUsage_DayMode_EgressBytes(t *testing.T) {
	// egress_bytes = 1e9 bytes = 1 GB
	usageRow := []any{"live", "s2", float64(50.0), uint64(1_000_000_000), uint64(500_000_000)}

	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(), nil).        // concurrencyPeaks: empty
		withQuery(acctNewFakeRows(usageRow), nil) // main usage

	acct := reports.NewAccountant(conn, nil)
	rep, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From: time.Now().AddDate(0, -1, 0),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rep.Rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rep.Rows))
	}
	r := rep.Rows[0]
	// egress_bytes > 0 → AMSRestStatsByteCounter
	if r.EgressMethod != reports.EgressMethodAMSRestStatsByteCounter {
		t.Errorf("EgressMethod: got %q, want %q", r.EgressMethod, reports.EgressMethodAMSRestStatsByteCounter)
	}
	// 1e9 bytes / 1e9 = 1.0 GB
	if r.EgressGB != 1.0 {
		t.Errorf("EgressGB: got %f, want 1.0", r.EgressGB)
	}
	// recording_bytes = 5e8 / 1e9 = 0.5 GB
	if r.RecordingGB != 0.5 {
		t.Errorf("RecordingGB: got %f, want 0.5", r.RecordingGB)
	}
	// VD-41 (audit [10]): the only row used byte counters → report-level =
	// byte-counter. Before the fix this was hardcoded to bitrate_x_watch_time.
	if rep.EgressMethod != reports.EgressMethodAMSRestStatsByteCounter {
		t.Errorf("report EgressMethod: got %q, want %q", rep.EgressMethod, reports.EgressMethodAMSRestStatsByteCounter)
	}
}

// ─── ComputeUsage — mixed egress methods → report-level "mixed" (VD-41, audit [10]) ─

func TestAcctConn_ComputeUsage_DayMode_MixedEgressMethod(t *testing.T) {
	// Two streams in the daily path: s1 has no byte-counter data (egress_bytes=0 →
	// bitrate model), s2 has byte-counter data (egress_bytes>0 → byte-counter). The
	// aggregate Totals.EgressGB blends both, so the report-level disclosure must be
	// "mixed" — neither pure label describes the report honestly.
	bitrateRow := []any{"live", "s1", float64(100.0), uint64(0), uint64(0)}
	byteRow := []any{"live", "s2", float64(50.0), uint64(1_000_000_000), uint64(0)}

	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(), nil).                   // concurrencyPeaks: empty
		withQuery(acctNewFakeRows(bitrateRow, byteRow), nil) // main usage: 2 rows

	acct := reports.NewAccountant(conn, nil)
	rep, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From: time.Now().AddDate(0, -1, 0),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rep.Rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rep.Rows))
	}
	// Both methods contributed → "mixed". Before VD-41 this field was hardcoded to
	// bitrate_x_watch_time regardless of the rows, falsely disclosing pure bitrate.
	if rep.EgressMethod != reports.EgressMethodMixed {
		t.Errorf("report EgressMethod: got %q, want %q", rep.EgressMethod, reports.EgressMethodMixed)
	}
}

// ─── ComputeUsage — hour mode ─────────────────────────────────────────────────

func TestAcctConn_ComputeUsage_HourMode(t *testing.T) {
	// hour mode: 3 columns (app, stream_id, viewer_minutes)
	usageRow := []any{"live", "s3", float64(30.0)}

	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(), nil).        // concurrencyPeaks (empty)
		withQuery(acctNewFakeRows(usageRow), nil) // main usage hour

	acct := reports.NewAccountant(conn, nil)
	rep, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From:     time.Now().AddDate(0, 0, -1),
		To:       time.Now(),
		Interval: "hour",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rep.Rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rep.Rows))
	}
	r := rep.Rows[0]
	if r.ViewerMinutes != 30.0 {
		t.Errorf("ViewerMinutes: got %f, want 30.0", r.ViewerMinutes)
	}
	// hour mode always uses bitrate_x_watch_time
	if r.EgressMethod != reports.EgressMethodBitrateXWatchTime {
		t.Errorf("EgressMethod: got %q, want bitrate_x_watch_time", r.EgressMethod)
	}
	// VD-41 (audit [10]): hour path never takes the bytes branch → report-level = bitrate.
	if rep.EgressMethod != reports.EgressMethodBitrateXWatchTime {
		t.Errorf("report EgressMethod: got %q, want bitrate_x_watch_time", rep.EgressMethod)
	}
}

// ─── ComputeUsage — app+stream filter branches ───────────────────────────────

func TestAcctConn_ComputeUsage_AppStreamFilter(t *testing.T) {
	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(), nil).
		withQuery(acctNewFakeRows(), nil)

	acct := reports.NewAccountant(conn, nil)
	rep, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From:     time.Now().AddDate(0, -1, 0),
		To:       time.Now(),
		App:      "live",
		StreamID: "s1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rep.Rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rep.Rows))
	}
}

func TestAcctConn_ComputeUsage_HourMode_AppStreamFilter(t *testing.T) {
	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(), nil).
		withQuery(acctNewFakeRows(), nil)

	acct := reports.NewAccountant(conn, nil)
	rep, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From:     time.Now().AddDate(0, 0, -1),
		To:       time.Now(),
		Interval: "hour",
		App:      "live",
		StreamID: "s1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rep == nil {
		t.Fatal("result nil")
	}
}

// ─── ComputeUsage — tenant filter ────────────────────────────────────────────

func TestAcctConn_ComputeUsage_TenantFilter(t *testing.T) {
	// Two usage rows, but tenant filter should exclude one.
	usageRow1 := []any{"live", "tenant-a-stream", float64(10.0), uint64(0), uint64(0)}
	usageRow2 := []any{"live", "other-stream", float64(20.0), uint64(0), uint64(0)}

	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(), nil). // concurrencyPeaks empty
		withQuery(acctNewFakeRows(usageRow1, usageRow2), nil)

	tm := reports.NewTenantMatcher([]meta.TenantRow{
		{ID: "1", Name: "tenant-a", StreamPattern: "tenant-a-*"},
	})
	acct := reports.NewAccountant(conn, nil)
	acct.SetTenantMatcher(tm)

	rep, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From:   time.Now().AddDate(0, -1, 0),
		To:     time.Now(),
		Tenant: "tenant-a",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the tenant-a-stream row should survive the filter.
	if len(rep.Rows) != 1 {
		t.Errorf("want 1 row after tenant filter, got %d", len(rep.Rows))
	}
}

// ─── ComputeUsage — tenant-excluded byte-counter row must not skew disclosure ─

// Regression guard (VD-41 / audit [10]): the egress-method trackers must count
// ONLY rows that survive the tenant filter. The excluded row here carries
// byte-counter data (egress_bytes>0); the surviving row is bitrate. If a future
// change moved the sawByteCounter/sawBitrate assignments before the tenant
// `continue`, the excluded row would leak into the report as a false "mixed"
// disclosure — this test pins the correct placement.
func TestAcctConn_ComputeUsage_TenantFilter_ExcludedByteCounterRow_NotMixed(t *testing.T) {
	included := []any{"live", "tenant-a-stream", float64(10.0), uint64(0), uint64(0)}          // bitrate, kept
	excluded := []any{"live", "other-stream", float64(20.0), uint64(1_000_000_000), uint64(0)} // byte-counter, filtered out

	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(), nil). // concurrencyPeaks empty
		withQuery(acctNewFakeRows(included, excluded), nil)

	tm := reports.NewTenantMatcher([]meta.TenantRow{
		{ID: "1", Name: "tenant-a", StreamPattern: "tenant-a-*"},
	})
	acct := reports.NewAccountant(conn, nil)
	acct.SetTenantMatcher(tm)

	rep, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From:   time.Now().AddDate(0, -1, 0),
		To:     time.Now(),
		Tenant: "tenant-a",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rep.Rows) != 1 {
		t.Fatalf("want 1 row after tenant filter, got %d", len(rep.Rows))
	}
	// The surviving row is bitrate; the excluded byte-counter row must not contribute.
	if rep.EgressMethod != reports.EgressMethodBitrateXWatchTime {
		t.Errorf("report EgressMethod: got %q, want %q (excluded byte-counter row must not leak into disclosure)",
			rep.EgressMethod, reports.EgressMethodBitrateXWatchTime)
	}
}

// ─── ComputeUsage — fetchConcurrencyPeaks error is non-fatal ─────────────────

func TestAcctConn_ComputeUsage_ConcurrencyPeakError_NonFatal(t *testing.T) {
	// Queue 1: fetchConcurrencyPeaks → error (non-fatal)
	// Queue 2: main usage → 1 row
	usageRow := []any{"live", "s1", float64(60.0), uint64(0), uint64(0)}

	conn := newAcctFakeConn().
		withQuery(nil, errors.New("concurrency error")). // non-fatal
		withQuery(acctNewFakeRows(usageRow), nil)

	acct := reports.NewAccountant(conn, nil)
	rep, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From: time.Now().AddDate(0, -1, 0),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatalf("expected no error (concurrency error is non-fatal), got: %v", err)
	}
	if len(rep.Rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rep.Rows))
	}
	// peak_concurrency falls back to 0 since concurrencyMap was empty.
	if rep.Rows[0].PeakConcurrency != 0 {
		t.Errorf("PeakConcurrency: got %d, want 0 (fallback)", rep.Rows[0].PeakConcurrency)
	}
}

// ─── ComputeUsage — main query error ─────────────────────────────────────────

func TestAcctConn_ComputeUsage_QueryError(t *testing.T) {
	sentinel := errors.New("usage table unavailable")

	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(), nil). // concurrencyPeaks ok
		withQuery(nil, sentinel)           // main query error

	acct := reports.NewAccountant(conn, nil)
	_, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From: time.Now().AddDate(0, -1, 0),
		To:   time.Now(),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "usage query:") {
		t.Errorf("error should contain 'usage query:': %q", err.Error())
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error should wrap sentinel: %v", err)
	}
}

// ─── ComputeUsage — scan error ───────────────────────────────────────────────

func TestAcctConn_ComputeUsage_ScanError(t *testing.T) {
	scanErr := errors.New("scan failure")
	rows := &acctFakeRows{data: [][]any{{"live", "s1"}}, idx: -1, scanErr: scanErr}

	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(), nil).
		withQuery(rows, nil)

	acct := reports.NewAccountant(conn, nil)
	_, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From: time.Now().AddDate(0, -1, 0),
		To:   time.Now(),
	})
	if err == nil {
		t.Fatal("expected error from scan, got nil")
	}
	if !strings.Contains(err.Error(), "scan") {
		t.Errorf("error should mention 'scan': %q", err.Error())
	}
}

// ─── ComputeUsage — rows.Err propagation ─────────────────────────────────────

func TestAcctConn_ComputeUsage_RowsErr(t *testing.T) {
	sentinel := errors.New("rows iter error")

	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(), nil).
		withQuery(acctNewErrRows(sentinel), nil)

	acct := reports.NewAccountant(conn, nil)
	_, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From: time.Now().AddDate(0, -1, 0),
		To:   time.Now(),
	})
	if err == nil {
		t.Fatal("expected error from rows.Err, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error should wrap sentinel: %v", err)
	}
}

// ─── Reconcile — NilConn ─────────────────────────────────────────────────────

func TestAcctConn_Reconcile_NilConn(t *testing.T) {
	acct := reports.NewAccountant(nil, nil)
	r, err := acct.Reconcile(context.Background(), time.Now().AddDate(0, -1, 0), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.WithinTolerance {
		t.Error("nil-conn Reconcile should always be within tolerance")
	}
	if r.DriftPct != 0 {
		t.Errorf("DriftPct: got %f, want 0", r.DriftPct)
	}
}

// ─── Reconcile — within tolerance ────────────────────────────────────────────

func TestAcctConn_Reconcile_WithinTolerance(t *testing.T) {
	// rollupMinutes=100.0, rawMinutes=100.5 → drift=0.5% < 1.0%
	conn := newAcctFakeConn().
		withRow(acctNewFakeRow(float64(100.0))).
		withRow(acctNewFakeRow(float64(100.5), uint64(500)))

	acct := reports.NewAccountant(conn, nil)
	r, err := acct.Reconcile(context.Background(),
		time.Now().AddDate(0, -1, 0), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.WithinTolerance {
		t.Errorf("0.5%% drift should be within tolerance, got WithinTolerance=%v DriftPct=%.4f", r.WithinTolerance, r.DriftPct)
	}
	if r.RollupViewerMinutes != 100.0 {
		t.Errorf("RollupViewerMinutes: got %f, want 100.0", r.RollupViewerMinutes)
	}
	if r.RawViewerMinutes != 100.5 {
		t.Errorf("RawViewerMinutes: got %f, want 100.5", r.RawViewerMinutes)
	}
	if r.DataPoints != 500 {
		t.Errorf("DataPoints: got %d, want 500", r.DataPoints)
	}
}

// ─── Reconcile — outside tolerance ───────────────────────────────────────────

func TestAcctConn_Reconcile_OutsideTolerance(t *testing.T) {
	// rollupMinutes=100.0, rawMinutes=97.0 → drift=3.09% > 1.0%
	conn := newAcctFakeConn().
		withRow(acctNewFakeRow(float64(100.0))).
		withRow(acctNewFakeRow(float64(97.0), uint64(200)))

	acct := reports.NewAccountant(conn, nil)
	r, err := acct.Reconcile(context.Background(),
		time.Now().AddDate(0, -1, 0), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.WithinTolerance {
		t.Errorf("3%% drift should NOT be within tolerance, got WithinTolerance=%v DriftPct=%.4f", r.WithinTolerance, r.DriftPct)
	}
}

// ─── Reconcile — rollup query error ──────────────────────────────────────────

func TestAcctConn_Reconcile_RollupQueryError(t *testing.T) {
	sentinel := errors.New("rollup table gone")
	conn := newAcctFakeConn().
		withRow(acctNewErrRow(sentinel))

	acct := reports.NewAccountant(conn, nil)
	_, err := acct.Reconcile(context.Background(),
		time.Now().AddDate(0, -1, 0), time.Now())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "rollup query:") {
		t.Errorf("error should contain 'rollup query:': %q", err.Error())
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error should wrap sentinel: %v", err)
	}
}

// ─── Reconcile — raw sessions query error ────────────────────────────────────

func TestAcctConn_Reconcile_RawQueryError(t *testing.T) {
	sentinel := errors.New("viewer_sessions unavailable")
	conn := newAcctFakeConn().
		withRow(acctNewFakeRow(float64(100.0))).
		withRow(acctNewErrRow(sentinel))

	acct := reports.NewAccountant(conn, nil)
	_, err := acct.Reconcile(context.Background(),
		time.Now().AddDate(0, -1, 0), time.Now())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "raw sessions query:") {
		t.Errorf("error should contain 'raw sessions query:': %q", err.Error())
	}
}

// ─── Reconcile — zero rawMinutes (no drift) ──────────────────────────────────

func TestAcctConn_Reconcile_ZeroRaw(t *testing.T) {
	conn := newAcctFakeConn().
		withRow(acctNewFakeRow(float64(0.0))).
		withRow(acctNewFakeRow(float64(0.0), uint64(0)))

	acct := reports.NewAccountant(conn, nil)
	r, err := acct.Reconcile(context.Background(),
		time.Now().AddDate(0, -1, 0), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.DriftPct != 0.0 {
		t.Errorf("DriftPct: got %f, want 0.0 when rawMinutes=0", r.DriftPct)
	}
	if !r.WithinTolerance {
		t.Error("WithinTolerance should be true when drift=0")
	}
}

// ─── AggregateByTenant ────────────────────────────────────────────────────────

func TestAggregateByTenant_MultiTenant(t *testing.T) {
	tenantA := "tenant-a"
	tenantB := "tenant-b"
	rows := []reports.UsageRow{
		{App: "live", Tenant: &tenantA, ViewerMinutes: 100, EgressGB: 1.0, PeakConcurrency: 10},
		{App: "live", Tenant: &tenantA, ViewerMinutes: 50, EgressGB: 0.5, PeakConcurrency: 5},
		{App: "live", Tenant: &tenantB, ViewerMinutes: 200, EgressGB: 2.0, PeakConcurrency: 20},
	}

	stats := reports.AggregateByTenant(rows)
	if len(stats) != 2 {
		t.Fatalf("want 2 tenant stats, got %d", len(stats))
	}

	byName := map[string]reports.TenantStat{}
	for _, s := range stats {
		byName[s.Tenant] = s
	}

	a := byName["tenant-a"]
	if a.ViewerMinutes != 150 {
		t.Errorf("tenant-a ViewerMinutes: got %f, want 150", a.ViewerMinutes)
	}
	if a.EgressGB != 1.5 {
		t.Errorf("tenant-a EgressGB: got %f, want 1.5", a.EgressGB)
	}
	if a.PeakConcurrency != 10 {
		t.Errorf("tenant-a PeakConcurrency: got %d, want 10", a.PeakConcurrency)
	}

	b := byName["tenant-b"]
	if b.ViewerMinutes != 200 {
		t.Errorf("tenant-b ViewerMinutes: got %f, want 200", b.ViewerMinutes)
	}
}

func TestAggregateByTenant_EmptyInput(t *testing.T) {
	stats := reports.AggregateByTenant(nil)
	if stats != nil && len(stats) != 0 {
		t.Errorf("want empty/nil, got %v", stats)
	}
}

func TestAggregateByTenant_UnassignedFallback(t *testing.T) {
	// Nil tenant pointer → "unassigned".
	rows := []reports.UsageRow{
		{App: "live", Tenant: nil, ViewerMinutes: 75, EgressGB: 0.75},
	}
	stats := reports.AggregateByTenant(rows)
	if len(stats) != 1 {
		t.Fatalf("want 1 stat, got %d", len(stats))
	}
	if stats[0].Tenant != "unassigned" {
		t.Errorf("Tenant: got %q, want unassigned", stats[0].Tenant)
	}
}

func TestAggregateByTenant_EmptyTenantString(t *testing.T) {
	// Empty string tenant → "unassigned".
	empty := ""
	rows := []reports.UsageRow{
		{App: "live", Tenant: &empty, ViewerMinutes: 50},
	}
	stats := reports.AggregateByTenant(rows)
	if len(stats) != 1 {
		t.Fatalf("want 1 stat, got %d", len(stats))
	}
	if stats[0].Tenant != "unassigned" {
		t.Errorf("Tenant: got %q, want unassigned", stats[0].Tenant)
	}
}

func TestAggregateByTenant_RecordingGB(t *testing.T) {
	ten := "t1"
	rows := []reports.UsageRow{
		{App: "live", Tenant: &ten, RecordingGB: 1.0},
		{App: "live", Tenant: &ten, RecordingGB: 2.5},
	}
	stats := reports.AggregateByTenant(rows)
	if len(stats) != 1 {
		t.Fatalf("want 1 stat, got %d", len(stats))
	}
	if stats[0].RecordingGB != 3.5 {
		t.Errorf("RecordingGB: got %f, want 3.5", stats[0].RecordingGB)
	}
}

// ─── SetTenantMatcher + resolveTenantMatcher ──────────────────────────────────

func TestSetTenantMatcher_Wires(t *testing.T) {
	acct := reports.NewAccountant(nil, nil)
	tm := reports.NewTenantMatcher([]meta.TenantRow{
		{ID: "1", Name: "tenant-x", StreamPattern: "x/*"},
	})
	acct.SetTenantMatcher(tm)

	// ComputeUsage with nil conn uses the tenant matcher via resolveTenantMatcher.
	// Even though no rows are returned, SetTenantMatcher must not panic.
	rep, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From: time.Now().AddDate(0, -1, 0),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rep == nil {
		t.Fatal("result nil")
	}
}

func TestSetTenantMatcher_ConnUsesCustomMatcher(t *testing.T) {
	// Matcher assigns stream "vip-stream" to "premium".
	tm := reports.NewTenantMatcher([]meta.TenantRow{
		{ID: "1", Name: "premium", StreamPattern: "vip-*"},
	})

	usageRow := []any{"live", "vip-stream", float64(10.0), uint64(0), uint64(0)}
	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(), nil).
		withQuery(acctNewFakeRows(usageRow), nil)

	acct := reports.NewAccountant(conn, nil)
	acct.SetTenantMatcher(tm)

	rep, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From: time.Now().AddDate(0, -1, 0),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rep.Rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rep.Rows))
	}
	r := rep.Rows[0]
	if r.Tenant == nil || *r.Tenant != "premium" {
		t.Errorf("Tenant: got %v, want 'premium'", r.Tenant)
	}
}

func TestResolveTenantMatcher_NilMeta(t *testing.T) {
	// meta == nil and no explicit matcher → NewTenantMatcher(nil) with no tenants.
	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(), nil).
		withQuery(acctNewFakeRows(), nil)

	acct := reports.NewAccountant(conn, nil)
	rep, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From: time.Now().AddDate(0, -1, 0),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rep == nil {
		t.Fatal("result nil")
	}
}

// ─── Scheduler — SetAlertStore ────────────────────────────────────────────────

func TestScheduler_SetAlertStore(t *testing.T) {
	acct := reports.NewAccountant(nil, nil)
	svc := reports.NewScheduler(reports.SchedulerConfig{
		ArtifactsDir: t.TempDir(),
		TickInterval: time.Hour,
	}, acct, nil, acctLogger(t))

	hw := &fakeHistoryWriter{}
	svc.SetAlertStore(hw)

	// RunOnce with nil meta → returns immediately (nil meta guard).
	// No panic means SetAlertStore did not break anything.
	ctx := context.Background()
	svc.RunOnce(ctx)
}

// ─── Scheduler — Start/Stop lifecycle ────────────────────────────────────────

func TestScheduler_StartStop_NoHang(t *testing.T) {
	acct := reports.NewAccountant(nil, nil)
	svc := reports.NewScheduler(reports.SchedulerConfig{
		ArtifactsDir: t.TempDir(),
		TickInterval: 100 * time.Millisecond,
	}, acct, nil, acctLogger(t))

	ctx, cancel := context.WithCancel(context.Background())
	svc.Start(ctx)

	// Let it run briefly so the goroutine is definitely started.
	time.Sleep(10 * time.Millisecond)

	// Stop via channel.
	svc.Stop()
	t.Log("Stop() returned — no hang")

	// Also cancel context to clean up.
	cancel()
}

func TestScheduler_StartStop_CtxCancel(t *testing.T) {
	acct := reports.NewAccountant(nil, nil)
	svc := reports.NewScheduler(reports.SchedulerConfig{
		ArtifactsDir: t.TempDir(),
		TickInterval: 100 * time.Millisecond,
	}, acct, nil, acctLogger(t))

	ctx, cancel := context.WithCancel(context.Background())
	svc.Start(ctx)
	time.Sleep(10 * time.Millisecond)

	// Cancel context instead of Stop.
	cancel()
	// Stop should still work (or can be called multiple times is an issue, so just Stop).
	// The Stop closes the stopCh; after ctx cancel the goroutine exits via ctx.Done.
	// We just want to ensure no deadlock/panic.
	t.Log("ctx cancelled — goroutine should exit")
}

// ─── Scheduler — runDue nil meta (early return) ───────────────────────────────

func TestScheduler_RunDue_NilMeta(t *testing.T) {
	acct := reports.NewAccountant(nil, nil)
	svc := reports.NewScheduler(reports.SchedulerConfig{
		ArtifactsDir: t.TempDir(),
		TickInterval: time.Hour,
	}, acct, nil, acctLogger(t)) // nil meta

	// RunOnce → runDue → nil meta guard → returns without panic.
	svc.RunOnce(context.Background())
}

// ─── Scheduler — writeFailureAlert (schedule that fails ComputeUsage) ─────────

func TestScheduler_WriteFailureAlert_OnComputeError(t *testing.T) {
	ctx := context.Background()
	ms := acctMetaStore(t)

	// Create a due schedule.
	pastDue := time.Now().Add(-1 * time.Second).UnixMilli()
	sched := meta.ReportScheduleRow{
		Cron:      "0 0 *",
		Format:    "csv",
		ScopeJSON: "{}",
		NextRunAt: &pastDue,
	}
	if _, err := ms.CreateReportSchedule(ctx, sched); err != nil {
		t.Fatalf("CreateReportSchedule: %v", err)
	}

	// Conn that fails the main usage query (so ComputeUsage returns error).
	// Queue 1: concurrencyPeaks → empty (non-fatal)
	// Queue 2: main usage → error (fatal → runSchedule fails → writeFailureAlert called)
	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(), nil).
		withQuery(nil, errors.New("forced compute failure"))

	acct := reports.NewAccountant(conn, ms)

	hw := &fakeHistoryWriter{}
	svc := reports.NewScheduler(reports.SchedulerConfig{
		ArtifactsDir: t.TempDir(),
		TickInterval: time.Hour,
	}, acct, ms, acctLogger(t))
	svc.SetAlertStore(hw)

	svc.RunOnce(ctx)

	hw.mu.Lock()
	n := len(hw.calls)
	hw.mu.Unlock()

	if n == 0 {
		t.Error("expected writeFailureAlert to call CreateAlertHistory at least once")
	}
	t.Logf("writeFailureAlert called %d times", n)
}

func TestScheduler_WriteFailureAlert_NilAlertStore(t *testing.T) {
	// No alertStore set → writeFailureAlert should return without panic.
	ctx := context.Background()
	ms := acctMetaStore(t)

	pastDue := time.Now().Add(-1 * time.Second).UnixMilli()
	sched := meta.ReportScheduleRow{
		Cron:      "0 0 *",
		Format:    "csv",
		ScopeJSON: "{}",
		NextRunAt: &pastDue,
	}
	if _, err := ms.CreateReportSchedule(ctx, sched); err != nil {
		t.Fatalf("CreateReportSchedule: %v", err)
	}

	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(), nil).
		withQuery(nil, errors.New("forced failure"))

	acct := reports.NewAccountant(conn, ms)
	svc := reports.NewScheduler(reports.SchedulerConfig{
		ArtifactsDir: t.TempDir(),
		TickInterval: time.Hour,
	}, acct, ms, acctLogger(t))
	// No SetAlertStore — alertStore is nil.

	// Should not panic.
	svc.RunOnce(ctx)
}

func TestScheduler_WriteFailureAlert_AlertStoreError(t *testing.T) {
	// alertStore.CreateAlertHistory returns an error → scheduler logs but doesn't crash.
	ctx := context.Background()
	ms := acctMetaStore(t)

	pastDue := time.Now().Add(-1 * time.Second).UnixMilli()
	sched := meta.ReportScheduleRow{
		Cron:      "0 0 *",
		Format:    "csv",
		ScopeJSON: "{}",
		NextRunAt: &pastDue,
	}
	if _, err := ms.CreateReportSchedule(ctx, sched); err != nil {
		t.Fatalf("CreateReportSchedule: %v", err)
	}

	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(), nil).
		withQuery(nil, errors.New("forced failure"))

	acct := reports.NewAccountant(conn, ms)
	hw := &fakeHistoryWriter{err: errors.New("alert store error")}
	svc := reports.NewScheduler(reports.SchedulerConfig{
		ArtifactsDir: t.TempDir(),
		TickInterval: time.Hour,
	}, acct, ms, acctLogger(t))
	svc.SetAlertStore(hw)

	// Should not panic even when alertStore.CreateAlertHistory returns error.
	svc.RunOnce(ctx)
}

// ─── ParseWhitelabelHeader ────────────────────────────────────────────────────

func TestParseWhitelabelHeader_Valid(t *testing.T) {
	json := `{"logo_path":"/img/logo.png","name":"Acme TV","address":"123 Main St"}`
	h := reports.ParseWhitelabelHeader(json)
	if h == nil {
		t.Fatal("expected non-nil header")
	}
	if h.Name != "Acme TV" {
		t.Errorf("Name: got %q, want Acme TV", h.Name)
	}
	if h.LogoPath != "/img/logo.png" {
		t.Errorf("LogoPath: got %q, want /img/logo.png", h.LogoPath)
	}
	if h.Address != "123 Main St" {
		t.Errorf("Address: got %q, want 123 Main St", h.Address)
	}
}

func TestParseWhitelabelHeader_Empty(t *testing.T) {
	h := reports.ParseWhitelabelHeader("")
	if h != nil {
		t.Errorf("expected nil for empty input, got %v", h)
	}
}

func TestParseWhitelabelHeader_Malformed(t *testing.T) {
	h := reports.ParseWhitelabelHeader("not json at all {{{")
	if h != nil {
		t.Errorf("expected nil for malformed JSON, got %v", h)
	}
}

func TestParseWhitelabelHeader_NoName(t *testing.T) {
	// Valid JSON but no name field → nil (name is required).
	h := reports.ParseWhitelabelHeader(`{"logo_path":"/img/logo.png","address":"123 Main"}`)
	if h != nil {
		t.Errorf("expected nil when name is empty, got %v", h)
	}
}

func TestParseWhitelabelHeader_NameOnly(t *testing.T) {
	h := reports.ParseWhitelabelHeader(`{"name":"Brand Co"}`)
	if h == nil {
		t.Fatal("expected non-nil header with name only")
	}
	if h.Name != "Brand Co" {
		t.Errorf("Name: got %q, want Brand Co", h.Name)
	}
}

// ─── escapePDFString — via GenerateStatement PDF ─────────────────────────────

func TestGenerateStatement_PDF_EscapeSpecialChars(t *testing.T) {
	// Build a report whose row contains parentheses and backslash — these must be
	// escaped in the PDF string literal by escapePDFString.
	stream := "stream(1)\\test"
	sid := stream
	rows := []reports.UsageRow{
		{
			App:           "live",
			StreamID:      &sid,
			ViewerMinutes: 10.0,
			EgressGB:      0.1,
			EgressMethod:  reports.EgressMethodBitrateXWatchTime,
		},
	}
	rep := &reports.UsageReport{
		Rows:         rows,
		Totals:       reports.UsageTotals{ViewerMinutes: 10.0, EgressGB: 0.1},
		EgressMethod: reports.EgressMethodBitrateXWatchTime,
	}

	stmt, err := reports.GenerateStatement(rep, reports.StatementOptions{
		From:   time.Now().AddDate(0, -1, 0),
		To:     time.Now(),
		Format: reports.FormatPDF,
	})
	if err != nil {
		t.Fatalf("GenerateStatement PDF: %v", err)
	}
	if !strings.HasPrefix(string(stmt.Data), "%PDF-") {
		t.Error("output should start with %PDF-")
	}
}

func TestGenerateStatement_PDF_NonASCIIReplaced(t *testing.T) {
	// Non-ASCII chars in whitelabel header should be replaced with '?' (not panic).
	wl := &reports.WhitelabelHeader{Name: "Müller GmbH", Address: "Straße 1"}
	rep := &reports.UsageReport{
		Rows:         []reports.UsageRow{},
		EgressMethod: reports.EgressMethodBitrateXWatchTime,
	}
	stmt, err := reports.GenerateStatement(rep, reports.StatementOptions{
		From:       time.Now().AddDate(0, -1, 0),
		To:         time.Now(),
		Format:     reports.FormatPDF,
		Whitelabel: wl,
	})
	if err != nil {
		t.Fatalf("GenerateStatement PDF: %v", err)
	}
	if len(stmt.Data) == 0 {
		t.Error("expected non-empty PDF output")
	}
}

func TestGenerateStatement_UnknownFormat(t *testing.T) {
	rep := &reports.UsageReport{EgressMethod: reports.EgressMethodBitrateXWatchTime}
	_, err := reports.GenerateStatement(rep, reports.StatementOptions{
		Format: reports.StatementFormat("xml"),
	})
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
}

// ─── resolveEnv — via S3 upload with custom envRef ───────────────────────────

func TestS3Upload_CustomEnvRef(t *testing.T) {
	// Test resolveEnv with non-empty AccessKeyEnvRef that maps to a set env var.
	fake := reports.NewS3FakeServer()
	srv := httptest.NewServer(fake)
	defer srv.Close()

	t.Setenv("CUSTOM_S3_KEY", "my-key-id")
	t.Setenv("CUSTOM_S3_SECRET", "my-secret-key")

	uploader := reports.NewS3Uploader(reports.S3Config{
		Endpoint:        srv.URL,
		Bucket:          "test-bucket",
		Prefix:          "rpt/",
		Region:          "eu-west-1",
		AccessKeyEnvRef: "CUSTOM_S3_KEY",
		SecretKeyEnvRef: "CUSTOM_S3_SECRET",
	}, acctLogger(t))

	data := []byte("test,data\n1,2\n")
	if err := uploader.Upload(context.Background(), "rpt/file.csv", "text/csv", data); err != nil {
		t.Fatalf("S3 upload with custom env refs failed: %v", err)
	}
}

func TestS3Upload_MissingCredentials(t *testing.T) {
	// Neither envRef nor fallback env vars are set → error.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("EMPTY_KEY", "")
	t.Setenv("EMPTY_SECRET", "")

	uploader := reports.NewS3Uploader(reports.S3Config{
		Endpoint:        "http://localhost:19999",
		Bucket:          "b",
		Region:          "us-east-1",
		AccessKeyEnvRef: "EMPTY_KEY",
		SecretKeyEnvRef: "EMPTY_SECRET",
	}, acctLogger(t))

	err := uploader.Upload(context.Background(), "k", "text/csv", []byte("x"))
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
	if !strings.Contains(err.Error(), "credentials") {
		t.Errorf("error should mention credentials: %q", err.Error())
	}
}

// ─── likeMatch remaining branches via TenantMatcher ──────────────────────────

func TestLikeMatch_PercentOnlyPattern(t *testing.T) {
	// pattern="%" matches any string (including empty).
	tm := reports.NewTenantMatcher([]meta.TenantRow{
		{ID: "1", Name: "catch-all", StreamPattern: "%"},
	})
	tests := []struct {
		stream string
	}{
		{"anything"},
		{""},
		{"very/long/stream/path"},
	}
	for _, tc := range tests {
		got := tm.Resolve(tc.stream, nil)
		if got != "catch-all" {
			t.Errorf("Resolve(%q) = %q, want catch-all", tc.stream, got)
		}
	}
}

func TestLikeMatch_EmptyPatternEmptyStream(t *testing.T) {
	// Empty pattern only matches empty stream.
	tm := reports.NewTenantMatcher([]meta.TenantRow{
		{ID: "1", Name: "empty-match", StreamPattern: ""},
	})
	// Empty pattern: globMatch("", "") calls likeMatch("", "") → true (pattern == s == "").
	// But globMatch normalizes first, and empty StreamPattern is skipped by Resolve phase-2.
	// The code: if t.StreamPattern != "" — so empty pattern never matches.
	got := tm.Resolve("", nil)
	if got != "" {
		// Empty StreamPattern is skipped → unassigned. Correct behavior.
		t.Errorf("Resolve with empty pattern: got %q, want ''", got)
	}
}

func TestLikeMatch_UnderscoreWildcard(t *testing.T) {
	// "_" matches exactly one character.
	tm := reports.NewTenantMatcher([]meta.TenantRow{
		{ID: "1", Name: "underscore-tenant", StreamPattern: "s_1"},
	})
	tests := []struct {
		stream string
		want   string
	}{
		{"s11", "underscore-tenant"},
		{"sA1", "underscore-tenant"},
		{"s1", ""},   // too short
		{"ss11", ""}, // too long
	}
	for _, tc := range tests {
		got := tm.Resolve(tc.stream, nil)
		if got != tc.want {
			t.Errorf("Resolve(%q) = %q, want %q", tc.stream, got, tc.want)
		}
	}
}

func TestLikeMatch_EmptyStringVsNonEmptyPattern(t *testing.T) {
	// Non-% pattern with empty stream → false (likeMatch "if s == "" { return false }").
	tm := reports.NewTenantMatcher([]meta.TenantRow{
		{ID: "1", Name: "exact", StreamPattern: "live"},
	})
	got := tm.Resolve("", nil)
	if got != "" {
		t.Errorf("empty stream should not match 'live', got %q", got)
	}
}

func TestLikeMatch_MiddlePercent(t *testing.T) {
	// "live%cast" matches "live-broadcast".
	tm := reports.NewTenantMatcher([]meta.TenantRow{
		{ID: "1", Name: "tv", StreamPattern: "live%cast"},
	})
	tests := []struct {
		stream string
		want   string
	}{
		{"livecast", "tv"},
		{"live-broadcast", "tv"},
		{"livethiscast", "tv"},
		{"liveCASTx", ""},
	}
	for _, tc := range tests {
		got := tm.Resolve(tc.stream, nil)
		if got != tc.want {
			t.Errorf("Resolve(%q) = %q, want %q", tc.stream, got, tc.want)
		}
	}
}

// ─── fetchConcurrencyPeaks scan error (via ComputeUsage) ─────────────────────

func TestAcctConn_FetchConcurrencyPeaks_ScanError_NonFatal(t *testing.T) {
	// fetchConcurrencyPeaks scan error → non-fatal, falls back to 0.
	scanErrRows := &acctFakeRows{
		data:    [][]any{{"live", "s1", int64(5)}},
		idx:     -1,
		scanErr: errors.New("scan error"),
	}
	usageRow := []any{"live", "s1", float64(10.0), uint64(0), uint64(0)}

	conn := newAcctFakeConn().
		withQuery(scanErrRows, nil).
		withQuery(acctNewFakeRows(usageRow), nil)

	acct := reports.NewAccountant(conn, nil)
	rep, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From: time.Now().AddDate(0, -1, 0),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatalf("expected no error (scan error in concurrency is non-fatal), got: %v", err)
	}
	if len(rep.Rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rep.Rows))
	}
	if rep.Rows[0].PeakConcurrency != 0 {
		t.Errorf("PeakConcurrency: got %d, want 0 (fallback after scan error)", rep.Rows[0].PeakConcurrency)
	}
}

func TestAcctConn_FetchConcurrencyPeaks_RowsErr_NonFatal(t *testing.T) {
	// fetchConcurrencyPeaks rows.Err → non-fatal, falls back to 0.
	conn := newAcctFakeConn().
		withQuery(acctNewErrRows(errors.New("rows iter error")), nil).
		withQuery(acctNewFakeRows([]any{"live", "s1", float64(5.0), uint64(0), uint64(0)}), nil)

	acct := reports.NewAccountant(conn, nil)
	rep, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From: time.Now().AddDate(0, -1, 0),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(rep.Rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rep.Rows))
	}
}

// ─── ComputeUsage — multiple rows, totals check ───────────────────────────────

func TestAcctConn_ComputeUsage_Totals(t *testing.T) {
	// Two streams: viewer_minutes=100+200=300, egress_bytes=0 for both → bitrate model.
	row1 := []any{"live", "s1", float64(100.0), uint64(0), uint64(0)}
	row2 := []any{"live", "s2", float64(200.0), uint64(0), uint64(0)}
	concRow1 := []any{"live", "s1", int64(10)}
	concRow2 := []any{"live", "s2", int64(20)}

	conn := newAcctFakeConn().
		withQuery(acctNewFakeRows(concRow1, concRow2), nil).
		withQuery(acctNewFakeRows(row1, row2), nil)

	acct := reports.NewAccountant(conn, nil)
	rep, err := acct.ComputeUsage(context.Background(), reports.UsageParams{
		From: time.Now().AddDate(0, -1, 0),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rep.Rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rep.Rows))
	}
	// Totals.
	if rep.Totals.ViewerMinutes != 300.0 {
		t.Errorf("Totals.ViewerMinutes: got %f, want 300.0", rep.Totals.ViewerMinutes)
	}
	if rep.Totals.PeakConcurrency != 20 {
		t.Errorf("Totals.PeakConcurrency: got %d, want 20", rep.Totals.PeakConcurrency)
	}
	// EgressGB total = kbpsToGBPerMinute(100,1000) + kbpsToGBPerMinute(200,1000) = 0.75 + 1.5 = 2.25
	wantEgress := 0.75 + 1.5
	if rep.Totals.EgressGB != wantEgress {
		t.Errorf("Totals.EgressGB: got %f, want %f", rep.Totals.EgressGB, wantEgress)
	}
}

// ─── kbpsToGBPerMinute formula guard ─────────────────────────────────────────

func TestKbpsToGBFormula_ViaSession(t *testing.T) {
	// One session: 60 viewer-minutes at 1000 kbps → 0.45 GB.
	// Formula: 60 * 1000 * 60 * 1000 / 8 / 1e9 = 3.6e9 / 8e9 / 1e9... let me recalc:
	// 60 * 1000 = 60000 (viewer_min * kbps)
	// * 60 = 3,600,000 (× seconds per minute)
	// * 1000 = 3,600,000,000 (× millibit/sec → bits per... hmm wait)
	// Actually: kbpsToGBPerMinute(60, 1000) = 60 * 1000 * 60 * 1000 / 8 / 1e9
	// = 60 * 1000 * 60000 / 8 / 1e9
	// = 3,600,000,000 / 8 / 1e9
	// = 450,000,000 / 1e9 = 0.45 GB
	sessions := []reports.SyntheticSession{{
		App:         "live",
		StreamID:    "s1",
		WatchTimeS:  3600, // 60 minutes
		BitrateKbps: 1000,
	}}
	rep := reports.ComputeUsageFromSessions(sessions, nil)
	if len(rep.Rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rep.Rows))
	}
	const wantEgress = 0.45
	if rep.Rows[0].EgressGB != wantEgress {
		t.Errorf("EgressGB: got %f, want %f", rep.Rows[0].EgressGB, wantEgress)
	}
}

// ─── GenerateStatement — CSV whitelabel with address ─────────────────────────

func TestGenerateStatement_CSV_WhitelabelAddress(t *testing.T) {
	wl := &reports.WhitelabelHeader{
		Name:    "Brand Inc",
		Address: "456 Oak Ave",
	}
	rep := &reports.UsageReport{
		Rows:         []reports.UsageRow{},
		EgressMethod: reports.EgressMethodBitrateXWatchTime,
	}
	stmt, err := reports.GenerateStatement(rep, reports.StatementOptions{
		From:       time.Now().AddDate(0, -1, 0),
		To:         time.Now(),
		Format:     reports.FormatCSV,
		Whitelabel: wl,
	})
	if err != nil {
		t.Fatalf("GenerateStatement CSV: %v", err)
	}
	body := string(stmt.Data)
	if !strings.Contains(body, "Brand Inc") {
		t.Errorf("CSV missing whitelabel name, body: %q", body[:min(200, len(body))])
	}
	if !strings.Contains(body, "456 Oak Ave") {
		t.Errorf("CSV missing whitelabel address, body: %q", body[:min(200, len(body))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ─── ensure reflect import is used ────────────────────────────────────────────

var _ = reflect.TypeOf
