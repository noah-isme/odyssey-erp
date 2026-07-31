package fx

import (
	"context"
	"testing"
)

type overrideAuth struct{ err error }

func (a overrideAuth) AuthorizeFXOverride(context.Context, int64) error { return a.err }

type overrideRepo struct{ called bool }

func (r *overrideRepo) UpsertManualRate(context.Context, FXQuoteSet) error {
	r.called = true
	return nil
}

type overrideAudit struct{ called bool }

func (a *overrideAudit) RecordFXOverride(context.Context, OverrideInput) error {
	a.called = true
	return nil
}

func TestOverrideRequiresAuthorization(t *testing.T) {
	r := &overrideRepo{}
	a := &overrideAudit{}
	s := OverrideService{Authorizer: overrideAuth{err: ErrManualOverrideDenied}, Repo: r, Audit: a}
	if err := s.SetRate(context.Background(), OverrideInput{ActorID: 7, BaseCurrency: "IDR", QuoteCurrency: "USD", Rate: MustDecimal("15000"), Reason: "approved"}); err == nil {
		t.Fatal("expected authorization error")
	}
	if r.called || a.called {
		t.Fatal("unauthorized override must not persist or audit")
	}
}
