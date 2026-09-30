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
		{value: "15/09/2026", want: true},
		{value: "14/01/2001", want: false},
		{value: "2026-09-21", want: false},
		{value: "unknown", want: false},
	} {
		if got := matchesDateRange(test.value, from, to); got != test.want {
			t.Errorf("matchesDateRange(%q) = %t, want %t", test.value, got, test.want)
		}
	}
}

func TestMatchesDateRangeWithDayFirstDOB(t *testing.T) {
	from := testDate(1994, time.September, 1)
	to := testDate(2006, time.December, 31)
	if !matchesDateRange("14/01/2001", from, to) {
		t.Fatal("DOB 14/01/2001 should be included in 01/09/1994 through 31/12/2006")
	}
}

func TestVisibleVotingEvents(t *testing.T) {
	events := []votingEventRow{
		{id: 1, ownerID: 1, eventType: "Vote", eventClass: "Public", isActive: true},
		{id: 2, ownerID: 2, eventType: "Vote", eventClass: "Public", isActive: false},
		{id: 3, ownerID: 1, eventType: "Personal", eventClass: "Private", isActive: true},
	}
	adminEvents := visibleVotingEvents(events, 1, true)
	if len(adminEvents) != 1 || adminEvents[0].id != 1 {
		t.Fatalf("admin events = %#v; want only active non-Personal public event", adminEvents)
	}
	userEvents := visibleVotingEvents(events, 1, false)
	if len(userEvents) != 2 || userEvents[0].id != 1 || userEvents[1].id != 3 {
		t.Fatalf("user events = %#v; want active public and own Personal event", userEvents)
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

func TestClockInRange(t *testing.T) {
	for _, test := range []struct {
		value string
		want  bool
	}{
		{value: "06:00", want: true},
		{value: "14:30", want: true},
		{value: "22:00", want: true},
		{value: "05:59", want: false},
		{value: "22:01", want: false},
		{value: "24:00", want: false},
	} {
		if got := clockInRange(test.value, "06:00", "22:00"); got != test.want {
			t.Errorf("clockInRange(%q) = %t, want %t", test.value, got, test.want)
		}
	}
}

func TestEventIsActive(t *testing.T) {
	location := time.Local
	opens := time.Date(2026, time.September, 26, 9, 0, 0, 0, location)
	event := votingEventRow{
		opensAt:  opens.Format("2006-01-02 15:04"),
		closesAt: opens.Add(time.Hour).Format("2006-01-02 15:04"),
	}
	for _, test := range []struct {
		name string
		now  time.Time
		want bool
	}{
		{name: "before opening", now: opens.Add(-time.Minute), want: false},
		{name: "at opening", now: opens, want: true},
		{name: "before close", now: opens.Add(59 * time.Minute), want: true},
		{name: "at close", now: opens.Add(time.Hour), want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := eventIsActive(event, test.now); got != test.want {
				t.Errorf("eventIsActive() = %t, want %t", got, test.want)
			}
		})
	}
}
