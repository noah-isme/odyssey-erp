package mrp

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SignatureChallengeService handles one-time signature challenge generation and verification
type SignatureChallengeService struct {
	db            *pgx.Conn
	challengeTTL  time.Duration // 5 minutes default
}

// NewSignatureChallengeService creates a new challenge service
func NewSignatureChallengeService(db *pgx.Conn) *SignatureChallengeService {
	return &SignatureChallengeService{
		db:           db,
		challengeTTL: 5 * time.Minute,
	}
}

// GenerateChallengeInput represents the input to generate a challenge
type GenerateChallengeInput struct {
	PolicyVersionID int64
	RecordID        int64
	RecordVersion   string
	RequiresReauth  bool
}

// GeneratedChallenge represents a generated challenge
type GeneratedChallenge struct {
	ChallengeID string
	ExpiresAt   time.Time
	ExpiresIn   time.Duration
}

// GenerateChallenge creates a new one-time signature challenge
func (scs *SignatureChallengeService) GenerateChallenge(ctx context.Context, input GenerateChallengeInput) (*GeneratedChallenge, error) {
	// Generate unique challenge ID
	challengeID := uuid.New()

	// Calculate expiry (5 minutes from now)
	now := time.Now()
	expiry := now.Add(scs.challengeTTL)

	// Insert into database
	const query = `
		INSERT INTO signature_challenges (
			challenge_id, policy_version_id, record_id, record_version,
			expiry, reauthentication_required
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING challenge_id, expiry
	`

	row := scs.db.QueryRow(ctx, query,
		challengeID,
		input.PolicyVersionID,
		input.RecordID,
		input.RecordVersion,
		expiry,
		input.RequiresReauth,
	)

	var returnedChallengeID uuid.UUID
	var returnedExpiry time.Time

	if err := row.Scan(&returnedChallengeID, &returnedExpiry); err != nil {
		return nil, &ComplianceGateError{
			Code:    "CHALLENGE_GENERATION_FAILED",
			Message: "Failed to generate signature challenge",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	return &GeneratedChallenge{
		ChallengeID: returnedChallengeID.String(),
		ExpiresAt:   returnedExpiry,
		ExpiresIn:   scs.challengeTTL,
	}, nil
}

// VerifyChallengeInput represents the input to verify a challenge
type VerifyChallengeInput struct {
	ChallengeID      string
	RecordID         int64
	RecordVersion    string
	ReauthToken      string // Password/2FA token for reauthentication
	ActorID          int64  // User performing the action
}

// VerifyChallengeResult represents the result of challenge verification
type VerifyChallengeResult struct {
	Valid         bool
	ChallengeID   string
	RecordID      int64
	RecordVersion string
	Message       string
}

// VerifyChallenge verifies a signature challenge
func (scs *SignatureChallengeService) VerifyChallenge(ctx context.Context, input VerifyChallengeInput) (*VerifyChallengeResult, error) {
	// Parse challenge ID
	challengeUUID, err := uuid.Parse(input.ChallengeID)
	if err != nil {
		return &VerifyChallengeResult{
			Valid:   false,
			Message: "Invalid challenge ID format",
		}, nil
	}

	// Query challenge with lock
	const query = `
		SELECT id, challenge_id, policy_version_id, record_id, record_version,
		       expiry, reauthentication_required, used, created_at
		FROM signature_challenges
		WHERE challenge_id = $1
		FOR UPDATE
	`

	row := scs.db.QueryRow(ctx, query, challengeUUID)
	var challenge SignatureChallenge

	if err := row.Scan(
		&challenge.ID, &challenge.ChallengeID, &challenge.PolicyVersionID,
		&challenge.RecordID, &challenge.RecordVersion, &challenge.Expiry,
		&challenge.ReauthenticationRequired, &challenge.Used, &challenge.CreatedAt,
	); err != nil {
		return &VerifyChallengeResult{
			Valid:   false,
			Message: "Challenge not found",
		}, nil
	}

	// Check expiry
	if time.Now().After(challenge.Expiry) {
		return &VerifyChallengeResult{
			Valid:   false,
			Message: "Challenge has expired",
		}, nil
	}

	// Check if already used
	if challenge.Used {
		return &VerifyChallengeResult{
			Valid:   false,
			Message: "Challenge has already been used",
		}, nil
	}

	// Check record ID matches
	if challenge.RecordID != input.RecordID {
		return &VerifyChallengeResult{
			Valid:   false,
			Message: "Challenge record ID does not match",
		}, nil
	}

	// Check record version matches
	if challenge.RecordVersion != nil && input.RecordVersion != *challenge.RecordVersion {
		return &VerifyChallengeResult{
			Valid:   false,
			Message: "Challenge record version does not match",
		}, nil
	}

	// If reauthentication required, verify password/token
	if challenge.ReauthenticationRequired {
		if !scs.verifyReauthentication(ctx, input.ActorID, input.ReauthToken) {
			return &VerifyChallengeResult{
				Valid:   false,
				Message: "Reauthentication failed",
			}, nil
		}
	}

	// Mark challenge as used
	const updateQuery = `UPDATE signature_challenges SET used = TRUE WHERE id = $1`
	if _, err := scs.db.Exec(ctx, updateQuery, challenge.ID); err != nil {
		return &VerifyChallengeResult{
			Valid:   false,
			Message: "Failed to mark challenge as used",
		}, nil
	}

	return &VerifyChallengeResult{
		Valid:         true,
		ChallengeID:   challenge.ChallengeID.String(),
		RecordID:      challenge.RecordID,
		RecordVersion: *challenge.RecordVersion,
		Message:       "Challenge verified successfully",
	}, nil
}

// verifyReauthentication verifies password or 2FA token
// This is a placeholder - in real implementation would call auth service
func (scs *SignatureChallengeService) verifyReauthentication(ctx context.Context, actorID int64, token string) bool {
	// TODO: Call auth service to verify token against user password or 2FA
	// For now, placeholder that checks token is not empty
	return token != ""
}

// CleanupExpiredChallenges removes expired challenges (should be run periodically)
func (scs *SignatureChallengeService) CleanupExpiredChallenges(ctx context.Context, limit int) (int64, error) {
	const query = `
		DELETE FROM signature_challenges
		WHERE expiry < NOW() AND used = FALSE
		LIMIT $1
	`

	result, err := scs.db.Exec(ctx, query, limit)
	if err != nil {
		return 0, &ComplianceGateError{
			Code:    "CLEANUP_FAILED",
			Message: "Failed to cleanup expired challenges",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	return result.RowsAffected(), nil
}

// GetChallenge retrieves a challenge by ID (for debugging/admin purposes)
func (scs *SignatureChallengeService) GetChallenge(ctx context.Context, challengeID string) (*SignatureChallenge, error) {
	parsedID, err := uuid.Parse(challengeID)
	if err != nil {
		return nil, &ComplianceGateError{
			Code:    "INVALID_CHALLENGE_ID",
			Message: "Invalid challenge ID format",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	const query = `
		SELECT id, challenge_id, policy_version_id, record_id, record_version,
		       expiry, reauthentication_required, used, created_at
		FROM signature_challenges
		WHERE challenge_id = $1
	`

	row := scs.db.QueryRow(ctx, query, parsedID)
	var challenge SignatureChallenge

	if err := row.Scan(
		&challenge.ID, &challenge.ChallengeID, &challenge.PolicyVersionID,
		&challenge.RecordID, &challenge.RecordVersion, &challenge.Expiry,
		&challenge.ReauthenticationRequired, &challenge.Used, &challenge.CreatedAt,
	); err != nil {
		return nil, &ComplianceGateError{
			Code:    "CHALLENGE_NOT_FOUND",
			Message: "Challenge not found",
			Details: map[string]interface{}{"challenge_id": challengeID},
		}
	}

	return &challenge, nil
}

// ListPendingChallenges lists all pending (unused, unexpired) challenges for a record
func (scs *SignatureChallengeService) ListPendingChallenges(ctx context.Context, recordID int64) ([]SignatureChallenge, error) {
	const query = `
		SELECT id, challenge_id, policy_version_id, record_id, record_version,
		       expiry, reauthentication_required, used, created_at
		FROM signature_challenges
		WHERE record_id = $1
		  AND used = FALSE
		  AND expiry > NOW()
		ORDER BY created_at DESC
	`

	rows, err := scs.db.Query(ctx, query, recordID)
	if err != nil {
		return nil, &ComplianceGateError{
			Code:    "QUERY_FAILED",
			Message: "Failed to query pending challenges",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}
	defer rows.Close()

	var challenges []SignatureChallenge
	for rows.Next() {
		var challenge SignatureChallenge
		if err := rows.Scan(
			&challenge.ID, &challenge.ChallengeID, &challenge.PolicyVersionID,
			&challenge.RecordID, &challenge.RecordVersion, &challenge.Expiry,
			&challenge.ReauthenticationRequired, &challenge.Used, &challenge.CreatedAt,
		); err != nil {
			return nil, &ComplianceGateError{
				Code:    "SCAN_FAILED",
				Message: "Failed to scan challenge",
				Details: map[string]interface{}{"error": err.Error()},
			}
		}
		challenges = append(challenges, challenge)
	}

	if err = rows.Err(); err != nil {
		return nil, &ComplianceGateError{
			Code:    "ROWS_ERROR",
			Message: "Error iterating challenges",
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	return challenges, nil
}
