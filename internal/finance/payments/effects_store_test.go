package payments

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func settlementResultForEffects() SettlementResult {
	instruction := paymentInstruction()
	return SettlementResult{
		CompanyID:            instruction.Reference.Connection.CompanyID,
		ResultID:             "result-effects-1",
		InstructionReference: instruction.Reference,
		ProviderReference:    providerReference(),
		Status:               ResultStatusSettled,
		State:                StateSettled,
		SettledAmount:        automation.MustParseExact("123.45"),
		SettledAt:            time.Date(2026, time.August, 12, 9, 5, 0, 0, time.UTC),
	}
}

func TestPostgresSettlementEffectsAppliesLinksInOneTransaction(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	request := SettlementEffectRequest{
		CompanyID: 7,
		EffectKey: "effect-effects-1",
		Result:    settlementResultForEffects(),
	}
	applierCalled := false
	applier := SettlementEffectsApplierFunc(func(_ context.Context, _ pgx.Tx, got SettlementEffectRequest) ([]SettlementEffectLink, error) {
		applierCalled = true
		require.Equal(t, request.EffectKey, got.EffectKey)
		return []SettlementEffectLink{{
			LinkType:   "AP_PAYMENT",
			EntityType: "ap_payment",
			EntityID:   "42",
			Amount:     automation.MustParseExact("123.45"),
			Metadata:   map[string]any{"currency": "USD"},
		}}, nil
	})
	store := NewPostgresSettlementEffectsWithApplier(db, applier)

	db.ExpectBegin()
	db.ExpectExec("INSERT INTO payment_settlement_effects").WithArgs(
		int64(7), "effect-effects-1", "result-effects-1", "SETTLED", pgxmock.AnyArg(), pgxmock.AnyArg(),
	).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectExec("INSERT INTO payment_settlement_effect_links").WithArgs(
		int64(7), "effect-effects-1", "result-effects-1", "AP_PAYMENT", "ap_payment", "42",
		"123.45", "USD", pgxmock.AnyArg(),
	).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectCommit()

	outcome, err := store.ApplySettlementEffects(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, SettlementEffectOutcome{Applied: true}, outcome)
	require.True(t, applierCalled)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestPostgresSettlementEffectsRollsBackDomainFailure(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	request := SettlementEffectRequest{CompanyID: 7, EffectKey: "effect-failure-1", Result: settlementResultForEffects()}
	domainErr := errors.New("accounting mapping missing")
	store := NewPostgresSettlementEffectsWithApplier(db, SettlementEffectsApplierFunc(func(context.Context, pgx.Tx, SettlementEffectRequest) ([]SettlementEffectLink, error) {
		return nil, domainErr
	}))

	db.ExpectBegin()
	db.ExpectExec("INSERT INTO payment_settlement_effects").WithArgs(
		int64(7), "effect-failure-1", "result-effects-1", "SETTLED", pgxmock.AnyArg(), pgxmock.AnyArg(),
	).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectRollback()

	_, err = store.ApplySettlementEffects(context.Background(), request)
	require.ErrorIs(t, err, domainErr)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestPostgresSettlementEffectsReturnsAlreadyAppliedForSameFingerprint(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	request := SettlementEffectRequest{CompanyID: 7, EffectKey: "effect-replay-1", Result: settlementResultForEffects()}
	store := NewPostgresSettlementEffects(db)
	fingerprint := settlementEffectFingerprint(request)

	db.ExpectBegin()
	db.ExpectExec("INSERT INTO payment_settlement_effects").WithArgs(
		int64(7), "effect-replay-1", "result-effects-1", "SETTLED", fingerprint, pgxmock.AnyArg(),
	).WillReturnResult(pgxmock.NewResult("INSERT", 0))
	db.ExpectQuery("SELECT fingerprint").WithArgs(int64(7), "effect-replay-1").
		WillReturnRows(pgxmock.NewRows([]string{"fingerprint"}).AddRow(fingerprint))
	db.ExpectCommit()

	outcome, err := store.ApplySettlementEffects(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, SettlementEffectOutcome{AlreadyApplied: true}, outcome)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestSettlementEffectRequestRejectsInvalidExactLink(t *testing.T) {
	request := SettlementEffectRequest{
		CompanyID: 7,
		EffectKey: "effect-invalid-link",
		Result:    settlementResultForEffects(),
		Links: []SettlementEffectLink{{
			LinkType:   "AP_PAYMENT",
			EntityType: "ap_payment",
			EntityID:   "42",
			Amount:     automation.MustParseExact("123.45"),
		}},
	}
	request.Links[0].Amount.Currency = "EUR"
	require.ErrorIs(t, request.Validate(), ErrSettlementResultReferenceMismatch)
}
