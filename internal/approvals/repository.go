package approvals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) ResolvePolicy(ctx context.Context, module string, companyID *int64, amount float64) (Policy, error) {
	var p Policy
	err := r.pool.QueryRow(ctx, `SELECT id,name,module,company_id,min_amount,max_amount,is_active
		FROM approval_policies WHERE module=$1 AND is_active=TRUE
		AND (company_id=$2 OR company_id IS NULL) AND min_amount <= $3 AND (max_amount IS NULL OR max_amount >= $3)
		ORDER BY (company_id IS NOT NULL) DESC, min_amount DESC, id DESC LIMIT 1`, module, companyID, amount).
		Scan(&p.ID, &p.Name, &p.Module, &p.CompanyID, &p.MinAmount, &p.MaxAmount, &p.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{}, ErrNoPolicy
	}
	if err != nil {
		return Policy{}, err
	}
	p.Steps, err = r.loadSteps(ctx, r.pool, p.ID)
	return p, err
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (r *Repository) loadSteps(ctx context.Context, q queryer, policyID int64) ([]PolicyStep, error) {
	rows, err := q.Query(ctx, `SELECT id,policy_id,step_order,name,approver_user_id,approver_role_id,required_approvals,escalation_hours
		FROM approval_policy_steps WHERE policy_id=$1 ORDER BY step_order`, policyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var steps []PolicyStep
	for rows.Next() {
		var s PolicyStep
		if err := rows.Scan(&s.ID, &s.PolicyID, &s.Order, &s.Name, &s.ApproverUserID, &s.ApproverRoleID, &s.RequiredApprovals, &s.EscalationHours); err != nil {
			return nil, err
		}
		steps = append(steps, s)
	}
	return steps, rows.Err()
}

func (r *Repository) CreateRequest(ctx context.Context, policy Policy, in Submission) (Request, []int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Request{}, nil, err
	}
	defer tx.Rollback(ctx)
	var req Request
	err = tx.QueryRow(ctx, `INSERT INTO approval_requests(policy_id,module,document_id,company_id,amount,requester_id,current_step,status)
		VALUES($1,$2,$3,$4,$5,$6,1,'PENDING') RETURNING id,policy_id,module,document_id,company_id,amount,requester_id,current_step,status,submitted_at`,
		policy.ID, in.Module, in.DocumentID, in.CompanyID, in.Amount, in.RequesterID).Scan(&req.ID, &req.PolicyID, &req.Module, &req.DocumentID, &req.CompanyID, &req.Amount, &req.RequesterID, &req.CurrentStep, &req.Status, &req.SubmittedAt)
	if err != nil {
		return Request{}, nil, err
	}
	for _, step := range policy.Steps {
		users, err := r.resolveStepUsers(ctx, tx, step, in.Module)
		if err != nil {
			return Request{}, nil, err
		}
		if len(users) == 0 {
			return Request{}, nil, fmt.Errorf("approvals: step %d has no approvers", step.Order)
		}
		for _, u := range users {
			var due any
			if step.EscalationHours != nil {
				due = time.Now().Add(time.Duration(*step.EscalationHours) * time.Hour)
			}
			_, err = tx.Exec(ctx, `INSERT INTO approval_assignments(request_id,policy_step_id,step_order,approver_id,delegated_from,status,due_at)
				VALUES($1,$2,$3,$4,$5,'PENDING',$6)`, req.ID, step.ID, step.Order, u.userID, u.delegatedFrom, due)
			if err != nil {
				return Request{}, nil, err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Request{}, nil, err
	}
	return req, r.assigneesForStep(policy.Steps, req.CurrentStep, ctx, req.ID), nil
}

type resolvedUser struct {
	userID        int64
	delegatedFrom *int64
}

func (r *Repository) resolveStepUsers(ctx context.Context, tx pgx.Tx, step PolicyStep, module string) ([]resolvedUser, error) {
	var ids []int64
	if step.ApproverUserID != nil {
		ids = []int64{*step.ApproverUserID}
	} else {
		rows, err := tx.Query(ctx, `SELECT ur.user_id FROM user_roles ur JOIN users u ON u.id=ur.user_id WHERE ur.role_id=$1 AND u.is_active`, step.ApproverRoleID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	result := make([]resolvedUser, 0, len(ids))
	for _, id := range ids {
		var delegate int64
		err := tx.QueryRow(ctx, `SELECT delegate_id FROM approval_delegations WHERE delegator_id=$1 AND is_active AND starts_at<=NOW() AND ends_at>=NOW() AND (module IS NULL OR module=$2) ORDER BY starts_at DESC LIMIT 1`, id, module).Scan(&delegate)
		if err == nil {
			original := id
			result = append(result, resolvedUser{delegate, &original})
		} else if errors.Is(err, pgx.ErrNoRows) {
			result = append(result, resolvedUser{userID: id})
		} else {
			return nil, err
		}
	}
	return result, nil
}

func (r *Repository) assigneesForStep(_ []PolicyStep, step int, ctx context.Context, requestID int64) []int64 {
	rows, err := r.pool.Query(ctx, `SELECT approver_id FROM approval_assignments WHERE request_id=$1 AND step_order=$2 AND status='PENDING'`, requestID, step)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func (r *Repository) Decide(ctx context.Context, requestID, actorID int64, decision, note string) (DecisionResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DecisionResult{}, err
	}
	defer tx.Rollback(ctx)
	var req Request
	err = tx.QueryRow(ctx, `SELECT id,policy_id,module,document_id,company_id,amount,requester_id,current_step,status,submitted_at FROM approval_requests WHERE id=$1 FOR UPDATE`, requestID).Scan(&req.ID, &req.PolicyID, &req.Module, &req.DocumentID, &req.CompanyID, &req.Amount, &req.RequesterID, &req.CurrentStep, &req.Status, &req.SubmittedAt)
	if err != nil {
		return DecisionResult{}, err
	}
	if req.Status != StatusPending {
		return DecisionResult{}, ErrInvalid
	}
	var assignmentID, stepID int64
	err = tx.QueryRow(ctx, `SELECT id,policy_step_id FROM approval_assignments WHERE request_id=$1 AND step_order=$2 AND approver_id=$3 AND status='PENDING' FOR UPDATE`, requestID, req.CurrentStep, actorID).Scan(&assignmentID, &stepID)
	if errors.Is(err, pgx.ErrNoRows) {
		return DecisionResult{}, ErrNotAssigned
	}
	if err != nil {
		return DecisionResult{}, err
	}
	status := StatusApproved
	if decision == DecisionReject {
		status = StatusRejected
	}
	if _, err = tx.Exec(ctx, `UPDATE approval_assignments SET status=$2,updated_at=NOW() WHERE id=$1`, assignmentID, status); err != nil {
		return DecisionResult{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO approval_decisions(request_id,assignment_id,actor_id,decision,note) VALUES($1,$2,$3,$4,$5)`, requestID, assignmentID, actorID, decision, note); err != nil {
		return DecisionResult{}, err
	}
	result := DecisionResult{Request: req}
	if decision == DecisionReject {
		_, err = tx.Exec(ctx, `UPDATE approval_assignments SET status='CANCELLED',updated_at=NOW() WHERE request_id=$1 AND status='PENDING'`, requestID)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE approval_requests SET status='REJECTED',completed_at=NOW(),updated_at=NOW() WHERE id=$1`, requestID)
		}
		req.Status = StatusRejected
		result.Request = req
		result.Finalized = true
	} else {
		var required, approved int
		err = tx.QueryRow(ctx, `SELECT s.required_approvals,COUNT(a.id) FILTER(WHERE a.status='APPROVED') FROM approval_policy_steps s JOIN approval_assignments a ON a.policy_step_id=s.id WHERE s.id=$1 GROUP BY s.required_approvals`, stepID).Scan(&required, &approved)
		if err != nil {
			return DecisionResult{}, err
		}
		if approved >= required {
			_, err = tx.Exec(ctx, `UPDATE approval_assignments SET status='CANCELLED',updated_at=NOW() WHERE request_id=$1 AND step_order=$2 AND status='PENDING'`, requestID, req.CurrentStep)
			if err != nil {
				return DecisionResult{}, err
			}
			var next int
			err = tx.QueryRow(ctx, `SELECT step_order FROM approval_policy_steps WHERE policy_id=$1 AND step_order>$2 ORDER BY step_order LIMIT 1`, req.PolicyID, req.CurrentStep).Scan(&next)
			if errors.Is(err, pgx.ErrNoRows) {
				_, err = tx.Exec(ctx, `UPDATE approval_requests SET status='APPROVED',completed_at=NOW(),updated_at=NOW() WHERE id=$1`, requestID)
				req.Status = StatusApproved
				result.Finalized = true
			} else if err == nil {
				_, err = tx.Exec(ctx, `UPDATE approval_requests SET current_step=$2,updated_at=NOW() WHERE id=$1`, requestID, next)
				req.CurrentStep = next
				result.Advanced = true
			} else {
				return DecisionResult{}, err
			}
		}
	}
	if err != nil {
		return DecisionResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return DecisionResult{}, err
	}
	if result.Advanced {
		result.AssignedTo = r.assigneesForStep(nil, result.Request.CurrentStep, ctx, requestID)
	}
	return result, nil
}

func (r *Repository) ListInbox(ctx context.Context, userID int64) ([]Assignment, error) {
	rows, err := r.pool.Query(ctx, `SELECT a.id,a.request_id,a.policy_step_id,a.approver_id,a.delegated_from,a.step_order,a.status,r.module,p.name,s.name,r.document_id,r.amount,a.due_at,a.created_at FROM approval_assignments a JOIN approval_requests r ON r.id=a.request_id JOIN approval_policies p ON p.id=r.policy_id JOIN approval_policy_steps s ON s.id=a.policy_step_id WHERE a.approver_id=$1 AND a.status='PENDING' AND r.status='PENDING' AND a.step_order=r.current_step ORDER BY a.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Assignment
	for rows.Next() {
		var a Assignment
		if err := rows.Scan(&a.ID, &a.RequestID, &a.StepID, &a.ApproverID, &a.DelegatedFrom, &a.StepOrder, &a.Status, &a.Module, &a.PolicyName, &a.StepName, &a.DocumentID, &a.Amount, &a.DueAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

func (r *Repository) ListPolicies(ctx context.Context) ([]Policy, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,name,module,company_id,min_amount,max_amount,is_active FROM approval_policies ORDER BY module,min_amount,name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Policy
	for rows.Next() {
		var p Policy
		if err := rows.Scan(&p.ID, &p.Name, &p.Module, &p.CompanyID, &p.MinAmount, &p.MaxAmount, &p.Active); err != nil {
			return nil, err
		}
		p.Steps, _ = r.loadSteps(ctx, r.pool, p.ID)
		items = append(items, p)
	}
	return items, rows.Err()
}

func (r *Repository) CreatePolicy(ctx context.Context, in CreatePolicyInput) (Policy, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Policy{}, err
	}
	defer tx.Rollback(ctx)
	var p Policy
	err = tx.QueryRow(ctx, `INSERT INTO approval_policies(name,module,company_id,min_amount,max_amount,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,name,module,company_id,min_amount,max_amount,is_active`, in.Name, in.Module, in.CompanyID, in.MinAmount, in.MaxAmount, in.CreatedBy).Scan(&p.ID, &p.Name, &p.Module, &p.CompanyID, &p.MinAmount, &p.MaxAmount, &p.Active)
	if err != nil {
		return Policy{}, err
	}
	for i, s := range in.Steps {
		order := s.Order
		if order == 0 {
			order = i + 1
		}
		if s.RequiredApprovals == 0 {
			s.RequiredApprovals = 1
		}
		err = tx.QueryRow(ctx, `INSERT INTO approval_policy_steps(policy_id,step_order,name,approver_user_id,approver_role_id,required_approvals,escalation_hours) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, p.ID, order, s.Name, s.ApproverUserID, s.ApproverRoleID, s.RequiredApprovals, s.EscalationHours).Scan(&s.ID)
		if err != nil {
			return Policy{}, err
		}
		s.PolicyID = p.ID
		s.Order = order
		p.Steps = append(p.Steps, s)
	}
	if err = tx.Commit(ctx); err != nil {
		return Policy{}, err
	}
	return p, nil
}

func (r *Repository) EscalateOverdue(ctx context.Context) ([]Assignment, error) {
	rows, err := r.pool.Query(ctx, `UPDATE approval_assignments a SET due_at=NOW()+INTERVAL '24 hours',updated_at=NOW() FROM approval_requests r WHERE r.id=a.request_id AND r.status='PENDING' AND a.status='PENDING' AND a.step_order=r.current_step AND a.due_at<NOW() RETURNING a.id,a.request_id,a.approver_id,r.module,r.document_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Assignment
	for rows.Next() {
		var a Assignment
		if err := rows.Scan(&a.ID, &a.RequestID, &a.ApproverID, &a.Module, &a.DocumentID); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repository) CreateDelegation(ctx context.Context, in DelegationInput) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO approval_delegations(delegator_id,delegate_id,module,starts_at,ends_at) VALUES($1,$2,NULLIF($3,''),$4,$5)`, in.DelegatorID, in.DelegateID, in.Module, in.StartsAt, in.EndsAt)
	return err
}

func (r *Repository) FindPendingRequest(ctx context.Context, module string, documentID int64) (Request, error) {
	var q Request
	err := r.pool.QueryRow(ctx, `SELECT id,policy_id,module,document_id,company_id,amount,requester_id,current_step,status,submitted_at FROM approval_requests WHERE module=$1 AND document_id=$2 AND status='PENDING'`, module, documentID).Scan(&q.ID, &q.PolicyID, &q.Module, &q.DocumentID, &q.CompanyID, &q.Amount, &q.RequesterID, &q.CurrentStep, &q.Status, &q.SubmittedAt)
	return q, err
}
