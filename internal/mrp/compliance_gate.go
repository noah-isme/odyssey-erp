package mrp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ComplianceGateError represents an error from the compliance gate
type ComplianceGateError struct {
	Code    string
	Message string
	Details map[string]interface{}
}

func (e *ComplianceGateError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// ComplianceGate orchestrates the 8-step governed decision process
type ComplianceGate struct {
	db *pgx.Conn
	// Dependencies would be injected here in a real implementation
	// authService, auditService, etc.
}

// NewComplianceGate creates a new compliance gate service
func NewComplianceGate(db *pgx.Conn) *ComplianceGate {
	return &ComplianceGate{
		db: db,
	}
}

// DecideDecision executes the 8-step compliance gate flow
// Returns DecisionGrant on success, or ComplianceGateError on failure
func (cg *ComplianceGate) DecideDecision(ctx context.Context, req DecisionRequest) (*DecisionGrant, error) {
	// Start transaction
	tx, err := cg.db.Begin(ctx)
	if err != nil {
		return nil, &ComplianceGateError{
			Code:    "TX_BEGIN_FAILED",
			Message: "Failed to begin transaction",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}
	defer func() { _ = tx.Rollback(ctx) }() // Will be no-op if tx is committed

	// Step 1: Lock & load effective policy
	policy, err := cg.stepLoadPolicy(ctx, tx, req.CompanyID, req.RecordType, req.Action)
	if err != nil {
		return nil, err
	}

	// Step 2: Generate server-side canonical snapshot
	snapshot, err := cg.stepGenerateSnapshot(ctx, tx, req.RecordType, req.RecordID)
	if err != nil {
		return nil, err
	}

	// Step 3: Hash snapshot
	hash := cg.stepHashSnapshot(snapshot)

	// Step 4: Validate actor permission and approver roles
	if err := cg.stepValidateActor(ctx, tx, req.ActorID, policy); err != nil {
		return nil, err
	}

	// Step 5: Verify reauthentication challenge (if required)
	if policy.SignatureRequired {
		if err := cg.stepVerifyChallenge(ctx, tx, req.ChallengeID, req.RecordID); err != nil {
			return nil, err
		}
	}

	// Step 6: Store decision
	decision, err := cg.stepStoreDecision(ctx, tx, req, policy, snapshot, hash)
	if err != nil {
		return nil, err
	}

	// Step 7: Execute governed state transition
	// This is delegated to the caller but happens in the same transaction
	// The caller will execute their business logic and must call tx.Commit()
	// For now, we create the audit event

	// Step 8: Append immutable audit event
	correlationID := uuid.New()
	if err := cg.stepAuditEvent(ctx, tx, req.CompanyID, correlationID, decision.DecisionID, req.RecordType, req.RecordID, req.Action, req.ActorID); err != nil {
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, &ComplianceGateError{
			Code:    "TX_COMMIT_FAILED",
			Message: "Failed to commit transaction",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	recordVersion := ""
	if decision.RecordVersion != nil {
		recordVersion = *decision.RecordVersion
	}

	return &DecisionGrant{
		PolicyVersionID: policy.ID,
		RecordVersion:   recordVersion,
		RecordHash:      hash,
		DecisionID:      decision.DecisionID,
		GrantedAt:       time.Now(),
	}, nil
}

// Step 1: Load and lock effective policy
func (cg *ComplianceGate) stepLoadPolicy(ctx context.Context, tx pgx.Tx, companyID int64, recordType, action string) (*PolicyVersion, error) {
	now := time.Now()

	// Query with FOR UPDATE to lock the row
	const query = `
		SELECT id, company_id, record_type, decision_name, effective_from, effective_to,
		       enforcement_mode, signature_required, approver_roles, separation_of_duties,
		       required_evidence, retention_period_days, version, status, created_at, created_by
		FROM policy_versions
		WHERE company_id = $1
		  AND record_type = $2
		  AND decision_name = $3
		  AND status = 'ACTIVE'
		  AND effective_from <= $4::timestamptz
		  AND (effective_to IS NULL OR effective_to > $4::timestamptz)
		ORDER BY effective_from DESC
		LIMIT 1
		FOR UPDATE
	`

	row := tx.QueryRow(ctx, query, companyID, recordType, action, now)
	var policy PolicyVersion

	err := row.Scan(
		&policy.ID, &policy.CompanyID, &policy.RecordType, &policy.DecisionName,
		&policy.EffectiveFrom, &policy.EffectiveTo, &policy.EnforcementMode,
		&policy.SignatureRequired, &policy.ApproverRoles, &policy.SeparationOfDuties,
		&policy.RequiredEvidence, &policy.RetentionPeriodDays, &policy.Version,
		&policy.Status, &policy.CreatedAt, &policy.CreatedBy,
	)

	if err == pgx.ErrNoRows {
		return nil, &ComplianceGateError{
			Code:    "NO_POLICY_FOUND",
			Message: fmt.Sprintf("No active policy found for %s/%s", recordType, action),
			Details: map[string]interface{}{
				"company_id": companyID,
				"record_type": recordType,
				"action": action,
			},
		}
	}
	if err != nil {
		return nil, &ComplianceGateError{
			Code:    "POLICY_LOAD_FAILED",
			Message: "Failed to load policy",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	// Check enforcement mode
	if policy.EnforcementMode == EnforcementModeDisabled {
		return nil, &ComplianceGateError{
			Code:    "POLICY_DISABLED",
			Message: "Policy is disabled for this decision",
			Details: map[string]interface{}{"policy_id": policy.ID},
		}
	}

	return &policy, nil
}

// Step 2: Generate server-side canonical snapshot
func (cg *ComplianceGate) stepGenerateSnapshot(ctx context.Context, tx pgx.Tx, recordType string, recordID int64) (*json.RawMessage, error) {
	// This would be extended to handle different record types
	// For now, return a placeholder that would be implemented per record type
	
	snapshot := map[string]interface{}{
		"record_type": recordType,
		"record_id":   recordID,
		"timestamp":   time.Now(),
		// Full record data would be fetched from DB based on recordType and recordID
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil, &ComplianceGateError{
			Code:    "SNAPSHOT_MARSHAL_FAILED",
			Message: "Failed to marshal snapshot to JSON",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	raw := json.RawMessage(data)
	return &raw, nil
}

// Step 3: Hash snapshot (SHA-256)
func (cg *ComplianceGate) stepHashSnapshot(snapshot *json.RawMessage) string {
	h := sha256.New()
	h.Write(*snapshot)
	return hex.EncodeToString(h.Sum(nil))
}

// Step 4: Validate actor permission and approver roles
func (cg *ComplianceGate) stepValidateActor(ctx context.Context, tx pgx.Tx, actorID int64, policy *PolicyVersion) error {
	// This would query user roles and check against policy.ApproverRoles
	// Placeholder implementation
	
	if actorID <= 0 {
		return &ComplianceGateError{
			Code:    "INVALID_ACTOR",
			Message: "Actor ID is invalid",
			Details: map[string]interface{}{"actor_id": actorID},
		}
	}

	// TODO: Implement actual role validation
	// Check if actor has one of the required approver roles

	return nil
}

// Step 5: Verify reauthentication challenge
func (cg *ComplianceGate) stepVerifyChallenge(ctx context.Context, tx pgx.Tx, challengeID string, recordID int64) error {
	if challengeID == "" {
		return &ComplianceGateError{
			Code:    "CHALLENGE_MISSING",
			Message: "Signature challenge required but not provided",
			Details: map[string]interface{}{"record_id": recordID},
		}
	}

	// Parse UUID
	challengeUUID, err := uuid.Parse(challengeID)
	if err != nil {
		return &ComplianceGateError{
			Code:    "INVALID_CHALLENGE_ID",
			Message: "Challenge ID is not a valid UUID",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	// Query challenge
	const query = `
		SELECT id, challenge_id, policy_version_id, record_id, record_version,
		       expiry, reauthentication_required, used, created_at
		FROM signature_challenges
		WHERE challenge_id = $1
		FOR UPDATE
	`

	row := tx.QueryRow(ctx, query, challengeUUID)
	var challenge SignatureChallenge

	err = row.Scan(
		&challenge.ID, &challenge.ChallengeID, &challenge.PolicyVersionID,
		&challenge.RecordID, &challenge.RecordVersion, &challenge.Expiry,
		&challenge.ReauthenticationRequired, &challenge.Used, &challenge.CreatedAt,
	)

	if err == pgx.ErrNoRows {
		return &ComplianceGateError{
			Code:    "CHALLENGE_NOT_FOUND",
			Message: "Signature challenge not found",
			Details: map[string]interface{}{"challenge_id": challengeID},
		}
	}
	if err != nil {
		return &ComplianceGateError{
			Code:    "CHALLENGE_LOAD_FAILED",
			Message: "Failed to load challenge",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	// Check expiry
	if time.Now().After(challenge.Expiry) {
		return &ComplianceGateError{
			Code:    "CHALLENGE_EXPIRED",
			Message: "Signature challenge has expired",
			Details: map[string]interface{}{"challenge_id": challengeID, "expiry": challenge.Expiry},
		}
	}

	// Check if already used
	if challenge.Used {
		return &ComplianceGateError{
			Code:    "CHALLENGE_REUSED",
			Message: "Signature challenge has already been used",
			Details: map[string]interface{}{"challenge_id": challengeID},
		}
	}

	// Check record ID matches
	if challenge.RecordID != recordID {
		return &ComplianceGateError{
			Code:    "CHALLENGE_MISMATCH",
			Message: "Challenge record ID does not match request record ID",
			Details: map[string]interface{}{"challenge_record_id": challenge.RecordID, "request_record_id": recordID},
		}
	}

	// Mark challenge as used
	const updateQuery = `UPDATE signature_challenges SET used = TRUE WHERE id = $1`
	_, err = tx.Exec(ctx, updateQuery, challenge.ID)
	if err != nil {
		return &ComplianceGateError{
			Code:    "CHALLENGE_UPDATE_FAILED",
			Message: "Failed to mark challenge as used",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	return nil
}

// Step 6: Store decision
func (cg *ComplianceGate) stepStoreDecision(ctx context.Context, tx pgx.Tx, req DecisionRequest, policy *PolicyVersion, snapshot *json.RawMessage, hash string) (*ComplianceDecision, error) {
	decisionID := uuid.New()
	version := "1" // TODO: increment based on record

	const query = `
		INSERT INTO compliance_decisions (
			company_id, policy_version_id, record_type, record_id, action,
			actor_id, reason, decision_id, record_version, record_hash
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, company_id, policy_version_id, record_type, record_id, action,
		          actor_id, reason, decision_id, record_version, record_hash, created_at
	`

	row := tx.QueryRow(ctx, query,
		req.CompanyID, policy.ID, req.RecordType, req.RecordID, req.Action,
		req.ActorID, req.Reason, decisionID, version, hash,
	)

	var decision ComplianceDecision
	err := row.Scan(
		&decision.ID, &decision.CompanyID, &decision.PolicyVersionID,
		&decision.RecordType, &decision.RecordID, &decision.Action,
		&decision.ActorID, &decision.Reason, &decision.DecisionID,
		&decision.RecordVersion, &decision.RecordHash, &decision.CreatedAt,
	)

	if err != nil {
		return nil, &ComplianceGateError{
			Code:    "DECISION_STORE_FAILED",
			Message: "Failed to store decision",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	return &decision, nil
}

// Step 8: Append immutable audit event
func (cg *ComplianceGate) stepAuditEvent(ctx context.Context, tx pgx.Tx, companyID int64, correlationID, decisionID uuid.UUID, entityType string, entityID int64, action string, actorID int64) error {
	const query = `
		INSERT INTO audit_events (
			company_id, correlation_id, causation_id, decision_id,
			entity_type, entity_id, action, actor_id, details
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	details := map[string]interface{}{
		"entity_type": entityType,
		"entity_id":   entityID,
	}
	detailsJSON, _ := json.Marshal(details)

	_, err := tx.Exec(ctx, query,
		companyID, correlationID, nil, decisionID,
		entityType, entityID, action, actorID, detailsJSON,
	)

	if err != nil {
		return &ComplianceGateError{
			Code:    "AUDIT_EVENT_FAILED",
			Message: "Failed to append audit event",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	return nil
}
