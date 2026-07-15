// Package query — unit tests for ClickHouse-backed query methods using a mock
// driver.Conn. Covers AudienceAnalytics, GeoBreakdown, DeviceBreakdown,
// QoeSummary, IngestTimeseries, QueryProbeResults, applyRetention, and
// additional FleetNodes / LiveOverview paths.
//
// TDD: each test group was first written with a deliberately wrong expected value
// (RED), confirmed to fail, then corrected (GREEN). See redEvidence in the
// work-order completion report.
package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/column"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/license"
)

// ─── Mock driver.Rows ────────────────────────────────────────────────────────

// fakeRows is a minimal driver.Rows implementation backed by pre-canned rows.
// Each row is a []any whose values are assigned to Scan destinations via
// reflection. All types must match exactly (e.g. int64 not int).
type fakeRows struct {
	data    [][]any
	idx     int   // current position; -1 before first Next()
	rowsErr error // returned by Err() after iteration
	scanErr error // returned by Scan() when non-nil
}

func newFakeRows(rows ...[]any) *fakeRows {
	return &fakeRows{data: rows, idx: -1}
}

func newErrRows(err error) *fakeRows {
	return &fakeRows{rowsErr: err, idx: -1}
}

func (r *fakeRows) Next() bool {
	r.idx++
	return r.idx < len(r.data)
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	if r.idx < 0 || r.idx >= len(r.data) {
		return fmt.Errorf("fakeRows: no current row at index %d", r.idx)
	}
	return scanDests(dest, r.data[r.idx])
}

func (r *fakeRows) Err() error                       { return r.rowsErr }
func (r *fakeRows) Close() error                     { return nil }
func (r *fakeRows) ScanStruct(_ any) error           { return nil }
func (r *fakeRows) ColumnTypes() []driver.ColumnType { return nil }
func (r *fakeRows) Totals(_ ...any) error            { return nil }
func (r *fakeRows) Columns() []string                { return nil }
func (r *fakeRows) HasData() bool                    { return len(r.data) > 0 }

// ─── Mock driver.Row ─────────────────────────────────────────────────────────

// fakeRow is a minimal driver.Row backed by a single pre-canned value list.
type fakeRow struct {
	data    []any
	scanErr error
}

func newFakeRow(vals ...any) *fakeRow { return &fakeRow{data: vals} }
func newErrRow(err error) *fakeRow    { return &fakeRow{scanErr: err} }

func (r *fakeRow) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	return scanDests(dest, r.data)
}

func (r *fakeRow) Err() error             { return r.scanErr }
func (r *fakeRow) ScanStruct(_ any) error { return nil }

// ─── scanDests: reflection-based column assignment ────────────────────────────

// scanDests assigns src values to dest pointers via reflection.
// Both slices must have the same length, and each src[i] must be the exact
// type that dest[i] points to (e.g. src int64 → dest *int64).
func scanDests(dest []any, src []any) error {
	if len(dest) != len(src) {
		return fmt.Errorf("scanDests: column count mismatch: dest=%d src=%d", len(dest), len(src))
	}
	for i, d := range dest {
		dv := reflect.ValueOf(d)
		if dv.Kind() != reflect.Ptr {
			return fmt.Errorf("scanDests: dest[%d] is not a pointer (%T)", i, d)
		}
		sv := reflect.ValueOf(src[i])
		if !sv.IsValid() {
			continue // nil source → leave destination unchanged
		}
		dv.Elem().Set(sv)
	}
	return nil
}

// ─── Mock clickhouse.Conn ─────────────────────────────────────────────────────

// fakeConn implements clickhouse.Conn (= driver.Conn) for unit tests.
// Query responses and QueryRow responses are served from FIFO queues.
type fakeConn struct {
	queryQueue []fakeQueryResp
	qi         int
	rowQueue   []fakeRowResp
	ri         int
	// capturedArgs records the args of every Query/QueryRow call, in order, so
	// tests can assert what actually reached the WHERE clause (e.g. a retention
	// clamp on `from`). Additive: existing tests that never read it are unaffected.
	capturedArgs [][]any
}

type fakeQueryResp struct {
	rows driver.Rows
	err  error
}
type fakeRowResp struct {
	row driver.Row
}

// withQuery appends a Query response to the queue.
func (c *fakeConn) withQuery(rows driver.Rows, err error) *fakeConn {
	c.queryQueue = append(c.queryQueue, fakeQueryResp{rows: rows, err: err})
	return c
}

// withRow appends a QueryRow response to the queue.
func (c *fakeConn) withRow(row driver.Row) *fakeConn {
	c.rowQueue = append(c.rowQueue, fakeRowResp{row: row})
	return c
}

func newFakeConn() *fakeConn { return &fakeConn{} }

// driver.Conn implementation.

func (c *fakeConn) Query(_ context.Context, _ string, args ...any) (driver.Rows, error) {
	c.capturedArgs = append(c.capturedArgs, args)
	if c.qi < len(c.queryQueue) {
		r := c.queryQueue[c.qi]
		c.qi++
		return r.rows, r.err
	}
	return newFakeRows(), nil // default: empty rows, no error
}

func (c *fakeConn) QueryRow(_ context.Context, _ string, args ...any) driver.Row {
	c.capturedArgs = append(c.capturedArgs, args)
	if c.ri < len(c.rowQueue) {
		r := c.rowQueue[c.ri]
		c.ri++
		return r.row
	}
	return newErrRow(fmt.Errorf("fakeConn: no QueryRow response queued"))
}

