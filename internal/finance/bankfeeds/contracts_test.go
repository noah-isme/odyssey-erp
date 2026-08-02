package bankfeeds

import (
	"context"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

var _ FeedPort = fakeFeed{}
var _ WebhookVerifier = fakeFeed{}

type fakeFeed struct{}

func (fakeFeed) ValidateConnection(context.Context, automation.ConnectionRef) error { return nil }
func (fakeFeed) ListAccounts(context.Context, automation.ConnectionRef) ([]Account, error) {
	return nil, nil
}
func (fakeFeed) Balances(context.Context, automation.ConnectionRef, []automation.ExternalReference) ([]Balance, error) {
	return nil, nil
}
func (fakeFeed) Transactions(context.Context, SyncRequest) (TransactionPage, error) {
	return TransactionPage{}, nil
}
func (fakeFeed) VerifyWebhook(context.Context, automation.ConnectionRef, map[string]string, []byte) (InboundEvent, error) {
	return InboundEvent{}, nil
}
