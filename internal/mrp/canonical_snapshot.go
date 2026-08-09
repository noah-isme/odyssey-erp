package mrp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// canonicalRecordSnapshot is the only record representation accepted by the
// compliance gate and electronic-signature writer. The JSON is generated from
// locked database rows; callers never supply the signed payload.
type canonicalRecordSnapshot struct {
	CompanyID     int64
	RecordType    string
	RecordID      int64
	RecordVersion string
	CanonicalJSON []byte
	Hash          string
	CapturedAt    time.Time
	CreatedBy     *int64
	SnapshotID    int64
}

func normalizeRecordType(recordType string) (string, error) {
	switch strings.ToUpper(strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.TrimSpace(recordType))) {
	case "BOM", "BOMREVISION", "MRPBOM":
		return "BOM", nil
	case "WORKORDER", "MRPWORKORDER":
		return "WORK_ORDER", nil
	case "OPERATION", "MRPOPERATION":
		return "OPERATION", nil
	case "ROUTING", "MRPROUTING":
		return "ROUTING", nil
	case "INSPECTION", "MRPINSPECTION":
		return "INSPECTION", nil
	case "QUALITYHOLD", "HOLD", "MRPQUALITYHOLD":
		return "QUALITY_HOLD", nil
	case "NCR", "NONCONFORMANCE", "MRPNONCONFORMANCE":
		return "NCR", nil
	case "CAPA", "MRPCAPA":
		return "CAPA", nil
	default:
		return "", fmt.Errorf("unsupported controlled record type %q", recordType)
	}
}

func recordTypesEqual(left, right string) bool {
	normalizedLeft, leftErr := normalizeRecordType(left)
	normalizedRight, rightErr := normalizeRecordType(right)
	return leftErr == nil && rightErr == nil && normalizedLeft == normalizedRight
}

// canonicalizeJSON normalizes object key order and rejects trailing data.
// UseNumber preserves database numeric values instead of converting them to
// float64 before hashing.
func canonicalizeJSON(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("snapshot contains trailing JSON values")
		}
		return nil, err
	}
	return json.Marshal(value)
}

