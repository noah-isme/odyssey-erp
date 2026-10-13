// Package schedule manages company holidays, work shifts and effective-dated
// employee shift assignments, and answers "which days does this employee
// work" for leave, attendance, overtime and payroll calculations.
//
// Every id is validated with a company-scoped query, and every write is
// audited with RecordTx inside the same transaction as the state change.
package schedule

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Audit actions recorded by this package.
const (
	ActionHolidayCreated = "HOLIDAY_CREATED"
	ActionHolidayDeleted = "HOLIDAY_DELETED"
	ActionShiftCreated   = "SHIFT_CREATED"
	ActionShiftUpdated   = "SHIFT_UPDATED"
	ActionShiftAssigned  = "SHIFT_ASSIGNED"

	entityHoliday  = "hr_holiday"
	entityShift    = "hr_shift"
	entityEmployee = "hr_employee"
)

var (
	// ErrNotFound is returned (possibly wrapped) when an id does not exist in
	// the session company.
	ErrNotFound         = errors.New("hr: not found")
	ErrHolidayNotFound  = fmt.Errorf("%w: holiday", ErrNotFound)
	ErrShiftNotFound    = fmt.Errorf("%w: shift", ErrNotFound)
	ErrEmployeeNotFound = fmt.Errorf("%w: employee", ErrNotFound)
	ErrNoAssignment     = fmt.Errorf("%w: shift assignment as of date", ErrNotFound)

	ErrCompanyRequired       = errors.New("hr: company required")
	ErrEmployeeRequired      = errors.New("hr: employee required")
	ErrDateRequired          = errors.New("hr: date required")
	ErrInvalidRange          = errors.New("hr: end date is before start date")
	ErrNameRequired          = errors.New("hr: name required")
	ErrCodeRequired          = errors.New("hr: shift code required")
	ErrInvalidTime           = errors.New("hr: shift times must be HH:MM")
	ErrInvalidBreak          = errors.New("hr: break minutes must be between 0 and 1440")
	ErrWorkDaysRequired      = errors.New("hr: at least one work day required")
	ErrHolidayExists         = errors.New("hr: a holiday already exists on that date")
	ErrShiftCodeExists       = errors.New("hr: shift code already exists")
	ErrShiftInactive         = errors.New("hr: shift is inactive")
	ErrLaterAssignmentExists = errors.New("hr: a later shift assignment already exists")
	errAuditNotConfigured    = errors.New("hr: schedule audit not configured")
)

var timePattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// Holiday maps an hr_holidays row.
type Holiday struct {
	ID        int64
	CompanyID int64
	Date      time.Time
	Name      string
	CreatedAt time.Time
}

// Shift maps an hr_shifts row. StartTime/EndTime are "HH:MM"; an EndTime
// earlier than StartTime denotes a shift crossing midnight.
type Shift struct {
	ID           int64
	CompanyID    int64
	Code         string
	Name         string
	StartTime    string
	EndTime      string
	BreakMinutes int
	WorkDays     []time.Weekday
	IsActive     bool
	CreatedAt    time.Time
}

// ShiftInput is used by CreateShift and UpdateShift.
type ShiftInput struct {
	Code         string
	Name         string
	StartTime    string
	EndTime      string
	BreakMinutes int
	WorkDays     []time.Weekday
	IsActive     bool
}

// ShiftAssignment maps an hr_employee_shifts row together with its shift.
type ShiftAssignment struct {
	ID         int64
	EmployeeID int64
	ShiftID    int64
	ValidFrom  time.Time
	ValidTo    *time.Time
	Shift      Shift
}

// Audit is satisfied by *shared.AuditLogger.
type Audit interface {
	RecordTx(context.Context, pgx.Tx, shared.AuditLog) error
}

type db interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Begin(context.Context) (pgx.Tx, error)
}

// Service manages holidays, shifts and shift assignments.
type Service struct {
	pool  db
	audit Audit
}

// NewService builds a Service backed by the pool.
func NewService(pool *pgxpool.Pool, audit Audit) *Service {
	return &Service{pool: pool, audit: audit}
}

// ---- Holidays ---------------------------------------------------------------

const holidayCols = `h.id, h.company_id, h.holiday_date, h.name, h.created_at`

func scanHoliday(row pgx.Row) (Holiday, error) {
	var h Holiday
	err := row.Scan(&h.ID, &h.CompanyID, &h.Date, &h.Name, &h.CreatedAt)
	return h, err
}

