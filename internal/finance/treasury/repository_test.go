package treasury

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestCreatePaymentBatchItemAtomicallyLocksAndRechecksInvoiceBalance(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	db.ExpectBegin()
	db.ExpectQuery(`SELECT status\s+FROM treasury_payment_batches\s+WHERE id = \$1\s+FOR UPDATE`).
		WithArgs(int64(20)).
		WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow("DRAFT"))
	db.ExpectQuery(`SELECT i\.total::text`).
		WithArgs(int64(101), int64(20), int64(55)).
		WillReturnRows(pgxmock.NewRows([]string{"total", "paid", "reserved"}).AddRow("100.00", "10.00", "25.00"))
	db.ExpectQuery(`INSERT INTO treasury_payment_batch_items`).
		WithArgs(int64(20), int64(55), int64(77), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "batch_id", "supplier_id", "bank_account_id", "amount", "ap_invoice_id", "status", "created_at",
		}).AddRow(int64(301), int64(20), int64(55), int64(77), "65.00", int64(101), "ACTIVE", time.Now().UTC()))
	db.ExpectCommit()

	repo := &PGRepository{queries: sqlc.New(db), pool: db}
	item, err := repo.CreatePaymentBatchItemAtomically(ctx, PaymentBatchItemCreate{
		BatchID:       20,
		SupplierID:    55,
		BankAccountID: 77,
		Amount:        MustParseAmount("65.00"),
		APInvoiceID:   int64Ptr(101),
	})
	require.NoError(t, err)
	require.Equal(t, int64(301), item.ID)
	require.Equal(t, int64(101), *item.APInvoiceID)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCreatePaymentBatchItemAtomicallyRejectsOverAllocationBeforeInsert(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	db.ExpectBegin()
	db.ExpectQuery(`SELECT status\s+FROM treasury_payment_batches`).
		WithArgs(int64(20)).
		WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow("PENDING_APPROVAL"))
	db.ExpectQuery(`SELECT i\.total::text`).
		WithArgs(int64(101), int64(20), int64(55)).
		WillReturnRows(pgxmock.NewRows([]string{"total", "paid", "reserved"}).AddRow("100.00", "10.00", "25.00"))
	db.ExpectRollback()

	repo := &PGRepository{queries: sqlc.New(db), pool: db}
	_, err = repo.CreatePaymentBatchItemAtomically(ctx, PaymentBatchItemCreate{
		BatchID:       20,
		SupplierID:    55,
		BankAccountID: 77,
		Amount:        MustParseAmount("66.00"),
		APInvoiceID:   int64Ptr(101),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrPaymentInvoiceAllocationLimit))
	require.NoError(t, db.ExpectationsWereMet())
}

func TestAPInvoiceAllocationAvailableIncludesDraftReservations(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	db.ExpectQuery(`(?s)SELECT EXISTS.*b\.status IN \('DRAFT', 'PENDING_APPROVAL'`).
		WithArgs(int64(101), int64(0), int64(7), "IDR", int64(20), "50.00").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))

	repo := &PGRepository{queries: sqlc.New(db), pool: db}
	available, err := repo.APInvoiceAllocationAvailable(
		context.Background(), 101, 0, 7, "idr", 20, MustParseAmount("50.00"),
	)
	require.NoError(t, err)
	require.True(t, available)
	require.NoError(t, db.ExpectationsWereMet())
}
