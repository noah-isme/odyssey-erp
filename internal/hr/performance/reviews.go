package performance

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Participant maps an hr_performance_participants row with its cycle,
// employee, and reviewer context.
type Participant struct {
	ID, CompanyID, CycleID          int64
	CycleName, CycleStatus          string
	PeriodStart, PeriodEnd          time.Time
	SelfReviewDue, ManagerReviewDue time.Time
	EmployeeID                      int64
	EmployeeName                    string
	EmployeeUserID                  *int64
	ReviewerEmployeeID              *int64
	ReviewerName                    string
	ReviewerUserID                  *int64
	Status                          string
	SelfRating                      *int
	SelfComments                    string
	ManagerRating                   *int
	ManagerComments                 string
	FinalRating                     *int
	SelfSubmittedAt                 *time.Time
	ManagerSubmittedAt              *time.Time
	CompletedAt                     *time.Time
	ReviewID                        *int64
}

// Goal maps an hr_performance_goals row. Weight is NUMERIC(5,2) text.
type Goal struct {
	ID, ParticipantID int64
	Seq               int
	Title             string
	Description       string
	KPITarget         string
	Weight            string
	SelfScore         *int
	ManagerScore      *int
	CreatedAt         time.Time
}

// weightPattern matches NUMERIC(5,2) text without sign or exponent.
var weightPattern = regexp.MustCompile(`^[0-9]{1,3}(\.[0-9]{1,2})?$`)

// GoalInput describes a goal to add or update. Weight is a decimal string
// between 0 and 100 (empty means 0).
type GoalInput struct {
	Title, Description, KPITarget, Weight string
}

// ReviewInput carries a self or manager review: an overall rating (1-5),
// comments, and optional per-goal scores (goal id -> 1-5).
type ReviewInput struct {
	Rating     int
	Comments   string
	GoalScores map[int64]int
}

// participantSelect reaches participants through their company-owning cycle
// ($1 is always the company id).
const participantSelect = `SELECT p.id,c.company_id,p.cycle_id,c.name,c.status,c.period_start,c.period_end,c.self_review_due,c.manager_review_due,
p.employee_id,e.name,e.user_id,p.reviewer_employee_id,COALESCE(r.name,''),r.user_id,p.status,p.self_rating,p.self_comments,p.manager_rating,p.manager_comments,p.final_rating,
p.self_submitted_at,p.manager_submitted_at,p.completed_at,p.review_id
FROM hr_performance_participants p
JOIN hr_performance_cycles c ON c.id=p.cycle_id AND c.company_id=$1
JOIN hr_employees e ON e.id=p.employee_id AND e.company_id=c.company_id
LEFT JOIN hr_employees r ON r.id=p.reviewer_employee_id AND r.company_id=c.company_id`

func scanParticipant(row pgx.Row) (Participant, error) {
	var p Participant
	err := row.Scan(&p.ID, &p.CompanyID, &p.CycleID, &p.CycleName, &p.CycleStatus, &p.PeriodStart, &p.PeriodEnd, &p.SelfReviewDue, &p.ManagerReviewDue,
		&p.EmployeeID, &p.EmployeeName, &p.EmployeeUserID, &p.ReviewerEmployeeID, &p.ReviewerName, &p.ReviewerUserID, &p.Status, &p.SelfRating, &p.SelfComments,
		&p.ManagerRating, &p.ManagerComments, &p.FinalRating, &p.SelfSubmittedAt, &p.ManagerSubmittedAt, &p.CompletedAt, &p.ReviewID)
	return p, err
}

