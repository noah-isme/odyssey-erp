// Package reports computes the HR dashboard and report figures: headcount,
// hires and leavers, turnover, absenteeism, leave liability, overtime cost
// and upcoming contract/probation expiries.
//
// Every query is company-scoped with the company id as $1 and takes dates as
// placeholders. Department and employment type are resolved as of the report
// date through a LATERAL lookup on hr_employee_job_history. No query selects
// national_id, tax_id, date_of_birth or address columns. Money is either
// NUMERIC text (leave liability) or payroll.Money (overtime cost); binary
// floating point is only used for rates.
package reports

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/schedule"
	"github.com/odyssey-erp/odyssey-erp/internal/payroll"
)

// Expiry kinds returned by UpcomingExpiries.
const (
	ExpiryContractEnd  = "CONTRACT_END"
	ExpiryProbationEnd = "PROBATION_END"
)

// Input limits.
const (
	MaxRangeDays      = 1100 // about three years
	MaxExpiryWindow   = 365
	leaveDailyDivisor = 21
)

var (
	ErrCompanyRequired = errors.New("hr: company required")
	ErrDateRequired    = errors.New("hr: date required")
	ErrInvalidRange    = errors.New("hr: end date is before start date")
	ErrRangeTooLong    = fmt.Errorf("hr: report range exceeds %d days", MaxRangeDays)
	ErrInvalidYear     = errors.New("hr: invalid year")
	ErrInvalidWindow   = fmt.Errorf("hr: expiry window must be between 1 and %d days", MaxExpiryWindow)
)

// Schedule is the part of *schedule.Service the reports need. Both methods
// issue exactly one query each.
type Schedule interface {
	HolidaysBetween(ctx context.Context, companyID int64, from, to time.Time) ([]time.Time, error)
	ShiftAssignmentsForEmployees(ctx context.Context, companyID int64, employeeIDs []int64, from, to time.Time) (map[int64][]schedule.ShiftAssignment, error)
}

var _ Schedule = (*schedule.Service)(nil)

type db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Service computes HR reports. It is read-only.
type Service struct {
	pool     db
	schedule Schedule
	now      func() time.Time
}

// NewService builds a Service backed by the pool and the schedule service.
func NewService(pool *pgxpool.Pool, sched Schedule) *Service {
	return &Service{pool: pool, schedule: sched, now: time.Now}
}

// ---- Shared SQL fragments ---------------------------------------------------

// jobAsOf joins the employee's job-history row effective on dateExpr (a
// placeholder or column) and the department it points to. When an employee
// has no history row the current hr_employees values are used.
func jobAsOf(dateExpr string) string {
	return `LEFT JOIN LATERAL (
	SELECT jh.department_id, jh.employment_type, jh.status
	FROM hr_employee_job_history jh
	WHERE jh.company_id=$1 AND jh.employee_id=e.id
	  AND jh.valid_from<=` + dateExpr + ` AND (jh.valid_to IS NULL OR jh.valid_to>=` + dateExpr + `)
	ORDER BY jh.valid_from DESC
	LIMIT 1
) jh ON TRUE
LEFT JOIN hr_departments d ON d.company_id=$1
	AND d.id=CASE WHEN jh.status IS NULL THEN e.department_id ELSE jh.department_id END`
}

// employedOn restricts e to employees on the payroll and active on dateExpr.
func employedOn(dateExpr string) string {
	return `e.hire_date<=` + dateExpr + ` AND (e.termination_date IS NULL OR e.termination_date>=` + dateExpr + `)
	AND COALESCE(jh.status, e.status)='ACTIVE'`
}

// compensationAsOf joins the compensation assignment effective on dateExpr.
// payroll_compensation_assignments has no company_id; it is reached only
// through the company-scoped employee e.
func compensationAsOf(dateExpr string) string {
	return `LEFT JOIN LATERAL (
	SELECT ca.base_salary
	FROM payroll_compensation_assignments ca
	WHERE ca.employee_id=e.id AND ca.effective_from<=` + dateExpr + ` AND (ca.effective_to IS NULL OR ca.effective_to>=` + dateExpr + `)
	ORDER BY ca.effective_from DESC
	LIMIT 1
) c ON TRUE`
}

