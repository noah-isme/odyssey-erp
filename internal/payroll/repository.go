package payroll

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func regularRunUUID(companyID, periodID int64) uuid.UUID {
	return uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("payroll:regular:%d:%d", companyID, periodID)))
}

func (r *Repository) CreateDraft(ctx context.Context, companyID, periodID, actorID int64) (Run, error) {
	var run Run
	run.RunUUID = regularRunUUID(companyID, periodID)
	err := r.pool.QueryRow(ctx, `WITH p AS (
		SELECT id,code,pay_date FROM payroll_periods WHERE id=$2 AND company_id=$1 AND status='OPEN'
	), tax AS (
		SELECT id FROM payroll_rule_versions WHERE rule_type='TAX' AND reviewed_at IS NOT NULL AND effective_from <= (SELECT pay_date FROM p) AND (effective_to IS NULL OR effective_to >= (SELECT pay_date FROM p)) ORDER BY effective_from DESC LIMIT 1
	), bpjs AS (
		SELECT id FROM payroll_rule_versions WHERE rule_type='BPJS' AND reviewed_at IS NOT NULL AND effective_from <= (SELECT pay_date FROM p) AND (effective_to IS NULL OR effective_to >= (SELECT pay_date FROM p)) ORDER BY effective_from DESC LIMIT 1
	), policy AS (
		SELECT id FROM payroll_company_policies WHERE company_id=$1 AND effective_from <= (SELECT pay_date FROM p) AND (effective_to IS NULL OR effective_to >= (SELECT pay_date FROM p)) ORDER BY effective_from DESC LIMIT 1
	)
	INSERT INTO payroll_runs(run_uuid,company_id,period_id,tax_rule_version_id,bpjs_rule_version_id,company_policy_id,created_by)
	SELECT $4,$1,p.id,tax.id,bpjs.id,policy.id,$3 FROM p,tax,bpjs,policy
	RETURNING id,run_uuid,company_id,period_id,run_type,tax_rule_version_id,bpjs_rule_version_id,company_policy_id,status,created_by,created_at`, companyID, periodID, actorID, run.RunUUID).
		Scan(&run.ID, &run.RunUUID, &run.CompanyID, &run.PeriodID, &run.RunType, &run.TaxRuleVersionID, &run.BPJSRuleVersionID, &run.PolicyID, &run.Status, &run.CreatedBy, &run.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrConfiguration
	}
	return run, err
}