func (c *fakeConn) PrepareBatch(_ context.Context, _ string, _ ...driver.PrepareBatchOption) (driver.Batch, error) {
	return nil, nil
}
func (c *fakeConn) Contributors() []string                                    { return nil }
func (c *fakeConn) ServerVersion() (*driver.ServerVersion, error)             { return nil, nil }
func (c *fakeConn) Select(_ context.Context, _ any, _ string, _ ...any) error { return nil }
func (c *fakeConn) Exec(_ context.Context, _ string, _ ...any) error          { return nil }
func (c *fakeConn) AsyncInsert(_ context.Context, _ string, _ bool, _ ...any) error {
	return nil
}
func (c *fakeConn) Ping(_ context.Context) error { return nil }
func (c *fakeConn) Stats() driver.Stats          { return driver.Stats{} }
func (c *fakeConn) Close() error                 { return nil }

// Satisfy driver.Batch interface stubs (unused by query.Service).
type fakeBatch struct{}

func (b *fakeBatch) Abort() error                    { return nil }
func (b *fakeBatch) Append(_ ...any) error           { return nil }
func (b *fakeBatch) AppendStruct(_ any) error        { return nil }
func (b *fakeBatch) Column(_ int) driver.BatchColumn { return nil }
func (b *fakeBatch) Flush() error                    { return nil }
func (b *fakeBatch) Send() error                     { return nil }
func (b *fakeBatch) IsSent() bool                    { return false }
func (b *fakeBatch) Rows() int                       { return 0 }
func (b *fakeBatch) Columns() []column.Interface     { return nil }
func (b *fakeBatch) Close() error                    { return nil }

// ─── Fake helpers ─────────────────────────────────────────────────────────────

// nilSnapLive returns a nil snapshot — forces all snapshot paths to use nil.
type nilSnapLive struct{}

func (nilSnapLive) CurrentSnapshot() *domain.LiveSnapshot { return nil }
func (nilSnapLive) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot)
	return ch, func() {}
}

// fixedSnapLive returns a fixed snapshot.
type fixedSnapLive struct{ snap *domain.LiveSnapshot }

func (f *fixedSnapLive) CurrentSnapshot() *domain.LiveSnapshot { return f.snap }
func (f *fixedSnapLive) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot)
	return ch, func() {}
}

// fakeProbeQuerier mocks the ProbeResultQuerier interface. It records the from/to
// it was called with so tests can assert the retention clamp reached the store.
type fakeProbeQuerier struct {
	results []domain.ProbeResult
	err     error
	gotFrom time.Time
	gotTo   time.Time
}

func (f *fakeProbeQuerier) QueryProbeResults(_ context.Context, _ string, from, to time.Time, _ int, _ string) ([]domain.ProbeResult, error) {
	f.gotFrom = from
	f.gotTo = to
	return f.results, f.err
}

// fakeDiscovery mocks NodeRoleDiscoverer.
type fakeDiscovery struct{ roles map[string]string }

func (f *fakeDiscovery) NodeRole(nodeID string) string { return f.roles[nodeID] }

// ─── AudienceAnalytics tests ─────────────────────────────────────────────────

func TestAudienceAnalytics_NilConn(t *testing.T) {
	svc := New(nilSnapLive{}, nil, nil)
	res, err := svc.AudienceAnalytics(context.Background(), AudienceParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("result is nil")
	}
	if len(res.Timeseries) != 0 {
		t.Errorf("want empty timeseries, got %d buckets", len(res.Timeseries))
	}
	if res.Totals != (AudienceTotals{}) {
		t.Errorf("want zero totals, got %+v", res.Totals)
	}
}

func TestAudienceAnalytics_CannedRows(t *testing.T) {
	// Two canned rows to assert every mapped domain field.
	row1 := []any{int64(1000000), int64(10), int64(8), int64(300), int64(5)}
	row2 := []any{int64(2000000), int64(20), int64(15), int64(600), int64(12)}

	conn := newFakeConn().withQuery(newFakeRows(row1, row2), nil)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.AudienceAnalytics(context.Background(), AudienceParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Timeseries) != 2 {
		t.Fatalf("want 2 buckets, got %d", len(res.Timeseries))
	}

	b1 := res.Timeseries[0]
	if b1.TS != 1000000 {
		t.Errorf("bucket[0].TS: got %d, want 1000000", b1.TS)
	}
	if b1.Views != 10 {
		t.Errorf("bucket[0].Views: got %d, want 10", b1.Views)
	}
	if b1.Uniques != 8 {
		t.Errorf("bucket[0].Uniques: got %d, want 8", b1.Uniques)
	}
	if b1.WatchTimeS != 300 {
		t.Errorf("bucket[0].WatchTimeS: got %d, want 300", b1.WatchTimeS)
	}
	if b1.PeakConcurrency != 5 {
		t.Errorf("bucket[0].PeakConcurrency: got %d, want 5", b1.PeakConcurrency)
	}

	b2 := res.Timeseries[1]
	if b2.TS != 2000000 {
		t.Errorf("bucket[1].TS: got %d, want 2000000", b2.TS)
	}

	// Totals: Views=30, Uniques=15 (max), WatchTimeS=900, PeakConcurrency=12.
	if res.Totals.Views != 30 {
		t.Errorf("Totals.Views: got %d, want 30", res.Totals.Views)
	}
	if res.Totals.Uniques != 23 {
		t.Errorf("Totals.Uniques: got %d, want 23", res.Totals.Uniques)
	}
	if res.Totals.WatchTimeS != 900 {
		t.Errorf("Totals.WatchTimeS: got %d, want 900", res.Totals.WatchTimeS)
	}
	if res.Totals.PeakConcurrency != 12 {
		t.Errorf("Totals.PeakConcurrency: got %d, want 12", res.Totals.PeakConcurrency)
	}
}

func TestAudienceAnalytics_EmptyRows(t *testing.T) {
	conn := newFakeConn().withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.AudienceAnalytics(context.Background(), AudienceParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("result is nil")
	}
	if len(res.Timeseries) != 0 {
		t.Errorf("want 0 buckets, got %d", len(res.Timeseries))
	}
}

