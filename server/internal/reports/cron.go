// Package reports — cron parsing for report schedules (WO-204 item 5).
// Self-contained copy of the 3-field cron parser from alert/wave2.go to
// avoid an import cycle between reports and alert packages.
package reports

import (
	"fmt"
	"strconv"
	"strings"
)

// parseCronFieldsInternal parses a simplified 3-field cron "min hour weekday".
// Returns (min, hour, weekday) where -1 means "any" (wildcard).
// Weekday 0=Sunday..6=Saturday (matching time.Weekday).
func parseCronFieldsInternal(expr string) (min, hour, weekday int, err error) {
	fields := strings.Fields(expr)
	if len(fields) < 2 || len(fields) > 3 {
		return 0, 0, -1, fmt.Errorf("cron: expected 2-3 fields, got %d in %q", len(fields), expr)
	}
	parseField := func(s string) (int, error) {
		if s == "*" {
			return -1, nil
		}
		// Handle range like "1-5" → return first value.
		if idx := strings.Index(s, "-"); idx >= 0 {
			s = s[:idx]
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
