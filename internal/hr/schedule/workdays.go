package schedule

import "time"

// DefaultWorkDays is the Monday-Friday week used when an employee has no
// shift assignment.
func DefaultWorkDays() []time.Weekday {
	return []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}
}

// WorkingDays counts the working days in the inclusive range [start, end].
// A day counts when its weekday is in workDays and it is not in holidays.
// startHalf/endHalf make the first/last day count as half a day when that day
// is itself a working day; a single-day range with either flag set counts as
// 0.5. An empty workDays slice means no day is a working day; callers wanting
// the default week pass DefaultWorkDays(). end before start yields 0.
func WorkingDays(start, end time.Time, workDays []time.Weekday, holidays []time.Time, startHalf, endHalf bool) float64 {
	start, end = dateOnly(start), dateOnly(end)
	if end.Before(start) {
		return 0
	}
	working := make(map[time.Weekday]bool, len(workDays))
	for _, wd := range workDays {
		working[wd] = true
	}
	off := make(map[time.Time]bool, len(holidays))
	for _, h := range holidays {
		off[dateOnly(h)] = true
	}
	isWorking := func(d time.Time) bool { return working[d.Weekday()] && !off[d] }

	var total float64
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if isWorking(d) {
			total++
		}
	}
	if start.Equal(end) {
		if total > 0 && (startHalf || endHalf) {
			return 0.5
		}
		return total
	}
	if startHalf && isWorking(start) {
		total -= 0.5
	}
	if endHalf && isWorking(end) {
		total -= 0.5
	}
	return total
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// isoToWeekdays maps ISO weekday numbers (1=Monday..7=Sunday, as stored in
// hr_shifts.work_days) to time.Weekday. Out-of-range values are dropped.
func isoToWeekdays(days []int16) []time.Weekday {
	out := make([]time.Weekday, 0, len(days))
	for _, d := range days {
		if d < 1 || d > 7 {
			continue
		}
		out = append(out, time.Weekday(d%7))
	}
	return out
}

func weekdaysToISO(days []time.Weekday) []int16 {
	out := make([]int16, 0, len(days))
	for _, d := range days {
		if d == time.Sunday {
			out = append(out, 7)
			continue
		}
		out = append(out, int16(d))
	}
	return out
}
