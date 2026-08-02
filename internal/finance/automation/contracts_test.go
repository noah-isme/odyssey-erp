package automation

import (
	"context"
	"testing"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	"github.com/stretchr/testify/require"
)

func TestExactAmountUsesExactMoneyAndCurrency(t *testing.T) {
	amount := ExactAmount{Amount: accountingmoney.Must("123.45", 2), Currency: "idr"}
	require.NoError(t, amount.Validate())
	require.ErrorIs(t, (ExactAmount{Amount: accountingmoney.Must("1", 0), Currency: "ID"}).Validate(), ErrInvalidAmount)
}

func TestSettingsDefaultsAreConservative(t *testing.T) {
	settings := DefaultSettings(7)
	require.NoError(t, settings.Validate())
	require.False(t, settings.BankFeedAutoSyncEnabled)
	require.False(t, settings.CashForecastEnabled)
	require.False(t, settings.PaymentSchedulingEnabled)
	require.False(t, settings.PaymentExecutionEnabled)
	require.False(t, settings.P2PAutoPostEnabled)
	require.False(t, settings.AssetOperationsEnabled)
	require.True(t, settings.PaymentMakerCheckerEnabled)
	require.True(t, settings.PaymentExecutorSeparationEnabled)
	require.EqualValues(t, 13, settings.ForecastHorizonWeeks)
}

func TestSettingsServiceKeepsCompanySettingsIsolated(t *testing.T) {
	store := &settingsStoreFake{items: map[int64]Settings{1: DefaultSettings(1), 2: DefaultSettings(2)}}
	service := NewSettingsService(store)

	companyOne := DefaultSettings(1)
	companyOne.CashForecastEnabled = true
	companyOne.UpdatedBy = 11
	_, err := service.Update(context.Background(), companyOne)
	require.NoError(t, err)

	gotOne, err := service.Get(context.Background(), 1)
	require.NoError(t, err)
	require.True(t, gotOne.CashForecastEnabled)
	gotTwo, err := service.Get(context.Background(), 2)
	require.NoError(t, err)
	require.False(t, gotTwo.CashForecastEnabled)
}

func TestSettingsRejectsLivePaymentExecutionWithoutScheduling(t *testing.T) {
	settings := DefaultSettings(1)
	settings.PaymentExecutionEnabled = true
	require.ErrorIs(t, settings.Validate(), ErrInvalidSettings)
}

func TestPaymentDutySeparation(t *testing.T) {
	settings := DefaultSettings(1)
	require.ErrorIs(t, ValidatePaymentDutySeparation(settings, 1, 1, 3), ErrIncompatiblePaymentDuties)
	require.ErrorIs(t, ValidatePaymentDutySeparation(settings, 1, 2, 2), ErrIncompatiblePaymentDuties)
	require.NoError(t, ValidatePaymentDutySeparation(settings, 1, 2, 3))

	settings.PaymentMakerCheckerEnabled = false
	settings.PaymentExecutorSeparationEnabled = false
	require.NoError(t, ValidatePaymentDutySeparation(settings, 1, 1, 1))
}

func TestOutboxMessageValidation(t *testing.T) {
	input := EnqueueInput{
		CompanyID:      1,
		Topic:          "payment.submit",
		AggregateType:  "payment_batch",
		AggregateID:    "42",
		Operation:      "payment.submit",
		Correlation:    Correlation{ID: "req-1"},
		IdempotencyKey: "batch-42-revision-1",
	}
	require.NoError(t, input.message().Validate())

	input.Payload = []byte("not-json")
	require.ErrorIs(t, input.message().Validate(), ErrInvalidOutboxMessage)
}

type settingsStoreFake struct{ items map[int64]Settings }

func (s *settingsStoreFake) Settings(_ context.Context, companyID int64) (Settings, error) {
	item, ok := s.items[companyID]
	if !ok {
		return Settings{}, ErrInvalidSettings
	}
	return item, nil
}

func (s *settingsStoreFake) SaveSettings(_ context.Context, settings Settings) (Settings, error) {
	if err := settings.Validate(); err != nil {
		return Settings{}, err
	}
	s.items[settings.CompanyID] = settings
	return settings, nil
}
