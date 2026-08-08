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

func TestRepositoryCreateIsIdempotentForDeliveryKey(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	repo := NewRepository(db)
	createdAt := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt
	db.ExpectQuery("INSERT INTO notifications").
		WithArgs(int64(7), "invoice:12", TypeInvoiceIssued, "Invoice issued", "Ready", "/finance/ar/invoices/12").
		WillReturnRows(pgxmock.NewRows([]string{"id", "recipient_id", "dedupe_key", "type", "title", "body", "url", "read_at", "created_at", "updated_at"}).
			AddRow(int64(42), int64(7), "invoice:12", TypeInvoiceIssued, "Invoice issued", "Ready", "/finance/ar/invoices/12", nil, createdAt, updatedAt))

	n, err := repo.Create(context.Background(), Notification{RecipientID: 7, DedupeKey: "invoice:12", Type: TypeInvoiceIssued, Title: "Invoice issued", Body: "Ready", URL: "/finance/ar/invoices/12"})
	require.NoError(t, err)
	require.Equal(t, int64(42), n.ID)
	require.Equal(t, "invoice:12", n.DedupeKey)
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

func TestRepositoryMarkAllRead(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	repo := NewRepository(db)
	at := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)

	db.ExpectExec("UPDATE notifications SET read_at").WithArgs(int64(7), at).WillReturnResult(pgxmock.NewResult("UPDATE", 2))
	updated, err := repo.MarkAllRead(context.Background(), 7, at)
	require.NoError(t, err)
	require.Equal(t, int64(2), updated)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestRepositoryChannelsUsesPreferenceValues(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	repo := NewRepository(db)

	db.ExpectQuery("SELECT in_app_enabled").WithArgs(int64(3), TypePasswordReset).WillReturnRows(pgxmock.NewRows([]string{"in_app_enabled", "email_enabled", "sms_enabled", "whatsapp_enabled"}).AddRow(false, true, false, false))
	channels, err := repo.Channels(context.Background(), 3, TypePasswordReset)
	require.NoError(t, err)
	require.Equal(t, Channels{InApp: false, Email: true, SMS: false, WhatsApp: false}, channels)
	require.NoError(t, db.ExpectationsWereMet())
}
