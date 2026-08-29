package payments

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestPostgresSettlementResultStoreRejectsPayloadIdentityMismatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(SettlementResult) SettlementResult
		want   error
	}{
		{
			name: "company",
			mutate: func(result SettlementResult) SettlementResult {
				result.CompanyID = 8
				return result
			},
			want: ErrSettlementResultCompanyMismatch,
		},
		{
			name: "result id",
			mutate: func(result SettlementResult) SettlementResult {
				result.ResultID = "different-result"
				return result
			},
			want: ErrSettlementResultReferenceMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer db.Close()

			payloadResult := tt.mutate(settlementResultForEffects())
			payload, err := json.Marshal(payloadResult)
			require.NoError(t, err)
			db.ExpectQuery("SELECT payload, fingerprint, effect_applied, recorded_at").WithArgs(int64(7), "result-effects-1").WillReturnRows(
				pgxmock.NewRows([]string{"payload", "fingerprint", "effect_applied", "recorded_at"}).AddRow(
					payload, "fingerprint", false, time.Date(2026, time.August, 12, 9, 6, 0, 0, time.UTC),
				),
			)

			_, err = NewPostgresSettlementResultStore(db).GetSettlementResult(context.Background(), 7, " result-effects-1 ")
			require.ErrorIs(t, err, tt.want)
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}