// ListHolidays returns the company's holidays in the calendar year, by date.
func (s *Service) ListHolidays(ctx context.Context, companyID int64, year int) ([]Holiday, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	from := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC)
	rows, err := s.pool.Query(ctx, `SELECT `+holidayCols+`
FROM hr_holidays h
WHERE h.company_id=$1 AND h.holiday_date BETWEEN $2 AND $3
ORDER BY h.holiday_date`, companyID, from, to)
	if err != nil {
		return nil, fmt.Errorf("list holidays: %w", err)
	}
	defer rows.Close()
	var out []Holiday
	for rows.Next() {
		h, err := scanHoliday(rows)
		if err != nil {
			return nil, fmt.Errorf("list holidays: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list holidays: %w", err)
	}
	return out, nil
}

// HolidaysBetween returns the company's holiday dates in [from, to], sorted.
func (s *Service) HolidaysBetween(ctx context.Context, companyID int64, from, to time.Time) ([]time.Time, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	if from.IsZero() || to.IsZero() {
		return nil, ErrDateRequired
	}
	from, to = dateOnly(from), dateOnly(to)
	if to.Before(from) {
		return nil, ErrInvalidRange
	}
	rows, err := s.pool.Query(ctx, `SELECT h.holiday_date
FROM hr_holidays h
WHERE h.company_id=$1 AND h.holiday_date BETWEEN $2 AND $3
ORDER BY h.holiday_date`, companyID, from, to)
	if err != nil {
		return nil, fmt.Errorf("holidays between: %w", err)
	}
	defer rows.Close()
	var out []time.Time
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("holidays between: %w", err)
		}
		out = append(out, dateOnly(d))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("holidays between: %w", err)
	}
	return out, nil
}

