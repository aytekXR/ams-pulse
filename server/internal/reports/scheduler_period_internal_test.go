// scheduler_period_internal_test.go — S51 regression test (D-113) for S48
// finding [4]: the scheduled-report period must be the PREVIOUS calendar month,
// with an inclusive upper bound on the LAST day of that month — not the first
// day of the current month (which the inclusive `bucket <= ?` daily-rollup query
// would pull into the report). Internal test (package reports) because
// previousCalendarMonthUTC is unexported.
package reports

import (
	"testing"
	"time"
)

func TestPreviousCalendarMonthUTC(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time {
		return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
	}
	cases := []struct {
		name     string
		now      time.Time
		wantFrom time.Time
		wantTo   time.Time
	}{
		{
			name:     "mid-month July -> all of June",
			now:      time.Date(2026, 7, 15, 9, 30, 0, 0, time.UTC),
			wantFrom: d(2026, 6, 1),
			wantTo:   d(2026, 6, 30), // NOT 2026-07-01 (the bug)
		},
		{
			name:     "first of the month is still the previous month",
			now:      time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			wantFrom: d(2026, 6, 1),
			wantTo:   d(2026, 6, 30),
		},
		{
			name:     "January -> previous December (year rolls back)",
			now:      time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC),
			wantFrom: d(2025, 12, 1),
			wantTo:   d(2025, 12, 31),
		},
		{
			name:     "March -> February end is the 28th (2026 not a leap year)",
			now:      time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC),
			wantFrom: d(2026, 2, 1),
			wantTo:   d(2026, 2, 28),
		},
		{
			// A non-UTC `now` must be resolved in UTC: 2026-07-01 01:00 +05:00 is
			// 2026-06-30 20:00 UTC, so the "current month" is JUNE and the previous
			// month is MAY. Guards the now.UTC() normalization inside the helper.
			name:     "non-UTC now is normalized to UTC before bucketing",
			now:      time.Date(2026, 7, 1, 1, 0, 0, 0, time.FixedZone("plus5", 5*3600)),
			wantFrom: d(2026, 5, 1),
			wantTo:   d(2026, 5, 31),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			from, to := previousCalendarMonthUTC(tc.now)
			if !from.Equal(tc.wantFrom) {
				t.Errorf("from = %s, want %s", from.Format(time.RFC3339), tc.wantFrom.Format(time.RFC3339))
			}
			if !to.Equal(tc.wantTo) {
				t.Errorf("to = %s, want %s (an inclusive bucket<=? query must not reach into the current month)",
					to.Format(time.RFC3339), tc.wantTo.Format(time.RFC3339))
			}
		})
	}
}
