package approvals

import "time"

const (
	StatusPending   = "PENDING"
	StatusApproved  = "APPROVED"
	StatusRejected  = "REJECTED"
	DecisionApprove = "APPROVE"
	DecisionReject  = "REJECT"
)

type Policy struct {
	ID           int64
	Name, Module string
	CompanyID    *int64
	MinAmount    float64
	MaxAmount    *float64
	Active       bool
	Steps        []PolicyStep
}

type PolicyStep struct {
	ID, PolicyID                   int64
	Order                          int
	Name                           string
	ApproverUserID, ApproverRoleID *int64
	ApproverManager                bool
	RequiredApprovals              int
	EscalationHours                *int
}

type Submission struct {
	Module                  string
	DocumentID, RequesterID int64
	CompanyID               *int64
	Amount                  float64
	ManagerID               int64
}

type Request struct {
	ID, PolicyID, DocumentID, RequesterID int64
	Module, Status                        string
	CompanyID                             *int64
	Amount                                float64
	CurrentStep                           int
	SubmittedAt                           time.Time
}

type Assignment struct {
	ID, RequestID, StepID, ApproverID    int64
	DelegatedFrom                        *int64
	StepOrder                            int
	Status, Module, PolicyName, StepName string
	DocumentID                           int64
	Amount                               float64
	DueAt                                *time.Time
	CreatedAt                            time.Time
}

type DecisionResult struct {
	Request             Request
	Finalized, Advanced bool
	AssignedTo          []int64
}

type CreatePolicyInput struct {
	Name, Module string
	CompanyID    *int64
	MinAmount    float64
	MaxAmount    *float64
	CreatedBy    int64
	Steps        []PolicyStep
}

type DelegationInput struct {
	DelegatorID, DelegateID int64
	Module                  string
	StartsAt, EndsAt        time.Time
}
