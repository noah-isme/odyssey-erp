package approvals

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
)

type memoryStore struct {
	policies  []Policy
	requests  map[int64]Request
	assignees map[int64][]int64
	delegates map[int64]int64
	next      int64
}

func newMemoryStore(policies ...Policy) *memoryStore {
	return &memoryStore{policies: policies, requests: map[int64]Request{}, assignees: map[int64][]int64{}, delegates: map[int64]int64{}}
}
func (m *memoryStore) ResolvePolicy(_ context.Context, module string, company *int64, amount float64) (Policy, error) {
	var best *Policy
	for i := range m.policies {
		p := &m.policies[i]
		if p.Module != module || amount < p.MinAmount || (p.MaxAmount != nil && amount > *p.MaxAmount) {
			continue
		}
		if p.CompanyID != nil && (company == nil || *p.CompanyID != *company) {
			continue
		}
		if best == nil || (p.CompanyID != nil && best.CompanyID == nil) || p.MinAmount > best.MinAmount {
			best = p
		}
	}
	if best == nil {
		return Policy{}, ErrNoPolicy
	}
	return *best, nil
}
func (m *memoryStore) CreateRequest(_ context.Context, p Policy, in Submission) (Request, []int64, error) {
	m.next++
	r := Request{ID: m.next, PolicyID: p.ID, Module: in.Module, DocumentID: in.DocumentID, RequesterID: in.RequesterID, Amount: in.Amount, CurrentStep: 1, Status: StatusPending}
	m.requests[r.ID] = r
	for _, s := range p.Steps {
		if s.Order != 1 {
			continue
		}
		id := in.ManagerID
		if !s.ApproverManager && s.ApproverUserID != nil {
			id = *s.ApproverUserID
		}
		if id == 0 {
			continue
		}
		if d := m.delegates[id]; d > 0 {
			id = d
		}
		m.assignees[r.ID] = append(m.assignees[r.ID], id)
	}
	return r, m.assignees[r.ID], nil
}

type workflowNotifier struct{ assigned, completed int }

func (n *workflowNotifier) Assigned(context.Context, int64, Request) error  { n.assigned++; return nil }
func (n *workflowNotifier) Escalated(context.Context, int64, Request) error { return nil }
func (n *workflowNotifier) Completed(context.Context, int64, Request, string) error {
	n.completed++
	return nil
}

type leaveWorkflowFinalizer struct {
	pending, used float64
	audit         bool
}

func (f *leaveWorkflowFinalizer) FinalizeApproval(_ context.Context, _ Request, status string, _ int64, _ string) error {
	if status == StatusApproved {
		f.used += f.pending
	}
	f.pending = 0
	f.audit = true
	return nil
}
func (m *memoryStore) Decide(_ context.Context, id, actor int64, decision, note string) (DecisionResult, error) {
	r := m.requests[id]
	allowed := false
	for _, u := range m.assignees[id] {
		allowed = allowed || u == actor
	}
	if !allowed {
		return DecisionResult{}, ErrNotAssigned
	}
	r.Status = StatusApproved
	if decision == DecisionReject {
		r.Status = StatusRejected
	}
	m.requests[id] = r
	return DecisionResult{Request: r, Finalized: true}, nil
}
func (m *memoryStore) ListInbox(context.Context, int64) ([]Assignment, error) { return nil, nil }
func (m *memoryStore) ListPolicies(context.Context) ([]Policy, error)         { return m.policies, nil }
func (m *memoryStore) CreatePolicy(_ context.Context, in CreatePolicyInput) (Policy, error) {
	return Policy{Name: in.Name, Module: in.Module, Steps: in.Steps}, nil
}
func (m *memoryStore) EscalateOverdue(context.Context) ([]Assignment, error) { return nil, nil }
func (m *memoryStore) CreateDelegation(_ context.Context, in DelegationInput) error {
	m.delegates[in.DelegatorID] = in.DelegateID
	return nil
}
func (m *memoryStore) FindPendingRequest(_ context.Context, module string, doc int64) (Request, error) {
	for _, r := range m.requests {
		if r.Module == module && r.DocumentID == doc && r.Status == StatusPending {
			return r, nil
		}
	}
	return Request{}, ErrInvalid
}

func ptr[T any](v T) *T { return &v }

