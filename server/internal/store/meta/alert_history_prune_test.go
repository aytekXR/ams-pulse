// TDD tests for alert_history auto-pruning (item 7 — P2 hardening batch).
//
// Behavioral coverage:
//   - PruneAlertHistory keeps the N newest rows for a rule; other rules untouched
//   - Kept rows are the newest by ts; equal-ts rows broken deterministically by rowid DESC
//   - keep <= 0 is a safe no-op (no rows deleted)
//   - Fewer rows than keep is a no-op (nothing deleted)
//   - CreateAlertHistory auto-prunes at the configured cap on every insert
//   - Auto-prune stays trivial at n=2000 (timing assertion)
package meta_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// makeRule creates a minimal alert rule and returns its assigned ID.
func makeRule(t *testing.T, s *meta.Store, name string) string {
	t.Helper()
	ctx := context.Background()
	r := meta.AlertRuleRow{
		Name:      name,
		Metric:    "stream_offline",
		Operator:  "eq",
		Threshold: 1,
		WindowS:   0,
		Severity:  "critical",
		Enabled:   true,
	}
	created, err := s.CreateAlertRule(ctx, r)
	if err != nil {
		t.Fatalf("CreateAlertRule(%s): %v", name, err)
	}
	return created.ID
}

// countHistoryRows returns the number of alert_history rows for a given ruleID.
func countHistoryRows(t *testing.T, s *meta.Store, ruleID string) int {
	t.Helper()
	ctx := context.Background()
	rows, err := s.ListAlertHistory(ctx, ruleID, "", 0, 0, 0)
	if err != nil {
		t.Fatalf("ListAlertHistory(%s): %v", ruleID, err)
	}
	return len(rows)
}

// insertNHistory inserts n alert_history rows for the given ruleID.
// ts values are baseTS, baseTS+1, …, baseTS+n-1.
// state cycles through firing/resolved/delivery_failure.
func insertNHistory(t *testing.T, s *meta.Store, ruleID string, n int, baseTS int64) {
	t.Helper()
	ctx := context.Background()
	states := []string{"firing", "resolved", "delivery_failure"}
	for i := 0; i < n; i++ {
		h := meta.AlertHistoryRow{
			AlertID:   fmt.Sprintf("alert-%s-%06d", ruleID[:8], i),
			RuleID:    ruleID,
			State:     states[i%len(states)],
			Severity:  "critical",
			TS:        baseTS + int64(i),
			Metric:    "stream_offline",
			Value:     1,
			Threshold: 1,
		}
		if err := s.CreateAlertHistory(ctx, h); err != nil {
			t.Fatalf("insertNHistory[%d]: %v", i, err)
		}
	}
}

