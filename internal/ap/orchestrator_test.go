package ap

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/jobs"
)

// orchestratorFakeRepo records the writes ProcessInvoice performs so tests can
// assert idempotency. It embeds memoryAPRepo for the remaining Repository
// methods.
type orchestratorFakeRepo struct {
	*memoryAPRepo
	lockBusy     bool
	lockErr      error
	lockCalls    int
	releaseCalls int
	getCalls     int
	noPolicy     bool
	postErr      error
	runs         []MatchingRun
	exceptions   []APException
	posts        []PostAPInvoiceInput
}

func newOrchestratorFakeRepo() *orchestratorFakeRepo {
	return &orchestratorFakeRepo{memoryAPRepo: newMemoryAPRepo()}
}

func (f *orchestratorFakeRepo) AcquireProcessingLock(context.Context, int64) (func(), bool, error) {
	f.lockCalls++
	if f.lockErr != nil {
		return nil, false, f.lockErr
	}
	if f.lockBusy {
		return nil, false, nil
	}
	return func() { f.releaseCalls++ }, true, nil
}

func (f *orchestratorFakeRepo) ExceptionExists(_ context.Context, invoiceID int64, runID *int64, excType string) (bool, error) {
	for _, exc := range f.exceptions {
		if exc.APInvoiceID != invoiceID || exc.ExceptionType != excType {
			continue
		}
		if (runID == nil) != (exc.APMatchingRunID == nil) {
			continue
		}
		if runID == nil || *runID == *exc.APMatchingRunID {
			return true, nil
		}
	}
	return false, nil
}

func (f *orchestratorFakeRepo) GetAPInvoice(ctx context.Context, id int64) (APInvoice, error) {
	f.getCalls++
	return f.memoryAPRepo.GetAPInvoice(ctx, id)
}

func (f *orchestratorFakeRepo) GetActiveMatchingPolicy(ctx context.Context, companyID, supplierID, categoryID *int64) (*MatchingPolicy, error) {
	if f.noPolicy {
		return nil, ErrMatchingPolicyNotFound
	}
	return f.memoryAPRepo.GetActiveMatchingPolicy(ctx, companyID, supplierID, categoryID)
}

func (f *orchestratorFakeRepo) GetLatestMatchingRun(_ context.Context, invoiceID int64) (*MatchingRun, error) {
	for i := len(f.runs) - 1; i >= 0; i-- {
		if f.runs[i].APInvoiceID == invoiceID {
			run := f.runs[i]
			return &run, nil
		}
	}
	return nil, nil
}

func (f *orchestratorFakeRepo) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	return fn(ctx, &orchestratorFakeTx{memoryAPTx: &memoryAPTx{repo: f.memoryAPRepo}, f: f})
}

func (f *orchestratorFakeRepo) writes() int {
	return len(f.runs) + len(f.exceptions) + len(f.posts)
}

type orchestratorFakeTx struct {
	*memoryAPTx
	f *orchestratorFakeRepo
}

func (t *orchestratorFakeTx) CreateMatchingRun(_ context.Context, run MatchingRun) (int64, error) {
	run.ID = int64(len(t.f.runs) + 100)
	t.f.runs = append(t.f.runs, run)
	return run.ID, nil
}

func (t *orchestratorFakeTx) CreateAPException(_ context.Context, exc APException) (int64, error) {
	t.f.exceptions = append(t.f.exceptions, exc)
	return int64(len(t.f.exceptions)), nil
}

func (t *orchestratorFakeTx) PostAPInvoice(ctx context.Context, input PostAPInvoiceInput) error {
	if t.f.postErr != nil {
		return t.f.postErr
	}
	t.f.posts = append(t.f.posts, input)
	return t.memoryAPTx.PostAPInvoice(ctx, input)
}

func int64Ptr(v int64) *int64 { return &v }