func (s *Service) queryParticipants(ctx context.Context, where string, args ...any) ([]Participant, error) {
	rows, err := s.pool.Query(ctx, participantSelect+where, args...)
	if err != nil {
		return nil, fmt.Errorf("list performance participants: %w", err)
	}
	defer rows.Close()
	var out []Participant
	for rows.Next() {
		p, err := scanParticipant(rows)
		if err != nil {
			return nil, fmt.Errorf("scan performance participant: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListParticipants returns the participants of a company cycle, by employee
// name.
func (s *Service) ListParticipants(ctx context.Context, companyID, cycleID int64) ([]Participant, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	return s.queryParticipants(ctx, ` WHERE p.cycle_id=$2 ORDER BY e.name, p.id`, companyID, cycleID)
}

// GetParticipant returns one participant of the company, or
// ErrParticipantNotFound. Callers enforce who may view it.
func (s *Service) GetParticipant(ctx context.Context, companyID, participantID int64) (Participant, error) {
	if companyID <= 0 {
		return Participant{}, ErrCompanyRequired
	}
	if participantID <= 0 {
		return Participant{}, ErrParticipantNotFound
	}
	p, err := scanParticipant(s.pool.QueryRow(ctx, participantSelect+` WHERE p.id=$2`, companyID, participantID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Participant{}, ErrParticipantNotFound
	}
	if err != nil {
		return Participant{}, fmt.Errorf("get performance participant: %w", err)
	}
	return p, nil
}

// MyReviews returns the reviews (non-draft cycles) where the user is the
// participant, newest period first.
func (s *Service) MyReviews(ctx context.Context, companyID, userID int64) ([]Participant, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	if userID <= 0 {
		return nil, nil
	}
	return s.queryParticipants(ctx, ` WHERE e.user_id=$2 AND c.status<>'DRAFT' ORDER BY c.period_end DESC, p.id DESC`, companyID, userID)
}

// TeamReviews returns the reviews (non-draft cycles) where the user is the
// reviewer, newest period first then by employee name.
func (s *Service) TeamReviews(ctx context.Context, companyID, userID int64) ([]Participant, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	if userID <= 0 {
		return nil, nil
	}
	return s.queryParticipants(ctx, ` WHERE r.user_id=$2 AND c.status<>'DRAFT' ORDER BY c.period_end DESC, e.name, p.id`, companyID, userID)
}

// ListGoals returns a company participant's goals in sequence order.
func (s *Service) ListGoals(ctx context.Context, companyID, participantID int64) ([]Goal, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	rows, err := s.pool.Query(ctx, `SELECT g.id,g.participant_id,g.seq,g.title,g.description,g.kpi_target,g.weight::text,g.self_score,g.manager_score,g.created_at
FROM hr_performance_goals g
JOIN hr_performance_participants p ON p.id=g.participant_id
JOIN hr_performance_cycles c ON c.id=p.cycle_id AND c.company_id=$1
WHERE g.participant_id=$2 ORDER BY g.seq`, companyID, participantID)
	if err != nil {
		return nil, fmt.Errorf("list performance goals: %w", err)
	}
	defer rows.Close()
	var out []Goal
	for rows.Next() {
		var g Goal
		if err := rows.Scan(&g.ID, &g.ParticipantID, &g.Seq, &g.Title, &g.Description, &g.KPITarget, &g.Weight, &g.SelfScore, &g.ManagerScore, &g.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan performance goal: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// lockedParticipant is the authorization and stage context of a locked
// participant row.
type lockedParticipant struct {
	id, cycleID, employeeID       int64
	cycleName, cycleStatus, stage string
	selfReviewDue                 time.Time
	employeeUserID                *int64
	reviewerEmployeeID            *int64
	reviewerUserID                *int64
	managerComments               string
}

// lockParticipant locks the participant row, reached through its
// company-owning cycle.
func lockParticipant(ctx context.Context, tx pgx.Tx, companyID, participantID int64) (lockedParticipant, error) {
	var lp lockedParticipant
	err := tx.QueryRow(ctx, `SELECT p.id,p.cycle_id,p.employee_id,c.name,c.status,p.status,c.self_review_due,e.user_id,p.reviewer_employee_id,r.user_id,p.manager_comments
FROM hr_performance_participants p
JOIN hr_performance_cycles c ON c.id=p.cycle_id AND c.company_id=$2
JOIN hr_employees e ON e.id=p.employee_id AND e.company_id=c.company_id
LEFT JOIN hr_employees r ON r.id=p.reviewer_employee_id AND r.company_id=c.company_id
WHERE p.id=$1 FOR UPDATE OF p`, participantID, companyID).
		Scan(&lp.id, &lp.cycleID, &lp.employeeID, &lp.cycleName, &lp.cycleStatus, &lp.stage, &lp.selfReviewDue, &lp.employeeUserID, &lp.reviewerEmployeeID, &lp.reviewerUserID, &lp.managerComments)
	if errors.Is(err, pgx.ErrNoRows) {
		return lockedParticipant{}, ErrParticipantNotFound
	}
	if err != nil {
		return lockedParticipant{}, fmt.Errorf("lock performance participant: %w", err)
	}
	return lp, nil
}

func sameUser(id *int64, actorID int64) bool { return id != nil && actorID > 0 && *id == actorID }

// goalEditable checks that the actor (participant's user or admin) may edit
// goals: the cycle must be ACTIVE and the participant not completed.
func goalEditable(lp lockedParticipant, actorID int64, isAdmin bool) error {
	if !isAdmin && !sameUser(lp.employeeUserID, actorID) {
		return ErrForbidden
	}
	if lp.cycleStatus != CycleActive {
		return ErrCycleNotActive
	}
	if lp.stage == StageCompleted {
		return ErrInvalidStage
	}
	return nil
}

func validateGoal(in GoalInput) (GoalInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.KPITarget = strings.TrimSpace(in.KPITarget)
	in.Weight = strings.TrimSpace(in.Weight)
	if in.Weight == "" {
		in.Weight = "0"
	}
	if in.Title == "" {
		return in, ErrInvalidGoal
	}
	if !weightPattern.MatchString(in.Weight) {
		return in, ErrInvalidGoal
	}
	if w, err := strconv.ParseFloat(in.Weight, 64); err != nil || w > 100 {
		return in, ErrInvalidGoal
	}
	return in, nil
}

// AddGoal appends a goal to the participant. Allowed for the participant's
// user or an admin while the cycle is ACTIVE.
func (s *Service) AddGoal(ctx context.Context, companyID, participantID, actorID int64, isAdmin bool, in GoalInput) (Goal, error) {
	if companyID <= 0 {
		return Goal{}, ErrCompanyRequired
	}
	if participantID <= 0 {
		return Goal{}, ErrParticipantNotFound
	}
	in, err := validateGoal(in)
	if err != nil {
		return Goal{}, err
	}
	var out Goal
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		lp, err := lockParticipant(ctx, tx, companyID, participantID)
		if err != nil {
			return err
		}
		if err := goalEditable(lp, actorID, isAdmin); err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `INSERT INTO hr_performance_goals(company_id,participant_id,seq,title,description,kpi_target,weight)
VALUES ($1,$2,(SELECT COALESCE(MAX(seq),0)+1 FROM hr_performance_goals WHERE participant_id=$2),$3,$4,$5,$6::text::numeric)
RETURNING id,participant_id,seq,title,description,kpi_target,weight::text,self_score,manager_score,created_at`,
			companyID, participantID, in.Title, in.Description, in.KPITarget, in.Weight).
			Scan(&out.ID, &out.ParticipantID, &out.Seq, &out.Title, &out.Description, &out.KPITarget, &out.Weight, &out.SelfScore, &out.ManagerScore, &out.CreatedAt)
		if err != nil {
			return fmt.Errorf("insert performance goal: %w", err)
		}
		return s.recordGoal(ctx, tx, actorID, lp, out.ID, "added")
	})
	if err != nil {
		return Goal{}, err
	}
	return out, nil
}

// UpdateGoal edits a goal's title, description, KPI target, and weight
// (same rules as AddGoal).
func (s *Service) UpdateGoal(ctx context.Context, companyID, goalID, actorID int64, isAdmin bool, in GoalInput) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if goalID <= 0 {
		return ErrGoalNotFound
	}
	in, err := validateGoal(in)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		lp, err := s.lockGoalParticipant(ctx, tx, companyID, goalID)
		if err != nil {
			return err
		}
		if err := goalEditable(lp, actorID, isAdmin); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE hr_performance_goals SET title=$2, description=$3, kpi_target=$4, weight=$5::text::numeric WHERE id=$1`,
			goalID, in.Title, in.Description, in.KPITarget, in.Weight); err != nil {
			return fmt.Errorf("update performance goal: %w", err)
		}
		return s.recordGoal(ctx, tx, actorID, lp, goalID, "updated")
	})
}

// DeleteGoal removes a goal (same rules as AddGoal).
func (s *Service) DeleteGoal(ctx context.Context, companyID, goalID, actorID int64, isAdmin bool) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if goalID <= 0 {
		return ErrGoalNotFound
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		lp, err := s.lockGoalParticipant(ctx, tx, companyID, goalID)
		if err != nil {
			return err
		}
		if err := goalEditable(lp, actorID, isAdmin); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM hr_performance_goals WHERE id=$1`, goalID); err != nil {
			return fmt.Errorf("delete performance goal: %w", err)
		}
		return s.recordGoal(ctx, tx, actorID, lp, goalID, "deleted")
	})
}

// lockGoalParticipant resolves the goal's participant through the company's
// cycle and locks the participant row.
func (s *Service) lockGoalParticipant(ctx context.Context, tx pgx.Tx, companyID, goalID int64) (lockedParticipant, error) {
	var participantID int64
	err := tx.QueryRow(ctx, `SELECT g.participant_id FROM hr_performance_goals g
JOIN hr_performance_participants p ON p.id=g.participant_id
JOIN hr_performance_cycles c ON c.id=p.cycle_id AND c.company_id=$2
WHERE g.id=$1`, goalID, companyID).Scan(&participantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return lockedParticipant{}, ErrGoalNotFound
	}
	if err != nil {
		return lockedParticipant{}, fmt.Errorf("find performance goal: %w", err)
	}
	return lockParticipant(ctx, tx, companyID, participantID)
}

func (s *Service) recordGoal(ctx context.Context, tx pgx.Tx, actorID int64, lp lockedParticipant, goalID int64, change string) error {
	return s.record(ctx, tx, actorID, ActionGoalChanged, entityParticipant, lp.id, map[string]any{
		"cycle_id": lp.cycleID, "participant_id": lp.id, "goal_id": goalID, "change": change, "stage": lp.stage,
	})
}

func validateReview(in ReviewInput) (ReviewInput, error) {
	if !validRating(in.Rating) {
		return in, ErrInvalidRating
	}
	for id, score := range in.GoalScores {
		if id <= 0 {
			return in, ErrGoalNotFound
		}
		if !validRating(score) {
			return in, ErrInvalidRating
		}
	}
	in.Comments = strings.TrimSpace(in.Comments)
	return in, nil
}

// applyGoalScores writes per-goal scores; every goal must belong to the
// participant.
func applyGoalScores(ctx context.Context, tx pgx.Tx, participantID int64, column string, scores map[int64]int) error {
	for goalID, score := range scores {
		tag, err := tx.Exec(ctx, `UPDATE hr_performance_goals SET `+column+`=$3 WHERE id=$1 AND participant_id=$2`, goalID, participantID, score)
		if err != nil {
			return fmt.Errorf("score performance goal: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrGoalNotFound
		}
	}
	return nil
}

// SubmitSelfReview records the participant's self review. Only the
// participant's own user may submit it (no admin override), the cycle must
// be ACTIVE, and the stage must be ENROLLED.
func (s *Service) SubmitSelfReview(ctx context.Context, companyID, participantID, actorID int64, in ReviewInput) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if participantID <= 0 {
		return ErrParticipantNotFound
	}
	in, err := validateReview(in)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		lp, err := lockParticipant(ctx, tx, companyID, participantID)
		if err != nil {
			return err
		}
		if !sameUser(lp.employeeUserID, actorID) {
			return ErrForbidden
		}
		if lp.cycleStatus != CycleActive {
			return ErrCycleNotActive
		}
		if lp.stage != StageEnrolled {
			return ErrInvalidStage
		}
		if err := applyGoalScores(ctx, tx, participantID, "self_score", in.GoalScores); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE hr_performance_participants SET status='SELF_SUBMITTED', self_rating=$2, self_comments=$3, self_submitted_at=NOW() WHERE id=$1`,
			participantID, in.Rating, in.Comments); err != nil {
			return fmt.Errorf("submit self review: %w", err)
		}
		return s.record(ctx, tx, actorID, ActionSelfSubmitted, entityParticipant, participantID, map[string]any{
			"cycle_id": lp.cycleID, "participant_id": participantID, "stage": StageSelfSubmitted,
		})
	})
}

// SubmitManagerReview records the reviewer's review. Only the reviewer's
// user or an admin may submit it, the cycle must be ACTIVE, and the stage
// must be SELF_SUBMITTED, or ENROLLED once self_review_due has passed.
func (s *Service) SubmitManagerReview(ctx context.Context, companyID, participantID, actorID int64, isAdmin bool, in ReviewInput) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if participantID <= 0 {
		return ErrParticipantNotFound
	}
	in, err := validateReview(in)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		lp, err := lockParticipant(ctx, tx, companyID, participantID)
		if err != nil {
			return err
		}
		if !isAdmin && !sameUser(lp.reviewerUserID, actorID) {
			return ErrForbidden
		}
		if lp.cycleStatus != CycleActive {
			return ErrCycleNotActive
		}
		switch lp.stage {
		case StageSelfSubmitted:
		case StageEnrolled:
			if !s.today().After(dateOnly(lp.selfReviewDue)) {
				return ErrSelfReviewPending
			}
		default:
			return ErrInvalidStage
		}
		if err := applyGoalScores(ctx, tx, participantID, "manager_score", in.GoalScores); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE hr_performance_participants SET status='MANAGER_SUBMITTED', manager_rating=$2, manager_comments=$3, manager_submitted_at=NOW() WHERE id=$1`,
			participantID, in.Rating, in.Comments); err != nil {
			return fmt.Errorf("submit manager review: %w", err)
		}
		return s.record(ctx, tx, actorID, ActionManagerSubmitted, entityParticipant, participantID, map[string]any{
			"cycle_id": lp.cycleID, "participant_id": participantID, "from": lp.stage, "stage": StageManagerSubmitted,
		})
	})
}

