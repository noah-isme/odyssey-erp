package procurement

import (
	"context"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestTxRepositoryValidateGRNQuantitiesUsesLockedCumulativeRows(t *testing.T) {
	db, err := pgxmock.NewConn()
	require.NoError(t, err)
	defer func() { _ = db.Close(context.Background()) }()

	db.ExpectQuery(`SELECT status, supplier_id FROM pos WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(pgxmock.NewRows([]string{"status", "supplier_id"}).AddRow("APPROVED", int64(7)))
	db.ExpectQuery(`WITH ordered`).
		WithArgs(int64(1)).
		WillReturnRows(pgxmock.NewRows([]string{"product_id", "ordered", "received"}).AddRow(int64(11), "5.0000", "4.0000"))

	tx := &txRepo{tx: db}
	err = tx.ValidateGRNQuantities(context.Background(), 1, 7, []GRNLineInput{{ProductID: 11, Qty: 1}})
	require.NoError(t, err)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestTxRepositoryValidateGRNQuantitiesRejectsCrossGRNOverReceipt(t *testing.T) {
	db, err := pgxmock.NewConn()
	require.NoError(t, err)
	defer func() { _ = db.Close(context.Background()) }()

	db.ExpectQuery(`SELECT status, supplier_id FROM pos WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(pgxmock.NewRows([]string{"status", "supplier_id"}).AddRow("APPROVED", int64(7)))
	db.ExpectQuery(`WITH ordered`).
		WithArgs(int64(1)).
		WillReturnRows(pgxmock.NewRows([]string{"product_id", "ordered", "received"}).AddRow(int64(11), "5.0000", "4.0000"))

	tx := &txRepo{tx: db}
	err = tx.ValidateGRNQuantities(context.Background(), 1, 7, []GRNLineInput{{ProductID: 11, Qty: 1.0001}})
	require.ErrorContains(t, err, "quantity for product 11 exceeds PO quantity")
	require.NoError(t, db.ExpectationsWereMet())
}