func TestPOPolicyResolvesDifferentApproversByAmountAndDelegation(t *testing.T) {
	store := newMemoryStore(
		Policy{ID: 1, Name: "PO standard", Module: "PO", MinAmount: 0, MaxAmount: ptr(9999.99), Steps: []PolicyStep{{Order: 1, ApproverUserID: ptr(int64(10))}}},
		Policy{ID: 2, Name: "PO large", Module: "PO", MinAmount: 10000, Steps: []PolicyStep{{Order: 1, ApproverUserID: ptr(int64(20))}, {Order: 2, ApproverUserID: ptr(int64(30))}}},
	)
	service := NewService(store, nil)
	require.NoError(t, service.Delegate(context.Background(), DelegationInput{DelegatorID: 20, DelegateID: 21, StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(time.Hour)}))
	low, err := service.Submit(context.Background(), Submission{Module: "PO", DocumentID: 100, RequesterID: 1, Amount: 5000})
	require.NoError(t, err)
	require.Equal(t, int64(1), low.PolicyID)
	require.Equal(t, []int64{10}, store.assignees[low.ID])
	high, err := service.Submit(context.Background(), Submission{Module: "PO", DocumentID: 101, RequesterID: 1, Amount: 25000})
	require.NoError(t, err)
	require.Equal(t, int64(2), high.PolicyID)
	require.Equal(t, []int64{21}, store.assignees[high.ID])
}

func TestLeaveManagerApprovalUpdatesBalanceAuditAndNotifications(t *testing.T) {
	store := newMemoryStore(Policy{ID: 3, Name: "Manager leave", Module: "LEAVE", Steps: []PolicyStep{{Order: 1, ApproverManager: true}}})
	notifier := &workflowNotifier{}
	finalizer := &leaveWorkflowFinalizer{pending: 2}
	service := NewService(store, notifier)
	service.RegisterFinalizer("LEAVE", finalizer)
	request, err := service.Submit(context.Background(), Submission{Module: "LEAVE", DocumentID: 77, RequesterID: 40, ManagerID: 50, Amount: 2})
	require.NoError(t, err)
	require.Equal(t, []int64{50}, store.assignees[request.ID])
	result, err := service.Decide(context.Background(), request.ID, 50, DecisionApprove, "approved")
	require.NoError(t, err)
	require.True(t, result.Finalized)
	require.Equal(t, float64(0), finalizer.pending)
	require.Equal(t, float64(2), finalizer.used)
	require.True(t, finalizer.audit)
	require.Equal(t, 1, notifier.assigned)
	require.Equal(t, 1, notifier.completed)
}

func TestPolicyResolutionPrefersMatchingCompany(t *testing.T) {
	companyID := int64(7)
	store := newMemoryStore(
		Policy{ID: 1, Name: "Global", Module: "PO", MinAmount: 0, Steps: []PolicyStep{{Order: 1, ApproverUserID: ptr(int64(10))}}},
		Policy{ID: 2, Name: "Company 7", Module: "PO", CompanyID: &companyID, MinAmount: 0, Steps: []PolicyStep{{Order: 1, ApproverUserID: ptr(int64(20))}}},
	)
	request, err := NewService(store, nil).Submit(context.Background(), Submission{Module: "PO", DocumentID: 8, RequesterID: 1, CompanyID: &companyID, Amount: 100})
	require.NoError(t, err)
	require.Equal(t, int64(2), request.PolicyID)
	require.Equal(t, []int64{20}, store.assignees[request.ID])
}

type phase3ApprovalStore struct {
	policies    []Policy
	requests    map[int64]Request
	assignments map[int64]map[int][]int64
	delegates   map[int64]int64
	next        int64
}

func newPhase3ApprovalStore(policies ...Policy) *phase3ApprovalStore {
	return &phase3ApprovalStore{
		policies: policies, requests: map[int64]Request{},
		assignments: map[int64]map[int][]int64{}, delegates: map[int64]int64{},
	}
}

func (s *phase3ApprovalStore) ResolvePolicy(_ context.Context, module string, company *int64, amount float64) (Policy, error) {
	var selected *Policy
	for i := range s.policies {
		p := &s.policies[i]
		if p.Module != module || amount < p.MinAmount || (p.MaxAmount != nil && amount > *p.MaxAmount) || (p.CompanyID != nil && (company == nil || *p.CompanyID != *company)) {
			continue
		}
		if selected == nil || (p.CompanyID != nil && selected.CompanyID == nil) || p.MinAmount > selected.MinAmount {
			selected = p
		}
	}
	if selected == nil {
		return Policy{}, ErrNoPolicy
	}
	return *selected, nil
}

func (s *phase3ApprovalStore) CreateRequest(_ context.Context, p Policy, in Submission) (Request, []int64, error) {
	s.next++
	r := Request{ID: s.next, PolicyID: p.ID, DocumentID: in.DocumentID, RequesterID: in.RequesterID, Module: in.Module, CompanyID: in.CompanyID, Amount: in.Amount, CurrentStep: 1, Status: StatusPending}
	s.requests[r.ID] = r
	s.assignments[r.ID] = map[int][]int64{}
	for _, step := range p.Steps {
		var approver int64
		if step.ApproverManager {
			approver = in.ManagerID
		} else if step.ApproverUserID != nil {
			approver = *step.ApproverUserID
		} else if step.ApproverRoleID != nil {
			approver = *step.ApproverRoleID
		}
		if delegated, ok := s.delegates[approver]; ok {
			approver = delegated
		}
		if approver > 0 {
			s.assignments[r.ID][step.Order] = []int64{approver}
		}
	}
	return r, append([]int64(nil), s.assignments[r.ID][1]...), nil
}

