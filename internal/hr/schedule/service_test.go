package schedule

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type auditFake struct{ logs []shared.AuditLog }

func (a *auditFake) RecordTx(_ context.Context, _ pgx.Tx, log shared.AuditLog) error {
	a.logs = append(a.logs, log)
	return nil
}

const (
	companyA = int64(7)
	companyB = int64(8)
	employee = int64(42)
	actor    = int64(99)
)

var (
	shiftColNames      = []string{"id", "company_id", "code", "name", "start_time", "end_time", "break_minutes", "work_days", "is_active", "created_at"}
	assignmentColNames = append([]string{"id", "employee_id", "shift_id", "valid_from", "valid_to"}, shiftColNames...)
	created            = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
)

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *Service, *auditFake) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(db.Close)
	audit := &auditFake{}
	return db, &Service{pool: db, audit: audit}, audit
}

func q(s string) string { return regexp.QuoteMeta(s) }

func assignmentRow(id, shiftID int64, from time.Time, to *time.Time) *pgxmock.Rows {
	return pgxmock.NewRows(assignmentColNames).AddRow(id, employee, shiftID, from, to,
		shiftID, companyA, "NIGHT", "Night shift", "22:00", "06:00", 30, []int16{1, 2, 3, 4, 5, 6}, true, created)
}

func expectEmployeeLock(db pgxmock.PgxPoolIface, company int64, found bool) {
	e := db.ExpectQuery(q("SELECT e.id FROM hr_employees e WHERE e.id=$1 AND e.company_id=$2 FOR NO KEY UPDATE")).
		WithArgs(employee, company)
	if found {
		e.WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(employee))
	} else {
		e.WillReturnError(pgx.ErrNoRows)
	}
}

func TestAssignShiftClosesPreviousAssignment(t *testing.T) {
	db, svc, audit := newMock(t)
	from := d(2026, 11, 1)

	db.ExpectBegin()
	expectEmployeeLock(db, companyA, true)
	db.ExpectQuery(q("SELECT s.is_active FROM hr_shifts s WHERE s.id=$1 AND s.company_id=$2")).
		WithArgs(int64(5), companyA).WillReturnRows(pgxmock.NewRows([]string{"is_active"}).AddRow(true))
	db.ExpectQuery(q("SELECT es.id, es.valid_from FROM hr_employee_shifts es")).
		WithArgs(companyA, employee, from).WillReturnError(pgx.ErrNoRows)
	db.ExpectQuery(q("UPDATE hr_employee_shifts SET valid_to=$3")+`\s*`+
		q("WHERE company_id=$1 AND employee_id=$2 AND valid_to IS NULL AND valid_from<$4")+`\s*`+q("RETURNING id")).
		WithArgs(companyA, employee, d(2026, 10, 31), from).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(3)))
	db.ExpectQuery(q("INSERT INTO hr_employee_shifts (company_id, employee_id, shift_id, valid_from)")).
		WithArgs(companyA, employee, int64(5), from).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(4)))
	db.ExpectQuery(q("JOIN hr_shifts s ON s.id=es.shift_id AND s.company_id=$1")+`(.|\n)*`+q("AND es.id=$2")).
		WithArgs(companyA, int64(4)).WillReturnRows(assignmentRow(4, 5, from, nil))
	db.ExpectCommit()
	db.ExpectRollback()

	got, err := svc.AssignShift(context.Background(), companyA, employee, 5, from.Add(15*time.Hour), actor)
	require.NoError(t, err)
	require.Equal(t, int64(4), got.ID)
	require.Equal(t, int64(5), got.ShiftID)
	require.Nil(t, got.ValidTo)
	require.Equal(t, "22:00", got.Shift.StartTime)
	require.Equal(t, []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday}, got.Shift.WorkDays)

	require.Len(t, audit.logs, 1)
	log := audit.logs[0]
	require.Equal(t, ActionShiftAssigned, log.Action)
	require.Equal(t, actor, log.ActorID)
	require.Equal(t, "hr_employee", log.Entity)
	require.Equal(t, "42", log.EntityID)
	require.Equal(t, int64(3), log.Meta["closed_assignment_id"])
	require.Equal(t, int64(4), log.Meta["assignment_id"])
	require.Equal(t, int64(5), log.Meta["shift_id"])
	require.Equal(t, "2026-11-01", log.Meta["valid_from"])
	require.NoError(t, db.ExpectationsWereMet())
}

