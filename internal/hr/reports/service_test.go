package reports

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/schedule"
	"github.com/odyssey-erp/odyssey-erp/internal/payroll"
)

const companyA = int64(7)

var (
	fixedNow  = time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)
	rangeFrom = day(2026, 6, 1)
	rangeTo   = day(2026, 6, 30)
)

// sqlRecorder is a pgxmock QueryMatcher that matches when the actual SQL
// contains the expected substring, and records every SQL string issued.
type sqlRecorder struct {
	mu   sync.Mutex
	seen []string
}

func (r *sqlRecorder) Match(expected, actual string) error {
	r.mu.Lock()
	r.seen = append(r.seen, actual)
	r.mu.Unlock()
	if !strings.Contains(actual, expected) {
		return fmt.Errorf("query %q does not contain %q", actual, expected)
	}
	return nil
}

func (r *sqlRecorder) queries() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	// pgxmock may call the matcher more than once per statement; dedupe
	// consecutive duplicates so each issued query appears once.
	var out []string
	for i, q := range r.seen {
		if i > 0 && r.seen[i-1] == q {
			continue
		}
		out = append(out, q)
	}
	return out
}

func newMock(t *testing.T, sched Schedule) (pgxmock.PgxPoolIface, *Service, *sqlRecorder) {
	t.Helper()
	rec := &sqlRecorder{}
	mock, err := pgxmock.NewPool(pgxmock.QueryMatcherOption(pgxmock.QueryMatcherFunc(rec.Match)))
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	return mock, &Service{pool: mock, schedule: sched, now: func() time.Time { return fixedNow }}, rec
}

// assertSafeQuery checks the invariants every report query must satisfy.
func assertSafeQuery(t *testing.T, sql string) {
	t.Helper()
	require.Contains(t, sql, "company_id=$1")
	lower := strings.ToLower(sql)
	for _, banned := range []string{"national_id", "tax_id", "date_of_birth", "address", "bank_account", "e.*", "select *"} {
		require.NotContains(t, lower, banned)
	}
}

func TestAllQueriesAreCompanyScopedAndSafe(t *testing.T) {
	all := map[string]string{
		"headcount":   headcountSQL,
		"hires":       hiresLeaversSQL,
		"turnover":    turnoverSQL,
		"absenteeism": absenteeismEmployeesSQL,
		"leave":       leaveLiabilitySQL,
		"otPosted":    overtimePostedSQL,
		"otPolicy":    overtimePolicySQL,
		"otUnrun":     overtimeUnrunSQL,
		"expiries":    expiriesSQL,
	}
	for name, sql := range all {
		t.Run(name, func(t *testing.T) {
			assertSafeQuery(t, sql)
			require.Contains(t, sql, "$2", "date/year placeholder")
		})
	}
	// Queries over a date range use $2 and $3 as dates.
	for _, sql := range []string{hiresLeaversSQL, turnoverSQL, absenteeismEmployeesSQL, overtimePostedSQL, overtimeUnrunSQL, expiriesSQL, leaveLiabilitySQL} {
		require.Contains(t, sql, "$3")
	}
	// As-of department/employment type through job history.
	for _, sql := range []string{headcountSQL, hiresLeaversSQL, turnoverSQL, absenteeismEmployeesSQL, leaveLiabilitySQL, overtimeUnrunSQL} {
		require.Contains(t, sql, "LEFT JOIN LATERAL")
		require.Contains(t, sql, "FROM hr_employee_job_history jh")
		require.Contains(t, sql, "jh.company_id=$1")
	}
}

