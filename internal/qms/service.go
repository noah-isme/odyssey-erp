package qms

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Service orchestrates QMS operations.
type Service struct {
	repo *Repository
	now  func() time.Time
}

// NewService constructs a Service instance.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// WithNow overrides the clock for deterministic tests.
func (s *Service) WithNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// CreateNCR creates a new non-conformance report.
func (s *Service) CreateNCR(ctx context.Context, req CreateNCRRequest) (NonConformanceReport, error) {
	if err := req.Validate(); err != nil {
		return NonConformanceReport{}, err
	}

	number, err := s.generateNCRNumber(ctx, req.CompanyID)
	if err != nil {
		return NonConformanceReport{}, err
	}

	ncr, err := s.repo.InsertNCR(ctx, req, number)
	if err != nil {
		return NonConformanceReport{}, err
	}
	return ncr, nil
}

func (s *Service) generateNCRNumber(ctx context.Context, companyID int64) (string, error) {
	year := time.Now().Year()
	prefix := fmt.Sprintf("NCR-%d-", year)
	count, err := s.repo.CountNCRsWithPrefix(ctx, companyID, prefix)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%05d", prefix, count+1), nil
}

// GetNCR loads a single NCR.
func (s *Service) GetNCR(ctx context.Context, id int64) (NonConformanceReport, error) {
	return s.repo.GetNCR(ctx, id)
}

// ListNCRs returns NCRs filtered by criteria.
func (s *Service) ListNCRs(ctx context.Context, filter ListNCRsFilter) ([]NonConformanceReport, error) {
	return s.repo.ListNCRs(ctx, filter)
}

// UpdateNCR updates an NCR.
func (s *Service) UpdateNCR(ctx context.Context, id int64, req UpdateNCRRequest) (NonConformanceReport, error) {
	if err := req.Validate(); err != nil {
		return NonConformanceReport{}, err
	}
	return s.repo.UpdateNCR(ctx, id, req)
}

// UpdateNCRStatus transitions an NCR's status.
func (s *Service) UpdateNCRStatus(ctx context.Context, id int64, status Status, actorID int64) (NonConformanceReport, error) {
	if err := validateNCRStatusTransition(status); err != nil {
		return NonConformanceReport{}, err
	}
	return s.repo.UpdateNCRStatus(ctx, id, status, actorID)
}

// RecordDisposition records the disposition for an NCR.
func (s *Service) RecordDisposition(ctx context.Context, req RecordDispositionRequest) (NCRDisposition, error) {
	if err := req.Validate(); err != nil {
		return NCRDisposition{}, err
	}
	return s.repo.InsertDisposition(ctx, req)
}

// CreateCAPA creates a new corrective/preventive action.
func (s *Service) CreateCAPA(ctx context.Context, req CreateCAPARequest) (CorrectiveAction, error) {
	if err := req.Validate(); err != nil {
		return CorrectiveAction{}, err
	}

	number, err := s.generateCAPANumber(ctx, req.CompanyID)
	if err != nil {
		return CorrectiveAction{}, err
	}

	capa, err := s.repo.InsertCAPA(ctx, req, number)
	if err != nil {
		return CorrectiveAction{}, err
	}
	return capa, nil
}

func (s *Service) generateCAPANumber(ctx context.Context, companyID int64) (string, error) {
	year := time.Now().Year()
	prefix := fmt.Sprintf("CAPA-%d-", year)
	count, err := s.repo.CountCAPAsWithPrefix(ctx, companyID, prefix)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%05d", prefix, count+1), nil
}

// GetCAPA loads a single CAPA.
func (s *Service) GetCAPA(ctx context.Context, id int64) (CorrectiveAction, error) {
	return s.repo.GetCAPA(ctx, id)
}

// ListCAPAs returns CAPAs filtered by criteria.
func (s *Service) ListCAPAs(ctx context.Context, filter ListCAPAsFilter) ([]CorrectiveAction, error) {
	return s.repo.ListCAPAs(ctx, filter)
}