// ---- Queries ----------------------------------------------------------------

var (
	headcountSQL = `SELECT COALESCE(d.id,0), COALESCE(d.name,''), COALESCE(jh.employment_type, e.employment_type), COUNT(*)
FROM hr_employees e
` + jobAsOf("$2") + `
WHERE e.company_id=$1 AND ` + employedOn("$2") + `
GROUP BY 1, 2, 3
ORDER BY 2, 3`

	hiresLeaversSQL = `SELECT to_char(ev.event_month,'YYYY-MM'), COALESCE(d.id,0), COALESCE(d.name,''), SUM(ev.hires)::bigint, SUM(ev.leavers)::bigint
FROM (
	SELECT h.id AS employee_id, h.hire_date AS event_date, date_trunc('month', h.hire_date)::date AS event_month, 1 AS hires, 0 AS leavers
	FROM hr_employees h
	WHERE h.company_id=$1 AND h.hire_date BETWEEN $2 AND $3
	UNION ALL
	SELECT t.id, t.termination_date, date_trunc('month', t.termination_date)::date, 0, 1
	FROM hr_employees t
	WHERE t.company_id=$1 AND t.termination_date BETWEEN $2 AND $3
) ev
JOIN hr_employees e ON e.id=ev.employee_id AND e.company_id=$1
` + jobAsOf("ev.event_date") + `
GROUP BY ev.event_month, 2, 3
ORDER BY ev.event_month, 3`

	turnoverSQL = `SELECT
	(SELECT COUNT(*) FROM hr_employees l WHERE l.company_id=$1 AND l.termination_date BETWEEN $2 AND $3),
	(SELECT COUNT(*) FROM hr_employees e
` + jobAsOf("$2") + `
	WHERE e.company_id=$1 AND ` + employedOn("$2") + `),
	(SELECT COUNT(*) FROM hr_employees e
` + jobAsOf("$3") + `
	WHERE e.company_id=$1 AND ` + employedOn("$3") + `)`

	// absenteeismEmployeesSQL is the only absenteeism query issued by this
	// package; holidays and shift assignments come from the schedule service
	// in one query each.
	absenteeismEmployeesSQL = `SELECT e.id, e.hire_date, e.termination_date, COALESCE(d.id,0), COALESCE(d.name,''),
	(SELECT COUNT(*) FROM hr_attendance a
	 WHERE a.employee_id=e.id AND a.status='ABSENT' AND a.attendance_date BETWEEN $2 AND $3)
FROM hr_employees e
` + jobAsOf("$3") + `
WHERE e.company_id=$1 AND e.hire_date<=$3 AND (e.termination_date IS NULL OR e.termination_date>=$2)
	AND (e.status='ACTIVE' OR e.termination_date IS NOT NULL)
ORDER BY e.id`

	leaveLiabilitySQL = `SELECT COALESCE(d.id,0), COALESCE(d.name,''),
	COUNT(DISTINCT e.id),
	COUNT(DISTINCT e.id) FILTER (WHERE c.base_salary IS NULL),
	COALESCE(SUM(GREATEST(b.entitled-b.used-b.pending,0)),0)::numeric(18,2)::text,
	COALESCE(ROUND(SUM(GREATEST(b.entitled-b.used-b.pending,0) * c.base_salary / ` + fmt.Sprint(leaveDailyDivisor) + `), 2),0)::numeric(18,2)::text
FROM hr_leave_balances b
JOIN hr_employees e ON e.id=b.employee_id AND e.company_id=$1
JOIN hr_leave_types lt ON lt.id=b.leave_type_id AND (lt.company_id=$1 OR lt.company_id IS NULL)
` + jobAsOf("$3") + `
` + compensationAsOf("$3") + `
WHERE b.year=$2 AND ` + employedOn("$3") + `
GROUP BY 1, 2
ORDER BY 2`

	overtimePostedSQL = `SELECT COALESCE(d.id,0), COALESCE(d.name,''), COALESCE(ROUND(SUM(l.overtime)),0)::bigint
FROM payroll_run_lines l
JOIN payroll_runs r ON r.id=l.run_id AND r.company_id=$1
JOIN payroll_periods p ON p.id=r.period_id AND p.company_id=$1
LEFT JOIN hr_departments d ON d.id=l.department_id AND d.company_id=$1
WHERE r.status='POSTED' AND p.starts_on<=$3 AND p.ends_on>=$2 AND l.overtime<>0
GROUP BY 1, 2
ORDER BY 2`

	overtimePolicySQL = `SELECT pcp.id, pcp.overtime_divisor, pcp.first_hour_multiplier_bps, pcp.subsequent_hour_multiplier_bps, pcp.rounding_unit
FROM payroll_company_policies pcp
WHERE pcp.company_id=$1 AND pcp.effective_from<=$2 AND (pcp.effective_to IS NULL OR pcp.effective_to>=$2)
ORDER BY pcp.effective_from DESC
LIMIT 1`

	// overtimeUnrunSQL selects approved HR overtime not yet covered by a
	// POSTED payroll run of the company.
	overtimeUnrunSQL = `SELECT o.id, COALESCE(d.id,0), COALESCE(d.name,''), o.minutes, ROUND(c.base_salary)::bigint
FROM hr_overtime_requests o
JOIN hr_employees e ON e.id=o.employee_id AND e.company_id=$1
` + jobAsOf("o.worked_on") + `
` + compensationAsOf("o.worked_on") + `
WHERE o.company_id=$1 AND o.status='APPROVED' AND o.worked_on BETWEEN $2 AND $3
	AND NOT EXISTS (
		SELECT 1 FROM payroll_runs r
		JOIN payroll_periods p ON p.id=r.period_id AND p.company_id=$1
		WHERE r.company_id=$1 AND r.status='POSTED' AND o.worked_on BETWEEN p.starts_on AND p.ends_on
	)
ORDER BY o.id`

	expiriesSQL = `SELECT x.employee_id, x.employee_number, x.name, COALESCE(d.id,0), COALESCE(d.name,''), x.kind, x.expires_on
FROM (
	SELECT e.id AS employee_id, e.employee_number, e.name, e.department_id, '` + ExpiryContractEnd + `' AS kind, e.contract_end_date AS expires_on
	FROM hr_employees e
	WHERE e.company_id=$1 AND e.status='ACTIVE' AND e.termination_date IS NULL AND e.contract_end_date BETWEEN $2 AND $3
	UNION ALL
	SELECT p.id, p.employee_number, p.name, p.department_id, '` + ExpiryProbationEnd + `', p.probation_end_date
	FROM hr_employees p
	WHERE p.company_id=$1 AND p.status='ACTIVE' AND p.termination_date IS NULL AND p.probation_end_date BETWEEN $2 AND $3
) x
LEFT JOIN hr_departments d ON d.id=x.department_id AND d.company_id=$1
ORDER BY x.expires_on, x.name, x.kind`
)