// Complete finalizes a MANAGER_SUBMITTED participant of an ACTIVE or CLOSED
// cycle: it inserts the summary row into hr_performance_reviews (with
// company_id, the cycle name as review_period, and the manager comments),
// links it via review_id, and marks the participant COMPLETED. Callers must
// restrict it to performance admins. The reviewer is the participant's
// reviewer, or the actor's employee record in the company when the
// participant has none (ErrReviewerRequired when neither exists). It
// returns the new review id.
func (s *Service) Complete(ctx context.Context, companyID, participantID, actorID int64, finalRating int) (int64, error) {
	if companyID <= 0 {
		return 0, ErrCompanyRequired
	}
	if participantID <= 0 {
		return 0, ErrParticipantNotFound
	}
	if !validRating(finalRating) {
		return 0, ErrInvalidRating
	}
	var reviewID int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		lp, err := lockParticipant(ctx, tx, companyID, participantID)
		if err != nil {
			return err
		}
		if lp.cycleStatus == CycleDraft {
			return ErrCycleNotActive
		}
		if lp.stage != StageManagerSubmitted {
			return ErrInvalidStage
		}
		reviewer := lp.reviewerEmployeeID
		if reviewer == nil {
			var id int64
			err := tx.QueryRow(ctx, `SELECT id FROM hr_employees WHERE user_id=$1 AND company_id=$2`, actorID, companyID).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrReviewerRequired
			}
			if err != nil {
				return fmt.Errorf("resolve completing reviewer: %w", err)
			}
			reviewer = &id
		}
		if err := tx.QueryRow(ctx, `INSERT INTO hr_performance_reviews(company_id,employee_id,reviewer_id,review_period,rating,comments)
VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, companyID, lp.employeeID, *reviewer, truncateRunes(lp.cycleName, maxReviewPeriodRunes), finalRating, lp.managerComments).Scan(&reviewID); err != nil {
			return fmt.Errorf("insert performance review summary: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE hr_performance_participants SET status='COMPLETED', final_rating=$2, completed_at=NOW(), review_id=$3 WHERE id=$1`,
			participantID, finalRating, reviewID); err != nil {
			return fmt.Errorf("complete performance participant: %w", err)
		}
		return s.record(ctx, tx, actorID, ActionCompleted, entityParticipant, participantID, map[string]any{
			"cycle_id": lp.cycleID, "participant_id": participantID, "review_id": reviewID, "stage": StageCompleted,
		})
	})
	if err != nil {
		return 0, err
	}
	return reviewID, nil
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