func TestOrchestrator_ProcessInvoice(t *testing.T) {
	const invoiceID = int64(1)
	closedPeriod := errors.New("period closed")
	lockTimeout := context.DeadlineExceeded

	tests := []struct {
		name           string
		setup          func(f *orchestratorFakeRepo)
		createdBy      int64
		wantErr        error
		wantAnyErr     bool
		wantNoGet      bool
		wantRuns       int
		wantExceptions []string
		wantPosts      int
		wantPostedBy   int64
	}{
		{
			name:      "lock busy returns retryable busy error without any call",
			setup:     func(f *orchestratorFakeRepo) { f.lockBusy = true },
			createdBy: 123,
			wantErr:   ErrInvoiceProcessingBusy,
			wantNoGet: true,
		},
		{
			name:       "lock acquire timeout returns error for retry",
			setup:      func(f *orchestratorFakeRepo) { f.lockErr = lockTimeout },
			createdBy:  123,
			wantErr:    lockTimeout,
			wantAnyErr: true,
			wantNoGet:  true,
		},
		{
			name:      "invoice not found",
			setup:     func(f *orchestratorFakeRepo) { delete(f.invoices, invoiceID) },
			createdBy: 123,
			wantErr:   ErrInvoiceNotFound,
		},
		{
			name:      "actor mismatch on draft invoice",
			createdBy: 999,
			wantErr:   ErrActorMismatch,
		},
		{
			name: "actor mismatch on posted invoice",
			setup: func(f *orchestratorFakeRepo) {
				inv := f.invoices[invoiceID]
				inv.Status = APStatusPosted
				f.invoices[invoiceID] = inv
			},
			createdBy: 999,
			wantErr:   ErrActorMismatch,
		},
		{
			name: "payload 0 and creator 0 proceeds with actor 0",
			setup: func(f *orchestratorFakeRepo) {
				inv := f.invoices[invoiceID]
				inv.CreatedBy = 0
				f.invoices[invoiceID] = inv
			},
			createdBy:    0,
			wantRuns:     1,
			wantPosts:    1,
			wantPostedBy: 0,
		},
		{
			name: "legacy NULL creator keeps payload actor",
			setup: func(f *orchestratorFakeRepo) {
				inv := f.invoices[invoiceID]
				inv.CreatedBy = 0
				f.invoices[invoiceID] = inv
			},
			createdBy:    55,
			wantRuns:     1,
			wantPosts:    1,
			wantPostedBy: 55,
		},
		{
			name:         "payload 0 uses recorded creator",
			createdBy:    0,
			wantRuns:     1,
			wantPosts:    1,
			wantPostedBy: 123,
		},
		{
			name: "invoice not draft returns nil",
			setup: func(f *orchestratorFakeRepo) {
				inv := f.invoices[invoiceID]
				inv.Status = APStatusPosted
				f.invoices[invoiceID] = inv
			},
			createdBy: 123,
		},
		{
			name: "existing exception run with resolved mismatch creates nothing",
			setup: func(f *orchestratorFakeRepo) {
				f.runs = []MatchingRun{{ID: 7, APInvoiceID: invoiceID, Status: "EXCEPTION"}}
				f.exceptions = []APException{{APInvoiceID: invoiceID, APMatchingRunID: int64Ptr(7), ExceptionType: "MISMATCH", Status: "RESOLVED"}}
			},
			createdBy:      123,
			wantRuns:       1,
			wantExceptions: []string{"MISMATCH"},
		},
		{
			name: "existing exception run without exception creates one keyed by run",
			setup: func(f *orchestratorFakeRepo) {
				f.runs = []MatchingRun{{ID: 7, APInvoiceID: invoiceID, Status: "EXCEPTION"}}
			},
			createdBy:      123,
			wantRuns:       1,
			wantExceptions: []string{"MISMATCH"},
		},
		{
			name: "existing duplicate review run creates duplicate exception once",
			setup: func(f *orchestratorFakeRepo) {
				f.runs = []MatchingRun{{ID: 8, APInvoiceID: invoiceID, Status: "DUPLICATE_REVIEW"}}
			},
			createdBy:      123,
			wantRuns:       1,
			wantExceptions: []string{"DUPLICATE"},
		},
		{
			name: "existing matched run posts without new run",
			setup: func(f *orchestratorFakeRepo) {
				f.runs = []MatchingRun{{ID: 9, APInvoiceID: invoiceID, Status: "MATCHED"}}
			},
			createdBy:    123,
			wantRuns:     1,
			wantPosts:    1,
			wantPostedBy: 123,
		},
		{
			name: "post failure with existing closed period exception creates nothing",
			setup: func(f *orchestratorFakeRepo) {
				f.postErr = closedPeriod
				f.runs = []MatchingRun{{ID: 9, APInvoiceID: invoiceID, Status: "MATCHED"}}
				f.exceptions = []APException{{APInvoiceID: invoiceID, APMatchingRunID: int64Ptr(9), ExceptionType: "CLOSED_PERIOD", Status: "OPEN"}}
			},
			createdBy:      123,
			wantRuns:       1,
			wantExceptions: []string{"CLOSED_PERIOD"},
		},
		{
			name:         "fresh invoice matched creates one run and posts",
			createdBy:    123,
			wantRuns:     1,
			wantPosts:    1,
			wantPostedBy: 123,
		},
		{
			name: "fresh invoice with variance creates one run and one exception",
			setup: func(f *orchestratorFakeRepo) {
				inv := f.invoices[invoiceID]
				inv.Total = 100
				f.invoices[invoiceID] = inv
			},
			createdBy:      123,
			wantRuns:       1,
			wantExceptions: []string{"MISMATCH"},
		},
		{
			name:           "fresh invoice without policy creates one missing mapping exception",
			setup:          func(f *orchestratorFakeRepo) { f.noPolicy = true },
			createdBy:      123,
			wantExceptions: []string{"MISSING_MAPPING"},
		},
		{
			name: "missing mapping already recorded creates nothing",
			setup: func(f *orchestratorFakeRepo) {
				f.noPolicy = true
				f.exceptions = []APException{{APInvoiceID: invoiceID, ExceptionType: "MISSING_MAPPING", Status: "RESOLVED"}}
			},
			createdBy:      123,
			wantExceptions: []string{"MISSING_MAPPING"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			f := newOrchestratorFakeRepo()
			f.invoices[invoiceID] = APInvoice{ID: invoiceID, Status: APStatusDraft, CreatedBy: 123}
			if tc.setup != nil {
				tc.setup(f)
			}
			before := f.writes()
			orchestrator := NewOrchestrator(NewMatchingService(f), NewExceptionService(f), NewService(f, nil), f)

			err := orchestrator.ProcessInvoice(ctx, invoiceID, tc.createdBy)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Equal(t, before, f.writes(), "error path must not write")
			} else {
				require.NoError(t, err)
			}
			if tc.wantNoGet {
				require.Zero(t, f.getCalls, "invoice must not be loaded without the lock")
				require.Zero(t, f.releaseCalls)
				return
			}
			require.Equal(t, 1, f.lockCalls)
			require.Equal(t, 1, f.releaseCalls, "lock must be released exactly once")
			if tc.wantErr != nil {
				return
			}

			require.Len(t, f.runs, tc.wantRuns)
			var gotTypes []string
			for _, exc := range f.exceptions {
				gotTypes = append(gotTypes, exc.ExceptionType)
				if exc.ExceptionType != "MISSING_MAPPING" {
					require.NotNil(t, exc.APMatchingRunID, "run-derived exceptions are keyed by run")
				}
			}
			require.Equal(t, tc.wantExceptions, gotTypes)
			require.Len(t, f.posts, tc.wantPosts)
			if tc.wantPosts > 0 {
				require.Equal(t, tc.wantPostedBy, f.posts[0].PostedBy)
			}
		})
	}
}

