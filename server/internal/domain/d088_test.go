// D-088: tests for LiveNodeStats.Degraded() — the single source of truth for
// the node_degraded predicate shared by alert (wave2) and display (query).
// RED before implementation: Degraded() does not exist → compile error.
package domain_test

import (
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

func TestLiveNodeStats_Degraded_CPU(t *testing.T) {
	n := domain.LiveNodeStats{CPUPCT: 91.0, MemPCT: 0, ConsecAPIErrors: 0}
	if !n.Degraded() {
		t.Error("CPUPCT=91 > 90 must be degraded")
	}
}

func TestLiveNodeStats_Degraded_Mem(t *testing.T) {
	n := domain.LiveNodeStats{CPUPCT: 0, MemPCT: 91.0, ConsecAPIErrors: 0}
	if !n.Degraded() {
		t.Error("MemPCT=91 > 90 must be degraded")
	}
}

func TestLiveNodeStats_Degraded_ConsecAPIErrors_Three(t *testing.T) {
	n := domain.LiveNodeStats{CPUPCT: 0, MemPCT: 0, ConsecAPIErrors: 3}
	if !n.Degraded() {
		t.Error("ConsecAPIErrors=3 >= 3 must be degraded")
	}
}

func TestLiveNodeStats_Degraded_ConsecAPIErrors_Two_NotDegraded(t *testing.T) {
	n := domain.LiveNodeStats{CPUPCT: 0, MemPCT: 0, ConsecAPIErrors: 2}
	if n.Degraded() {
		t.Error("ConsecAPIErrors=2 < 3 must NOT be degraded")
	}
}

func TestLiveNodeStats_Degraded_Healthy(t *testing.T) {
	n := domain.LiveNodeStats{CPUPCT: 50.0, MemPCT: 50.0, ConsecAPIErrors: 0}
	if n.Degraded() {
		t.Error("healthy node (cpu=50, mem=50, consecErr=0) must NOT be degraded")
	}
}

func TestLiveNodeStats_Degraded_Boundary_CPU90(t *testing.T) {
	// exactly 90 is NOT degraded (condition is strictly > 90)
	n := domain.LiveNodeStats{CPUPCT: 90.0, MemPCT: 0, ConsecAPIErrors: 0}
	if n.Degraded() {
		t.Error("CPUPCT=90 (not > 90) must NOT be degraded")
	}
}

func TestLiveNodeStats_Degraded_Boundary_Mem90(t *testing.T) {
	n := domain.LiveNodeStats{CPUPCT: 0, MemPCT: 90.0, ConsecAPIErrors: 0}
	if n.Degraded() {
		t.Error("MemPCT=90 (not > 90) must NOT be degraded")
	}
}
