package mrp

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

var ErrInvalidSchedule = errors.New("mrp: invalid schedule")

type CapacityDay struct {
	WorkCenterID int64
	Date         time.Time
	Hours        float64
}
type SchedulableOperation struct {
	ID, WorkCenterID int64
	DurationHours    float64
	Due              time.Time
	Predecessors     []int64
	Manual           bool
	Start, End       time.Time
}
type ScheduleException struct {
	OperationID  int64
	Type, Detail string
}

// ScheduleFinite assigns only non-manual operations.  Capacity is consumed in
// daily buckets and predecessor completion is honoured before an operation can
// start.  It is pure to keep scheduling repeatable and testable.
func ScheduleFinite(asOf time.Time, capacity []CapacityDay, operations []SchedulableOperation) ([]SchedulableOperation, []ScheduleException, error) {
	if asOf.IsZero() {
		return nil, nil, ErrInvalidSchedule
	}
	available := map[string]float64{}
	for _, day := range capacity {
		if day.WorkCenterID <= 0 || day.Hours < 0 || day.Date.IsZero() {
			return nil, nil, ErrInvalidSchedule
		}
		available[capacityKey(day.WorkCenterID, day.Date)] += day.Hours
	}
	for i := range operations {
		if operations[i].ID <= 0 || operations[i].WorkCenterID <= 0 || operations[i].DurationHours < 0 {
			return nil, nil, ErrInvalidSchedule
		}
	}
	sort.SliceStable(operations, func(i, j int) bool { return operations[i].Due.Before(operations[j].Due) })
	byID := map[int64]*SchedulableOperation{}
	for i := range operations {
		byID[operations[i].ID] = &operations[i]
	}
	var issues []ScheduleException
	for i := range operations {
		op := &operations[i]
		if op.Manual {
			continue
		}
		start := dayStart(asOf)
		for _, dep := range op.Predecessors {
			p, ok := byID[dep]
			if !ok {
				return nil, nil, ErrInvalidSchedule
			}
			if p.End.IsZero() {
				issues = append(issues, ScheduleException{op.ID, "DEPENDENCY", "predecessor is not scheduled"})
				continue
			}
			if p.End.After(start) {
				start = dayStart(p.End)
			}
		}
		remaining := op.DurationHours
		first := time.Time{}
		end := time.Time{}
		for guard := 0; remaining > 0 && guard < 730; guard++ {
			key := capacityKey(op.WorkCenterID, start)
			free := available[key]
			if free > 0 {
				take := free
				if take > remaining {
					take = remaining
				}
				if first.IsZero() {
					first = start
				}
				remaining -= take
				available[key] -= take
				end = start.Add(time.Duration(take * float64(time.Hour)))
			}
			start = start.AddDate(0, 0, 1)
		}
		if remaining > 0 {
			issues = append(issues, ScheduleException{op.ID, "MISSING_CAPACITY", "no calendar capacity in scheduling horizon"})
			continue
		}
		op.Start, op.End = first, end
		if !op.Due.IsZero() && op.End.After(op.Due) {
			issues = append(issues, ScheduleException{op.ID, "LATE", "scheduled after due date"})
		}
	}
	return operations, issues, nil
}
func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
func capacityKey(wc int64, day time.Time) string {
	return fmt.Sprintf("%d:%s", wc, dayStart(day).Format("2006-01-02"))
}