func (s *phase3ApprovalStore) Decide(_ context.Context, requestID, actorID int64, decision, _ string) (DecisionResult, error) {
	r := s.requests[requestID]
	allowed := false
	for _, id := range s.assignments[requestID][r.CurrentStep] {
		allowed = allowed || id == actorID
	}
	if !allowed {
		return DecisionResult{}, ErrNotAssigned
	}
	if decision == DecisionReject {
		r.Status = StatusRejected
		s.requests[requestID] = r
		return DecisionResult{Request: r, Finalized: true}, nil
	}
	for _, step := range s.policies[r.PolicyID-1].Steps {
		if step.Order > r.CurrentStep {
			r.CurrentStep = step.Order
			s.requests[requestID] = r
			return DecisionResult{Request: r, Advanced: true, AssignedTo: append([]int64(nil), s.assignments[requestID][r.CurrentStep]...)}, nil
		}
	}
	r.Status = StatusApproved
	s.requests[requestID] = r
	return DecisionResult{Request: r, Finalized: true}, nil
}

func (s *phase3ApprovalStore) ListInbox(context.Context, int64) ([]Assignment, error) {
	return nil, nil
}
func (s *phase3ApprovalStore) ListPolicies(context.Context) ([]Policy, error) { return s.policies, nil }
func (s *phase3ApprovalStore) CreatePolicy(context.Context, CreatePolicyInput) (Policy, error) {
	return Policy{}, nil
}
func (s *phase3ApprovalStore) EscalateOverdue(_ context.Context) ([]Assignment, error) {
	return []Assignment{{RequestID: 1, ApproverID: 30, Module: "PO", DocumentID: 99}}, nil
}
func (s *phase3ApprovalStore) CreateDelegation(_ context.Context, in DelegationInput) error {
	s.delegates[in.DelegatorID] = in.DelegateID
	return nil
}
func (s *phase3ApprovalStore) FindPendingRequest(_ context.Context, module string, documentID int64) (Request, error) {
	for _, r := range s.requests {
		if r.Module == module && r.DocumentID == documentID && r.Status == StatusPending {
			return r, nil
		}
	}
	return Request{}, ErrInvalid
}

type phase3Notifier struct {
	assigned, escalated, completed int
	completedStatus                string
}

func (n *phase3Notifier) Assigned(context.Context, int64, Request) error  { n.assigned++; return nil }
func (n *phase3Notifier) Escalated(context.Context, int64, Request) error { n.escalated++; return nil }
func (n *phase3Notifier) Completed(_ context.Context, _ int64, _ Request, status string) error {
	n.completed++
	n.completedStatus = status
	return nil
}

type phase3Finalizer struct {
	calls  int
	status string
	actor  int64
}

func (f *phase3Finalizer) FinalizeApproval(_ context.Context, _ Request, status string, actorID int64, _ string) error {
	f.calls++
	f.status = status
	f.actor = actorID
	return nil
}

func TestApprovalWorkflowSupportsMultiStepDecisionsAndFinalizers(t *testing.T) {
	first, second := int64(11), int64(22)
	store := newPhase3ApprovalStore(Policy{ID: 1, Module: "PO", Steps: []PolicyStep{
		{Order: 1, ApproverUserID: &first}, {Order: 2, ApproverUserID: &second},
	}})
	notifier := &phase3Notifier{}
	finalizer := &phase3Finalizer{}
	service := NewService(store, notifier)
	service.RegisterFinalizer("PO", finalizer)

	req, err := service.Submit(context.Background(), Submission{Module: "po", DocumentID: 99, RequesterID: 7})
	require.NoError(t, err)
	require.Equal(t, 1, notifier.assigned)
	_, err = service.Decide(context.Background(), req.ID, 999, DecisionApprove, "")
	require.ErrorIs(t, err, ErrNotAssigned)
	advanced, err := service.Decide(context.Background(), req.ID, first, DecisionApprove, "step one")
	require.NoError(t, err)
	require.True(t, advanced.Advanced)
	require.Equal(t, 2, advanced.Request.CurrentStep)
	require.Equal(t, 2, notifier.assigned)
	completed, err := service.Decide(context.Background(), req.ID, second, DecisionApprove, "final")
	require.NoError(t, err)
	require.True(t, completed.Finalized)
	require.Equal(t, StatusApproved, finalizer.status)
	require.Equal(t, int64(22), finalizer.actor)
	require.Equal(t, 1, finalizer.calls)
	require.Equal(t, 1, notifier.completed)
}

