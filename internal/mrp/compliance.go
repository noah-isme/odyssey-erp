package mrp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SignatureInput struct {
	RecordType               string `json:"record_type"`
	RecordID                 int64  `json:"record_id"`
	RecordVersion            string `json:"record_version"`
	Meaning                  string `json:"meaning"`
	ReauthenticationEvidence string `json:"reauthentication_evidence"`
	Record                   any    `json:"record"`
}
type ComplianceService struct{ pool *pgxpool.Pool }

func NewComplianceService(pool *pgxpool.Pool) *ComplianceService {
	return &ComplianceService{pool: pool}
}
func (s *ComplianceService) Sign(ctx context.Context, companyID, actorID int64, in SignatureInput) error {
	if s == nil || s.pool == nil || companyID <= 0 || actorID <= 0 || in.RecordType == "" || in.RecordID <= 0 || in.RecordVersion == "" || in.Meaning == "" || in.ReauthenticationEvidence == "" {
		return ErrInvalidState
	}
	b, err := json.Marshal(in.Record)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	_, err = s.pool.Exec(ctx, `INSERT INTO mrp_electronic_signatures(company_id,record_type,record_id,record_version,record_hash,meaning,signer_id,reauthentication_evidence) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, companyID, in.RecordType, in.RecordID, in.RecordVersion, hex.EncodeToString(sum[:]), in.Meaning, actorID, in.ReauthenticationEvidence)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO mrp_audit_events(company_id,record_type,record_id,event_type,actor_id,detail) VALUES($1,$2,$3,'ELECTRONIC_SIGNATURE',$4,$5::jsonb)`, companyID, in.RecordType, in.RecordID, actorID, string(b))
	return err
}
