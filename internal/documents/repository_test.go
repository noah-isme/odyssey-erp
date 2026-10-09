package documents

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// captureUUID matches any pgtype.UUID argument and keeps it.
type captureUUID struct{ got *pgtype.UUID }

func (c captureUUID) Match(v any) bool {
	id, ok := v.(pgtype.UUID)
	if ok {
		*c.got = id
	}
	return ok && id.Valid
}

// document_signature_challenges has NOT NULL challenge_id, meaning and
// policy_version without defaults (migration 000082), so the insert must
// name them and the returned id must be the canonical UUID string.
func TestInsertSignatureChallengeNamesEveryRequiredColumn(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	repo := &Repository{queries: sqlc.New(mock)}

	expiry := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	var sent pgtype.UUID
	mock.ExpectQuery(regexp.QuoteMeta(
		`INSERT INTO document_signature_challenges (company_id, challenge_id, document_version_id, signer_id, meaning, policy_version, expiry)`)).
		WithArgs(int64(3), captureUUID{got: &sent}, int64(9), int64(7), "Approved as effective version", int32(1),
			pgtype.Timestamptz{Time: expiry, Valid: true}).
		WillReturnRows(pgxmock.NewRows([]string{"challenge_id"}).AddRow(pgtype.UUID{Bytes: uuid.MustParse("6f1c5e1e-6b4f-4a43-9a43-6c5f0d3c2b10"), Valid: true}))

	id, err := repo.InsertSignatureChallenge(context.Background(), SignatureChallengeRequest{
		CompanyID:         3,
		DocumentVersionID: 9,
		SignerID:          7,
		Meaning:           "Approved as effective version",
		PolicyVersion:     1,
		Expiry:            expiry,
	})
	require.NoError(t, err)
	parsed, err := uuid.Parse(id)
	require.NoError(t, err, "challenge id %q is not a canonical UUID", id)
	require.Equal(t, uuid.UUID(sent.Bytes), parsed, "returned id must be the challenge_id that was inserted")
	require.NoError(t, mock.ExpectationsWereMet())
}
