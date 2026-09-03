package mrp

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SignatureChallengeService handles one-time, actor-bound signature
// challenges. Credentials are checked against users inside the transaction;
// neither credentials nor client-supplied record JSON are persisted.
type SignatureChallengeService struct {
	db           *pgx.Conn
	challengeTTL time.Duration
	now          func() time.Time
}

// NewSignatureChallengeService creates a new challenge service.
func NewSignatureChallengeService(db *pgx.Conn) *SignatureChallengeService {
	return &SignatureChallengeService{
		db:           db,
		challengeTTL: 5 * time.Minute,
		now:          func() time.Time { return time.Now().UTC() },
	}
}

// WithClock makes expiry behavior deterministic in tests.
func (scs *SignatureChallengeService) WithClock(now func() time.Time) *SignatureChallengeService {
	if now != nil {
		scs.now = now
	}
	return scs
}

// GenerateChallengeInput represents the input to generate a challenge.
type GenerateChallengeInput struct {
	CompanyID       int64
	PolicyVersionID int64
	RecordType      string
	RecordID        int64
	RecordVersion   string
	ActorID         int64
	RequiresReauth  bool
}

// GeneratedChallenge represents a generated challenge.
type GeneratedChallenge struct {
	ChallengeID   string
	ExpiresAt     time.Time
	ExpiresIn     time.Duration
	RecordVersion string
	RecordHash    string
}

