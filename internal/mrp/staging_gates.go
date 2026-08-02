package mrp

import (
	"context"
	"fmt"
	"time"
)

// StagingGate represents a multi-stage certification gate for manufacturing decisions
type StagingGate struct {
	ID           int64
	Name         string
	Description  string
	RequiredRoles []string // e.g., ["QUALITY_LEAD", "ENGINEERING", "PRODUCTION_MANAGER"]
	Signatures   []SignatureRecord
	Status       string // PENDING, SIGNED, APPROVED, REJECTED
	CreatedAt    time.Time
}

// SignatureRecord tracks who signed and when
type SignatureRecord struct {
	ActorID    int64
	ActorRole  string
	Timestamp  time.Time
	Signature  string // Challenge response
	Decision   string // APPROVE or REJECT
	Comment    string
}

// BOMApprovalGate manages multi-signature approval for BOM changes
type BOMApprovalGate struct {
	complianceGate *ComplianceGate
	repo           *SQLRepository
}

// NewBOMApprovalGate creates a new BOM approval gate
func NewBOMApprovalGate(cg *ComplianceGate, repo *SQLRepository) *BOMApprovalGate {
	return &BOMApprovalGate{
		complianceGate: cg,
		repo:           repo,
	}
}

// InitiateCertification starts the BOM approval certification process
func (g *BOMApprovalGate) InitiateCertification(ctx context.Context, companyID, bomID, actorID int64, requiredRoles []string) (StagingGate, error) {
	gate := StagingGate{
		Name:          fmt.Sprintf("BOM Approval: %d", bomID),
		Description:   fmt.Sprintf("Multi-signature approval for BOM %d", bomID),
		RequiredRoles: requiredRoles,
		Status:        "PENDING",
		CreatedAt:     time.Now(),
	}

	// Validate BOM can be approved
	validator := NewBOMApprovalValidator(g.repo)
	result := validator.Validate(ctx, companyID, bomID)
	if !result.Valid {
		return StagingGate{}, fmt.Errorf("BOM validation failed: %s", result.Reason)
	}

	return gate, nil
}

// AddSignature records an actor's decision on the certification gate
func (g *BOMApprovalGate) AddSignature(ctx context.Context, gate *StagingGate, record SignatureRecord) error {
	// Check actor has required role
	hasRole := false
	for _, reqRole := range gate.RequiredRoles {
		if record.ActorRole == reqRole {
			hasRole = true
			break
		}
	}

	if !hasRole {
		return fmt.Errorf("actor role %s not required for this gate", record.ActorRole)
	}

	// Check if actor already signed
	for _, sig := range gate.Signatures {
		if sig.ActorID == record.ActorID {
			return fmt.Errorf("actor %d has already signed", record.ActorID)
		}
	}

	// Record signature
	record.Timestamp = time.Now()
	gate.Signatures = append(gate.Signatures, record)

	// Check if all required roles have signed
	signedRoles := make(map[string]bool)
	approvalCount := 0
	for _, sig := range gate.Signatures {
		signedRoles[sig.ActorRole] = true
		if sig.Decision == "APPROVE" {
			approvalCount++
		}
	}

	allRolesSigned := true
	for _, reqRole := range gate.RequiredRoles {
		if !signedRoles[reqRole] {
			allRolesSigned = false
			break
		}
	}

	// Gate requires unanimous approval
	if allRolesSigned && approvalCount == len(gate.Signatures) {
		gate.Status = "APPROVED"
	} else if allRolesSigned && len(gate.Signatures) > approvalCount {
		gate.Status = "REJECTED"
	}

	return nil
}

// WorkOrderReleaseGate manages multi-stage approval for work order release
type WorkOrderReleaseGate struct {
	complianceGate *ComplianceGate
	repo           *SQLRepository
}

// NewWorkOrderReleaseGate creates a new work order release gate
func NewWorkOrderReleaseGate(cg *ComplianceGate, repo *SQLRepository) *WorkOrderReleaseGate {
	return &WorkOrderReleaseGate{
		complianceGate: cg,
		repo:           repo,
	}
}

