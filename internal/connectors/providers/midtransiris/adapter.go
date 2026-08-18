// Package midtransiris implements the provider-neutral supplier payment
// execution port using Midtrans Payouts (formerly Iris).
//
// The package deliberately does not depend on the existing Midtrans Snap
// adapter. Supplier disbursements have different credentials, idempotency,
// and lifecycle semantics from customer collections. The legacy Iris API is
// supported for existing merchants; credentials containing BI-SNAP fields
// opt into the signed/tokenized transport.
package midtransiris

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/payments"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

const (
	Provider = "midtrans_iris"

	sandboxLegacyBaseURL    = "https://app.sandbox.midtrans.com/iris/api/v1"
	productionLegacyBaseURL = "https://app.midtrans.com/iris/api/v1"
	sandboxBISNAPBaseURL    = "https://merchants.sbx.midtrans.com"
	productionBISNAPBaseURL = "https://merchants.midtrans.com"

	defaultLegacyPayoutPath = "/payouts"
	defaultLegacyLookupPath = "/payouts/"
	defaultLegacyPingPath   = "/ping"
	defaultLegacyRejectPath = "/payouts/reject"

	defaultBISNAPTokenPath  = "/v1.0/access-token/b2b"
	defaultBISNAPSubmitPath = "/v1.0/debit/payment-host-to-host"
	defaultBISNAPLookupPath = "/v1.0/debit/status"
	defaultBISNAPCancelPath = "/v1.0/debit/cancel"

	maxProviderMessage = 256
)

var (
	ErrInvalidCredentials       = errors.New("midtrans iris: invalid credentials")
	ErrUnsupportedCurrency      = errors.New("midtrans iris: currency is not supported")
	ErrInvalidBeneficiary       = errors.New("midtrans iris: invalid beneficiary reference")
	ErrMissingProviderReference = errors.New("midtrans iris: provider reference is missing")
)

// ProviderError and ErrorCategory are aliases for the shared finance
// automation boundary, allowing callers to inspect retry/ambiguity without
// importing a provider-specific error implementation.
type ProviderError = automation.ProviderError
type ErrorCategory = automation.ErrorCategory

const (
	ErrorRetryable = automation.ErrorTransient
	ErrorAmbiguous = automation.ErrorAmbiguous
)

func IsAmbiguous(err error) bool {
	var providerErr *automation.ProviderError
	return errors.As(err, &providerErr) && providerErr != nil && providerErr.Category == automation.ErrorAmbiguous
}

func IsRetryable(err error) bool {
	var providerErr *automation.ProviderError
	return errors.As(err, &providerErr) && providerErr != nil && providerErr.Retryable()
}

// Credentials is the structured, vaulted Midtrans credential payload. APIKey
// is the Iris/Payouts key used with the legacy API. BI-SNAP credentials are
// optional and, when complete, select the tokenized request path.
//
// PrivateKeyPEM is decrypted only for the duration of an access-token request
// and is never retained in the adapter. AccessToken is intended for short-lived
// contract tests or an externally managed token broker; production should use
// ClientID + PrivateKeyPEM so the adapter can refresh a token itself.
type Credentials struct {
	APIKey     string `json:"api_key,omitempty"`
	IrisAPIKey string `json:"iris_api_key,omitempty"`
	ServerKey  string `json:"server_key,omitempty"`
	IsProd     bool   `json:"is_prod,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`

	ClientID      string `json:"client_id,omitempty"`
	ClientSecret  string `json:"client_secret,omitempty"`
	PartnerID     string `json:"partner_id,omitempty"`
	MerchantID    string `json:"merchant_id,omitempty"`
	PrivateKeyPEM string `json:"private_key_pem,omitempty"`
	PrivateKey    string `json:"private_key,omitempty"`
	AccessToken   string `json:"access_token,omitempty"`

	ChannelID string `json:"channel_id,omitempty"`
	DeviceID  string `json:"device_id,omitempty"`

	TokenPath  string `json:"token_path,omitempty"`
	SubmitPath string `json:"submit_path,omitempty"`
	LookupPath string `json:"lookup_path,omitempty"`
	CancelPath string `json:"cancel_path,omitempty"`
	PingPath   string `json:"ping_path,omitempty"`
}

