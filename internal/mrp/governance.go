package mrp

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrGovernanceNotEnforced = errors.New("governance enforcement mode is not active")
	ErrComplianceGateFailed  = errors.New("manufacturing compliance gate verification failed")
)

// GovernanceMode dictates whether policies are just warning logs or hard blocks.
type GovernanceMode string

const (
	GovernanceModeAudit   GovernanceMode = "AUDIT"
	GovernanceModeEnforce GovernanceMode = "ENFORCE"
)

// ComplianceGateService enforces controlled-record constraints on manufacturing runs.
type ComplianceGateService struct {
	pool *pgxpool.Pool
	mode GovernanceMode
}

func NewComplianceGateService(pool *pgxpool.Pool, mode GovernanceMode) *ComplianceGateService {
	return &ComplianceGateService{
		pool: pool,
		mode: mode,
	}
}

// SetMode allows activating ENFORCE mode dynamically.
func (s *ComplianceGateService) SetMode(mode GovernanceMode) {
	s.mode = mode
}

// VerifyProductionRun checks if a production run complies with all strict manufacturing governance policies (e.g., FDA/ISO controlled records).
func (s *ComplianceGateService) VerifyProductionRun(ctx context.Context, runID int64) error {
	if s.mode != GovernanceModeEnforce {
		// Log a warning in a real system: "Run %d passed without enforcement"
		return nil
	}

	// In a real database we would run complex cross-checks across bills of materials (BOM),
	// quality holds, calibration statuses of assigned machines, and operator certifications.
	
	// Mock implementation for the compliance gate:
	var state string
	var hasHolds bool
	var operatorCertified bool

	err := s.pool.QueryRow(ctx, `
		SELECT 
			state, 
			(SELECT COUNT(*) > 0 FROM qms_quality_holds WHERE run_id = mrp_runs.id AND released_at IS NULL),
			(SELECT COUNT(*) > 0 FROM mrp_operator_certs WHERE operator_id = mrp_runs.operator_id AND expires_at > NOW())
		FROM mrp_runs 
		WHERE id = $1
	`, runID).Scan(&state, &hasHolds, &operatorCertified)
	
	if err != nil {
		// If record not found or DB error, gate automatically fails in ENFORCE mode
		return err
	}

	if state != "DRAFT" && state != "APPROVAL" {
		return errors.New("production run must be in DRAFT or APPROVAL state for compliance verification")
	}

	if hasHolds {
		return errors.New("compliance gate failed: active quality holds exist on this run")
	}

	if !operatorCertified {
		return errors.New("compliance gate failed: operator certification is missing or expired")
	}

	// Gate passed
	return nil
}

// ApproveRun transitions a production run to POSTED only if the compliance gate clears.
func (s *ComplianceGateService) ApproveRun(ctx context.Context, runID int64, approverID int64) error {
	if err := s.VerifyProductionRun(ctx, runID); err != nil {
		return err
	}

	_, err := s.pool.Exec(ctx, `
		UPDATE mrp_runs 
		SET state = 'POSTED', approved_by = $1, approved_at = $2 
		WHERE id = $3 AND state = 'APPROVAL'
	`, approverID, time.Now().UTC(), runID)
	
	return err
}
