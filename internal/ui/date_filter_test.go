package ui

import (
	"testing"
	"time"
)

func testDate(year int, month time.Month, day int) *time.Time {
	value := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return &value
}

func TestMatchesDateRange(t *testing.T) {
	from := testDate(2026, time.September, 10)
	to := testDate(2026, time.September, 20)
	for _, test := range []struct {
		value string
		want  bool
	}{
		{value: "2026-09-10", want: true},
		{value: "2026-09-15", want: true},
		{value: "2026-09-21", want: false},
		{value: "unknown", want: false},
	} {
		if got := matchesDateRange(test.value, from, to); got != test.want {
			t.Errorf("matchesDateRange(%q) = %t, want %t", test.value, got, test.want)
		}
	}
}

func TestEventOverlapsDateRange(t *testing.T) {
	from := testDate(2026, time.September, 10)
	to := testDate(2026, time.September, 20)
	for _, test := range []struct {
		opensAt  string
		closesAt string
		want     bool
	}{
		{opensAt: "2026-09-01 09:00", closesAt: "2026-09-10 17:00", want: true},
		{opensAt: "2026-09-15 09:00", closesAt: "2026-09-18 17:00", want: true},
		{opensAt: "2026-09-21 09:00", closesAt: "2026-09-22 17:00", want: false},
		{opensAt: "invalid", closesAt: "2026-09-12 17:00", want: false},
	} {
		if got := eventOverlapsDateRange(test.opensAt, test.closesAt, from, to); got != test.want {
			t.Errorf("eventOverlapsDateRange(%q, %q) = %t, want %t", test.opensAt, test.closesAt, got, test.want)
		}
	}
}