func TestOrchestrator_ProcessInvoiceRedeliveryIsIdempotent(t *testing.T) {
	tests := []struct {
		name           string
		total          float64
		noPolicy       bool
		wantRuns       int
		wantExceptions int
		wantPosts      int
	}{
		{name: "missing mapping", noPolicy: true, wantRuns: 0, wantExceptions: 1},
		{name: "mismatch", total: 100, wantRuns: 1, wantExceptions: 1},
		{name: "matched", wantRuns: 1, wantPosts: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			f := newOrchestratorFakeRepo()
			f.noPolicy = tc.noPolicy
			f.invoices[1] = APInvoice{ID: 1, Status: APStatusDraft, CreatedBy: 123, Total: tc.total}
			orchestrator := NewOrchestrator(NewMatchingService(f), NewExceptionService(f), NewService(f, nil), f)
			for i := 0; i < 3; i++ {
				require.NoError(t, orchestrator.ProcessInvoice(ctx, 1, 123))
			}
			require.Len(t, f.runs, tc.wantRuns)
			require.Len(t, f.exceptions, tc.wantExceptions)
			require.Len(t, f.posts, tc.wantPosts)
		})
	}
}

// A busy lock must be retryable. Returning nil would complete the task even
// when the lock belongs to an orphaned session of a crashed worker, leaving
// the invoice DRAFT with no matching run forever.
func TestOrchestrator_ProcessInvoiceBusyIsRetryable(t *testing.T) {
	ctx := context.Background()
	f := newOrchestratorFakeRepo()
	f.invoices[1] = APInvoice{ID: 1, Status: APStatusDraft, CreatedBy: 123}
	orchestrator := NewOrchestrator(NewMatchingService(f), NewExceptionService(f), NewService(f, nil), f)

	f.lockBusy = true
	for i := 0; i < 3; i++ {
		err := orchestrator.ProcessInvoice(ctx, 1, 123)
		require.Error(t, err, "busy must not look like success")
		require.ErrorIs(t, err, ErrInvoiceProcessingBusy)
		require.NotErrorIs(t, err, asynq.SkipRetry, "busy must be retried, not archived on first delivery")
		require.NotErrorIs(t, err, ErrInvoiceNotFound)
		require.NotErrorIs(t, err, ErrActorMismatch)
	}
	require.Zero(t, f.writes(), "busy deliveries must not write")
	require.Zero(t, f.getCalls, "busy deliveries must not load the invoice")
	require.Zero(t, f.releaseCalls)

	// Once the holder is gone a retry converges: exactly one run and one
	// auto-post, and any later redelivery is a no-op.
	f.lockBusy = false
	require.NoError(t, orchestrator.ProcessInvoice(ctx, 1, 123))
	require.NoError(t, orchestrator.ProcessInvoice(ctx, 1, 123))
	require.Len(t, f.runs, 1)
	require.Len(t, f.posts, 1)
	require.Equal(t, 2, f.releaseCalls)
}