// TestAlertHistory_PruneKeepsNewest verifies the core pruning invariant:
// after PruneAlertHistory(ctx, ruleID, keep), exactly `keep` rows remain for
// that rule, they are the newest by ts, and other rules are untouched.
func TestAlertHistory_PruneKeepsNewest(t *testing.T) {
	s := openStore(t)
	s.SetAlertHistoryCap(100000) // disable auto-prune during setup
	ctx := context.Background()

	const (
		nX   = 15
		nY   = 5
		keep = 5
	)
	baseTS := int64(1_000_000)

	ruleX := makeRule(t, s, "prune-test-x")
	ruleY := makeRule(t, s, "prune-test-y")

	// Insert 15 rows for rule X (ts = baseTS..baseTS+14), mixed states.
	states := []string{"firing", "resolved", "delivery_failure"}
	for i := 0; i < nX; i++ {
		h := meta.AlertHistoryRow{
			AlertID:   fmt.Sprintf("ax-%06d", i),
			RuleID:    ruleX,
			State:     states[i%len(states)],
			Severity:  "critical",
			TS:        baseTS + int64(i),
			Metric:    "stream_offline",
			Value:     1,
			Threshold: 1,
		}
		if err := s.CreateAlertHistory(ctx, h); err != nil {
			t.Fatalf("CreateAlertHistory ruleX[%d]: %v", i, err)
		}
	}

	// Insert 5 rows for rule Y (ts = baseTS..baseTS+4).
	for i := 0; i < nY; i++ {
		h := meta.AlertHistoryRow{
			AlertID:   fmt.Sprintf("ay-%06d", i),
			RuleID:    ruleY,
			State:     "resolved",
			Severity:  "info",
			TS:        baseTS + int64(i),
			Metric:    "node_cpu",
			Value:     0.5,
			Threshold: 0.8,
		}
		if err := s.CreateAlertHistory(ctx, h); err != nil {
			t.Fatalf("CreateAlertHistory ruleY[%d]: %v", i, err)
		}
	}

	// Sanity: all rows present before explicit prune.
	if got := countHistoryRows(t, s, ruleX); got != nX {
		t.Fatalf("pre-prune count ruleX: want %d, got %d", nX, got)
	}
	if got := countHistoryRows(t, s, ruleY); got != nY {
		t.Fatalf("pre-prune count ruleY: want %d, got %d", nY, got)
	}

	// Prune ruleX to keep=5.
	if err := s.PruneAlertHistory(ctx, ruleX, keep); err != nil {
		t.Fatalf("PruneAlertHistory: %v", err)
	}

	// Post-prune: ruleX has exactly keep rows.
	gotX := countHistoryRows(t, s, ruleX)
	if gotX != keep {
		t.Fatalf("post-prune count ruleX: want %d, got %d", keep, gotX)
	}

	// The kept rows must be the newest (highest ts: baseTS+10 .. baseTS+14).
	kept, err := s.ListAlertHistory(ctx, ruleX, "", 0, 0, 0)
	if err != nil {
		t.Fatalf("ListAlertHistory after prune: %v", err)
	}
	minExpectedTS := baseTS + int64(nX-keep) // baseTS+10
	for _, row := range kept {
		if row.TS < minExpectedTS {
			t.Errorf("pruned wrong row: ts=%d is below min expected ts=%d", row.TS, minExpectedTS)
		}
	}
	t.Logf("PASS: ruleX: %d rows kept, all with ts >= %d", gotX, minExpectedTS)

	// ruleY must be untouched.
	gotY := countHistoryRows(t, s, ruleY)
	if gotY != nY {
		t.Fatalf("ruleY was modified by prune: want %d, got %d", nY, gotY)
	}
	t.Logf("PASS: ruleY untouched: %d rows", gotY)
}

// TestAlertHistory_PruneEqualTsDeterministic verifies that equal-ts rows are
// pruned deterministically (rowid DESC tiebreak: higher rowid = later insert = kept).
func TestAlertHistory_PruneEqualTsDeterministic(t *testing.T) {
	s := openStore(t)
	s.SetAlertHistoryCap(100000) // disable auto-prune
	ctx := context.Background()

	const (
		nRows  = 8
		keep   = 3
		sameTS = int64(9_999_999)
	)

	ruleEq := makeRule(t, s, "prune-eq-ts")

	// Insert 8 rows all with the same ts.
	var insertedAlertIDs []string
	for i := 0; i < nRows; i++ {
		alertID := fmt.Sprintf("eq-ts-%06d", i)
		insertedAlertIDs = append(insertedAlertIDs, alertID)
		h := meta.AlertHistoryRow{
			AlertID:   alertID,
			RuleID:    ruleEq,
			State:     "firing",
			Severity:  "critical",
			TS:        sameTS,
			Metric:    "stream_offline",
			Value:     1,
			Threshold: 1,
		}
		if err := s.CreateAlertHistory(ctx, h); err != nil {
			t.Fatalf("CreateAlertHistory[%d]: %v", i, err)
		}
	}

	if err := s.PruneAlertHistory(ctx, ruleEq, keep); err != nil {
		t.Fatalf("PruneAlertHistory: %v", err)
	}

	got, err := s.ListAlertHistory(ctx, ruleEq, "", 0, 0, 0)
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(got) != keep {
		t.Fatalf("equal-ts prune: want %d rows, got %d", keep, len(got))
	}

	// The kept rows must be the last 3 inserted (highest rowids).
	expectedIDs := map[string]bool{
		insertedAlertIDs[nRows-1]: true,
		insertedAlertIDs[nRows-2]: true,
		insertedAlertIDs[nRows-3]: true,
	}
	for _, row := range got {
		if !expectedIDs[row.AlertID] {
			t.Errorf("unexpected row kept: alertID=%s (expected one of last %d inserted)", row.AlertID, keep)
		}
	}
	t.Logf("PASS: equal-ts prune kept the %d newest-inserted rows (rowid tiebreak)", keep)
}