func TestCrossTenantNegativeNoQuery(t *testing.T) {
	sched := &fakeSchedule{}
	mock, svc, _ := newMock(t, sched)
	ctx := context.Background()
	for _, companyID := range []int64{0, -1} {
		_, err := svc.Headcount(ctx, companyID, rangeTo)
		require.ErrorIs(t, err, ErrCompanyRequired)
		_, err = svc.HiresAndLeaversByMonth(ctx, companyID, rangeFrom, rangeTo)
		require.ErrorIs(t, err, ErrCompanyRequired)
		_, err = svc.Turnover(ctx, companyID, rangeFrom, rangeTo)
		require.ErrorIs(t, err, ErrCompanyRequired)
		_, err = svc.Absenteeism(ctx, companyID, rangeFrom, rangeTo)
		require.ErrorIs(t, err, ErrCompanyRequired)
		_, err = svc.LeaveLiability(ctx, companyID, 2026)
		require.ErrorIs(t, err, ErrCompanyRequired)
		_, err = svc.OvertimeCost(ctx, companyID, rangeFrom, rangeTo)
		require.ErrorIs(t, err, ErrCompanyRequired)
		_, err = svc.UpcomingExpiries(ctx, companyID, 30)
		require.ErrorIs(t, err, ErrCompanyRequired)
	}
	require.Zero(t, sched.holidayCalls+sched.assignmentCalls)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInputValidation(t *testing.T) {
	mock, svc, _ := newMock(t, &fakeSchedule{})
	ctx := context.Background()
	_, err := svc.Headcount(ctx, companyA, time.Time{})
	require.ErrorIs(t, err, ErrDateRequired)
	_, err = svc.Turnover(ctx, companyA, rangeTo, rangeFrom)
	require.ErrorIs(t, err, ErrInvalidRange)
	_, err = svc.Absenteeism(ctx, companyA, day(2020, 1, 1), day(2026, 1, 1))
	require.ErrorIs(t, err, ErrRangeTooLong)
	_, err = svc.LeaveLiability(ctx, companyA, 0)
	require.ErrorIs(t, err, ErrInvalidYear)
	_, err = svc.UpcomingExpiries(ctx, companyA, 0)
	require.ErrorIs(t, err, ErrInvalidWindow)
	_, err = svc.UpcomingExpiries(ctx, companyA, 400)
	require.ErrorIs(t, err, ErrInvalidWindow)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHeadcount(t *testing.T) {
	mock, svc, rec := newMock(t, &fakeSchedule{})
	mock.ExpectQuery("FROM hr_employee_job_history jh").
		WithArgs(companyA, rangeTo).
		WillReturnRows(pgxmock.NewRows([]string{"dept_id", "dept", "type", "count"}).
			AddRow(int64(1), "Finance", "CONTRACT", int64(2)).
			AddRow(int64(1), "Finance", "PERMANENT", int64(5)).
			AddRow(int64(0), "", "PERMANENT", int64(1)))

	got, err := svc.Headcount(context.Background(), companyA, rangeTo.Add(13*time.Hour))
	require.NoError(t, err)
	require.Equal(t, int64(8), got.Total)
	require.Len(t, got.Rows, 3)
	require.Equal(t, "CONTRACT", got.Rows[0].EmploymentType)
	require.NoError(t, mock.ExpectationsWereMet())

	q := rec.queries()
	require.Len(t, q, 1)
	assertSafeQuery(t, q[0])
	require.Contains(t, q[0], "LEFT JOIN LATERAL")
	require.Contains(t, q[0], "jh.valid_from<=$2 AND (jh.valid_to IS NULL OR jh.valid_to>=$2)")
	require.Contains(t, q[0], "e.hire_date<=$2")
}

func TestHiresAndLeaversByMonth(t *testing.T) {
	mock, svc, rec := newMock(t, &fakeSchedule{})
	mock.ExpectQuery("t.termination_date BETWEEN $2 AND $3").
		WithArgs(companyA, rangeFrom, rangeTo).
		WillReturnRows(pgxmock.NewRows([]string{"month", "dept_id", "dept", "hires", "leavers"}).
			AddRow("2026-06", int64(1), "Finance", int64(2), int64(1)))

	got, err := svc.HiresAndLeaversByMonth(context.Background(), companyA, rangeFrom, rangeTo)
	require.NoError(t, err)
	require.Equal(t, []HiresLeaversRow{{Month: "2026-06", DepartmentID: 1, Department: "Finance", Hires: 2, Leavers: 1}}, got)
	q := rec.queries()
	require.NotEmpty(t, q)
	assertSafeQuery(t, q[len(q)-1])
	require.Contains(t, q[len(q)-1], "jh.valid_from<=ev.event_date")
}

func TestTurnover(t *testing.T) {
	mock, svc, rec := newMock(t, &fakeSchedule{})
	mock.ExpectQuery("l.termination_date BETWEEN $2 AND $3").
		WithArgs(companyA, rangeFrom, rangeTo).
		WillReturnRows(pgxmock.NewRows([]string{"leavers", "opening", "closing"}).AddRow(int64(3), int64(10), int64(20)))

	got, err := svc.Turnover(context.Background(), companyA, rangeFrom, rangeTo)
	require.NoError(t, err)
	require.Equal(t, int64(3), got.Leavers)
	require.InDelta(t, 15.0, got.AverageHeadcount, 1e-9)
	require.InDelta(t, 0.2, got.Rate, 1e-9)
	require.NoError(t, mock.ExpectationsWereMet())
	q := rec.queries()
	assertSafeQuery(t, q[len(q)-1])
	require.Contains(t, q[len(q)-1], "jh.valid_from<=$2")
	require.Contains(t, q[len(q)-1], "jh.valid_from<=$3")
}

// fakeSchedule mimics *schedule.Service: each method issues exactly one
// query through the same mocked pool, so the pool sees every query the
// report causes.
type fakeSchedule struct {
	pool            pgxmock.PgxPoolIface
	holidayCalls    int
	assignmentCalls int
	gotEmployeeIDs  []int64
	assignments     map[int64][]schedule.ShiftAssignment
}

const (
	fakeHolidaySQL    = `SELECT h.holiday_date FROM hr_holidays h WHERE h.company_id=$1 AND h.holiday_date BETWEEN $2 AND $3`
	fakeAssignmentSQL = `SELECT es.id FROM hr_employee_shifts es WHERE es.company_id=$1 AND es.employee_id = ANY($2)`
)

func (f *fakeSchedule) HolidaysBetween(ctx context.Context, companyID int64, from, to time.Time) ([]time.Time, error) {
	f.holidayCalls++
	rows, err := f.pool.Query(ctx, fakeHolidaySQL, companyID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []time.Time
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (f *fakeSchedule) ShiftAssignmentsForEmployees(ctx context.Context, companyID int64, ids []int64, from, to time.Time) (map[int64][]schedule.ShiftAssignment, error) {
	f.assignmentCalls++
	f.gotEmployeeIDs = append([]int64(nil), ids...)
	rows, err := f.pool.Query(ctx, fakeAssignmentSQL, companyID, ids)
	if err != nil {
		return nil, err
	}
	rows.Close()
	return f.assignments, rows.Err()
}

func TestAbsenteeismIssuesExactlyThreeQueries(t *testing.T) {
	monSat := []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday}
	for _, n := range []int{1, 3, 50} {
		t.Run(fmt.Sprintf("%d employees", n), func(t *testing.T) {
			sched := &fakeSchedule{assignments: map[int64][]schedule.ShiftAssignment{
				// Employee 1 works Mon-Sat all month.
				1: {shiftAssignment(day(2026, 1, 1), nil, monSat...)},
			}}
			mock, svc, rec := newMock(t, sched)
			sched.pool = mock

			empRows := pgxmock.NewRows([]string{"id", "hire", "term", "dept_id", "dept", "absent"})
			for i := 1; i <= n; i++ {
				dept, name := int64(1), "Ops"
				if i%2 == 0 {
					dept, name = 2, "Sales"
				}
				var term *time.Time
				hire := day(2020, 1, 1)
				if i == 3 {
					// Hired mid-month: June 15-30 default week = 12 days.
					hire = day(2026, 6, 15)
				}
				empRows.AddRow(int64(i), hire, term, dept, name, int64(1))
			}
			mock.ExpectQuery("a.status='ABSENT'").WithArgs(companyA, rangeFrom, rangeTo).WillReturnRows(empRows)
			// One weekday holiday (Mon June 1).
			mock.ExpectQuery("FROM hr_holidays h").WithArgs(companyA, rangeFrom, rangeTo).
				WillReturnRows(pgxmock.NewRows([]string{"holiday_date"}).AddRow(day(2026, 6, 1)))
			mock.ExpectQuery("FROM hr_employee_shifts es").WithArgs(companyA, pgxmock.AnyArg()).
				WillReturnRows(pgxmock.NewRows([]string{"id"}))

			got, err := svc.Absenteeism(context.Background(), companyA, rangeFrom, rangeTo)
			require.NoError(t, err)
			require.NoError(t, mock.ExpectationsWereMet())
			require.Len(t, rec.queries(), 3, "exactly three queries regardless of employee count")
			require.Equal(t, 1, sched.holidayCalls)
			require.Equal(t, 1, sched.assignmentCalls)
			require.Len(t, sched.gotEmployeeIDs, n)
			assertSafeQuery(t, rec.queries()[0])
			require.Contains(t, rec.queries()[0], "LEFT JOIN LATERAL")

			// Expected scheduled days: emp 1 Mon-Sat 26-1=25; emp 3 = 12;
			// everyone else Mon-Fri 22-1=21.
			want := 25.0
			if n >= 3 {
				want += 12 + float64(n-2)*21
			}
			require.Equal(t, int64(n), got.Employees)
			require.Equal(t, int64(n), got.AbsentDays)
			require.InDelta(t, want, got.ScheduledDays, 1e-9)
			require.InDelta(t, float64(n)/want, got.Rate, 1e-9)
			var deptSum float64
			for _, r := range got.ByDepartment {
				deptSum += r.ScheduledDays
			}
			require.InDelta(t, want, deptSum, 1e-9)
		})
	}
}

func TestLeaveLiabilityDecimalSafe(t *testing.T) {
	mock, svc, rec := newMock(t, &fakeSchedule{})
	mock.ExpectQuery("FROM hr_leave_balances b").
		WithArgs(companyA, 2026, day(2026, 10, 12)).
		WillReturnRows(pgxmock.NewRows([]string{"dept_id", "dept", "emps", "nosal", "days", "amount"}).
			AddRow(int64(1), "Finance", int64(2), int64(0), "10.50", "5000000.10").
			AddRow(int64(2), "Ops", int64(1), int64(1), "3.00", "0.20"))

	got, err := svc.LeaveLiability(context.Background(), companyA, 2026)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, "13.50", got.AvailableDays)
	require.Equal(t, "5000000.30", got.Amount) // exact; float64 would also pass here but big.Rat never drifts
	require.Equal(t, day(2026, 10, 12), got.ValuationDate)
	q := rec.queries()
	assertSafeQuery(t, q[0])
	require.Contains(t, q[0], "c.base_salary / 21")
	require.Contains(t, q[0], "jh.valid_from<=$3")
	require.Contains(t, q[0], "ca.effective_from<=$3")
	require.NotContains(t, q[0], "::float")
	require.NotContains(t, q[0], "double precision")
}

func TestLeaveLiabilityRejectsBadDecimal(t *testing.T) {
	mock, svc, _ := newMock(t, &fakeSchedule{})
	mock.ExpectQuery("FROM hr_leave_balances b").
		WithArgs(companyA, 2025, day(2025, 12, 31)).
		WillReturnRows(pgxmock.NewRows([]string{"dept_id", "dept", "emps", "nosal", "days", "amount"}).
			AddRow(int64(1), "Finance", int64(1), int64(0), "1.00", "abc"))
	_, err := svc.LeaveLiability(context.Background(), companyA, 2025)
	require.Error(t, err)
}

func TestOvertimeCost(t *testing.T) {
	mock, svc, rec := newMock(t, &fakeSchedule{})
	mock.ExpectQuery("FROM payroll_run_lines l").
		WithArgs(companyA, rangeFrom, rangeTo).
		WillReturnRows(pgxmock.NewRows([]string{"dept_id", "dept", "amount"}).
			AddRow(int64(1), "Finance", int64(250000)))
	mock.ExpectQuery("FROM payroll_company_policies pcp").
		WithArgs(companyA, day(2026, 10, 12)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "divisor", "first", "subsequent", "rounding"}).
			AddRow(int64(9), int64(173), int64(15000), int64(20000), int64(1)))
	base := int64(8_650_000)
	mock.ExpectQuery("FROM hr_overtime_requests o").
		WithArgs(companyA, rangeFrom, rangeTo).
		WillReturnRows(pgxmock.NewRows([]string{"id", "dept_id", "dept", "minutes", "base"}).
			AddRow(int64(11), int64(1), "Finance", int64(120), &base).
			AddRow(int64(12), int64(2), "Ops", int64(90), &base).
			AddRow(int64(13), int64(2), "Ops", int64(30), (*int64)(nil)))

	got, err := svc.OvertimeCost(context.Background(), companyA, rangeFrom, rangeTo)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	policy := payroll.Policy{VersionID: 9, OvertimeDivisor: 173, FirstHourMultiplierBPS: 15000, SubsequentHourMultiplierBPS: 20000, RoundingUnit: 1}
	pay120 := payroll.OvertimePay(payroll.Money(base), 120, policy)
	pay90 := payroll.OvertimePay(payroll.Money(base), 90, policy)
	require.Equal(t, payroll.Money(175000), pay120) // 50,000/h: 1.5x first hour + 2x second hour
	require.Equal(t, int64(9), got.PolicyID)
	require.False(t, got.PolicyMissing)
	require.Equal(t, payroll.Money(250000), got.Posted)
	require.Equal(t, pay120+pay90, got.Unrun)
	require.Equal(t, got.Posted+got.Unrun, got.Total)
	require.Equal(t, int64(240), got.UnrunMinutes)
	require.Equal(t, int64(30), got.UnpricedMinutes)
	require.Len(t, got.ByDepartment, 2)
	require.Equal(t, payroll.Money(250000)+pay120, got.ByDepartment[0].Total)
	require.Equal(t, pay90, got.ByDepartment[1].Total)

	q := rec.queries()
	require.Len(t, q, 3)
	for _, sql := range q {
		assertSafeQuery(t, sql)
	}
	require.Contains(t, q[0], "r.status='POSTED' AND p.starts_on<=$3 AND p.ends_on>=$2")
	require.Contains(t, q[1], "pcp.company_id=$1")
	require.Contains(t, q[2], "o.status='APPROVED' AND o.worked_on BETWEEN $2 AND $3")
	require.Contains(t, q[2], "NOT EXISTS")
}