// UpdateCAPA updates a CAPA.
func (s *Service) UpdateCAPA(ctx context.Context, id int64, req UpdateCAPARequest) (CorrectiveAction, error) {
	if err := req.Validate(); err != nil {
		return CorrectiveAction{}, err
	}
	return s.repo.UpdateCAPA(ctx, id, req)
}

// UpdateCAPAStatus transitions a CAPA's status.
func (s *Service) UpdateCAPAStatus(ctx context.Context, id int64, status Status, actorID int64) (CorrectiveAction, error) {
	if err := validateCAPAStatusTransition(status); err != nil {
		return CorrectiveAction{}, err
	}
	return s.repo.UpdateCAPAStatus(ctx, id, status, actorID)
}

// CreateAudit creates a new audit.
func (s *Service) CreateAudit(ctx context.Context, req CreateAuditRequest) (Audit, error) {
	if err := req.Validate(); err != nil {
		return Audit{}, err
	}

	number, err := s.generateAuditNumber(ctx, req.CompanyID)
	if err != nil {
		return Audit{}, err
	}

	audit, err := s.repo.InsertAudit(ctx, req, number)
	if err != nil {
		return Audit{}, err
	}
	return audit, nil
}

func (s *Service) generateAuditNumber(ctx context.Context, companyID int64) (string, error) {
	year := time.Now().Year()
	prefix := fmt.Sprintf("AUD-%d-", year)
	count, err := s.repo.CountAuditsWithPrefix(ctx, companyID, prefix)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%05d", prefix, count+1), nil
}

// GetAudit loads a single audit.
func (s *Service) GetAudit(ctx context.Context, id int64) (Audit, error) {
	return s.repo.GetAudit(ctx, id)
}

// ListAudits returns audits filtered by criteria.
func (s *Service) ListAudits(ctx context.Context, companyID int64, status *Status, auditType string, limit, offset int) ([]Audit, error) {
	return s.repo.ListAudits(ctx, companyID, status, auditType, limit, offset)
}

// UpdateAudit updates an audit.
func (s *Service) UpdateAudit(ctx context.Context, id int64, req UpdateAuditRequest) (Audit, error) {
	return s.repo.UpdateAudit(ctx, id, req)
}

// AddFinding adds a finding to an audit.
func (s *Service) AddFinding(ctx context.Context, req CreateFindingRequest) (AuditFinding, error) {
	if err := req.Validate(); err != nil {
		return AuditFinding{}, err
	}
	return s.repo.InsertFinding(ctx, req)
}

// GetFindings returns findings for an audit.
func (s *Service) GetFindings(ctx context.Context, auditID int64) ([]AuditFinding, error) {
	return s.repo.GetFindings(ctx, auditID)
}

// UpdateFinding updates an audit finding.
func (s *Service) UpdateFinding(ctx context.Context, id int64, req UpdateFindingRequest) (AuditFinding, error) {
	return s.repo.UpdateFinding(ctx, id, req)
}

// CreateSupplierQuality creates a supplier quality record.
func (s *Service) CreateSupplierQuality(ctx context.Context, req CreateSupplierQualityRequest) (SupplierQuality, error) {
	if err := req.Validate(); err != nil {
		return SupplierQuality{}, err
	}
	return s.repo.InsertSupplierQuality(ctx, req)
}

// GetSupplierQuality loads a supplier quality record.
func (s *Service) GetSupplierQuality(ctx context.Context, id int64) (SupplierQuality, error) {
	return s.repo.GetSupplierQuality(ctx, id)
}

// ListSupplierQuality returns supplier quality records.
func (s *Service) ListSupplierQuality(ctx context.Context, companyID int64, status *Status, limit, offset int) ([]SupplierQuality, error) {
	return s.repo.ListSupplierQuality(ctx, companyID, status, limit, offset)
}

// CreateSupplierAudit creates a supplier audit.
func (s *Service) CreateSupplierAudit(ctx context.Context, req CreateSupplierAuditRequest) (SupplierAudit, error) {
	if err := req.Validate(); err != nil {
		return SupplierAudit{}, err
	}
	return s.repo.InsertSupplierAudit(ctx, req)
}

