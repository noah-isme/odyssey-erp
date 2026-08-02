// Package bankfeeds defines provider-neutral bank-feed contracts.
package bankfeeds

import (
	"context"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

// Account is a provider account mapped to an Odyssey bank account in a later
// phase. It intentionally contains no credentials or raw provider payload.
type Account struct {
	Reference automation.ExternalReference
	Name      string
	Currency  string
}

type Balance struct {
	Account   automation.ExternalReference
	Amount    automation.ExactAmount
	AsOf      time.Time
	Available bool
}

// Transaction is normalized before it reaches the banking statement service.
// ProviderTransactionID must be stable when the provider supplies one.
type Transaction struct {
	Reference             automation.ExternalReference
	Account               automation.ExternalReference
	Amount                automation.ExactAmount
	BookedAt              time.Time
	ValueDate             time.Time
	Description           string
	CounterpartyReference string
	ProviderStatus        string
}

type SyncRequest struct {
	Connection automation.ConnectionRef
	Account    automation.ExternalReference
	Cursor     string
	From       time.Time
	To         time.Time
}

type TransactionPage struct {
	Transactions []Transaction
	NextCursor   string
	HasMore      bool
}

// InboundEvent is verified by the adapter before it enters the durable inbox
// in the bank-feed application service.
type InboundEvent struct {
	Reference   automation.ExternalReference
	OccurredAt  time.Time
	EventType   string
	PayloadHash string
}

// FeedPort is implemented by a provider adapter. Polling and callback paths
// must converge on the same normalized transaction ingestion service.
type FeedPort interface {
	ValidateConnection(context.Context, automation.ConnectionRef) error
	ListAccounts(context.Context, automation.ConnectionRef) ([]Account, error)
	Balances(context.Context, automation.ConnectionRef, []automation.ExternalReference) ([]Balance, error)
	Transactions(context.Context, SyncRequest) (TransactionPage, error)
}

// WebhookVerifier is optional because not every bank provider supports webhooks.
type WebhookVerifier interface {
	VerifyWebhook(context.Context, automation.ConnectionRef, map[string]string, []byte) (InboundEvent, error)
}