// ---- Input validation -------------------------------------------------------

func checkRange(companyID int64, from, to time.Time) (time.Time, time.Time, error) {
	if companyID <= 0 {
		return time.Time{}, time.Time{}, ErrCompanyRequired
	}
	if from.IsZero() || to.IsZero() {
		return time.Time{}, time.Time{}, ErrDateRequired
	}
	from, to = dateOnly(from), dateOnly(to)
	if to.Before(from) {
		return time.Time{}, time.Time{}, ErrInvalidRange
	}
	if to.Sub(from) > MaxRangeDays*24*time.Hour {
		return time.Time{}, time.Time{}, ErrRangeTooLong
	}
	return from, to, nil
}

// ---- Headcount --------------------------------------------------------------

// HeadcountRow is the number of active employees in one department and
// employment type. DepartmentID 0 means no department.
type HeadcountRow struct {
	DepartmentID   int64
	Department     string
	EmploymentType string
	Count          int64
}

// HeadcountReport is the headcount as of a date.
type HeadcountReport struct {
	AsOf  time.Time
	Total int64
	Rows  []HeadcountRow
}

// Headcount counts employees employed and ACTIVE on asOf, grouped by their
// as-of department and employment type.
func (s *Service) Headcount(ctx context.Context, companyID int64, asOf time.Time) (HeadcountReport, error) {
	if companyID <= 0 {
		return HeadcountReport{}, ErrCompanyRequired
	}
	if asOf.IsZero() {
		return HeadcountReport{}, ErrDateRequired
	}
	asOf = dateOnly(asOf)
	rows, err := s.pool.Query(ctx, headcountSQL, companyID, asOf)
	if err != nil {
		return HeadcountReport{}, fmt.Errorf("headcount: %w", err)
	}
	defer rows.Close()
	out := HeadcountReport{AsOf: asOf}
	for rows.Next() {
		var r HeadcountRow
		if err := rows.Scan(&r.DepartmentID, &r.Department, &r.EmploymentType, &r.Count); err != nil {
			return HeadcountReport{}, fmt.Errorf("headcount: %w", err)
		}
		out.Total += r.Count
		out.Rows = append(out.Rows, r)
	}
	if err := rows.Err(); err != nil {
		return HeadcountReport{}, fmt.Errorf("headcount: %w", err)
	}
	return out, nil
}

