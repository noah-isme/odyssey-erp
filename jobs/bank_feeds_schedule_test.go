package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

type bankFeedScheduleStoreFake struct {
	connections []BankFeedScheduleConnection
	err         error
}

func (f bankFeedScheduleStoreFake) ListDueBankFeedConnections(context.Context) ([]BankFeedScheduleConnection, error) {
	return f.connections, f.err
}

func TestBankFeedsSyncScanEnqueuesUniqueScopedConnections(t *testing.T) {
	store := bankFeedScheduleStoreFake{connections: []BankFeedScheduleConnection{
		{ID: 20, CompanyID: 2},
		{ID: 10, CompanyID: 1},
		{ID: 20, CompanyID: 2},
	}}
	var enqueued []int64
	handler := HandleBankFeedsSyncScanTask(store, func(_ context.Context, connectionID int64) error {
		enqueued = append(enqueued, connectionID)
		return nil
	})

	require.NoError(t, handler(context.Background(), NewBankFeedsSyncScanTask()))
	require.Equal(t, []int64{20, 10}, enqueued)
}

func TestBankFeedsSyncScanTreatsDuplicateQueueAsSuccess(t *testing.T) {
	store := bankFeedScheduleStoreFake{connections: []BankFeedScheduleConnection{{ID: 10, CompanyID: 1}}}
	handler := HandleBankFeedsSyncScanTask(store, func(context.Context, int64) error {
		return asynq.ErrTaskIDConflict
	})

	require.NoError(t, handler(context.Background(), NewBankFeedsSyncScanTask()))
}

func TestBankFeedsSyncScanRejectsInvalidOrConflictingScope(t *testing.T) {
	tests := []struct {
		name         string
		connections  []BankFeedScheduleConnection
		wantFragment string
	}{
		{name: "invalid connection", connections: []BankFeedScheduleConnection{{ID: 0, CompanyID: 1}}, wantFragment: "invalid connection scope"},
		{name: "invalid company", connections: []BankFeedScheduleConnection{{ID: 1, CompanyID: 0}}, wantFragment: "invalid connection scope"},
		{name: "conflicting company", connections: []BankFeedScheduleConnection{{ID: 1, CompanyID: 1}, {ID: 1, CompanyID: 2}}, wantFragment: "conflicting companies"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := HandleBankFeedsSyncScanTask(bankFeedScheduleStoreFake{connections: tt.connections}, func(context.Context, int64) error {
				t.Fatal("invalid scope must not enqueue a task")
				return nil
			})
			err := handler(context.Background(), NewBankFeedsSyncScanTask())
			require.Error(t, err)
			require.ErrorIs(t, err, asynq.SkipRetry)
			require.Contains(t, err.Error(), tt.wantFragment)
		})
	}
}

func TestBankFeedsSyncScanFailsClosedOnMissingDependenciesAndStoreErrors(t *testing.T) {
	validStore := bankFeedScheduleStoreFake{connections: []BankFeedScheduleConnection{{ID: 1, CompanyID: 1}}}
	validEnqueue := func(context.Context, int64) error { return nil }
	tests := []struct {
		name    string
		store   BankFeedScheduleStore
		enqueue BankFeedsSyncEnqueuer
		task    *asynq.Task
		wantErr error
	}{
		{name: "nil store", enqueue: validEnqueue, task: NewBankFeedsSyncScanTask(), wantErr: asynq.SkipRetry},
		{name: "nil enqueuer", store: validStore, task: NewBankFeedsSyncScanTask(), wantErr: asynq.SkipRetry},
		{name: "nil task", store: validStore, enqueue: validEnqueue, wantErr: asynq.SkipRetry},
		{name: "store error", store: bankFeedScheduleStoreFake{err: errors.New("database unavailable")}, enqueue: validEnqueue, task: NewBankFeedsSyncScanTask(), wantErr: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := HandleBankFeedsSyncScanTask(tt.store, tt.enqueue)(context.Background(), tt.task)
			require.Error(t, err)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}
