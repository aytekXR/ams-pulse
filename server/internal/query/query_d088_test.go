// D-088: unified node-degraded predicate — RED tests.
// These tests confirm that FleetNodes and LiveOverview use the full
// three-condition predicate (CPUPCT>90 || MemPCT>90 || ConsecAPIErrors>=3).
// All tests that are currently failing are marked in comments.
package query

import (
	"context"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// ─── FleetNodes D-088 ────────────────────────────────────────────────────────

// TestFleetNodes_ConsecAPIErrors_Degraded: a standalone AMS node with three
// consecutive API failures (and cpu=mem=0) must report status "degraded".
// RED before fix: FleetNodes only checks n.CPUPCT > 90, so it returns "up".
func TestFleetNodes_ConsecAPIErrors_Degraded(t *testing.T) {
	now := time.Now().UTC()
	snap := &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			"api-err-node": {
				CPUPCT:          0,
				MemPCT:          0,
				ConsecAPIErrors: 3, // rung-2 threshold
				UpdatedAt:       now,
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
		t.Errorf("Status: got %q, want degraded (ConsecAPIErrors=3)", res.Items[0].Status)
	}
}

// TestFleetNodes_ConsecAPIErrors_Two_Up: ConsecAPIErrors=2 is below the >=3
// threshold — status must remain "up". Regression guard.
func TestFleetNodes_ConsecAPIErrors_Two_Up(t *testing.T) {
	now := time.Now().UTC()
	snap := &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			"almost-degraded": {
				CPUPCT:          0,
				MemPCT:          0,
				ConsecAPIErrors: 2,
				UpdatedAt:       now,
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
	if res.Items[0].Status != "up" {
		t.Errorf("Status: got %q, want up (ConsecAPIErrors=2 < threshold)", res.Items[0].Status)
	}
}

// TestFleetNodes_MemDegraded: MemPCT=95 with cpu=0 must report "degraded" in
// the FleetNodes path. RED before fix: FleetNodes only checks CPUPCT>90.
func TestFleetNodes_MemDegraded(t *testing.T) {
	now := time.Now().UTC()
	snap := &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			"mem-hot-node": {
				CPUPCT:    0,
				MemPCT:    95.0, // > 90 → degraded
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
		t.Errorf("Status: got %q, want degraded (MemPCT=95>90)", res.Items[0].Status)
	}
}

// ─── LiveOverview D-088 ──────────────────────────────────────────────────────

// TestLiveOverview_ConsecAPIErrors_Degraded: a node with ConsecAPIErrors=3 and
// cpu=mem=0 must report Status="degraded" in the LiveOverview nodes list.
// RED before fix: LiveOverview only checks CPUPCT>90 || MemPCT>90.
func TestLiveOverview_ConsecAPIErrors_Degraded(t *testing.T) {
	now := time.Now().UTC()
	snap := &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			"api-err-ov-node": {
				CPUPCT:          0,
				MemPCT:          0,
				ConsecAPIErrors: 3,
				UpdatedAt:       now,
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
		t.Errorf("Status: got %q, want degraded (ConsecAPIErrors=3)", res.Nodes[0].Status)
	}
}
