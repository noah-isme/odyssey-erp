// Package performance manages performance review cycles (cycles,
// participants, goals, self and manager reviews) and the summary rows in
// hr_performance_reviews written when a participant is completed.
//
// Every query is company-scoped: participants and goals are always reached
// through their company-owning cycle. Every state change is audited with
// RecordTx inside the same transaction; audit metadata carries ids and stages
// only, never ratings, scores, or comments.
package performance

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Cycle statuses (hr_performance_cycles.status).
const (
	CycleDraft  = "DRAFT"
	CycleActive = "ACTIVE"
	CycleClosed = "CLOSED"
)

// Participant stages (hr_performance_participants.status).
const (
	StageEnrolled         = "ENROLLED"
	StageSelfSubmitted    = "SELF_SUBMITTED"
	StageManagerSubmitted = "MANAGER_SUBMITTED"
	StageCompleted        = "COMPLETED"
)

// Audit actions recorded by this package.
const (
	ActionCycleCreated       = "PERF_CYCLE_CREATED"
	ActionCycleActivated     = "PERF_CYCLE_ACTIVATED"
	ActionCycleClosed        = "PERF_CYCLE_CLOSED"
	ActionParticipantsAdded  = "PERF_PARTICIPANTS_ADDED"
	ActionGoalChanged        = "PERF_GOAL_CHANGED"
	ActionSelfSubmitted      = "PERF_SELF_SUBMITTED"
	ActionManagerSubmitted   = "PERF_MANAGER_SUBMITTED"
	ActionCompleted          = "PERF_COMPLETED"
	entityCycle              = "hr_performance_cycle"
	entityParticipant        = "hr_performance_participant"
	maxReviewPeriodRunes     = 100
	maxParticipantsPerInsert = 1000
)

var (
	// ErrNotFound is returned (possibly wrapped) when an id does not exist in
	// the caller's company. Cross-tenant ids are indistinguishable from
	// missing ones.
	ErrNotFound            = errors.New("hr: not found")
	ErrCycleNotFound       = fmt.Errorf("%w: performance cycle", ErrNotFound)
	ErrParticipantNotFound = fmt.Errorf("%w: performance participant", ErrNotFound)
	ErrGoalNotFound        = fmt.Errorf("%w: performance goal", ErrNotFound)
	ErrEmployeeNotFound    = fmt.Errorf("%w: employee", ErrNotFound)

	ErrCompanyRequired    = errors.New("hr: company required")
	ErrInvalidReview      = errors.New("hr: invalid performance review data")
	ErrInvalidCycle       = errors.New("hr: invalid performance cycle")
	ErrDuplicateCycleName = errors.New("hr: a performance cycle with this name already exists")
	ErrCycleNotDraft      = errors.New("hr: performance cycle is not a draft")
	ErrCycleNotActive     = errors.New("hr: performance cycle is not active")
	ErrCycleClosed        = errors.New("hr: performance cycle is closed")
	ErrEmployeeInactive   = errors.New("hr: only active employees can be added to a performance cycle")
	ErrNoParticipants     = errors.New("hr: performance cycle has no participants")
	ErrNoEmployees        = errors.New("hr: select at least one employee")
	ErrTooManyEmployees   = errors.New("hr: too many employees in one request")
	ErrInvalidGoal        = errors.New("hr: invalid performance goal")
	ErrInvalidRating      = errors.New("hr: rating must be between 1 and 5")
	ErrInvalidStage       = errors.New("hr: review is not at the expected stage")
	ErrSelfReviewPending  = errors.New("hr: self review not submitted and not yet due")
	ErrReviewerRequired   = errors.New("hr: participant has no reviewer")
	ErrForbidden          = errors.New("hr: not allowed to act on this review")

	errAuditNotConfigured     = errors.New("hr: performance audit not configured")
	errEmployeesNotConfigured = errors.New("hr: performance employee lookups not configured")
)

// AsOfResolver is satisfied by *employees.Service (effective-dated job
// lookups used to resolve each participant's reviewer).
type AsOfResolver interface {
	AssignmentAsOf(ctx context.Context, companyID, employeeID int64, asOf time.Time) (employees.JobHistory, error)
}

