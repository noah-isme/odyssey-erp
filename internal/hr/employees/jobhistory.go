package employees

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// MaxManagerChainDepth bounds ManagerChainAsOf (and IsManagerOfAsOf).
const MaxManagerChainDepth = 10

// JobHistory maps an hr_employee_job_history row. Department, Position,
// Manager, and EmployeeName are display names resolved by join.
type JobHistory struct {
	ID, CompanyID, EmployeeID           int64
	DepartmentID, PositionID, ManagerID *int64
	EmployeeName                        string
	Department, Position, Manager       string
	Status, EmploymentType, Grade       string
	ValidFrom                           time.Time
	ValidTo                             *time.Time
	Reason                              string
	CreatedBy                           *int64
	CreatedAt                           time.Time
}

// JobChangeInput describes a new effective-dated job record. Status defaults
// to ACTIVE and EmploymentType to PERMANENT when empty.
type JobChangeInput struct {
	DepartmentID, PositionID, ManagerID *int64
	Status, EmploymentType, Grade       string
	ValidFrom                           time.Time
	Reason                              string
	ActorID                             int64
}

// Job history rows are always reached through their company-owning
// hr_employees row (e), never by jh.company_id alone.
const jobHistorySelect = `SELECT jh.id,e.company_id,jh.employee_id,jh.department_id,jh.position_id,jh.manager_id,e.name,COALESCE(d.name,''),COALESCE(p.name,''),COALESCE(m.name,''),
jh.status,jh.employment_type,jh.grade,jh.valid_from,jh.valid_to,jh.reason,jh.created_by,jh.created_at
FROM hr_employee_job_history jh
JOIN hr_employees e ON e.id=jh.employee_id AND e.company_id=$1
LEFT JOIN hr_departments d ON d.id=jh.department_id
LEFT JOIN hr_positions p ON p.id=jh.position_id
LEFT JOIN hr_employees m ON m.id=jh.manager_id`

// asOfPredicate selects the row effective on $3.
const asOfPredicate = `jh.valid_from<=$3 AND (jh.valid_to IS NULL OR jh.valid_to>=$3)`

func scanJobHistory(row pgx.Row) (JobHistory, error) {
	var j JobHistory
	err := row.Scan(&j.ID, &j.CompanyID, &j.EmployeeID, &j.DepartmentID, &j.PositionID, &j.ManagerID, &j.EmployeeName, &j.Department, &j.Position, &j.Manager,
		&j.Status, &j.EmploymentType, &j.Grade, &j.ValidFrom, &j.ValidTo, &j.Reason, &j.CreatedBy, &j.CreatedAt)
	return j, err
}