func (r *Repository) Calculate(ctx context.Context, runID int64) (Run, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return Run{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	run, err := getRun(ctx, tx, runID, true)
	if err != nil {
		return Run{}, err
	}
	if run.Status != StatusDraft {
		return Run{}, ErrInvalidState
	}
	rules, policy, err := loadRules(ctx, tx, run)
	if err != nil {
		return Run{}, err
	}
	// Recalculation atomically replaces the draft snapshot. Posted runs cannot
	// enter this transaction, so their component breakdown remains immutable.
	if _, err = tx.Exec(ctx, `DELETE FROM payroll_run_lines WHERE run_id=$1`, runID); err != nil {
		return Run{}, err
	}
	rows, err := tx.Query(ctx, `SELECT e.id,e.department_id,c.base_salary::bigint,c.ptkp_code,c.bpjs_health,c.bpjs_employment,
		COALESCE((SELECT SUM(rc.amount) FROM payroll_recurring_components rc JOIN payroll_components pc ON pc.id=rc.component_id WHERE rc.assignment_id=c.id AND pc.kind='EARNING' AND rc.effective_from<=p.pay_date AND (rc.effective_to IS NULL OR rc.effective_to>=p.pay_date)),0)::bigint,
		COALESCE((SELECT SUM(rc.amount) FROM payroll_recurring_components rc JOIN payroll_components pc ON pc.id=rc.component_id WHERE rc.assignment_id=c.id AND pc.kind='DEDUCTION' AND rc.effective_from<=p.pay_date AND (rc.effective_to IS NULL OR rc.effective_to>=p.pay_date)),0)::bigint,
		COALESCE((SELECT SUM(rc.amount) FROM payroll_recurring_components rc JOIN payroll_components pc ON pc.id=rc.component_id WHERE rc.assignment_id=c.id AND pc.bpjs_base AND rc.effective_from<=p.pay_date AND (rc.effective_to IS NULL OR rc.effective_to>=p.pay_date)),0)::bigint,
		COALESCE((SELECT SUM(a.amount) FROM payroll_adjustments a JOIN payroll_components pc ON pc.id=a.component_id WHERE a.employee_id=e.id AND a.period_id=p.id AND pc.kind='EARNING'),0)::bigint,
		COALESCE((SELECT SUM(ABS(a.amount)) FROM payroll_adjustments a JOIN payroll_components pc ON pc.id=a.component_id WHERE a.employee_id=e.id AND a.period_id=p.id AND pc.kind='DEDUCTION'),0)::bigint,
		COALESCE((SELECT SUM(o.minutes) FROM payroll_overtime o WHERE o.employee_id=e.id AND o.period_id=p.id AND o.approved_by IS NOT NULL),0)::bigint,
		COALESCE((SELECT amount FROM payroll_thr t WHERE t.employee_id=e.id AND t.period_id=p.id),0)::bigint,
		COALESCE((SELECT COUNT(*) FROM hr_attendance a WHERE a.employee_id=e.id AND a.attendance_date BETWEEN p.starts_on AND p.ends_on AND a.status='PRESENT'),0)::bigint,
		COALESCE((SELECT SUM(lr.days) FROM hr_leave_requests lr WHERE lr.employee_id=e.id AND lr.status='APPROVED' AND lr.start_date<=p.ends_on AND lr.end_date>=p.starts_on),0)::bigint
	FROM payroll_runs pr JOIN payroll_periods p ON p.id=pr.period_id JOIN hr_employees e ON e.company_id=pr.company_id AND e.status='ACTIVE'
	JOIN LATERAL (SELECT * FROM payroll_compensation_assignments ca WHERE ca.employee_id=e.id AND ca.effective_from<=p.pay_date AND (ca.effective_to IS NULL OR ca.effective_to>=p.pay_date) ORDER BY ca.effective_from DESC LIMIT 1) c ON TRUE
	WHERE pr.id=$1 ORDER BY e.id`, runID)
	if err != nil {
		return Run{}, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var employeeID int64
		var departmentID *int64
		var base, allowances, recurringDeductions, bpjsAllowance, adjustment, oneOffDeduction, thr Money
		var ptkp string
		var health, employment bool
		var overtimeMinutes, attendanceDays, leaveDays int64
		if err = rows.Scan(&employeeID, &departmentID, &base, &ptkp, &health, &employment, &allowances, &recurringDeductions, &bpjsAllowance, &adjustment, &oneOffDeduction, &overtimeMinutes, &thr, &attendanceDays, &leaveDays); err != nil {
			return Run{}, err
		}
		result, calcErr := Calculate(Input{EmployeeID: employeeID, PTKPCode: ptkp, BaseSalary: base, Allowances: allowances, Adjustments: adjustment, OtherDeductions: recurringDeductions + oneOffDeduction, THR: thr, BPJSWage: base + bpjsAllowance, OvertimeMinutes: overtimeMinutes, BPJSHealth: health, BPJSEmployment: employment, AttendanceDays: attendanceDays, LeaveDays: leaveDays}, rules, policy)
		if calcErr != nil {
			return Run{}, fmt.Errorf("employee %d: %w", employeeID, calcErr)
		}
		breakdown, _ := json.Marshal(result)
		_, err = tx.Exec(ctx, `INSERT INTO payroll_run_lines(run_id,employee_id,department_id,ptkp_code,ter_category,base_salary,allowances,overtime,thr,gross,employee_bpjs,employer_bpjs,pph21,other_deductions,net_pay,breakdown) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, runID, employeeID, departmentID, result.PTKPCode, result.TERCategory, result.BaseSalary, result.Allowances, result.Overtime, result.THR, result.Gross, result.EmployeeBPJS, result.EmployerBPJS, result.PPh21, result.OtherDeductions, result.NetPay, breakdown)
		if err != nil {
			return Run{}, err
		}
		count++
	}
	if err = rows.Err(); err != nil {
		return Run{}, err
	}
	if count == 0 {
		return Run{}, ErrConfiguration
	}
	if err = tx.Commit(ctx); err != nil {
		return Run{}, err
	}
	return r.GetRun(ctx, runID)
}

func loadRules(ctx context.Context, tx pgx.Tx, run Run) (Rules, Policy, error) {
	rules := Rules{TaxVersionID: run.TaxRuleVersionID, BPJSVersionID: run.BPJSRuleVersionID, PTKPCategory: map[string]string{}, PTKPAnnual: map[string]Money{}}
	rows, err := tx.Query(ctx, `SELECT code,ter_category,annual_amount::bigint FROM payroll_ptkp_statuses WHERE rule_version_id=$1`, run.TaxRuleVersionID)
	if err != nil {
		return Rules{}, Policy{}, err
	}
	for rows.Next() {
		var code, category string
		var annual Money
		if err = rows.Scan(&code, &category, &annual); err != nil {
			rows.Close()
			return Rules{}, Policy{}, err
		}
		rules.PTKPCategory[code] = category
		rules.PTKPAnnual[code] = annual
	}
	rows.Close()
	rows, err = tx.Query(ctx, `SELECT category,lower_bound::bigint,upper_bound::bigint,rate_bps FROM payroll_ter_brackets WHERE rule_version_id=$1 ORDER BY category,lower_bound`, run.TaxRuleVersionID)
	if err != nil {
		return Rules{}, Policy{}, err
	}
	for rows.Next() {
		var b TERBracket
		if err = rows.Scan(&b.Category, &b.LowerBound, &b.UpperBound, &b.RateBPS); err != nil {
			rows.Close()
			return Rules{}, Policy{}, err
		}
		rules.TER = append(rules.TER, b)
	}
	rows.Close()
	rows, err = tx.Query(ctx, `SELECT program,employee_rate_bps,employer_rate_bps,COALESCE(wage_floor,0)::bigint,COALESCE(wage_cap,0)::bigint,employer_taxable FROM payroll_bpjs_rates WHERE rule_version_id=$1 AND (program<>'JKK' OR risk_class=(SELECT jkk_risk_class FROM payroll_company_policies WHERE id=$2))`, run.BPJSRuleVersionID, run.PolicyID)
	if err != nil {
		return Rules{}, Policy{}, err
	}
	for rows.Next() {
		var b BPJSRule
		if err = rows.Scan(&b.Program, &b.EmployeeRateBPS, &b.EmployerRateBPS, &b.WageFloor, &b.WageCap, &b.EmployerTaxable); err != nil {
			rows.Close()
			return Rules{}, Policy{}, err
		}
		rules.BPJS = append(rules.BPJS, b)
	}
	rows.Close()
	var policy Policy
	err = tx.QueryRow(ctx, `SELECT id,overtime_divisor,first_hour_multiplier_bps,subsequent_hour_multiplier_bps,rounding_unit FROM payroll_company_policies WHERE id=$1`, run.PolicyID).Scan(&policy.VersionID, &policy.OvertimeDivisor, &policy.FirstHourMultiplierBPS, &policy.SubsequentHourMultiplierBPS, &policy.RoundingUnit)
	return rules, policy, err
}

func (r *Repository) GetRun(ctx context.Context, id int64) (Run, error) {
	return getRun(ctx, r.pool, id, false)
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func getRun(ctx context.Context, q rowQuerier, id int64, lock bool) (Run, error) {
	if lock {
		var lockedID int64
		if err := q.QueryRow(ctx, `SELECT id FROM payroll_runs WHERE id=$1 FOR UPDATE`, id).Scan(&lockedID); err != nil {
			return Run{}, err
		}
	}
	query := `SELECT r.id,r.run_uuid,r.company_id,r.period_id,r.run_type,r.tax_rule_version_id,r.bpjs_rule_version_id,r.company_policy_id,r.status,r.approval_request_id,r.journal_entry_id,r.created_by,r.created_at,p.code,p.pay_date,COALESCE(SUM(l.gross),0)::bigint,COALESCE(SUM(l.net_pay),0)::bigint FROM payroll_runs r JOIN payroll_periods p ON p.id=r.period_id LEFT JOIN payroll_run_lines l ON l.run_id=r.id WHERE r.id=$1 GROUP BY r.id,p.code,p.pay_date`
	var run Run
	err := q.QueryRow(ctx, query, id).Scan(&run.ID, &run.RunUUID, &run.CompanyID, &run.PeriodID, &run.RunType, &run.TaxRuleVersionID, &run.BPJSRuleVersionID, &run.PolicyID, &run.Status, &run.ApprovalRequestID, &run.JournalEntryID, &run.CreatedBy, &run.CreatedAt, &run.PeriodCode, &run.PayDate, &run.Gross, &run.NetPay)
	return run, err
}

func (r *Repository) ListRuns(ctx context.Context, companyID int64) ([]Run, error) {
	rows, err := r.pool.Query(ctx, `SELECT r.id,r.run_uuid,r.company_id,r.period_id,r.run_type,r.status,r.created_by,r.created_at,p.code,p.pay_date,COALESCE(SUM(l.gross),0)::bigint,COALESCE(SUM(l.net_pay),0)::bigint FROM payroll_runs r JOIN payroll_periods p ON p.id=r.period_id LEFT JOIN payroll_run_lines l ON l.run_id=r.id WHERE r.company_id=$1 GROUP BY r.id,p.code,p.pay_date ORDER BY r.created_at DESC`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		var x Run
		if err = rows.Scan(&x.ID, &x.RunUUID, &x.CompanyID, &x.PeriodID, &x.RunType, &x.Status, &x.CreatedBy, &x.CreatedAt, &x.PeriodCode, &x.PayDate, &x.Gross, &x.NetPay); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) SetApproval(ctx context.Context, runID, requestID int64) error {
	tag, err := r.pool.Exec(ctx, `UPDATE payroll_runs SET status='APPROVAL',approval_request_id=$2,submitted_at=NOW(),updated_at=NOW() WHERE id=$1 AND status='DRAFT'`, runID, requestID)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrInvalidState
	}
	return err
}
func (r *Repository) ResetRejected(ctx context.Context, runID, actorID int64, note string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE payroll_runs SET status='DRAFT',approval_request_id=NULL,submitted_at=NULL,updated_at=NOW() WHERE id=$1 AND status='APPROVAL'`, runID)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrInvalidState
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO payroll_run_events(run_id,event_type,actor_id,note) VALUES($1,'REJECTED',$2,$3)`, runID, actorID, note); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) PostingData(ctx context.Context, runID int64) (Run, []PostingGroup, AccountMappings, int64, error) {
	run, err := r.GetRun(ctx, runID)
	if err != nil {
		return Run{}, nil, AccountMappings{}, 0, err
	}
	rows, err := r.pool.Query(ctx, `SELECT ad.id,cc.id,SUM(l.gross)::bigint,SUM(l.employer_bpjs)::bigint,SUM(l.employee_bpjs)::bigint,SUM(l.pph21)::bigint,SUM(l.other_deductions)::bigint,SUM(l.net_pay)::bigint FROM payroll_run_lines l LEFT JOIN hr_departments hd ON hd.id=l.department_id LEFT JOIN departments ad ON ad.company_id=$2 AND ad.code=hd.code LEFT JOIN LATERAL (SELECT id FROM cost_centers WHERE company_id=$2 AND department_id=ad.id AND is_active ORDER BY id LIMIT 1) cc ON TRUE WHERE l.run_id=$1 GROUP BY ad.id,cc.id`, runID, run.CompanyID)
	if err != nil {
		return Run{}, nil, AccountMappings{}, 0, err
	}
	defer rows.Close()
	var groups []PostingGroup
	for rows.Next() {
		var x PostingGroup
		if err = rows.Scan(&x.DepartmentID, &x.CostCenterID, &x.Gross, &x.EmployerBPJS, &x.EmployeeBPJS, &x.Tax, &x.OtherDeductions, &x.Net); err != nil {
			return Run{}, nil, AccountMappings{}, 0, err
		}
		groups = append(groups, x)
	}
	var m AccountMappings
	err = r.pool.QueryRow(ctx, `SELECT COALESCE(MAX(account_id) FILTER(WHERE mapping_type='SALARY_EXPENSE'),0),COALESCE(MAX(account_id) FILTER(WHERE mapping_type='EMPLOYER_BPJS_EXPENSE'),0),COALESCE(MAX(account_id) FILTER(WHERE mapping_type='PAYROLL_PAYABLE'),0),COALESCE(MAX(account_id) FILTER(WHERE mapping_type='PPH21_PAYABLE'),0),COALESCE(MAX(account_id) FILTER(WHERE mapping_type='BPJS_PAYABLE'),0) FROM payroll_account_mappings WHERE company_id=$1`, run.CompanyID).Scan(&m.SalaryExpense, &m.EmployerBPJSExpense, &m.PayrollPayable, &m.PPh21Payable, &m.BPJSPayable)
	if err != nil {
		return Run{}, nil, AccountMappings{}, 0, err
	}
	var accountingPeriod int64
	err = r.pool.QueryRow(ctx, `SELECT id FROM periods WHERE start_date<=$1 AND end_date>=$1 AND status<>'LOCKED' ORDER BY id DESC LIMIT 1`, run.PayDate).Scan(&accountingPeriod)
	return run, groups, m, accountingPeriod, err
}

func (r *Repository) MarkPosted(ctx context.Context, runID, journalID int64) ([]RunLine, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE payroll_runs SET status='POSTED',journal_entry_id=$2,posted_at=NOW(),updated_at=NOW() WHERE id=$1 AND status='APPROVAL'`, runID, journalID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, ErrInvalidState
	}
	_, err = tx.Exec(ctx, `INSERT INTO payroll_payment_batches(run_id,instruction_count,total_amount) SELECT $1,COUNT(*),SUM(net_pay) FROM payroll_run_lines WHERE run_id=$1 ON CONFLICT(run_id) DO NOTHING`, runID)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO payroll_payslips(run_line_id,document_key,checksum) SELECT id,'payroll/'||run_id||'/'||employee_id||'.pdf',md5(breakdown::text) FROM payroll_run_lines WHERE run_id=$1 ON CONFLICT(run_line_id) DO NOTHING`, runID)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT l.id,ps.id,l.run_id,l.employee_id,e.name,e.email,e.user_id,m.user_id,l.department_id,l.cost_center_id,p.code FROM payroll_run_lines l JOIN payroll_payslips ps ON ps.run_line_id=l.id JOIN payroll_runs r ON r.id=l.run_id JOIN payroll_periods p ON p.id=r.period_id JOIN hr_employees e ON e.id=l.employee_id LEFT JOIN hr_employees m ON m.id=e.manager_id WHERE l.run_id=$1`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RunLine
	for rows.Next() {
		var x RunLine
		if err = rows.Scan(&x.ID, &x.PayslipID, &x.RunID, &x.EmployeeID, &x.EmployeeName, &x.Email, &x.UserID, &x.ManagerUserID, &x.DepartmentID, &x.CostCenterID, &x.PeriodCode); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) PendingPayslips(ctx context.Context, runID int64) ([]RunLine, error) {
	rows, err := r.pool.Query(ctx, `SELECT l.id,ps.id,l.run_id,l.employee_id,e.name,e.email,e.user_id,m.user_id,l.department_id,l.cost_center_id,p.code FROM payroll_run_lines l JOIN payroll_payslips ps ON ps.run_line_id=l.id AND ps.delivered_at IS NULL JOIN payroll_runs r ON r.id=l.run_id JOIN payroll_periods p ON p.id=r.period_id JOIN hr_employees e ON e.id=l.employee_id LEFT JOIN hr_employees m ON m.id=e.manager_id WHERE ($1=0 OR l.run_id=$1) ORDER BY ps.id LIMIT 500`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RunLine
	for rows.Next() {
		var x RunLine
		if err = rows.Scan(&x.ID, &x.PayslipID, &x.RunID, &x.EmployeeID, &x.EmployeeName, &x.Email, &x.UserID, &x.ManagerUserID, &x.DepartmentID, &x.CostCenterID, &x.PeriodCode); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) Payslip(ctx context.Context, payslipID, actorID int64, payrollStaff bool) (RunLine, error) {
	var x RunLine
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT l.id,ps.id,l.run_id,l.employee_id,e.name,e.email,e.user_id,m.user_id,l.department_id,l.cost_center_id,p.code,l.breakdown FROM payroll_payslips ps JOIN payroll_run_lines l ON l.id=ps.run_line_id JOIN payroll_runs r ON r.id=l.run_id JOIN payroll_periods p ON p.id=r.period_id JOIN hr_employees e ON e.id=l.employee_id LEFT JOIN hr_employees m ON m.id=e.manager_id WHERE ps.id=$1 AND ($3 OR e.user_id=$2 OR m.user_id=$2)`, payslipID, actorID, payrollStaff).Scan(&x.ID, &x.PayslipID, &x.RunID, &x.EmployeeID, &x.EmployeeName, &x.Email, &x.UserID, &x.ManagerUserID, &x.DepartmentID, &x.CostCenterID, &x.PeriodCode, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return RunLine{}, ErrUnauthorized
	}
	if err != nil {
		return RunLine{}, err
	}
	err = json.Unmarshal(raw, &x.Result)
	return x, err
}

func (r *Repository) PaymentInstructions(ctx context.Context, runID int64) ([]PaymentInstruction, error) {
	rows, err := r.pool.Query(ctx, `SELECT e.employee_number,e.name,c.bank_code,c.bank_account_number,c.bank_account_name,l.net_pay::bigint FROM payroll_run_lines l JOIN payroll_runs r ON r.id=l.run_id JOIN payroll_periods p ON p.id=r.period_id JOIN hr_employees e ON e.id=l.employee_id JOIN LATERAL(SELECT * FROM payroll_compensation_assignments ca WHERE ca.employee_id=e.id AND ca.effective_from<=p.pay_date AND(ca.effective_to IS NULL OR ca.effective_to>=p.pay_date)ORDER BY ca.effective_from DESC LIMIT 1)c ON TRUE WHERE l.run_id=$1 ORDER BY e.employee_number`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PaymentInstruction
	for rows.Next() {
		var x PaymentInstruction
		if err = rows.Scan(&x.EmployeeNumber, &x.EmployeeName, &x.BankCode, &x.AccountNumber, &x.AccountName, &x.Amount); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) DeliveryPayslip(ctx context.Context, payslipID int64) (PayslipRecord, error) {
	var record PayslipRecord
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT ps.id,l.id,l.run_id,l.employee_id,e.name,e.email,e.user_id,m.user_id,l.department_id,l.cost_center_id,l.breakdown,p.code FROM payroll_payslips ps JOIN payroll_run_lines l ON l.id=ps.run_line_id JOIN payroll_runs r ON r.id=l.run_id JOIN payroll_periods p ON p.id=r.period_id JOIN hr_employees e ON e.id=l.employee_id LEFT JOIN hr_employees m ON m.id=e.manager_id WHERE ps.id=$1`, payslipID).Scan(&record.ID, &record.Line.ID, &record.Line.RunID, &record.Line.EmployeeID, &record.Line.EmployeeName, &record.Line.Email, &record.Line.UserID, &record.Line.ManagerUserID, &record.Line.DepartmentID, &record.Line.CostCenterID, &raw, &record.PeriodCode)
	if err != nil {
		return PayslipRecord{}, err
	}
	err = json.Unmarshal(raw, &record.Line.Result)
	return record, err
}

func (r *Repository) MarkPayslipDelivered(ctx context.Context, payslipID int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE payroll_payslips SET delivered_at=COALESCE(delivered_at,NOW()) WHERE id=$1`, payslipID)
	return err
}