// Audit is satisfied by *shared.AuditLogger.
type Audit interface {
	RecordTx(context.Context, pgx.Tx, shared.AuditLog) error
}

type db interface {
	Begin(context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Review maps an hr_performance_reviews row (the summary written when a
// participant is completed, or a legacy standalone review).
type Review struct {
	ID           int64
	CompanyID    int64
	EmployeeID   int64
	ReviewerID   int64
	ReviewPeriod string
	Rating       int
	Comments     string
	CreatedAt    time.Time
}

// Service manages performance cycles and reviews.
type Service struct {
	pool      db
	employees AsOfResolver
	audit     Audit
	now       func() time.Time
}

// NewService builds a Service. employees resolves each participant's manager
// as of the cycle's period_end.
func NewService(pool *pgxpool.Pool, emp AsOfResolver, audit Audit) *Service {
	return &Service{pool: pool, employees: emp, audit: audit, now: time.Now}
}

// ListEmployeeReviews returns the employee's summary reviews in the company,
// newest first.
func (s *Service) ListEmployeeReviews(ctx context.Context, companyID, employeeID int64) ([]Review, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	rows, err := s.pool.Query(ctx, `SELECT id, company_id, employee_id, reviewer_id, review_period, rating, COALESCE(comments,''), created_at FROM hr_performance_reviews WHERE company_id = $1 AND employee_id = $2 ORDER BY created_at DESC`, companyID, employeeID)
	if err != nil {
		return nil, fmt.Errorf("list performance reviews: %w", err)
	}
	defer rows.Close()
	var out []Review
	for rows.Next() {
		var r Review
		if err := rows.Scan(&r.ID, &r.CompanyID, &r.EmployeeID, &r.ReviewerID, &r.ReviewPeriod, &r.Rating, &r.Comments, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan performance review: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CreateReview inserts a standalone summary review. Both the employee and the
// reviewer must belong to companyID; otherwise ErrEmployeeNotFound.
func (s *Service) CreateReview(ctx context.Context, companyID int64, r Review) (Review, error) {
	if companyID <= 0 {
		return Review{}, ErrCompanyRequired
	}
	if r.EmployeeID <= 0 || r.ReviewerID <= 0 || r.ReviewPeriod == "" || r.Rating < 1 || r.Rating > 5 {
		return Review{}, ErrInvalidReview
	}
	err := s.pool.QueryRow(ctx, `INSERT INTO hr_performance_reviews (company_id, employee_id, reviewer_id, review_period, rating, comments)
SELECT $1, $2, $3, $4, $5, $6
WHERE EXISTS (SELECT 1 FROM hr_employees WHERE id = $2 AND company_id = $1)
  AND EXISTS (SELECT 1 FROM hr_employees WHERE id = $3 AND company_id = $1)
RETURNING id, company_id, employee_id, reviewer_id, review_period, rating, COALESCE(comments,''), created_at`,
		companyID, r.EmployeeID, r.ReviewerID, r.ReviewPeriod, r.Rating, r.Comments).
		Scan(&r.ID, &r.CompanyID, &r.EmployeeID, &r.ReviewerID, &r.ReviewPeriod, &r.Rating, &r.Comments, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Review{}, ErrEmployeeNotFound
	}
	if err != nil {
		return Review{}, fmt.Errorf("create performance review: %w", err)
	}
	return r, nil
}

func (s *Service) today() time.Time {
	if s.now == nil {
		return dateOnly(time.Now())
	}
	return dateOnly(s.now())
}

func (s *Service) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, actorID int64, action, entity string, id int64, meta map[string]any) error {
	if s.audit == nil {
		return errAuditNotConfigured
	}
	if err := s.audit.RecordTx(ctx, tx, shared.AuditLog{ActorID: actorID, Action: action, Entity: entity, EntityID: strconv.FormatInt(id, 10), Meta: meta}); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func validRating(r int) bool { return r >= 1 && r <= 5 }