func TestApprovalWorkflowRejectsAndEscalates(t *testing.T) {
	approver := int64(10)
	store := newPhase3ApprovalStore(Policy{ID: 1, Module: "LEAVE", Steps: []PolicyStep{{Order: 1, ApproverUserID: &approver}}})
	notifier := &phase3Notifier{}
	finalizer := &phase3Finalizer{}
	service := NewService(store, notifier)
	service.RegisterFinalizer("LEAVE", finalizer)
	req, err := service.Submit(context.Background(), Submission{Module: "LEAVE", DocumentID: 1, RequesterID: 7})
	require.NoError(t, err)
	result, err := service.Decide(context.Background(), req.ID, approver, DecisionReject, "not eligible")
	require.NoError(t, err)
	require.Equal(t, StatusRejected, result.Request.Status)
	require.Equal(t, StatusRejected, finalizer.status)
	count, err := service.Escalate(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, 1, notifier.escalated)
}

func TestApprovalDelegationValidatesActorsAndDates(t *testing.T) {
	service := NewService(newPhase3ApprovalStore(), nil)
	now := time.Now()
	for _, input := range []DelegationInput{
		{DelegatorID: 0, DelegateID: 2, StartsAt: now, EndsAt: now.Add(time.Hour)},
		{DelegatorID: 1, DelegateID: 1, StartsAt: now, EndsAt: now.Add(time.Hour)},
		{DelegatorID: 1, DelegateID: 2, StartsAt: now.Add(time.Hour), EndsAt: now},
	} {
		require.ErrorIs(t, service.Delegate(context.Background(), input), ErrInvalid)
	}
}

func TestPolicyResolutionCoversModuleAmountCompanyAndApproverKinds(t *testing.T) {
	companyID := int64(9)
	roleID, userID := int64(41), int64(42)
	store := newPhase3ApprovalStore(
		Policy{ID: 1, Module: "PO", MinAmount: 0, MaxAmount: ptr(999.0), Steps: []PolicyStep{{Order: 1, ApproverRoleID: &roleID}}},
		Policy{ID: 2, Module: "PO", CompanyID: &companyID, MinAmount: 1000, Steps: []PolicyStep{{Order: 1, ApproverUserID: &userID}}},
	)
	service := NewService(store, nil)
	roleRequest, err := service.Submit(context.Background(), Submission{Module: "po", DocumentID: 1, RequesterID: 7, Amount: 500})
	require.NoError(t, err)
	require.Equal(t, int64(1), roleRequest.PolicyID)
	companyRequest, err := service.Submit(context.Background(), Submission{Module: "PO", DocumentID: 2, RequesterID: 7, CompanyID: &companyID, Amount: 1500})
	require.NoError(t, err)
	require.Equal(t, int64(2), companyRequest.PolicyID)
	_, err = service.Submit(context.Background(), Submission{Module: "AP", DocumentID: 3, RequesterID: 7, Amount: 500})
	require.ErrorIs(t, err, ErrNoPolicy)

	manager := int64(77)
	managerStore := newPhase3ApprovalStore(Policy{ID: 3, Module: "LEAVE", Steps: []PolicyStep{{Order: 1, ApproverManager: true}}})
	managerRequest, err := NewService(managerStore, nil).Submit(context.Background(), Submission{Module: "LEAVE", DocumentID: 4, RequesterID: 7, ManagerID: manager})
	require.NoError(t, err)
	require.Equal(t, []int64{manager}, managerStore.assignments[managerRequest.ID][1])
}

func TestApprovalServiceEmitsPhase2NotificationsAcrossWorkflow(t *testing.T) {
	store := newPhase3ApprovalStore(Policy{ID: 1, Module: "PO", Steps: []PolicyStep{{Order: 1, ApproverUserID: ptr(int64(10))}}})
	notificationStore := &notificationsTestStore{memoryStore: &notificationsMemoryStore{}}
	dispatcher := notifications.NewDispatcher(notifications.NewService(notificationStore.memoryStore), notificationsPreferenceFake{channels: notifications.Channels{InApp: true}}, nil)
	service := NewService(store, NewNotificationAdapter(dispatcher))
	req, err := service.Submit(context.Background(), Submission{Module: "PO", DocumentID: 88, RequesterID: 7})
	require.NoError(t, err)
	_, err = service.Decide(context.Background(), req.ID, 10, DecisionApprove, "approved")
	require.NoError(t, err)
	require.Len(t, notificationStore.memoryStore.items, 2)
	require.Equal(t, notifications.TypeApprovalAssigned, notificationStore.memoryStore.items[0].Type)
	require.Equal(t, notifications.TypeApprovalApproved, notificationStore.memoryStore.items[1].Type)
	require.Equal(t, int64(7), notificationStore.memoryStore.items[1].RecipientID)
}
