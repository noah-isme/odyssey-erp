package approvals

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

type recordingFinalizer struct {
	request Request
	status  string
	calls   int
}

func (f *recordingFinalizer) FinalizeApproval(_ context.Context, req Request, status string, _ int64, _ string) error {
	f.request, f.status = req, status
	f.calls++
	return nil
}

type recordingNotifier struct {
	assigned  []Request
	assignees []int64
	completed []string
}

func (n *recordingNotifier) Assigned(_ context.Context, userID int64, req Request) error {
	n.assignees = append(n.assignees, userID)
	n.assigned = append(n.assigned, req)
	return nil
}

func (n *recordingNotifier) Escalated(context.Context, int64, Request) error { return nil }

func (n *recordingNotifier) Completed(_ context.Context, _ int64, _ Request, status string) error {
	n.completed = append(n.completed, status)
	return nil
}

func newDecideMock(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	return mock
}

// expectDecisionRecorded covers the shared prefix of Decide: lock the pending
// request at currentStep, lock the actor's assignment and record the decision.
func expectDecisionRecorded(mock pgxmock.PgxPoolIface, requestID, actorID int64, currentStep int, status string) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`FROM approval_requests WHERE id=$1 FOR UPDATE`)).
		WithArgs(requestID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "policy_id", "module", "document_id", "company_id", "amount", "requester_id", "current_step", "status", "submitted_at"}).
			AddRow(requestID, int64(3), "LEAVE", int64(77), (*int64)(nil), float64(2), int64(40), currentStep, StatusPending, time.Now()))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id,policy_step_id FROM approval_assignments`)).
		WithArgs(requestID, currentStep, actorID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "policy_step_id"}).AddRow(int64(10), int64(20)))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE approval_assignments SET status=$2,updated_at=NOW() WHERE id=$1`)).
		WithArgs(int64(10), status).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO approval_decisions`)).
		WithArgs(requestID, int64(10), actorID, pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
}

func expectStepSatisfied(mock pgxmock.PgxPoolIface, requestID int64, currentStep int) {
	// The quorum must count only this request's approvals, not every request on the step.
	mock.ExpectQuery(regexp.QuoteMeta(`ON a.policy_step_id=s.id AND a.request_id=$2 WHERE s.id=$1`)).
		WithArgs(int64(20), requestID).
		WillReturnRows(pgxmock.NewRows([]string{"required_approvals", "count"}).AddRow(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE approval_assignments SET status='CANCELLED',updated_at=NOW() WHERE request_id=$1 AND step_order=$2`)).
		WithArgs(requestID, currentStep).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
}

func TestRepositoryDecideFinalApprovalReturnsApprovedRequest(t *testing.T) {
	mock := newDecideMock(t)
	expectDecisionRecorded(mock, 5, 50, 1, StatusApproved)
	expectStepSatisfied(mock, 5, 1)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT step_order FROM approval_policy_steps`)).
		WithArgs(int64(3), 1).
		WillReturnError(pgx.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE approval_requests SET status='APPROVED'`)).
		WithArgs(int64(5)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()
	mock.ExpectRollback()

	finalizer := &recordingFinalizer{}
	notifier := &recordingNotifier{}
	service := NewService(&Repository{pool: mock}, notifier)
	service.RegisterFinalizer("LEAVE", finalizer)
	result, err := service.Decide(context.Background(), 5, 50, DecisionApprove, "ok")
	require.NoError(t, err)
	require.True(t, result.Finalized)
	require.Equal(t, StatusApproved, result.Request.Status)
	require.Equal(t, 1, finalizer.calls)
	require.Equal(t, StatusApproved, finalizer.status, "finalizer must see the approved status, not the pre-decision PENDING")
	require.Equal(t, StatusApproved, finalizer.request.Status)
	require.Equal(t, []string{StatusApproved}, notifier.completed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryDecideAdvanceReturnsNextStepAndAssignees(t *testing.T) {
	mock := newDecideMock(t)
	expectDecisionRecorded(mock, 6, 50, 1, StatusApproved)
	expectStepSatisfied(mock, 6, 1)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT step_order FROM approval_policy_steps`)).
		WithArgs(int64(3), 1).
		WillReturnRows(pgxmock.NewRows([]string{"step_order"}).AddRow(2))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE approval_requests SET current_step=$2`)).
		WithArgs(int64(6), 2).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT approver_id FROM approval_assignments`)).
		WithArgs(int64(6), 2).
		WillReturnRows(pgxmock.NewRows([]string{"approver_id"}).AddRow(int64(60)))
	mock.ExpectRollback()

	finalizer := &recordingFinalizer{}
	notifier := &recordingNotifier{}
	service := NewService(&Repository{pool: mock}, notifier)
	service.RegisterFinalizer("LEAVE", finalizer)
	result, err := service.Decide(context.Background(), 6, 50, DecisionApprove, "")
	require.NoError(t, err)
	require.True(t, result.Advanced)
	require.False(t, result.Finalized)
	require.Equal(t, StatusPending, result.Request.Status)
	require.Equal(t, 2, result.Request.CurrentStep)
	require.Equal(t, []int64{60}, result.AssignedTo)
	require.Equal(t, []int64{60}, notifier.assignees, "next-step approver must be notified")
	require.Equal(t, 2, notifier.assigned[0].CurrentStep)
	require.Empty(t, notifier.completed)
	require.Zero(t, finalizer.calls)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryDecideUnsatisfiedStepStaysPending(t *testing.T) {
	mock := newDecideMock(t)
	expectDecisionRecorded(mock, 8, 50, 1, StatusApproved)
	mock.ExpectQuery(regexp.QuoteMeta(`ON a.policy_step_id=s.id AND a.request_id=$2 WHERE s.id=$1`)).
		WithArgs(int64(20), int64(8)).
		WillReturnRows(pgxmock.NewRows([]string{"required_approvals", "count"}).AddRow(2, 1))
	mock.ExpectCommit()
	mock.ExpectRollback()

	finalizer := &recordingFinalizer{}
	notifier := &recordingNotifier{}
	service := NewService(&Repository{pool: mock}, notifier)
	service.RegisterFinalizer("LEAVE", finalizer)
	result, err := service.Decide(context.Background(), 8, 50, DecisionApprove, "")
	require.NoError(t, err)
	require.False(t, result.Finalized)
	require.False(t, result.Advanced)
	require.Equal(t, StatusPending, result.Request.Status)
	require.Equal(t, 1, result.Request.CurrentStep)
	require.Empty(t, result.AssignedTo)
	require.Zero(t, finalizer.calls)
	require.Empty(t, notifier.assignees)
	require.Empty(t, notifier.completed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryDecideRejectReturnsRejectedRequest(t *testing.T) {
	mock := newDecideMock(t)
	expectDecisionRecorded(mock, 7, 50, 1, StatusRejected)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE approval_assignments SET status='CANCELLED',updated_at=NOW() WHERE request_id=$1 AND status='PENDING'`)).
		WithArgs(int64(7)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE approval_requests SET status='REJECTED'`)).
		WithArgs(int64(7)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()
	mock.ExpectRollback()

	result, err := (&Repository{pool: mock}).Decide(context.Background(), 7, 50, DecisionReject, "no")
	require.NoError(t, err)
	require.True(t, result.Finalized)
	require.Equal(t, StatusRejected, result.Request.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}
