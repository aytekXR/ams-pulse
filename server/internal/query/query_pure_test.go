// Package query — unit tests for pure, Conn-free helper functions.
// No //go:build integration tag: runs in the plain coverage pass.
package query

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// emptyLiveProvider returns a non-nil snapshot with zero streams/nodes — the case
// that made LiveOverview/FleetNodes serialize apps/nodes as JSON null (nil slice).
type emptyLiveProvider struct{}

func (emptyLiveProvider) CurrentSnapshot() *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes:   map[string]*domain.LiveNodeStats{},
	}
}

func (emptyLiveProvider) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot)
	return ch, func() {}
}

// TestLiveOverview_EmptyArrays_SerializeAsBrackets_NotNull guards the D-045 fix:
// with zero streams/nodes, LiveOverview.apps/nodes and FleetNodes.items MUST
// serialize as [] (OpenAPI `type: array`), never null. Before the fix these were
// nil slices (`var apps []AppOverview`) and json.Marshal encoded them as null,
// violating the spec — caught by the response-body conformance test's empty path.
func TestLiveOverview_EmptyArrays_SerializeAsBrackets_NotNull(t *testing.T) {
	qsvc := New(emptyLiveProvider{}, nil, nil) // conn nil: these paths read the live snapshot, not ClickHouse

	ov, err := qsvc.LiveOverview(context.Background(), "", "", "")
	if err != nil {
		t.Fatalf("LiveOverview: %v", err)
	}
	ovJSON, err := json.Marshal(ov)
	if err != nil {
		t.Fatalf("marshal LiveOverview: %v", err)
	}
	s := string(ovJSON)
	if strings.Contains(s, `"apps":null`) || !strings.Contains(s, `"apps":[]`) {
		t.Errorf("LiveOverview apps must serialize as [] not null: %s", s)
	}
	if strings.Contains(s, `"nodes":null`) || !strings.Contains(s, `"nodes":[]`) {
		t.Errorf("LiveOverview nodes must serialize as [] not null: %s", s)
	}

	fleet, err := qsvc.FleetNodes(context.Background(), 50, "")
	if err != nil {
		t.Fatalf("FleetNodes: %v", err)
	}
	fleetJSON, err := json.Marshal(fleet)
	if err != nil {
		t.Fatalf("marshal FleetNodes: %v", err)
	}
	if fs := string(fleetJSON); strings.Contains(fs, `"items":null`) || !strings.Contains(fs, `"items":[]`) {
		t.Errorf("FleetNodes items must serialize as [] not null: %s", fs)
	}
}

// ─── jsonSafeFloat ────────────────────────────────────────────────────────────

func TestJsonSafeFloat_NaN(t *testing.T) {
	got := jsonSafeFloat(math.NaN())
	if got != 0 {
		t.Fatalf("NaN: want 0, got %v", got)
	}
}

func TestJsonSafeFloat_PosInf(t *testing.T) {
	got := jsonSafeFloat(math.Inf(1))
	if got != 0 {
		t.Fatalf("+Inf: want 0, got %v", got)
	}
}

func TestJsonSafeFloat_NegInf(t *testing.T) {
	got := jsonSafeFloat(math.Inf(-1))
	if got != 0 {
		t.Fatalf("-Inf: want 0, got %v", got)
	}
}

func TestJsonSafeFloat_Normal(t *testing.T) {
	const v = 42.5
	got := jsonSafeFloat(v)
	if got != v {
		t.Fatalf("normal: want %v, got %v", v, got)
	}
}

func TestJsonSafeFloat_Zero(t *testing.T) {
	got := jsonSafeFloat(0)
	if got != 0 {
		t.Fatalf("zero: want 0, got %v", got)
	}
}

func TestJsonSafeFloat_Negative(t *testing.T) {
	const v = -3.14
	got := jsonSafeFloat(v)
	if got != v {
		t.Fatalf("negative: want %v, got %v", v, got)
	}
}

// ─── buildTimeWhere ──────────────────────────────────────────────────────────

var (
	tFrom = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	tTo   = time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)
)

func TestBuildTimeWhere_BothZero(t *testing.T) {
	sql, args := buildTimeWhere(time.Time{}, time.Time{})
	if sql != "1=1" {
		t.Fatalf("both zero: want sql=%q, got %q", "1=1", sql)
	}
	if args != nil {
		t.Fatalf("both zero: want nil args, got %v", args)
	}
}

func TestBuildTimeWhere_FromOnly(t *testing.T) {
	sql, args := buildTimeWhere(tFrom, time.Time{})
	wantSQL := "bucket >= ?"
	wantArgs := []any{tFrom}
	if sql != wantSQL {
		t.Fatalf("from-only: want sql=%q, got %q", wantSQL, sql)
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("from-only: want args=%v, got %v", wantArgs, args)
	}
}