// TestAlertHistory_PruneKeepNonPositiveIsNoop verifies that keep<=0 is a safe
// no-op — no rows are deleted.
// Semantics: keep=0 / keep<0 means "skip pruning" (not "delete everything").
// This prevents accidental mass-deletion if the cap is misconfigured.
func TestAlertHistory_PruneKeepNonPositiveIsNoop(t *testing.T) {
	s := openStore(t)
	s.SetAlertHistoryCap(100000)
	ctx := context.Background()

	ruleNoop := makeRule(t, s, "prune-noop")

	const n = 10
	for i := 0; i < n; i++ {
		h := meta.AlertHistoryRow{
			AlertID:   fmt.Sprintf("noop-%06d", i),
			RuleID:    ruleNoop,
			State:     "firing",
			Severity:  "info",
			TS:        int64(i + 1),
			Metric:    "stream_offline",
			Value:     1,
			Threshold: 1,
		}
		if err := s.CreateAlertHistory(ctx, h); err != nil {
			t.Fatalf("CreateAlertHistory[%d]: %v", i, err)
		}
	}

	// keep=0 must not delete anything.
	if err := s.PruneAlertHistory(ctx, ruleNoop, 0); err != nil {
		t.Fatalf("PruneAlertHistory(keep=0): %v", err)
	}
	if got := countHistoryRows(t, s, ruleNoop); got != n {
		t.Errorf("keep=0 deleted rows: want %d, got %d", n, got)
	}

	// keep=-1 must not delete anything either.
	if err := s.PruneAlertHistory(ctx, ruleNoop, -1); err != nil {
		t.Fatalf("PruneAlertHistory(keep=-1): %v", err)
	}
	if got := countHistoryRows(t, s, ruleNoop); got != n {
		t.Errorf("keep=-1 deleted rows: want %d, got %d", n, got)
	}
	t.Logf("PASS: keep<=0 is a no-op; all %d rows untouched", n)
}

// TestAlertHistory_PruneFewIsNoop verifies that when the row count is already
// <= keep, PruneAlertHistory deletes nothing.
func TestAlertHistory_PruneFewIsNoop(t *testing.T) {
	s := openStore(t)
	s.SetAlertHistoryCap(100000)
	ctx := context.Background()

	ruleFew := makeRule(t, s, "prune-few")

	const (
		n    = 3
		keep = 10
	)

	for i := 0; i < n; i++ {
		h := meta.AlertHistoryRow{
			AlertID:   fmt.Sprintf("few-%06d", i),
			RuleID:    ruleFew,
			State:     "firing",
			Severity:  "info",
			TS:        int64(i + 1),
			Metric:    "stream_offline",
			Value:     1,
			Threshold: 1,
		}
		if err := s.CreateAlertHistory(ctx, h); err != nil {
			t.Fatalf("CreateAlertHistory[%d]: %v", i, err)
		}
	}

	if err := s.PruneAlertHistory(ctx, ruleFew, keep); err != nil {
		t.Fatalf("PruneAlertHistory: %v", err)
	}
	if got := countHistoryRows(t, s, ruleFew); got != n {
		t.Errorf("fewer-than-keep: want %d, got %d", n, got)
	}
	t.Logf("PASS: %d rows < keep=%d → nothing pruned", n, keep)
}

