package qms

import (
	"errors"
	"strings"
	"time"
)

// Status captures the state of a quality record.
type Status string

const (
	// NCR Statuses
	NCRStatusOpen           Status = "OPEN"
	NCRStatusUnderReview    Status = "UNDER_REVIEW"
	NCRStatusDispositioned  Status = "DISPOSITIONED"
	NCRStatusClosed         Status = "CLOSED"
	NCRStatusCancelled      Status = "CANCELLED"

	// CAPA Statuses (reuses OPEN, CLOSED from NCR — same string values are intentional)
	CAPAStatusOpen      = NCRStatusOpen
	CAPAStatusInProgress Status = "IN_PROGRESS"
	CAPAStatusVerifying  Status = "VERIFYING"
	CAPAStatusEffective  Status = "EFFECTIVE"
	CAPAStatusClosed     = NCRStatusClosed

	// Audit Statuses
	AuditStatusPlanned    Status = "PLANNED"
	AuditStatusInProgress = CAPAStatusInProgress
	AuditStatusCompleted  Status = "COMPLETED"
	AuditStatusReported   Status = "REPORTED"
	AuditStatusClosed     = NCRStatusClosed

	// Supplier Quality Statuses
	SupplierStatusApproved    Status = "APPROVED"
	SupplierStatusConditional Status = "CONDITIONAL"
	SupplierStatusRejected    Status = "REJECTED"
	SupplierStatusOnHold      Status = "ON_HOLD"
)