// CreateQualityObjective creates a quality objective.
func (s *Service) CreateQualityObjective(ctx context.Context, req CreateQualityObjectiveRequest) (QualityObjective, error) {
	if err := req.Validate(); err != nil {
		return QualityObjective{}, err
	}
	return s.repo.InsertQualityObjective(ctx, req)
}

// RecordMeasurement records a measurement for a quality objective.
func (s *Service) RecordMeasurement(ctx context.Context, req RecordMeasurementRequest) (QualityObjectiveMeasurement, error) {
	if err := req.Validate(); err != nil {
		return QualityObjectiveMeasurement{}, err
	}
	return s.repo.InsertMeasurement(ctx, req)
}

// GetMeasurements returns measurements for an objective.
func (s *Service) GetMeasurements(ctx context.Context, objectiveID int64, limit int) ([]QualityObjectiveMeasurement, error) {
	return s.repo.GetMeasurements(ctx, objectiveID, limit)
}

// UpdateNCRRequest defines the payload for updating an NCR.
type UpdateNCRRequest struct {
	Title              string
	Description        string
	Category           string
	Severity           string
	ResponsiblePartyID *int64
	AssignedTo         *int64
	TargetClosureDate  *time.Time
	RootCause          string
	ContainmentAction  string
	ActorID            int64
}

func (r UpdateNCRRequest) Validate() error {
	if r.ActorID <= 0 {
		return errors.New("qms: actor id required")
	}
	return nil
}

// RecordDispositionRequest defines the payload for recording a disposition.
type RecordDispositionRequest struct {
	NCRID           int64
	DispositionType string
	Description     string
	ApprovedBy      int64
	ActorID         int64
}

func (r RecordDispositionRequest) Validate() error {
	if r.NCRID <= 0 {
		return errors.New("qms: ncr id required")
	}
	if strings.TrimSpace(r.DispositionType) == "" {
		return errors.New("qms: disposition type required")
	}
	if r.ApprovedBy <= 0 {
		return errors.New("qms: approved by required")
	}
	if r.ActorID <= 0 {
		return errors.New("qms: actor id required")
	}
	return nil
}

// UpdateCAPARequest defines the payload for updating a CAPA.
type UpdateCAPARequest struct {
	Title              string
	Description        string
	Priority           string
	OwnerID            *int64
	TeamMembers        []int64
	RootCause          string
	RootCauseMethod    string
	CorrectiveAction   string
	PreventiveAction   string
	VerificationMethod string
	VerificationResult string
	EffectivenessCheck string
	TargetDate         *time.Time
	ActorID            int64
}

func (r UpdateCAPARequest) Validate() error {
	if r.ActorID <= 0 {
		return errors.New("qms: actor id required")
	}
	return nil
}

// UpdateAuditRequest defines the payload for updating an audit.
type UpdateAuditRequest struct {
	Title        string
	Description  string
	Scope        string
	LeadAuditorID *int64
	AuditTeamIDs []int64
	AuditeeID    *int64
	PlannedStart *time.Time
	PlannedEnd   *time.Time
	ActorID      int64
}

// CreateFindingRequest defines the payload for creating an audit finding.
type CreateFindingRequest struct {
	AuditID         int64
	Category        string
	Clause          string
	Description     string
	Evidence        string
	Requirement     string
	RiskLevel       string
	AssignedTo      *int64
	ResponseDueDate *time.Time
	ActorID         int64
}

func (r CreateFindingRequest) Validate() error {
	if r.AuditID <= 0 {
		return errors.New("qms: audit id required")
	}
	if strings.TrimSpace(r.Category) == "" {
		return errors.New("qms: category required")
	}
	if strings.TrimSpace(r.Description) == "" {
		return errors.New("qms: description required")
	}
	if r.ActorID <= 0 {
		return errors.New("qms: actor id required")
	}
	return nil
}

// UpdateFindingRequest defines the payload for updating a finding.
type UpdateFindingRequest struct {
	Category        string
	Description     string
	Evidence        string
	Requirement     string
	RiskLevel       string
	Status          string
	Response        string
	ResponseDueDate *time.Time
	AssignedTo      *int64
	VerifiedBy      *int64
	ActorID         int64
}

