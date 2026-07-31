package fx

import (
	"context"
	"time"
)

type Operations struct {
	Service    *Service
	Repository *SQLRepository
}

func (o Operations) Fetch(ctx context.Context, date time.Time, force bool) error {
	currencies, err := o.Repository.CompanyBaseCurrencies(ctx)
	if err != nil {
		return err
	}
	for _, currency := range currencies {
		if _, err := o.Service.FetchDailyRates(ctx, currency, date, force); err != nil {
			return err
		}
	}
	return nil
}
func (o Operations) Status(ctx context.Context, date time.Time) ([]RateStatus, error) {
	return o.Repository.Status(ctx, date)
}