// GenerateChallenge creates a new one-time signature challenge bound to the
// active policy, actor, company, and current server snapshot.
func (scs *SignatureChallengeService) GenerateChallenge(ctx context.Context, input GenerateChallengeInput) (*GeneratedChallenge, error) {
	if scs == nil || scs.db == nil || input.CompanyID <= 0 || input.PolicyVersionID <= 0 || input.RecordID <= 0 || input.ActorID <= 0 || strings.TrimSpace(input.RecordType) == "" || !input.RequiresReauth {
		return nil, &ComplianceGateError{Code: "INVALID_CHALLENGE_REQUEST", Message: "Company, policy, record, actor, and reauthentication are required"}
	}
	if scs.now == nil {
		scs.now = func() time.Time { return time.Now().UTC() }
	}
	normalizedType, err := normalizeRecordType(input.RecordType)
	if err != nil {
		return nil, &ComplianceGateError{Code: "INVALID_RECORD_TYPE", Message: err.Error()}
	}

	tx, err := scs.db.Begin(ctx)
	if err != nil {
		return nil, &ComplianceGateError{Code: "TX_BEGIN_FAILED", Message: "Failed to begin challenge transaction", Details: map[string]interface{}{"error": err.Error()}}
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var policyCompany int64
	var policyRecordType string
	var signatureRequired bool
	var policyStatus string
	if err := tx.QueryRow(ctx, `
		SELECT company_id, record_type, signature_required, status
		FROM policy_versions
		WHERE id = $1
		FOR SHARE`, input.PolicyVersionID).Scan(&policyCompany, &policyRecordType, &signatureRequired, &policyStatus); err != nil {
		return nil, &ComplianceGateError{Code: "POLICY_NOT_FOUND", Message: "Signature policy was not found", Details: map[string]interface{}{"error": err.Error()}}
	}
	if policyCompany != input.CompanyID || !recordTypesEqual(policyRecordType, normalizedType) || policyStatus != PolicyStatusActive || !signatureRequired {
		return nil, &ComplianceGateError{Code: "POLICY_MISMATCH", Message: "Challenge does not match an active signature policy"}
	}

	snapshot, err := loadCanonicalRecordSnapshot(ctx, tx, input.CompanyID, input.RecordType, input.RecordID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.RecordVersion) != "" && input.RecordVersion != snapshot.RecordVersion {
		return nil, &ComplianceGateError{Code: "RECORD_VERSION_MISMATCH", Message: "Requested challenge version is not the current server version"}
	}

	challengeID := uuid.New()
	now := scs.now()
	expiry := now.Add(scs.challengeTTL)
	var returnedID uuid.UUID
	var returnedExpiry time.Time
	if err := tx.QueryRow(ctx, `
		INSERT INTO signature_challenges (
			challenge_id, policy_version_id, company_id, record_type, signer_id,
			record_id, record_version, record_hash, expiry, reauthentication_required
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, TRUE)
		RETURNING challenge_id, expiry`,
		challengeID, input.PolicyVersionID, input.CompanyID, normalizedType, input.ActorID,
		input.RecordID, snapshot.RecordVersion, snapshot.Hash, expiry,
	).Scan(&returnedID, &returnedExpiry); err != nil {
		return nil, &ComplianceGateError{Code: "CHALLENGE_GENERATION_FAILED", Message: "Failed to generate signature challenge", Details: map[string]interface{}{"error": err.Error()}}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, &ComplianceGateError{Code: "TX_COMMIT_FAILED", Message: "Failed to commit challenge", Details: map[string]interface{}{"error": err.Error()}}
	}
	return &GeneratedChallenge{
		ChallengeID:   returnedID.String(),
		ExpiresAt:     returnedExpiry,
		ExpiresIn:     returnedExpiry.Sub(now),
		RecordVersion: snapshot.RecordVersion,
		RecordHash:    snapshot.Hash,
	}, nil
}

// VerifyChallengeInput represents the input to verify a challenge.
type VerifyChallengeInput struct {
	ChallengeID   string
	CompanyID     int64
	RecordType    string
	RecordID      int64
	RecordVersion string
	ReauthToken   string // Password or current TOTP code; never stored
	ActorID       int64  // User performing the action
}

// VerifyChallengeResult represents the result of challenge verification.
type VerifyChallengeResult struct {
	Valid         bool
	ChallengeID   string
	RecordID      int64
	RecordVersion string
	RecordHash    string
	AuthMethod    string
	Message       string
}

// VerifyChallenge verifies a challenge and atomically consumes it.
func (scs *SignatureChallengeService) VerifyChallenge(ctx context.Context, input VerifyChallengeInput) (*VerifyChallengeResult, error) {
	invalid := func(message string) *VerifyChallengeResult {
		return &VerifyChallengeResult{Valid: false, Message: message}
	}
	if scs == nil || scs.db == nil || input.ActorID <= 0 || strings.TrimSpace(input.ChallengeID) == "" {
		return invalid("Challenge and actor are required"), nil
	}
	challengeUUID, err := uuid.Parse(input.ChallengeID)
	if err != nil {
		return invalid("Invalid challenge ID format"), nil
	}
	if scs.now == nil {
		scs.now = func() time.Time { return time.Now().UTC() }
	}

	tx, err := scs.db.Begin(ctx)
	if err != nil {
		return nil, &ComplianceGateError{Code: "TX_BEGIN_FAILED", Message: "Failed to begin challenge verification", Details: map[string]interface{}{"error": err.Error()}}
	}
	defer func() { _ = tx.Rollback(ctx) }()

	challenge, err := scanSignatureChallenge(tx.QueryRow(ctx, `
		SELECT id, challenge_id, policy_version_id, company_id, record_type, signer_id,
		       record_id, record_version, record_hash, expiry, reauthentication_required,
		       used, reauthentication_method, reauthenticated_at, created_at
		FROM signature_challenges
		WHERE challenge_id = $1
		FOR UPDATE`, challengeUUID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid("Challenge not found"), nil
		}
		return nil, &ComplianceGateError{Code: "CHALLENGE_LOAD_FAILED", Message: "Failed to load challenge", Details: map[string]interface{}{"error": err.Error()}}
	}
	if !challenge.CompanyID.Valid || !challenge.RecordType.Valid || !challenge.SignerID.Valid || !challenge.RecordHash.Valid || challenge.Challenge.RecordVersion == nil {
		return invalid("Challenge is not bound to a company, actor, and record snapshot"), nil
	}
	if input.CompanyID > 0 && input.CompanyID != challenge.CompanyID.Int64 {
		return invalid("Challenge company does not match"), nil
	}
	if strings.TrimSpace(input.RecordType) != "" && !recordTypesEqual(input.RecordType, challenge.RecordType.String) {
		return invalid("Challenge record type does not match"), nil
	}
	if challenge.SignerID.Int64 != input.ActorID {
		return invalid("Challenge signer does not match actor"), nil
	}
	if !scs.now().Before(challenge.Challenge.Expiry) {
		return invalid("Challenge has expired"), nil
	}
	if challenge.Challenge.Used {
		return invalid("Challenge has already been used"), nil
	}
	if challenge.Challenge.RecordID != input.RecordID {
		return invalid("Challenge record ID does not match"), nil
	}
	if strings.TrimSpace(input.RecordVersion) != "" && input.RecordVersion != *challenge.Challenge.RecordVersion {
		return invalid("Challenge record version does not match"), nil
	}

	snapshot, err := loadCanonicalRecordSnapshot(ctx, tx, challenge.CompanyID.Int64, challenge.RecordType.String, challenge.Challenge.RecordID)
	if err != nil {
		return nil, err
	}
	if snapshot.RecordVersion != *challenge.Challenge.RecordVersion || snapshot.Hash != challenge.RecordHash.String {
		return invalid("Challenge record snapshot is stale"), nil
	}
	if !challenge.Challenge.ReauthenticationRequired {
		return invalid("Challenge is not configured for reauthentication"), nil
	}
	authMethod, valid, err := verifyUserReauthentication(ctx, tx, input.ActorID, input.ReauthToken)
	if err != nil {
		return nil, &ComplianceGateError{Code: "REAUTHENTICATION_CHECK_FAILED", Message: "Failed to verify reauthentication", Details: map[string]interface{}{"error": err.Error()}}
	}
	if !valid {
		return invalid("Reauthentication failed"), nil
	}
	result, err := tx.Exec(ctx, `
		UPDATE signature_challenges
		SET used = TRUE, reauthentication_method = $2, reauthenticated_at = $3
		WHERE id = $1 AND used = FALSE`, challenge.Challenge.ID, authMethod, scs.now())
	if err != nil {
		return nil, &ComplianceGateError{Code: "CHALLENGE_UPDATE_FAILED", Message: "Failed to consume challenge", Details: map[string]interface{}{"error": err.Error()}}
	}
	if result.RowsAffected() != 1 {
		return invalid("Challenge has already been used"), nil
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, &ComplianceGateError{Code: "TX_COMMIT_FAILED", Message: "Failed to commit challenge verification", Details: map[string]interface{}{"error": err.Error()}}
	}
	return &VerifyChallengeResult{
		Valid:         true,
		ChallengeID:   challenge.Challenge.ChallengeID.String(),
		RecordID:      challenge.Challenge.RecordID,
		RecordVersion: *challenge.Challenge.RecordVersion,
		RecordHash:    challenge.RecordHash.String,
		AuthMethod:    authMethod,
		Message:       "Challenge verified successfully",
	}, nil
}

type signatureChallengeRecord struct {
	Challenge  SignatureChallenge
	CompanyID  pgtype.Int8
	RecordType pgtype.Text
	SignerID   pgtype.Int8
	RecordHash pgtype.Text
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSignatureChallenge(row rowScanner) (signatureChallengeRecord, error) {
	var result signatureChallengeRecord
	var recordVersion pgtype.Text
	var reauthMethod pgtype.Text
	var reauthenticatedAt pgtype.Timestamptz
	if err := row.Scan(
		&result.Challenge.ID, &result.Challenge.ChallengeID, &result.Challenge.PolicyVersionID,
		&result.CompanyID, &result.RecordType, &result.SignerID, &result.Challenge.RecordID,
		&recordVersion, &result.RecordHash, &result.Challenge.Expiry,
		&result.Challenge.ReauthenticationRequired, &result.Challenge.Used, &reauthMethod,
		&reauthenticatedAt, &result.Challenge.CreatedAt,
	); err != nil {
		return signatureChallengeRecord{}, err
	}
	if recordVersion.Valid {
		result.Challenge.RecordVersion = &recordVersion.String
	}
	result.Challenge.CompanyID = result.CompanyID.Int64
	result.Challenge.RecordType = result.RecordType.String
	result.Challenge.SignerID = result.SignerID.Int64
	if result.RecordHash.Valid {
		result.Challenge.RecordHash = &result.RecordHash.String
	}
	if reauthMethod.Valid {
		result.Challenge.ReauthenticationMethod = &reauthMethod.String
	}
	if reauthenticatedAt.Valid {
		value := reauthenticatedAt.Time
		result.Challenge.ReauthenticatedAt = &value
	}
	return result, nil
}

// CleanupExpiredChallenges removes expired, unused challenges in bounded
// batches. PostgreSQL does not support DELETE ... LIMIT directly.
func (scs *SignatureChallengeService) CleanupExpiredChallenges(ctx context.Context, limit int) (int64, error) {
	if scs == nil || scs.db == nil {
		return 0, &ComplianceGateError{Code: "CLEANUP_FAILED", Message: "Challenge service is not initialized"}
	}
	if limit <= 0 {
		limit = 100
	}
	result, err := scs.db.Exec(ctx, `
		DELETE FROM signature_challenges
		WHERE id IN (
			SELECT id FROM signature_challenges
			WHERE expiry < NOW() AND used = FALSE
			ORDER BY expiry
			LIMIT $1
		)`, limit)
	if err != nil {
		return 0, &ComplianceGateError{Code: "CLEANUP_FAILED", Message: "Failed to cleanup expired challenges", Details: map[string]interface{}{"error": err.Error()}}
	}
	return result.RowsAffected(), nil
}

// GetChallenge retrieves a challenge by ID for administrative inspection.
func (scs *SignatureChallengeService) GetChallenge(ctx context.Context, challengeID string) (*SignatureChallenge, error) {
	if scs == nil || scs.db == nil {
		return nil, &ComplianceGateError{Code: "CHALLENGE_NOT_FOUND", Message: "Challenge service is not initialized"}
	}
	parsedID, err := uuid.Parse(challengeID)
	if err != nil {
		return nil, &ComplianceGateError{Code: "INVALID_CHALLENGE_ID", Message: "Invalid challenge ID format"}
	}
	challenge, err := scanSignatureChallenge(scs.db.QueryRow(ctx, `
		SELECT id, challenge_id, policy_version_id, company_id, record_type, signer_id,
		       record_id, record_version, record_hash, expiry, reauthentication_required,
		       used, reauthentication_method, reauthenticated_at, created_at
		FROM signature_challenges WHERE challenge_id = $1`, parsedID))
	if err != nil {
		return nil, &ComplianceGateError{Code: "CHALLENGE_NOT_FOUND", Message: "Challenge not found", Details: map[string]interface{}{"error": err.Error()}}
	}
	return &challenge.Challenge, nil
}

// ListPendingChallenges lists unused, unexpired challenges for a record.
func (scs *SignatureChallengeService) ListPendingChallenges(ctx context.Context, recordID int64) ([]SignatureChallenge, error) {
	if scs == nil || scs.db == nil || recordID <= 0 {
		return nil, &ComplianceGateError{Code: "QUERY_FAILED", Message: "Invalid challenge query"}
	}
	rows, err := scs.db.Query(ctx, `
		SELECT id, challenge_id, policy_version_id, company_id, record_type, signer_id,
		       record_id, record_version, record_hash, expiry, reauthentication_required,
		       used, reauthentication_method, reauthenticated_at, created_at
		FROM signature_challenges
		WHERE record_id = $1 AND used = FALSE AND expiry > NOW()
		ORDER BY created_at DESC`, recordID)
	if err != nil {
		return nil, &ComplianceGateError{Code: "QUERY_FAILED", Message: "Failed to query pending challenges", Details: map[string]interface{}{"error": err.Error()}}
	}
	defer rows.Close()
	var challenges []SignatureChallenge
	for rows.Next() {
		challenge, err := scanSignatureChallenge(rows)
		if err != nil {
			return nil, &ComplianceGateError{Code: "SCAN_FAILED", Message: "Failed to scan challenge", Details: map[string]interface{}{"error": err.Error()}}
		}
		challenges = append(challenges, challenge.Challenge)
	}
	if err := rows.Err(); err != nil {
		return nil, &ComplianceGateError{Code: "ROWS_ERROR", Message: "Error iterating challenges", Details: map[string]interface{}{"error": err.Error()}}
	}
	return challenges, nil
}