// CreateSupplierQualityRequest defines the payload for creating supplier quality.
type CreateSupplierQualityRequest struct {
	CompanyID     int64
	SupplierID    int64
	Status        Status
	QualityRating float64
	RiskLevel     string
	ApprovedDate  *time.Time
	ExpiryDate    *time.Time
	Notes         string
	ActorID       int64
}

func (r CreateSupplierQualityRequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("qms: company id required")
	}
	if r.SupplierID <= 0 {
		return errors.New("qms: supplier id required")
	}
	if r.ActorID <= 0 {
		return errors.New("qms: actor id required")
	}
	return nil
}

// CreateSupplierAuditRequest defines the payload for creating a supplier audit.
type CreateSupplierAuditRequest struct {
	CompanyID    int64
	SupplierID   int64
	AuditType    string
	Standard     string
	PlannedDate  *time.Time
	LeadAuditorID int64
	ActorID      int64
}

func (r CreateSupplierAuditRequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("qms: company id required")
	}
	if r.SupplierID <= 0 {
		return errors.New("qms: supplier id required")
	}
	if r.LeadAuditorID <= 0 {
		return errors.New("qms: lead auditor id required")
	}
	if r.ActorID <= 0 {
		return errors.New("qms: actor id required")
	}
	return nil
}

// CreateQualityObjectiveRequest defines the payload for creating a quality objective.
type CreateQualityObjectiveRequest struct {
	CompanyID   int64
	Name        string
	Description string
	MetricType  string
	TargetValue float64
	Unit        string
	Frequency   string
	OwnerID     int64
	StartDate   time.Time
	EndDate     *time.Time
	ActorID     int64
}

func (r CreateQualityObjectiveRequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("qms: company id required")
	}
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("qms: name required")
	}
	if strings.TrimSpace(r.MetricType) == "" {
		return errors.New("qms: metric type required")
	}
	if r.OwnerID <= 0 {
		return errors.New("qms: owner id required")
	}
	if r.ActorID <= 0 {
		return errors.New("qms: actor id required")
	}
	return nil
}

// RecordMeasurementRequest defines the payload for recording a measurement.
type RecordMeasurementRequest struct {
	ObjectiveID     int64
	Value           float64
	MeasurementDate time.Time
	Notes           string
	RecordedBy      int64
	ActorID         int64
}

func (r RecordMeasurementRequest) Validate() error {
	if r.ObjectiveID <= 0 {
		return errors.New("qms: objective id required")
	}
	if r.RecordedBy <= 0 {
		return errors.New("qms: recorded by required")
	}
	if r.ActorID <= 0 {
		return errors.New("qms: actor id required")
	}
	return nil
}

func validateNCRStatusTransition(newStatus Status) error {
	validStatuses := []Status{
		NCRStatusOpen, NCRStatusUnderReview, NCRStatusDispositioned,
		NCRStatusClosed, NCRStatusCancelled,
	}
	for _, s := range validStatuses {
		if s == newStatus {
			return nil
		}
	}
	return fmt.Errorf("qms: invalid NCR status %s", newStatus)
}

func validateCAPAStatusTransition(newStatus Status) error {
	validStatuses := []Status{
		CAPAStatusOpen, CAPAStatusInProgress, CAPAStatusVerifying,
		CAPAStatusEffective, CAPAStatusClosed,
	}
	for _, s := range validStatuses {
		if s == newStatus {
			return nil
		}
	}
	return fmt.Errorf("qms: invalid CAPA status %s", newStatus)
}

// =============================================================================
// Inspections
// =============================================================================

// CreateInspection creates a new quality inspection.
func (s *Service) CreateInspection(ctx context.Context, req CreateInspectionRequest) (Inspection, error) {
	if err := req.Validate(); err != nil {
		return Inspection{}, err
	}
	return s.repo.InsertInspection(ctx, req)
}

