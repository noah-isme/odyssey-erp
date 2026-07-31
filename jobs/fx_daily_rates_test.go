package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

type fxCompaniesFake struct{ currencies []string }

func (f fxCompaniesFake) CompanyBaseCurrencies(context.Context) ([]string, error) {
	return f.currencies, nil
}

type fxFetcherFake struct {
	dates []time.Time
	err   error
}

func (f *fxFetcherFake) FetchDailyRates(_ context.Context, _ string, date time.Time, _ bool) error {
	f.dates = append(f.dates, date)
	return f.err
}

func TestFXDailyRatesUsesJakartaBusinessDateAndAllCompanies(t *testing.T) {
	fetcher := &fxFetcherFake{}
	task := asynq.NewTask(TaskFXDailyRates, []byte(`{}`))
	location, _ := time.LoadLocation("Asia/Jakarta")
	if err := HandleFXDailyRatesTask(fetcher, fxCompaniesFake{currencies: []string{"IDR", "USD"}}, location, nil)(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if len(fetcher.dates) != 2 || fetcher.dates[0].Location() != location {
		t.Fatalf("unexpected fetches: %+v", fetcher.dates)
	}
}

func TestFXDailyRatesReturnsRetryableErrorsAndRejectsBadPayload(t *testing.T) {
	fetcher := &fxFetcherFake{err: errors.New("temporary provider failure")}
	location, _ := time.LoadLocation("Asia/Jakarta")
	if err := HandleFXDailyRatesTask(fetcher, fxCompaniesFake{currencies: []string{"IDR"}}, location, nil)(context.Background(), asynq.NewTask(TaskFXDailyRates, []byte(`{}`))); err == nil {
		t.Fatal("expected retryable error")
	}
	if err := HandleFXDailyRatesTask(fetcher, fxCompaniesFake{currencies: []string{"IDR"}}, location, nil)(context.Background(), asynq.NewTask(TaskFXDailyRates, []byte(`{"date":"bad"}`))); !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("expected SkipRetry, got %v", err)
	}
}
