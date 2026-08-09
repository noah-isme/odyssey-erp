package mrp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	db  *pgx.Conn
	now func() time.Time
}

// NewComplianceGate creates a new compliance gate service
func NewComplianceGate(db *pgx.Conn) *ComplianceGate {
	return &ComplianceGate{
		db:  db,
		now: func() time.Time { return time.Now().UTC() },
	}
}

// WithClock makes compliance decisions deterministic in tests and keeps all
// expiry/retention calculations on one clock.
func (cg *ComplianceGate) WithClock(now func() time.Time) *ComplianceGate {
	if now != nil {
		cg.now = now
	}
	return cg
}

// DecideDecision executes the 8-step compliance gate flow
// Returns DecisionGrant on success, or ComplianceGateError on failure
func (cg *ComplianceGate) DecideDecision(ctx context.Context, req DecisionRequest) (*DecisionGrant, error) {
	if cg == nil || cg.db == nil || req.CompanyID <= 0 || req.RecordID <= 0 || req.ActorID <= 0 || strings.TrimSpace(req.RecordType) == "" || strings.TrimSpace(req.Action) == "" {
		return nil, &ComplianceGateError{Code: "INVALID_REQUEST", Message: "Company, record, action, and actor are required"}
	}
	if cg.now == nil {
		cg.now = func() time.Time { return time.Now().UTC() }
	}
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
	snapshot, err := cg.stepGenerateSnapshot(ctx, tx, req.CompanyID, req.RecordType, req.RecordID)
	if err != nil {
		return nil, err
	}

	// Step 3: Hash snapshot
	hash := snapshot.Hash

	// Step 4: Validate actor permission and approver roles
	if err := cg.stepValidateActor(ctx, tx, req, snapshot, policy); err != nil {
		return nil, err
	}

	// Step 5: Verify reauthentication challenge (if required)
	if policy.SignatureRequired {
		if err := cg.stepVerifyChallenge(ctx, tx, req, policy, snapshot); err != nil {
			return nil, err
		}
	}

	// Step 6: Store decision
	decision, err := cg.stepStoreDecision(ctx, tx, req, policy, snapshot, hash)
	if err != nil {
		return nil, err
	}

	// Step 7: The caller performs the governed business transition after this
	// grant. The immutable decision and audit evidence are committed here.

	// Step 8: Append immutable audit event
	correlationID := uuid.New()
	if err := cg.stepAuditEvent(ctx, tx, req.CompanyID, correlationID, decision, snapshot, req.Action, req.ActorID); err != nil {
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
		GrantedAt:       cg.now(),
	}, nil
}

