package mrp

import (
	"context"
	"fmt"
)

// ValidatorResult represents the outcome of a validation check
type ValidatorResult struct {
	Valid  bool
	Reason string
	Data   map[string]interface{}
}

// BOMApprovalValidator validates BOM readiness for approval
type BOMApprovalValidator struct {
	repo *SQLRepository
}

// NewBOMApprovalValidator creates a new BOM approval validator
func NewBOMApprovalValidator(repo *SQLRepository) *BOMApprovalValidator {
	return &BOMApprovalValidator{repo: repo}
}

// Validate checks structural completeness and validity of a BOM
func (v *BOMApprovalValidator) Validate(ctx context.Context, companyID, bomID int64) ValidatorResult {
	result := ValidatorResult{
		Data: make(map[string]interface{}),
	}

	// Check 1: BOM exists and is in DRAFT status
	bom, err := v.repo.GetBOM(ctx, companyID, bomID)
	if err != nil {
		result.Valid = false
		result.Reason = fmt.Sprintf("BOM not found: %v", err)
		return result
	}

	if bom.RevisionStatus != "DRAFT" && bom.RevisionStatus != "PENDING" {
		result.Valid = false
		result.Reason = fmt.Sprintf("BOM status %s is not eligible for approval", bom.RevisionStatus)
		return result
	}

	// Check 2: BOM has at least one line item
	if len(bom.Lines) == 0 {
		result.Valid = false
		result.Reason = "BOM has no line items"
		return result
	}

	result.Data["line_count"] = len(bom.Lines)

	// Check 3: All line items have valid quantities
	for _, line := range bom.Lines {
		if line.Quantity <= 0 {
			result.Valid = false
			result.Reason = fmt.Sprintf("BOM line has invalid quantity: %f", line.Quantity)
			return result
		}
	}

	// Check 4: Scrap percentage is reasonable
	if bom.ScrapPct < 0 || bom.ScrapPct > 100 {
		result.Valid = false
		result.Reason = fmt.Sprintf("BOM scrap percentage invalid: %f", bom.ScrapPct)
		return result
	}

	result.Data["scrap_pct"] = bom.ScrapPct
	result.Data["version"] = bom.Version

	// All checks passed
	result.Valid = true
	result.Reason = "BOM structure is complete and valid for approval"
	return result
}

// WorkOrderReleaseValidator validates work order readiness for release
type WorkOrderReleaseValidator struct {
	repo *SQLRepository
}

// NewWorkOrderReleaseValidator creates a new work order release validator
func NewWorkOrderReleaseValidator(repo *SQLRepository) *WorkOrderReleaseValidator {
	return &WorkOrderReleaseValidator{repo: repo}
}

// Validate checks work order is in planned state and has operations
func (v *WorkOrderReleaseValidator) Validate(ctx context.Context, companyID, woID int64) ValidatorResult {
	result := ValidatorResult{
		Data: make(map[string]interface{}),
	}

	// Check 1: Work order exists and is in PLANNED status
	wo, err := v.repo.GetWorkOrder(ctx, companyID, woID)
	if err != nil {
		result.Valid = false
		result.Reason = fmt.Sprintf("Work order not found: %v", err)
		return result
	}

	if wo.Status != "PLANNED" {
		result.Valid = false
		result.Reason = fmt.Sprintf("Work order status %s is not PLANNED", wo.Status)
		return result
	}

	// Check 2: Work order has BOM assigned
	if wo.BOMID == 0 {
		result.Valid = false
		result.Reason = "Work order has no BOM assigned"
		return result
	}

	// Check 3: Planned quantity is valid
	if wo.PlannedQty <= 0 {
		result.Valid = false
		result.Reason = "Work order planned quantity must be greater than zero"
		return result
	}

	result.Data["bom_id"] = wo.BOMID
	result.Data["planned_qty"] = wo.PlannedQty
	result.Data["warehouse_id"] = wo.WarehouseID

	// All checks passed
	result.Valid = true
	result.Reason = "Work order is ready for release"
	return result
}

// OperationCompletionValidator validates operation readiness for completion
type OperationCompletionValidator struct {
	repo *SQLRepository
}

// NewOperationCompletionValidator creates a new operation completion validator
func NewOperationCompletionValidator(repo *SQLRepository) *OperationCompletionValidator {
	return &OperationCompletionValidator{repo: repo}
}

// Validate checks operation status and time tracking
func (v *OperationCompletionValidator) Validate(ctx context.Context, woOp WorkOrderOperation) ValidatorResult {
	result := ValidatorResult{
		Data: make(map[string]interface{}),
	}

	// Check 1: Operation is in IN_PROGRESS status
	if woOp.Status != "IN_PROGRESS" {
		result.Valid = false
		result.Reason = fmt.Sprintf("Operation status %s is not IN_PROGRESS", woOp.Status)
		return result
	}

	// Check 2: Scheduled times are set
	if woOp.ScheduledStart.IsZero() {
		result.Valid = false
		result.Reason = "Operation scheduled start time not set"
		return result
	}

	if woOp.ScheduledEnd.IsZero() {
		result.Valid = false
		result.Reason = "Operation scheduled end time not set"
		return result
	}

	// Check 3: Actual time tracking is recorded
	if woOp.ActualSetupMinutes < 0 || woOp.ActualRunMinutes < 0 {
		result.Valid = false
		result.Reason = "Actual setup and run minutes must be non-negative"
		return result
	}

	if woOp.ActualSetupMinutes == 0 && woOp.ActualRunMinutes == 0 {
		result.Valid = false
		result.Reason = "Operation time tracking not recorded"
		return result
	}

	// Check 4: Output quantity is recorded
	if woOp.GoodQuantity <= 0 {
		result.Valid = false
		result.Reason = "Operation good quantity must be greater than zero"
		return result
	}

	result.Data["setup_minutes"] = woOp.ActualSetupMinutes
	result.Data["run_minutes"] = woOp.ActualRunMinutes
	result.Data["good_quantity"] = woOp.GoodQuantity
	result.Data["scrap_quantity"] = woOp.ScrapQuantity

	// All checks passed
	result.Valid = true
	result.Reason = "Operation has time tracking and output recorded"
	return result
}

