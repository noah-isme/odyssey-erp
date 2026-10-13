package leave

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestLeaveServiceRequiresCompanyWithoutQuerying(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	service := &Service{pool: db}
	ctx := context.Background()
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	calls := map[string]func() error{
		"ListAll":         func() error { _, e := service.ListAll(ctx, 0, LeaveFilter{}); return e },
		"ListAllBalances": func() error { _, e := service.ListAllBalances(ctx, 0, 2026); return e },
		"ListActive":      func() error { _, e := service.ListActiveEmployees(ctx, -1); return e },
		"GetOwnBalances":  func() error { _, e := service.GetOwnBalances(ctx, 0, 42, 2026); return e },
		"Cancel":          func() error { return service.Cancel(ctx, 0, 99, 42) },
		"Submit": func() error {
			_, e := service.Submit(ctx, CreateInput{UserID: 42, LeaveTypeID: 1, StartDate: day, EndDate: day})
			return e
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			require.EqualError(t, call(), "hr: company required")
		})
	}
	require.NoError(t, db.ExpectationsWereMet())
}

func TestLeaveListQueriesScopeByCompany(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	service := &Service{pool: db}

	db.ExpectQuery(`e\.company_id=\$1`).WithArgs(int64(3)).WillReturnRows(pgxmock.NewRows([]string{"id", "employee_number", "name", "dept"}))
	_, err = service.ListActiveEmployees(context.Background(), 3)
	require.NoError(t, err)
	db.ExpectQuery(`e\.company_id=\$1 AND b\.year=\$2`).WithArgs(int64(3), 2026).WillReturnRows(pgxmock.NewRows([]string{"x"}))
	_, err = service.ListAllBalances(context.Background(), 3, 2026)
	require.NoError(t, err)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestSubmitEmployeeLookupIsCompanyScoped(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	service := &Service{pool: db}
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	// Employee belongs to another company: lookup with session company finds nothing.
	db.ExpectBegin()
	db.ExpectQuery(`e\.user_id=\$1 AND e\.company_id=\$2`).WithArgs(int64(42), int64(1)).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()
	_, err = service.Submit(context.Background(), CreateInput{CompanyID: 1, UserID: 42, LeaveTypeID: 1, StartDate: day, EndDate: day})
	require.Error(t, err)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCancelOtherCompanyRequestNotFound(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	service := &Service{pool: db}

	db.ExpectBegin()
	db.ExpectQuery(`e\.company_id=\$3`).WithArgs(int64(99), int64(42), int64(1)).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()
	err = service.Cancel(context.Background(), 1, 99, 42)
	require.EqualError(t, err, "hr: leave request not found or unauthorized")
	require.NoError(t, db.ExpectationsWereMet())
}