// Step 1: Load and lock effective policy
func (cg *ComplianceGate) stepLoadPolicy(ctx context.Context, tx pgx.Tx, companyID int64, recordType, action string) (*PolicyVersion, error) {
	now := cg.now()

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
				"company_id":  companyID,
				"record_type": recordType,
				"action":      action,
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

// Step 2: Generate a server-side canonical snapshot of the locked record.
func (cg *ComplianceGate) stepGenerateSnapshot(ctx context.Context, tx pgx.Tx, companyID int64, recordType string, recordID int64) (*canonicalRecordSnapshot, error) {
	snapshot, err := loadCanonicalRecordSnapshot(ctx, tx, companyID, recordType, recordID)
	if err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// Step 4: Validate actor permission and approver roles
func (cg *ComplianceGate) stepValidateActor(ctx context.Context, tx pgx.Tx, req DecisionRequest, snapshot *canonicalRecordSnapshot, policy *PolicyVersion) error {
	if req.ActorID <= 0 || policy == nil || snapshot == nil {
		return &ComplianceGateError{
			Code:    "INVALID_ACTOR",
			Message: "Actor ID is invalid",
			Details: map[string]interface{}{"actor_id": req.ActorID},
		}
	}
	if len(policy.ApproverRoles) == 0 {
		return &ComplianceGateError{Code: "POLICY_ROLES_MISSING", Message: "Active policy has no approver roles configured"}
	}
	var active bool
	if err := tx.QueryRow(ctx, `SELECT is_active FROM users WHERE id = $1`, req.ActorID).Scan(&active); err != nil {
		return &ComplianceGateError{Code: "ACTOR_LOAD_FAILED", Message: "Failed to load actor", Details: map[string]interface{}{"error": err.Error()}}
	}
	if !active {
		return &ComplianceGateError{Code: "ACTOR_INACTIVE", Message: "Actor is inactive"}
	}
	roles := make([]string, 0, len(policy.ApproverRoles))
	for _, role := range policy.ApproverRoles {
		role = strings.ToLower(strings.TrimSpace(role))
		if role != "" {
			roles = append(roles, role)
		}
	}
	if len(roles) == 0 {
		return &ComplianceGateError{Code: "POLICY_ROLES_MISSING", Message: "Active policy has no approver roles configured"}
	}
	var hasRole bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM user_roles ur
			JOIN roles r ON r.id = ur.role_id
			WHERE ur.user_id = $1
			  AND LOWER(TRIM(r.name)) = ANY($2::text[])
		)`, req.ActorID, roles).Scan(&hasRole); err != nil {
		return &ComplianceGateError{Code: "ACTOR_ROLE_CHECK_FAILED", Message: "Failed to verify actor role", Details: map[string]interface{}{"error": err.Error()}}
	}
	if !hasRole {
		return &ComplianceGateError{Code: "ACTOR_ROLE_FORBIDDEN", Message: "Actor does not hold an approver role required by policy", Details: map[string]interface{}{"actor_id": req.ActorID, "required_roles": policy.ApproverRoles}}
	}
	if policy.SeparationOfDuties != nil && *policy.SeparationOfDuties {
		if snapshot.CreatedBy == nil {
			return &ComplianceGateError{Code: "SEPARATION_OF_DUTIES_UNVERIFIABLE", Message: "Policy requires separation of duties but the record has no creator"}
		}
		if *snapshot.CreatedBy == req.ActorID {
			return &ComplianceGateError{Code: "SEPARATION_OF_DUTIES_VIOLATION", Message: "Record creator cannot approve this decision"}
		}
	}
	for _, evidenceType := range policy.RequiredEvidence {
		if strings.TrimSpace(evidenceType) == "" {
			continue
		}
		if req.Evidence == nil {
			return &ComplianceGateError{Code: "REQUIRED_EVIDENCE_MISSING", Message: "Required decision evidence was not supplied", Details: map[string]interface{}{"evidence_type": evidenceType}}
		}
		if _, ok := req.Evidence[evidenceType]; !ok {
			return &ComplianceGateError{Code: "REQUIRED_EVIDENCE_MISSING", Message: "Required decision evidence was not supplied", Details: map[string]interface{}{"evidence_type": evidenceType}}
		}
	}
	return nil
}

// Step 5: Verify reauthentication challenge
func (cg *ComplianceGate) stepVerifyChallenge(ctx context.Context, tx pgx.Tx, req DecisionRequest, policy *PolicyVersion, snapshot *canonicalRecordSnapshot) error {
	if strings.TrimSpace(req.ChallengeID) == "" {
		return &ComplianceGateError{
			Code:    "CHALLENGE_MISSING",
			Message: "Signature challenge required but not provided",
			Details: map[string]interface{}{"record_id": req.RecordID},
		}
	}

	// Parse UUID
	challengeUUID, err := uuid.Parse(req.ChallengeID)
	if err != nil {
		return &ComplianceGateError{
			Code:    "INVALID_CHALLENGE_ID",
			Message: "Challenge ID is not a valid UUID",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	// Query challenge
	const query = `
		SELECT id, challenge_id, policy_version_id, company_id, record_type, signer_id,
		       record_id, record_version, record_hash, expiry, reauthentication_required,
		       used, created_at
		FROM signature_challenges
		WHERE challenge_id = $1
		FOR UPDATE
	`

	row := tx.QueryRow(ctx, query, challengeUUID)
	var challenge SignatureChallenge
	var companyID int64
	var recordType string
	var signerID int64
	var recordHash *string

	err = row.Scan(
		&challenge.ID, &challenge.ChallengeID, &challenge.PolicyVersionID, &companyID,
		&recordType, &signerID, &challenge.RecordID, &challenge.RecordVersion, &recordHash,
		&challenge.Expiry, &challenge.ReauthenticationRequired, &challenge.Used, &challenge.CreatedAt,
	)

	if err == pgx.ErrNoRows {
		return &ComplianceGateError{
			Code:    "CHALLENGE_NOT_FOUND",
			Message: "Signature challenge not found",
			Details: map[string]interface{}{"challenge_id": req.ChallengeID},
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
	if !cg.now().Before(challenge.Expiry) {
		return &ComplianceGateError{
			Code:    "CHALLENGE_EXPIRED",
			Message: "Signature challenge has expired",
			Details: map[string]interface{}{"challenge_id": req.ChallengeID, "expiry": challenge.Expiry},
		}
	}

	// Check if already used
	if challenge.Used {
		return &ComplianceGateError{
			Code:    "CHALLENGE_REUSED",
			Message: "Signature challenge has already been used",
			Details: map[string]interface{}{"challenge_id": req.ChallengeID},
		}
	}

	// Check record ID matches
	if challenge.RecordID != req.RecordID || companyID != req.CompanyID || signerID != req.ActorID || !recordTypesEqual(recordType, snapshot.RecordType) {
		return &ComplianceGateError{
			Code:    "CHALLENGE_MISMATCH",
			Message: "Challenge record ID does not match request record ID",
			Details: map[string]interface{}{"challenge_record_id": challenge.RecordID, "request_record_id": req.RecordID},
		}
	}
	if challenge.PolicyVersionID != policy.ID || challenge.RecordVersion == nil || *challenge.RecordVersion != snapshot.RecordVersion || recordHash == nil || *recordHash != snapshot.Hash {
		return &ComplianceGateError{Code: "CHALLENGE_SNAPSHOT_MISMATCH", Message: "Signature challenge does not match the current record snapshot"}
	}
	if !challenge.ReauthenticationRequired {
		return &ComplianceGateError{Code: "CHALLENGE_REAUTH_REQUIRED", Message: "Signature challenge was not configured for reauthentication"}
	}
	_, valid, err := verifyUserReauthentication(ctx, tx, req.ActorID, req.ReauthToken)
	if err != nil {
		return &ComplianceGateError{Code: "REAUTHENTICATION_CHECK_FAILED", Message: "Failed to verify reauthentication", Details: map[string]interface{}{"error": err.Error()}}
	}
	if !valid {
		return &ComplianceGateError{Code: "REAUTHENTICATION_FAILED", Message: "Reauthentication failed"}
	}

	// Mark challenge as used
	const updateQuery = `UPDATE signature_challenges SET used = TRUE, reauthentication_method = 'VERIFIED', reauthenticated_at = $2 WHERE id = $1 AND used = FALSE`
	result, err := tx.Exec(ctx, updateQuery, challenge.ID, cg.now())
	if err != nil {
		return &ComplianceGateError{
			Code:    "CHALLENGE_UPDATE_FAILED",
			Message: "Failed to mark challenge as used",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}
	if result.RowsAffected() != 1 {
		return &ComplianceGateError{Code: "CHALLENGE_REUSED", Message: "Signature challenge has already been used"}
	}

	return nil
}

// Step 6: Store decision
func (cg *ComplianceGate) stepStoreDecision(ctx context.Context, tx pgx.Tx, req DecisionRequest, policy *PolicyVersion, snapshot *canonicalRecordSnapshot, hash string) (*ComplianceDecision, error) {
	decisionID := uuid.New()
	snapshotID, err := persistCanonicalSnapshot(ctx, tx, *snapshot, req.ActorID, policy.RetentionPeriodDays)
	if err != nil {
		return nil, &ComplianceGateError{Code: "SNAPSHOT_STORE_FAILED", Message: "Failed to persist immutable record snapshot", Details: map[string]interface{}{"error": err.Error()}}
	}

	const query = `
		INSERT INTO compliance_decisions (
			company_id, policy_version_id, record_type, record_id, action,
			actor_id, reason, decision_id, record_version, record_hash, snapshot_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, company_id, policy_version_id, record_type, record_id, action,
		          actor_id, reason, decision_id, record_version, record_hash, snapshot_id, created_at
	`

	row := tx.QueryRow(ctx, query,
		req.CompanyID, policy.ID, req.RecordType, req.RecordID, req.Action,
		req.ActorID, req.Reason, decisionID, snapshot.RecordVersion, hash, snapshotID,
	)

	var decision ComplianceDecision
	err = row.Scan(
		&decision.ID, &decision.CompanyID, &decision.PolicyVersionID,
		&decision.RecordType, &decision.RecordID, &decision.Action,
		&decision.ActorID, &decision.Reason, &decision.DecisionID,
		&decision.RecordVersion, &decision.RecordHash, &decision.SnapshotID, &decision.CreatedAt,
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
func (cg *ComplianceGate) stepAuditEvent(ctx context.Context, tx pgx.Tx, companyID int64, correlationID uuid.UUID, decision *ComplianceDecision, snapshot *canonicalRecordSnapshot, action string, actorID int64) error {
	const query = `
		INSERT INTO audit_events (
			company_id, correlation_id, causation_id, decision_id,
			entity_type, entity_id, action, actor_id, details
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	details := map[string]interface{}{
		"entity_type":    decision.RecordType,
		"entity_id":      decision.RecordID,
		"decision_id":    decision.DecisionID.String(),
		"snapshot_id":    decision.SnapshotID,
		"record_version": snapshot.RecordVersion,
		"record_hash":    snapshot.Hash,
	}
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return &ComplianceGateError{Code: "AUDIT_EVENT_FAILED", Message: "Failed to serialize compliance audit details", Details: map[string]interface{}{"error": err.Error()}}
	}

	_, err = tx.Exec(ctx, query,
		companyID, correlationID, nil, decision.ID,
		decision.RecordType, decision.RecordID, action, actorID, detailsJSON,
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
