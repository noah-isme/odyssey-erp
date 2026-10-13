package schedule

import (
	"testing"
	"time"
)

func d(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }

func TestWorkingDays(t *testing.T) {
	monFri := DefaultWorkDays()
	// 2026-10-12 is a Monday.
	tests := []struct {
		name               string
		start, end         time.Time
		workDays           []time.Weekday
		holidays           []time.Time
		startHalf, endHalf bool
		want               float64
	}{
		{name: "full week Mon-Fri", start: d(2026, 10, 12), end: d(2026, 10, 16), workDays: monFri, want: 5},
		{name: "weekend span Fri-Mon", start: d(2026, 10, 16), end: d(2026, 10, 19), workDays: monFri, want: 2},
		{name: "weekend only", start: d(2026, 10, 17), end: d(2026, 10, 18), workDays: monFri, want: 0},
		{name: "holiday inside", start: d(2026, 10, 12), end: d(2026, 10, 16), workDays: monFri,
			holidays: []time.Time{d(2026, 10, 14)}, want: 4},
		{name: "holiday outside range ignored", start: d(2026, 10, 12), end: d(2026, 10, 16), workDays: monFri,
			holidays: []time.Time{d(2026, 10, 20)}, want: 5},
		{name: "single-day start half", start: d(2026, 10, 13), end: d(2026, 10, 13), workDays: monFri, startHalf: true, want: 0.5},
		{name: "single-day end half", start: d(2026, 10, 13), end: d(2026, 10, 13), workDays: monFri, endHalf: true, want: 0.5},
		{name: "single-day both halves", start: d(2026, 10, 13), end: d(2026, 10, 13), workDays: monFri, startHalf: true, endHalf: true, want: 0.5},
		{name: "single full day", start: d(2026, 10, 13), end: d(2026, 10, 13), workDays: monFri, want: 1},
		{name: "both halves on two-day span", start: d(2026, 10, 13), end: d(2026, 10, 14), workDays: monFri, startHalf: true, endHalf: true, want: 1.0},
		{name: "start half only on three days", start: d(2026, 10, 13), end: d(2026, 10, 15), workDays: monFri, startHalf: true, want: 2.5},
		{name: "all-holiday range", start: d(2026, 10, 13), end: d(2026, 10, 14), workDays: monFri,
			holidays: []time.Time{d(2026, 10, 13), d(2026, 10, 14)}, startHalf: true, want: 0},
		{name: "single holiday with half", start: d(2026, 10, 13), end: d(2026, 10, 13), workDays: monFri,
			holidays: []time.Time{d(2026, 10, 13)}, startHalf: true, want: 0},
		{name: "start on weekend", start: d(2026, 10, 18), end: d(2026, 10, 20), workDays: monFri, want: 2},
		{name: "start on weekend half ignored", start: d(2026, 10, 18), end: d(2026, 10, 20), workDays: monFri, startHalf: true, want: 2},
		{name: "six-day shift includes Saturday", start: d(2026, 10, 12), end: d(2026, 10, 18),
			workDays: append(DefaultWorkDays(), time.Saturday), want: 6},
		{name: "end before start", start: d(2026, 10, 16), end: d(2026, 10, 12), workDays: monFri, want: 0},
		{name: "no work days", start: d(2026, 10, 12), end: d(2026, 10, 16), workDays: nil, want: 0},
		{name: "time of day ignored", start: time.Date(2026, 10, 12, 23, 30, 0, 0, time.UTC), end: time.Date(2026, 10, 13, 1, 0, 0, 0, time.UTC),
			workDays: monFri, holidays: []time.Time{time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)}, want: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := WorkingDays(tc.start, tc.end, tc.workDays, tc.holidays, tc.startHalf, tc.endHalf)
			if got != tc.want {
				t.Fatalf("WorkingDays = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWeekdayISORoundTrip(t *testing.T) {
	all := []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday}
	iso := weekdaysToISO(all)
	want := []int16{1, 2, 3, 4, 5, 6, 7}
	for i := range want {
		if iso[i] != want[i] {
			t.Fatalf("weekdaysToISO = %v, want %v", iso, want)
		}
	}
	back := isoToWeekdays(append(iso, 0, 8))
	if len(back) != len(all) {
		t.Fatalf("isoToWeekdays dropped/kept wrong values: %v", back)
	}
	for i := range all {
		if back[i] != all[i] {
			t.Fatalf("isoToWeekdays = %v, want %v", back, all)
		}
	}
}
