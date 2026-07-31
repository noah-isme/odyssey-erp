package leave

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSubmitRejectsInvalidDateRangeBeforeDatabaseAccess(t *testing.T) {
	service := NewService(nil, nil, nil)
	_, err := service.Submit(context.Background(), CreateInput{
		UserID: 1, LeaveTypeID: 1,
		StartDate: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
	})
	require.EqualError(t, err, "hr: invalid leave request")
}

func TestSubmitRejectsMissingIdentityAndLeaveType(t *testing.T) {
	service := NewService(nil, nil, nil)
	for name, input := range map[string]CreateInput{
		"user":       {LeaveTypeID: 1, StartDate: time.Now(), EndDate: time.Now()},
		"leave type": {UserID: 1, StartDate: time.Now(), EndDate: time.Now()},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := service.Submit(context.Background(), input)
			require.EqualError(t, err, "hr: invalid leave request")
		})
	}
}
