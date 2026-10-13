package performance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
)

// Cycle maps an hr_performance_cycles row plus participant counts.
type Cycle struct {
	ID, CompanyID                   int64
	Name                            string
	PeriodStart, PeriodEnd          time.Time
	SelfReviewDue, ManagerReviewDue time.Time
	Status                          string
	CreatedBy                       *int64
	CreatedAt, UpdatedAt            time.Time
	ParticipantCount                int64
	CompletedCount                  int64
}

// CycleInput describes a new cycle.
type CycleInput struct {
	Name                            string
	PeriodStart, PeriodEnd          time.Time
	SelfReviewDue, ManagerReviewDue time.Time
	ActorID                         int64
}

const cycleSelect = `SELECT c.id,c.company_id,c.name,c.period_start,c.period_end,c.self_review_due,c.manager_review_due,c.status,c.created_by,c.created_at,c.updated_at,
(SELECT COUNT(*) FROM hr_performance_participants p WHERE p.cycle_id=c.id),
(SELECT COUNT(*) FROM hr_performance_participants p WHERE p.cycle_id=c.id AND p.status='COMPLETED')
FROM hr_performance_cycles c`

func scanCycle(row pgx.Row) (Cycle, error) {
	var c Cycle
	err := row.Scan(&c.ID, &c.CompanyID, &c.Name, &c.PeriodStart, &c.PeriodEnd, &c.SelfReviewDue, &c.ManagerReviewDue, &c.Status, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
		&c.ParticipantCount, &c.CompletedCount)
	return c, err
}

func validateCycle(in CycleInput) (CycleInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || in.PeriodStart.IsZero() || in.PeriodEnd.IsZero() || in.SelfReviewDue.IsZero() || in.ManagerReviewDue.IsZero() {
		return in, ErrInvalidCycle
	}
	in.PeriodStart, in.PeriodEnd = dateOnly(in.PeriodStart), dateOnly(in.PeriodEnd)
	in.SelfReviewDue, in.ManagerReviewDue = dateOnly(in.SelfReviewDue), dateOnly(in.ManagerReviewDue)
	if in.PeriodEnd.Before(in.PeriodStart) || in.ManagerReviewDue.Before(in.SelfReviewDue) || in.SelfReviewDue.Before(in.PeriodStart) {
		return in, ErrInvalidCycle
	}
	return in, nil
}

// CreateCycle inserts a DRAFT cycle.
func (s *Service) CreateCycle(ctx context.Context, companyID int64, in CycleInput) (Cycle, error) {
	if companyID <= 0 {
		return Cycle{}, ErrCompanyRequired
	}
	in, err := validateCycle(in)
	if err != nil {
		return Cycle{}, err
	}
	var out Cycle
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var createdBy *int64
		if in.ActorID > 0 {
			createdBy = &in.ActorID
		}
		var id int64
		if err := tx.QueryRow(ctx, `INSERT INTO hr_performance_cycles(company_id,name,period_start,period_end,self_review_due,manager_review_due,status,created_by)
VALUES ($1,$2,$3,$4,$5,$6,'DRAFT',$7) RETURNING id`, companyID, in.Name, in.PeriodStart, in.PeriodEnd, in.SelfReviewDue, in.ManagerReviewDue, createdBy).Scan(&id); err != nil {
			if isUniqueViolation(err) {
				return ErrDuplicateCycleName
			}
			return fmt.Errorf("insert performance cycle: %w", err)
		}
		c, err := scanCycle(tx.QueryRow(ctx, cycleSelect+` WHERE c.company_id=$1 AND c.id=$2`, companyID, id))
		if err != nil {
			return fmt.Errorf("load performance cycle: %w", err)
		}
		out = c
		return s.record(ctx, tx, in.ActorID, ActionCycleCreated, entityCycle, id, map[string]any{"cycle_id": id, "status": CycleDraft})
	})
	return out, err
}

