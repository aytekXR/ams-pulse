// Package reports — cron parsing for report schedules (WO-204 item 5).
// Self-contained copy of the cron parser from alert/wave2.go (no import cycle).
//
// Supported formats:
//   - 2-field:  "min hour"                  (every day)
//   - 3-field:  "min hour weekday"           (day-of-week filter)
//   - 5-field:  "min hour dom month weekday" (standard cron subset; VD-36)
//
// For 5-field expressions, the day-of-month (dom) field IS honored by
// nextCronTime (D-107) so UI presets like "0 6 1 * *" (1st of each month at
// 06:00) fire monthly, not daily. The month field (position 3) is accepted but
// not used — no UI preset restricts by month.
package reports

import (
	"fmt"
	"strconv"
	"strings"
)

// parseCronFieldsInternal parses 2-, 3-, or 5-field cron expressions.
// Returns (min, hour, dom, weekday) where -1 means "any" (wildcard).
//
// 5-field format: "min hour dom month weekday" (standard cron). The month field
// (position 3) is accepted but not parsed or used — any value there is silently
// ignored. Day-of-month (position 2) and weekday (position 4) are honored by
// nextCronTime.
//
// VD-36: previously this returned an error for 5-field inputs, causing the
// scheduler to fall back to a 1-month interval for all UI preset cron strings.
// D-107: previously it dropped the dom field, so "0 6 1 * *" fired every day.
func parseCronFieldsInternal(expr string) (min, hour, dom, weekday int, err error) {
	fields := strings.Fields(expr)
	switch len(fields) {
	case 2, 3:
		// 2-field: "min hour"; 3-field: "min hour weekday". No day-of-month.
		min, hour, weekday, err = parseCronNField(fields)
		return min, hour, -1, weekday, err
	case 5:
		// 5-field standard cron: "min hour dom month weekday".
		if min, err = parseCronField(fields[0]); err != nil {
			return 0, 0, -1, -1, fmt.Errorf("cron: invalid minute %q: %w", fields[0], err)
		}
		if hour, err = parseCronField(fields[1]); err != nil {
			return 0, 0, -1, -1, fmt.Errorf("cron: invalid hour %q: %w", fields[1], err)
		}
		if dom, err = parseCronField(fields[2]); err != nil {
			return 0, 0, -1, -1, fmt.Errorf("cron: invalid day-of-month %q: %w", fields[2], err)
		}
		// fields[3] (month) is accepted but not used.
		if weekday, err = parseCronField(fields[4]); err != nil {
			return 0, 0, -1, -1, fmt.Errorf("cron: invalid weekday %q: %w", fields[4], err)
		}
		return min, hour, dom, weekday, nil
	default:
		return 0, 0, -1, -1, fmt.Errorf("cron: expected 2, 3, or 5 fields, got %d in %q", len(fields), expr)
	}
}

// parseCronField parses a single cron field: "*" → -1 (any); a range "a-b" → its
// low bound (sufficient for a forward time search); a bare integer → that value.
func parseCronField(s string) (int, error) {
	if s == "*" {
		return -1, nil
	}
	if idx := strings.Index(s, "-"); idx >= 0 {
		return strconv.Atoi(s[:idx])
	}
	return strconv.Atoi(s)
}

// parseCronNField parses a 2- or 3-element slice [min, hour[, weekday]].
func parseCronNField(fields []string) (min, hour, weekday int, err error) {
	if min, err = parseCronField(fields[0]); err != nil {
		return 0, 0, -1, fmt.Errorf("cron: invalid minute %q: %w", fields[0], err)
	}
	if hour, err = parseCronField(fields[1]); err != nil {
		return 0, 0, -1, fmt.Errorf("cron: invalid hour %q: %w", fields[1], err)
	}
	if len(fields) == 3 {
		if weekday, err = parseCronField(fields[2]); err != nil {
			return 0, 0, -1, fmt.Errorf("cron: invalid weekday %q: %w", fields[2], err)
		}
	} else {
		weekday = -1
	}
	return min, hour, weekday, nil
}
