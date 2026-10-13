package reports

import (
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/schedule"
)

// TurnoverRate returns leavers divided by the average of the opening and
// closing headcount. It returns 0 when the average headcount is 0.
func TurnoverRate(leavers, openingHeadcount, closingHeadcount int64) float64 {
	avg := AverageHeadcount(openingHeadcount, closingHeadcount)
	if avg <= 0 || leavers <= 0 {
		return 0
	}
	return float64(leavers) / avg
}

// AverageHeadcount is the mean of the opening and closing headcount.
func AverageHeadcount(opening, closing int64) float64 {
	return float64(opening+closing) / 2
}

// AbsenteeismRate returns absent days divided by scheduled working days, or 0
// when nothing was scheduled.
func AbsenteeismRate(absentDays int64, scheduledDays float64) float64 {
	if scheduledDays <= 0 || absentDays <= 0 {
		return 0
	}
	return float64(absentDays) / scheduledDays
}

// ScheduledDays counts the employee's scheduled working days in the inclusive
// range [from, to]. Days covered by a shift assignment use that shift's work
// days; uncovered days use schedule.DefaultWorkDays. Holidays never count.
// assignments must be ordered by ValidFrom (as ShiftAssignmentsForEmployees
// returns them). Pure function: no database access.
func ScheduledDays(from, to time.Time, assignments []schedule.ShiftAssignment, holidays []time.Time) float64 {
	from, to = dateOnly(from), dateOnly(to)
	if to.Before(from) {
		return 0
	}
	var total float64
	cursor := from
	for _, a := range assignments {
		if cursor.After(to) {
			break
		}
		segStart := dateOnly(a.ValidFrom)
		segEnd := to
		if a.ValidTo != nil && dateOnly(*a.ValidTo).Before(segEnd) {
			segEnd = dateOnly(*a.ValidTo)
		}
		if segEnd.Before(cursor) {
			continue
		}
		if segStart.After(to) {
			break
		}
		if segStart.After(cursor) {
			// Gap before this assignment: default week.
			total += schedule.WorkingDays(cursor, segStart.AddDate(0, 0, -1), schedule.DefaultWorkDays(), holidays, false, false)
			cursor = segStart
		}
		total += schedule.WorkingDays(cursor, segEnd, a.Shift.WorkDays, holidays, false, false)
		cursor = segEnd.AddDate(0, 0, 1)
	}
	if !cursor.After(to) {
		total += schedule.WorkingDays(cursor, to, schedule.DefaultWorkDays(), holidays, false, false)
	}
	return total
}

// employmentWindow clips [from, to] to the employee's hire and termination
// dates. ok is false when the employee was not employed in the range.
func employmentWindow(from, to, hire time.Time, termination *time.Time) (time.Time, time.Time, bool) {
	start, end := dateOnly(from), dateOnly(to)
	if h := dateOnly(hire); h.After(start) {
		start = h
	}
	if termination != nil {
		if t := dateOnly(*termination); t.Before(end) {
			end = t
		}
	}
	return start, end, !end.Before(start)
}

// valuationDate is the date leave liability is priced at for a balance year:
// today clamped into [Jan 1, Dec 31] of that year.
func valuationDate(year int, now time.Time) time.Time {
	first := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)
	d := dateOnly(now)
	if d.Before(first) {
		return first
	}
	if d.After(last) {
		return last
	}
	return d
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
