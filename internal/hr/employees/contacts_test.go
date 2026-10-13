package employees

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

var contactColNames = []string{"id", "employee_id", "name", "relationship", "phone", "email", "is_primary", "created_at", "updated_at"}

func TestListContactsScopedThroughEmployee(t *testing.T) {
	db, service, _ := newMock(t)
	now := time.Now()
	db.ExpectQuery(q("FROM hr_employee_emergency_contacts c JOIN hr_employees e ON e.id=c.employee_id AND e.company_id=$1 WHERE c.employee_id=$2")).
		WithArgs(int64(2), int64(8)).
		WillReturnRows(pgxmock.NewRows(contactColNames).AddRow(int64(1), int64(8), "Bob", "Spouse", "0812", "", true, now, now))
	got, err := service.ListContacts(context.Background(), 2, 8)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.True(t, got[0].IsPrimary)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestUpsertContact(t *testing.T) {
	now := time.Now()
	lockEmployee := func(db pgxmock.PgxPoolIface) {
		db.ExpectQuery(q("SELECT id FROM hr_employees WHERE id=$1 AND company_id=$2 FOR UPDATE")).WithArgs(int64(8), int64(2)).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(8)))
	}
	tests := []struct {
		name    string
		in      ContactInput
		setup   func(db pgxmock.PgxPoolIface)
		wantErr string
	}{
		{
			name: "insert primary clears others",
			in:   ContactInput{Name: " Bob ", Relationship: "Spouse", Phone: "0812", IsPrimary: true, ActorID: 99},
			setup: func(db pgxmock.PgxPoolIface) {
				db.ExpectBegin()
				lockEmployee(db)
				db.ExpectQuery(q("INSERT INTO hr_employee_emergency_contacts")).WithArgs(int64(2), int64(8), "Bob", "Spouse", "0812", "", true).
					WillReturnRows(pgxmock.NewRows(contactColNames).AddRow(int64(11), int64(8), "Bob", "Spouse", "0812", "", true, now, now))
				db.ExpectExec(q("UPDATE hr_employee_emergency_contacts SET is_primary=FALSE")).WithArgs(int64(8), int64(11)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				db.ExpectCommit()
				db.ExpectRollback()
			},
		},
		{
			name: "update contact of another employee or company is not found",
			in:   ContactInput{ID: 12, Name: "Eve"},
			setup: func(db pgxmock.PgxPoolIface) {
				db.ExpectBegin()
				lockEmployee(db)
				db.ExpectQuery(q("UPDATE hr_employee_emergency_contacts c SET")+`(.|\n)*`+q("FROM hr_employees e WHERE e.id=c.employee_id AND e.company_id=$6 AND c.employee_id=$7 AND c.id=$8")).
					WithArgs("Eve", "", "", "", false, int64(2), int64(8), int64(12)).WillReturnError(pgx.ErrNoRows)
				db.ExpectRollback()
			},
			wantErr: "hr: not found: emergency contact",
		},
		{
			name: "cross-tenant employee",
			in:   ContactInput{Name: "Eve"},
			setup: func(db pgxmock.PgxPoolIface) {
				db.ExpectBegin()
				db.ExpectQuery(q("SELECT id FROM hr_employees WHERE id=$1 AND company_id=$2 FOR UPDATE")).WithArgs(int64(8), int64(2)).WillReturnError(pgx.ErrNoRows)
				db.ExpectRollback()
			},
			wantErr: "hr: not found: employee",
		},
		{name: "name required", in: ContactInput{Name: " "}, setup: func(pgxmock.PgxPoolIface) {}, wantErr: "hr: invalid emergency contact"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, service, audit := newMock(t)
			tc.setup(db)
			got, err := service.UpsertContact(context.Background(), 2, 8, tc.in)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				require.Empty(t, audit.logs)
			} else {
				require.NoError(t, err)
				require.Equal(t, int64(11), got.ID)
				require.Len(t, audit.logs, 1)
				require.Equal(t, ActionEmployeeContactUpsert, audit.logs[0].Action)
				requireNoPII(t, audit.logs, "Bob", "0812", "Spouse")
			}
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}

func TestDeleteContact(t *testing.T) {
	for _, tc := range []struct {
		name     string
		affected int64
		wantErr  string
	}{
		{name: "deleted", affected: 1},
		{name: "other company not found", affected: 0, wantErr: "hr: not found: emergency contact"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, service, audit := newMock(t)
			db.ExpectBegin()
			db.ExpectExec(q("DELETE FROM hr_employee_emergency_contacts c USING hr_employees e WHERE e.id=c.employee_id AND e.company_id=$1 AND c.employee_id=$2 AND c.id=$3")).
				WithArgs(int64(2), int64(8), int64(11)).WillReturnResult(pgxmock.NewResult("DELETE", tc.affected))
			if tc.wantErr != "" {
				db.ExpectRollback()
			} else {
				db.ExpectCommit()
				db.ExpectRollback()
			}
			err := service.DeleteContact(context.Background(), 2, 8, 11, 99)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				require.Empty(t, audit.logs)
			} else {
				require.NoError(t, err)
				require.Equal(t, ActionEmployeeContactDeleted, audit.logs[0].Action)
				require.Equal(t, map[string]any{"contact_id": int64(11)}, audit.logs[0].Meta)
			}
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}