// TestAlertHistory_AutoPruneOnCreate verifies that CreateAlertHistory calls
// PruneAlertHistory automatically after each insert, so that after
// cap+extraInserts calls the count stays bounded at the configured cap.
func TestAlertHistory_AutoPruneOnCreate(t *testing.T) {
	s := openStore(t)
	const smallCap = 20
	s.SetAlertHistoryCap(smallCap)
	ctx := context.Background()

	ruleAuto := makeRule(t, s, "prune-auto")

	const total = smallCap + 7 // insert more than the cap

	for i := 0; i < total; i++ {
		h := meta.AlertHistoryRow{
			AlertID:   fmt.Sprintf("auto-%06d", i),
			RuleID:    ruleAuto,
			State:     "firing",
			Severity:  "critical",
			TS:        int64(1_000 + i),
			Metric:    "stream_offline",
			Value:     1,
			Threshold: 1,
		}
		if err := s.CreateAlertHistory(ctx, h); err != nil {
			t.Fatalf("CreateAlertHistory[%d]: %v", i, err)
		}
	}

	got := countHistoryRows(t, s, ruleAuto)
	if got != smallCap {
		t.Fatalf("auto-prune: want %d rows, got %d", smallCap, got)
	}
	t.Logf("PASS: auto-prune kept count at %d after %d inserts", smallCap, total)
}

// TestAlertHistory_PruneTimingAt2000 inserts 2000 rows for one rule, then calls
// PruneAlertHistory(keep=1000) and asserts the DELETE stays trivial.
//
// Budget derivation (D-060, D-042-compliant): the prune is a single indexed
// DELETE of 1000 rows; it must beat the measured wall-clock of the 2000
// individual insert transactions that precede it. Both sides scale together
// under -race and CPU contention, so the bound is load-immune — a fixed
// 500ms budget was observed failing at 538ms under 4-way contention on a
// 2-vCPU host while a pathological (O(n^2)) prune would still exceed the
// insert baseline by orders of magnitude. The 500ms floor preserves the
// original absolute intent on fast, idle machines.
func TestAlertHistory_PruneTimingAt2000(t *testing.T) {
	s := openStore(t)
	s.SetAlertHistoryCap(100000) // disable auto-prune during setup
	ctx := context.Background()

	ruleTiming := makeRule(t, s, "prune-timing")

	const (
		n    = 2000
		keep = 1000
	)

	// Insert 2000 rows with sequential ts values; time them as the budget baseline.
	insertStart := time.Now()
	for i := 0; i < n; i++ {
		h := meta.AlertHistoryRow{
			AlertID:   fmt.Sprintf("timing-%06d", i),
			RuleID:    ruleTiming,
			State:     "firing",
			Severity:  "critical",
			TS:        int64(i),
			Metric:    "stream_offline",
			Value:     1,
			Threshold: 1,
		}
		if err := s.CreateAlertHistory(ctx, h); err != nil {
			t.Fatalf("insert[%d]: %v", i, err)
		}
	}
	insertElapsed := time.Since(insertStart)

	start := time.Now()
	if err := s.PruneAlertHistory(ctx, ruleTiming, keep); err != nil {
		t.Fatalf("PruneAlertHistory: %v", err)
	}
	elapsed := time.Since(start)

	got := countHistoryRows(t, s, ruleTiming)
	if got != keep {
		t.Fatalf("timing test: want %d rows, got %d", keep, got)
	}

	budget := insertElapsed
	if budget < 500*time.Millisecond {
		budget = 500 * time.Millisecond
	}

	t.Logf("PruneAlertHistory(n=2000, keep=1000) elapsed: %v (budget %v, insert baseline %v)",
		elapsed, budget, insertElapsed)
	if elapsed > budget {
		t.Errorf("prune too slow: %v > %v budget (insert baseline %v)", elapsed, budget, insertElapsed)
	} else {
		t.Logf("PASS: prune completed in %v (within %v)", elapsed, budget)
	}
}
