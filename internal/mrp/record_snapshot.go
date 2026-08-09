package mrp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// RecordSnapshotService handles generation and management of record snapshots
type RecordSnapshotService struct {
	db *pgx.Conn
}

// NewRecordSnapshotService creates a new snapshot service
func NewRecordSnapshotService(db *pgx.Conn) *RecordSnapshotService {
	return &RecordSnapshotService{
		db: db,
	}
}

// SnapshotResult represents the result of snapshot generation
type SnapshotResult struct {
	CanonicalJSON []byte // Canonical JSON representation
	Hash          string // SHA-256 hex string
	Version       string // Version identifier
	Timestamp     time.Time
}

// SnapshotBOM generates a canonical snapshot of a BOM
func (rss *RecordSnapshotService) SnapshotBOM(ctx context.Context, bomID int64) (*SnapshotResult, error) {
	const query = `
		SELECT id, company_id, product_id, version, revision_status, 
		       change_reason, active, scrap_pct, effective_from, effective_to,
		       approved_by, approved_at, created_by, created_at, updated_at
		FROM bom_revisions
		WHERE id = $1
	`

	row := rss.db.QueryRow(ctx, query, bomID)

	type bomData struct {
		ID             int64      `json:"id"`
		CompanyID      int64      `json:"company_id"`
		ProductID      int64      `json:"product_id"`
		Version        string     `json:"version"`
		RevisionStatus string     `json:"revision_status"`
		ChangeReason   string     `json:"change_reason"`
		Active         bool       `json:"active"`
		ScrapPct       float64    `json:"scrap_pct"`
		EffectiveFrom  time.Time  `json:"effective_from"`
		EffectiveTo    *time.Time `json:"effective_to"`
		ApprovedBy     *int64     `json:"approved_by"`
		ApprovedAt     *time.Time `json:"approved_at"`
		CreatedBy      int64      `json:"created_by"`
		CreatedAt      time.Time  `json:"created_at"`
		UpdatedAt      time.Time  `json:"updated_at"`
	}

	var bom bomData
	if err := row.Scan(
		&bom.ID, &bom.CompanyID, &bom.ProductID, &bom.Version,
		&bom.RevisionStatus, &bom.ChangeReason, &bom.Active,
		&bom.ScrapPct, &bom.EffectiveFrom, &bom.EffectiveTo,
		&bom.ApprovedBy, &bom.ApprovedAt, &bom.CreatedBy,
		&bom.CreatedAt, &bom.UpdatedAt,
	); err != nil {
		return nil, &ComplianceGateError{
			Code:    "BOM_SNAPSHOT_FAILED",
			Message: "Failed to snapshot BOM",
			Details: map[string]interface{}{"error": err.Error(), "bom_id": bomID},
		}
	}

	// Fetch BOM lines
	linesQuery := `
		SELECT component_product_id, quantity, scrap_pct
		FROM bom_lines
		WHERE bom_revision_id = $1
		ORDER BY id
	`

	linesRows, err := rss.db.Query(ctx, linesQuery, bomID)
	if err != nil {
		return nil, &ComplianceGateError{
			Code:    "BOM_LINES_QUERY_FAILED",
			Message: "Failed to query BOM lines",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}
	defer linesRows.Close()

	type bomLine struct {
		ComponentProductID int64   `json:"component_product_id"`
		Quantity           float64 `json:"quantity"`
		ScrapPct           float64 `json:"scrap_pct"`
	}

	var lines []bomLine
	for linesRows.Next() {
		var line bomLine
		if err := linesRows.Scan(&line.ComponentProductID, &line.Quantity, &line.ScrapPct); err != nil {
			return nil, &ComplianceGateError{
				Code:    "BOM_LINE_SCAN_FAILED",
				Message: "Failed to scan BOM line",
				Details: map[string]interface{}{"error": err.Error()},
			}
		}
		lines = append(lines, line)
	}

	// Create canonical snapshot
	snapshot := map[string]interface{}{
		"record_type": "BOM",
		"bom":         bom,
		"lines":       lines,
	}

	// Marshal to canonical JSON (sorted keys)
	jsonData, err := json.Marshal(snapshot)
	if err != nil {
		return nil, &ComplianceGateError{
			Code:    "SNAPSHOT_MARSHAL_FAILED",
			Message: "Failed to marshal snapshot",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	// Calculate hash
	hash := sha256.Sum256(jsonData)
	hashHex := hex.EncodeToString(hash[:])

	return &SnapshotResult{
		CanonicalJSON: jsonData,
		Hash:          hashHex,
		Version:       bom.Version,
		Timestamp:     time.Now(),
	}, nil
}

// SnapshotWorkOrder generates a canonical snapshot of a work order
func (rss *RecordSnapshotService) SnapshotWorkOrder(ctx context.Context, woID int64) (*SnapshotResult, error) {
	const query = `
		SELECT id, company_id, product_id, bom_id, warehouse_id,
		       planned_qty, completed_qty, status, 
		       scheduled_start, scheduled_end,
		       created_by, created_at, updated_at
		FROM work_orders
		WHERE id = $1
	`

	row := rss.db.QueryRow(ctx, query, woID)

	type woData struct {
		ID             int64      `json:"id"`
		CompanyID      int64      `json:"company_id"`
		ProductID      int64      `json:"product_id"`
		BomID          int64      `json:"bom_id"`
		WarehouseID    int64      `json:"warehouse_id"`
		PlannedQty     float64    `json:"planned_qty"`
		CompletedQty   float64    `json:"completed_qty"`
		Status         string     `json:"status"`
		ScheduledStart *time.Time `json:"scheduled_start"`
		ScheduledEnd   *time.Time `json:"scheduled_end"`
		CreatedBy      int64      `json:"created_by"`
		CreatedAt      time.Time  `json:"created_at"`
		UpdatedAt      time.Time  `json:"updated_at"`
	}

	var wo woData
	if err := row.Scan(
		&wo.ID, &wo.CompanyID, &wo.ProductID, &wo.BomID, &wo.WarehouseID,
		&wo.PlannedQty, &wo.CompletedQty, &wo.Status,
		&wo.ScheduledStart, &wo.ScheduledEnd,
		&wo.CreatedBy, &wo.CreatedAt, &wo.UpdatedAt,
	); err != nil {
		return nil, &ComplianceGateError{
			Code:    "WO_SNAPSHOT_FAILED",
			Message: "Failed to snapshot work order",
			Details: map[string]interface{}{"error": err.Error(), "work_order_id": woID},
		}
	}

	// Create canonical snapshot
	snapshot := map[string]interface{}{
		"record_type": "WorkOrder",
		"work_order":  wo,
	}

	jsonData, err := json.Marshal(snapshot)
	if err != nil {
		return nil, &ComplianceGateError{
			Code:    "SNAPSHOT_MARSHAL_FAILED",
			Message: "Failed to marshal snapshot",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	hash := sha256.Sum256(jsonData)
	hashHex := hex.EncodeToString(hash[:])

	return &SnapshotResult{
		CanonicalJSON: jsonData,
		Hash:          hashHex,
		Version:       wo.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Timestamp:     time.Now(),
	}, nil
}

// SnapshotOperation generates a canonical snapshot of an operation
func (rss *RecordSnapshotService) SnapshotOperation(ctx context.Context, opID int64) (*SnapshotResult, error) {
	const query = `
		SELECT id, company_id, work_order_id, routing_operation_id, work_center_id,
		       sequence, code, name, status,
		       planned_setup_minutes, planned_run_minutes,
		       actual_setup_minutes, actual_run_minutes,
		       good_quantity, scrap_quantity, operator_id,
		       scheduled_start, scheduled_end, schedule_manual,
		       created_at, updated_at
		FROM work_order_operations
		WHERE id = $1
	`

	row := rss.db.QueryRow(ctx, query, opID)

	type opData struct {
		ID                  int64      `json:"id"`
		CompanyID           int64      `json:"company_id"`
		WorkOrderID         int64      `json:"work_order_id"`
		RoutingOperationID  int64      `json:"routing_operation_id"`
		WorkCenterID        int64      `json:"work_center_id"`
		Sequence            int        `json:"sequence"`
		Code                string     `json:"code"`
		Name                string     `json:"name"`
		Status              string     `json:"status"`
		PlannedSetupMinutes float64    `json:"planned_setup_minutes"`
		PlannedRunMinutes   float64    `json:"planned_run_minutes"`
		ActualSetupMinutes  *float64   `json:"actual_setup_minutes"`
		ActualRunMinutes    *float64   `json:"actual_run_minutes"`
		GoodQuantity        *float64   `json:"good_quantity"`
		ScrapQuantity       *float64   `json:"scrap_quantity"`
		OperatorID          *int64     `json:"operator_id"`
		ScheduledStart      *time.Time `json:"scheduled_start"`
		ScheduledEnd        *time.Time `json:"scheduled_end"`
		ScheduleManual      bool       `json:"schedule_manual"`
		CreatedAt           time.Time  `json:"created_at"`
		UpdatedAt           time.Time  `json:"updated_at"`
	}

	var op opData
	if err := row.Scan(
		&op.ID, &op.CompanyID, &op.WorkOrderID, &op.RoutingOperationID, &op.WorkCenterID,
		&op.Sequence, &op.Code, &op.Name, &op.Status,
		&op.PlannedSetupMinutes, &op.PlannedRunMinutes,
		&op.ActualSetupMinutes, &op.ActualRunMinutes,
		&op.GoodQuantity, &op.ScrapQuantity, &op.OperatorID,
		&op.ScheduledStart, &op.ScheduledEnd, &op.ScheduleManual,
		&op.CreatedAt, &op.UpdatedAt,
	); err != nil {
		return nil, &ComplianceGateError{
			Code:    "OPERATION_SNAPSHOT_FAILED",
			Message: "Failed to snapshot operation",
			Details: map[string]interface{}{"error": err.Error(), "operation_id": opID},
		}
	}

	// Create canonical snapshot
	snapshot := map[string]interface{}{
		"record_type": "Operation",
		"operation":   op,
	}

	jsonData, err := json.Marshal(snapshot)
	if err != nil {
		return nil, &ComplianceGateError{
			Code:    "SNAPSHOT_MARSHAL_FAILED",
			Message: "Failed to marshal snapshot",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	hash := sha256.Sum256(jsonData)
	hashHex := hex.EncodeToString(hash[:])

	return &SnapshotResult{
		CanonicalJSON: jsonData,
		Hash:          hashHex,
		Version:       op.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Timestamp:     time.Now(),
	}, nil
}

// SnapshotHold generates a canonical snapshot of a quality hold
func (rss *RecordSnapshotService) SnapshotHold(ctx context.Context, holdID int64) (*SnapshotResult, error) {
	const query = `
		SELECT id, company_id, inspection_id, record_type, record_id,
		       status, created_by, created_at, updated_at
		FROM quality_holds
		WHERE id = $1
	`

	row := rss.db.QueryRow(ctx, query, holdID)

	type holdData struct {
		ID           int64     `json:"id"`
		CompanyID    int64     `json:"company_id"`
		InspectionID *int64    `json:"inspection_id"`
		RecordType   *string   `json:"record_type"`
		RecordID     *int64    `json:"record_id"`
		Status       string    `json:"status"`
		CreatedBy    int64     `json:"created_by"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
	}

	var hold holdData
	if err := row.Scan(
		&hold.ID, &hold.CompanyID, &hold.InspectionID, &hold.RecordType, &hold.RecordID,
		&hold.Status, &hold.CreatedBy, &hold.CreatedAt, &hold.UpdatedAt,
	); err != nil {
		return nil, &ComplianceGateError{
			Code:    "HOLD_SNAPSHOT_FAILED",
			Message: "Failed to snapshot hold",
			Details: map[string]interface{}{"error": err.Error(), "hold_id": holdID},
		}
	}

	snapshot := map[string]interface{}{
		"record_type": "Hold",
		"hold":        hold,
	}

	jsonData, err := json.Marshal(snapshot)
	if err != nil {
		return nil, &ComplianceGateError{
			Code:    "SNAPSHOT_MARSHAL_FAILED",
			Message: "Failed to marshal snapshot",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	hash := sha256.Sum256(jsonData)
	hashHex := hex.EncodeToString(hash[:])

	return &SnapshotResult{
		CanonicalJSON: jsonData,
		Hash:          hashHex,
		Version:       hold.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Timestamp:     time.Now(),
	}, nil
}

// SnapshotNCR generates a canonical snapshot of an NCR
func (rss *RecordSnapshotService) SnapshotNCR(ctx context.Context, ncrID int64) (*SnapshotResult, error) {
	const query = `
		SELECT id, company_id, number, status, created_by, created_at, updated_at
		FROM quality_ncrs
		WHERE id = $1
	`

	row := rss.db.QueryRow(ctx, query, ncrID)

	type ncrData struct {
		ID        int64     `json:"id"`
		CompanyID int64     `json:"company_id"`
		Number    string    `json:"number"`
		Status    string    `json:"status"`
		CreatedBy int64     `json:"created_by"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}

	var ncr ncrData
	if err := row.Scan(
		&ncr.ID, &ncr.CompanyID, &ncr.Number, &ncr.Status, &ncr.CreatedBy, &ncr.CreatedAt, &ncr.UpdatedAt,
	); err != nil {
		return nil, &ComplianceGateError{
			Code:    "NCR_SNAPSHOT_FAILED",
			Message: "Failed to snapshot NCR",
			Details: map[string]interface{}{"error": err.Error(), "ncr_id": ncrID},
		}
	}

	snapshot := map[string]interface{}{
		"record_type": "NCR",
		"ncr":         ncr,
	}

	jsonData, err := json.Marshal(snapshot)
	if err != nil {
		return nil, &ComplianceGateError{
			Code:    "SNAPSHOT_MARSHAL_FAILED",
			Message: "Failed to marshal snapshot",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	hash := sha256.Sum256(jsonData)
	hashHex := hex.EncodeToString(hash[:])

	return &SnapshotResult{
		CanonicalJSON: jsonData,
		Hash:          hashHex,
		Version:       ncr.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Timestamp:     time.Now(),
	}, nil
}

// SnapshotCAPA generates a canonical snapshot of a CAPA
func (rss *RecordSnapshotService) SnapshotCAPA(ctx context.Context, capaID int64) (*SnapshotResult, error) {
	const query = `
		SELECT id, company_id, number, status, created_by, created_at, updated_at
		FROM quality_capas
		WHERE id = $1
	`

	row := rss.db.QueryRow(ctx, query, capaID)

	type capaData struct {
		ID        int64     `json:"id"`
		CompanyID int64     `json:"company_id"`
		Number    string    `json:"number"`
		Status    string    `json:"status"`
		CreatedBy int64     `json:"created_by"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}

	var capa capaData
	if err := row.Scan(
		&capa.ID, &capa.CompanyID, &capa.Number, &capa.Status, &capa.CreatedBy, &capa.CreatedAt, &capa.UpdatedAt,
	); err != nil {
		return nil, &ComplianceGateError{
			Code:    "CAPA_SNAPSHOT_FAILED",
			Message: "Failed to snapshot CAPA",
			Details: map[string]interface{}{"error": err.Error(), "capa_id": capaID},
		}
	}

	snapshot := map[string]interface{}{
		"record_type": "CAPA",
		"capa":        capa,
	}

	jsonData, err := json.Marshal(snapshot)
	if err != nil {
		return nil, &ComplianceGateError{
			Code:    "SNAPSHOT_MARSHAL_FAILED",
			Message: "Failed to marshal snapshot",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	hash := sha256.Sum256(jsonData)
	hashHex := hex.EncodeToString(hash[:])

	return &SnapshotResult{
		CanonicalJSON: jsonData,
		Hash:          hashHex,
		Version:       capa.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Timestamp:     time.Now(),
	}, nil
}

// VerifyHash verifies that a snapshot matches its hash
func (rss *RecordSnapshotService) VerifyHash(snapshotJSON []byte, expectedHash string) bool {
	canonical, err := canonicalizeJSON(snapshotJSON)
	if err != nil {
		return false
	}
	return hashCanonicalJSON(canonical) == expectedHash
}