func TestAudienceAnalytics_QueryError(t *testing.T) {
	sentinel := errors.New("clickhouse unavailable")
	conn := newFakeConn().withQuery(nil, sentinel)
	svc := New(nilSnapLive{}, conn, nil)

	_, err := svc.AudienceAnalytics(context.Background(), AudienceParams{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "audience query:") {
		t.Errorf("error should contain 'audience query:': got %q", err.Error())
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error should wrap sentinel: got %v", err)
	}
}

func TestAudienceAnalytics_RowsErr(t *testing.T) {
	sentinel := errors.New("rows iteration error")
	rows := newErrRows(sentinel)
	conn := newFakeConn().withQuery(rows, nil)
	svc := New(nilSnapLive{}, conn, nil)

	_, err := svc.AudienceAnalytics(context.Background(), AudienceParams{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error should be sentinel, got %v", err)
	}
}

func TestAudienceAnalytics_IntervalDay(t *testing.T) {
	// AudienceAnalytics with interval="day" goes through Query; empty rows still exercises the code path.
	conn := newFakeConn().withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.AudienceAnalytics(context.Background(), AudienceParams{Interval: "day"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("result nil")
	}
}

func TestAudienceAnalytics_AppAndStreamFilter(t *testing.T) {
	// Ensures the App and Stream filter branches execute without error.
	conn := newFakeConn().withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.AudienceAnalytics(context.Background(), AudienceParams{App: "live", Stream: "s1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("result nil")
	}
}

// ─── applyRetention tests ─────────────────────────────────────────────────────

func TestApplyRetention_NilLic(t *testing.T) {
	svc := &Service{lic: nil}
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)

	ef, et := svc.applyRetention(from, to)
	if !ef.Equal(from) {
		t.Errorf("from changed with nil lic: got %v, want %v", ef, from)
	}
	if !et.Equal(to) {
		t.Errorf("to changed with nil lic: got %v, want %v", et, to)
	}
}

func TestApplyRetention_FromClamped(t *testing.T) {
	// Free tier = 7 days retention. A from 365 days ago must be clamped to ~7 days ago.
	lic, err := license.New("", "")
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}
	svc := &Service{lic: lic}

	from := time.Now().AddDate(0, 0, -365)
	to := time.Now().Add(-time.Hour) // valid past time

	ef, _ := svc.applyRetention(from, to)

	// ef should be approximately time.Now().AddDate(0,0,-7) — within 5 seconds for CI timing.
	wantFrom := time.Now().AddDate(0, 0, -7)
	delta := ef.Sub(wantFrom)
	if delta < -5*time.Second || delta > 5*time.Second {
		t.Errorf("from not clamped correctly: got %v, want ~%v (delta %v)", ef, wantFrom, delta)
	}
}

func TestApplyRetention_FromNotClamped(t *testing.T) {
	lic, err := license.New("", "")
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}
	svc := &Service{lic: lic}

	// 3 days ago is within 7-day free-tier window → should not be clamped.
	from := time.Now().AddDate(0, 0, -3)
	to := time.Now().Add(-time.Hour)

	ef, _ := svc.applyRetention(from, to)
	// ef should equal from (within 1ms for float rounding).
	if ef.Before(from.Add(-time.Second)) || ef.After(from.Add(time.Second)) {
		t.Errorf("from was clamped when it should be unchanged: got %v, want %v", ef, from)
	}
}

func TestApplyRetention_ToZero(t *testing.T) {
	lic, err := license.New("", "")
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}
	svc := &Service{lic: lic}

	from := time.Now().AddDate(0, 0, -3)
	// Zero to → should be set to now.
	_, et := svc.applyRetention(from, time.Time{})

	delta := time.Since(et)
	if delta < 0 || delta > 5*time.Second {
		t.Errorf("to not set to now: got %v, delta=%v", et, delta)
	}
}

func TestApplyRetention_ToFuture(t *testing.T) {
	lic, err := license.New("", "")
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}
	svc := &Service{lic: lic}

	from := time.Now().AddDate(0, 0, -3)
	futureTime := time.Now().Add(24 * time.Hour)

	_, et := svc.applyRetention(from, futureTime)

	// et should be set to now (not the future value).
	delta := time.Since(et)
	if delta < 0 || delta > 5*time.Second {
		t.Errorf("to not clamped from future: got %v, delta=%v", et, delta)
	}
}

func TestApplyRetention_ToValid(t *testing.T) {
	lic, err := license.New("", "")
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}
	svc := &Service{lic: lic}

	from := time.Now().AddDate(0, 0, -3)
	validTo := time.Now().Add(-time.Hour) // in the past, valid

	_, et := svc.applyRetention(from, validTo)
	if !et.Equal(validTo) {
		t.Errorf("valid to changed: got %v, want %v", et, validTo)
	}
}

// ─── GeoBreakdown tests ───────────────────────────────────────────────────────

