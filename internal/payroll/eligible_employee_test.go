package payroll

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// TestPayrollEligibleEmployeeSQL evaluates the real filter fragment used by
// Calculate against literal rows, so no schema or data is needed. It needs a
// localhost PG_DSN (never a remote database) and is skipped otherwise.
func TestPayrollEligibleEmployeeSQL(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN is required")
	}
	if u, err := url.Parse(dsn); err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") || strings.Contains(dsn, "host=") {
		t.Skip("PG_DSN is not a localhost URL; refusing to run")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)

	// period starts 2026-05-01
	rows := `(VALUES
	  ('active',                NULL::date,       'ACTIVE'),
	  ('leaver this period',    DATE '2026-05-20', 'INACTIVE'),
	  ('leaver last period',    DATE '2026-04-30', 'INACTIVE'),
	  ('offboarded, not rehired', DATE '2026-03-31', 'INACTIVE'),
	  ('rehired (termination cleared)', NULL::date, 'ACTIVE'),
	  ('never active',          NULL::date,       'INACTIVE')
	) e(label, termination_date, status) CROSS JOIN (SELECT DATE '2026-05-01' AS starts_on) p`
	r, err := conn.Query(ctx, `SELECT e.label FROM `+rows+` WHERE `+payrollEligibleEmployeeSQL+` ORDER BY e.label`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(r, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"active", "leaver this period", "rehired (termination cleared)"}
	if len(got) != len(want) {
		t.Fatalf("eligible=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("eligible=%v want %v", got, want)
		}
	}
}
