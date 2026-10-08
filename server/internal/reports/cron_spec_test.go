package reports

import (
	"testing"
	"time"
)

// cron_spec_test.go — the report-schedule cron semantics fixed in S126. Before: a range
// kept only its low bound, the month field was ignored, nothing was bounds-checked, and
// an unparseable expression was stored and silently run a month later.

func TestNextCronTime_Semantics(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.Parse("2006-01-02 15:04", s)
		if err != nil {
			t.Fatalf("bad time %q", s)
		}
		return v.UTC()
	}
	for _, tc := range []struct {
		cron, from, want, why string
	}{
		{"0 9 * * 1-5", "2026-10-10 12:00", "2026-10-12 09:00", "Saturday → Monday (weekday range)"},
		{"0 9 * * 1-5", "2026-10-13 10:00", "2026-10-14 09:00", "Tuesday after 09:00 → WEDNESDAY, not next Monday"},
		{"0 9-17 * * *", "2026-10-08 10:30", "2026-10-08 11:00", "hour range fires every hour, not only at 09:00"},
		{"0 6 1 1 *", "2026-02-15 00:00", "2027-01-01 06:00", "month field honoured: yearly, not monthly"},
		{"*/15 * * * *", "2026-10-08 10:07", "2026-10-08 10:15", "minute step"},
		{"0,30 * * * *", "2026-10-08 10:07", "2026-10-08 10:30", "minute list"},
		{"0 0 29 2 *", "2026-03-01 00:00", "2028-02-29 00:00", "leap day found beyond one year"},
		{"0 8 * * 7", "2026-10-08 10:00", "2026-10-11 08:00", "weekday 7 is Sunday"},
		{"0 0 13 * 5", "2026-10-08 10:00", "2026-10-09 00:00", "dom AND weekday restricted → either (Friday 9th)"},
		{"0 6 */2 * 1", "2026-10-08 10:00", "2026-10-19 06:00", "a step starting with * counts as unrestricted (Vixie DOM_STAR): odd-day Mondays, not the 9th"},
		{"0 6 1 * *", "2026-10-08 10:00", "2026-11-01 06:00", "monthly preset (D-107 unchanged)"},
		{"30 2", "2026-10-08 10:00", "2026-10-09 02:30", "2-field form"},
		{"0 3 0", "2026-10-08 10:00", "2026-10-11 03:00", "3-field form: min hour weekday"},
	} {
		got := nextCronTime(tc.cron, at(tc.from))
		if !got.Equal(at(tc.want)) {
			t.Errorf("%-14q from %s = %s, want %s (%s)", tc.cron, tc.from, got.Format("2006-01-02 15:04 Mon"), tc.want, tc.why)
		}
		if err := ValidateCron(tc.cron); err != nil {
			t.Errorf("ValidateCron(%q) = %v, want valid", tc.cron, err)
		}
	}
}

func TestValidateCron_RefusesWhatCannotRun(t *testing.T) {
	for _, bad := range []string{
		"", "invalid cron", "* * * *", "* * * * * *",
		"61 * * * *", "* 24 * * *", "* * 0 * *", "* * 32 * *", "* * * 13 *", "* * * 0 *", "* * * * 8",
		"5-1 * * * *", "*/0 * * * *", "1,2,x * * * *", "MON * * * *", "0 0 31 2 *",
	} {
		if err := ValidateCron(bad); err == nil {
			t.Errorf("ValidateCron(%q) = nil, want an error", bad)
		}
	}
}
