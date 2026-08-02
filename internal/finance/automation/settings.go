package automation

import (
	"context"
	"errors"
	"fmt"
)

var ErrInvalidSettings = errors.New("finance automation: invalid settings")

// Settings controls finance automation per company. Every enabled flag is
// false by default so a migration cannot activate a workflow unexpectedly.
type Settings struct {
	CompanyID                        int64
	BankFeedAutoSyncEnabled          bool
	CashForecastEnabled              bool
	PaymentSchedulingEnabled         bool
	PaymentExecutionEnabled          bool
	P2PAutoPostEnabled               bool
	AssetOperationsEnabled           bool
	ForecastHorizonWeeks             int16
	BankFeedSyncIntervalMinutes      int
	PaymentMakerCheckerEnabled       bool
	PaymentExecutorSeparationEnabled bool
	UpdatedBy                        int64
}

func DefaultSettings(companyID int64) Settings {
	return Settings{
		CompanyID:                        companyID,
		ForecastHorizonWeeks:             13,
		BankFeedSyncIntervalMinutes:      1440,
		PaymentMakerCheckerEnabled:       true,
		PaymentExecutorSeparationEnabled: true,
	}
}

func (s Settings) Validate() error {
	if s.CompanyID <= 0 || s.ForecastHorizonWeeks < 1 || s.ForecastHorizonWeeks > 26 || s.BankFeedSyncIntervalMinutes < 15 || s.BankFeedSyncIntervalMinutes > 10080 {
		return ErrInvalidSettings
	}
	if s.PaymentExecutionEnabled && !s.PaymentSchedulingEnabled {
		return fmt.Errorf("%w: payment execution requires payment scheduling", ErrInvalidSettings)
	}
	return nil
}

// SettingsStore must keep every read and write scoped by company ID.
type SettingsStore interface {
	Settings(context.Context, int64) (Settings, error)
	SaveSettings(context.Context, Settings) (Settings, error)
}

type SettingsService struct{ store SettingsStore }

func NewSettingsService(store SettingsStore) *SettingsService { return &SettingsService{store: store} }

func (s *SettingsService) Get(ctx context.Context, companyID int64) (Settings, error) {
	if companyID <= 0 || s == nil || s.store == nil {
		return Settings{}, ErrInvalidSettings
	}
	return s.store.Settings(ctx, companyID)
}

func (s *SettingsService) Update(ctx context.Context, settings Settings) (Settings, error) {
	if s == nil || s.store == nil {
		return Settings{}, ErrInvalidSettings
	}
	if err := settings.Validate(); err != nil {
		return Settings{}, err
	}
	return s.store.SaveSettings(ctx, settings)
}
