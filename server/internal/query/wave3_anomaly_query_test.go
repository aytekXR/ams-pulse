package query

// wave3_anomaly_query_test.go — TDD tests for AnomalyBaselineForMetric (S11 WO-B).
// Layer 3: mock-Conn unit tests. Written before implementation (RED phase).

import (
	"context"
	"math"
	"testing"
)

// ─── TestAnomalyBaselineForMetric_NilConn ────────────────────────────────────

// When conn is nil, returns (0, 0, 0, nil) — no panic, no error.
func TestAnomalyBaselineForMetric_NilConn(t *testing.T) {
	svc := New(nilSnapLive{}, nil, nil)
	mean, stddev, n, err := svc.AnomalyBaselineForMetric(context.Background(), "viewer_count", "", 3600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mean != 0 || stddev != 0 || n != 0 {
		t.Errorf("nil conn: expected (0,0,0), got (%g,%g,%d)", mean, stddev, n)
	}
}

// ─── TestAnomalyBaselineForMetric_ViewerCount ─────────────────────────────────

// For viewer_count, uses server_events path.
// Mock conn returns mean=42.5, stddev=5.0, n=100.
func TestAnomalyBaselineForMetric_ViewerCount(t *testing.T) {
	conn := newFakeConn().withRow(newFakeRow(float64(42.5), float64(5.0), int64(100)))
	svc := New(nilSnapLive{}, conn, nil)

	mean, stddev, n, err := svc.AnomalyBaselineForMetric(context.Background(), "viewer_count", "", 3600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mean != 42.5 {
		t.Errorf("mean: got %g, want 42.5", mean)
	}
	if stddev != 5.0 {
		t.Errorf("stddev: got %g, want 5.0", stddev)
	}
	if n != 100 {
		t.Errorf("n: got %d, want 100", n)
	}
}

// ─── TestAnomalyBaselineForMetric_RebufferRatio ──────────────────────────────

// For rebuffer_ratio, uses rollup_qoe_1h path.
func TestAnomalyBaselineForMetric_RebufferRatio(t *testing.T) {
	conn := newFakeConn().withRow(newFakeRow(float64(0.05), float64(0.01), int64(50)))
	svc := New(nilSnapLive{}, conn, nil)

	mean, stddev, n, err := svc.AnomalyBaselineForMetric(context.Background(), "rebuffer_ratio", "stream-1", 3600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mean != 0.05 {
		t.Errorf("mean: got %g, want 0.05", mean)
	}
	if stddev != 0.01 {
		t.Errorf("stddev: got %g, want 0.01", stddev)
	}
	if n != 50 {
		t.Errorf("n: got %d, want 50", n)
	}
}

// ─── TestAnomalyBaselineForMetric_EmptyResult ─────────────────────────────────

// Scan error (e.g., no data) returns (0, 0, 0, nil) — not an error to the caller.
func TestAnomalyBaselineForMetric_EmptyResult(t *testing.T) {
	conn := newFakeConn().withRow(newErrRow(errNoData))
	svc := New(nilSnapLive{}, conn, nil)

	mean, stddev, n, err := svc.AnomalyBaselineForMetric(context.Background(), "viewer_count", "", 3600)
	if err != nil {
		t.Fatalf("expected nil error on scan failure, got: %v", err)
	}
	if mean != 0 || stddev != 0 || n != 0 {
		t.Errorf("expected (0,0,0) on scan error, got (%g,%g,%d)", mean, stddev, n)
	}
}

// ─── TestAnomalyBaselineForMetric_NaNSanitized ───────────────────────────────

// NaN results are sanitized to 0 via jsonSafeFloat pattern.
func TestAnomalyBaselineForMetric_NaNSanitized(t *testing.T) {
	conn := newFakeConn().withRow(newFakeRow(math.NaN(), math.NaN(), int64(5)))
	svc := New(nilSnapLive{}, conn, nil)

	mean, stddev, n, err := svc.AnomalyBaselineForMetric(context.Background(), "viewer_count", "", 3600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.IsNaN(mean) {
		t.Error("mean should not be NaN after sanitization")
	}
	if mean != 0 {
		t.Errorf("NaN mean: expected 0 after sanitization, got %g", mean)
	}
	if math.IsNaN(stddev) {
		t.Error("stddev should not be NaN after sanitization")
	}
	if stddev != 0 {
		t.Errorf("NaN stddev: expected 0 after sanitization, got %g", stddev)
	}
	if n != 5 {
		t.Errorf("n: got %d, want 5", n)
	}
}

// ─── TestAnomalyBaselineForMetric_WithStreamID ───────────────────────────────

// streamID parameter is applied to the WHERE clause (mock exercises the code path).
func TestAnomalyBaselineForMetric_WithStreamID(t *testing.T) {
	conn := newFakeConn().withRow(newFakeRow(float64(10.0), float64(2.0), int64(20)))
	svc := New(nilSnapLive{}, conn, nil)

	mean, _, n, err := svc.AnomalyBaselineForMetric(context.Background(), "viewer_count", "my-stream", 3600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mean != 10.0 {
		t.Errorf("mean: got %g, want 10.0", mean)
	}
	if n != 20 {
		t.Errorf("n: got %d, want 20", n)
	}
}

// errNoData is a sentinel for scan-failure tests.
var errNoData = errSentinel("no rows in result set")

type errSentinel string

func (e errSentinel) Error() string { return string(e) }
