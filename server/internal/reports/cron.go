// Package reports — cron parsing for report schedules (WO-204 item 5).
// Self-contained (no import of alert/wave2.go: import cycle).
//
// Accepted forms, all evaluated in UTC:
//   - 5 fields: "min hour dom month weekday" (standard cron)
//   - 3 fields: "min hour weekday"
//   - 2 fields: "min hour"                    (every day)
//
// Each field is "*", a number, a range "a-b", a step "*/n" or "a-b/n", or a comma list
// of those. Bounds: minute 0-59, hour 0-23, day-of-month 1-31, month 1-12, weekday
// 0-7 (0 and 7 are Sunday). Day-of-month and weekday follow Vixie cron: when both are
// restricted (neither starts with "*"), a day matches if EITHER matches.
//
// History (S126): a range used to keep only its low bound ("1-5" fired on Mondays only,
// "9-17" in the hour field at 09:00 only), the month field was ignored (a yearly
// "0 6 1 1 *" fired every month), nothing was bounds-checked, and an expression that
// did not parse was stored anyway and silently run a month later. ValidateCron lets the
// API refuse such input; nextCronTime uses the same parser, so the two cannot disagree.
package reports

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cronField is one parsed field: the set of values it matches, and whether it was
// written starting with "*" (Vixie's DOM_STAR / DOW_STAR, which decides AND vs OR).
type cronField struct {
	star bool
	set  [64]bool // index = value
}

// cronSpec is a parsed schedule.
type cronSpec struct {
	min, hour, dom, month, wday cronField
}

// parseCronSpec parses a 2-, 3- or 5-field expression.
func parseCronSpec(expr string) (cronSpec, error) {
	f := strings.Fields(expr)
	var parts [5]string
	switch len(f) {
	case 2:
		parts = [5]string{f[0], f[1], "*", "*", "*"}
	case 3:
		parts = [5]string{f[0], f[1], "*", "*", f[2]}
	case 5:
		copy(parts[:], f)
	default:
		return cronSpec{}, fmt.Errorf("cron: expected 2, 3 or 5 fields (min hour [dom month] weekday), got %d in %q", len(f), expr)
	}
	var c cronSpec
	var err error
	if c.min, err = parseCronFieldSet(parts[0], 0, 59, "minute"); err != nil {
		return cronSpec{}, err
	}
	if c.hour, err = parseCronFieldSet(parts[1], 0, 23, "hour"); err != nil {
		return cronSpec{}, err
	}
	if c.dom, err = parseCronFieldSet(parts[2], 1, 31, "day-of-month"); err != nil {
		return cronSpec{}, err
	}
	if c.month, err = parseCronFieldSet(parts[3], 1, 12, "month"); err != nil {
		return cronSpec{}, err
	}
	if c.wday, err = parseCronFieldSet(parts[4], 0, 7, "weekday"); err != nil {
		return cronSpec{}, err
	}
	if c.wday.set[7] { // 7 is Sunday, like 0
		c.wday.set[0] = true
	}
	return c, nil
}

// parseCronFieldSet parses one field into its value set.
func parseCronFieldSet(s string, lo, hi int, name string) (cronField, error) {
	var out cronField
	out.star = strings.HasPrefix(s, "*")
	for _, item := range strings.Split(s, ",") {
		rng, step := item, 1
		if i := strings.Index(item, "/"); i >= 0 {
			n, err := strconv.Atoi(item[i+1:])
			if err != nil || n < 1 {
				return cronField{}, fmt.Errorf("cron: invalid %s step in %q", name, item)
			}
			rng, step = item[:i], n
		}
		a, b := lo, hi
		switch {
		case rng == "*":
		case strings.Contains(rng, "-"):
			i := strings.Index(rng, "-")
			var errA, errB error
			a, errA = strconv.Atoi(rng[:i])
			b, errB = strconv.Atoi(rng[i+1:])
			if errA != nil || errB != nil || a > b {
				return cronField{}, fmt.Errorf("cron: invalid %s range %q", name, rng)
			}
		default:
			n, err := strconv.Atoi(rng)
			if err != nil {
				return cronField{}, fmt.Errorf("cron: invalid %s %q (use *, n, a-b, */n or a comma list)", name, item)
			}
			a, b = n, n
			if strings.Contains(item, "/") { // "n/step" = n through the field maximum
				b = hi
			}
		}
		if a < lo || b > hi {
			return cronField{}, fmt.Errorf("cron: %s %q out of range %d-%d", name, item, lo, hi)
		}
		for v := a; v <= b; v += step {
			out.set[v] = true
		}
	}
	return out, nil
}

// dayMatches applies Vixie semantics to day-of-month and weekday.
func (c cronSpec) dayMatches(day time.Time) bool {
	domOK := c.dom.set[day.Day()]
	wdayOK := c.wday.set[int(day.Weekday())]
	if c.dom.star || c.wday.star {
		return domOK && wdayOK
	}
	return domOK || wdayOK
}

// next returns the first matching minute strictly after from (UTC), searching four
// years ahead so a 29 February schedule is still found. ok=false: it never fires.
func (c cronSpec) next(from time.Time) (time.Time, bool) {
	t := from.UTC().Truncate(time.Minute).Add(time.Minute)
	end := t.AddDate(4, 0, 1)
	for day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC); day.Before(end); day = day.AddDate(0, 0, 1) {
		if !c.month.set[int(day.Month())] || !c.dayMatches(day) {
			continue
		}
		for h := 0; h < 24; h++ {
			if !c.hour.set[h] {
				continue
			}
			for m := 0; m < 60; m++ {
				if !c.min.set[m] {
					continue
				}
				if cand := day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute); !cand.Before(t) {
					return cand, true
				}
			}
		}
	}
	return time.Time{}, false
}

// ValidateCron reports whether expr is a schedule nextCronTime can run: it must parse,
// stay within bounds, and fire at least once in the next four years (so "0 0 31 2 *",
// 31 February, is refused).
func ValidateCron(expr string) error {
	c, err := parseCronSpec(expr)
	if err != nil {
		return err
	}
	if _, ok := c.next(time.Now()); !ok {
		return fmt.Errorf("cron: %q never fires (no date matches its day-of-month, month and weekday)", expr)
	}
	return nil
}