func (c Credentials) key() string {
	for _, value := range []string{c.APIKey, c.IrisAPIKey, c.ServerKey} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (c Credentials) biSnap() bool {
	return strings.TrimSpace(c.ClientID) != "" || strings.TrimSpace(c.ClientSecret) != "" || strings.TrimSpace(c.PartnerID) != "" || strings.TrimSpace(c.PrivateKeyPEM) != "" || strings.TrimSpace(c.PrivateKey) != ""
}

// Beneficiary is the bank destination required by the Iris payout API. A
// resolver may populate it from a company-owned beneficiary record. The
// adapter also accepts this shape encoded as JSON in Instruction.BeneficiaryRef.
type Beneficiary struct {
	Name      string `json:"name,omitempty"`
	Account   string `json:"account,omitempty"`
	Bank      string `json:"bank,omitempty"`
	Email     string `json:"email,omitempty"`
	AliasName string `json:"alias_name,omitempty"`
}

// CredentialResolver resolves a company-scoped reference through the
// application's secret/configuration boundary. The returned credentials must
// be structured in memory and must not be retained by the resolver.
type CredentialResolver func(context.Context, automation.ConnectionRef) (Credentials, error)

// SecretResolver is a convenience for applications whose connection
// repository already returns the decrypted JSON string.
type SecretResolver func(context.Context, automation.ConnectionRef) (string, error)

// ConnectionResolver adapts the existing connector connection repository to
// this provider-neutral port. The adapter verifies company and connection IDs
// before asking ProviderOptions to decrypt the secret.
type ConnectionResolver func(context.Context, automation.ConnectionRef) (*connectors.Connection, error)

// BeneficiaryResolver resolves a stable local beneficiary reference to bank
// details. It is preferable to embedding account data in an instruction.
type BeneficiaryResolver func(context.Context, string) (Beneficiary, error)

// ScopedBeneficiaryResolver retains the company-owned connection scope while
// resolving a local beneficiary reference. Live wiring should use this form
// so a reference cannot resolve another tenant's bank account.
type ScopedBeneficiaryResolver func(context.Context, automation.ConnectionRef, string) (Beneficiary, error)

// Options controls the transport and credential boundary. ProviderOptions is
// deliberately embedded so the same vault, HTTP client, TLS, and retry
// conventions as the other connectors remain available.
type Options struct {
	ProviderOptions connectors.ProviderOptions

	// ConnectionResolver is the preferred name. CredentialResolver remains as
	// a source-compatible alias for an early version of this package.
	ConnectionResolver        ConnectionResolver
	CredentialResolver        ConnectionResolver
	Credentials               CredentialResolver
	SecretResolver            SecretResolver
	BeneficiaryResolver       BeneficiaryResolver
	ScopedBeneficiaryResolver ScopedBeneficiaryResolver

	// StaticCredentials is accepted only when DevelopmentMode or
	// AllowPlaintextCredentials is enabled. It exists for deterministic
	// contract tests and local development, never application wiring.
	StaticCredentials Credentials

	Now       func() time.Time
	ChannelID string
	DeviceID  string
}

// Adapter implements payments.ExecutionPort for Midtrans Payouts/Iris.
type Adapter struct {
	logger  *slog.Logger
	options Options

	mu                   sync.Mutex
	tokens               map[string]tokenCacheEntry
	refs                 map[string]string
	providerInstructions map[string]automation.ExternalReference
}

type tokenCacheEntry struct {
	token     string
	expiresAt time.Time
}

// NewAdapter creates an Iris adapter. The vault argument is retained to make
// construction parallel with the existing connector adapters. Production
// connections should resolve through ConnectionResolver and that vault; tests
// may pass AllowPlaintextCredentials in Options.ProviderOptions.
func NewAdapter(logger *slog.Logger, vault *shared.Vault, options ...Options) *Adapter {
	var opts Options
	if len(options) > 0 {
		opts = options[0]
	}
	if opts.ProviderOptions.Vault == nil {
		opts.ProviderOptions.Vault = vault
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{
		logger:               logger,
		options:              opts,
		tokens:               make(map[string]tokenCacheEntry),
		refs:                 make(map[string]string),
		providerInstructions: make(map[string]automation.ExternalReference),
	}
}

// NewAdapterWithOptions is a constructor for callers that do not need the
// legacy vault argument.
func NewAdapterWithOptions(logger *slog.Logger, options Options) *Adapter {
	return NewAdapter(logger, nil, options)
}

var _ payments.ExecutionPort = (*Adapter)(nil)

// ValidateConnection checks credentials and performs a non-mutating provider
// probe. BI-SNAP validation obtains an access token; legacy validation calls
// Iris' ping endpoint.
func (a *Adapter) ValidateConnection(ctx context.Context, ref automation.ConnectionRef) error {
	if err := validateConnectionRef(ref); err != nil {
		return err
	}
	creds, err := a.resolveCredentials(ctx, ref)
	if err != nil {
		return err
	}
	if creds.biSnap() {
		if _, err := a.accessToken(ctx, ref, creds); err != nil {
			return fmt.Errorf("midtrans iris: validate BI-SNAP credentials: %w", err)
		}
		return nil
	}
	if creds.key() == "" {
		return fmt.Errorf("%w: API key is required", ErrInvalidCredentials)
	}
	_, _, err = a.request(ctx, ref, creds, http.MethodGet, a.pathURL(creds, pathOr(creds.PingPath, defaultLegacyPingPath)), nil, requestOptions{operation: "validate", sideEffect: false, legacy: true})
	return err
}

// Submit creates one payout. The local instruction reference is used as the
// stable idempotency identity; retries should use Lookup when the outcome is
// unknown and must never blindly create another payout.
func (a *Adapter) Submit(ctx context.Context, ref automation.ConnectionRef, instruction payments.Instruction) (payments.Submission, error) {
	if err := validateConnectionRef(ref); err != nil {
		return payments.Submission{}, err
	}
	if instruction.Reference.Connection != ref {
		return payments.Submission{}, fmt.Errorf("%w: instruction connection mismatch", automation.ErrInvalidReference)
	}
	if err := instruction.Validate(); err != nil {
		return payments.Submission{}, err
	}
	if err := validateIDR(instruction.Amount); err != nil {
		return payments.Submission{}, err
	}
	creds, err := a.resolveCredentials(ctx, ref)
	if err != nil {
		return payments.Submission{}, err
	}
	beneficiary, err := a.resolveBeneficiary(ctx, ref, instruction.BeneficiaryRef, instruction.BeneficiaryName)
	if err != nil {
		return payments.Submission{}, err
	}

	var result payoutResult
	if creds.biSnap() {
		body, marshalErr := a.biSnapSubmitBody(instruction, beneficiary)
		if marshalErr != nil {
			return payments.Submission{}, marshalErr
		}
		result, err = a.biSnapPayout(ctx, ref, creds, instruction, body)
	} else {
		body, marshalErr := json.Marshal(legacyPayoutRequest{Payouts: []legacyPayoutDetail{{
			BeneficiaryName:    beneficiary.Name,
			BeneficiaryAccount: beneficiary.Account,
			BeneficiaryBank:    beneficiary.Bank,
			BeneficiaryEmail:   beneficiary.Email,
			Amount:             instruction.Amount.Amount.String(),
			Notes:              payoutNotes(instruction),
		}}})
		if marshalErr != nil {
			return payments.Submission{}, fmt.Errorf("midtrans iris: encode payout: %w", marshalErr)
		}
		result, err = a.legacyPayout(ctx, ref, creds, instruction, body)
	}
	if err != nil {
		return payments.Submission{}, err
	}
	if err := validateReturnedAmount("submit", instruction.Amount, result.Amount); err != nil {
		return payments.Submission{}, err
	}
	providerID := strings.TrimSpace(result.ReferenceNo)
	if providerID == "" {
		return payments.Submission{}, providerError("submit", automation.ErrorAmbiguous, "MISSING_REFERENCE", ErrMissingProviderReference)
	}
	a.rememberProviderID(instruction.Reference, providerID)
	a.logProviderResult(ref, "submit", providerID, result.Status)
	return payments.Submission{
		Reference:  providerReference(ref, providerID),
		Status:     normalizeProviderStatus(result.Status),
		OccurredAt: result.OccurredAt,
	}, nil
}

// Lookup retrieves the authoritative provider state for the stable payout
// reference. The argument is normally the original instruction reference;
// provider references are represented by ObjectID when callers have one.
func (a *Adapter) Lookup(ctx context.Context, ref automation.ConnectionRef, payoutRef automation.ExternalReference) (payments.Settlement, error) {
	return a.lookupWithInstruction(ctx, ref, payoutRef, a.originalInstructionReference(payoutRef))
}

// LookupWithInstruction resolves a persisted provider reference while
// retaining the canonical local instruction identity in the settlement.
func (a *Adapter) LookupWithInstruction(ctx context.Context, ref automation.ConnectionRef, instructionRef automation.ExternalReference, payoutRef automation.ExternalReference) (payments.Settlement, error) {
	return a.lookupWithInstruction(ctx, ref, payoutRef, instructionRef)
}

func (a *Adapter) lookupWithInstruction(ctx context.Context, ref automation.ConnectionRef, payoutRef, instructionRef automation.ExternalReference) (payments.Settlement, error) {
	if err := validateConnectionRef(ref); err != nil {
		return payments.Settlement{}, err
	}
	if err := payoutRef.Validate(); err != nil {
		return payments.Settlement{}, err
	}
	if err := instructionRef.Validate(); err != nil {
		return payments.Settlement{}, err
	}
	if payoutRef.Connection != ref {
		return payments.Settlement{}, fmt.Errorf("%w: lookup connection mismatch", automation.ErrInvalidReference)
	}
	if instructionRef.Connection != ref {
		return payments.Settlement{}, fmt.Errorf("%w: lookup instruction connection mismatch", automation.ErrInvalidReference)
	}
	creds, err := a.resolveCredentials(ctx, ref)
	if err != nil {
		return payments.Settlement{}, err
	}
	providerID := a.providerID(payoutRef)
	if providerID == "" {
		return payments.Settlement{}, fmt.Errorf("%w: payout ID is required", automation.ErrInvalidReference)
	}
	providerLookupRef := payoutRef
	providerLookupRef.ObjectType = "midtrans_iris_payout"
	providerLookupRef.ObjectID = providerID
	var result payoutResult
	if creds.biSnap() {
		result, err = a.biSnapLookup(ctx, ref, creds, providerLookupRef)
	} else {
		result, err = a.legacyLookup(ctx, ref, creds, providerLookupRef)
	}
	if err != nil {
		return payments.Settlement{}, err
	}
	settlement, err := a.settlementFromResult(ref, instructionRef, result)
	if err != nil {
		return payments.Settlement{}, err
	}
	a.logProviderResult(ref, "lookup", result.ReferenceNo, result.Status)
	return settlement, nil
}

// Cancel rejects a requested Iris payout. Iris exposes cancellation as the
// reject endpoint; once a payout is processing, the resulting ambiguity must
// be resolved by Lookup.
func (a *Adapter) Cancel(ctx context.Context, ref automation.ConnectionRef, payoutRef automation.ExternalReference) (payments.Settlement, error) {
	return a.cancelWithInstruction(ctx, ref, payoutRef, a.originalInstructionReference(payoutRef))
}

// CancelWithInstruction is the durable-reference cancellation variant used
// by the coordinator when a provider submission reference is persisted.
func (a *Adapter) CancelWithInstruction(ctx context.Context, ref automation.ConnectionRef, instructionRef automation.ExternalReference, payoutRef automation.ExternalReference) (payments.Settlement, error) {
	return a.cancelWithInstruction(ctx, ref, payoutRef, instructionRef)
}

func (a *Adapter) cancelWithInstruction(ctx context.Context, ref automation.ConnectionRef, payoutRef, instructionRef automation.ExternalReference) (payments.Settlement, error) {
	if err := validateConnectionRef(ref); err != nil {
		return payments.Settlement{}, err
	}
	if err := payoutRef.Validate(); err != nil {
		return payments.Settlement{}, err
	}
	if payoutRef.Connection != ref {
		return payments.Settlement{}, fmt.Errorf("%w: cancel connection mismatch", automation.ErrInvalidReference)
	}
	if err := instructionRef.Validate(); err != nil {
		return payments.Settlement{}, err
	}
	if instructionRef.Connection != ref {
		return payments.Settlement{}, fmt.Errorf("%w: cancel instruction connection mismatch", automation.ErrInvalidReference)
	}
	creds, err := a.resolveCredentials(ctx, ref)
	if err != nil {
		return payments.Settlement{}, err
	}
	providerID := a.providerID(payoutRef)
	if providerID == "" {
		return payments.Settlement{}, fmt.Errorf("%w: payout ID is required", automation.ErrInvalidReference)
	}
	providerLookupRef := payoutRef
	providerLookupRef.ObjectType = "midtrans_iris_payout"
	providerLookupRef.ObjectID = providerID
	var result payoutResult
	if creds.biSnap() {
		result, err = a.biSnapCancel(ctx, ref, creds, providerLookupRef)
	} else {
		result, err = a.legacyCancel(ctx, ref, creds, providerLookupRef)
	}
	if err != nil {
		return payments.Settlement{}, err
	}
	result.Status = mapCancelStatus(result.Status)
	settlement, err := a.settlementFromResult(ref, instructionRef, result)
	if err != nil {
		return payments.Settlement{}, err
	}
	a.logProviderResult(ref, "cancel", result.ReferenceNo, result.Status)
	return settlement, nil
}

// GenerateFile is intentionally unsupported: file export is owned by the
// controlled bank-file path, not a live provider adapter.
func (a *Adapter) GenerateFile(context.Context, automation.ConnectionRef, []payments.Instruction) (payments.ExportArtifact, error) {
	return payments.ExportArtifact{}, fmt.Errorf("midtrans iris: generate file: %w", connectors.ErrUnsupportedOperation)
}

type requestOptions struct {
	operation  string
	sideEffect bool
	legacy     bool
	biSnap     bool
	headers    http.Header
}

type legacyPayoutRequest struct {
	Payouts []legacyPayoutDetail `json:"payouts"`
}

type legacyPayoutDetail struct {
	BeneficiaryName    string `json:"beneficiary_name"`
	BeneficiaryAccount string `json:"beneficiary_account"`
	BeneficiaryBank    string `json:"beneficiary_bank"`
	BeneficiaryEmail   string `json:"beneficiary_email,omitempty"`
	Amount             string `json:"amount"`
	Notes              string `json:"notes,omitempty"`
}

type biSnapPayoutRequest struct {
	PartnerReferenceNo   string       `json:"partnerReferenceNo"`
	Amount               biSnapAmount `json:"amount"`
	BeneficiaryAccountNo string       `json:"beneficiaryAccountNo,omitempty"`
	BeneficiaryBankCode  string       `json:"beneficiaryBankCode,omitempty"`
	BeneficiaryName      string       `json:"beneficiaryName,omitempty"`
	Remark               string       `json:"remark,omitempty"`
}

type biSnapAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type payoutResult struct {
	ReferenceNo       string
	Status            string
	Amount            automation.ExactAmount
	Fee               automation.ExactAmount
	OccurredAt        time.Time
	EndToEndReference string
	ErrorCode         string
	ErrorMessage      string
}

type legacyPayoutResponse struct {
	Payouts []struct {
		ReferenceNo  string `json:"reference_no"`
		Status       string `json:"status"`
		Amount       string `json:"amount"`
		CreatedAt    string `json:"created_at"`
		UpdatedAt    string `json:"updated_at"`
		Notes        string `json:"notes"`
		ErrorMessage string `json:"error_message"`
		ErrorCode    string `json:"error_code"`
	} `json:"payouts"`
	Status       string          `json:"status"`
	ErrorMessage string          `json:"error_message"`
	Errors       json.RawMessage `json:"errors"`
}

type legacyPayoutDetailResponse struct {
	ReferenceNo  string `json:"reference_no"`
	Status       string `json:"status"`
	Amount       string `json:"amount"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	Notes        string `json:"notes"`
	ErrorMessage string `json:"error_message"`
	ErrorCode    string `json:"error_code"`
}

type biSnapResponse struct {
	ResponseCode               string       `json:"responseCode"`
	ResponseMessage            string       `json:"responseMessage"`
	ReferenceNo                string       `json:"referenceNo"`
	PartnerReferenceNo         string       `json:"partnerReferenceNo"`
	OriginalReferenceNo        string       `json:"originalReferenceNo"`
	OriginalPartnerReferenceNo string       `json:"originalPartnerReferenceNo"`
	OriginalExternalID         string       `json:"originalExternalId"`
	Status                     string       `json:"status"`
	TransactionStatus          string       `json:"transactionStatus"`
	LatestTransactionStatus    string       `json:"latestTransactionStatus"`
	Amount                     biSnapAmount `json:"amount"`
	TransAmount                biSnapAmount `json:"transAmount"`
	Fee                        biSnapAmount `json:"fee"`
	DateTime                   string       `json:"dateTime"`
	TransactionDate            string       `json:"transactionDate"`
	PaidTime                   string       `json:"paidTime"`
	AdditionalInfo             struct {
		Status string       `json:"status"`
		Fee    biSnapAmount `json:"fee"`
	} `json:"additionalInfo"`
}

type tokenResponse struct {
	AccessToken      string          `json:"accessToken"`
	AccessTokenSnake string          `json:"access_token"`
	ExpiresIn        json.RawMessage `json:"expiresIn"`
	ExpiresInSnake   json.RawMessage `json:"expires_in"`
	ResponseCode     string          `json:"responseCode"`
	ResponseMessage  string          `json:"responseMessage"`
}

func (a *Adapter) legacyPayout(ctx context.Context, ref automation.ConnectionRef, creds Credentials, instruction payments.Instruction, body []byte) (payoutResult, error) {
	path := pathOr(creds.SubmitPath, defaultLegacyPayoutPath)
	resp, responseBody, err := a.request(ctx, ref, creds, http.MethodPost, a.pathURL(creds, path), body, requestOptions{operation: "submit", sideEffect: true, legacy: true, headers: idempotencyHeaders(instruction.Reference.ObjectID)})
	if err != nil {
		return payoutResult{}, err
	}
	var decoded legacyPayoutResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return payoutResult{}, providerError("submit", automation.ErrorPermanent, "INVALID_RESPONSE", err)
	}
	if len(decoded.Payouts) == 0 {
		return payoutResult{}, classifyProviderResponse("submit", resp.StatusCode, decoded.ErrorMessage, decoded.Errors, "")
	}
	payout := decoded.Payouts[0]
	if isFailureStatus(payout.Status) {
		return payoutResult{}, classifyProviderResponse("submit", resp.StatusCode, payout.ErrorMessage, nil, payout.ErrorCode)
	}
	amount, amountErr := exactAmount(payout.Amount, instruction.Amount.Currency)
	if amountErr != nil && strings.TrimSpace(payout.Amount) != "" {
		return payoutResult{}, providerError("submit", automation.ErrorPermanent, "INVALID_AMOUNT", amountErr)
	}
	return payoutResult{ReferenceNo: payout.ReferenceNo, Status: payout.Status, Amount: amount, OccurredAt: parseProviderTime(payout.CreatedAt, payout.UpdatedAt), EndToEndReference: payout.Notes, ErrorCode: payout.ErrorCode, ErrorMessage: payout.ErrorMessage}, nil
}

func (a *Adapter) legacyLookup(ctx context.Context, ref automation.ConnectionRef, creds Credentials, payoutRef automation.ExternalReference) (payoutResult, error) {
	path := pathOr(creds.LookupPath, defaultLegacyLookupPath) + url.PathEscape(payoutRef.ObjectID)
	resp, responseBody, err := a.request(ctx, ref, creds, http.MethodGet, a.pathURL(creds, path), nil, requestOptions{operation: "lookup", legacy: true})
	if err != nil {
		return payoutResult{}, err
	}
	var decoded legacyPayoutDetailResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return payoutResult{}, providerError("lookup", automation.ErrorPermanent, "INVALID_RESPONSE", err)
	}
	if isFailureStatus(decoded.Status) && strings.TrimSpace(decoded.ReferenceNo) == "" {
		return payoutResult{}, classifyProviderResponse("lookup", resp.StatusCode, decoded.ErrorMessage, nil, decoded.ErrorCode)
	}
	amount, amountErr := exactAmount(decoded.Amount, "IDR")
	if amountErr != nil && strings.TrimSpace(decoded.Amount) != "" {
		return payoutResult{}, providerError("lookup", automation.ErrorPermanent, "INVALID_AMOUNT", amountErr)
	}
	return payoutResult{ReferenceNo: firstNonEmpty(decoded.ReferenceNo, payoutRef.ObjectID), Status: decoded.Status, Amount: amount, OccurredAt: parseProviderTime(decoded.CreatedAt, decoded.UpdatedAt), EndToEndReference: decoded.Notes, ErrorCode: decoded.ErrorCode, ErrorMessage: decoded.ErrorMessage}, nil
}

func (a *Adapter) legacyCancel(ctx context.Context, ref automation.ConnectionRef, creds Credentials, payoutRef automation.ExternalReference) (payoutResult, error) {
	body, err := json.Marshal(map[string]any{"reference_nos": []string{payoutRef.ObjectID}, "reject_reason": "cancelled by Odyssey"})
	if err != nil {
		return payoutResult{}, fmt.Errorf("midtrans iris: encode cancel: %w", err)
	}
	path := pathOr(creds.CancelPath, defaultLegacyRejectPath)
	resp, responseBody, err := a.request(ctx, ref, creds, http.MethodPost, a.pathURL(creds, path), body, requestOptions{operation: "cancel", sideEffect: true, legacy: true, headers: idempotencyHeaders(payoutRef.ObjectID + "-cancel")})
	if err != nil {
		return payoutResult{}, err
	}
	var decoded struct {
		Status       string          `json:"status"`
		ErrorMessage string          `json:"error_message"`
		Errors       json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return payoutResult{}, providerError("cancel", automation.ErrorPermanent, "INVALID_RESPONSE", err)
	}
	if strings.TrimSpace(decoded.Status) == "" {
		return payoutResult{}, classifyProviderResponse("cancel", resp.StatusCode, decoded.ErrorMessage, decoded.Errors, "")
	}
	if isFailureStatus(decoded.Status) && !isCancelStatus(decoded.Status) {
		return payoutResult{}, classifyProviderResponse("cancel", resp.StatusCode, decoded.ErrorMessage, decoded.Errors, "")
	}
	return payoutResult{ReferenceNo: payoutRef.ObjectID, Status: decoded.Status}, nil
}

func (a *Adapter) biSnapPayout(ctx context.Context, ref automation.ConnectionRef, creds Credentials, instruction payments.Instruction, body []byte) (payoutResult, error) {
	path := pathOr(creds.SubmitPath, defaultBISNAPSubmitPath)
	resp, responseBody, err := a.biSnapRequest(ctx, ref, creds, http.MethodPost, path, body, instruction.Reference.ObjectID, "submit", true)
	if err != nil {
		return payoutResult{}, err
	}
	decoded, err := decodeBISNAPResponse(responseBody)
	if err != nil {
		return payoutResult{}, providerError("submit", automation.ErrorPermanent, "INVALID_RESPONSE", err)
	}
	if !biSnapSuccess(resp.StatusCode, decoded) {
		return payoutResult{}, classifyProviderResponse("submit", resp.StatusCode, decoded.ResponseMessage, nil, decoded.ResponseCode)
	}
	amountValue := firstNonEmpty(decoded.Amount.Value, decoded.TransAmount.Value)
	amountCurrency := firstNonEmpty(decoded.Amount.Currency, decoded.TransAmount.Currency, instruction.Amount.Currency)
	amount, amountErr := exactAmount(amountValue, amountCurrency)
	if amountErr != nil && strings.TrimSpace(amountValue) != "" {
		return payoutResult{}, providerError("submit", automation.ErrorPermanent, "INVALID_AMOUNT", amountErr)
	}
	return payoutResult{ReferenceNo: firstNonEmpty(decoded.ReferenceNo, decoded.PartnerReferenceNo, decoded.OriginalReferenceNo, decoded.OriginalPartnerReferenceNo), Status: biSnapStatus(decoded), Amount: amount, OccurredAt: parseProviderTime(decoded.DateTime, decoded.TransactionDate, decoded.PaidTime)}, nil
}

func (a *Adapter) biSnapLookup(ctx context.Context, ref automation.ConnectionRef, creds Credentials, payoutRef automation.ExternalReference) (payoutResult, error) {
	path := pathOr(creds.LookupPath, defaultBISNAPLookupPath)
	bodyPayload := map[string]string{
		"originalPartnerReferenceNo": payoutRef.ObjectID,
		"originalExternalId":         trimExternalID(payoutRef.ObjectID),
		"serviceCode":                "54",
	}
	if strings.TrimSpace(creds.MerchantID) != "" {
		bodyPayload["merchantId"] = creds.MerchantID
	}
	body, err := json.Marshal(bodyPayload)
	if err != nil {
		return payoutResult{}, fmt.Errorf("midtrans iris: encode lookup: %w", err)
	}
	resp, responseBody, err := a.biSnapRequest(ctx, ref, creds, http.MethodPost, path, body, payoutRef.ObjectID+"-lookup", "lookup", false)
	if err != nil {
		return payoutResult{}, err
	}
	decoded, err := decodeBISNAPResponse(responseBody)
	if err != nil {
		return payoutResult{}, providerError("lookup", automation.ErrorPermanent, "INVALID_RESPONSE", err)
	}
	if !biSnapSuccess(resp.StatusCode, decoded) {
		return payoutResult{}, classifyProviderResponse("lookup", resp.StatusCode, decoded.ResponseMessage, nil, decoded.ResponseCode)
	}
	amountValue := firstNonEmpty(decoded.Amount.Value, decoded.TransAmount.Value)
	amountCurrency := firstNonEmpty(decoded.Amount.Currency, decoded.TransAmount.Currency, "IDR")
	amount, amountErr := exactAmount(amountValue, amountCurrency)
	if amountErr != nil && strings.TrimSpace(amountValue) != "" {
		return payoutResult{}, providerError("lookup", automation.ErrorPermanent, "INVALID_AMOUNT", amountErr)
	}
	feeValue := firstNonEmpty(decoded.Fee.Value, decoded.AdditionalInfo.Fee.Value)
	feeCurrency := firstNonEmpty(decoded.Fee.Currency, decoded.AdditionalInfo.Fee.Currency, decoded.Amount.Currency)
	fee, feeErr := exactAmount(feeValue, feeCurrency)
	if feeErr != nil && strings.TrimSpace(feeValue) != "" {
		return payoutResult{}, providerError("lookup", automation.ErrorPermanent, "INVALID_FEE", feeErr)
	}
	return payoutResult{ReferenceNo: firstNonEmpty(decoded.ReferenceNo, decoded.PartnerReferenceNo, decoded.OriginalReferenceNo, decoded.OriginalPartnerReferenceNo, payoutRef.ObjectID), Status: biSnapStatus(decoded), Amount: amount, Fee: fee, OccurredAt: parseProviderTime(decoded.DateTime, decoded.TransactionDate, decoded.PaidTime)}, nil
}

func (a *Adapter) biSnapCancel(ctx context.Context, ref automation.ConnectionRef, creds Credentials, payoutRef automation.ExternalReference) (payoutResult, error) {
	body, err := json.Marshal(map[string]string{"originalPartnerReferenceNo": payoutRef.ObjectID, "serviceCode": "54"})
	if err != nil {
		return payoutResult{}, fmt.Errorf("midtrans iris: encode cancel: %w", err)
	}
	path := pathOr(creds.CancelPath, defaultBISNAPCancelPath)
	resp, responseBody, err := a.biSnapRequest(ctx, ref, creds, http.MethodPost, path, body, payoutRef.ObjectID+"-cancel", "cancel", true)
	if err != nil {
		return payoutResult{}, err
	}
	decoded, err := decodeBISNAPResponse(responseBody)
	if err != nil {
		return payoutResult{}, providerError("cancel", automation.ErrorPermanent, "INVALID_RESPONSE", err)
	}
	if !biSnapSuccess(resp.StatusCode, decoded) {
		return payoutResult{}, classifyProviderResponse("cancel", resp.StatusCode, decoded.ResponseMessage, nil, decoded.ResponseCode)
	}
	return payoutResult{ReferenceNo: firstNonEmpty(decoded.ReferenceNo, decoded.PartnerReferenceNo, decoded.OriginalReferenceNo, decoded.OriginalPartnerReferenceNo, payoutRef.ObjectID), Status: mapCancelStatus(biSnapStatus(decoded))}, nil
}

func (a *Adapter) biSnapSubmitBody(instruction payments.Instruction, beneficiary Beneficiary) ([]byte, error) {
	payload := biSnapPayoutRequest{
		PartnerReferenceNo:   instruction.Reference.ObjectID,
		Amount:               biSnapAmount{Value: instruction.Amount.Amount.String(), Currency: strings.ToUpper(instruction.Amount.Currency)},
		BeneficiaryAccountNo: beneficiary.Account,
		BeneficiaryBankCode:  beneficiary.Bank,
		BeneficiaryName:      beneficiary.Name,
		Remark:               payoutNotes(instruction),
	}
	return json.Marshal(payload)
}

func (a *Adapter) biSnapRequest(ctx context.Context, ref automation.ConnectionRef, creds Credentials, method, path string, body []byte, externalID, operation string, sideEffect bool) (*http.Response, []byte, error) {
	token, err := a.accessToken(ctx, ref, creds)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(externalID) == "" {
		externalID = ref.Provider + "-" + strconv.FormatInt(ref.ConnectionID, 10)
	}
	path = normalizePath(path)
	timestamp := a.now().Format(time.RFC3339)
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)
	headers.Set("X-TIMESTAMP", timestamp)
	headers.Set("X-PARTNER-ID", creds.PartnerID)
	headers.Set("X-EXTERNAL-ID", trimExternalID(externalID))
	channelID := firstNonEmpty(creds.ChannelID, a.options.ChannelID, "00000")
	deviceID := firstNonEmpty(creds.DeviceID, a.options.DeviceID, "odyssey-erp")
	headers.Set("CHANNEL-ID", channelID)
	headers.Set("X-DEVICE-ID", deviceID)
	headers.Set("Idempotency-Key", trimExternalID(externalID))
	headers.Set("X-SIGNATURE", transactionalSignature(method, path, token, body, timestamp, creds.ClientSecret))
	return a.request(ctx, ref, creds, method, a.pathURL(creds, path), body, requestOptions{operation: operation, sideEffect: sideEffect, biSnap: true, headers: headers})
}

func (a *Adapter) accessToken(ctx context.Context, ref automation.ConnectionRef, creds Credentials) (string, error) {
	if token := strings.TrimSpace(creds.AccessToken); token != "" {
		return token, nil
	}
	if strings.TrimSpace(creds.ClientID) == "" || strings.TrimSpace(creds.PrivateKeyPEM) == "" && strings.TrimSpace(creds.PrivateKey) == "" {
		return "", fmt.Errorf("%w: BI-SNAP client_id and private_key_pem are required", ErrInvalidCredentials)
	}
	cacheKey := fmt.Sprintf("%d/%d/%s", ref.CompanyID, ref.ConnectionID, creds.ClientID)
	now := a.now()
	a.mu.Lock()
	if cached, ok := a.tokens[cacheKey]; ok && cached.token != "" && now.Before(cached.expiresAt.Add(-30*time.Second)) {
		a.mu.Unlock()
		return cached.token, nil
	}
	a.mu.Unlock()

	privateKey, err := parsePrivateKey(firstNonEmpty(creds.PrivateKeyPEM, creds.PrivateKey))
	if err != nil {
		return "", providerError("access_token", automation.ErrorConfiguration, "PRIVATE_KEY", err)
	}
	timestamp := now.Format(time.RFC3339)
	signature, err := rsaSignature(privateKey, creds.ClientID+"|"+timestamp)
	if err != nil {
		return "", providerError("access_token", automation.ErrorConfiguration, "SIGNATURE", err)
	}
	body := []byte(`{"grantType":"client_credentials"}`)
	headers := http.Header{}
	headers.Set("X-TIMESTAMP", timestamp)
	headers.Set("X-CLIENT-KEY", creds.ClientID)
	headers.Set("X-SIGNATURE", signature)
	path := normalizePath(pathOr(creds.TokenPath, defaultBISNAPTokenPath))
	resp, responseBody, err := a.request(ctx, ref, creds, http.MethodPost, a.pathURL(creds, path), body, requestOptions{operation: "access_token", headers: headers, biSnap: true})
	if err != nil {
		return "", err
	}
	var token tokenResponse
	if err := json.Unmarshal(responseBody, &token); err != nil {
		return "", providerError("access_token", automation.ErrorPermanent, "INVALID_RESPONSE", err)
	}
	value := firstNonEmpty(token.AccessToken, token.AccessTokenSnake)
	if value == "" {
		return "", classifyProviderResponse("access_token", resp.StatusCode, token.ResponseMessage, nil, token.ResponseCode)
	}
	expires := parseExpiresIn(firstNonEmptyRaw(token.ExpiresIn, token.ExpiresInSnake))
	if expires <= 0 {
		expires = 15 * time.Minute
	}
	a.mu.Lock()
	a.tokens[cacheKey] = tokenCacheEntry{token: value, expiresAt: now.Add(expires)}
	a.mu.Unlock()
	return value, nil
}

func (a *Adapter) request(ctx context.Context, ref automation.ConnectionRef, creds Credentials, method, endpoint string, body []byte, opts requestOptions) (*http.Response, []byte, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, nil, providerError(opts.operation, automation.ErrorConfiguration, "ENDPOINT", errors.New("invalid provider endpoint"))
	}
	headers := make(http.Header)
	for key, values := range opts.headers {
		for _, value := range values {
			headers.Add(key, value)
		}
	}
	headers.Set("Accept", "application/json")
	headers.Set("Content-Type", "application/json")
	if opts.legacy {
		key := creds.key()
		if key == "" {
			return nil, nil, fmt.Errorf("%w: API key is required", ErrInvalidCredentials)
		}
		headers.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(key+":")))
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, nil, providerError(opts.operation, categoryForTransport(opts.sideEffect), "REQUEST", err)
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := a.options.ProviderOptions.HTTPClientOrDefault().Do(req)
	if err != nil {
		return nil, nil, providerError(opts.operation, categoryForTransport(opts.sideEffect), "TRANSPORT", err)
	}
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, nil, providerError(opts.operation, categoryForTransport(opts.sideEffect), "READ", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		providerErr := classifyProviderResponse(opts.operation, resp.StatusCode, safeResponseMessage(responseBody), nil, "")
		if opts.sideEffect && providerErrorCategory(providerErr) == automation.ErrorTransient {
			providerErr = markAmbiguous(providerErr)
		}
		return nil, nil, providerErr
	}
	return resp, responseBody, nil
}

func providerErrorCategory(err error) automation.ErrorCategory {
	var providerErr *automation.ProviderError
	if errors.As(err, &providerErr) && providerErr != nil {
		return providerErr.Category
	}
	return ""
}

func (a *Adapter) pathURL(creds Credentials, path string) string {
	base := strings.TrimSpace(creds.BaseURL)
	if base == "" && a != nil {
		base = strings.TrimSpace(a.options.ProviderOptions.BaseURL)
	}
	if base == "" {
		if creds.biSnap() {
			if creds.IsProd {
				base = productionBISNAPBaseURL
			} else {
				base = sandboxBISNAPBaseURL
			}
		} else if creds.IsProd {
			base = productionLegacyBaseURL
		} else {
			base = sandboxLegacyBaseURL
		}
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}

func (a *Adapter) resolveCredentials(ctx context.Context, ref automation.ConnectionRef) (Credentials, error) {
	if a == nil {
		return Credentials{}, ErrInvalidCredentials
	}
	var creds Credentials
	var err error
	switch {
	case a.options.Credentials != nil:
		creds, err = a.options.Credentials(ctx, ref)
	case a.options.SecretResolver != nil:
		var secret string
		secret, err = a.options.SecretResolver(ctx, ref)
		if err == nil {
			creds, err = parseCredentials(secret, true)
		}
	case a.options.ConnectionResolver != nil || a.options.CredentialResolver != nil:
		var conn *connectors.Connection
		resolver := a.options.ConnectionResolver
		if resolver == nil {
			resolver = a.options.CredentialResolver
		}
		conn, err = resolver(ctx, ref)
		if err == nil {
			if conn == nil || conn.CompanyID != ref.CompanyID || conn.ID != ref.ConnectionID {
				err = fmt.Errorf("%w: connection scope mismatch", automation.ErrInvalidReference)
			} else {
				var secret string
				secret, err = a.options.ProviderOptions.ResolveSecret(conn)
				if err == nil {
					creds, err = parseCredentials(secret, true)
				}
			}
		}
	default:
		if !a.options.ProviderOptions.DevelopmentMode && !a.options.ProviderOptions.AllowPlaintextCredentials {
			return Credentials{}, fmt.Errorf("%w: credential resolver is required", ErrInvalidCredentials)
		}
		creds, err = parseCredentialsFromStatic(a.options.StaticCredentials)
	}
	if err != nil {
		return Credentials{}, err
	}
	if strings.TrimSpace(creds.BaseURL) == "" {
		creds.BaseURL = strings.TrimSpace(a.options.ProviderOptions.BaseURL)
	}
	if creds.BaseURL != "" {
		parsed, parseErr := url.Parse(creds.BaseURL)
		if parseErr != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Scheme != "https" && !a.options.ProviderOptions.DevelopmentMode {
			return Credentials{}, fmt.Errorf("%w: base_url must be an absolute HTTPS URL", ErrInvalidCredentials)
		}
	}
	if !creds.biSnap() && creds.key() == "" {
		return Credentials{}, fmt.Errorf("%w: API key is required", ErrInvalidCredentials)
	}
	if creds.biSnap() && strings.TrimSpace(creds.ClientID) == "" {
		return Credentials{}, fmt.Errorf("%w: BI-SNAP client_id is required", ErrInvalidCredentials)
	}
	return creds, nil
}

func parseCredentialsFromStatic(creds Credentials) (Credentials, error) {
	if creds.key() == "" && !creds.biSnap() {
		return Credentials{}, fmt.Errorf("%w: credentials are required", ErrInvalidCredentials)
	}
	return creds, nil
}

func parseCredentials(secret string, allowRaw bool) (Credentials, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return Credentials{}, fmt.Errorf("%w: empty credential secret", ErrInvalidCredentials)
	}
	var creds Credentials
	if err := json.Unmarshal([]byte(secret), &creds); err != nil {
		if allowRaw && !strings.HasPrefix(secret, "{") {
			creds.APIKey = secret
		} else {
			return Credentials{}, fmt.Errorf("%w: structured credential JSON: %v", ErrInvalidCredentials, err)
		}
	}
	return creds, nil
}

func (a *Adapter) resolveBeneficiary(ctx context.Context, connection automation.ConnectionRef, ref, name string) (Beneficiary, error) {
	if a.options.ScopedBeneficiaryResolver != nil {
		beneficiary, err := a.options.ScopedBeneficiaryResolver(ctx, connection, ref)
		if err != nil {
			return Beneficiary{}, fmt.Errorf("midtrans iris: resolve beneficiary: %w", err)
		}
		if strings.TrimSpace(beneficiary.Name) == "" {
			beneficiary.Name = name
		}
		return validateBeneficiary(beneficiary)
	}
	if a.options.BeneficiaryResolver != nil {
		beneficiary, err := a.options.BeneficiaryResolver(ctx, ref)
		if err != nil {
			return Beneficiary{}, fmt.Errorf("midtrans iris: resolve beneficiary: %w", err)
		}
		if strings.TrimSpace(beneficiary.Name) == "" {
			beneficiary.Name = name
		}
		return validateBeneficiary(beneficiary)
	}
	beneficiary, err := parseBeneficiaryRef(ref, name)
	if err != nil {
		return Beneficiary{}, err
	}
	return validateBeneficiary(beneficiary)
}

func parseBeneficiaryRef(ref, name string) (Beneficiary, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Beneficiary{}, ErrInvalidBeneficiary
	}
	if strings.HasPrefix(ref, "{") {
		var beneficiary Beneficiary
		if err := json.Unmarshal([]byte(ref), &beneficiary); err != nil {
			return Beneficiary{}, fmt.Errorf("%w: JSON: %v", ErrInvalidBeneficiary, err)
		}
		if beneficiary.Name == "" {
			beneficiary.Name = name
		}
		return beneficiary, nil
	}
	if strings.Contains(ref, "=") {
		values, err := url.ParseQuery(ref)
		if err == nil {
			return Beneficiary{Name: firstNonEmpty(values.Get("name"), name), Account: values.Get("account"), Bank: values.Get("bank"), Email: values.Get("email"), AliasName: values.Get("alias_name")}, nil
		}
	}
	for _, separator := range []string{"|", ":", "/"} {
		parts := strings.SplitN(ref, separator, 3)
		if len(parts) >= 2 {
			return Beneficiary{Name: name, Bank: strings.TrimSpace(parts[0]), Account: strings.TrimSpace(parts[1]), Email: strings.TrimSpace(valueAt(parts, 2))}, nil
		}
	}
	// A bare value is retained as an alias. Resolvers may turn aliases into
	// bank details; without a resolver, fail closed instead of guessing a bank.
	return Beneficiary{Name: name, AliasName: ref}, nil
}

func validateBeneficiary(beneficiary Beneficiary) (Beneficiary, error) {
	beneficiary.Name = strings.TrimSpace(beneficiary.Name)
	beneficiary.Account = strings.TrimSpace(beneficiary.Account)
	beneficiary.Bank = strings.TrimSpace(beneficiary.Bank)
	beneficiary.Email = strings.TrimSpace(beneficiary.Email)
	beneficiary.AliasName = strings.TrimSpace(beneficiary.AliasName)
	if beneficiary.Name == "" {
		return Beneficiary{}, fmt.Errorf("%w: name is required", ErrInvalidBeneficiary)
	}
	if beneficiary.Account == "" || beneficiary.Bank == "" {
		return Beneficiary{}, fmt.Errorf("%w: bank and account are required (use bank:account or a resolver)", ErrInvalidBeneficiary)
	}
	return beneficiary, nil
}

func (a *Adapter) settlementFromResult(ref automation.ConnectionRef, instructionRef automation.ExternalReference, result payoutResult) (payments.Settlement, error) {
	status := normalizeProviderStatus(result.Status)
	if status == "" {
		return payments.Settlement{}, providerError("lookup", automation.ErrorPermanent, "MISSING_STATUS", errors.New("provider status is missing"))
	}
	providerID := firstNonEmpty(result.ReferenceNo, instructionRef.ObjectID)
	if providerID == "" {
		return payments.Settlement{}, ErrMissingProviderReference
	}
	return payments.Settlement{
		Reference:         providerReference(ref, providerID),
		Instruction:       instructionRef,
		Status:            mapSettlementStatus(status),
		SettledAmount:     result.Amount,
		SettledAt:         result.OccurredAt,
		ProviderFee:       result.Fee,
		EndToEndReference: result.EndToEndReference,
	}, nil
}

func validateConnectionRef(ref automation.ConnectionRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(ref.Provider)) {
	case Provider, "midtrans-iris", "midtransiris", "iris", "midtrans":
		return nil
	default:
		return fmt.Errorf("%w: provider %q", ErrInvalidCredentials, ref.Provider)
	}
}

func validateIDR(amount automation.ExactAmount) error {
	if err := amount.Validate(); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(amount.Currency), "IDR") {
		return fmt.Errorf("%w: got %q", ErrUnsupportedCurrency, amount.Currency)
	}
	return nil
}

func validateReturnedAmount(operation string, expected, actual automation.ExactAmount) error {
	if actual.Currency == "" && actual.Amount.String() == "0" {
		return nil
	}
	if err := actual.Validate(); err != nil {
		return providerError(operation, automation.ErrorPermanent, "INVALID_AMOUNT", err)
	}
	if !strings.EqualFold(strings.TrimSpace(expected.Currency), strings.TrimSpace(actual.Currency)) || expected.Amount.Cmp(actual.Amount) != 0 {
		return providerError(operation, automation.ErrorPermanent, "AMOUNT_MISMATCH", fmt.Errorf("provider returned %s %s, requested %s %s", actual.Amount.String(), actual.Currency, expected.Amount.String(), expected.Currency))
	}
	return nil
}

func providerReference(ref automation.ConnectionRef, id string) automation.ExternalReference {
	return automation.ExternalReference{Connection: ref, ObjectType: "midtrans_iris_payout", ObjectID: strings.TrimSpace(id)}
}

func referenceKey(ref automation.ExternalReference) string {
	return fmt.Sprintf("%d/%d/%s/%s/%s", ref.Connection.CompanyID, ref.Connection.ConnectionID, ref.Connection.Provider, ref.ObjectType, ref.ObjectID)
}

func (a *Adapter) rememberProviderID(instructionRef automation.ExternalReference, providerID string) {
	if a == nil || strings.TrimSpace(providerID) == "" {
		return
	}
	a.mu.Lock()
	if a.refs == nil {
		a.refs = make(map[string]string)
	}
	if a.providerInstructions == nil {
		a.providerInstructions = make(map[string]automation.ExternalReference)
	}
	a.refs[referenceKey(instructionRef)] = strings.TrimSpace(providerID)
	a.providerInstructions[referenceKey(providerReference(instructionRef.Connection, providerID))] = instructionRef
	a.mu.Unlock()
}

func (a *Adapter) originalInstructionReference(ref automation.ExternalReference) automation.ExternalReference {
	if a == nil {
		return ref
	}
	a.mu.Lock()
	instructionRef, ok := a.providerInstructions[referenceKey(ref)]
	a.mu.Unlock()
	if ok {
		return instructionRef
	}
	return ref
}

func (a *Adapter) providerID(ref automation.ExternalReference) string {
	if strings.TrimSpace(ref.ObjectType) == "midtrans_iris_payout" {
		return strings.TrimSpace(ref.ObjectID)
	}
	if a == nil {
		return strings.TrimSpace(ref.ObjectID)
	}
	a.mu.Lock()
	providerID := a.refs[referenceKey(ref)]
	a.mu.Unlock()
	if strings.TrimSpace(providerID) != "" {
		return strings.TrimSpace(providerID)
	}
	return strings.TrimSpace(ref.ObjectID)
}

func payoutNotes(instruction payments.Instruction) string {
	if value := strings.TrimSpace(instruction.EndToEndReference); value != "" {
		return value
	}
	return instruction.Reference.ObjectID
}

func idempotencyHeaders(value string) http.Header {
	value = trimExternalID(value)
	return http.Header{"Idempotency-Key": []string{value}}
}

func transactionalSignature(method, path, token string, body []byte, timestamp, secret string) string {
	hash := sha256.Sum256(body)
	stringToSign := strings.ToUpper(method) + ":" + path + ":" + token + ":" + hex.EncodeToString(hash[:]) + ":" + timestamp
	mac := hmac.New(sha512.New, []byte(secret))
	_, _ = mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func rsaSignature(key *rsa.PrivateKey, value string) (string, error) {
	digest := sha256.Sum256([]byte(value))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func parsePrivateKey(value string) (*rsa.PrivateKey, error) {
	block, _ := pemDecode([]byte(strings.TrimSpace(value)))
	if block == nil {
		return nil, errors.New("private key is not PEM encoded")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
	}
	if key, err := x509.ParsePKCS1PrivateKey(block); err == nil {
		return key, nil
	}
	return nil, errors.New("private key must be RSA PKCS#8 or PKCS#1")
}

// pemDecode is split out to make the parser's accepted formats explicit.
func pemDecode(data []byte) ([]byte, []byte) {
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" && block.Type != "RSA PRIVATE KEY" {
		return nil, rest
	}
	return block.Bytes, rest
}

func (a *Adapter) now() time.Time {
	if a != nil && a.options.Now != nil {
		return a.options.Now()
	}
	return time.Now().UTC()
}

func (a *Adapter) logProviderResult(ref automation.ConnectionRef, operation, providerID, status string) {
	if a == nil || a.logger == nil {
		return
	}
	a.logger.Debug("midtrans iris provider operation", slog.String("operation", operation), slog.Int64("company_id", ref.CompanyID), slog.Int64("connection_id", ref.ConnectionID), slog.String("provider_reference", redactReference(providerID)), slog.String("status", normalizeProviderStatus(status)))
}

func redactReference(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return "[redacted]"
	}
	return value[:4] + "…" + value[len(value)-4:]
}

func providerError(operation string, category automation.ErrorCategory, code string, err error) error {
	message := "provider request failed"
	if err != nil {
		message = sanitizeMessage(err.Error())
	}
	return &automation.ProviderError{Category: category, Operation: "midtrans_iris." + operation, ProviderCode: strings.TrimSpace(code), Message: message, Err: err}
}

func classifyProviderResponse(operation string, status int, message string, rawErrors json.RawMessage, code string) error {
	if strings.TrimSpace(message) == "" && len(rawErrors) > 0 {
		message = safeResponseMessage(rawErrors)
	}
	category := automation.ErrorPermanent
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		category = automation.ErrorAuthentication
	case status == http.StatusTooManyRequests:
		category = automation.ErrorRateLimited
	case status >= 500:
		category = automation.ErrorTransient
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		category = automation.ErrorInvalidRequest
	case status == http.StatusConflict:
		category = automation.ErrorDuplicate
	}
	return providerError(operation, category, code, errors.New(sanitizeMessage(firstNonEmpty(message, http.StatusText(status), "provider request failed"))))
}

func safeResponseMessage(body []byte) string {
	var payload map[string]any
	if json.Unmarshal(body, &payload) == nil {
		for _, key := range []string{"error_message", "responseMessage", "status_message", "message", "error"} {
			if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
				return sanitizeMessage(value)
			}
		}
	}
	return "provider request failed"
}

func sanitizeMessage(value string) string {
	value = strings.TrimSpace(value)
	for _, token := range []string{"SB-Mid-", "Mid-server-", "IRIS-", "Bearer ", "Basic "} {
		if idx := strings.Index(value, token); idx >= 0 {
			value = value[:idx] + "[redacted]"
		}
	}
	if len(value) > maxProviderMessage {
		value = value[:maxProviderMessage]
	}
	if value == "" {
		return "provider request failed"
	}
	return value
}

func categoryForTransport(sideEffect bool) automation.ErrorCategory {
	if sideEffect {
		return automation.ErrorAmbiguous
	}
	return automation.ErrorTransient
}

func markAmbiguous(err error) error {
	var providerErr *automation.ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil {
		return err
	}
	copy := *providerErr
	copy.Category = automation.ErrorAmbiguous
	return &copy
}

func normalizeProviderStatus(value string) string { return strings.ToUpper(strings.TrimSpace(value)) }

func mapSettlementStatus(status string) string {
	switch normalizeProviderStatus(status) {
	case "REQUESTED", "CREATED", "APPROVED", "PROCESSING", "INITIATED", "PENDING":
		return payments.SettlementStatusPending
	case "COMPLETED", "SUCCESS", "SUCCEEDED", "SETTLED", "PAID":
		return payments.SettlementStatusSettled
	case "PARTIAL", "PARTIALLY_SETTLED":
		return payments.SettlementStatusPartial
	case "CANCELLED", "CANCELED", "REJECTED", "DECLINED", "FAILED", "EXPIRED":
		if strings.Contains(normalizeProviderStatus(status), "CANCEL") {
			return payments.SettlementStatusCancelled
		}
		return payments.SettlementStatusFailed
	default:
		return normalizeProviderStatus(status)
	}
}

func mapCancelStatus(status string) string {
	if isCancelStatus(status) || strings.EqualFold(strings.TrimSpace(status), "REJECTED") {
		return payments.SettlementStatusCancelled
	}
	return status
}

func isCancelStatus(status string) bool {
	switch normalizeProviderStatus(status) {
	case "CANCELLED", "CANCELED", "CANCEL", "REJECTED":
		return true
	default:
		return false
	}
}

func isFailureStatus(status string) bool {
	switch normalizeProviderStatus(status) {
	case "FAILED", "REJECTED", "DECLINED", "ERROR", "INVALID", "EXPIRED":
		return true
	default:
		return false
	}
}

func decodeBISNAPResponse(body []byte) (biSnapResponse, error) {
	var response biSnapResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return biSnapResponse{}, err
	}
	return response, nil
}

func biSnapStatus(response biSnapResponse) string {
	status := firstNonEmpty(response.Status, response.TransactionStatus, response.AdditionalInfo.Status)
	if status != "" {
		return status
	}
	switch strings.TrimSpace(response.LatestTransactionStatus) {
	case "00":
		return "COMPLETED"
	case "01":
		return "INITIATED"
	case "03":
		return "PROCESSING"
	case "04":
		return "REFUNDED"
	case "05":
		return "CANCELLED"
	case "06":
		return "FAILED"
	case "07":
		return "NOT_FOUND"
	case "08":
		return "EXPIRED"
	case "09":
		return "REJECTED"
	default:
		return ""
	}
}

func biSnapSuccess(status int, response biSnapResponse) bool {
	if status < 200 || status >= 300 {
		return false
	}
	code := strings.TrimSpace(response.ResponseCode)
	return code == "" || strings.HasPrefix(code, "200") || strings.EqualFold(response.ResponseMessage, "success") || strings.EqualFold(response.ResponseMessage, "successful")
}

func exactAmount(value, currency string) (automation.ExactAmount, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return automation.ExactAmount{}, nil
	}
	if strings.ContainsAny(value, "/eE") {
		return automation.ExactAmount{}, errors.New("amount must be a decimal string")
	}
	scale := 0
	if dot := strings.IndexByte(value, '.'); dot >= 0 {
		scale = len(value) - dot - 1
	}
	parsed, err := accountingmoney.Parse(value, scale)
	if err != nil {
		return automation.ExactAmount{}, err
	}
	if strings.TrimSpace(currency) == "" {
		currency = "IDR"
	}
	return automation.ExactAmount{Amount: parsed, Currency: strings.ToUpper(strings.TrimSpace(currency))}, nil
}

func parseProviderTime(values ...string) time.Time {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05 -0700", "2006-01-02 15:04:05"} {
			if parsed, err := time.Parse(layout, value); err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}

func parseExpiresIn(raw json.RawMessage) time.Duration {
	if len(raw) == 0 {
		return 0
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		seconds, _ := strconv.Atoi(strings.TrimSpace(value))
		return time.Duration(seconds) * time.Second
	}
	var seconds int64
	if json.Unmarshal(raw, &seconds) == nil {
		return time.Duration(seconds) * time.Second
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmptyRaw(values ...json.RawMessage) json.RawMessage {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func pathOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func normalizePath(value string) string {
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		return "/" + value
	}
	return value
}

func trimExternalID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 46 {
		return value
	}
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])[:46]
}

func valueAt(values []string, index int) string {
	if index >= 0 && index < len(values) {
		return values[index]
	}
	return ""
}

func firstNonEmptyExact(value automation.ExactAmount, fallback automation.ExactAmount) automation.ExactAmount {
	if value.Amount.String() != "0" || value.Currency != "" {
		return value
	}
	return fallback
}

// Keep this reference in the package so static analysis catches accidental
// removal of the exact cryptographic primitive used by BI-SNAP.
var _ = firstNonEmptyExact
var _ = crypto.SHA256
