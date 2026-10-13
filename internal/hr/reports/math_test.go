package reports

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/schedule"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestTurnoverRate(t *testing.T) {
	cases := []struct {
		name                      string
		leavers, opening, closing int64
		want                      float64
	}{
		{"no headcount", 3, 0, 0, 0},
		{"no leavers", 0, 10, 10, 0},
		{"flat headcount", 2, 20, 20, 0.1},
		{"growing headcount", 3, 10, 20, 0.2},
		{"shrinking to zero", 4, 8, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.InDelta(t, tc.want, TurnoverRate(tc.leavers, tc.opening, tc.closing), 1e-9)
		})
	}
	require.InDelta(t, 15.0, AverageHeadcount(10, 20), 1e-9)
}

func TestAbsenteeismRate(t *testing.T) {
	require.Equal(t, 0.0, AbsenteeismRate(2, 0))
	require.Equal(t, 0.0, AbsenteeismRate(0, 20))
	require.InDelta(t, 0.1, AbsenteeismRate(2, 20), 1e-9)
	require.InDelta(t, 0.25, AbsenteeismRate(5, 20), 1e-9)
}

func shiftAssignment(from time.Time, to *time.Time, days ...time.Weekday) schedule.ShiftAssignment {
	return schedule.ShiftAssignment{ValidFrom: from, ValidTo: to, Shift: schedule.Shift{WorkDays: days}}
}

func TestScheduledDays(t *testing.T) {
	// June 2026: Mon 1 .. Tue 30; 22 Mon-Fri days, 26 Mon-Sat days.
	from, to := day(2026, 6, 1), day(2026, 6, 30)
	monSat := []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday}
	june14 := day(2026, 6, 14)

	cases := []struct {
		name        string
		assignments []schedule.ShiftAssignment
		holidays    []time.Time
		want        float64
	}{
		{"default week", nil, nil, 22},
		{"default week minus weekday holiday", nil, []time.Time{day(2026, 6, 1)}, 21},
		{"weekend holiday ignored", nil, []time.Time{day(2026, 6, 6)}, 22},
		{"six-day shift whole month", []schedule.ShiftAssignment{shiftAssignment(day(2026, 1, 1), nil, monSat...)}, nil, 26},
		{
			// Default Mon-Fri 1-14 (10 days), Mon-Sat 15-30 (14 days).
			"gap then assignment", []schedule.ShiftAssignment{shiftAssignment(day(2026, 6, 15), nil, monSat...)}, nil, 24,
		},
		{
			// Mon-Sat 1-14 (12 days), default 15-30 (12 days).
			"assignment then gap", []schedule.ShiftAssignment{shiftAssignment(day(2026, 5, 1), &june14, monSat...)}, nil, 24,
		},
		{
			// Mon-Sat 1-14 (12), Mon/Wed/Fri 15-30 (7).
			"two assignments", []schedule.ShiftAssignment{
				shiftAssignment(day(2026, 5, 1), &june14, monSat...),
				shiftAssignment(day(2026, 6, 15), nil, time.Monday, time.Wednesday, time.Friday),
			}, nil, 19,
		},
		{"assignment ending before range", []schedule.ShiftAssignment{shiftAssignment(day(2026, 1, 1), ptr(day(2026, 5, 31)), monSat...)}, nil, 22},
		{"assignment starting after range", []schedule.ShiftAssignment{shiftAssignment(day(2026, 7, 1), nil, monSat...)}, nil, 22},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.InDelta(t, tc.want, ScheduledDays(from, to, tc.assignments, tc.holidays), 1e-9)
		})
	}
	require.Equal(t, 0.0, ScheduledDays(to, from, nil, nil))
}

func ptr[T any](v T) *T { return &v }

func TestEmploymentWindow(t *testing.T) {
	from, to := day(2026, 6, 1), day(2026, 6, 30)
	s, e, ok := employmentWindow(from, to, day(2020, 1, 1), nil)
	require.True(t, ok)
	require.Equal(t, from, s)
	require.Equal(t, to, e)

	s, e, ok = employmentWindow(from, to, day(2026, 6, 10), ptr(day(2026, 6, 20)))
	require.True(t, ok)
	require.Equal(t, day(2026, 6, 10), s)
	require.Equal(t, day(2026, 6, 20), e)

	_, _, ok = employmentWindow(from, to, day(2026, 7, 1), nil)
	require.False(t, ok)
	_, _, ok = employmentWindow(from, to, day(2020, 1, 1), ptr(day(2026, 5, 31)))
	require.False(t, ok)
}

func TestValuationDate(t *testing.T) {
	now := time.Date(2026, 10, 12, 15, 4, 5, 0, time.UTC)
	require.Equal(t, day(2026, 10, 12), valuationDate(2026, now))
	require.Equal(t, day(2025, 12, 31), valuationDate(2025, now))
	require.Equal(t, day(2027, 1, 1), valuationDate(2027, now))
}
