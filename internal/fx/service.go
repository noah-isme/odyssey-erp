package fx

import (
	"context"
	"fmt"
	"time"
)

type Service struct {
	Provider   FXProvider
	Repo       RateRepository
	MaxRateAge time.Duration
	Now        func() time.Time
}

func (s *Service) FetchDailyRates(ctx context.Context, base string, date time.Time, force bool) (FXQuoteSet, error) {
	if s == nil || s.Provider == nil || s.Repo == nil {
		return FXQuoteSet{}, fmt.Errorf("fx service: provider and repository are required")
	}
	set, err := s.Provider.DailyRates(ctx, base, date)
	if err != nil {
		_ = s.Repo.RecordFetchRun(ctx, FetchRun{RateDate: date, Source: "EXCHANGERATE_API", Status: "FAILED", ErrorMessage: err.Error()})
		return FXQuoteSet{}, err
	}
	_ = force // repository upsert is append-safe and force is retained for the job API.
	if err := s.Repo.UpsertDailyRates(ctx, set); err != nil {
		_ = s.Repo.RecordFetchRun(ctx, FetchRun{RateDate: date, Source: set.Source, Status: "FAILED", ResponseHash: set.RawPayloadHash, ErrorMessage: err.Error()})
		return FXQuoteSet{}, err
	}
	if err := s.Repo.RecordFetchRun(ctx, FetchRun{RateDate: date, Source: set.Source, Status: "SUCCESS", ResponseHash: set.RawPayloadHash}); err != nil {
		return FXQuoteSet{}, err
	}
	return set, nil
}

// FetchDailyRatesForJob adapts the service to the worker's task interface.
func (s *Service) FetchDailyRatesForJob(ctx context.Context, base string, date time.Time, force bool) error {
	_, err := s.FetchDailyRates(ctx, base, date, force)
	return err
}
