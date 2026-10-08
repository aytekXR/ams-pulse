package alert

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// maxMaintenanceWindowS caps one window at 24 h. cronMatches looks back one day (a window
// that started yesterday may still be open), so a longer window would close early without
// a word. Multi-day maintenance is several windows — maintenance_windows is an array.
const maxMaintenanceWindowS = 86400

// ValidateMaintenanceWindows checks a rule's maintenance_windows exactly as the evaluator
// will read them and returns the canonical JSON to store. nil/absent → "[]".
//
// Before S126 the API stored any array: an object written with the field names the
// alerting runbook showed (`cron_expr`) decoded to an empty start_cron, the evaluator
// skipped it, and notifications went out during the intended window.
func ValidateMaintenanceWindows(v any) (string, error) {
	if v == nil {
		return "[]", nil
	}
	items, ok := v.([]any)
	if !ok {
		return "", fmt.Errorf("maintenance_windows must be an array of {start_cron, duration_s}")
	}
	out := make([]maintenanceWindow, 0, len(items))
	for i, it := range items {
		obj, ok := it.(map[string]any)
		if !ok {
			return "", fmt.Errorf("maintenance_windows[%d] must be an object {start_cron, duration_s}", i)
		}
		for k := range obj {
			if k != "start_cron" && k != "duration_s" {
				hint := ""
				if k == "cron_expr" || k == "cron" {
					hint = " (the field is start_cron)"
				}
				return "", fmt.Errorf("maintenance_windows[%d]: unknown field %q%s", i, k, hint)
			}
		}
		cron, ok := obj["start_cron"].(string)
		if !ok || strings.TrimSpace(cron) == "" {
			return "", fmt.Errorf("maintenance_windows[%d].start_cron is required (\"min hour [weekday]\", UTC)", i)
		}
		if err := validateWindowCron(cron); err != nil {
			return "", fmt.Errorf("maintenance_windows[%d].start_cron: %w", i, err)
		}
		d, ok := obj["duration_s"].(float64)
		if !ok || d != float64(int(d)) || d < 1 || d > maxMaintenanceWindowS {
			return "", fmt.Errorf("maintenance_windows[%d].duration_s must be a whole number of seconds from 1 to %d (24 h; use one window per day for longer maintenance)", i, maxMaintenanceWindowS)
		}
		out = append(out, maintenanceWindow{StartCron: strings.Join(strings.Fields(cron), " "), DurationS: int(d)})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// validateWindowCron accepts what cronMatches implements: "min hour [weekday]" with a
// single minute (0-59) and hour (0-23) — "*" means 0, the window start is one time of
// day — and a weekday of "*", n or a-b within 0-6 (0 = Sunday).
func validateWindowCron(cron string) error {
	f := strings.Fields(cron)
	if len(f) < 2 || len(f) > 3 {
		return fmt.Errorf("expected \"min hour [weekday]\", got %d fields", len(f))
	}
	single := func(s, name string, hi int) error {
		if s == "*" {
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > hi {
			return fmt.Errorf("%s must be a number 0-%d or * (the window starts at one time of day), got %q", name, hi, s)
		}
		return nil
	}
	if err := single(f[0], "minute", 59); err != nil {
		return err
	}
	if err := single(f[1], "hour", 23); err != nil {
		return err
	}
	if len(f) == 3 {
		set, any, err := cronFieldSet(f[2])
		if err != nil {
			return fmt.Errorf("weekday must be *, 0-6 or a range like 1-5, got %q", f[2])
		}
		if !any {
			for d := range set {
				if d < 0 || d > 6 {
					return fmt.Errorf("weekday %d out of range 0-6 (0 = Sunday)", d)
				}
			}
		}
	}
	return nil
}
