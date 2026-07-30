package approvals

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrNoPolicy    = errors.New("approvals: no matching policy")
	ErrNotAssigned = errors.New("approvals: user is not assigned to current step")
	ErrInvalid     = errors.New("approvals: invalid input")
)

type Store interface {
	ResolvePolicy(context.Context, string, *int64, float64) (Policy, error)
	CreateRequest(context.Context, Policy, Submission) (Request, []int64, error)
	Decide(context.Context, int64, int64, string, string) (DecisionResult, error)
	ListInbox(context.Context, int64) ([]Assignment, error)
	ListPolicies(context.Context) ([]Policy, error)
	CreatePolicy(context.Context, CreatePolicyInput) (Policy, error)
	EscalateOverdue(context.Context) ([]Assignment, error)
	CreateDelegation(context.Context, DelegationInput) error
	FindPendingRequest(context.Context, string, int64) (Request, error)
}

type Notifier interface {
	Assigned(context.Context, int64, Request) error
	Escalated(context.Context, int64, Request) error
	Completed(context.Context, int64, Request, string) error
}
type Finalizer interface {
	FinalizeApproval(context.Context, Request, string, int64, string) error
}

type Service struct {
	store      Store
	notifier   Notifier
	finalizers map[string]Finalizer
}

func NewService(store Store, notifier Notifier) *Service {
	return &Service{store: store, notifier: notifier, finalizers: map[string]Finalizer{}}
}
func (s *Service) RegisterFinalizer(module string, f Finalizer) {
	if f != nil {
		s.finalizers[strings.ToUpper(strings.TrimSpace(module))] = f
	}
}

func (s *Service) Submit(ctx context.Context, in Submission) (Request, error) {
	in.Module = strings.ToUpper(strings.TrimSpace(in.Module))
	if in.Module == "" || in.DocumentID <= 0 || in.RequesterID <= 0 || in.Amount < 0 {
		return Request{}, ErrInvalid
	}
	policy, err := s.store.ResolvePolicy(ctx, in.Module, in.CompanyID, in.Amount)
	if err != nil {
		return Request{}, err
	}
	request, assignees, err := s.store.CreateRequest(ctx, policy, in)
	if err != nil {
		return Request{}, err
	}
	for _, userID := range assignees {
		if s.notifier != nil {
			_ = s.notifier.Assigned(ctx, userID, request)
		}
	}
	return request, nil
}

func (s *Service) Decide(ctx context.Context, requestID, actorID int64, decision, note string) (DecisionResult, error) {
	decision = strings.ToUpper(strings.TrimSpace(decision))
	if requestID <= 0 || actorID <= 0 || (decision != DecisionApprove && decision != DecisionReject) {
		return DecisionResult{}, ErrInvalid
	}
	result, err := s.store.Decide(ctx, requestID, actorID, decision, strings.TrimSpace(note))
	if err != nil {
		return DecisionResult{}, err
	}
	for _, userID := range result.AssignedTo {
		if s.notifier != nil {
			_ = s.notifier.Assigned(ctx, userID, result.Request)
		}
	}
	if result.Finalized && s.notifier != nil {
		_ = s.notifier.Completed(ctx, result.Request.RequesterID, result.Request, result.Request.Status)
	}
	if result.Finalized {
		if f := s.finalizers[result.Request.Module]; f != nil {
			if err := f.FinalizeApproval(ctx, result.Request, result.Request.Status, actorID, note); err != nil {
				return DecisionResult{}, err
			}
		}
	}
	return result, nil
}
func (s *Service) DecideDocument(ctx context.Context, module string, documentID, actorID int64, decision, note string) (DecisionResult, error) {
	req, err := s.store.FindPendingRequest(ctx, strings.ToUpper(strings.TrimSpace(module)), documentID)
	if err != nil {
		return DecisionResult{}, err
	}
	return s.Decide(ctx, req.ID, actorID, decision, note)
}

func (s *Service) Inbox(ctx context.Context, userID int64) ([]Assignment, error) {
	if userID <= 0 {
		return nil, ErrInvalid
	}
	return s.store.ListInbox(ctx, userID)
}
func (s *Service) Policies(ctx context.Context) ([]Policy, error) { return s.store.ListPolicies(ctx) }
func (s *Service) CreatePolicy(ctx context.Context, in CreatePolicyInput) (Policy, error) {
	in.Name, in.Module = strings.TrimSpace(in.Name), strings.ToUpper(strings.TrimSpace(in.Module))
	if in.Name == "" || in.Module == "" || in.CreatedBy <= 0 || len(in.Steps) == 0 || in.MinAmount < 0 {
		return Policy{}, ErrInvalid
	}
	return s.store.CreatePolicy(ctx, in)
}
func (s *Service) Escalate(ctx context.Context) (int, error) {
	items, err := s.store.EscalateOverdue(ctx)
	if err != nil {
		return 0, err
	}
	for _, item := range items {
		if s.notifier != nil {
			_ = s.notifier.Escalated(ctx, item.ApproverID, Request{ID: item.RequestID, Module: item.Module, DocumentID: item.DocumentID, Status: StatusPending})
		}
	}
	return len(items), nil
}
func (s *Service) Delegate(ctx context.Context, in DelegationInput) error {
	if in.DelegatorID <= 0 || in.DelegateID <= 0 || in.DelegatorID == in.DelegateID || !in.EndsAt.After(in.StartsAt) {
		return ErrInvalid
	}
	in.Module = strings.ToUpper(strings.TrimSpace(in.Module))
	return s.store.CreateDelegation(ctx, in)
}
