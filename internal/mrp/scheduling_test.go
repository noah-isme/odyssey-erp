package mrp

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestScheduleFiniteRespectsCapacityAndDependencies(t *testing.T) {
	day := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	out, issues, err := ScheduleFinite(day, []CapacityDay{{WorkCenterID: 1, Date: day, Hours: 2}, {WorkCenterID: 1, Date: day.AddDate(0, 0, 1), Hours: 8}}, []SchedulableOperation{{ID: 1, WorkCenterID: 1, DurationHours: 2, Due: day.Add(24 * time.Hour)}, {ID: 2, WorkCenterID: 1, DurationHours: 3, Due: day.Add(48 * time.Hour), Predecessors: []int64{1}}})
	require.NoError(t, err)
	require.Len(t, issues, 0)
	require.Equal(t, day, out[0].Start)
	require.Equal(t, day.AddDate(0, 0, 1), out[1].Start)
}
