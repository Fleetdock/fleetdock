// Package cron parses standard 5-field cron expressions
// (minute hour day-of-month month day-of-week) and computes the next matching
// time. It supports '*', lists (a,b), ranges (a-b) and steps (*/n or a-b/n).
// It intentionally has no external dependencies.
package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is a parsed cron expression.
type Schedule struct {
	minute  uint64 // bit i set => minute i matches (0-59)
	hour    uint64 // 0-23
	dom     uint64 // 1-31
	month   uint64 // 1-12
	dow     uint64 // 0-6 (Sunday=0)
	domStar bool   // day-of-month was '*'
	dowStar bool   // day-of-week was '*'
}

// Parse compiles a 5-field cron expression.
func Parse(expr string) (*Schedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d", len(fields))
	}
	s := &Schedule{}
	var err error
	if s.minute, err = parseField(fields[0], 0, 59); err != nil {
		return nil, fmt.Errorf("cron minute: %w", err)
	}
	if s.hour, err = parseField(fields[1], 0, 23); err != nil {
		return nil, fmt.Errorf("cron hour: %w", err)
	}
	if s.dom, err = parseField(fields[2], 1, 31); err != nil {
		return nil, fmt.Errorf("cron day-of-month: %w", err)
	}
	if s.month, err = parseField(fields[3], 1, 12); err != nil {
		return nil, fmt.Errorf("cron month: %w", err)
	}
	if s.dow, err = parseField(fields[4], 0, 6); err != nil {
		return nil, fmt.Errorf("cron day-of-week: %w", err)
	}
	s.domStar = fields[2] == "*"
	s.dowStar = fields[4] == "*"
	return s, nil
}

// Next returns the earliest time strictly after `after` (truncated to the
// minute) that matches the schedule, or the zero time if none is found within
// roughly four years (guards against impossible expressions).
func (s *Schedule) Next(after time.Time) time.Time {
	t := after.Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(4, 0, 0)
	for t.Before(limit) {
		if s.matches(t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}
}

func (s *Schedule) matches(t time.Time) bool {
	if !bit(s.minute, t.Minute()) || !bit(s.hour, t.Hour()) || !bit(s.month, int(t.Month())) {
		return false
	}
	// Per cron convention, when both day-of-month and day-of-week are
	// restricted (not '*'), a match on *either* is sufficient.
	domMatch := bit(s.dom, t.Day())
	dowMatch := bit(s.dow, int(t.Weekday()))
	if s.domStar && s.dowStar {
		return true
	}
	if s.domStar {
		return dowMatch
	}
	if s.dowStar {
		return domMatch
	}
	return domMatch || dowMatch
}

func bit(mask uint64, i int) bool { return mask&(1<<uint(i)) != 0 }

func parseField(field string, min, max int) (uint64, error) {
	var mask uint64
	for _, part := range strings.Split(field, ",") {
		step := 1
		rng := part
		if slash := strings.IndexByte(part, '/'); slash >= 0 {
			var err error
			step, err = strconv.Atoi(part[slash+1:])
			if err != nil || step <= 0 {
				return 0, fmt.Errorf("invalid step %q", part)
			}
			rng = part[:slash]
		}

		lo, hi := min, max
		switch {
		case rng == "*":
			// full range
		case strings.ContainsRune(rng, '-'):
			bounds := strings.SplitN(rng, "-", 2)
			var err error
			if lo, err = strconv.Atoi(bounds[0]); err != nil {
				return 0, fmt.Errorf("invalid range %q", part)
			}
			if hi, err = strconv.Atoi(bounds[1]); err != nil {
				return 0, fmt.Errorf("invalid range %q", part)
			}
		default:
			n, err := strconv.Atoi(rng)
			if err != nil {
				return 0, fmt.Errorf("invalid value %q", part)
			}
			lo, hi = n, n
		}

		if lo < min || hi > max || lo > hi {
			return 0, fmt.Errorf("value %q out of bounds [%d-%d]", part, min, max)
		}
		for v := lo; v <= hi; v += step {
			mask |= 1 << uint(v)
		}
	}
	return mask, nil
}
