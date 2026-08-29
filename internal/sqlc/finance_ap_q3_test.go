package sqlc

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func apExceptionRows() *pgxmock.Rows {
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	return pgxmock.NewRows([]string{
		"id", "ap_invoice_id", "ap_matching_run_id", "exception_type", "severity", "status",
		"owner_id", "sla_due_at", "reason", "evidence", "comments",
		"created_at", "updated_at", "resolved_at", "resolved_by",
	}).AddRow(
		int64(101), int64(11), nil, "MISMATCH", "HIGH", "OPEN",
		nil, nil, "price variance", nil, []string{"review"}, now, now, nil, nil,
	)
}

func TestGetAPExceptionForCompanyScopesThroughInvoice(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	db.ExpectQuery("JOIN ap_invoices i ON i.id = e.ap_invoice_id").
		WithArgs(int64(101), int64(7)).
		WillReturnRows(apExceptionRows())

	exception, err := New(db).GetAPExceptionForCompany(context.Background(), GetAPExceptionForCompanyParams{
		ExceptionID: 101,
		CompanyID:   7,
	})
	require.NoError(t, err)
	require.Equal(t, int64(101), exception.ID)
	require.Equal(t, int64(11), exception.ApInvoiceID)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestListAPExceptionsForCompanyAppliesScopeBeforePagination(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	db.ExpectQuery("JOIN ap_invoices i ON i.id = e.ap_invoice_id").
		WithArgs(int64(7), "OPEN", int64(3), int64(11), int32(20), int32(100)).
		WillReturnRows(apExceptionRows())

	exceptions, err := New(db).ListAPExceptionsForCompany(context.Background(), ListAPExceptionsForCompanyParams{
		CompanyID:  7,
		Status:     "OPEN",
		OwnerID:    3,
		InvoiceID:  11,
		PageOffset: 20,
		PageLimit:  100,
	})
	require.NoError(t, err)
	require.Len(t, exceptions, 1)
	require.Equal(t, int64(11), exceptions[0].ApInvoiceID)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestUpdateAPExceptionStatusForCompanyWritesOnlyScopedRows(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	db.ExpectExec("UPDATE ap_exceptions AS e").
		WithArgs("RESOLVED", pgtype.Int8{Int64: 2, Valid: true}, int64(101), int64(7)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	rows, err := New(db).UpdateAPExceptionStatusForCompany(context.Background(), UpdateAPExceptionStatusForCompanyParams{
		Status:      "RESOLVED",
		ResolvedBy:  pgtype.Int8{Int64: 2, Valid: true},
		ExceptionID: 101,
		CompanyID:   7,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestResolveAPExceptionAppendsResolutionEventAtomically(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	db.ExpectQuery("WITH target AS MATERIALIZED").
		WithArgs("RESOLVED", "quantity confirmed", int64(2), int64(101)).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(501)))

	eventID, err := New(db).ResolveAPException(context.Background(), ResolveAPExceptionParams{
		ToStatus:    "RESOLVED",
		Comment:     "quantity confirmed",
		ResolvedBy:  2,
		ExceptionID: 101,
	})
	require.NoError(t, err)
	require.Equal(t, int64(501), eventID)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestResolveAPExceptionForCompanyScopesTransitionAndEvent(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	db.ExpectQuery("WITH target AS MATERIALIZED").
		WithArgs("REJECTED", "supplier dispute", int64(9), int64(101), int64(7)).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(502)))

	eventID, err := New(db).ResolveAPExceptionForCompany(context.Background(), ResolveAPExceptionForCompanyParams{
		ToStatus:    "REJECTED",
		Comment:     "supplier dispute",
		ResolvedBy:  9,
		ExceptionID: 101,
		CompanyID:   7,
	})
	require.NoError(t, err)
	require.Equal(t, int64(502), eventID)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestResolveAPExceptionReturnsNoRowsForAlreadyTerminalException(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	db.ExpectQuery("WITH target AS MATERIALIZED").
		WithArgs("RESOLVED", "duplicate attempt", int64(2), int64(101)).
		WillReturnError(pgx.ErrNoRows)

	_, err = New(db).ResolveAPException(context.Background(), ResolveAPExceptionParams{
		ToStatus:    "RESOLVED",
		Comment:     "duplicate attempt",
		ResolvedBy:  2,
		ExceptionID: 101,
	})
	require.ErrorIs(t, err, pgx.ErrNoRows)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestListAPExceptionResolutionEventsForCompanyScopesByTenant(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	db.ExpectQuery("FROM ap_exception_resolution_events").
		WithArgs(int64(7), int64(101)).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "ap_exception_id", "company_id", "from_status", "to_status", "comment", "actor_id", "created_at",
		}).AddRow(int64(501), int64(101), int64(7), "IN_REVIEW", "RESOLVED", "quantity confirmed", int64(2), now))

	events, err := New(db).ListAPExceptionResolutionEventsForCompany(context.Background(), ListAPExceptionResolutionEventsForCompanyParams{
		CompanyID:   7,
		ExceptionID: 101,
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "quantity confirmed", events[0].Comment)
	require.Equal(t, int64(7), events[0].CompanyID)
	require.NoError(t, db.ExpectationsWereMet())
}
