// Package query — S37 (D-099) retention-enforcement tests.
//
// Regression guard for the S37 entitlement audit: GeoBreakdown, DeviceBreakdown,
// QoeSummary and IngestTimeseries did NOT clamp their time range to the license
// retention window, so a Free-tier caller (7-day retention) could read arbitrarily
// far into the past by passing an old `from`. AudienceAnalytics already clamped;
// these four now call s.applyRetention too.
//
// Each test passes `from = 365 days ago` on a Free-tier license and asserts the
// time value that actually reached the query WHERE clause was clamped to ~now-7d.
// Mutation proof: removing the `p.From, p.To = s.applyRetention(...)` line in the
// method under test makes the captured `from` the un-clamped 365-days-ago value,
// which fails the assertion (RED).
package query

import (
	"context"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/license"
)

// firstTimeArg returns the first time.Time found across all captured query args,
// which for every method under test is the (clamped) `from` bound.
func firstTimeArg(t *testing.T, conn *fakeConn) time.Time {
	t.Helper()
	for _, call := range conn.capturedArgs {
		for _, a := range call {
			if tv, ok := a.(time.Time); ok {
				return tv
			}
		}
	}
	t.Fatalf("no time.Time arg captured (calls=%d)", len(conn.capturedArgs))
	return time.Time{}
}

// assertClampedToFreeRetention checks that `got` is the ~now-7d Free-tier
// retention horizon, not the far-past `from` the caller supplied.
func assertClampedToFreeRetention(t *testing.T, got time.Time) {
	t.Helper()
	want := time.Now().AddDate(0, 0, -7) // Free tier = 7-day retention
	delta := got.Sub(want)
	if delta < -time.Hour || delta > time.Hour {
		t.Errorf("from not clamped to retention window: got %v, want ~%v (delta %v)", got, want, delta)
	}
}

func freeSvc(t *testing.T, conn *fakeConn) *Service {
	t.Helper()
	lic, err := license.New("", "") // Free tier
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}
	return New(nilSnapLive{}, conn, lic)
}

func TestGeoBreakdown_ClampsRetention(t *testing.T) {
	conn := newFakeConn()
	svc := freeSvc(t, conn)
	from := time.Now().AddDate(0, 0, -365)
	to := time.Now().Add(-time.Hour)

	if _, err := svc.GeoBreakdown(context.Background(), GeoParams{From: from, To: to}); err != nil {
		t.Fatalf("GeoBreakdown: %v", err)
	}
	assertClampedToFreeRetention(t, firstTimeArg(t, conn))
}

func TestDeviceBreakdown_ClampsRetention(t *testing.T) {
	conn := newFakeConn()
	svc := freeSvc(t, conn)
	from := time.Now().AddDate(0, 0, -365)
	to := time.Now().Add(-time.Hour)

	if _, err := svc.DeviceBreakdown(context.Background(), DeviceParams{From: from, To: to}); err != nil {
		t.Fatalf("DeviceBreakdown: %v", err)
	}
	assertClampedToFreeRetention(t, firstTimeArg(t, conn))
}

func TestQoeSummary_ClampsRetention(t *testing.T) {
	conn := newFakeConn()
	svc := freeSvc(t, conn)
	from := time.Now().AddDate(0, 0, -365)
	to := time.Now().Add(-time.Hour)

	// QoeSummary runs a QueryRow (totals) then Query (timeline); we assert on the
	// captured args regardless of the (unqueued) row Scan error.
	_, _ = svc.QoeSummary(context.Background(), QoeParams{From: from, To: to})
	assertClampedToFreeRetention(t, firstTimeArg(t, conn))
}

func TestIngestTimeseries_ClampsRetention(t *testing.T) {
	conn := newFakeConn()
	svc := freeSvc(t, conn)
	from := time.Now().AddDate(0, 0, -365)
	to := time.Now().Add(-time.Hour)

	if _, err := svc.IngestTimeseries(context.Background(), IngestTimeseriesParams{From: from, To: to}); err != nil {
		t.Fatalf("IngestTimeseries: %v", err)
	}
	assertClampedToFreeRetention(t, firstTimeArg(t, conn))
}

// TestQueryProbeResults_ClampsRetention guards the gap the S37 adversarial review
// caught: QueryProbeResults forwarded the caller's from/to straight to the store,
// so a Free tenant could read probe history past its 7-day retention horizon by
// passing an old ?from=. Mutation proof: removing the applyRetention line makes the
// captured from the un-clamped 365-days-ago value.
func TestQueryProbeResults_ClampsRetention(t *testing.T) {
	lic, err := license.New("", "") // Free tier
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}
	svc := New(nilSnapLive{}, nil, lic)
	pq := &fakeProbeQuerier{}
	svc.SetProbeResultQuerier(pq)

	from := time.Now().AddDate(0, 0, -365)
	to := time.Now().Add(-time.Hour)
	if _, err := svc.QueryProbeResults(context.Background(), "probe-1", from, to, 100, ""); err != nil {
		t.Fatalf("QueryProbeResults: %v", err)
	}
	assertClampedToFreeRetention(t, pq.gotFrom)
}

// TestIngestTimeseries_UnboundedRequestClamped guards the specific IngestTimeseries
// behaviour: an unbounded request (zero From/To) previously applied NO time filter
// at all, letting Free tier scan all history. applyRetention now fills the range
// with [now-7d, now]. If the clamp is removed, zero From/To means no time.Time
// args are captured and firstTimeArg fails.
func TestIngestTimeseries_UnboundedRequestClamped(t *testing.T) {
	conn := newFakeConn()
	svc := freeSvc(t, conn)

	if _, err := svc.IngestTimeseries(context.Background(), IngestTimeseriesParams{}); err != nil {
		t.Fatalf("IngestTimeseries: %v", err)
	}
	assertClampedToFreeRetention(t, firstTimeArg(t, conn))
}
