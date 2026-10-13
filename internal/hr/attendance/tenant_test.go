package attendance

import (
	"context"
	"strings"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestAttendanceServiceRequiresCompanyWithoutQuerying(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	service := &Service{pool: db}
	ctx := context.Background()

	calls := map[string]func() error{
		"ListRecords":     func() error { _, _, e := service.ListRecords(ctx, 0, AttendanceFilter{}); return e },
		"ListEmployees":   func() error { _, e := service.ListEmployees(ctx, 0); return e },
		"ListDepartments": func() error { _, e := service.ListDepartments(ctx, -1); return e },
		"Recent":          func() error { _, e := service.Recent(ctx, 0); return e },
		"Import": func() error {
			_, e := service.Import(ctx, 0, 1, "a.csv", strings.NewReader(""))
			return e
		},
		"RecordManual": func() error {
			return service.RecordManual(ctx, 0, ManualRecordInput{EmployeeID: 1, Date: time.Now()})
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			require.EqualError(t, call(), "hr: company required")
		})
	}
	require.NoError(t, db.ExpectationsWereMet())
}

func TestAttendanceListQueriesScopeByCompany(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	service := &Service{pool: db}
	ctx := context.Background()

	db.ExpectQuery(`WHERE e\.company_id=\$1 AND e\.status='ACTIVE'`).WithArgs(int64(4)).WillReturnRows(pgxmock.NewRows([]string{"x"}))
	_, err = service.ListEmployees(ctx, 4)
	require.NoError(t, err)
	db.ExpectQuery(`WHERE company_id=\$1`).WithArgs(int64(4)).WillReturnRows(pgxmock.NewRows([]string{"x"}))
	_, err = service.ListDepartments(ctx, 4)
	require.NoError(t, err)
	require.NoError(t, db.ExpectationsWereMet())
}
