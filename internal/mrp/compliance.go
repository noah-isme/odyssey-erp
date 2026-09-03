package mrp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SignatureInput struct {
	RecordType               string `json:"record_type"`
	RecordID                 int64  `json:"record_id"`
	RecordVersion            string `json:"record_version,omitempty"` // Optional assertion; server remains authoritative.
	Meaning                  string `json:"meaning"`
	ReauthToken              string `json:"reauth_token,omitempty"`
	ReauthenticationEvidence string `json:"reauthentication_evidence,omitempty"` // Legacy alias for the credential during migration.
	ChallengeID              string `json:"challenge_id,omitempty"`
	Record                   any    `json:"record,omitempty"` // Ignored; never trusted for signing.
}

type ComplianceService struct{ pool *pgxpool.Pool }

func NewComplianceService(pool *pgxpool.Pool) *ComplianceService {
	return &ComplianceService{pool: pool}
}

// Sign records a signature over the current server-side record snapshot.
// Client JSON, client hashes, and client version numbers are treated only as
// optional assertions and never become compliance evidence.
func (s *ComplianceService) Sign(ctx context.Context, companyID, actorID int64, in SignatureInput) error {
	if s == nil || s.pool == nil || companyID <= 0 || actorID <= 0 || strings.TrimSpace(in.RecordType) == "" || in.RecordID <= 0 || strings.TrimSpace(in.Meaning) == "" {
		return ErrInvalidState
	}
	token := strings.TrimSpace(in.ReauthToken)
	if token == "" {
		token = strings.TrimSpace(in.ReauthenticationEvidence)
	}
	if token == "" {
		return fmt.Errorf("%w: reauthentication is required", ErrInvalidState)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		requiresSignature bool
		retentionDays     int
		active            bool
		roles             []string
		requiresReauth    bool
	)
	if err := tx.QueryRow(ctx, `
		SELECT requires_signature, retention_days, active, approver_roles, reauthentication_required
		FROM mrp_controlled_record_policies
		WHERE company_id = $1 AND LOWER(TRIM(record_type)) = LOWER(TRIM($2))
		FOR SHARE`, companyID, in.RecordType).Scan(&requiresSignature, &retentionDays, &active, &roles, &requiresReauth); err != nil {
		return fmt.Errorf("%w: controlled-record policy unavailable", ErrInvalidState)
	}
	if !active || !requiresSignature || !requiresReauth {
		return fmt.Errorf("%w: controlled-record policy does not permit a signature", ErrInvalidState)
	}
	if err := validateSignatureActor(ctx, tx, actorID, roles); err != nil {
		return err
	}

	snapshot, err := loadCanonicalRecordSnapshot(ctx, tx, companyID, in.RecordType, in.RecordID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(in.RecordVersion) != "" && in.RecordVersion != snapshot.RecordVersion {
		return fmt.Errorf("%w: record version does not match current server snapshot", ErrInvalidState)
	}

	authMethod, valid, err := verifyUserReauthentication(ctx, tx, actorID, token)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("%w: reauthentication failed", ErrInvalidState)
	}
	snapshotID, err := persistCanonicalSnapshot(ctx, tx, snapshot, actorID, &retentionDays)
	if err != nil {
		return err
	}

	evidence := fmt.Sprintf("auth_method=%s;verified_at=%s", authMethod, time.Now().UTC().Format(time.RFC3339Nano))
	if strings.TrimSpace(in.ChallengeID) != "" {
		evidence += ";challenge_id=" + strings.TrimSpace(in.ChallengeID)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO mrp_electronic_signatures (
			company_id, record_type, record_id, record_version, record_hash,
			meaning, signer_id, reauthentication_evidence, snapshot_id, auth_method
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		companyID, snapshot.RecordType, in.RecordID, snapshot.RecordVersion, snapshot.Hash,
		strings.TrimSpace(in.Meaning), actorID, evidence, snapshotID, authMethod); err != nil {
		return err
	}

	detail, err := json.Marshal(map[string]any{
		"snapshot_id":    snapshotID,
		"record_version": snapshot.RecordVersion,
		"record_hash":    snapshot.Hash,
		"meaning":        strings.TrimSpace(in.Meaning),
		"auth_method":    authMethod,
	})
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO mrp_audit_events(company_id, record_type, record_id, event_type, actor_id, detail)
		VALUES ($1, $2, $3, 'ELECTRONIC_SIGNATURE', $4, $5::jsonb)`,
		companyID, snapshot.RecordType, in.RecordID, actorID, string(detail)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func validateSignatureActor(ctx context.Context, tx pgx.Tx, actorID int64, requiredRoles []string) error {
	if actorID <= 0 || len(requiredRoles) == 0 {
		return fmt.Errorf("%w: signature actor or policy roles are missing", ErrInvalidState)
	}
	roles := make([]string, 0, len(requiredRoles))
	for _, role := range requiredRoles {
		if role = strings.ToLower(strings.TrimSpace(role)); role != "" {
			roles = append(roles, role)
		}
	}
	if len(roles) == 0 {
		return fmt.Errorf("%w: signature policy has no approver roles", ErrInvalidState)
	}
	var active bool
	if err := tx.QueryRow(ctx, `SELECT is_active FROM users WHERE id = $1`, actorID).Scan(&active); err != nil {
		return err
	}
	if !active {
		return fmt.Errorf("%w: signature actor is inactive", ErrInvalidState)
	}
	var hasRole bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM user_roles ur
			JOIN roles r ON r.id = ur.role_id
			WHERE ur.user_id = $1
			  AND LOWER(TRIM(r.name)) = ANY($2::text[])
		)`, actorID, roles).Scan(&hasRole); err != nil {
		return err
	}
	if !hasRole {
		return fmt.Errorf("%w: signature actor does not hold a policy approver role", ErrInvalidState)
	}
	return nil
}