func collectJobHistory(rows pgx.Rows) ([]JobHistory, error) {
	defer rows.Close()
	var out []JobHistory
	for rows.Next() {
		j, err := scanJobHistory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// businessToday is the current calendar date in the business timezone
// (Asia/Jakarta, see shared.BusinessLocation) as a UTC midnight date,
// comparable with dateOnly values.
func businessToday() time.Time { return shared.BusinessToday() }

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// ListJobHistory returns the employee's job records, newest first.
func (s *Service) ListJobHistory(ctx context.Context, companyID, employeeID int64) ([]JobHistory, error) {
	if companyID <= 0 {
		return nil, errCompanyRequired
	}
	rows, err := s.pool.Query(ctx, jobHistorySelect+` WHERE jh.employee_id=$2 ORDER BY jh.valid_from DESC`, companyID, employeeID)
	if err != nil {
		return nil, err
	}
	return collectJobHistory(rows)
}

// AssignmentAsOf returns the job record effective on asOf
// (valid_from <= asOf AND (valid_to IS NULL OR valid_to >= asOf)). It returns
// ErrNoJobAssignment when none is effective (e.g. before hire, or the
// employee belongs to another company).
func (s *Service) AssignmentAsOf(ctx context.Context, companyID, employeeID int64, asOf time.Time) (JobHistory, error) {
	if companyID <= 0 {
		return JobHistory{}, errCompanyRequired
	}
	j, err := scanJobHistory(s.pool.QueryRow(ctx, jobHistorySelect+` WHERE jh.employee_id=$2 AND `+asOfPredicate+` ORDER BY jh.valid_from DESC LIMIT 1`, companyID, employeeID, dateOnly(asOf)))
	if errors.Is(err, pgx.ErrNoRows) {
		return JobHistory{}, ErrNoJobAssignment
	}
	return j, err
}

// DirectReportsAsOf returns the job records, effective on asOf and with
// status ACTIVE, whose manager_id is managerEmployeeID, ordered by name.
func (s *Service) DirectReportsAsOf(ctx context.Context, companyID, managerEmployeeID int64, asOf time.Time) ([]JobHistory, error) {
	if companyID <= 0 {
		return nil, errCompanyRequired
	}
	rows, err := s.pool.Query(ctx, jobHistorySelect+` WHERE jh.manager_id=$2 AND jh.status='ACTIVE' AND `+asOfPredicate+` ORDER BY e.name, jh.employee_id`, companyID, managerEmployeeID, dateOnly(asOf))
	if err != nil {
		return nil, err
	}
	return collectJobHistory(rows)
}

// ManagerChainAsOf returns the employee ids of the employee's managers as of
// asOf, nearest first: the manager on the employee's effective job record,
// then that manager's effective manager, and so on. The chain stops at a
// record without a manager, at a manager without an effective record in the
// company, on a cycle, or after MaxManagerChainDepth managers.
func (s *Service) ManagerChainAsOf(ctx context.Context, companyID, employeeID int64, asOf time.Time) ([]int64, error) {
	if companyID <= 0 {
		return nil, errCompanyRequired
	}
	rows, err := s.pool.Query(ctx, `WITH RECURSIVE chain(depth, manager_id, path) AS (
	SELECT 1, jh.manager_id, ARRAY[jh.employee_id]
	FROM hr_employee_job_history jh
	JOIN hr_employees e ON e.id=jh.employee_id AND e.company_id=$1
	WHERE jh.employee_id=$2 AND `+asOfPredicate+`
	UNION ALL
	SELECT c.depth+1, jh.manager_id, c.path || jh.employee_id
	FROM chain c
	JOIN hr_employee_job_history jh ON jh.employee_id=c.manager_id AND `+asOfPredicate+`
	JOIN hr_employees e ON e.id=jh.employee_id AND e.company_id=$1
	WHERE c.depth < $4 AND NOT (jh.employee_id = ANY(c.path))
)
SELECT depth, manager_id FROM chain WHERE manager_id IS NOT NULL ORDER BY depth`, companyID, employeeID, dateOnly(asOf), MaxManagerChainDepth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[int64]bool{employeeID: true}
	var out []int64
	lastDepth := 0
	stopped := false
	for rows.Next() {
		var depth int
		var managerID int64
		if err := rows.Scan(&depth, &managerID); err != nil {
			return nil, err
		}
		if stopped || depth == lastDepth {
			continue // defensive: one manager per level
		}
		if depth != lastDepth+1 || seen[managerID] || len(out) >= MaxManagerChainDepth {
			stopped = true
			continue
		}
		seen[managerID] = true
		out = append(out, managerID)
		lastDepth = depth
	}
	return out, rows.Err()
}

// IsManagerOfAsOf reports whether the user's employee record in the company
// is a direct or indirect manager (within MaxManagerChainDepth) of
// employeeID as of asOf. A user without an employee record in the company,
// or the employee themself, is never a manager.
func (s *Service) IsManagerOfAsOf(ctx context.Context, companyID, managerUserID, employeeID int64, asOf time.Time) (bool, error) {
	if companyID <= 0 {
		return false, errCompanyRequired
	}
	if managerUserID <= 0 || employeeID <= 0 {
		return false, nil
	}
	var managerEmployeeID int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM hr_employees WHERE user_id=$1 AND company_id=$2`, managerUserID, companyID).Scan(&managerEmployeeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("resolve manager: %w", err)
	}
	if managerEmployeeID == employeeID {
		return false, nil
	}
	chain, err := s.ManagerChainAsOf(ctx, companyID, employeeID, asOf)
	if err != nil {
		return false, err
	}
	for _, id := range chain {
		if id == managerEmployeeID {
			return true, nil
		}
	}
	return false, nil
}

// RecordJobChange records a new effective-dated job record in one
// transaction: it closes the latest record at ValidFrom - 1 day (which must be
// on or after that record's valid_from), inserts the new record, mirrors the
// job fields onto hr_employees when ValidFrom is today or earlier, and audits
// EMPLOYEE_JOB_CHANGED with the changed field names.
//
// When ValidFrom equals the valid_from of the open latest record (a same-day
// correction, e.g. fixing the "Hired" record of a new or pre-boarded
// employee), that record is updated in place instead, because
// (employee_id, valid_from) is unique; the audit meta then has "corrected":
// true. A change that sets status ACTIVE on an employee whose termination_date
// is before ValidFrom is a rehire: termination_date and termination_reason are
// cleared in the same transaction and EMPLOYEE_REHIRED is audited.
func (s *Service) RecordJobChange(ctx context.Context, companyID, employeeID int64, in JobChangeInput) (JobHistory, error) {
	in, err := s.prepareJobChange(companyID, employeeID, in)
	if err != nil {
		return JobHistory{}, err
	}
	var out JobHistory
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.recordJobChangeTx(ctx, tx, companyID, employeeID, in)
		return err
	})
	if err != nil {
		return JobHistory{}, err
	}
	return out, nil
}

// UpdateWithJobChange applies Update and (when job is non-nil) RecordJobChange
// in one transaction, so a rejected job change leaves no partial profile edit.
func (s *Service) UpdateWithJobChange(ctx context.Context, companyID, id int64, in UpdateInput, job *JobChangeInput) (Employee, error) {
	var prepared JobChangeInput
	if job != nil {
		var err error
		if prepared, err = s.prepareJobChange(companyID, id, *job); err != nil {
			return Employee{}, err
		}
	}
	if err := s.validateUpdate(companyID, id, &in); err != nil {
		return Employee{}, err
	}
	var out Employee
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		if out, err = s.updateTx(ctx, tx, companyID, id, in); err != nil {
			return err
		}
		if job != nil {
			if _, err = s.recordJobChangeTx(ctx, tx, companyID, id, prepared); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Employee{}, err
	}
	return out, nil
}

// prepareJobChange normalizes and validates a job change without touching the
// database.
func (s *Service) prepareJobChange(companyID, employeeID int64, in JobChangeInput) (JobChangeInput, error) {
	if companyID <= 0 {
		return in, errCompanyRequired
	}
	in.Status = strings.ToUpper(strings.TrimSpace(in.Status))
	if in.Status == "" {
		in.Status = StatusActive
	}
	in.EmploymentType = strings.ToUpper(strings.TrimSpace(in.EmploymentType))
	if in.EmploymentType == "" {
		in.EmploymentType = EmploymentPermanent
	}
	in.Grade = strings.TrimSpace(in.Grade)
	in.Reason = strings.TrimSpace(in.Reason)
	switch {
	case employeeID <= 0 || in.ValidFrom.IsZero():
		return in, errors.New("hr: invalid job change")
	case !validStatus(in.Status):
		return in, ErrInvalidStatus
	case !validEmploymentType(in.EmploymentType):
		return in, ErrInvalidEmploymentType
	case in.ManagerID != nil && *in.ManagerID == employeeID:
		return in, ErrSelfManager
	}
	if s.audit == nil {
		return in, errAuditNotConfigured
	}
	return in, nil
}

// recordJobChangeTx is RecordJobChange inside a caller-owned transaction; in
// must come from prepareJobChange.
func (s *Service) recordJobChangeTx(ctx context.Context, tx pgx.Tx, companyID, employeeID int64, in JobChangeInput) (JobHistory, error) {
	validFrom := dateOnly(in.ValidFrom)
	var terminated *time.Time
	err := tx.QueryRow(ctx, `SELECT termination_date FROM hr_employees WHERE id=$1 AND company_id=$2 FOR UPDATE`, employeeID, companyID).Scan(&terminated)
	if errors.Is(err, pgx.ErrNoRows) {
		return JobHistory{}, ErrEmployeeNotFound
	}
	if err != nil {
		return JobHistory{}, fmt.Errorf("lock employee: %w", err)
	}
	if err := validateRefs(ctx, tx, companyID, in.DepartmentID, in.PositionID, in.ManagerID); err != nil {
		return JobHistory{}, err
	}

	prev, err := scanJobHistory(tx.QueryRow(ctx, jobHistorySelect+` WHERE jh.employee_id=$2 ORDER BY jh.valid_from DESC LIMIT 1 FOR UPDATE OF jh`, companyID, employeeID))
	hasPrev := true
	if errors.Is(err, pgx.ErrNoRows) {
		hasPrev = false
	} else if err != nil {
		return JobHistory{}, fmt.Errorf("lock job history: %w", err)
	}

	out := JobHistory{CompanyID: companyID, EmployeeID: employeeID, DepartmentID: in.DepartmentID, PositionID: in.PositionID, ManagerID: in.ManagerID,
		Status: in.Status, EmploymentType: in.EmploymentType, Grade: in.Grade, ValidFrom: validFrom, Reason: in.Reason, CreatedBy: nullableID(in.ActorID)}
	corrected := false
	if hasPrev {
		prevFrom := dateOnly(prev.ValidFrom)
		switch {
		case prev.ValidTo == nil && prevFrom.Equal(validFrom):
			// Same-day correction: (employee_id, valid_from) is unique, so the
			// open record is rewritten in place.
			corrected = true
			if _, err := tx.Exec(ctx, `UPDATE hr_employee_job_history SET department_id=$2, position_id=$3, manager_id=$4, status=$5, employment_type=$6, grade=$7, reason=$8 WHERE id=$1`,
				prev.ID, in.DepartmentID, in.PositionID, in.ManagerID, in.Status, in.EmploymentType, in.Grade, in.Reason); err != nil {
				return JobHistory{}, fmt.Errorf("correct job history: %w", err)
			}
			out.ID, out.CreatedBy, out.CreatedAt = prev.ID, prev.CreatedBy, prev.CreatedAt
		default:
			closeAt := validFrom.AddDate(0, 0, -1)
			if closeAt.Before(prevFrom) {
				return JobHistory{}, fmt.Errorf("hr: job change must start after %s (the latest job record)", prevFrom.Format("2006-01-02"))
			}
			if prev.ValidTo == nil {
				if _, err := tx.Exec(ctx, `UPDATE hr_employee_job_history SET valid_to=$2 WHERE id=$1`, prev.ID, closeAt); err != nil {
					return JobHistory{}, fmt.Errorf("close job history: %w", err)
				}
			} else if !dateOnly(*prev.ValidTo).Before(validFrom) {
				return JobHistory{}, fmt.Errorf("hr: job change must start after %s (the latest job record ends then)", dateOnly(*prev.ValidTo).Format("2006-01-02"))
			}
		}
	}
	if !corrected {
		if err := tx.QueryRow(ctx, `INSERT INTO hr_employee_job_history(company_id,employee_id,department_id,position_id,manager_id,status,employment_type,grade,valid_from,reason,created_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id,created_at`,
			companyID, employeeID, in.DepartmentID, in.PositionID, in.ManagerID, in.Status, in.EmploymentType, in.Grade, validFrom, in.Reason, out.CreatedBy).Scan(&out.ID, &out.CreatedAt); err != nil {
			return JobHistory{}, fmt.Errorf("insert job history: %w", err)
		}
	}

	if !validFrom.After(businessToday()) {
		if _, err := tx.Exec(ctx, `UPDATE hr_employees SET department_id=$1, position_id=$2, manager_id=$3, status=$4, employment_type=$5, updated_at=NOW() WHERE id=$6 AND company_id=$7`,
			in.DepartmentID, in.PositionID, in.ManagerID, in.Status, in.EmploymentType, employeeID, companyID); err != nil {
			return JobHistory{}, fmt.Errorf("mirror job fields: %w", err)
		}
	}

	if in.Status == StatusActive && terminated != nil && dateOnly(*terminated).Before(validFrom) {
		if _, err := tx.Exec(ctx, `UPDATE hr_employees SET termination_date=NULL, termination_reason='', updated_at=NOW() WHERE id=$1 AND company_id=$2`, employeeID, companyID); err != nil {
			return JobHistory{}, fmt.Errorf("clear termination: %w", err)
		}
		if err := s.recordTx(ctx, tx, in.ActorID, ActionEmployeeRehired, employeeID, map[string]any{
			"job_history_id": out.ID, "valid_from": validFrom.Format("2006-01-02"), "previous_termination_date": dateOnly(*terminated).Format("2006-01-02"),
		}); err != nil {
			return JobHistory{}, err
		}
	}

	meta := map[string]any{"job_history_id": out.ID, "valid_from": validFrom.Format("2006-01-02")}
	if hasPrev {
		meta["fields"] = changedJobFields(prev, out)
	}
	if corrected {
		meta["corrected"] = true
	}
	if err := s.recordTx(ctx, tx, in.ActorID, ActionEmployeeJobChanged, employeeID, meta); err != nil {
		return JobHistory{}, err
	}
	return out, nil
}

// SystemActorID attributes scheduled audit events whose job record has no
// author. audit_logs.actor_id is NOT NULL and references users, so a real user
// id is required; user 1 is the system placeholder the cmms and documents
// modules also use. That user must exist (a missing user fails the audit
// insert, so the job fails visibly instead of applying silently).
const SystemActorID int64 = 1

// jobMirrorDiffers is the SQL predicate (e = hr_employees, jh = the job
// record effective on the as-of date) for a mirrored column being stale.
const jobMirrorDiffers = `(e.department_id IS DISTINCT FROM jh.department_id OR e.position_id IS DISTINCT FROM jh.position_id
OR e.manager_id IS DISTINCT FROM jh.manager_id OR e.status IS DISTINCT FROM jh.status OR e.employment_type IS DISTINCT FROM jh.employment_type)`

// ApplyDueJobChanges mirrors, for every company, the job record effective on
// asOf onto hr_employees (department_id, position_id, manager_id, status,
// employment_type) where the employee row differs. RecordJobChange mirrors a
// future-dated record only when it is recorded, so this scheduled catch-up is
// what makes it take effect. It is idempotent: an employee that already
// matches is not written or audited. Each employee is updated in its own
// transaction, audited as EMPLOYEE_JOB_CHANGED with source "scheduled" and,
// as actor, the user who recorded the job change (SystemActorID when the
// record has no author); one failing employee does not stop the others (the
// errors are joined and the applied count is still returned).
//
// Employees with a termination_date are skipped: offboarding deactivates them
// immediately and manages their status itself, so re-mirroring an earlier
// (still current) ACTIVE record would undo that.
func (s *Service) ApplyDueJobChanges(ctx context.Context, asOf time.Time) (int, error) {
	return s.applyDueJobChanges(ctx, 0, asOf)
}

// ApplyDueJobChangesForCompany is ApplyDueJobChanges limited to one company.
func (s *Service) ApplyDueJobChangesForCompany(ctx context.Context, companyID int64, asOf time.Time) (int, error) {
	if companyID <= 0 {
		return 0, errCompanyRequired
	}
	return s.applyDueJobChanges(ctx, companyID, asOf)
}

// applyDueJobChanges applies for one company, or for all when companyID is 0.
func (s *Service) applyDueJobChanges(ctx context.Context, companyID int64, asOf time.Time) (int, error) {
	if s.audit == nil {
		return 0, errAuditNotConfigured
	}
	day := dateOnly(asOf)
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT e.id, e.company_id
FROM hr_employees e
JOIN hr_employee_job_history jh ON jh.employee_id=e.id
WHERE ($2::bigint=0 OR e.company_id=$2) AND e.termination_date IS NULL AND jh.valid_from<=$1 AND (jh.valid_to IS NULL OR jh.valid_to>=$1) AND `+jobMirrorDiffers+`
ORDER BY e.company_id, e.id`, day, companyID)
	if err != nil {
		return 0, fmt.Errorf("list due job changes: %w", err)
	}
	type target struct{ employeeID, companyID int64 }
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.employeeID, &t.companyID); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan due job change: %w", err)
		}
		targets = append(targets, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("list due job changes: %w", err)
	}

	applied := 0
	var errs []error
	for _, t := range targets {
		ok, err := s.applyDueJobChange(ctx, t.companyID, t.employeeID, day)
		if err != nil {
			errs = append(errs, fmt.Errorf("employee %d (company %d): %w", t.employeeID, t.companyID, err))
			continue
		}
		if ok {
			applied++
		}
	}
	return applied, errors.Join(errs...)
}

// applyDueJobChange locks the employee row in its own statement, then reads
// the effective record and the employee's current values in a second
// statement. Under READ COMMITTED a join with FOR UPDATE would keep the job
// record version from before the lock wait, so a concurrent RecordJobChange
// that committed meanwhile could be missed; the separate read sees it. It
// writes only the columns that differ and is audited in the same transaction,
// so a second worker finds the row already in sync and does nothing.
func (s *Service) applyDueJobChange(ctx context.Context, companyID, employeeID int64, asOf time.Time) (bool, error) {
	applied := false
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var (
			jhID      int64
			validFrom time.Time
			createdBy *int64
			cur, next JobHistory
		)
		var locked int64
		err := tx.QueryRow(ctx, `SELECT id FROM hr_employees WHERE id=$1 AND company_id=$2 AND termination_date IS NULL FOR UPDATE`, employeeID, companyID).Scan(&locked)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // gone or terminated: nothing to apply
		}
		if err != nil {
			return fmt.Errorf("lock employee: %w", err)
		}
		err = tx.QueryRow(ctx, `SELECT jh.id, jh.valid_from, e.department_id, e.position_id, e.manager_id, e.status, e.employment_type,
jh.department_id, jh.position_id, jh.manager_id, jh.status, jh.employment_type, jh.created_by
FROM hr_employees e
JOIN hr_employee_job_history jh ON jh.employee_id=e.id
WHERE e.id=$1 AND e.company_id=$2 AND jh.valid_from<=$3 AND (jh.valid_to IS NULL OR jh.valid_to>=$3)
ORDER BY jh.valid_from DESC LIMIT 1`, employeeID, companyID, asOf).
			Scan(&jhID, &validFrom, &cur.DepartmentID, &cur.PositionID, &cur.ManagerID, &cur.Status, &cur.EmploymentType,
				&next.DepartmentID, &next.PositionID, &next.ManagerID, &next.Status, &next.EmploymentType, &createdBy)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // no record effective on asOf
		}
		if err != nil {
			return fmt.Errorf("read job record: %w", err)
		}
		fields := changedJobFields(cur, next)
		if len(fields) == 0 {
			return nil // already in sync (idempotent)
		}
		if _, err := tx.Exec(ctx, `UPDATE hr_employees SET department_id=$1, position_id=$2, manager_id=$3, status=$4, employment_type=$5, updated_at=NOW() WHERE id=$6 AND company_id=$7`,
			next.DepartmentID, next.PositionID, next.ManagerID, next.Status, next.EmploymentType, employeeID, companyID); err != nil {
			return fmt.Errorf("mirror job fields: %w", err)
		}
		applied = true
		actor := SystemActorID
		if createdBy != nil && *createdBy > 0 {
			actor = *createdBy
		}
		return s.recordTx(ctx, tx, actor, ActionEmployeeJobChanged, employeeID, map[string]any{
			"job_history_id": jhID, "valid_from": dateOnly(validFrom).Format("2006-01-02"), "fields": fields, "source": "scheduled",
		})
	})
	if err != nil {
		return false, err
	}
	return applied, nil
}

func changedJobFields(old, next JobHistory) []string {
	var out []string
	add := func(changed bool, name string) {
		if changed {
			out = append(out, name)
		}
	}
	add(!sameID(old.DepartmentID, next.DepartmentID), "department_id")
	add(!sameID(old.PositionID, next.PositionID), "position_id")
	add(!sameID(old.ManagerID, next.ManagerID), "manager_id")
	add(old.Status != next.Status, "status")
	add(old.EmploymentType != next.EmploymentType, "employment_type")
	add(old.Grade != next.Grade, "grade")
	return out
}