// InitiateCertification starts the work order release certification process
func (g *WorkOrderReleaseGate) InitiateCertification(ctx context.Context, companyID, woID, actorID int64) (StagingGate, error) {
	gate := StagingGate{
		Name:          fmt.Sprintf("WO Release: %d", woID),
		Description:   fmt.Sprintf("Inventory and capacity verification for work order %d", woID),
		RequiredRoles: []string{"PLANNER", "PRODUCTION_MANAGER"},
		Status:        "PENDING",
		CreatedAt:     time.Now(),
	}

	// Validate work order can be released
	validator := NewWorkOrderReleaseValidator(g.repo)
	result := validator.Validate(ctx, companyID, woID)
	if !result.Valid {
		return StagingGate{}, fmt.Errorf("work order validation failed: %s", result.Reason)
	}

	return gate, nil
}

// AddSignature records an approver's decision
func (g *WorkOrderReleaseGate) AddSignature(ctx context.Context, gate *StagingGate, record SignatureRecord) error {
	// Check actor has required role
	hasRole := false
	for _, reqRole := range gate.RequiredRoles {
		if record.ActorRole == reqRole {
			hasRole = true
			break
		}
	}

	if !hasRole {
		return fmt.Errorf("actor role %s not required for work order release", record.ActorRole)
	}

	record.Timestamp = time.Now()
	gate.Signatures = append(gate.Signatures, record)

	// For WO release, need at least one planner + one PM approval
	plannerApproved := false
	pmApproved := false
	for _, sig := range gate.Signatures {
		if sig.ActorRole == "PLANNER" && sig.Decision == "APPROVE" {
			plannerApproved = true
		}
		if sig.ActorRole == "PRODUCTION_MANAGER" && sig.Decision == "APPROVE" {
			pmApproved = true
		}
		if sig.Decision == "REJECT" {
			gate.Status = "REJECTED"
			return nil
		}
	}

	if plannerApproved && pmApproved {
		gate.Status = "APPROVED"
	}

	return nil
}

// HoldReleaseGate manages quality hold release approvals
type HoldReleaseGate struct {
	complianceGate *ComplianceGate
	repo           *SQLRepository
}

// NewHoldReleaseGate creates a new hold release gate
func NewHoldReleaseGate(cg *ComplianceGate, repo *SQLRepository) *HoldReleaseGate {
	return &HoldReleaseGate{
		complianceGate: cg,
		repo:           repo,
	}
}

// InitiateCertification starts the hold release certification process
func (g *HoldReleaseGate) InitiateCertification(ctx context.Context, holdID, actorID int64) (StagingGate, error) {
	gate := StagingGate{
		Name:          fmt.Sprintf("Hold Release: %d", holdID),
		Description:   fmt.Sprintf("Quality hold release authorization for hold %d", holdID),
		RequiredRoles: []string{"QUALITY_MANAGER"},
		Status:        "PENDING",
		CreatedAt:     time.Now(),
	}

	// Validate hold can be released
	validator := NewHoldReleaseValidator(g.repo)
	result := validator.Validate(ctx, holdID)
	if !result.Valid {
		return StagingGate{}, fmt.Errorf("hold validation failed: %s", result.Reason)
	}

	return gate, nil
}

// AddSignature records the quality manager's decision
func (g *HoldReleaseGate) AddSignature(ctx context.Context, gate *StagingGate, record SignatureRecord) error {
	if record.ActorRole != "QUALITY_MANAGER" {
		return fmt.Errorf("only QUALITY_MANAGER can approve hold release")
	}

	record.Timestamp = time.Now()
	gate.Signatures = append(gate.Signatures, record)

	if record.Decision == "APPROVE" {
		gate.Status = "APPROVED"
	} else if record.Decision == "REJECT" {
		gate.Status = "REJECTED"
	}

	return nil
}

// NCRDispositionGate manages NCR quality disposition approvals
type NCRDispositionGate struct {
	complianceGate *ComplianceGate
	repo           *SQLRepository
}

// NewNCRDispositionGate creates a new NCR disposition gate
func NewNCRDispositionGate(cg *ComplianceGate, repo *SQLRepository) *NCRDispositionGate {
	return &NCRDispositionGate{
		complianceGate: cg,
		repo:           repo,
	}
}