// CreateHoliday adds a company holiday.
func (s *Service) CreateHoliday(ctx context.Context, companyID int64, date time.Time, name string, actorID int64) (Holiday, error) {
	if companyID <= 0 {
		return Holiday{}, ErrCompanyRequired
	}
	if date.IsZero() {
		return Holiday{}, ErrDateRequired
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Holiday{}, ErrNameRequired
	}
	date = dateOnly(date)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Holiday{}, fmt.Errorf("begin create holiday: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	h, err := scanHoliday(tx.QueryRow(ctx, `INSERT INTO hr_holidays AS h (company_id, holiday_date, name)
VALUES ($1, $2, $3)
RETURNING `+holidayCols, companyID, date, name))
	if isUniqueViolation(err) {
		return Holiday{}, ErrHolidayExists
	}
	if err != nil {
		return Holiday{}, fmt.Errorf("insert holiday: %w", err)
	}
	if err := s.record(ctx, tx, actorID, ActionHolidayCreated, entityHoliday, h.ID, map[string]any{
		"holiday_date": date.Format("2006-01-02"),
		"name":         name,
	}); err != nil {
		return Holiday{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Holiday{}, fmt.Errorf("commit create holiday: %w", err)
	}
	return h, nil
}

// DeleteHoliday removes a company holiday.
func (s *Service) DeleteHoliday(ctx context.Context, companyID, holidayID, actorID int64) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if holidayID <= 0 {
		return ErrHolidayNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete holiday: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var (
		date time.Time
		name string
	)
	err = tx.QueryRow(ctx, `DELETE FROM hr_holidays WHERE id=$1 AND company_id=$2 RETURNING holiday_date, name`,
		holidayID, companyID).Scan(&date, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrHolidayNotFound
	}
	if err != nil {
		return fmt.Errorf("delete holiday: %w", err)
	}
	if err := s.record(ctx, tx, actorID, ActionHolidayDeleted, entityHoliday, holidayID, map[string]any{
		"holiday_date": dateOnly(date).Format("2006-01-02"),
		"name":         name,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete holiday: %w", err)
	}
	return nil
}

// ---- Shifts -----------------------------------------------------------------

const shiftCols = `s.id, s.company_id, s.code, s.name, to_char(s.start_time, 'HH24:MI'), to_char(s.end_time, 'HH24:MI'),
	s.break_minutes, s.work_days, s.is_active, s.created_at`

func scanShift(row pgx.Row) (Shift, error) {
	var (
		sh   Shift
		days []int16
	)
	err := row.Scan(&sh.ID, &sh.CompanyID, &sh.Code, &sh.Name, &sh.StartTime, &sh.EndTime,
		&sh.BreakMinutes, &days, &sh.IsActive, &sh.CreatedAt)
	sh.WorkDays = isoToWeekdays(days)
	return sh, err
}

// ListShifts returns the company's shifts ordered by code.
func (s *Service) ListShifts(ctx context.Context, companyID int64) ([]Shift, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	rows, err := s.pool.Query(ctx, `SELECT `+shiftCols+`
FROM hr_shifts s
WHERE s.company_id=$1
ORDER BY s.code`, companyID)
	if err != nil {
		return nil, fmt.Errorf("list shifts: %w", err)
	}
	defer rows.Close()
	var out []Shift
	for rows.Next() {
		sh, err := scanShift(rows)
		if err != nil {
			return nil, fmt.Errorf("list shifts: %w", err)
		}
		out = append(out, sh)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list shifts: %w", err)
	}
	return out, nil
}

// CreateShift adds a shift to the company.
func (s *Service) CreateShift(ctx context.Context, companyID int64, in ShiftInput, actorID int64) (Shift, error) {
	if companyID <= 0 {
		return Shift{}, ErrCompanyRequired
	}
	in, err := normalizeShift(in)
	if err != nil {
		return Shift{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Shift{}, fmt.Errorf("begin create shift: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	sh, err := scanShift(tx.QueryRow(ctx, `INSERT INTO hr_shifts AS s
(company_id, code, name, start_time, end_time, break_minutes, work_days, is_active)
VALUES ($1, $2, $3, $4::time, $5::time, $6, $7::smallint[], $8)
RETURNING `+shiftCols,
		companyID, in.Code, in.Name, in.StartTime, in.EndTime, in.BreakMinutes, weekdaysToISO(in.WorkDays), in.IsActive))
	if isUniqueViolation(err) {
		return Shift{}, ErrShiftCodeExists
	}
	if err != nil {
		return Shift{}, fmt.Errorf("insert shift: %w", err)
	}
	if err := s.record(ctx, tx, actorID, ActionShiftCreated, entityShift, sh.ID, shiftMeta(sh)); err != nil {
		return Shift{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Shift{}, fmt.Errorf("commit create shift: %w", err)
	}
	return sh, nil
}

// UpdateShift replaces a company shift's attributes.
func (s *Service) UpdateShift(ctx context.Context, companyID, shiftID int64, in ShiftInput, actorID int64) (Shift, error) {
	if companyID <= 0 {
		return Shift{}, ErrCompanyRequired
	}
	if shiftID <= 0 {
		return Shift{}, ErrShiftNotFound
	}
	in, err := normalizeShift(in)
	if err != nil {
		return Shift{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Shift{}, fmt.Errorf("begin update shift: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	sh, err := scanShift(tx.QueryRow(ctx, `UPDATE hr_shifts AS s
SET code=$3, name=$4, start_time=$5::time, end_time=$6::time, break_minutes=$7, work_days=$8::smallint[], is_active=$9
WHERE s.id=$1 AND s.company_id=$2
RETURNING `+shiftCols,
		shiftID, companyID, in.Code, in.Name, in.StartTime, in.EndTime, in.BreakMinutes, weekdaysToISO(in.WorkDays), in.IsActive))
	if errors.Is(err, pgx.ErrNoRows) {
		return Shift{}, ErrShiftNotFound
	}
	if isUniqueViolation(err) {
		return Shift{}, ErrShiftCodeExists
	}
	if err != nil {
		return Shift{}, fmt.Errorf("update shift: %w", err)
	}
	if err := s.record(ctx, tx, actorID, ActionShiftUpdated, entityShift, sh.ID, shiftMeta(sh)); err != nil {
		return Shift{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Shift{}, fmt.Errorf("commit update shift: %w", err)
	}
	return sh, nil
}

// ---- Assignments ------------------------------------------------------------

const assignmentCols = `es.id, es.employee_id, es.shift_id, es.valid_from, es.valid_to, ` + shiftCols

// assignmentFrom joins every assignment to its company-owning employee and
// shift; $1 is always the company id.
const assignmentFrom = `FROM hr_employee_shifts es
JOIN hr_employees e ON e.id=es.employee_id AND e.company_id=$1
JOIN hr_shifts s ON s.id=es.shift_id AND s.company_id=$1
WHERE es.company_id=$1`

func scanAssignment(row pgx.Row) (ShiftAssignment, error) {
	var (
		a    ShiftAssignment
		days []int16
	)
	err := row.Scan(&a.ID, &a.EmployeeID, &a.ShiftID, &a.ValidFrom, &a.ValidTo,
		&a.Shift.ID, &a.Shift.CompanyID, &a.Shift.Code, &a.Shift.Name, &a.Shift.StartTime, &a.Shift.EndTime,
		&a.Shift.BreakMinutes, &days, &a.Shift.IsActive, &a.Shift.CreatedAt)
	a.Shift.WorkDays = isoToWeekdays(days)
	return a, err
}

// AssignShift assigns shiftID to the employee from validFrom. In one
// transaction it validates the employee and shift belong to the company,
// closes the employee's open assignment the day before validFrom, and inserts
// the new open-ended assignment. Assigning on the same valid_from as an
// existing assignment replaces its shift; an assignment starting after
// validFrom is rejected with ErrLaterAssignmentExists.
func (s *Service) AssignShift(ctx context.Context, companyID, employeeID, shiftID int64, validFrom time.Time, actorID int64) (ShiftAssignment, error) {
	if companyID <= 0 {
		return ShiftAssignment{}, ErrCompanyRequired
	}
	if employeeID <= 0 {
		return ShiftAssignment{}, ErrEmployeeRequired
	}
	if shiftID <= 0 {
		return ShiftAssignment{}, ErrShiftNotFound
	}
	if validFrom.IsZero() {
		return ShiftAssignment{}, ErrDateRequired
	}
	validFrom = dateOnly(validFrom)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ShiftAssignment{}, fmt.Errorf("begin assign shift: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the employee row to serialise assignment changes per employee.
	var id int64
	err = tx.QueryRow(ctx, `SELECT e.id FROM hr_employees e WHERE e.id=$1 AND e.company_id=$2 FOR NO KEY UPDATE`,
		employeeID, companyID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ShiftAssignment{}, ErrEmployeeNotFound
	}
	if err != nil {
		return ShiftAssignment{}, fmt.Errorf("lock employee: %w", err)
	}

	var active bool
	err = tx.QueryRow(ctx, `SELECT s.is_active FROM hr_shifts s WHERE s.id=$1 AND s.company_id=$2`,
		shiftID, companyID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return ShiftAssignment{}, ErrShiftNotFound
	}
	if err != nil {
		return ShiftAssignment{}, fmt.Errorf("validate shift: %w", err)
	}
	if !active {
		return ShiftAssignment{}, ErrShiftInactive
	}

	var (
		laterID   int64
		laterFrom time.Time
	)
	err = tx.QueryRow(ctx, `SELECT es.id, es.valid_from FROM hr_employee_shifts es
WHERE es.company_id=$1 AND es.employee_id=$2 AND es.valid_from>=$3
ORDER BY es.valid_from DESC
LIMIT 1`, companyID, employeeID, validFrom).Scan(&laterID, &laterFrom)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		laterID = 0
	case err != nil:
		return ShiftAssignment{}, fmt.Errorf("check later shift assignments: %w", err)
	case dateOnly(laterFrom).After(validFrom):
		return ShiftAssignment{}, ErrLaterAssignmentExists
	}

	var assignmentID, closedID int64
	if laterID > 0 {
		// Same start date: replace the shift in place.
		if _, err := tx.Exec(ctx, `UPDATE hr_employee_shifts SET shift_id=$3 WHERE id=$1 AND company_id=$2`,
			laterID, companyID, shiftID); err != nil {
			return ShiftAssignment{}, fmt.Errorf("update shift assignment: %w", err)
		}
		assignmentID = laterID
	} else {
		err = tx.QueryRow(ctx, `UPDATE hr_employee_shifts SET valid_to=$3
WHERE company_id=$1 AND employee_id=$2 AND valid_to IS NULL AND valid_from<$4
RETURNING id`, companyID, employeeID, validFrom.AddDate(0, 0, -1), validFrom).Scan(&closedID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return ShiftAssignment{}, fmt.Errorf("close shift assignment: %w", err)
		}
		if err := tx.QueryRow(ctx, `INSERT INTO hr_employee_shifts (company_id, employee_id, shift_id, valid_from)
VALUES ($1, $2, $3, $4)
RETURNING id`, companyID, employeeID, shiftID, validFrom).Scan(&assignmentID); err != nil {
			return ShiftAssignment{}, fmt.Errorf("insert shift assignment: %w", err)
		}
	}

	out, err := scanAssignment(tx.QueryRow(ctx, `SELECT `+assignmentCols+`
`+assignmentFrom+` AND es.id=$2`, companyID, assignmentID))
	if err != nil {
		return ShiftAssignment{}, fmt.Errorf("load shift assignment: %w", err)
	}

	meta := map[string]any{
		"assignment_id": assignmentID,
		"shift_id":      shiftID,
		"valid_from":    validFrom.Format("2006-01-02"),
	}
	if closedID > 0 {
		meta["closed_assignment_id"] = closedID
	}
	if err := s.record(ctx, tx, actorID, ActionShiftAssigned, entityEmployee, employeeID, meta); err != nil {
		return ShiftAssignment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ShiftAssignment{}, fmt.Errorf("commit assign shift: %w", err)
	}
	return out, nil
}

// ShiftAsOf returns the employee's shift assignment effective on date, or
// ErrNoAssignment.
func (s *Service) ShiftAsOf(ctx context.Context, companyID, employeeID int64, date time.Time) (ShiftAssignment, error) {
	if companyID <= 0 {
		return ShiftAssignment{}, ErrCompanyRequired
	}
	if employeeID <= 0 {
		return ShiftAssignment{}, ErrEmployeeRequired
	}
	if date.IsZero() {
		return ShiftAssignment{}, ErrDateRequired
	}
	a, err := scanAssignment(s.pool.QueryRow(ctx, `SELECT `+assignmentCols+`
`+assignmentFrom+` AND es.employee_id=$2 AND es.valid_from<=$3 AND (es.valid_to IS NULL OR es.valid_to>=$3)
ORDER BY es.valid_from DESC
LIMIT 1`, companyID, employeeID, dateOnly(date)))
	if errors.Is(err, pgx.ErrNoRows) {
		return ShiftAssignment{}, ErrNoAssignment
	}
	if err != nil {
		return ShiftAssignment{}, fmt.Errorf("shift as of: %w", err)
	}
	return a, nil
}

// WorkDaysFor returns the employee's working weekdays on date: the work days
// of the shift assigned on that date, or DefaultWorkDays (Mon-Fri) when the
// employee has no assignment. An employee outside the company yields
// ErrEmployeeNotFound.
func (s *Service) WorkDaysFor(ctx context.Context, companyID, employeeID int64, date time.Time) ([]time.Weekday, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	if employeeID <= 0 {
		return nil, ErrEmployeeRequired
	}
	if date.IsZero() {
		return nil, ErrDateRequired
	}
	var days []int16
	err := s.pool.QueryRow(ctx, `SELECT sh.work_days
FROM hr_employees e
LEFT JOIN LATERAL (
	SELECT s.work_days
	FROM hr_employee_shifts es
	JOIN hr_shifts s ON s.id=es.shift_id AND s.company_id=$1
	WHERE es.company_id=$1 AND es.employee_id=e.id
	  AND es.valid_from<=$3 AND (es.valid_to IS NULL OR es.valid_to>=$3)
	ORDER BY es.valid_from DESC
	LIMIT 1
) sh ON TRUE
WHERE e.id=$2 AND e.company_id=$1`, companyID, employeeID, dateOnly(date)).Scan(&days)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEmployeeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("work days for employee: %w", err)
	}
	if days == nil {
		return DefaultWorkDays(), nil
	}
	return isoToWeekdays(days), nil
}

// ShiftAssignmentsForEmployees batch-loads the assignments overlapping
// [from, to] for the given employees, keyed by employee id and ordered by
// valid_from. Employees without assignments are absent from the map.
func (s *Service) ShiftAssignmentsForEmployees(ctx context.Context, companyID int64, employeeIDs []int64, from, to time.Time) (map[int64][]ShiftAssignment, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	if from.IsZero() || to.IsZero() {
		return nil, ErrDateRequired
	}
	from, to = dateOnly(from), dateOnly(to)
	if to.Before(from) {
		return nil, ErrInvalidRange
	}
	out := make(map[int64][]ShiftAssignment)
	if len(employeeIDs) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT `+assignmentCols+`
`+assignmentFrom+` AND es.employee_id = ANY($2) AND es.valid_from<=$4 AND (es.valid_to IS NULL OR es.valid_to>=$3)
ORDER BY es.employee_id, es.valid_from`, companyID, employeeIDs, from, to)
	if err != nil {
		return nil, fmt.Errorf("batch shift assignments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, fmt.Errorf("batch shift assignments: %w", err)
		}
		out[a.EmployeeID] = append(out[a.EmployeeID], a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("batch shift assignments: %w", err)
	}
	return out, nil
}

// EmployeeShift is an active employee with the shift assigned on a date
// (Shift is nil when the employee has no assignment then).
type EmployeeShift struct {
	EmployeeID     int64
	EmployeeNumber string
	Name           string
	Department     string
	Shift          *ShiftAssignment
}

// ListEmployeeShifts returns the company's ACTIVE employees ordered by name,
// each with the shift assignment effective on asOf.
func (s *Service) ListEmployeeShifts(ctx context.Context, companyID int64, asOf time.Time) ([]EmployeeShift, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	if asOf.IsZero() {
		return nil, ErrDateRequired
	}
	asOf = dateOnly(asOf)
	rows, err := s.pool.Query(ctx, `SELECT e.id, e.employee_number, e.name, COALESCE(d.name,'')
FROM hr_employees e
LEFT JOIN hr_departments d ON d.id=e.department_id AND d.company_id=e.company_id
WHERE e.company_id=$1 AND e.status='ACTIVE'
ORDER BY e.name, e.id`, companyID)
	if err != nil {
		return nil, fmt.Errorf("list employee shifts: %w", err)
	}
	var (
		out []EmployeeShift
		ids []int64
	)
	for rows.Next() {
		var es EmployeeShift
		if err := rows.Scan(&es.EmployeeID, &es.EmployeeNumber, &es.Name, &es.Department); err != nil {
			rows.Close()
			return nil, fmt.Errorf("list employee shifts: %w", err)
		}
		out = append(out, es)
		ids = append(ids, es.EmployeeID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list employee shifts: %w", err)
	}
	if len(ids) == 0 {
		return out, nil
	}
	assignments, err := s.ShiftAssignmentsForEmployees(ctx, companyID, ids, asOf, asOf)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if list := assignments[out[i].EmployeeID]; len(list) > 0 {
			a := list[len(list)-1]
			out[i].Shift = &a
		}
	}
	return out, nil
}

// ---- helpers ----------------------------------------------------------------

func (s *Service) record(ctx context.Context, tx pgx.Tx, actorID int64, action, entity string, entityID int64, meta map[string]any) error {
	if s.audit == nil {
		return errAuditNotConfigured
	}
	if err := s.audit.RecordTx(ctx, tx, shared.AuditLog{
		ActorID:  actorID,
		Action:   action,
		Entity:   entity,
		EntityID: strconv.FormatInt(entityID, 10),
		Meta:     meta,
	}); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}

func shiftMeta(sh Shift) map[string]any {
	return map[string]any{
		"code":          sh.Code,
		"start_time":    sh.StartTime,
		"end_time":      sh.EndTime,
		"break_minutes": sh.BreakMinutes,
		"work_days":     weekdaysToISO(sh.WorkDays),
		"is_active":     sh.IsActive,
	}
}

func normalizeShift(in ShiftInput) (ShiftInput, error) {
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.StartTime = strings.TrimSpace(in.StartTime)
	in.EndTime = strings.TrimSpace(in.EndTime)
	if in.Code == "" {
		return in, ErrCodeRequired
	}
	if in.Name == "" {
		return in, ErrNameRequired
	}
	if !timePattern.MatchString(in.StartTime) || !timePattern.MatchString(in.EndTime) {
		return in, ErrInvalidTime
	}
	if in.BreakMinutes < 0 || in.BreakMinutes > 1440 {
		return in, ErrInvalidBreak
	}
	seen := map[time.Weekday]bool{}
	days := make([]time.Weekday, 0, len(in.WorkDays))
	for _, d := range in.WorkDays {
		if d < time.Sunday || d > time.Saturday || seen[d] {
			continue
		}
		seen[d] = true
		days = append(days, d)
	}
	if len(days) == 0 {
		return in, ErrWorkDaysRequired
	}
	in.WorkDays = days
	return in, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
