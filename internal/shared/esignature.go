package shared

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ElectronicSignature represents a 21 CFR Part 11 compliant signature.
type ElectronicSignature struct {
	ID        int64
	CompanyID int64
	ActorID   int64
	Entity    string
	EntityID  string
	Intent    string // e.g., "Reviewed", "Approved", "Authored"
	Remarks   string
	Timestamp time.Time
}

// ESignatureManager handles recording electronic signatures.
type ESignatureManager struct {
	pool        *pgxpool.Pool
	auditLogger *AuditLogger
}

// NewESignatureManager creates a new compliance manager for electronic signatures.
func NewESignatureManager(pool *pgxpool.Pool, audit *AuditLogger) *ESignatureManager {
	return &ESignatureManager{
		pool:        pool,
		auditLogger: audit,
	}
}

// VerifyAndSign ensures a user has actively re-authenticated before signing, 
// recording the signature and emitting a tamper-proof audit log.
func (m *ESignatureManager) VerifyAndSign(ctx context.Context, tx pgx.Tx, sig ElectronicSignature, authenticator func() bool) error {
	if m == nil {
		return errors.New("signature manager not initialized")
	}

	// In 21 CFR Part 11, the user must explicitly re-authenticate right before signing.
	// The `authenticator` callback should validate their password or TOTP MFA token.
	if !authenticator() {
		return errors.New("electronic signature authentication failed")
	}

	// Record the signature in the DB
	_, err := tx.Exec(ctx, `
		INSERT INTO compliance_esignatures 
		(company_id, actor_id, entity, entity_id, intent, remarks, signed_at) 
		VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, NOW()))
	`, sig.CompanyID, sig.ActorID, sig.Entity, sig.EntityID, sig.Intent, sig.Remarks, sig.Timestamp)
	if err != nil {
		return err
	}

	// Must also emit a strict audit log
	return m.auditLogger.RecordTx(ctx, tx, AuditLog{
		ActorID:  sig.ActorID,
		Action:   "Electronic Signature Applied",
		Entity:   sig.Entity,
		EntityID: sig.EntityID,
		Meta: map[string]any{
			"intent": sig.Intent,
			"compliance": "21_CFR_Part_11",
		},
		At: time.Now(),
	})
}