func hashCanonicalJSON(canonical []byte) string {
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// loadCanonicalRecordSnapshot locks the current record and its child rows for
// the duration of the caller's transaction. Every supported record type uses
// the live schema and includes its relevant child collection in the signed
// envelope.
func loadCanonicalRecordSnapshot(ctx context.Context, tx pgx.Tx, companyID int64, recordType string, recordID int64) (canonicalRecordSnapshot, error) {
	if companyID <= 0 || recordID <= 0 {
		return canonicalRecordSnapshot{}, fmt.Errorf("company, record, and record id are required")
	}

	normalizedType, err := normalizeRecordType(recordType)
	if err != nil {
		return canonicalRecordSnapshot{}, err
	}

	var (
		rawJSON     string
		versionText string
		versionTime time.Time
		createdBy   pgtype.Int8
	)

	query := ""
	switch normalizedType {
	case "BOM":
		query = `
			SELECT jsonb_build_object(
				'record_type', 'BOM',
				'record_id', b.id,
				'record', to_jsonb(b),
				'lines', COALESCE((SELECT jsonb_agg(to_jsonb(l) ORDER BY l.id)
					FROM mrp_bom_lines l WHERE l.bom_id = b.id), '[]'::jsonb)
			)::text, b.version, b.created_at, b.created_by
			FROM mrp_boms b
			WHERE b.company_id = $1 AND b.id = $2
			FOR UPDATE`
	case "WORK_ORDER":
		query = `
			SELECT jsonb_build_object(
				'record_type', 'WORK_ORDER',
				'record_id', wo.id,
				'record', to_jsonb(wo),
				'operations', COALESCE((SELECT jsonb_agg(to_jsonb(op) ORDER BY op.sequence, op.id)
					FROM mrp_work_order_operations op WHERE op.work_order_id = wo.id), '[]'::jsonb)
			)::text, '', wo.updated_at, wo.created_by
			FROM mrp_work_orders wo
			WHERE wo.company_id = $1 AND wo.id = $2
			FOR UPDATE`
	case "OPERATION":
		query = `
			SELECT jsonb_build_object(
				'record_type', 'OPERATION',
				'record_id', op.id,
				'record', to_jsonb(op)
			)::text, '', op.updated_at, NULL::bigint
			FROM mrp_work_order_operations op
			WHERE op.company_id = $1 AND op.id = $2
			FOR UPDATE`
	case "ROUTING":
		query = `
			SELECT jsonb_build_object(
				'record_type', 'ROUTING',
				'record_id', r.id,
				'record', to_jsonb(r),
				'operations', COALESCE((SELECT jsonb_agg(to_jsonb(op) ORDER BY op.sequence, op.id)
					FROM mrp_routing_operations op WHERE op.routing_id = r.id), '[]'::jsonb)
			)::text, r.version, r.created_at, r.created_by
			FROM mrp_routings r
			WHERE r.company_id = $1 AND r.id = $2
			FOR UPDATE`
	case "INSPECTION":
		query = `
			SELECT jsonb_build_object(
				'record_type', 'INSPECTION',
				'record_id', i.id,
				'record', to_jsonb(i)
			)::text, '', i.updated_at, i.inspector_id
			FROM mrp_inspections i
			WHERE i.company_id = $1 AND i.id = $2
			FOR UPDATE`
	case "QUALITY_HOLD":
		query = `
			SELECT jsonb_build_object(
				'record_type', 'QUALITY_HOLD',
				'record_id', h.id,
				'record', to_jsonb(h)
			)::text, h.status || ':' || COALESCE(h.released_at::text, ''),
				COALESCE(h.released_at, h.created_at), h.created_by
			FROM mrp_quality_holds h
			WHERE h.company_id = $1 AND h.id = $2
			FOR UPDATE`
	case "NCR":
		query = `
			SELECT jsonb_build_object(
				'record_type', 'NCR',
				'record_id', n.id,
				'record', to_jsonb(n)
			)::text, n.status || ':' || COALESCE(n.owner_id::text, ''), n.created_at, n.owner_id
			FROM mrp_nonconformances n
			WHERE n.company_id = $1 AND n.id = $2
			FOR UPDATE`
	case "CAPA":
		query = `
			SELECT jsonb_build_object(
				'record_type', 'CAPA',
				'record_id', c.id,
				'record', to_jsonb(c)
			)::text, c.status || ':' || COALESCE(c.closed_at::text, ''),
				COALESCE(c.closed_at, TIMESTAMPTZ 'epoch'), c.owner_id
			FROM mrp_capas c
			WHERE c.company_id = $1 AND c.id = $2
			FOR UPDATE`
	}

	if err := tx.QueryRow(ctx, query, companyID, recordID).Scan(&rawJSON, &versionText, &versionTime, &createdBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return canonicalRecordSnapshot{}, &ComplianceGateError{
				Code:    "RECORD_NOT_FOUND",
				Message: "Controlled record was not found in the requested company",
				Details: map[string]interface{}{"company_id": companyID, "record_type": recordType, "record_id": recordID},
			}
		}
		return canonicalRecordSnapshot{}, &ComplianceGateError{
			Code:    "SNAPSHOT_LOAD_FAILED",
			Message: "Failed to load controlled record snapshot",
			Details: map[string]interface{}{"record_type": normalizedType, "record_id": recordID, "error": err.Error()},
		}
	}

	canonicalJSON, err := canonicalizeJSON([]byte(rawJSON))
	if err != nil {
		return canonicalRecordSnapshot{}, &ComplianceGateError{
			Code:    "SNAPSHOT_CANONICALIZATION_FAILED",
			Message: "Controlled record snapshot is not valid JSON",
			Details: map[string]interface{}{"record_type": normalizedType, "record_id": recordID, "error": err.Error()},
		}
	}
	version := strings.TrimSpace(versionText)
	if version == "" && !versionTime.IsZero() {
		version = versionTime.UTC().Format(time.RFC3339Nano)
	}
	if version == "" {
		return canonicalRecordSnapshot{}, &ComplianceGateError{
			Code:    "RECORD_VERSION_MISSING",
			Message: "Controlled record has no stable version",
			Details: map[string]interface{}{"record_type": normalizedType, "record_id": recordID},
		}
	}

	return canonicalRecordSnapshot{
		CompanyID:     companyID,
		RecordType:    normalizedType,
		RecordID:      recordID,
		RecordVersion: version,
		CanonicalJSON: canonicalJSON,
		Hash:          hashCanonicalJSON(canonicalJSON),
		CapturedAt:    time.Now().UTC(),
		CreatedBy:     int8Pointer(&createdBy),
	}, nil
}

func int8Pointer(value *pgtype.Int8) *int64 {
	if value == nil || !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func persistCanonicalSnapshot(ctx context.Context, tx pgx.Tx, snapshot canonicalRecordSnapshot, actorID int64, retentionDays *int) (int64, error) {
	if actorID <= 0 {
		return 0, errors.New("snapshot actor is required")
	}
	var retentionUntil any
	if retentionDays != nil && *retentionDays > 0 {
		retentionUntil = snapshot.CapturedAt.AddDate(0, 0, *retentionDays)
	}

	var snapshotID int64
	err := tx.QueryRow(ctx, `
		INSERT INTO mrp_record_snapshots (
			company_id, record_type, record_id, record_version, snapshot,
			record_hash, captured_by, captured_at, retention_until
		)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9)
		ON CONFLICT (company_id, record_type, record_id, record_version) DO NOTHING
		RETURNING id`,
		snapshot.CompanyID, snapshot.RecordType, snapshot.RecordID, snapshot.RecordVersion,
		snapshot.CanonicalJSON, snapshot.Hash, actorID, snapshot.CapturedAt, retentionUntil,
	).Scan(&snapshotID)
	if err == nil {
		return snapshotID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}

	var existingHash string
	if err := tx.QueryRow(ctx, `
		SELECT id, record_hash
		FROM mrp_record_snapshots
		WHERE company_id = $1 AND record_type = $2 AND record_id = $3 AND record_version = $4
		FOR SHARE`, snapshot.CompanyID, snapshot.RecordType, snapshot.RecordID, snapshot.RecordVersion).Scan(&snapshotID, &existingHash); err != nil {
		return 0, err
	}
	if existingHash != snapshot.Hash {
		return 0, &ComplianceGateError{
			Code:    "RECORD_VERSION_CONFLICT",
			Message: "Record version already points to a different snapshot",
			Details: map[string]interface{}{"record_type": snapshot.RecordType, "record_id": snapshot.RecordID, "record_version": snapshot.RecordVersion},
		}
	}
	return snapshotID, nil
}
