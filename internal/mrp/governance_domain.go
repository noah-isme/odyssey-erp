package mrp

import (
	"time"

	"github.com/google/uuid"
)

// DecisionRequest represents a request to make a governed manufacturing decision
type DecisionRequest struct {
	CompanyID   int64
	RecordType  string // BOM, WorkOrder, Operation, etc.
	RecordID    int64
	Action      string // Approve, Release, Complete, etc.
	ActorID     int64
	Reason      string
	ChallengeID string // UUID of one-time signature challenge
	ReauthToken string // Current password or TOTP code; never persisted
	Evidence    map[string]interface{}
}

// DecisionGrant represents approval from the compliance gate
type DecisionGrant struct {
	PolicyVersionID int64
	RecordVersion   string
	RecordHash      string // SHA-256 of canonical snapshot
	DecisionID      uuid.UUID
	GrantedAt       time.Time
}

// PolicyVersion represents a versioned, effective-dated governance rule
type PolicyVersion struct {
	ID                  int64
	CompanyID           int64
	RecordType          string
	DecisionName        string
	EffectiveFrom       time.Time
	EffectiveTo         *time.Time
	EnforcementMode     string // DISABLED, WARN, ENFORCE
	SignatureRequired   bool
	ApproverRoles       []string
	SeparationOfDuties  *bool
	RequiredEvidence    []string
	RetentionPeriodDays *int
	Version             int
	Status              string // DRAFT, ACTIVE, RETIRED
	CreatedAt           time.Time
	CreatedBy           int64
}

// ComplianceDecision represents a decision made through the governance gate
type ComplianceDecision struct {
	ID              int64
	CompanyID       int64
	PolicyVersionID int64
	SnapshotID      *int64
	RecordType      string
	RecordID        int64
	Action          string
	ActorID         int64
	Reason          *string
	DecisionID      uuid.UUID
	RecordVersion   *string
	RecordHash      *string // SHA-256 of canonical snapshot
	CreatedAt       time.Time
}

// SignatureChallenge represents a one-time challenge for reauthentication
type SignatureChallenge struct {
	ID                       int64
	ChallengeID              uuid.UUID
	PolicyVersionID          int64
	CompanyID                int64
	RecordType               string
	SignerID                 int64
	RecordID                 int64
	RecordVersion            *string
	RecordHash               *string
	Expiry                   time.Time
	ReauthenticationRequired bool
	Used                     bool
	ReauthenticationMethod   *string
	ReauthenticatedAt        *time.Time
	CreatedAt                time.Time
}

// EvidenceRecord represents immutable proof linked to a decision
type EvidenceRecord struct {
	ID           int64
	DecisionID   int64
	EvidenceType *string
	Content      map[string]interface{} // JSONB data
	CreatedAt    time.Time
}

// AuditEvent represents an immutable audit trail entry
type AuditEvent struct {
	ID            int64
	CompanyID     int64
	CorrelationID uuid.UUID  // Groups related decisions
	CausationID   *uuid.UUID // Links cause → effect
	DecisionID    *int64
	EntityType    *string
	EntityID      *int64
	Action        *string
	ActorID       *int64
	Details       map[string]interface{} // JSONB data
	CreatedAt     time.Time
}

// QualityInspection represents an inspection record
type QualityInspection struct {
	ID               int64
	CompanyID        int64
	ProductID        int64
	WorkOrderID      *int64
	OperationID      *int64
	InspectionPlanID *int64
	Status           string // PENDING, PASSED, FAILED, HOLD, RELEASED
	ResultSnapshot   map[string]interface{}
	ResultVersion    *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// QualityHold represents a quality hold
type QualityHold struct {
	ID           int64
	CompanyID    int64
	InspectionID *int64
	RecordType   *string // work_order, operation, etc.
	RecordID     *int64
	Status       string // OPEN, RELEASED
	CreatedBy    int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// QualityNCR represents a nonconformance record
type QualityNCR struct {
	ID        int64
	CompanyID int64
	Number    string
	Status    string // OPEN, INVESTIGATING, DISPOSITIONED, CLOSED
	CreatedBy int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// QualityCAPA represents a corrective/preventive action
type QualityCAPA struct {
	ID        int64
	CompanyID int64
	Number    string
	Status    string // OPEN, IN_PROGRESS, VERIFICATION, CLOSED
	CreatedBy int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SubcontractReceipt represents received subcontract goods
type SubcontractReceipt struct {
	ID          int64
	CompanyID   int64
	WorkOrderID int64
	OperationID int64
	Status      string // SENT, RECEIVED, INSPECTING, ACCEPTED, CLOSED
	SentQty     float64
	ReceivedQty *float64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// EnforcementModes
const (
	EnforcementModeDisabled = "DISABLED"
	EnforcementModeWarn     = "WARN"
	EnforcementModeEnforce  = "ENFORCE"
)

// PolicyStatus
const (
	PolicyStatusDraft   = "DRAFT"
	PolicyStatusActive  = "ACTIVE"
	PolicyStatusRetired = "RETIRED"
)

// Quality Record Statuses
const (
	InspectionStatusPending  = "PENDING"
	InspectionStatusPassed   = "PASSED"
	InspectionStatusFailed   = "FAILED"
	InspectionStatusHold     = "HOLD"
	InspectionStatusReleased = "RELEASED"

	HoldStatusOpen     = "OPEN"
	HoldStatusReleased = "RELEASED"

	NCRStatusOpen          = "OPEN"
	NCRStatusInvestigating = "INVESTIGATING"
	NCRStatusDispositioned = "DISPOSITIONED"
	NCRStatusClosed        = "CLOSED"

	CAPAStatusOpen         = "OPEN"
	CAPAStatusInProgress   = "IN_PROGRESS"
	CAPAStatusVerification = "VERIFICATION"
	CAPAStatusClosed       = "CLOSED"

	SubcontractStatusSent       = "SENT"
	SubcontractStatusReceived   = "RECEIVED"
	SubcontractStatusInspecting = "INSPECTING"
	SubcontractStatusAccepted   = "ACCEPTED"
	SubcontractStatusClosed     = "CLOSED"
)