// CreateInspectionPlan creates a new inspection plan.
func (s *Service) CreateInspectionPlan(ctx context.Context, req CreateInspectionPlanRequest) (InspectionPlan, error) {
	if err := req.Validate(); err != nil {
		return InspectionPlan{}, err
	}
	return s.repo.InsertInspectionPlan(ctx, req)
}

// GetInspection retrieves an inspection by ID.
func (s *Service) GetInspection(ctx context.Context, id int64) (Inspection, error) {
	return s.repo.GetInspection(ctx, id)
}

// ListInspections retrieves a page of inspections.
func (s *Service) ListInspections(ctx context.Context, companyID int64, status, refModule string, limit, offset int32) ([]Inspection, error) {
	return s.repo.ListInspections(ctx, companyID, status, refModule, limit, offset)
}

// UpdateInspectionStatus transitions the state of an inspection.
func (s *Service) UpdateInspectionStatus(ctx context.Context, id int64, status string) error {
	var startedAt, completedAt *time.Time
	now := s.now()
	
	switch status {
	case "IN_PROGRESS":
		startedAt = &now
	case "PASSED", "FAILED":
		completedAt = &now
	}
	
	return s.repo.UpdateInspectionStatus(ctx, id, status, startedAt, completedAt)
}

// =============================================================================
// Customer Complaints
// =============================================================================

// CreateComplaint creates a new customer complaint.
func (s *Service) CreateComplaint(ctx context.Context, req CreateComplaintRequest) (CustomerComplaint, error) {
	if err := req.Validate(); err != nil {
		return CustomerComplaint{}, err
	}
	
	number := fmt.Sprintf("CMPL-%d-%d", time.Now().Year(), time.Now().UnixNano()%10000)
	
	return s.repo.InsertComplaint(ctx, req, number)
}

// GetComplaint retrieves a complaint by ID.
func (s *Service) GetComplaint(ctx context.Context, id int64) (CustomerComplaint, error) {
	return s.repo.GetComplaint(ctx, id)
}

// ListComplaints retrieves a page of complaints.
func (s *Service) ListComplaints(ctx context.Context, companyID int64, status, severity string, customerID *int64, limit, offset int32) ([]CustomerComplaint, error) {
	return s.repo.ListComplaints(ctx, companyID, status, severity, customerID, limit, offset)
}

// UpdateComplaintStatus transitions the state of a complaint.
func (s *Service) UpdateComplaintStatus(ctx context.Context, id int64, status string) error {
	var closedAt *time.Time
	if status == "CLOSED" {
		now := s.now()
		closedAt = &now
	}
	
	return s.repo.UpdateComplaintStatus(ctx, id, status, closedAt)
}

// =============================================================================
// Holds & Plans
// =============================================================================

func (s *Service) CreateQualityHold(ctx context.Context, req CreateQualityHoldRequest) (QualityHold, error) {
	if req.CompanyID <= 0 {
		return QualityHold{}, errors.New("qms: company id required")
	}
	if req.ReferenceModule == "" {
		return QualityHold{}, errors.New("qms: reference module required")
	}
	if req.ReferenceID <= 0 {
		return QualityHold{}, errors.New("qms: reference id required")
	}
	if req.ActorID <= 0 {
		return QualityHold{}, errors.New("qms: actor id required")
	}

	id, err := s.repo.InsertQualityHold(ctx, req)
	if err != nil {
		return QualityHold{}, err
	}

	return QualityHold{
		ID:              id,
		CompanyID:       req.CompanyID,
		ReferenceModule: req.ReferenceModule,
		ReferenceID:     req.ReferenceID,
		Reason:          req.Reason,
		Status:          "ACTIVE",
		CreatedBy:       req.ActorID,
		CreatedAt:       s.now(),
	}, nil
}

func (s *Service) ReleaseQualityHold(ctx context.Context, id, companyID, actorID int64) error {
	if id <= 0 || companyID <= 0 || actorID <= 0 {
		return errors.New("qms: invalid arguments for release hold")
	}
	return s.repo.ReleaseQualityHold(ctx, id, companyID, actorID)
}