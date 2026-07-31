package fx

import (
	"context"
	"fmt"
	"time"
)

type OverrideInput struct {
	ActorID                     int64
	BaseCurrency, QuoteCurrency string
	Rate                        Decimal
	RateDate                    time.Time
	Reason                      string
}

type OverrideAuthorizer interface {
	AuthorizeFXOverride(context.Context, int64) error
}
type OverrideRepository interface {
	UpsertManualRate(context.Context, FXQuoteSet) error
}
type OverrideAudit interface {
	RecordFXOverride(context.Context, OverrideInput) error
}

type OverrideService struct {
	Authorizer OverrideAuthorizer
	Repo       OverrideRepository
	Audit      OverrideAudit
}

func (s *OverrideService) SetRate(ctx context.Context, in OverrideInput) error {
	if s == nil || s.Authorizer == nil || s.Repo == nil || s.Audit == nil {
		return fmt.Errorf("fx override: authorizer, repository, and audit are required")
	}
	if err := s.Authorizer.AuthorizeFXOverride(ctx, in.ActorID); err != nil {
		return fmt.Errorf("%w: %v", ErrManualOverrideDenied, err)
	}
	base, err := Currency(in.BaseCurrency)
	if err != nil {
		return err
	}
	quote, err := Currency(in.QuoteCurrency)
	if err != nil {
		return err
	}
	if base == quote || in.Rate.Cmp(MustDecimal("0")) <= 0 || in.Reason == "" {
		return ErrInvalidRate
	}
	if err := s.Repo.UpsertManualRate(ctx, FXQuoteSet{BaseCurrency: base, RateDate: in.RateDate, Source: "MANUAL", SourceReference: in.Reason, Rates: map[string]Decimal{quote: in.Rate}}); err != nil {
		return err
	}
	return s.Audit.RecordFXOverride(ctx, in)
}