func TestBuildTimeWhere_ToOnly(t *testing.T) {
	sql, args := buildTimeWhere(time.Time{}, tTo)
	wantSQL := "bucket <= ?"
	wantArgs := []any{tTo}
	if sql != wantSQL {
		t.Fatalf("to-only: want sql=%q, got %q", wantSQL, sql)
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("to-only: want args=%v, got %v", wantArgs, args)
	}
}

func TestBuildTimeWhere_FromAndTo(t *testing.T) {
	sql, args := buildTimeWhere(tFrom, tTo)
	wantSQL := "bucket >= ? AND bucket <= ?"
	wantArgs := []any{tFrom, tTo}
	if sql != wantSQL {
		t.Fatalf("from+to: want sql=%q, got %q", wantSQL, sql)
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("from+to: want args=%v, got %v", wantArgs, args)
	}
}

// Verify arg count matches placeholder count.
func TestBuildTimeWhere_ArgCount(t *testing.T) {
	cases := []struct {
		from, to         time.Time
		wantPlaceholders int
	}{
		{time.Time{}, time.Time{}, 0},
		{tFrom, time.Time{}, 1},
		{time.Time{}, tTo, 1},
		{tFrom, tTo, 2},
	}
	for _, c := range cases {
		sql, args := buildTimeWhere(c.from, c.to)
		count := 0
		for i := 0; i < len(sql); i++ {
			if sql[i] == '?' {
				count++
			}
		}
		if count != c.wantPlaceholders {
			t.Errorf("buildTimeWhere(%v,%v): sql=%q has %d placeholders, want %d",
				c.from, c.to, sql, count, c.wantPlaceholders)
		}
		if len(args) != c.wantPlaceholders {
			t.Errorf("buildTimeWhere(%v,%v): got %d args, want %d",
				c.from, c.to, len(args), c.wantPlaceholders)
		}
	}
}

// ─── buildSessionTimeWhere ───────────────────────────────────────────────────

func TestBuildSessionTimeWhere_BothZero(t *testing.T) {
	sql, args := buildSessionTimeWhere(time.Time{}, time.Time{})
	if sql != "1=1" {
		t.Fatalf("both zero: want sql=%q, got %q", "1=1", sql)
	}
	if args != nil {
		t.Fatalf("both zero: want nil args, got %v", args)
	}
}

func TestBuildSessionTimeWhere_FromOnly(t *testing.T) {
	sql, args := buildSessionTimeWhere(tFrom, time.Time{})
	wantSQL := "started_at >= ?"
	wantArgs := []any{tFrom}
	if sql != wantSQL {
		t.Fatalf("from-only: want sql=%q, got %q", wantSQL, sql)
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("from-only: want args=%v, got %v", wantArgs, args)
	}
}

func TestBuildSessionTimeWhere_ToOnly(t *testing.T) {
	sql, args := buildSessionTimeWhere(time.Time{}, tTo)
	wantSQL := "started_at <= ?"
	wantArgs := []any{tTo}
	if sql != wantSQL {
		t.Fatalf("to-only: want sql=%q, got %q", wantSQL, sql)
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("to-only: want args=%v, got %v", wantArgs, args)
	}
}

func TestBuildSessionTimeWhere_FromAndTo(t *testing.T) {
	sql, args := buildSessionTimeWhere(tFrom, tTo)
	wantSQL := "started_at >= ? AND started_at <= ?"
	wantArgs := []any{tFrom, tTo}
	if sql != wantSQL {
		t.Fatalf("from+to: want sql=%q, got %q", wantSQL, sql)
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("from+to: want args=%v, got %v", wantArgs, args)
	}
}

// Verify arg count matches placeholder count.
func TestBuildSessionTimeWhere_ArgCount(t *testing.T) {
	cases := []struct {
		from, to         time.Time
		wantPlaceholders int
	}{
		{time.Time{}, time.Time{}, 0},
		{tFrom, time.Time{}, 1},
		{time.Time{}, tTo, 1},
		{tFrom, tTo, 2},
	}
	for _, c := range cases {
		sql, args := buildSessionTimeWhere(c.from, c.to)
		count := 0
		for i := 0; i < len(sql); i++ {
			if sql[i] == '?' {
				count++
			}
		}
		if count != c.wantPlaceholders {
			t.Errorf("buildSessionTimeWhere(%v,%v): sql=%q has %d placeholders, want %d",
				c.from, c.to, sql, count, c.wantPlaceholders)
		}
		if len(args) != c.wantPlaceholders {
			t.Errorf("buildSessionTimeWhere(%v,%v): got %d args, want %d",
				c.from, c.to, len(args), c.wantPlaceholders)
		}
	}
}