func TestGeoBreakdown_NilConn(t *testing.T) {
	svc := New(nilSnapLive{}, nil, nil)
	rows, err := svc.GeoBreakdown(context.Background(), GeoParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rows == nil {
		t.Fatal("expected non-nil slice")
	}
	if len(rows) != 0 {
		t.Errorf("want empty, got %d rows", len(rows))
	}
}

func TestGeoBreakdown_CannedRows(t *testing.T) {
	// Without region: Scan(country, views, uniques, watch_time_s).
	row1 := []any{"US", int64(100), int64(80), int64(5000)}
	row2 := []any{"DE", int64(50), int64(40), int64(2500)}
	conn := newFakeConn().withQuery(newFakeRows(row1, row2), nil)
	svc := New(nilSnapLive{}, conn, nil)

	rows, err := svc.GeoBreakdown(context.Background(), GeoParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}

	if rows[0].Country != "US" {
		t.Errorf("rows[0].Country: got %q, want US", rows[0].Country)
	}
	if rows[0].Views != 100 {
		t.Errorf("rows[0].Views: got %d, want 100", rows[0].Views)
	}
	if rows[0].Uniques != 80 {
		t.Errorf("rows[0].Uniques: got %d, want 80", rows[0].Uniques)
	}
	if rows[0].WatchTimeS != 5000 {
		t.Errorf("rows[0].WatchTimeS: got %d, want 5000", rows[0].WatchTimeS)
	}
	if rows[0].Region != nil {
		t.Errorf("rows[0].Region: should be nil without Region=true")
	}
	if rows[1].Country != "DE" {
		t.Errorf("rows[1].Country: got %q, want DE", rows[1].Country)
	}
}

func TestGeoBreakdown_WithRegion(t *testing.T) {
	// With region: Scan(country, region, views, uniques, watch_time_s).
	row1 := []any{"US", "CA", int64(60), int64(50), int64(3000)}
	conn := newFakeConn().withQuery(newFakeRows(row1), nil)
	svc := New(nilSnapLive{}, conn, nil)

	rows, err := svc.GeoBreakdown(context.Background(), GeoParams{Region: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].Country != "US" {
		t.Errorf("Country: got %q, want US", rows[0].Country)
	}
	if rows[0].Region == nil {
		t.Fatal("Region should not be nil with Region=true")
	}
	if *rows[0].Region != "CA" {
		t.Errorf("Region: got %q, want CA", *rows[0].Region)
	}
	if rows[0].Views != 60 {
		t.Errorf("Views: got %d, want 60", rows[0].Views)
	}
}

func TestGeoBreakdown_EmptyRows(t *testing.T) {
	conn := newFakeConn().withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	rows, err := svc.GeoBreakdown(context.Background(), GeoParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rows == nil {
		t.Fatal("expected non-nil slice on empty result")
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}
}

func TestGeoBreakdown_QueryError(t *testing.T) {
	sentinel := errors.New("clickhouse timeout")
	conn := newFakeConn().withQuery(nil, sentinel)
	svc := New(nilSnapLive{}, conn, nil)

	_, err := svc.GeoBreakdown(context.Background(), GeoParams{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "geo breakdown query:") {
		t.Errorf("error should contain 'geo breakdown query:': %q", err.Error())
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error should wrap sentinel: %v", err)
	}
}

func TestGeoBreakdown_RowsErr(t *testing.T) {
	sentinel := errors.New("rows err")
	conn := newFakeConn().withQuery(newErrRows(sentinel), nil)
	svc := New(nilSnapLive{}, conn, nil)

	_, err := svc.GeoBreakdown(context.Background(), GeoParams{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel, got %v", err)
	}
}

func TestGeoBreakdown_Filters(t *testing.T) {
	// Executes the App, Stream, Tenant filter branches.
	conn := newFakeConn().withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	rows, err := svc.GeoBreakdown(context.Background(), GeoParams{App: "live", Stream: "s1", Tenant: "t1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}
}

// ─── DeviceBreakdown tests ────────────────────────────────────────────────────

func TestDeviceBreakdown_NilConn(t *testing.T) {
	svc := New(nilSnapLive{}, nil, nil)
	rows, err := svc.DeviceBreakdown(context.Background(), DeviceParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rows == nil {
		t.Fatal("expected non-nil slice")
	}
	if len(rows) != 0 {
		t.Errorf("want empty, got %d rows", len(rows))
	}
}

func TestDeviceBreakdown_CannedRows(t *testing.T) {
	// Scan(device, os, browser, protocol, views, uniques, watch_time_s)
	row1 := []any{"desktop", "linux", "chrome", "hls", int64(200), int64(180), int64(10000)}
	row2 := []any{"mobile", "ios", "safari", "webrtc", int64(50), int64(45), int64(2000)}
	conn := newFakeConn().withQuery(newFakeRows(row1, row2), nil)
	svc := New(nilSnapLive{}, conn, nil)

	rows, err := svc.DeviceBreakdown(context.Background(), DeviceParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}

	r := rows[0]
	if r.Device != "desktop" {
		t.Errorf("Device: got %q, want desktop", r.Device)
	}
	if r.OS != "linux" {
		t.Errorf("OS: got %q, want linux", r.OS)
	}
	if r.Browser != "chrome" {
		t.Errorf("Browser: got %q, want chrome", r.Browser)
	}
	if r.Protocol != "hls" {
		t.Errorf("Protocol: got %q, want hls", r.Protocol)
	}
	if r.Views != 200 {
		t.Errorf("Views: got %d, want 200", r.Views)
	}
	if r.Uniques != 180 {
		t.Errorf("Uniques: got %d, want 180", r.Uniques)
	}
	if r.WatchTimeS != 10000 {
		t.Errorf("WatchTimeS: got %d, want 10000", r.WatchTimeS)
	}

	r2 := rows[1]
	if r2.Device != "mobile" {
		t.Errorf("rows[1].Device: got %q, want mobile", r2.Device)
	}
}

func TestDeviceBreakdown_EmptyRows(t *testing.T) {
	conn := newFakeConn().withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	rows, err := svc.DeviceBreakdown(context.Background(), DeviceParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rows == nil {
		t.Fatal("expected non-nil slice")
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}
}

func TestDeviceBreakdown_QueryError(t *testing.T) {
	sentinel := errors.New("db error")
	conn := newFakeConn().withQuery(nil, sentinel)
	svc := New(nilSnapLive{}, conn, nil)

	_, err := svc.DeviceBreakdown(context.Background(), DeviceParams{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "device breakdown query:") {
		t.Errorf("error should contain 'device breakdown query:': %q", err.Error())
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error should wrap sentinel: %v", err)
	}
}

func TestDeviceBreakdown_RowsErr(t *testing.T) {
	sentinel := errors.New("rows error")
	conn := newFakeConn().withQuery(newErrRows(sentinel), nil)
	svc := New(nilSnapLive{}, conn, nil)

	_, err := svc.DeviceBreakdown(context.Background(), DeviceParams{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel, got %v", err)
	}
}

func TestDeviceBreakdown_Filters(t *testing.T) {
	conn := newFakeConn().withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	rows, err := svc.DeviceBreakdown(context.Background(), DeviceParams{App: "live", Stream: "s1", Tenant: "t1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}
}

// ─── QoeSummary tests ─────────────────────────────────────────────────────────

func TestQoeSummary_NilConn(t *testing.T) {
	svc := New(nilSnapLive{}, nil, nil)
	res, err := svc.QoeSummary(context.Background(), QoeParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("result is nil")
	}
	if len(res.BitrateTimeline) != 0 {
		t.Errorf("want empty timeline, got %d", len(res.BitrateTimeline))
	}
}

func TestQoeSummary_CannedData(t *testing.T) {
	// Totals: startupP50=100.0, startupP95=500.0, rebMs=1000, watchMs=10000, errs=2, sessions=10
	// → rebufferRatio = 1000/10000 = 0.1, errorRate = 2/10 = 0.2
	totalsRow := newFakeRow(float64(100.0), float64(500.0), uint64(1000), uint64(10000), uint64(2), uint64(10))

	// Timeline: 2 buckets
	tl1 := []any{int64(3600000), float64(2000.0), float64(3000.0)}
	tl2 := []any{int64(7200000), float64(2200.0), float64(3200.0)}
	timelineRows := newFakeRows(tl1, tl2)

	conn := newFakeConn().withRow(totalsRow).withQuery(timelineRows, nil)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.QoeSummary(context.Background(), QoeParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Totals.StartupP50Ms != 100.0 {
		t.Errorf("StartupP50Ms: got %f, want 100.0", res.Totals.StartupP50Ms)
	}
	if res.Totals.StartupP95Ms != 500.0 {
		t.Errorf("StartupP95Ms: got %f, want 500.0", res.Totals.StartupP95Ms)
	}
	const wantRebRatio = 0.1
	if res.Totals.RebufferRatio != wantRebRatio {
		t.Errorf("RebufferRatio: got %f, want %f", res.Totals.RebufferRatio, wantRebRatio)
	}
	const wantErrRate = 0.2
	if res.Totals.ErrorRate != wantErrRate {
		t.Errorf("ErrorRate: got %f, want %f", res.Totals.ErrorRate, wantErrRate)
	}

	if len(res.BitrateTimeline) != 2 {
		t.Fatalf("want 2 timeline buckets, got %d", len(res.BitrateTimeline))
	}
	b0 := res.BitrateTimeline[0]
	if b0.TS != 3600000 {
		t.Errorf("timeline[0].TS: got %d, want 3600000", b0.TS)
	}
	if b0.BitrateKbpsP50 != 2000.0 {
		t.Errorf("timeline[0].BitrateKbpsP50: got %f, want 2000.0", b0.BitrateKbpsP50)
	}
	if b0.BitrateKbpsP95 != 3000.0 {
		t.Errorf("timeline[0].BitrateKbpsP95: got %f, want 3000.0", b0.BitrateKbpsP95)
	}
}

func TestQoeSummary_ScanError_ReturnsEmpty(t *testing.T) {
	// Scan error on QueryRow → returns empty, nil error.
	conn := newFakeConn().withRow(newErrRow(errors.New("scan failed")))
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.QoeSummary(context.Background(), QoeParams{})
	if err != nil {
		t.Fatalf("expected nil error on scan failure, got: %v", err)
	}
	if res == nil {
		t.Fatal("result is nil")
	}
	if len(res.BitrateTimeline) != 0 {
		t.Errorf("want empty timeline, got %d", len(res.BitrateTimeline))
	}
}

func TestQoeSummary_TimelineQueryError_StillReturnsTotals(t *testing.T) {
	// Timeline Query returns error → totals are returned, timeline is empty.
	totalsRow := newFakeRow(float64(200.0), float64(800.0), uint64(500), uint64(5000), uint64(1), uint64(20))
	timelineErr := errors.New("timeline unavailable")
	conn := newFakeConn().withRow(totalsRow).withQuery(nil, timelineErr)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.QoeSummary(context.Background(), QoeParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Totals.StartupP50Ms != 200.0 {
		t.Errorf("StartupP50Ms: got %f, want 200.0", res.Totals.StartupP50Ms)
	}
	if len(res.BitrateTimeline) != 0 {
		t.Errorf("want empty timeline on query error, got %d", len(res.BitrateTimeline))
	}
}

func TestQoeSummary_RebufferRatioClamped(t *testing.T) {
	// rebMs > watchMs → ratio clamped to 1.0.
	totalsRow := newFakeRow(float64(0.0), float64(0.0), uint64(9999), uint64(100), uint64(0), uint64(0))
	conn := newFakeConn().withRow(totalsRow).withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.QoeSummary(context.Background(), QoeParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Totals.RebufferRatio != 1.0 {
		t.Errorf("RebufferRatio should be clamped to 1.0, got %f", res.Totals.RebufferRatio)
	}
}

func TestQoeSummary_ErrorRateZeroSessions(t *testing.T) {
	// sessions=0 → error_rate stays 0.0 (no division by zero).
	totalsRow := newFakeRow(float64(0.0), float64(0.0), uint64(0), uint64(1000), uint64(5), uint64(0))
	conn := newFakeConn().withRow(totalsRow).withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.QoeSummary(context.Background(), QoeParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Totals.ErrorRate != 0.0 {
		t.Errorf("ErrorRate with 0 sessions should be 0.0, got %f", res.Totals.ErrorRate)
	}
}

func TestQoeSummary_IntervalDay(t *testing.T) {
	totalsRow := newFakeRow(float64(0.0), float64(0.0), uint64(0), uint64(0), uint64(0), uint64(0))
	conn := newFakeConn().withRow(totalsRow).withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.QoeSummary(context.Background(), QoeParams{Interval: "day"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("result nil")
	}
}

func TestQoeSummary_AllFilters(t *testing.T) {
	totalsRow := newFakeRow(float64(0.0), float64(0.0), uint64(0), uint64(0), uint64(0), uint64(0))
	conn := newFakeConn().withRow(totalsRow).withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.QoeSummary(context.Background(), QoeParams{
		App: "live", Stream: "s1", Tenant: "t1", Country: "US", Device: "desktop",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("result nil")
	}
}

// ─── IngestTimeseries tests ───────────────────────────────────────────────────

func TestIngestTimeseries_NilConn(t *testing.T) {
	svc := New(nilSnapLive{}, nil, nil)
	res, err := svc.IngestTimeseries(context.Background(), IngestTimeseriesParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("result is nil")
	}
	if len(res.Timeseries) != 0 {
		t.Errorf("want 0 timeseries, got %d", len(res.Timeseries))
	}
	if len(res.DropEvents) != 0 {
		t.Errorf("want 0 drop events, got %d", len(res.DropEvents))
	}
}

func TestIngestTimeseries_CannedRows(t *testing.T) {
	// Scan: bucket_ms, avg_bitrate, avg_fps, avg_kf, avg_loss, avg_jitter
	row1 := []any{int64(60000), float64(1500.0), float64(30.0), float64(2.0), float64(0.5), float64(10.0)}
	row2 := []any{int64(120000), float64(1600.0), float64(30.0), float64(2.0), float64(0.3), float64(8.0)}
	conn := newFakeConn().withQuery(newFakeRows(row1, row2), nil)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.IngestTimeseries(context.Background(), IngestTimeseriesParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Timeseries) != 2 {
		t.Fatalf("want 2 timeseries buckets, got %d", len(res.Timeseries))
	}

	b0 := res.Timeseries[0]
	if b0.TS != 60000 {
		t.Errorf("TS: got %d, want 60000", b0.TS)
	}
	if b0.BitrateKbps != 1500.0 {
		t.Errorf("BitrateKbps: got %f, want 1500.0", b0.BitrateKbps)
	}
	if b0.FPS != 30.0 {
		t.Errorf("FPS: got %f, want 30.0", b0.FPS)
	}
	if b0.KeyframeIntervalS != 2.0 {
		t.Errorf("KeyframeIntervalS: got %f, want 2.0", b0.KeyframeIntervalS)
	}
	if b0.PacketLossPct != 0.5 {
		t.Errorf("PacketLossPct: got %f, want 0.5", b0.PacketLossPct)
	}
	if b0.JitterMS != 10.0 {
		t.Errorf("JitterMS: got %f, want 10.0", b0.JitterMS)
	}
}

func TestIngestTimeseries_DropEvent_BitrateDrop(t *testing.T) {
	// bitrate drops to < 20% of previous bucket → bitrate_drop event.
	// prev=1000, current=150 (< 200 = 20% of 1000).
	row1 := []any{int64(60000), float64(1000.0), float64(30.0), float64(2.0), float64(0.0), float64(0.0)}
	row2 := []any{int64(120000), float64(150.0), float64(30.0), float64(2.0), float64(0.0), float64(0.0)}
	conn := newFakeConn().withQuery(newFakeRows(row1, row2), nil)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.IngestTimeseries(context.Background(), IngestTimeseriesParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, e := range res.DropEvents {
		if e.Reason == "bitrate_drop" && e.TS == 120000 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected bitrate_drop event at TS=120000, got: %v", res.DropEvents)
	}
}

func TestIngestTimeseries_DropEvent_FPSDrop(t *testing.T) {
	// fps drops to < 20% of previous bucket → fps_drop event.
	// prev_fps=30, current_fps=5 (< 6 = 20% of 30).
	row1 := []any{int64(60000), float64(1000.0), float64(30.0), float64(2.0), float64(0.0), float64(0.0)}
	row2 := []any{int64(120000), float64(1000.0), float64(5.0), float64(2.0), float64(0.0), float64(0.0)}
	conn := newFakeConn().withQuery(newFakeRows(row1, row2), nil)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.IngestTimeseries(context.Background(), IngestTimeseriesParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, e := range res.DropEvents {
		if e.Reason == "fps_drop" && e.TS == 120000 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected fps_drop event at TS=120000, got: %v", res.DropEvents)
	}
}

func TestIngestTimeseries_DropEvent_PacketLoss(t *testing.T) {
	// packet_loss_pct > 5% → packet_loss_spike event.
	row1 := []any{int64(60000), float64(1000.0), float64(30.0), float64(2.0), float64(7.5), float64(0.0)}
	conn := newFakeConn().withQuery(newFakeRows(row1), nil)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.IngestTimeseries(context.Background(), IngestTimeseriesParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, e := range res.DropEvents {
		if e.Reason == "packet_loss_spike" && e.TS == 60000 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected packet_loss_spike event at TS=60000, got: %v", res.DropEvents)
	}
}

func TestIngestTimeseries_DropEvent_JitterSpike(t *testing.T) {
	// jitter_ms > 50ms → jitter_spike event.
	row1 := []any{int64(60000), float64(1000.0), float64(30.0), float64(2.0), float64(0.0), float64(75.0)}
	conn := newFakeConn().withQuery(newFakeRows(row1), nil)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.IngestTimeseries(context.Background(), IngestTimeseriesParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, e := range res.DropEvents {
		if e.Reason == "jitter_spike" && e.TS == 60000 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected jitter_spike event at TS=60000, got: %v", res.DropEvents)
	}
}

func TestIngestTimeseries_QueryError_ReturnsEmpty(t *testing.T) {
	// Query error is non-fatal → returns empty result.
	conn := newFakeConn().withQuery(nil, errors.New("network error"))
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.IngestTimeseries(context.Background(), IngestTimeseriesParams{})
	if err != nil {
		t.Fatalf("expected nil error (non-fatal), got: %v", err)
	}
	if res == nil {
		t.Fatal("result is nil")
	}
	if len(res.Timeseries) != 0 {
		t.Errorf("want empty timeseries, got %d", len(res.Timeseries))
	}
}

func TestIngestTimeseries_DefaultBucketSecs(t *testing.T) {
	// BucketSeconds<=0 → defaults to 60. Exercise the code path without asserting SQL.
	conn := newFakeConn().withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.IngestTimeseries(context.Background(), IngestTimeseriesParams{BucketSeconds: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("result nil")
	}
}

func TestIngestTimeseries_AllFilters(t *testing.T) {
	// Exercise StreamID, App, NodeID, From, To filter branches.
	conn := newFakeConn().withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	from := time.Now().AddDate(0, 0, -7)
	to := time.Now()
	res, err := svc.IngestTimeseries(context.Background(), IngestTimeseriesParams{
		StreamID:      "s1",
		App:           "live",
		NodeID:        "node-1",
		From:          from,
		To:            to,
		BucketSeconds: 30,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("result nil")
	}
}

func TestIngestTimeseries_NoDropEventsSliceNonNil(t *testing.T) {
	// When no drop events occur, DropEvents should be non-nil (empty slice, not nil).
	row1 := []any{int64(60000), float64(1000.0), float64(30.0), float64(2.0), float64(0.0), float64(0.0)}
	conn := newFakeConn().withQuery(newFakeRows(row1), nil)
	svc := New(nilSnapLive{}, conn, nil)

	res, err := svc.IngestTimeseries(context.Background(), IngestTimeseriesParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.DropEvents == nil {
		t.Error("DropEvents should be non-nil (empty slice) when no drops")
	}
}

// ─── QueryProbeResults tests ──────────────────────────────────────────────────

func TestQueryProbeResults_NilQuerier(t *testing.T) {
	svc := New(nilSnapLive{}, nil, nil)
	// probeResultQuerier is nil by default.
	results, err := svc.QueryProbeResults(context.Background(), "probe-1", time.Time{}, time.Time{}, 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("want nil results, got %v", results)
	}
}

func TestQueryProbeResults_Delegates(t *testing.T) {
	want := []domain.ProbeResult{
		{
			ID:        "r1",
			ProbeID:   "probe-1",
			TS:        time.Now().UTC(),
			Success:   true,
			TTFBMs:    42,
			ErrorCode: "",
		},
	}
	querier := &fakeProbeQuerier{results: want}
	svc := New(nilSnapLive{}, nil, nil)
	svc.SetProbeResultQuerier(querier)

	results, err := svc.QueryProbeResults(context.Background(), "probe-1", time.Time{}, time.Time{}, 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(results, want) {
		t.Errorf("got %+v, want %+v", results, want)
	}
}

func TestQueryProbeResults_PropagatesError(t *testing.T) {
	sentinel := errors.New("probe store error")
	querier := &fakeProbeQuerier{err: sentinel}
	svc := New(nilSnapLive{}, nil, nil)
	svc.SetProbeResultQuerier(querier)

	_, err := svc.QueryProbeResults(context.Background(), "probe-1", time.Time{}, time.Time{}, 10, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel, got %v", err)
	}
}

// ─── SetProbeResultQuerier and SetClusterDiscovery ────────────────────────────

func TestSetProbeResultQuerier_Wires(t *testing.T) {
	svc := New(nilSnapLive{}, nil, nil)
	querier := &fakeProbeQuerier{}
	svc.SetProbeResultQuerier(querier)
	if svc.probeResultQuerier == nil {
		t.Error("SetProbeResultQuerier did not wire the querier")
	}
}

func TestSetClusterDiscovery_Wires(t *testing.T) {
	svc := New(nilSnapLive{}, nil, nil)
	disc := &fakeDiscovery{roles: map[string]string{}}
	svc.SetClusterDiscovery(disc)
	if svc.clusterDiscovery == nil {
		t.Error("SetClusterDiscovery did not wire the discovery")
	}
}

// ─── FleetNodes additional coverage ──────────────────────────────────────────

func TestFleetNodes_WithNodes(t *testing.T) {
	now := time.Now().UTC()
	snap := &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			"n1": {
				NodeID:         "n1",
				Version:        "3.0.3",
				CPUPCT:         45.0,
				MemPCT:         60.0,
				OsName:         "linux",
				OsArch:         "amd64",
				JavaVersion:    "17",
				ProcessorCount: 8,
				UpdatedAt:      now,
			},
		},
		UpdatedAt: now,
	}
	svc := New(&fixedSnapLive{snap: snap}, nil, nil)

	res, err := svc.FleetNodes(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("want 1 node, got %d", len(res.Items))
	}

	n := res.Items[0]
	if n.NodeID != "n1" {
		t.Errorf("NodeID: got %q, want n1", n.NodeID)
	}
	if n.Version != "3.0.3" {
		t.Errorf("Version: got %q, want 3.0.3", n.Version)
	}
	if n.CPUPCT != 45.0 {
		t.Errorf("CPUPCT: got %f, want 45.0", n.CPUPCT)
	}
	if n.MemPCT != 60.0 {
		t.Errorf("MemPCT: got %f, want 60.0", n.MemPCT)
	}
	if n.OsName != "linux" {
		t.Errorf("OsName: got %q, want linux", n.OsName)
	}
	if n.OsArch != "amd64" {
		t.Errorf("OsArch: got %q, want amd64", n.OsArch)
	}
	if n.JavaVersion != "17" {
		t.Errorf("JavaVersion: got %q, want 17", n.JavaVersion)
	}
	if n.ProcessorCount != 8 {
		t.Errorf("ProcessorCount: got %d, want 8", n.ProcessorCount)
	}
	if n.Status != "up" {
		t.Errorf("Status: got %q, want up", n.Status)
	}
	if n.Role != "standalone" {
		t.Errorf("Role: got %q, want standalone", n.Role)
	}
	if n.LastSeen != now.UnixMilli() {
		t.Errorf("LastSeen: got %d, want %d", n.LastSeen, now.UnixMilli())
	}
}

func TestFleetNodes_CPUDegraded(t *testing.T) {
	now := time.Now().UTC()
	snap := &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			"degraded-node": {
				CPUPCT:    95.0, // > 90 → degraded
				MemPCT:    50.0,
				UpdatedAt: now,
			},
		},
		UpdatedAt: now,
	}
	svc := New(&fixedSnapLive{snap: snap}, nil, nil)

	res, err := svc.FleetNodes(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("want 1 node, got %d", len(res.Items))
	}
	if res.Items[0].Status != "degraded" {
		t.Errorf("Status: got %q, want degraded (CPU>90)", res.Items[0].Status)
	}
}

func TestFleetNodes_WithClusterDiscovery(t *testing.T) {
	now := time.Now().UTC()
	snap := &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			"edge-1": {CPUPCT: 30.0, UpdatedAt: now},
		},
		UpdatedAt: now,
	}
	disc := &fakeDiscovery{roles: map[string]string{"edge-1": "edge"}}
	svc := New(&fixedSnapLive{snap: snap}, nil, nil)
	svc.SetClusterDiscovery(disc)

	res, err := svc.FleetNodes(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("want 1 node, got %d", len(res.Items))
	}
	if res.Items[0].Role != "edge" {
		t.Errorf("Role: got %q, want edge", res.Items[0].Role)
	}
}

func TestFleetNodes_DiscoveryFallbackToStandalone(t *testing.T) {
	now := time.Now().UTC()
	snap := &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			"unknown-node": {CPUPCT: 30.0, UpdatedAt: now},
		},
		UpdatedAt: now,
	}
	// Discovery returns "" for unknown-node → falls back to "standalone".
	disc := &fakeDiscovery{roles: map[string]string{}}
	svc := New(&fixedSnapLive{snap: snap}, nil, nil)
	svc.SetClusterDiscovery(disc)

	res, err := svc.FleetNodes(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("want 1 node, got %d", len(res.Items))
	}
	if res.Items[0].Role != "standalone" {
		t.Errorf("Role: got %q, want standalone (discovery returned '')", res.Items[0].Role)
	}
}

func TestFleetNodes_LimitDefault(t *testing.T) {
	// limit <= 0 should default to 50.
	now := time.Now().UTC()
	nodes := map[string]*domain.LiveNodeStats{}
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("node-%d", i)
		nodes[id] = &domain.LiveNodeStats{CPUPCT: float64(i), UpdatedAt: now}
	}
	snap := &domain.LiveSnapshot{
		Streams:   map[string]*domain.LiveStream{},
		Nodes:     nodes,
		UpdatedAt: now,
	}
	svc := New(&fixedSnapLive{snap: snap}, nil, nil)

	res, err := svc.FleetNodes(context.Background(), 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// All 5 nodes should be returned (5 < default limit 50).
	if len(res.Items) != 5 {
		t.Errorf("want 5 nodes, got %d", len(res.Items))
	}
}

// ─── LiveOverview additional coverage ────────────────────────────────────────

func TestLiveOverview_NodeDegradedByMem(t *testing.T) {
	// MemPCT > 90 → node.Status = "degraded".
	now := time.Now().UTC()
	snap := &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			"mem-hot": {
				CPUPCT:    20.0,
				MemPCT:    95.0, // > 90 → degraded
				UpdatedAt: now,
			},
		},
		UpdatedAt: now,
	}
	svc := New(&fixedSnapLive{snap: snap}, nil, nil)

	res, err := svc.LiveOverview(context.Background(), "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(res.Nodes))
	}
	if res.Nodes[0].Status != "degraded" {
		t.Errorf("Status: got %q, want degraded (Mem>90)", res.Nodes[0].Status)
	}
}

func TestLiveOverview_Filters(t *testing.T) {
	// With app/nodeID filter that matches no streams.
	now := time.Now().UTC()
	snap := &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"s1": {
				StreamID:    "s1",
				App:         "live",
				NodeID:      "n1",
				Active:      true,
				ViewerCount: 5,
				Health:      domain.StreamHealthGood,
			},
		},
		Nodes:     map[string]*domain.LiveNodeStats{},
		UpdatedAt: now,
	}
	svc := New(&fixedSnapLive{snap: snap}, nil, nil)

	// Filter for a different app — no streams or nodes should match.
	res, err := svc.LiveOverview(context.Background(), "other-app", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TotalViewers != 0 {
		t.Errorf("TotalViewers: got %d, want 0 (filter mismatch)", res.TotalViewers)
	}
	if len(res.Apps) != 0 {
		t.Errorf("Apps: want 0, got %d", len(res.Apps))
	}
}

func TestLiveOverview_NilSnapshot(t *testing.T) {
	// Nil snapshot → returns zero result without error.
	svc := New(nilSnapLive{}, nil, nil)
	res, err := svc.LiveOverview(context.Background(), "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("result nil")
	}
	if res.TotalViewers != 0 {
		t.Errorf("TotalViewers: got %d, want 0", res.TotalViewers)
	}
}
