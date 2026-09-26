package ui

import (
	"strings"
	"time"
)

func dateRangeValid(from, to *time.Time) bool {
	return from == nil || to == nil || !calendarDate(*from).After(calendarDate(*to))
}

func matchesDateRange(value string, from, to *time.Time) bool {
	if from == nil && to == nil {
		return true
	}
	date, ok := parseStoredDate(value)
	if !ok {
		return false
	}
	date = calendarDate(date)
	return (from == nil || !date.Before(calendarDate(*from))) &&
		(to == nil || !date.After(calendarDate(*to)))
}

func eventOverlapsDateRange(opensAt, closesAt string, from, to *time.Time) bool {
	if from == nil && to == nil {
		return true
	}
	opens, opensOK := parseStoredDate(opensAt)
	closes, closesOK := parseStoredDate(closesAt)
	if !opensOK || !closesOK {
		return false
	}
	opens = calendarDate(opens)
	closes = calendarDate(closes)
	if closes.Before(opens) {
		return false
	}
	return (from == nil || !closes.Before(calendarDate(*from))) &&
		(to == nil || !opens.After(calendarDate(*to)))
}

func parseStoredDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04", "2006-01-02"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func calendarDate(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}
