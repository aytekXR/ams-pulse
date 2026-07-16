// S45 (D-107) — nextCronTime must honor the day-of-month field.
//
// The UI's DEFAULT report-schedule preset is "Monthly (1st of month, 6 AM UTC)"
// = "0 6 1 * *". The old parser dropped the day-of-month field, so nextCronTime
// matched the next 06:00 on ANY day — i.e. the "Monthly" preset fired DAILY.
//
// Mutation proof: revert cron.go/scheduler.go to ignore the dom field →
// "0 6 1 * *" from 14 Jun resolves to 15 Jun (tomorrow), not 1 Jul → RED.
package reports_test

import (
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/reports"
)

func TestNextCronTime_HonorsDayOfMonth(t *testing.T) {
	cases := []struct {
		name string
		cron string
		from time.Time
		want time.Time
	}{
		{
			name: "monthly 1st, mid-month -> next 1st (NOT tomorrow)",
			cron: "0 6 1 * *",
			from: time.Date(2026, 6, 14, 8, 0, 0, 0, time.UTC),
			want: time.Date(2026, 7, 1, 6, 0, 0, 0, time.UTC),
		},
		{
			name: "monthly 1st, on the 1st before 06:00 -> same day",
			cron: "0 6 1 * *",
			from: time.Date(2026, 6, 1, 5, 0, 0, 0, time.UTC),
			want: time.Date(2026, 6, 1, 6, 0, 0, 0, time.UTC),
		},
		{
			name: "monthly 1st, on the 1st after 06:00 -> next month",
			cron: "0 6 1 * *",
			from: time.Date(2026, 6, 1, 7, 0, 0, 0, time.UTC),
			want: time.Date(2026, 7, 1, 6, 0, 0, 0, time.UTC),
		},
		{
			name: "weekly Monday still resolves correctly",
			cron: "0 6 * * 1",
			from: time.Date(2026, 6, 14, 8, 0, 0, 0, time.UTC), // Sun 14 Jun 2026
			want: time.Date(2026, 6, 15, 6, 0, 0, 0, time.UTC), // Mon 15 Jun
		},
		{
			name: "daily unaffected",
			cron: "0 6 * * *",
			from: time.Date(2026, 6, 14, 8, 0, 0, 0, time.UTC),
			want: time.Date(2026, 6, 15, 6, 0, 0, 0, time.UTC),
		},
		// Both dom AND weekday restricted → Vixie OR-semantics: fire when EITHER
		// matches. These pin both arms of the OR branch in cronDayMatches.
		{
			name: "dom+weekday OR: 1st-or-Monday, weekday arm fires first",
			cron: "0 6 1 * 1",                                  // 1st of month OR Monday, at 06:00
			from: time.Date(2026, 6, 14, 8, 0, 0, 0, time.UTC), // Sun 14 Jun
			want: time.Date(2026, 6, 15, 6, 0, 0, 0, time.UTC), // Mon 15 Jun (weekday arm)
		},
		{
			name: "dom+weekday OR: 1st-or-Monday, dom arm fires first",
			cron: "0 6 1 * 1",
			from: time.Date(2026, 6, 30, 8, 0, 0, 0, time.UTC), // Tue 30 Jun; next Mon is 6 Jul
			want: time.Date(2026, 7, 1, 6, 0, 0, 0, time.UTC),  // Wed 1 Jul (dom arm, before next Mon)
		},
		// S51 / D-113 (S48 finding [15]): the cron must be interpreted in UTC even
		// when the caller seeds it with a non-UTC (local-timezone) time. The seed
		// below is 2026-06-14 03:00 in UTC-5, i.e. 2026-06-14 08:00 UTC. nextCronTime
		// normalises the seed to UTC, so "0 6 1 * *" resolves to 06:00 UTC on Jul 1.
		// Without that normalisation it would search in UTC-5 and return 06:00 UTC-5
		// (= 11:00 UTC), which .Equal would reject — making this case mutation-proof.
		{
			name: "non-UTC seed is interpreted in UTC (finding 15)",
			cron: "0 6 1 * *",
			from: time.Date(2026, 6, 14, 3, 0, 0, 0, time.FixedZone("minus5", -5*3600)),
			want: time.Date(2026, 7, 1, 6, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := reports.NextCronTime(tc.cron, tc.from)
			if !got.Equal(tc.want) {
				t.Errorf("NextCronTime(%q, %s) = %s, want %s",
					tc.cron, tc.from.Format(time.RFC3339),
					got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
		})
	}
}
