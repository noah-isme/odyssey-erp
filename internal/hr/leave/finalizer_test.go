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
	company := int64(7)
	start := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)

	db.ExpectBegin()
	db.ExpectQuery("SELECT r.employee_id,r.leave_type_id,r.start_date,r.days,r.status,e.company_id").WithArgs(int64(55)).
		WillReturnRows(pgxmock.NewRows([]string{"employee_id", "leave_type_id", "start_date", "days", "status", "company_id"}).
			AddRow(int64(8), int64(2), start, 2.0, "PENDING", int64(7)))
	db.ExpectExec("UPDATE hr_leave_requests SET status").WithArgs(int64(55), "APPROVED").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("UPDATE hr_leave_balances SET pending").WithArgs(int64(8), int64(2), 2026, 2.0, 2.0).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	err = service.FinalizeApproval(context.Background(), approvals.Request{DocumentID: 55, CompanyID: &company}, approvals.StatusApproved, 99, "ok")
	require.NoError(t, err)
	require.Len(t, audit.logs, 1)
	require.Equal(t, "LEAVE_APPROVED", audit.logs[0].Action)
	require.Equal(t, int64(99), audit.logs[0].ActorID)
	require.NoError(t, db.ExpectationsWereMet())
}

var finalizeCols = []string{"employee_id", "leave_type_id", "start_date", "days", "status", "company_id"}

func TestFinalizeApprovalSecondCallIsNoOp(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	audit := &leaveAuditFake{}
	service := &Service{pool: db, audit: audit}
	company := int64(7)
	start := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)

	db.ExpectBegin()
	db.ExpectQuery("SELECT r.employee_id").WithArgs(int64(55)).
		WillReturnRows(pgxmock.NewRows(finalizeCols).AddRow(int64(8), int64(2), start, 2.0, "APPROVED", int64(7)))
	db.ExpectRollback()

	err = service.FinalizeApproval(context.Background(), approvals.Request{DocumentID: 55, CompanyID: &company}, approvals.StatusApproved, 99, "again")
	require.NoError(t, err)
	require.Len(t, audit.logs, 1)
	require.Equal(t, "LEAVE_FINALIZE_SKIPPED", audit.logs[0].Action)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestFinalizeApprovalRejectsCompanyMismatch(t *testing.T) {
	other, mine := int64(9), int64(7)
	for name, req := range map[string]approvals.Request{
		"different company": {DocumentID: 55, CompanyID: &other},
		"missing company":   {DocumentID: 55},
	} {
		t.Run(name, func(t *testing.T) {
			db, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer db.Close()
			service := &Service{pool: db}
			start := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)

			db.ExpectBegin()
			db.ExpectQuery("SELECT r.employee_id").WithArgs(int64(55)).
				WillReturnRows(pgxmock.NewRows(finalizeCols).AddRow(int64(8), int64(2), start, 2.0, "PENDING", mine))
			db.ExpectRollback()

			err = service.FinalizeApproval(context.Background(), req, approvals.StatusApproved, 99, "")
			require.EqualError(t, err, "hr: approval company does not match leave request")
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}