// The busy retries of ap:invoice_process must outlast an orphaned advisory
// lock. The arithmetic is documented at jobs.APInvoiceProcessMaxRetry; this
// test fails if the idle bound, the task timeout, the retry budget or asynq's
// default backoff formula drifts apart.
func TestAPInvoiceBusyRetryWindowOutlastsOrphanLock(t *testing.T) {
	// The documented minimum delay n^4+15s and maximum n^4+15+29*(n+1)s must
	// hold for asynq's real DefaultRetryDelayFunc.
	for n := 0; n <= jobs.APInvoiceProcessMaxRetry; n++ {
		lo := time.Duration(math.Pow(float64(n), 4)+15) * time.Second
		hi := lo + time.Duration(29*(n+1))*time.Second
		for i := 0; i < 500; i++ {
			d := asynq.DefaultRetryDelayFunc(n, errors.New("busy"), nil)
			require.GreaterOrEqual(t, d, lo, "asynq default retry delay for n=%d below the documented minimum", n)
			require.LessOrEqual(t, d, hi, "asynq default retry delay for n=%d above the documented maximum", n)
		}
	}

	require.Greater(t, processingLockIdleTimeout, jobs.APInvoiceProcessTimeout,
		"a healthy holder (bounded by the task timeout) must never be cut off by the idle bound")
	require.LessOrEqual(t, processingLockIdleTimeout, jobs.APInvoiceProcessTimeout+2*time.Minute,
		"the idle bound should stay just above the task timeout")

	var minWindow time.Duration
	for n := 0; n < jobs.APInvoiceProcessMaxRetry; n++ {
		minWindow += time.Duration(math.Pow(float64(n), 4)+15) * time.Second
	}
	const margin = time.Minute
	require.Greater(t, minWindow, processingLockIdleTimeout+margin,
		"even with zero jitter the busy retries must outlast an orphaned lock by %s", margin)
	// One fewer retry would not be enough: MaxRetry is "just enough".
	require.LessOrEqual(t, minWindow-time.Duration(math.Pow(float64(jobs.APInvoiceProcessMaxRetry-1), 4)+15)*time.Second,
		processingLockIdleTimeout+margin)
}