// NonConformanceReport represents a non-conformance record.
type NonConformanceReport struct {
	ID                  int64
	CompanyID           int64
	Number              string
	Title               string
	Description         string
	SourceType          string // INTERNAL, SUPPLIER, CUSTOMER, AUDIT, PRODUCTION
	SourceID            *int64
	SourceReference     string
	Category            string // MATERIAL, PROCESS, PRODUCT, DOCUMENTATION, SERVICE
	Severity            string // MINOR, MAJOR, CRITICAL
	Status              Status
	DetectedBy          int64
	DetectedAt          time.Time
	DetectedLocation    string
	ResponsiblePartyID  *int64
	AssignedTo          *int64
	TargetClosureDate   *time.Time
	ActualClosureDate   *time.Time
	RootCause           string
	ContainmentAction   string
	CreatedBy           int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// NCRDisposition represents the disposition decision for an NCR.
type NCRDisposition struct {
	ID              int64
	NCRID           int64
	DispositionType string // REWORK, REPAIR, USE_AS_IS, SCRAP, RETURN_TO_SUPPLIER
	Description     string
	ApprovedBy      int64
	ApprovedAt      time.Time
	CreatedAt       time.Time
}

// CorrectiveAction represents a CAPA record.
type CorrectiveAction struct {
	ID                  int64
	CompanyID           int64
	Number              string
	Title               string
	Description         string
	SourceType          string // NCR, AUDIT, CUSTOMER_COMPLAINT, REGULATORY, INTERNAL
	SourceID            *int64
	SourceReference     string
	Status              Status
	Priority            string // LOW, MEDIUM, HIGH, CRITICAL
	OwnerID             int64
	TeamMembers         []int64
	RootCause           string
	RootCauseMethod     string // FIVE_WHYS, FISHBONE, FAULT_TREE, PARETO
	CorrectiveAction    string
	PreventiveAction    string
	VerificationMethod  string
	VerificationResult  string
	EffectivenessCheck  string
	TargetDate          *time.Time
	CompletionDate      *time.Time
	EffectivenessDate   *time.Time
	CreatedBy           int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Audit represents a quality audit.
type Audit struct {
	ID              int64
	CompanyID       int64
	Number          string
	Title           string
	Description     string
	AuditType       string // INTERNAL, SUPPLIER, REGULATORY, CERTIFICATION, PROCESS, PRODUCT
	Status          Status
	Standard        string // ISO9001, ISO13485, IATF16949, AS9100, CUSTOM
	Scope           string
	LeadAuditorID   int64
	AuditTeamIDs    []int64
	AuditeeID       *int64
	PlannedStart    *time.Time
	PlannedEnd      *time.Time
	ActualStart     *time.Time
	ActualEnd       *time.Time
	ReportNumber    string
	ReportDate      *time.Time
	CreatedBy       int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// AuditFinding represents a finding from an audit.
type AuditFinding struct {
	ID              int64
	AuditID         int64
	FindingNumber   string
	Category        string // MAJOR, MINOR, OBSERVATION, OPPORTUNITY
	Clause          string
	Description     string
	Evidence        string
	Requirement     string
	RiskLevel       string // HIGH, MEDIUM, LOW
	Status          Status // OPEN, IN_PROGRESS, CLOSED
	Response        string
	ResponseDueDate *time.Time
	ResponseDate    *time.Time
	AssignedTo      *int64
	VerifiedBy      *int64
	VerifiedAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// SupplierQuality represents a supplier quality record.
type SupplierQuality struct {
	ID              int64
	CompanyID       int64
	SupplierID      int64
	SupplierName    string
	Status          Status
	QualityRating   float64 // 0-100
	RiskLevel       string // LOW, MEDIUM, HIGH, CRITICAL
	ApprovedDate    *time.Time
	ExpiryDate      *time.Time
	LastAuditDate   *time.Time
	NextAuditDate   *time.Time
	Notes           string
	CreatedBy       int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// SupplierAudit represents a supplier audit.
type SupplierAudit struct {
	ID              int64
	CompanyID       int64
	SupplierID      int64
	AuditNumber     string
	AuditType       string // INITIAL, SURVEILLANCE, REQUALIFICATION, FOR_CAUSE
	Status          Status
	Standard        string
	PlannedDate     *time.Time
	ActualDate      *time.Time
	Score           float64
	LeadAuditorID   int64
	ReportNumber    string
	CreatedBy       int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// QualityObjective represents a quality objective/KPI.
type QualityObjective struct {
	ID              int64
	CompanyID       int64
	Name            string
	Description     string
	MetricType      string // DPPM, FPY, COQ, OTD, CUSTOMER_COMPLAINTS
	TargetValue     float64
	CurrentValue    float64
	Unit            string
	Frequency       string // DAILY, WEEKLY, MONTHLY, QUARTERLY, ANNUAL
	OwnerID         int64
	Status          string // ACTIVE, INACTIVE, ACHIEVED
	StartDate       time.Time
	EndDate         *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// QualityObjectiveMeasurement represents a measurement for a quality objective.
type QualityObjectiveMeasurement struct {
	ID              int64
	ObjectiveID     int64
	Value           float64
	MeasurementDate time.Time
	Notes           string
	RecordedBy      int64
	CreatedAt       time.Time
}

// CreateNCRRequest defines the payload for creating an NCR.
type CreateNCRRequest struct {
	CompanyID          int64
	Title              string
	Description        string
	SourceType         string
	SourceID           *int64
	SourceReference    string
	Category           string
	Severity           string
	DetectedBy         int64
	DetectedLocation   string
	ResponsiblePartyID *int64
	AssignedTo         *int64
	TargetClosureDate  *time.Time
	ActorID            int64
}

// Validate ensures the creation request can be processed.
func (r CreateNCRRequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("qms: company id required")
	}
	if strings.TrimSpace(r.Title) == "" {
		return errors.New("qms: title required")
	}
	if r.DetectedBy <= 0 {
		return errors.New("qms: detected by required")
	}
	if r.ActorID <= 0 {
		return errors.New("qms: actor id required")
	}
	return nil
}

// CreateCAPARequest defines the payload for creating a CAPA.
type CreateCAPARequest struct {
	CompanyID        int64
	Title            string
	Description      string
	SourceType       string
	SourceID         *int64
	SourceReference  string
	Priority         string
	OwnerID          int64
	TeamMembers      []int64
	RootCauseMethod  string
	TargetDate       *time.Time
	ActorID          int64
}

// Validate ensures the creation request can be processed.
func (r CreateCAPARequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("qms: company id required")
	}
	if strings.TrimSpace(r.Title) == "" {
		return errors.New("qms: title required")
	}
	if r.OwnerID <= 0 {
		return errors.New("qms: owner id required")
	}
	if r.ActorID <= 0 {
		return errors.New("qms: actor id required")
	}
	return nil
}

// CreateAuditRequest defines the payload for creating an audit.
type CreateAuditRequest struct {
	CompanyID     int64
	Title         string
	Description   string
	AuditType     string
	Standard      string
	Scope         string
	LeadAuditorID int64
	AuditTeamIDs  []int64
	AuditeeID     *int64
	PlannedStart  *time.Time
	PlannedEnd    *time.Time
	ActorID       int64
}

// Validate ensures the creation request can be processed.
func (r CreateAuditRequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("qms: company id required")
	}
	if strings.TrimSpace(r.Title) == "" {
		return errors.New("qms: title required")
	}
	if r.LeadAuditorID <= 0 {
		return errors.New("qms: lead auditor id required")
	}
	if r.ActorID <= 0 {
		return errors.New("qms: actor id required")
	}
	return nil
}

// ListNCRsFilter configures ListNCRs queries.
type ListNCRsFilter struct {
	CompanyID         int64
	SourceType        string
	Category          string
	Severity          string
	Status            *Status
	AssignedTo        *int64
	DateFrom          *time.Time
	DateTo            *time.Time
	Limit             int
	Offset            int
}

// ListCAPAsFilter configures ListCAPAs queries.
type ListCAPAsFilter struct {
	CompanyID   int64
	SourceType  string
	Status      *Status
	Priority    string
	OwnerID     *int64
	DateFrom    *time.Time
	DateTo      *time.Time
	Limit       int
	Offset      int
}

var (
	ErrNCRNotFound    = errors.New("qms: non-conformance report not found")
	ErrCAPANotFound   = errors.New("qms: corrective action not found")
	ErrAuditNotFound  = errors.New("qms: audit not found")
	ErrInvalidStatus  = errors.New("qms: invalid status transition")
	ErrInspectionNotFound = errors.New("qms: inspection not found")
	ErrComplaintNotFound  = errors.New("qms: customer complaint not found")
)

// Inspection represents a QMS Inspection.
type Inspection struct {
	ID              int64
	CompanyID       int64
	Name            string
	Description     string
	ReferenceModule string
	ReferenceID     *int64
	Status          string
	InspectorID     *int64
	ScheduledAt     *time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	CreatedBy       int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// InspectionResult represents a single result characteristic of an Inspection.
type InspectionResult struct {
	ID                 int64
	CompanyID          int64
	InspectionID       int64
	CharacteristicName string
	ExpectedValue      string
	ActualValue        string
	IsConforming       bool
	Notes              string
	CreatedBy          int64
	CreatedAt          time.Time
}

// CustomerComplaint represents a customer complaint.
type CustomerComplaint struct {
	ID               int64
	CompanyID        int64
	ComplaintNumber  string
	CustomerID       int64
	Title            string
	Description      string
	Status           string
	Severity         string
	AssignedTo       *int64
	ResponseEvidence string
	ReceivedAt       time.Time
	ClosedAt         *time.Time
	CreatedBy        int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// CreateInspectionRequest defines the payload for creating an Inspection.
type CreateInspectionRequest struct {
	CompanyID       int64
	Name            string
	Description     string
	ReferenceModule string
	ReferenceID     *int64
	InspectorID     *int64
	ScheduledAt     *time.Time
	ActorID         int64
}

func (r CreateInspectionRequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("qms: company id required")
	}
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("qms: name required")
	}
	if r.ActorID <= 0 {
		return errors.New("qms: actor id required")
	}
	return nil
}

// CreateComplaintRequest defines the payload for creating a Customer Complaint.
type CreateComplaintRequest struct {
	CompanyID       int64
	CustomerID      int64
	Title           string
	Description     string
	Severity        string
	AssignedTo      *int64
	ActorID         int64
}

func (r CreateComplaintRequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("qms: company id required")
	}
	if r.CustomerID <= 0 {
		return errors.New("qms: customer id required")
	}
	if strings.TrimSpace(r.Title) == "" {
		return errors.New("qms: title required")
	}
	if r.ActorID <= 0 {
		return errors.New("qms: actor id required")
	}
	return nil
}

// QualityHold represents a blocking hold on an entity in QMS.
type QualityHold struct {
	ID              int64     `json:"id"`
	CompanyID       int64     `json:"company_id"`
	ReferenceModule string    `json:"reference_module"`
	ReferenceID     int64     `json:"reference_id"`
	Reason          string    `json:"reason"`
	Status          string    `json:"status"` // ACTIVE, RELEASED
	CreatedBy       int64     `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
	ReleasedBy      *int64    `json:"released_by"`
	ReleasedAt      *time.Time `json:"released_at"`
}

// CreateQualityHoldRequest contains input for creating a hold.
type CreateQualityHoldRequest struct {
	CompanyID       int64
	ReferenceModule string
	ReferenceID     int64
	Reason          string
	ActorID         int64
}

// CreateInspectionPlanRequest defines the payload for creating an InspectionPlan.
type CreateInspectionPlanRequest struct {
	CompanyID       int64
	Name            string
	Description     string
	ReferenceModule string
	ReferenceID     int64
	ActorID         int64
}

func (r CreateInspectionPlanRequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("qms: company id required")
	}
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("qms: name required")
	}
	if r.ActorID <= 0 {
		return errors.New("qms: actor id required")
	}
	return nil
}

// InspectionPlan represents a predefined inspection plan.
type InspectionPlan struct {
	ID              int64     `json:"id"`
	CompanyID       int64     `json:"company_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	ReferenceModule string    `json:"reference_module"`
	ReferenceID     int64     `json:"reference_id"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	CreatedBy       int64     `json:"created_by"`
}

// NormaliseStatus uppercases and trims the provided status string.
func NormaliseStatus(v string) Status {
	v = strings.TrimSpace(strings.ToUpper(v))
	switch Status(v) {
	case "OPEN", "UNDER_REVIEW", "DISPOSITIONED", "CLOSED", "CANCELLED",
		"IN_PROGRESS", "VERIFYING", "EFFECTIVE",
		"PLANNED", "COMPLETED", "REPORTED",
		"APPROVED", "CONDITIONAL", "REJECTED", "ON_HOLD",
		"PASSED", "FAILED", "RECEIVED", "TRIAGED", "INVESTIGATING", "RESPONDED":
		return Status(v)
	default:
		return NCRStatusOpen
	}
}