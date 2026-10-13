package shared

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
)

func TestAuditTimeArgZeroIsNil(t *testing.T) {
	if got := auditTimeArg(time.Time{}); got != nil {
		t.Fatalf("zero time must be NULL so the database clock applies, got %v", got)
	}
	now := time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC)
	if got := auditTimeArg(now); got != now {
		t.Fatalf("explicit time must pass through, got %v", got)
	}
}

func TestRecordTxPassesNilTimeWhenUnset(t *testing.T) {
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.ExpectBegin()
	db.ExpectExec("INSERT INTO audit_logs").WithArgs(int64(7), "A", "e", "1", pgxmock.AnyArg(), nil).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectRollback()
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := (&AuditLogger{}).RecordTx(context.Background(), tx, AuditLog{ActorID: 7, Action: "A", Entity: "e", EntityID: "1"}); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(context.Background())
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	db.ExpectBegin()
	at := time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC)
	db.ExpectExec("INSERT INTO audit_logs").WithArgs(int64(7), "A", "e", "1", pgxmock.AnyArg(), at).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectRollback()
	tx, _ = db.Begin(context.Background())
	if err := (&AuditLogger{}).RecordTx(context.Background(), tx, AuditLog{ActorID: 7, Action: "A", Entity: "e", EntityID: "1", At: at}); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(context.Background())
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestRecordTxStoresDatabaseClock runs against a localhost
// database only (skipped otherwise) and rolls back.
func TestRecordTxStoresDatabaseClock(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN is required")
	}
	if u, err := url.Parse(dsn); err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		t.Skip("PG_DSN is not a localhost URL; refusing to run")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// audit_logs.actor_id is NOT NULL and references users, so use a real user.
	var actor int64
	if err := tx.QueryRow(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&actor); err != nil {
		t.Skip("no users in the database")
	}
	if err := (&AuditLogger{}).RecordTx(ctx, tx, AuditLog{ActorID: actor, Action: "TEST_AUDIT_TIME", Entity: "test", EntityID: "1"}); err != nil {
		t.Fatal(err)
	}
	var at time.Time
	if err := tx.QueryRow(ctx, `SELECT occurred_at FROM audit_logs WHERE action='TEST_AUDIT_TIME' ORDER BY id DESC LIMIT 1`).Scan(&at); err != nil {
		t.Fatal(err)
	}
	if at.Year() < 2026 {
		t.Fatalf("occurred_at=%s, want the database clock", at)
	}
}
