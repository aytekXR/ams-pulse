// nonfinite_presence_test.go — REVIEW-MP3 round-3 regression pin R5.
//
// Rejecting a non-finite JMX reading is only half a fix. Before R5 the rejected
// value fell through to the alias field, which is 0 on a real AMS (it never sends
// cpuUsage/memoryUsage) — so a single "cpu":"NaN" hiccup was recorded as a
// MEASURED CPU of 0%: it dragged the Welford anomaly baseline toward zero and
// rendered a falsely idle node in the Fleet card. A rejected reading must be
// reported ABSENT so callers omit the field entirely.
package amsclient

import (
	"testing"
)

func TestR5_NonFiniteWithoutAlias_ReportsAbsent(t *testing.T) {
	for _, wire := range []string{"NaN", "Infinity", "-Infinity", "nonsense", "NaN%"} {
		t.Run(wire, func(t *testing.T) {
			n := ClusterNodeDTO{ID: "n1", CPU: flexString(wire), Memory: flexString(wire)}

			if v, ok := n.CPUPctOK(); ok {
				t.Errorf("FAIL(R5): cpu %q → (%v, ok=true); a rejected reading must be "+
					"ABSENT, not a fabricated measurement", wire, v)
			}
			if v, ok := n.MemPctOK(); ok {
				t.Errorf("FAIL(R5): memory %q → (%v, ok=true); must be absent", wire, v)
			}
		})
	}
}

func TestR5_NonFiniteWithAlias_UsesAlias(t *testing.T) {
	// A mock/alias profile that carries a real value must still be honoured — the
	// fix must not throw away usable data.
	n := ClusterNodeDTO{
		ID: "n1", CPU: flexString("NaN"), Memory: flexString("NaN"),
		CPUUsage: 33.0, MemoryUsage: 44.0,
	}
	if v, ok := n.CPUPctOK(); !ok || v != 33.0 {
		t.Errorf("cpu = (%v, %v), want (33, true) from the alias", v, ok)
	}
	if v, ok := n.MemPctOK(); !ok || v != 44.0 {
		t.Errorf("mem = (%v, %v), want (44, true) from the alias", v, ok)
	}
}

func TestR5_FiniteWireValues_Present(t *testing.T) {
	n := ClusterNodeDTO{ID: "n1", CPU: flexString("15.3"), Memory: flexString("40.2%")}
	if v, ok := n.CPUPctOK(); !ok || v != 15.3 {
		t.Errorf("cpu = (%v, %v), want (15.3, true)", v, ok)
	}
	if v, ok := n.MemPctOK(); !ok || v != 40.2 {
		t.Errorf("mem = (%v, %v), want (40.2, true)", v, ok)
	}
}

func TestR5_EmptyWireValues_FallBackToAliasAsPresent(t *testing.T) {
	// Alias-only profiles (no real wire field) read from the alias.
	n := ClusterNodeDTO{NodeID: "mock", CPUUsage: 22.5, MemoryUsage: 61.0}
	if v, ok := n.CPUPctOK(); !ok || v != 22.5 {
		t.Errorf("alias-only cpu = (%v, %v), want (22.5, true)", v, ok)
	}
	if v, ok := n.MemPctOK(); !ok || v != 61.0 {
		t.Errorf("alias-only mem = (%v, %v), want (61, true)", v, ok)
	}
}

// F-14 (external review round 4): a node carrying NEITHER the real wire field nor the
// alias has no reading at all. This previously returned (0, true) — a fabricated
// "measured 0%" reaching the Fleet card and the anomaly baselines through a different
// input than the NaN/Inf case R5 fixed.
func TestF14_NoCPUFieldAtAll_IsAbsentNotZero(t *testing.T) {
	n := ClusterNodeDTO{NodeID: "node-1"} // no cpu, no cpuUsage, no memory, no memoryUsage
	if v, ok := n.CPUPctOK(); ok {
		t.Errorf("cpu with no fields = (%v, %v), want (0, false) — absent, not a measured zero", v, ok)
	}
	if v, ok := n.MemPctOK(); ok {
		t.Errorf("mem with no fields = (%v, %v), want (0, false) — absent, not a measured zero", v, ok)
	}
}