// ScheduleOverrideValidator validates schedule override decisions
type ScheduleOverrideValidator struct {
	repo *SQLRepository
}

// NewScheduleOverrideValidator creates a new schedule override validator
func NewScheduleOverrideValidator(repo *SQLRepository) *ScheduleOverrideValidator {
	return &ScheduleOverrideValidator{repo: repo}
}

// Validate checks schedule override impact
func (v *ScheduleOverrideValidator) Validate(ctx context.Context, companyID, woID int64) ValidatorResult {
	result := ValidatorResult{
		Data: make(map[string]interface{}),
	}

	// Check 1: Work order exists
	wo, err := v.repo.GetWorkOrder(ctx, companyID, woID)
	if err != nil {
		result.Valid = false
		result.Reason = fmt.Sprintf("Work order not found: %v", err)
		return result
	}

	result.Data["work_order_id"] = wo.ID
	result.Data["product_id"] = wo.ProductID
	result.Data["planned_qty"] = wo.PlannedQty

	// Check 2: Work order is in a state that allows schedule change
	if wo.Status != "PLANNED" && wo.Status != "RELEASED" {
		result.Valid = false
		result.Reason = fmt.Sprintf("Work order status %s does not allow schedule changes", wo.Status)
		return result
	}

	// All checks passed
	result.Valid = true
	result.Reason = "Schedule override is eligible for review"
	return result
}

// HoldReleaseValidator validates quality hold release conditions
type HoldReleaseValidator struct {
	repo *SQLRepository
}

// NewHoldReleaseValidator creates a new hold release validator
func NewHoldReleaseValidator(repo *SQLRepository) *HoldReleaseValidator {
	return &HoldReleaseValidator{repo: repo}
}

// Validate checks hold can be released
func (v *HoldReleaseValidator) Validate(ctx context.Context, holdID int64) ValidatorResult {
	result := ValidatorResult{
		Data: make(map[string]interface{}),
	}

	result.Data["hold_id"] = holdID
	
	// Placeholder for governance implementation
	// In production, this would query QualityHold table from governance schema
	result.Valid = true
	result.Reason = "Hold release is eligible for decision gate"
	return result
}

// QualityDispositionValidator validates NCR quality disposition
type QualityDispositionValidator struct {
	repo *SQLRepository
}

// NewQualityDispositionValidator creates a new quality disposition validator
func NewQualityDispositionValidator(repo *SQLRepository) *QualityDispositionValidator {
	return &QualityDispositionValidator{repo: repo}
}

// Validate checks NCR is ready for disposition
func (v *QualityDispositionValidator) Validate(ctx context.Context, ncrID int64) ValidatorResult {
	result := ValidatorResult{
		Data: make(map[string]interface{}),
	}

	result.Data["ncr_id"] = ncrID

	// Placeholder for governance implementation
	// In production, this would query QualityNCR table from governance schema
	result.Valid = true
	result.Reason = "NCR is eligible for disposition decision"
	return result
}

// SubcontractAcceptanceValidator validates subcontract goods acceptance
type SubcontractAcceptanceValidator struct {
	repo *SQLRepository
}

// NewSubcontractAcceptanceValidator creates a new subcontract acceptance validator
func NewSubcontractAcceptanceValidator(repo *SQLRepository) *SubcontractAcceptanceValidator {
	return &SubcontractAcceptanceValidator{repo: repo}
}

// Validate checks subcontract receipt is ready for acceptance
func (v *SubcontractAcceptanceValidator) Validate(ctx context.Context, receiptID int64) ValidatorResult {
	result := ValidatorResult{
		Data: make(map[string]interface{}),
	}

	result.Data["receipt_id"] = receiptID

	// Placeholder for governance implementation
	// In production, this would query SubcontractReceipt table from governance schema
	result.Valid = true
	result.Reason = "Subcontract receipt is eligible for acceptance decision"
	return result
}

// GoodsReceiptValidator validates goods receipt readiness
type GoodsReceiptValidator struct {
	repo *SQLRepository
}

// NewGoodsReceiptValidator creates a new goods receipt validator
func NewGoodsReceiptValidator(repo *SQLRepository) *GoodsReceiptValidator {
	return &GoodsReceiptValidator{repo: repo}
}

// Validate checks goods receipt is complete
func (v *GoodsReceiptValidator) Validate(ctx context.Context, receiptID int64) ValidatorResult {
	result := ValidatorResult{
		Data: make(map[string]interface{}),
	}

	result.Data["receipt_id"] = receiptID

	// Placeholder for PO/goods receipt integration
	// In production, would validate against procurement module
	result.Valid = true
	result.Reason = "Goods receipt is eligible for quality acceptance"
	return result
}