func TestOvertimeCostWithoutPolicyLeavesUnrunUnpriced(t *testing.T) {
	mock, svc, _ := newMock(t, &fakeSchedule{})
	mock.ExpectQuery("FROM payroll_run_lines l").WithArgs(companyA, rangeFrom, rangeTo).
		WillReturnRows(pgxmock.NewRows([]string{"dept_id", "dept", "amount"}))
	mock.ExpectQuery("FROM payroll_company_policies pcp").WithArgs(companyA, day(2026, 10, 12)).
		WillReturnError(pgx.ErrNoRows)
	base := int64(5_000_000)
	mock.ExpectQuery("FROM hr_overtime_requests o").WithArgs(companyA, rangeFrom, rangeTo).
		WillReturnRows(pgxmock.NewRows([]string{"id", "dept_id", "dept", "minutes", "base"}).
			AddRow(int64(1), int64(0), "", int64(60), &base))

	got, err := svc.OvertimeCost(context.Background(), companyA, rangeFrom, rangeTo)
	require.NoError(t, err)
	require.True(t, got.PolicyMissing)
	require.Equal(t, payroll.Money(0), got.Total)
	require.Equal(t, int64(60), got.UnpricedMinutes)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOvertimeCostPropagatesErrors(t *testing.T) {
	mock, svc, _ := newMock(t, &fakeSchedule{})
	boom := errors.New("boom")
	mock.ExpectQuery("FROM payroll_run_lines l").WithArgs(companyA, rangeFrom, rangeTo).WillReturnError(boom)
	_, err := svc.OvertimeCost(context.Background(), companyA, rangeFrom, rangeTo)
	require.ErrorIs(t, err, boom)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpcomingExpiries(t *testing.T) {
	mock, svc, rec := newMock(t, &fakeSchedule{})
	mock.ExpectQuery("e.contract_end_date BETWEEN $2 AND $3").
		WithArgs(companyA, day(2026, 10, 12), day(2026, 11, 11)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "num", "name", "dept_id", "dept", "kind", "on"}).
			AddRow(int64(5), "E-005", "Ana", int64(1), "Finance", ExpiryProbationEnd, day(2026, 10, 20)).
			AddRow(int64(6), "E-006", "Budi", int64(0), "", ExpiryContractEnd, day(2026, 11, 1)))

	got, err := svc.UpcomingExpiries(context.Background(), companyA, 30)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, got, 2)
	require.Equal(t, ExpiryProbationEnd, got[0].Kind)
	q := rec.queries()
	assertSafeQuery(t, q[0])
	require.Contains(t, q[0], "p.probation_end_date BETWEEN $2 AND $3")
}
