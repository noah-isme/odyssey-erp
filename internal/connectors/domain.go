package connectors

import (
	"context"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// ConnectionStatus represents the health and state of a provider connection.
type ConnectionStatus string

const (
	StatusHealthy        ConnectionStatus = "healthy"
	StatusDegraded       ConnectionStatus = "degraded"
	StatusActionRequired ConnectionStatus = "action_required"
	StatusDisabled       ConnectionStatus = "disabled"
)

// Connection represents a company-scoped integration with an external provider.
type Connection struct {
	ID             int64
	CompanyID      int64
	Provider       string // e.g., "stripe", "dhl", "shopify"
	Type           string // e.g., "payment", "shipping", "marketplace"
	Name           string // User-friendly name
	SecretRef      string // Reference to encrypted secret or vault key
	Status         ConnectionStatus
	LastSync       *time.Time
	LastError      *string
	TokenExpiry    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// GetCredentials securely decrypts the SecretRef using the application Vault.
func (c *Connection) GetCredentials(vault *shared.Vault) (string, error) {
	if c.SecretRef == "" {
		return "", nil
	}
	return vault.DecryptSecure(c.SecretRef)
}

// SetCredentials encrypts the plaintext API key/secret and stores it in SecretRef.
func (c *Connection) SetCredentials(vault *shared.Vault, plaintext string) error {
	cipher, err := vault.EncryptSecure(plaintext)
	if err != nil {
		return err
	}
	c.SecretRef = cipher
	return nil
}

// ProviderAdapter defines the common interface every external provider must implement.
type ProviderAdapter interface {
	// ValidateConnection tests credentials and basic connectivity.
	ValidateConnection(ctx context.Context, conn *Connection) error
	
	// CheckHealth returns the current status of the provider.
	CheckHealth(ctx context.Context, conn *Connection) (ConnectionStatus, error)
	
	// RefreshToken attempts to refresh OAuth tokens if applicable.
	RefreshToken(ctx context.Context, conn *Connection) error
	
	// VerifyCallbackSignature ensures an incoming webhook is authentic.
	VerifyCallbackSignature(ctx context.Context, payload []byte, signature string) error

	// ExecuteCommand handles an outbound command for this provider.
	ExecuteCommand(ctx context.Context, conn *Connection, cmd *OutboxCommand) error

	// TranslateWebhook parses a raw provider webhook into zero or more canonical events.
	TranslateWebhook(ctx context.Context, conn *Connection, providerEventID string, payload []byte) ([]*CanonicalEvent, error)
}

// CanonicalEvent represents a provider-neutral event produced for domain modules.
type CanonicalEvent struct {
	ID            int64
	CompanyID     int64
	ConnectionID  int64
	EventType     string // e.g., "payment.captured", "shipment.delivered"
	EventTime     time.Time
	CorrelationID string
	CausationID   string
	Payload       []byte // JSON encoded canonical schema
	CreatedAt     time.Time
}

// OutboxCommand represents a domain module's request for a provider action.
type OutboxCommand struct {
	ID            int64
	CompanyID     int64
	ConnectionID  int64
	CommandType   string // e.g., "payment.create", "shipment.book"
	CorrelationID string
	Payload       []byte // JSON encoded canonical schema
	State         string // "pending", "processing", "completed", "failed", "dead_letter"
	Attempts      int
	NextAttempt   time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// InboxEvent represents a raw, deduplicated provider callback or polling result.
type InboxEvent struct {
	ID              int64
	CompanyID       int64
	ConnectionID    int64
	ProviderEventID string // Used for deduplication
	RawPayload      []byte
	Processed       bool
	CreatedAt       time.Time
	ProcessedAt     *time.Time
}