func TestAssignShiftSameDayReplacesShift(t *testing.T) {
	db, svc, audit := newMock(t)
	from := d(2026, 11, 1)

	db.ExpectBegin()
	expectEmployeeLock(db, companyA, true)
	db.ExpectQuery(q("SELECT s.is_active FROM hr_shifts s")).
		WithArgs(int64(6), companyA).WillReturnRows(pgxmock.NewRows([]string{"is_active"}).AddRow(true))
	db.ExpectQuery(q("SELECT es.id, es.valid_from FROM hr_employee_shifts es")).
		WithArgs(companyA, employee, from).
		WillReturnRows(pgxmock.NewRows([]string{"id", "valid_from"}).AddRow(int64(4), from))
	db.ExpectExec(q("UPDATE hr_employee_shifts SET shift_id=$3 WHERE id=$1 AND company_id=$2")).
		WithArgs(int64(4), companyA, int64(6)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectQuery(q("AND es.id=$2")).
		WithArgs(companyA, int64(4)).WillReturnRows(assignmentRow(4, 6, from, nil))
	db.ExpectCommit()
	db.ExpectRollback()

	got, err := svc.AssignShift(context.Background(), companyA, employee, 6, from, actor)
	require.NoError(t, err)
	require.Equal(t, int64(6), got.ShiftID)
	require.Len(t, audit.logs, 1)
	require.NotContains(t, audit.logs[0].Meta, "closed_assignment_id")
	require.NoError(t, db.ExpectationsWereMet())
}

func TestAssignShiftRejectsLaterAssignment(t *testing.T) {
	db, svc, audit := newMock(t)
	from := d(2026, 11, 1)

	db.ExpectBegin()
	expectEmployeeLock(db, companyA, true)
	db.ExpectQuery(q("SELECT s.is_active FROM hr_shifts s")).
		WithArgs(int64(5), companyA).WillReturnRows(pgxmock.NewRows([]string{"is_active"}).AddRow(true))
	db.ExpectQuery(q("SELECT es.id, es.valid_from FROM hr_employee_shifts es")).
		WithArgs(companyA, employee, from).
		WillReturnRows(pgxmock.NewRows([]string{"id", "valid_from"}).AddRow(int64(9), d(2026, 12, 1)))
	db.ExpectRollback()

	_, err := svc.AssignShift(context.Background(), companyA, employee, 5, from, actor)
	require.ErrorIs(t, err, ErrLaterAssignmentExists)
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestAssignShiftRejectsCrossTenantShift(t *testing.T) {
	db, svc, audit := newMock(t)

	// Shift 50 belongs to company B; the session company is A.
	db.ExpectBegin()
	expectEmployeeLock(db, companyA, true)
	db.ExpectQuery(q("SELECT s.is_active FROM hr_shifts s WHERE s.id=$1 AND s.company_id=$2")).
		WithArgs(int64(50), companyA).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()

	_, err := svc.AssignShift(context.Background(), companyA, employee, 50, d(2026, 11, 1), actor)
	require.ErrorIs(t, err, ErrShiftNotFound)
	require.ErrorIs(t, err, ErrNotFound)
	require.Contains(t, err.Error(), "hr: not found")
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestAssignShiftRejectsCrossTenantEmployee(t *testing.T) {
	db, svc, audit := newMock(t)

	db.ExpectBegin()
	expectEmployeeLock(db, companyB, false)
	db.ExpectRollback()

	_, err := svc.AssignShift(context.Background(), companyB, employee, 5, d(2026, 11, 1), actor)
	require.ErrorIs(t, err, ErrEmployeeNotFound)
	require.ErrorIs(t, err, ErrNotFound)
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestAssignShiftRejectsInactiveShift(t *testing.T) {
	db, svc, _ := newMock(t)
	db.ExpectBegin()
	expectEmployeeLock(db, companyA, true)
	db.ExpectQuery(q("SELECT s.is_active FROM hr_shifts s")).
		WithArgs(int64(5), companyA).WillReturnRows(pgxmock.NewRows([]string{"is_active"}).AddRow(false))
	db.ExpectRollback()

	_, err := svc.AssignShift(context.Background(), companyA, employee, 5, d(2026, 11, 1), actor)
	require.ErrorIs(t, err, ErrShiftInactive)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestHolidaysBetweenIsCompanyScoped(t *testing.T) {
	db, svc, _ := newMock(t)
	from := time.Date(2026, 12, 1, 13, 0, 0, 0, time.UTC)
	to := d(2026, 12, 31)

	db.ExpectQuery(q("SELECT h.holiday_date")+`\s*`+q("FROM hr_holidays h")+`\s*`+
		q("WHERE h.company_id=$1 AND h.holiday_date BETWEEN $2 AND $3")+`\s*`+q("ORDER BY h.holiday_date")).
		WithArgs(companyA, d(2026, 12, 1), to).
		WillReturnRows(pgxmock.NewRows([]string{"holiday_date"}).AddRow(d(2026, 12, 25)).AddRow(d(2026, 12, 26)))

	got, err := svc.HolidaysBetween(context.Background(), companyA, from, to)
	require.NoError(t, err)
	require.Equal(t, []time.Time{d(2026, 12, 25), d(2026, 12, 26)}, got)

	// Company B sees only its own rows (none here).
	db.ExpectQuery(q("WHERE h.company_id=$1 AND h.holiday_date BETWEEN $2 AND $3")).
		WithArgs(companyB, d(2026, 12, 1), to).
		WillReturnRows(pgxmock.NewRows([]string{"holiday_date"}))
	got, err = svc.HolidaysBetween(context.Background(), companyB, from, to)
	require.NoError(t, err)
	require.Empty(t, got)

	_, err = svc.HolidaysBetween(context.Background(), companyA, to, from)
	require.ErrorIs(t, err, ErrInvalidRange)
	_, err = svc.HolidaysBetween(context.Background(), 0, from, to)
	require.ErrorIs(t, err, ErrCompanyRequired)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCreateHolidayAuditsAndMapsDuplicate(t *testing.T) {
	db, svc, audit := newMock(t)
	date := d(2026, 12, 25)
	holidayCols := []string{"id", "company_id", "holiday_date", "name", "created_at"}

	db.ExpectBegin()
	db.ExpectQuery(q("INSERT INTO hr_holidays AS h (company_id, holiday_date, name)")).
		WithArgs(companyA, date, "Christmas").
		WillReturnRows(pgxmock.NewRows(holidayCols).AddRow(int64(1), companyA, date, "Christmas", created))
	db.ExpectCommit()
	db.ExpectRollback()

	h, err := svc.CreateHoliday(context.Background(), companyA, date, "  Christmas ", actor)
	require.NoError(t, err)
	require.Equal(t, int64(1), h.ID)
	require.Len(t, audit.logs, 1)
	require.Equal(t, ActionHolidayCreated, audit.logs[0].Action)
	require.Equal(t, "hr_holiday", audit.logs[0].Entity)
	require.Equal(t, "2026-12-25", audit.logs[0].Meta["holiday_date"])

	db.ExpectBegin()
	db.ExpectQuery(q("INSERT INTO hr_holidays")).
		WithArgs(companyA, date, "Christmas").
		WillReturnError(&pgconn.PgError{Code: "23505"})
	db.ExpectRollback()
	_, err = svc.CreateHoliday(context.Background(), companyA, date, "Christmas", actor)
	require.ErrorIs(t, err, ErrHolidayExists)
	require.Len(t, audit.logs, 1)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestDeleteHolidayCrossTenantRejected(t *testing.T) {
	db, svc, audit := newMock(t)
	db.ExpectBegin()
	db.ExpectQuery(q("DELETE FROM hr_holidays WHERE id=$1 AND company_id=$2 RETURNING holiday_date, name")).
		WithArgs(int64(77), companyA).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()

	err := svc.DeleteHoliday(context.Background(), companyA, 77, actor)
	require.ErrorIs(t, err, ErrNotFound)
	require.Empty(t, audit.logs)

	db.ExpectBegin()
	db.ExpectQuery(q("DELETE FROM hr_holidays")).
		WithArgs(int64(1), companyA).
		WillReturnRows(pgxmock.NewRows([]string{"holiday_date", "name"}).AddRow(d(2026, 12, 25), "Christmas"))
	db.ExpectCommit()
	db.ExpectRollback()
	require.NoError(t, svc.DeleteHoliday(context.Background(), companyA, 1, actor))
	require.Len(t, audit.logs, 1)
	require.Equal(t, ActionHolidayDeleted, audit.logs[0].Action)
	require.Equal(t, "1", audit.logs[0].EntityID)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCreateShiftValidatesAndAudits(t *testing.T) {
	db, svc, audit := newMock(t)
	in := ShiftInput{Code: " DAY ", Name: "Day", StartTime: "08:00", EndTime: "17:00", BreakMinutes: 60,
		WorkDays: []time.Weekday{time.Monday, time.Sunday, time.Monday}, IsActive: true}

	db.ExpectBegin()
	db.ExpectQuery(q("INSERT INTO hr_shifts AS s")).
		WithArgs(companyA, "DAY", "Day", "08:00", "17:00", 60, []int16{1, 7}, true).
		WillReturnRows(pgxmock.NewRows(shiftColNames).AddRow(int64(5), companyA, "DAY", "Day", "08:00", "17:00", 60, []int16{1, 7}, true, created))
	db.ExpectCommit()
	db.ExpectRollback()

	sh, err := svc.CreateShift(context.Background(), companyA, in, actor)
	require.NoError(t, err)
	require.Equal(t, []time.Weekday{time.Monday, time.Sunday}, sh.WorkDays)
	require.Len(t, audit.logs, 1)
	require.Equal(t, ActionShiftCreated, audit.logs[0].Action)
	require.Equal(t, "5", audit.logs[0].EntityID)

	bad := []struct {
		in   ShiftInput
		want error
	}{
		{ShiftInput{Name: "x", StartTime: "08:00", EndTime: "17:00", WorkDays: DefaultWorkDays()}, ErrCodeRequired},
		{ShiftInput{Code: "A", StartTime: "08:00", EndTime: "17:00", WorkDays: DefaultWorkDays()}, ErrNameRequired},
		{ShiftInput{Code: "A", Name: "x", StartTime: "8:00", EndTime: "17:00", WorkDays: DefaultWorkDays()}, ErrInvalidTime},
		{ShiftInput{Code: "A", Name: "x", StartTime: "08:00", EndTime: "24:00", WorkDays: DefaultWorkDays()}, ErrInvalidTime},
		{ShiftInput{Code: "A", Name: "x", StartTime: "08:00", EndTime: "17:00", BreakMinutes: -1, WorkDays: DefaultWorkDays()}, ErrInvalidBreak},
		{ShiftInput{Code: "A", Name: "x", StartTime: "08:00", EndTime: "17:00"}, ErrWorkDaysRequired},
	}
	for _, b := range bad {
		_, err := svc.CreateShift(context.Background(), companyA, b.in, actor)
		require.ErrorIs(t, err, b.want)
	}
	require.NoError(t, db.ExpectationsWereMet())
}

func TestUpdateShiftCrossTenantRejected(t *testing.T) {
	db, svc, audit := newMock(t)
	in := ShiftInput{Code: "DAY", Name: "Day", StartTime: "08:00", EndTime: "17:00", WorkDays: DefaultWorkDays(), IsActive: true}

	db.ExpectBegin()
	db.ExpectQuery(q("UPDATE hr_shifts AS s")+`(.|\n)*`+q("WHERE s.id=$1 AND s.company_id=$2")).
		WithArgs(int64(50), companyA, "DAY", "Day", "08:00", "17:00", 0, []int16{1, 2, 3, 4, 5}, true).
		WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()

	_, err := svc.UpdateShift(context.Background(), companyA, 50, in, actor)
	require.ErrorIs(t, err, ErrShiftNotFound)
	require.Contains(t, err.Error(), "hr: not found")
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestWorkDaysFor(t *testing.T) {
	ctx := context.Background()
	date := d(2026, 11, 3)
	sql := q("FROM hr_employees e") + `\s*` + q("LEFT JOIN LATERAL") + `(.|\n)*` + q("WHERE e.id=$2 AND e.company_id=$1")

	t.Run("fallback Mon-Fri without assignment", func(t *testing.T) {
		db, svc, _ := newMock(t)
		db.ExpectQuery(sql).WithArgs(companyA, employee, date).
			WillReturnRows(pgxmock.NewRows([]string{"work_days"}).AddRow([]int16(nil)))
		got, err := svc.WorkDaysFor(ctx, companyA, employee, date)
		require.NoError(t, err)
		require.Equal(t, DefaultWorkDays(), got)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("shift work days", func(t *testing.T) {
		db, svc, _ := newMock(t)
		db.ExpectQuery(sql).WithArgs(companyA, employee, date).
			WillReturnRows(pgxmock.NewRows([]string{"work_days"}).AddRow([]int16{2, 3, 4, 5, 6, 7}))
		got, err := svc.WorkDaysFor(ctx, companyA, employee, date)
		require.NoError(t, err)
		require.Equal(t, []time.Weekday{time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday}, got)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("cross-tenant employee", func(t *testing.T) {
		db, svc, _ := newMock(t)
		db.ExpectQuery(sql).WithArgs(companyB, employee, date).WillReturnError(pgx.ErrNoRows)
		_, err := svc.WorkDaysFor(ctx, companyB, employee, date)
		require.ErrorIs(t, err, ErrEmployeeNotFound)
		require.NoError(t, db.ExpectationsWereMet())
	})
}

func TestShiftAsOfNoAssignment(t *testing.T) {
	db, svc, _ := newMock(t)
	date := d(2026, 11, 3)
	db.ExpectQuery(q("AND es.employee_id=$2 AND es.valid_from<=$3 AND (es.valid_to IS NULL OR es.valid_to>=$3)")).
		WithArgs(companyA, employee, date).WillReturnError(pgx.ErrNoRows)
	_, err := svc.ShiftAsOf(context.Background(), companyA, employee, date)
	require.True(t, errors.Is(err, ErrNoAssignment))
	require.NoError(t, db.ExpectationsWereMet())
}

func TestShiftAssignmentsForEmployeesGroupsByEmployee(t *testing.T) {
	db, svc, _ := newMock(t)
	from, to := d(2026, 10, 1), d(2026, 10, 31)
	ids := []int64{employee, 43}
	rows := pgxmock.NewRows(assignmentColNames).
		AddRow(int64(1), employee, int64(5), d(2026, 1, 1), ptr(d(2026, 10, 14)), int64(5), companyA, "DAY", "Day", "08:00", "17:00", 60, []int16{1, 2, 3, 4, 5}, true, created).
		AddRow(int64(2), employee, int64(6), d(2026, 10, 15), (*time.Time)(nil), int64(6), companyA, "NIGHT", "Night", "22:00", "06:00", 30, []int16{1, 2, 3, 4, 5, 6}, true, created)
	db.ExpectQuery(q("JOIN hr_employees e ON e.id=es.employee_id AND e.company_id=$1")+`(.|\n)*`+
		q("AND es.employee_id = ANY($2) AND es.valid_from<=$4 AND (es.valid_to IS NULL OR es.valid_to>=$3)")).
		WithArgs(companyA, ids, from, to).WillReturnRows(rows)

	got, err := svc.ShiftAssignmentsForEmployees(context.Background(), companyA, ids, from, to)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[employee], 2)
	require.Equal(t, "NIGHT", got[employee][1].Shift.Code)

	empty, err := svc.ShiftAssignmentsForEmployees(context.Background(), companyA, nil, from, to)
	require.NoError(t, err)
	require.Empty(t, empty)
	require.NoError(t, db.ExpectationsWereMet())
}

func ptr(t time.Time) *time.Time { return &t }

func TestListEmployeeShiftsAttachesCurrentAssignment(t *testing.T) {
	db, svc, _ := newMock(t)
	asOf := d(2026, 10, 20)
	db.ExpectQuery(q("FROM hr_employees e") + `(.|\n)*` + q("WHERE e.company_id=$1 AND e.status='ACTIVE'")).
		WithArgs(companyA).
		WillReturnRows(pgxmock.NewRows([]string{"id", "employee_number", "name", "department"}).
			AddRow(int64(43), "E-043", "Alan Turing", "").
			AddRow(employee, "E-042", "Grace Hopper", "Ops"))
	db.ExpectQuery(q("AND es.employee_id = ANY($2)")).
		WithArgs(companyA, []int64{43, employee}, asOf, asOf).
		WillReturnRows(assignmentRow(9, 6, d(2026, 10, 15), nil))

	got, err := svc.ListEmployeeShifts(context.Background(), companyA, asOf)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Nil(t, got[0].Shift)
	require.NotNil(t, got[1].Shift)
	require.Equal(t, "NIGHT", got[1].Shift.Shift.Code)
	require.Equal(t, "Ops", got[1].Department)
	require.NoError(t, db.ExpectationsWereMet())

	_, err = svc.ListEmployeeShifts(context.Background(), 0, asOf)
	require.ErrorIs(t, err, ErrCompanyRequired)
}
