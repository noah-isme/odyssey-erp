package employees

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestEmployeeReadsRequireCompanyWithoutQuerying(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	service := &Service{pool: db}
	ctx := context.Background()

	calls := map[string]func() error{
		"List":            func() error { _, e := service.List(ctx, 0); return e },
		"GetByID":         func() error { _, e := service.GetByID(ctx, 0, 5); return e },
		"ListDepartments": func() error { _, e := service.ListDepartments(ctx, -1); return e },
		"ListPositions":   func() error { _, e := service.ListPositions(ctx, 0); return e },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			require.EqualError(t, call(), "hr: company required")
		})
	}
	require.NoError(t, db.ExpectationsWereMet())
}

func TestEmployeeQueriesScopeByCompany(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	service := &Service{pool: db}
	ctx := context.Background()

	db.ExpectQuery(`WHERE e\.company_id=\$1`).WithArgs(int64(2)).WillReturnRows(pgxmock.NewRows([]string{"x"}))
	_, err = service.List(ctx, 2)
	require.NoError(t, err)
	db.ExpectQuery(`WHERE d\.company_id=\$1`).WithArgs(int64(2)).WillReturnRows(pgxmock.NewRows([]string{"x"}))
	_, err = service.ListDepartments(ctx, 2)
	require.NoError(t, err)
	db.ExpectQuery(`WHERE p\.company_id=\$1`).WithArgs(int64(2)).WillReturnRows(pgxmock.NewRows([]string{"x"}))
	_, err = service.ListPositions(ctx, 2)
	require.NoError(t, err)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestGetByIDOtherCompanyNotFound(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	service := &Service{pool: db}

	db.ExpectQuery(`e\.id=\$1 AND e\.company_id=\$2`).WithArgs(int64(5), int64(1)).WillReturnError(pgx.ErrNoRows)
	_, err = service.GetByID(context.Background(), 1, 5)
	require.ErrorIs(t, err, pgx.ErrNoRows)
	require.NoError(t, db.ExpectationsWereMet())
}