// ---- Hires and leavers ------------------------------------------------------

// HiresLeaversRow counts hires (by hire_date) and leavers (by
// termination_date) in one month ("YYYY-MM") and department.
type HiresLeaversRow struct {
	Month        string
	DepartmentID int64
	Department   string
	Hires        int64
	Leavers      int64
}

// HiresAndLeaversByMonth returns hires and leavers in [from, to] by month and
// the department each employee belonged to on the event date.
func (s *Service) HiresAndLeaversByMonth(ctx context.Context, companyID int64, from, to time.Time) ([]HiresLeaversRow, error) {
	from, to, err := checkRange(companyID, from, to)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, hiresLeaversSQL, companyID, from, to)
	if err != nil {
		return nil, fmt.Errorf("hires and leavers: %w", err)
	}
	defer rows.Close()
	var out []HiresLeaversRow
	for rows.Next() {
		var r HiresLeaversRow
		if err := rows.Scan(&r.Month, &r.DepartmentID, &r.Department, &r.Hires, &r.Leavers); err != nil {
			return nil, fmt.Errorf("hires and leavers: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("hires and leavers: %w", err)
	}
	return out, nil
}

// ---- Turnover ---------------------------------------------------------------

// TurnoverReport is leavers divided by the average of the opening and
// closing headcount of the range.
type TurnoverReport struct {
	From, To         time.Time
	Leavers          int64
	OpeningHeadcount int64
	ClosingHeadcount int64
	AverageHeadcount float64
	Rate             float64 // fraction, e.g. 0.05 = 5%
}

// Turnover computes the turnover rate for [from, to].
func (s *Service) Turnover(ctx context.Context, companyID int64, from, to time.Time) (TurnoverReport, error) {
	from, to, err := checkRange(companyID, from, to)
	if err != nil {
		return TurnoverReport{}, err
	}
	out := TurnoverReport{From: from, To: to}
	if err := s.pool.QueryRow(ctx, turnoverSQL, companyID, from, to).Scan(&out.Leavers, &out.OpeningHeadcount, &out.ClosingHeadcount); err != nil {
		return TurnoverReport{}, fmt.Errorf("turnover: %w", err)
	}
	out.AverageHeadcount = AverageHeadcount(out.OpeningHeadcount, out.ClosingHeadcount)
	out.Rate = TurnoverRate(out.Leavers, out.OpeningHeadcount, out.ClosingHeadcount)
	return out, nil
}

// ---- Absenteeism ------------------------------------------------------------

// AbsenteeismRow is the absenteeism of one department.
type AbsenteeismRow struct {
	DepartmentID  int64
	Department    string
	Employees     int64
	AbsentDays    int64
	ScheduledDays float64
	Rate          float64
}

// AbsenteeismReport is ABSENT attendance days divided by scheduled working
// days for [from, to].
type AbsenteeismReport struct {
	From, To      time.Time
	Employees     int64
	AbsentDays    int64
	ScheduledDays float64
	Rate          float64
	ByDepartment  []AbsenteeismRow
}

// Absenteeism issues exactly three queries regardless of employee count: the
// employees with their ABSENT counts, the company holidays (once, via
// Schedule.HolidaysBetween) and all shift assignments (once, via
// Schedule.ShiftAssignmentsForEmployees). Scheduled days are then computed in
// memory per employee with schedule.WorkingDays over the employee's
// employment window.
func (s *Service) Absenteeism(ctx context.Context, companyID int64, from, to time.Time) (AbsenteeismReport, error) {
	from, to, err := checkRange(companyID, from, to)
	if err != nil {
		return AbsenteeismReport{}, err
	}
	type employee struct {
		id          int64
		hire        time.Time
		termination *time.Time
		deptID      int64
		dept        string
		absent      int64
	}
	rows, err := s.pool.Query(ctx, absenteeismEmployeesSQL, companyID, from, to)
	if err != nil {
		return AbsenteeismReport{}, fmt.Errorf("absenteeism employees: %w", err)
	}
	var employees []employee
	for rows.Next() {
		var e employee
		if err := rows.Scan(&e.id, &e.hire, &e.termination, &e.deptID, &e.dept, &e.absent); err != nil {
			rows.Close()
			return AbsenteeismReport{}, fmt.Errorf("absenteeism employees: %w", err)
		}
		employees = append(employees, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return AbsenteeismReport{}, fmt.Errorf("absenteeism employees: %w", err)
	}

	holidays, err := s.schedule.HolidaysBetween(ctx, companyID, from, to)
	if err != nil {
		return AbsenteeismReport{}, fmt.Errorf("absenteeism holidays: %w", err)
	}
	ids := make([]int64, len(employees))
	for i, e := range employees {
		ids[i] = e.id
	}
	assignments, err := s.schedule.ShiftAssignmentsForEmployees(ctx, companyID, ids, from, to)
	if err != nil {
		return AbsenteeismReport{}, fmt.Errorf("absenteeism shift assignments: %w", err)
	}

	out := AbsenteeismReport{From: from, To: to}
	byDept := map[int64]*AbsenteeismRow{}
	var order []int64
	for _, e := range employees {
		start, end, ok := employmentWindow(from, to, e.hire, e.termination)
		if !ok {
			continue
		}
		scheduled := ScheduledDays(start, end, assignments[e.id], holidays)
		row, seen := byDept[e.deptID]
		if !seen {
			row = &AbsenteeismRow{DepartmentID: e.deptID, Department: e.dept}
			byDept[e.deptID] = row
			order = append(order, e.deptID)
		}
		row.Employees++
		row.AbsentDays += e.absent
		row.ScheduledDays += scheduled
		out.Employees++
		out.AbsentDays += e.absent
		out.ScheduledDays += scheduled
	}
	for _, id := range order {
		row := byDept[id]
		row.Rate = AbsenteeismRate(row.AbsentDays, row.ScheduledDays)
		out.ByDepartment = append(out.ByDepartment, *row)
	}
	out.Rate = AbsenteeismRate(out.AbsentDays, out.ScheduledDays)
	return out, nil
}

// ---- Leave liability --------------------------------------------------------

// LeaveLiabilityRow is the leave liability of one department. AvailableDays
// and Amount are NUMERIC text with two decimals. EmployeesWithoutSalary
// counts employees with balances but no compensation assignment on the
// valuation date; their days are included in AvailableDays but priced at 0.
type LeaveLiabilityRow struct {
	DepartmentID           int64
	Department             string
	Employees              int64
	EmployeesWithoutSalary int64
	AvailableDays          string
	Amount                 string
}

// LeaveLiabilityReport sums available leave days x base_salary/21.
type LeaveLiabilityReport struct {
	Year          int
	ValuationDate time.Time
	AvailableDays string
	Amount        string
	Rows          []LeaveLiabilityRow
}

// LeaveLiability values the year's available leave balances
// (entitled-used-pending, floored at 0) of employees active on the valuation
// date (today clamped into the year) at base_salary/21 per day, grouped by
// department. All arithmetic is decimal (NUMERIC in SQL, big.Rat in Go).
func (s *Service) LeaveLiability(ctx context.Context, companyID int64, year int) (LeaveLiabilityReport, error) {
	if companyID <= 0 {
		return LeaveLiabilityReport{}, ErrCompanyRequired
	}
	if year < 1900 || year > 9999 {
		return LeaveLiabilityReport{}, ErrInvalidYear
	}
	asOf := valuationDate(year, s.now())
	rows, err := s.pool.Query(ctx, leaveLiabilitySQL, companyID, year, asOf)
	if err != nil {
		return LeaveLiabilityReport{}, fmt.Errorf("leave liability: %w", err)
	}
	defer rows.Close()
	out := LeaveLiabilityReport{Year: year, ValuationDate: asOf}
	totalDays, totalAmount := new(big.Rat), new(big.Rat)
	for rows.Next() {
		var r LeaveLiabilityRow
		if err := rows.Scan(&r.DepartmentID, &r.Department, &r.Employees, &r.EmployeesWithoutSalary, &r.AvailableDays, &r.Amount); err != nil {
			return LeaveLiabilityReport{}, fmt.Errorf("leave liability: %w", err)
		}
		if err := addDecimal(totalDays, r.AvailableDays); err != nil {
			return LeaveLiabilityReport{}, fmt.Errorf("leave liability days: %w", err)
		}
		if err := addDecimal(totalAmount, r.Amount); err != nil {
			return LeaveLiabilityReport{}, fmt.Errorf("leave liability amount: %w", err)
		}
		out.Rows = append(out.Rows, r)
	}
	if err := rows.Err(); err != nil {
		return LeaveLiabilityReport{}, fmt.Errorf("leave liability: %w", err)
	}
	out.AvailableDays = totalDays.FloatString(2)
	out.Amount = totalAmount.FloatString(2)
	return out, nil
}

// addDecimal adds the decimal string s to acc exactly.
func addDecimal(acc *big.Rat, s string) error {
	v, ok := new(big.Rat).SetString(s)
	if !ok {
		return fmt.Errorf("invalid decimal %q", s)
	}
	acc.Add(acc, v)
	return nil
}

// ---- Overtime cost ----------------------------------------------------------

// OvertimeCostRow is the overtime cost of one department.
type OvertimeCostRow struct {
	DepartmentID int64
	Department   string
	Posted       payroll.Money
	Unrun        payroll.Money
	Total        payroll.Money
	UnrunMinutes int64
}

// OvertimeCostReport sums overtime paid by POSTED payroll runs whose period
// overlaps the range plus approved HR overtime in the range not yet covered by
// a POSTED run, priced with payroll.OvertimePay and the company's current
// payroll policy. Each unrun request (one per employee per day) is priced on
// its own with the employee's base salary on the day worked.
// UnpricedMinutes are unrun minutes that could not be priced because the
// company has no current policy or the employee had no compensation
// assignment on that day.
type OvertimeCostReport struct {
	From, To        time.Time
	PolicyID        int64
	PolicyMissing   bool
	Posted          payroll.Money
	Unrun           payroll.Money
	Total           payroll.Money
	UnrunMinutes    int64
	UnpricedMinutes int64
	ByDepartment    []OvertimeCostRow
}

// OvertimeCost computes the overtime cost for [from, to].
func (s *Service) OvertimeCost(ctx context.Context, companyID int64, from, to time.Time) (OvertimeCostReport, error) {
	from, to, err := checkRange(companyID, from, to)
	if err != nil {
		return OvertimeCostReport{}, err
	}
	out := OvertimeCostReport{From: from, To: to}
	byDept := map[int64]*OvertimeCostRow{}
	var order []int64
	row := func(id int64, name string) *OvertimeCostRow {
		r, ok := byDept[id]
		if !ok {
			r = &OvertimeCostRow{DepartmentID: id, Department: name}
			byDept[id] = r
			order = append(order, id)
		}
		return r
	}

	rows, err := s.pool.Query(ctx, overtimePostedSQL, companyID, from, to)
	if err != nil {
		return OvertimeCostReport{}, fmt.Errorf("posted overtime: %w", err)
	}
	for rows.Next() {
		var (
			id     int64
			name   string
			amount int64
		)
		if err := rows.Scan(&id, &name, &amount); err != nil {
			rows.Close()
			return OvertimeCostReport{}, fmt.Errorf("posted overtime: %w", err)
		}
		row(id, name).Posted += payroll.Money(amount)
		out.Posted += payroll.Money(amount)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return OvertimeCostReport{}, fmt.Errorf("posted overtime: %w", err)
	}

	var policy payroll.Policy
	var rounding int64
	err = s.pool.QueryRow(ctx, overtimePolicySQL, companyID, dateOnly(s.now())).Scan(
		&policy.VersionID, &policy.OvertimeDivisor, &policy.FirstHourMultiplierBPS, &policy.SubsequentHourMultiplierBPS, &rounding)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		out.PolicyMissing = true
	case err != nil:
		return OvertimeCostReport{}, fmt.Errorf("overtime policy: %w", err)
	default:
		policy.RoundingUnit = payroll.Money(rounding)
		out.PolicyID = policy.VersionID
	}

	rows, err = s.pool.Query(ctx, overtimeUnrunSQL, companyID, from, to)
	if err != nil {
		return OvertimeCostReport{}, fmt.Errorf("unrun overtime: %w", err)
	}
	for rows.Next() {
		var (
			requestID, deptID, minutes int64
			name                       string
			base                       *int64
		)
		if err := rows.Scan(&requestID, &deptID, &name, &minutes, &base); err != nil {
			rows.Close()
			return OvertimeCostReport{}, fmt.Errorf("unrun overtime: %w", err)
		}
		r := row(deptID, name)
		r.UnrunMinutes += minutes
		out.UnrunMinutes += minutes
		if out.PolicyMissing || base == nil {
			out.UnpricedMinutes += minutes
			continue
		}
		pay := payroll.OvertimePay(payroll.Money(*base), minutes, policy)
		r.Unrun += pay
		out.Unrun += pay
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return OvertimeCostReport{}, fmt.Errorf("unrun overtime: %w", err)
	}

	for _, id := range order {
		r := byDept[id]
		r.Total = r.Posted + r.Unrun
		out.ByDepartment = append(out.ByDepartment, *r)
	}
	out.Total = out.Posted + out.Unrun
	return out, nil
}

// ---- Upcoming expiries ------------------------------------------------------

// Expiry is an active employee whose contract or probation ends soon. Only
// non-sensitive identity columns are returned.
type Expiry struct {
	EmployeeID     int64
	EmployeeNumber string
	Name           string
	DepartmentID   int64
	Department     string
	Kind           string // ExpiryContractEnd or ExpiryProbationEnd
	ExpiresOn      time.Time
}

// UpcomingExpiries lists contract_end_date and probation_end_date values of
// active, non-terminated employees falling within [today, today+withinDays].
func (s *Service) UpcomingExpiries(ctx context.Context, companyID int64, withinDays int) ([]Expiry, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	if withinDays < 1 || withinDays > MaxExpiryWindow {
		return nil, ErrInvalidWindow
	}
	from := dateOnly(s.now())
	to := from.AddDate(0, 0, withinDays)
	rows, err := s.pool.Query(ctx, expiriesSQL, companyID, from, to)
	if err != nil {
		return nil, fmt.Errorf("upcoming expiries: %w", err)
	}
	defer rows.Close()
	var out []Expiry
	for rows.Next() {
		var x Expiry
		if err := rows.Scan(&x.EmployeeID, &x.EmployeeNumber, &x.Name, &x.DepartmentID, &x.Department, &x.Kind, &x.ExpiresOn); err != nil {
			return nil, fmt.Errorf("upcoming expiries: %w", err)
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("upcoming expiries: %w", err)
	}
	return out, nil
}
