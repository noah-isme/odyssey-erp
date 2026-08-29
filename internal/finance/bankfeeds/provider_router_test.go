package bankfeeds

import (
	"context"
	"errors"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

type routerFeed struct{ validationCalls int }

func (p *routerFeed) ValidateConnection(context.Context, automation.ConnectionRef) error {
	p.validationCalls++
	return nil
}
func (routerFeed) ListAccounts(context.Context, automation.ConnectionRef) ([]Account, error) {
	return nil, nil
}
func (routerFeed) Balances(context.Context, automation.ConnectionRef, []automation.ExternalReference) ([]Balance, error) {
	return nil, nil
}
func (routerFeed) Transactions(context.Context, SyncRequest) (TransactionPage, error) {
	return TransactionPage{}, nil
}

func TestProviderRouterNormalizesNamesAndReportsRegisteredProviders(t *testing.T) {
	feed := &routerFeed{}
	router := NewProviderRouter(map[string]FeedPort{
		" Bank-Feed ": feed,
		"":            feed,
		"ignored":     nil,
	})

	if got, want := router.Providers(), []string{"bank_feed"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("Providers() = %#v, want %#v", got, want)
	}
	port, err := router.port(automation.ConnectionRef{CompanyID: 1, ConnectionID: 2, Provider: "BANK_FEED"})
	if err != nil {
		t.Fatalf("port() error = %v", err)
	}
	if port == nil {
		t.Fatal("port() returned nil adapter")
	}
}

func TestProviderRouterForwardsConnectionValidation(t *testing.T) {
	feed := &routerFeed{}
	router := NewProviderRouter(map[string]FeedPort{"bank-feed": feed})
	ref := automation.ConnectionRef{CompanyID: 1, ConnectionID: 2, Provider: "BANK_FEED"}

	if err := router.ValidateConnection(context.Background(), ref); err != nil {
		t.Fatalf("ValidateConnection() error = %v", err)
	}
	if feed.validationCalls != 1 {
		t.Fatalf("validation_calls = %d, want 1", feed.validationCalls)
	}
}

func TestProviderRouterFailsClosedWhenProviderIsMissing(t *testing.T) {
	router := NewProviderRouter(nil)
	_, err := router.port(automation.ConnectionRef{CompanyID: 1, ConnectionID: 2, Provider: "bank"})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("port() error = %v, want ErrProviderUnavailable", err)
	}

	var nilRouter *ProviderRouter
	_, err = nilRouter.port(automation.ConnectionRef{CompanyID: 1, ConnectionID: 2, Provider: "bank"})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("nil router port() error = %v, want ErrProviderUnavailable", err)
	}
}

func TestProviderRouterFailsClosedOnNormalizedNameCollision(t *testing.T) {
	router := NewProviderRouter(map[string]FeedPort{
		"bank-feed": &routerFeed{},
		"bank_feed": &routerFeed{},
	})

	if got := router.Providers(); got != nil {
		t.Fatalf("Providers() = %#v, want nil for invalid registry", got)
	}
	_, err := router.port(automation.ConnectionRef{CompanyID: 1, ConnectionID: 2, Provider: "bank-feed"})
	if !errors.Is(err, ErrProviderUnavailable) || !errors.Is(err, ErrInvalidProviderRegistration) {
		t.Fatalf("port() error = %v, want provider-unavailable and invalid-registration", err)
	}
}

func TestProviderRouterRejectsInvalidConnectionBeforeLookup(t *testing.T) {
	_, err := NewProviderRouter(map[string]FeedPort{"bank": &routerFeed{}}).port(automation.ConnectionRef{Provider: "bank"})
	if !errors.Is(err, automation.ErrInvalidReference) {
		t.Fatalf("port() error = %v, want ErrInvalidReference", err)
	}
}
