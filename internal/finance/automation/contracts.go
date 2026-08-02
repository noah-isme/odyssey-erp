// Package automation defines the shared contracts for finance automation.
// It deliberately contains no provider SDKs or domain mutation logic.
package automation

import (
	"errors"
	"fmt"
	"strings"
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	"github.com/odyssey-erp/odyssey-erp/internal/fx"
)

var (
	ErrInvalidReference = errors.New("finance automation: invalid external reference")
	ErrInvalidAmount    = errors.New("finance automation: invalid exact amount")
)

// ErrorCategory determines whether a provider failure may be retried safely.
type ErrorCategory string

const (
	ErrorInvalidRequest ErrorCategory = "INVALID_REQUEST"
	ErrorAuthentication ErrorCategory = "AUTHENTICATION"
	ErrorConfiguration  ErrorCategory = "CONFIGURATION"
	ErrorRateLimited    ErrorCategory = "RATE_LIMITED"
	ErrorTransient      ErrorCategory = "TRANSIENT"
	ErrorPermanent      ErrorCategory = "PERMANENT"
	ErrorAmbiguous      ErrorCategory = "AMBIGUOUS"
	ErrorDuplicate      ErrorCategory = "DUPLICATE"
)

// ProviderError retains a safe, typed failure boundary. Credentials and raw
// provider bodies must not be copied into Message.
type ProviderError struct {
	Category     ErrorCategory
	Operation    string
	ProviderCode string
	Message      string
	RetryAfter   time.Duration
	Err          error
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return fmt.Sprintf("finance automation %s: %s", e.Operation, e.Message)
	}
	if e.Err != nil {
		return fmt.Sprintf("finance automation %s: %v", e.Operation, e.Err)
	}
	return fmt.Sprintf("finance automation %s failed", e.Operation)
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Retryable reports whether the caller may retry after checking the relevant
// provider status. Ambiguous execution never authorizes blind resubmission.
func (e *ProviderError) Retryable() bool {
	if e == nil {
		return false
	}
	return e.Category == ErrorRateLimited || e.Category == ErrorTransient
}

// Correlation ties a command, callback, audit event, and worker attempt to
// the same business operation.
type Correlation struct {
	ID          string
	CausationID string
}

func (c Correlation) Validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return errors.New("finance automation: correlation id required")
	}
	return nil
}

// ConnectionRef identifies a company-owned external connection without
// carrying credentials or provider configuration.
type ConnectionRef struct {
	CompanyID    int64
	ConnectionID int64
	Provider     string
}

func (r ConnectionRef) Validate() error {
	if r.CompanyID <= 0 || r.ConnectionID <= 0 || strings.TrimSpace(r.Provider) == "" {
		return ErrInvalidReference
	}
	return nil
}

// ExternalReference is the canonical stable identity for a provider object.
// Provider object IDs must be immutable and unique within a connection/type.
type ExternalReference struct {
	Connection ConnectionRef
	ObjectType string
	ObjectID   string
}

func (r ExternalReference) Validate() error {
	if err := r.Connection.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.ObjectType) == "" || strings.TrimSpace(r.ObjectID) == "" {
		return ErrInvalidReference
	}
	return nil
}

// ExactAmount is the only money boundary for new finance automation ports.
// Amount uses the exact accounting money representation; Currency is ISO 4217.
type ExactAmount struct {
	Amount   accountingmoney.Money
	Currency string
}

func (a ExactAmount) Validate() error {
	if _, err := fx.Currency(a.Currency); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidAmount, err)
	}
	if _, err := accountingmoney.Parse(a.Amount.String(), a.Amount.Scale); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidAmount, err)
	}
	return nil
}

// RetryPolicy is persisted alongside a command's attempt metadata. It does
// not decide provider semantics; adapters classify failures using ProviderError.
type RetryPolicy struct {
	MaxAttempts int
	Lease       time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 10, Lease: 10 * time.Minute}
}

func (p RetryPolicy) Validate() error {
	if p.MaxAttempts < 1 || p.MaxAttempts > 100 || p.Lease <= 0 {
		return errors.New("finance automation: invalid retry policy")
	}
	return nil
}
