// Package reports — cron parsing for report schedules (WO-204 item 5).
// Self-contained copy of the cron parser from alert/wave2.go (no import cycle).
//
// Supported formats:
//   - 2-field:  "min hour"                  (every day)
//   - 3-field:  "min hour weekday"           (day-of-week filter)
//   - 5-field:  "min hour dom month weekday" (standard cron subset; VD-36)
//
// For 5-field expressions, dom and month fields are accepted but not used for
// nextCronTime computation (the scheduler uses a minute-by-minute search anyway).
// The meaningful fields are min, hour, and weekday (positions 0, 1, 4).
package reports

import (
	"fmt"
	"strconv"
	"strings"
)

// parseCronFieldsInternal parses 2-, 3-, or 5-field cron expressions.
// Returns (min, hour, weekday) where -1 means "any" (wildcard).
//
// 5-field format: "min hour dom month weekday" (standard cron).
// The dom (day-of-month) and month fields are parsed but not returned; they
// are accepted to support UI presets like "0 6 1 * *" (1st of each month at 06:00).
// For scheduling purposes, nextCronTime uses only min/hour/weekday.
//
// VD-36: previously this returned an error for 5-field inputs, causing the
// scheduler to fall back to a 1-month interval for all UI preset cron strings.
func parseCronFieldsInternal(expr string) (min, hour, weekday int, err error) {
	fields := strings.Fields(expr)
	switch len(fields) {
	case 2, 3:
		// 2-field: "min hour" or 3-field: "min hour weekday"
		return parseCronNField(fields)
	case 5:
		// 5-field standard cron: "min hour dom month weekday"
		// Extract the fields we care about: min(0), hour(1), weekday(4).
		return parseCronNField([]string{fields[0], fields[1], fields[4]})
	default:
		return 0, 0, -1, fmt.Errorf("cron: expected 2, 3, or 5 fields, got %d in %q", len(fields), expr)
	}
}

// parseCronNField parses a 2- or 3-element slice [min, hour[, weekday]].
func parseCronNField(fields []string) (min, hour, weekday int, err error) {
	parseField := func(s string) (int, error) {
		if s == "*" {
			return -1, nil
		}
		// Range like "1-5" → return low bound (sufficient for time search).
		if idx := strings.Index(s, "-"); idx >= 0 {
			n, err := strconv.Atoi(s[:idx])
			return n, err
		}
		n, err := strconv.Atoi(s)
		return n, err
	}

	min, err = parseField(fields[0])
	if err != nil {
		return 0, 0, -1, fmt.Errorf("cron: invalid minute %q: %w", fields[0], err)
	}
	hour, err = parseField(fields[1])
	if err != nil {
		return 0, 0, -1, fmt.Errorf("cron: invalid hour %q: %w", fields[1], err)
	}
	if len(fields) == 3 {
		weekday, err = parseField(fields[2])
		if err != nil {
			return 0, 0, -1, fmt.Errorf("cron: invalid weekday %q: %w", fields[2], err)
		}
	} else {
		weekday = -1
	}
	return min, hour, weekday, nil
}
