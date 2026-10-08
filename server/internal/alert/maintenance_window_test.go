package alert

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/aytekXR/ams-pulse/server/internal/store/meta"
)

// maintenance_window_test.go — what a maintenance window suppresses, when (S126: the
// feature had no test of its own semantics, and a window was cut off at midnight).

func TestCronMatches_WindowSemantics(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.Parse("2006-01-02 15:04 MST", s+" UTC")
		if err != nil {
			t.Fatalf("bad time %q", s)
		}
		return v
	}
	// 2026-10-10 is a Saturday (weekday 6), 2026-10-11 a Sunday (0).
	for _, tc := range []struct {
		cron   string
		dur    int
		now    string
		want   bool
		reason string
	}{
		{"0 2 *", 3600, "2026-10-08 02:30", true, "inside a daily window"},
		{"0 2 *", 3600, "2026-10-08 03:00", false, "the end is exclusive"},
		{"0 2 *", 3600, "2026-10-08 01:59", false, "before the start"},
		{"0 2 0", 3600, "2026-10-11 02:10", true, "Sunday window on a Sunday"},
		{"0 2 0", 3600, "2026-10-10 02:10", false, "Sunday window on a Saturday"},
		{"0 0 1-5", 7200, "2026-10-08 01:00", true, "weekday range"},
		{"0 0 1-5", 7200, "2026-10-10 01:00", false, "weekday range excludes Saturday"},
		{"0 23 *", 7200, "2026-10-09 00:30", true, "runs past midnight (was cut off at 00:00)"},
		{"0 23 *", 7200, "2026-10-09 01:00", false, "and still ends on time"},
		{"0 22 6", 86400, "2026-10-11 21:00", true, "a Saturday 24 h window covers Sunday until 22:00"},
		{"0 22 6", 86400, "2026-10-11 22:00", false, "and ends 24 h after it began"},
		{"0 22 6", 86400, "2026-10-12 21:00", false, "a Monday is not covered by a Saturday start"},
		{"30 *", 600, "2026-10-08 00:35", true, "a * hour means 00"},
	} {
		if got := cronMatches(tc.cron, tc.dur, at(tc.now)); got != tc.want {
			t.Errorf("cronMatches(%q, %d, %s) = %v, want %v (%s)", tc.cron, tc.dur, tc.now, got, tc.want, tc.reason)
		}
	}
}

// What ValidateMaintenanceWindows stores is what inMaintenanceWindowCron reads.
func TestValidatedWindowsAreTheOnesEvaluated(t *testing.T) {
	stored, err := ValidateMaintenanceWindows([]any{map[string]any{"start_cron": " 0  23  * ", "duration_s": 7200.0}})
	if err != nil {
		t.Fatalf("ValidateMaintenanceWindows: %v", err)
	}
	var w []maintenanceWindow
	if err := json.Unmarshal([]byte(stored), &w); err != nil || len(w) != 1 || w[0].StartCron != "0 23 *" {
		t.Fatalf("stored %s, want one window with start_cron \"0 23 *\"", stored)
	}
	rule := meta.AlertRuleRow{MaintenanceWindows: stored}
	if !inMaintenanceWindowCron(rule, time.Date(2026, 10, 9, 0, 15, 0, 0, time.UTC)) {
		t.Errorf("a validated window is not in effect at 00:15 the next day")
	}
}
