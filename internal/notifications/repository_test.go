package notifications

import (
	"context"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestRepositoryUnreadCountAndMarkRead(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	repo := NewRepository(db)

	db.ExpectQuery("SELECT COUNT").WithArgs(int64(7)).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(2)))
	count, err := repo.UnreadCount(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)

	at := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	db.ExpectExec("UPDATE notifications SET read_at").WithArgs(int64(11), int64(7), at).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	updated, err := repo.MarkRead(context.Background(), 7, 11, at)
	require.NoError(t, err)
	require.True(t, updated)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestRepositoryChannelsDefaultsWhenPreferenceMissing(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	repo := NewRepository(db)
	db.ExpectQuery("SELECT in_app_enabled").WithArgs(int64(3), TypeInvoiceIssued).WillReturnRows(pgxmock.NewRows([]string{"in_app_enabled", "email_enabled"}))
	channels, err := repo.Channels(context.Background(), 3, TypeInvoiceIssued)
	require.NoError(t, err)
	require.Equal(t, Channels{InApp: true, Email: true}, channels)
}
