package checklists

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
)

type lockedChecklist struct {
	employeeID      int64
	kind            string
	status          string
	terminationDate *time.Time
	userID          *int64
}

// lockChecklist locks a company checklist row (and its employee row) for
// update.
func lockChecklist(ctx context.Context, tx pgx.Tx, companyID, checklistID int64) (lockedChecklist, error) {
	var c lockedChecklist
	err := tx.QueryRow(ctx, `SELECT c.employee_id, c.kind, c.status, c.termination_date, e.user_id FROM hr_employee_checklists c JOIN hr_employees e ON e.id=c.employee_id AND e.company_id=c.company_id WHERE c.id=$1 AND c.company_id=$2 FOR UPDATE`, checklistID, companyID).
		Scan(&c.employeeID, &c.kind, &c.status, &c.terminationDate, &c.userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrChecklistNotFound
	}
	if err != nil {
		return c, fmt.Errorf("lock checklist: %w", err)
	}
	return c, nil
}

func countOpenTasks(ctx context.Context, tx pgx.Tx, companyID, checklistID int64) (int, error) {
	var n int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM hr_employee_checklist_tasks WHERE checklist_id=$1 AND company_id=$2 AND status='OPEN'`, checklistID, companyID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count open tasks: %w", err)
	}
	return n, nil
}

// OffboardingResult summarizes what CompleteOffboarding changed.
type OffboardingResult struct {
	EmployeeID                int64
	TerminationDate           time.Time
	CancelledLeave            int
	CancelledOvertime         int
	CancelledExpenses         int
	CancelledProfileChanges   int
	CancelledApprovalRequests int
	CancelledAssignments      int
	UserDeactivated           bool
}

// cancelPending cancels the employee's PENDING rows in an HR request table
// and returns the linked approval request ids.
func cancelPending(ctx context.Context, tx pgx.Tx, table string, employeeID int64) (int, []int64, error) {
	rows, err := tx.Query(ctx, `UPDATE `+table+` SET status='CANCELLED', updated_at=NOW() WHERE employee_id=$1 AND status='PENDING' RETURNING approval_request_id`, employeeID)
	if err != nil {
		return 0, nil, fmt.Errorf("cancel pending %s: %w", table, err)
	}
	defer rows.Close()
	n := 0
	var ids []int64
	for rows.Next() {
		var id *int64
		if err := rows.Scan(&id); err != nil {
			return 0, nil, err
		}
		n++
		if id != nil {
			ids = append(ids, *id)
		}
	}
	return n, ids, rows.Err()
}

// CompleteOffboarding runs the offboarding system actions in one
// transaction. It refuses while the termination date is in the future, any
// task is OPEN, the employee holds company assets, or the employee has
// direct reports (as of today or the termination date). It then cancels the
// employee's PENDING leave/overtime/expense/profile-change requests with
// their approval requests and assignments, cancels PENDING approval
// assignments in this company whose approver is the employee's user,
// deactivates the user (globally: users have no company), sets leave
// balances to entitled = used and pending = 0, ends benefits, closes job
// history at the termination date with an INACTIVE record from the next
// day, sets the employee INACTIVE, completes the checklist, and audits
// EMPLOYEE_OFFBOARDED.
func (s *Service) CompleteOffboarding(ctx context.Context, companyID, checklistID, actorID int64) (OffboardingResult, error) {
	if companyID <= 0 {
		return OffboardingResult{}, ErrCompanyRequired
	}
	if checklistID <= 0 {
		return OffboardingResult{}, ErrChecklistNotFound
	}
	if s.audit == nil {
		return OffboardingResult{}, errAuditNotConfigured
	}
	if s.employees == nil {
		return OffboardingResult{}, errEmployeesNotConfigured
	}
	today := s.today()
	var res OffboardingResult
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		c, err := lockChecklist(ctx, tx, companyID, checklistID)
		if err != nil {
			return err
		}
		if c.kind != KindOffboarding {
			return ErrWrongKind
		}
		if c.status != StatusOpen {
			return ErrNotOpen
		}
		if c.terminationDate == nil {
			return ErrTerminationRequired
		}
		term := dateOnly(*c.terminationDate)
		res.EmployeeID, res.TerminationDate = c.employeeID, term
		if today.Before(term) {
			return ErrFutureTermination
		}

		open, err := countOpenTasks(ctx, tx, companyID, checklistID)
		if err != nil {
			return err
		}
		if open > 0 {
			return fmt.Errorf("hr: complete or skip %d open checklist tasks first", open)
		}
		var assets int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM fixed_assets WHERE company_id=$1 AND custodian_employee_id=$2`, companyID, c.employeeID).Scan(&assets); err != nil {
			return fmt.Errorf("count held assets: %w", err)
		}
		if assets > 0 {
			return fmt.Errorf("hr: return %d assets held by the employee first", assets)
		}
		reports := map[int64]bool{}
		for _, day := range []time.Time{today, term} {
			rs, err := s.employees.DirectReportsAsOf(ctx, companyID, c.employeeID, day)
			if err != nil {
				return fmt.Errorf("direct reports: %w", err)
			}
			for _, r := range rs {
				reports[r.EmployeeID] = true
			}
		}
		if len(reports) > 0 {
			return fmt.Errorf("hr: reassign %d direct reports first", len(reports))
		}

		var approvalIDs []int64
		for _, t := range []struct {
			table string
			count *int
		}{
			{"hr_leave_requests", &res.CancelledLeave},
			{"hr_overtime_requests", &res.CancelledOvertime},
			{"hr_expense_claims", &res.CancelledExpenses},
			{"hr_profile_change_requests", &res.CancelledProfileChanges},
		} {
			n, ids, err := cancelPending(ctx, tx, t.table, c.employeeID)
			if err != nil {
				return err
			}
			*t.count = n
			approvalIDs = append(approvalIDs, ids...)
		}
		if len(approvalIDs) > 0 {
			tag, err := tx.Exec(ctx, `UPDATE approval_requests SET status='CANCELLED', updated_at=NOW() WHERE id = ANY($1) AND status='PENDING'`, approvalIDs)
			if err != nil {
				return fmt.Errorf("cancel approval requests: %w", err)
			}
			res.CancelledApprovalRequests = int(tag.RowsAffected())
			tag, err = tx.Exec(ctx, `UPDATE approval_assignments SET status='CANCELLED', updated_at=NOW() WHERE request_id = ANY($1) AND status='PENDING'`, approvalIDs)
			if err != nil {
				return fmt.Errorf("cancel approval assignments: %w", err)
			}
			res.CancelledAssignments = int(tag.RowsAffected())
		}
		if c.userID != nil {
			tag, err := tx.Exec(ctx, `UPDATE approval_assignments aa SET status='CANCELLED', updated_at=NOW() FROM approval_requests ar WHERE ar.id=aa.request_id AND ar.company_id=$2 AND aa.approver_id=$1 AND aa.status='PENDING'`, *c.userID, companyID)
			if err != nil {
				return fmt.Errorf("cancel approver assignments: %w", err)
			}
			res.CancelledAssignments += int(tag.RowsAffected())
			if _, err := tx.Exec(ctx, `UPDATE users SET is_active=false, updated_at=NOW() WHERE id=$1`, *c.userID); err != nil {
				return fmt.Errorf("deactivate user: %w", err)
			}
			res.UserDeactivated = true
		}

		if _, err := tx.Exec(ctx, `UPDATE hr_leave_balances SET entitled=used, pending=0, updated_at=NOW() WHERE employee_id=$1`, c.employeeID); err != nil {
			return fmt.Errorf("close leave balances: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE hr_employee_benefits SET status='ENDED' WHERE employee_id=$1 AND status<>'ENDED'`, c.employeeID); err != nil {
			return fmt.Errorf("end benefits: %w", err)
		}
		if err := closeJobHistory(ctx, tx, companyID, c.employeeID, term, actorID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE hr_employees SET status='INACTIVE', updated_at=NOW() WHERE id=$1 AND company_id=$2`, c.employeeID, companyID); err != nil {
			return fmt.Errorf("deactivate employee: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE hr_employee_checklists SET status='COMPLETED', completed_by=$2, completed_at=NOW() WHERE id=$1`, checklistID, nullableID(actorID)); err != nil {
			return fmt.Errorf("complete checklist: %w", err)
		}
		return s.record(ctx, tx, actorID, ActionEmployeeOffboarded, entityEmployee, c.employeeID, map[string]any{
			"checklist_id":                checklistID,
			"termination_date":            term.Format("2006-01-02"),
			"cancelled_leave":             res.CancelledLeave,
			"cancelled_overtime":          res.CancelledOvertime,
			"cancelled_expenses":          res.CancelledExpenses,
			"cancelled_profile_changes":   res.CancelledProfileChanges,
			"cancelled_approval_requests": res.CancelledApprovalRequests,
			"cancelled_assignments":       res.CancelledAssignments,
			"user_deactivated":            res.UserDeactivated,
		})
	})
	if err != nil {
		return OffboardingResult{}, err
	}
	return res, nil
}

