package approvals

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