// InitiateCertification starts the NCR disposition certification process
func (g *NCRDispositionGate) InitiateCertification(ctx context.Context, ncrID, actorID int64) (StagingGate, error) {
	gate := StagingGate{
		Name:          fmt.Sprintf("NCR Disposition: %d", ncrID),
		Description:   fmt.Sprintf("Quality disposition decision for NCR %d", ncrID),
		RequiredRoles: []string{"QUALITY_LEAD", "ENGINEERING"},
		Status:        "PENDING",
		CreatedAt:     time.Now(),
	}

	// Validate NCR can be disposed
	validator := NewQualityDispositionValidator(g.repo)
	result := validator.Validate(ctx, ncrID)
	if !result.Valid {
		return StagingGate{}, fmt.Errorf("NCR validation failed: %s", result.Reason)
	}

	return gate, nil
}

// AddSignature records a decision on NCR disposition
func (g *NCRDispositionGate) AddSignature(ctx context.Context, gate *StagingGate, record SignatureRecord) error {
	hasRole := false
	for _, reqRole := range gate.RequiredRoles {
		if record.ActorRole == reqRole {
			hasRole = true
			break
		}
	}

	if !hasRole {
		return fmt.Errorf("actor role %s not required for NCR disposition", record.ActorRole)
	}

	record.Timestamp = time.Now()
	gate.Signatures = append(gate.Signatures, record)

	// NCR disposition requires both quality lead and engineering to approve
	qualityApproved := false
	engApproved := false
	for _, sig := range gate.Signatures {
		if sig.ActorRole == "QUALITY_LEAD" && sig.Decision == "APPROVE" {
			qualityApproved = true
		}
		if sig.ActorRole == "ENGINEERING" && sig.Decision == "APPROVE" {
			engApproved = true
		}
		if sig.Decision == "REJECT" {
			gate.Status = "REJECTED"
			return nil
		}
	}

	if qualityApproved && engApproved {
		gate.Status = "APPROVED"
	}

	return nil
}

// CAPAClosureGate manages corrective action closure approvals
type CAPAClosureGate struct {
	complianceGate *ComplianceGate
	repo           *SQLRepository
}

// NewCAPAClosureGate creates a new CAPA closure gate
func NewCAPAClosureGate(cg *ComplianceGate, repo *SQLRepository) *CAPAClosureGate {
	return &CAPAClosureGate{
		complianceGate: cg,
		repo:           repo,
	}
}

// InitiateCertification starts the CAPA closure certification process
func (g *CAPAClosureGate) InitiateCertification(ctx context.Context, capaID, actorID int64) (StagingGate, error) {
	gate := StagingGate{
		Name:          fmt.Sprintf("CAPA Closure: %d", capaID),
		Description:   fmt.Sprintf("Corrective action closure verification for CAPA %d", capaID),
		RequiredRoles: []string{"QUALITY_MANAGER", "PROCESS_OWNER"},
		Status:        "PENDING",
		CreatedAt:     time.Now(),
	}

	// Placeholder validation - in production would check CAPA status
	return gate, nil
}

// AddSignature records an approval on CAPA closure
func (g *CAPAClosureGate) AddSignature(ctx context.Context, gate *StagingGate, record SignatureRecord) error {
	hasRole := false
	for _, reqRole := range gate.RequiredRoles {
		if record.ActorRole == reqRole {
			hasRole = true
			break
		}
	}

	if !hasRole {
		return fmt.Errorf("actor role %s not required for CAPA closure", record.ActorRole)
	}

	record.Timestamp = time.Now()
	gate.Signatures = append(gate.Signatures, record)

	// CAPA closure requires both QM and PO to approve
	qmApproved := false
	poApproved := false
	for _, sig := range gate.Signatures {
		if sig.ActorRole == "QUALITY_MANAGER" && sig.Decision == "APPROVE" {
			qmApproved = true
		}
		if sig.ActorRole == "PROCESS_OWNER" && sig.Decision == "APPROVE" {
			poApproved = true
		}
		if sig.Decision == "REJECT" {
			gate.Status = "REJECTED"
			return nil
		}
	}

	if qmApproved && poApproved {
		gate.Status = "APPROVED"
	}

	return nil
}