// ListCycles returns the company's cycles, newest period first.
func (s *Service) ListCycles(ctx context.Context, companyID int64) ([]Cycle, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	rows, err := s.pool.Query(ctx, cycleSelect+` WHERE c.company_id=$1 ORDER BY c.period_end DESC, c.id DESC`, companyID)
	if err != nil {
		return nil, fmt.Errorf("list performance cycles: %w", err)
	}
	defer rows.Close()
	var out []Cycle
	for rows.Next() {
		c, err := scanCycle(rows)
		if err != nil {
			return nil, fmt.Errorf("scan performance cycle: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCycle returns one cycle of the company, or ErrCycleNotFound.
func (s *Service) GetCycle(ctx context.Context, companyID, cycleID int64) (Cycle, error) {
	if companyID <= 0 {
		return Cycle{}, ErrCompanyRequired
	}
	if cycleID <= 0 {
		return Cycle{}, ErrCycleNotFound
	}
	c, err := scanCycle(s.pool.QueryRow(ctx, cycleSelect+` WHERE c.company_id=$1 AND c.id=$2`, companyID, cycleID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Cycle{}, ErrCycleNotFound
	}
	if err != nil {
		return Cycle{}, fmt.Errorf("get performance cycle: %w", err)
	}
	return c, nil
}

// lockCycle locks the company's cycle row and returns its status and
// period_end.
func lockCycle(ctx context.Context, tx pgx.Tx, companyID, cycleID int64) (string, time.Time, error) {
	var status string
	var periodEnd time.Time
	err := tx.QueryRow(ctx, `SELECT status, period_end FROM hr_performance_cycles WHERE id=$1 AND company_id=$2 FOR UPDATE`, cycleID, companyID).Scan(&status, &periodEnd)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, ErrCycleNotFound
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("lock performance cycle: %w", err)
	}
	return status, periodEnd, nil
}

// ActivateCycle moves a DRAFT cycle with at least one participant to ACTIVE.
func (s *Service) ActivateCycle(ctx context.Context, companyID, cycleID, actorID int64) error {
	return s.transitionCycle(ctx, companyID, cycleID, actorID, CycleDraft, CycleActive, ActionCycleActivated)
}

// CloseCycle moves an ACTIVE cycle to CLOSED. Participants that are not yet
// completed stay at their stage; an admin may still complete them.
func (s *Service) CloseCycle(ctx context.Context, companyID, cycleID, actorID int64) error {
	return s.transitionCycle(ctx, companyID, cycleID, actorID, CycleActive, CycleClosed, ActionCycleClosed)
}

func (s *Service) transitionCycle(ctx context.Context, companyID, cycleID, actorID int64, from, to, action string) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if cycleID <= 0 {
		return ErrCycleNotFound
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		status, _, err := lockCycle(ctx, tx, companyID, cycleID)
		if err != nil {
			return err
		}
		if status != from {
			if from == CycleDraft {
				return ErrCycleNotDraft
			}
			return ErrCycleNotActive
		}
		if to == CycleActive {
			var n int64
			if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM hr_performance_participants WHERE cycle_id=$1 AND company_id=$2`, cycleID, companyID).Scan(&n); err != nil {
				return fmt.Errorf("count performance participants: %w", err)
			}
			if n < 1 {
				return ErrNoParticipants
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE hr_performance_cycles SET status=$3, updated_at=NOW() WHERE id=$1 AND company_id=$2`, cycleID, companyID, to); err != nil {
			return fmt.Errorf("update performance cycle: %w", err)
		}
		return s.record(ctx, tx, actorID, action, entityCycle, cycleID, map[string]any{"cycle_id": cycleID, "from": from, "status": to})
	})
}

// AddParticipants enrolls employees in a DRAFT or ACTIVE cycle. Every
// employee id must belong to companyID and be ACTIVE (otherwise
// ErrEmployeeNotFound or ErrEmployeeInactive and nothing is inserted). Each participant's reviewer is the manager on their
// job record effective at the cycle's period_end (none when no record is
// effective). Employees already enrolled are skipped. It returns the number
// of participants added.
func (s *Service) AddParticipants(ctx context.Context, companyID, cycleID int64, employeeIDs []int64, actorID int64) (int, error) {
	if companyID <= 0 {
		return 0, ErrCompanyRequired
	}
	if cycleID <= 0 {
		return 0, ErrCycleNotFound
	}
	ids := dedupeIDs(employeeIDs)
	if len(ids) == 0 {
		return 0, ErrNoEmployees
	}
	if len(ids) > maxParticipantsPerInsert {
		return 0, ErrTooManyEmployees
	}
	for _, id := range ids {
		if id <= 0 {
			return 0, ErrEmployeeNotFound
		}
	}
	if s.employees == nil {
		return 0, errEmployeesNotConfigured
	}
	added := 0
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		status, periodEnd, err := lockCycle(ctx, tx, companyID, cycleID)
		if err != nil {
			return err
		}
		if status == CycleClosed {
			return ErrCycleClosed
		}
		var found, inactive int64
		if err := tx.QueryRow(ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE status<>'ACTIVE') FROM hr_employees WHERE company_id=$1 AND id = ANY($2)`, companyID, ids).Scan(&found, &inactive); err != nil {
			return fmt.Errorf("validate performance participants: %w", err)
		}
		if found != int64(len(ids)) {
			return ErrEmployeeNotFound
		}
		if inactive > 0 {
			return ErrEmployeeInactive
		}
		for _, empID := range ids {
			var reviewer *int64
			job, err := s.employees.AssignmentAsOf(ctx, companyID, empID, periodEnd)
			switch {
			case errors.Is(err, employees.ErrNoJobAssignment):
			case err != nil:
				return fmt.Errorf("resolve reviewer: %w", err)
			default:
				reviewer = job.ManagerID
			}
			tag, err := tx.Exec(ctx, `INSERT INTO hr_performance_participants(company_id,cycle_id,employee_id,reviewer_employee_id,status)
VALUES ($1,$2,$3,$4,'ENROLLED') ON CONFLICT (cycle_id, employee_id) DO NOTHING`, companyID, cycleID, empID, reviewer)
			if err != nil {
				return fmt.Errorf("insert performance participant: %w", err)
			}
			added += int(tag.RowsAffected())
		}
		return s.record(ctx, tx, actorID, ActionParticipantsAdded, entityCycle, cycleID, map[string]any{"cycle_id": cycleID, "employee_ids": ids, "added": added})
	})
	if err != nil {
		return 0, err
	}
	return added, nil
}

func dedupeIDs(in []int64) []int64 {
	seen := make(map[int64]struct{}, len(in))
	out := make([]int64, 0, len(in))
	for _, id := range in {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