// closeJobHistory closes the latest job record at term and inserts an
// INACTIVE record (same department/position/manager/type/grade) from
// term + 1.
func closeJobHistory(ctx context.Context, tx pgx.Tx, companyID, employeeID int64, term time.Time, actorID int64) error {
	var (
		id                                  int64
		validFrom                           time.Time
		validTo                             *time.Time
		departmentID, positionID, managerID *int64
		employmentType, grade               string
	)
	err := tx.QueryRow(ctx, `SELECT jh.id, jh.valid_from, jh.valid_to, jh.department_id, jh.position_id, jh.manager_id, jh.employment_type, jh.grade FROM hr_employee_job_history jh JOIN hr_employees e ON e.id=jh.employee_id AND e.company_id=$2 WHERE jh.employee_id=$1 ORDER BY jh.valid_from DESC LIMIT 1 FOR UPDATE OF jh`, employeeID, companyID).
		Scan(&id, &validFrom, &validTo, &departmentID, &positionID, &managerID, &employmentType, &grade)
	hasPrev := true
	if errors.Is(err, pgx.ErrNoRows) {
		hasPrev = false
		employmentType = employees.EmploymentPermanent
		if err := tx.QueryRow(ctx, `SELECT department_id, position_id, manager_id, employment_type FROM hr_employees WHERE id=$1 AND company_id=$2`, employeeID, companyID).Scan(&departmentID, &positionID, &managerID, &employmentType); err != nil {
			return fmt.Errorf("load employee job fields: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("lock job history: %w", err)
	}
	if hasPrev {
		if dateOnly(validFrom).After(term) {
			return ErrJobHistoryAfterLeaving
		}
		if validTo == nil || dateOnly(*validTo).After(term) {
			if _, err := tx.Exec(ctx, `UPDATE hr_employee_job_history SET valid_to=$2 WHERE id=$1`, id, term); err != nil {
				return fmt.Errorf("close job history: %w", err)
			}
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO hr_employee_job_history(company_id,employee_id,department_id,position_id,manager_id,status,employment_type,grade,valid_from,reason,created_by)
VALUES($1,$2,$3,$4,$5,'INACTIVE',$6,$7,$8,'Offboarded',$9)`, companyID, employeeID, departmentID, positionID, managerID, employmentType, grade, term.AddDate(0, 0, 1), nullableID(actorID)); err != nil {
		return fmt.Errorf("insert inactive job history: %w", err)
	}
	return nil
}

// CompleteOnboarding closes an OPEN onboarding checklist once no task is
// OPEN, audited CHECKLIST_COMPLETED.
func (s *Service) CompleteOnboarding(ctx context.Context, companyID, checklistID, actorID int64) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if checklistID <= 0 {
		return ErrChecklistNotFound
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		c, err := lockChecklist(ctx, tx, companyID, checklistID)
		if err != nil {
			return err
		}
		if c.kind != KindOnboarding {
			return ErrWrongKind
		}
		if c.status != StatusOpen {
			return ErrNotOpen
		}
		open, err := countOpenTasks(ctx, tx, companyID, checklistID)
		if err != nil {
			return err
		}
		if open > 0 {
			return fmt.Errorf("hr: complete or skip %d open checklist tasks first", open)
		}
		if _, err := tx.Exec(ctx, `UPDATE hr_employee_checklists SET status='COMPLETED', completed_by=$2, completed_at=NOW() WHERE id=$1`, checklistID, nullableID(actorID)); err != nil {
			return fmt.Errorf("complete checklist: %w", err)
		}
		return s.record(ctx, tx, actorID, ActionChecklistCompleted, entityChecklist, checklistID, map[string]any{"employee_id": c.employeeID, "kind": c.kind})
	})
}

// CancelOffboarding cancels an OPEN offboarding checklist, clears the
// employee's termination date and reason, leaves the tasks as they are, and
// audits OFFBOARDING_CANCELLED.
func (s *Service) CancelOffboarding(ctx context.Context, companyID, checklistID, actorID int64, reason string) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if checklistID <= 0 {
		return ErrChecklistNotFound
	}
	reason = strings.TrimSpace(reason)
	return s.inTx(ctx, func(tx pgx.Tx) error {
		c, err := lockChecklist(ctx, tx, companyID, checklistID)
		if err != nil {
			return err
		}
		if c.kind != KindOffboarding {
			return ErrWrongKind
		}
		if c.status != StatusOpen {
			return ErrNotOpen
		}
		if _, err := tx.Exec(ctx, `UPDATE hr_employee_checklists SET status='CANCELLED', completed_by=$2, completed_at=NOW() WHERE id=$1`, checklistID, nullableID(actorID)); err != nil {
			return fmt.Errorf("cancel checklist: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE hr_employees SET termination_date=NULL, termination_reason='', updated_at=NOW() WHERE id=$1 AND company_id=$2`, c.employeeID, companyID); err != nil {
			return fmt.Errorf("clear termination date: %w", err)
		}
		return s.record(ctx, tx, actorID, ActionOffboardingCanceled, entityChecklist, checklistID, map[string]any{"checklist_id": checklistID, "reason": reason})
	})
}
