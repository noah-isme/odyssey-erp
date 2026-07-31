package leave

import (
	"context"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/approvals"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type leaveAuditFake struct{ logs []shared.AuditLog }

func (a *leaveAuditFake) Record(_ context.Context, log shared.AuditLog) error {
	a.logs = append(a.logs, log)
	return nil
}

func TestFinalizeApprovalUpdatesLeaveBalanceAndAudit(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	audit := &leaveAuditFake{}
	service := &Service{pool: db, audit: audit}
	start := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)

	db.ExpectBegin()
	db.ExpectQuery("SELECT employee_id,leave_type_id,start_date,days,status").WithArgs(int64(55)).
		WillReturnRows(pgxmock.NewRows([]string{"employee_id", "leave_type_id", "start_date", "days", "status"}).
			AddRow(int64(8), int64(2), start, 2.0, "PENDING"))
	db.ExpectExec("UPDATE hr_leave_requests SET status").WithArgs(int64(55), "APPROVED").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("UPDATE hr_leave_balances SET pending").WithArgs(int64(8), int64(2), 2026, 2.0, 2.0).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	err = service.FinalizeApproval(context.Background(), approvals.Request{DocumentID: 55}, approvals.StatusApproved, 99, "ok")
	require.NoError(t, err)
	require.Len(t, audit.logs, 1)
	require.Equal(t, "LEAVE_APPROVED", audit.logs[0].Action)
	require.Equal(t, int64(99), audit.logs[0].ActorID)
	require.NoError(t, db.ExpectationsWereMet())
}
