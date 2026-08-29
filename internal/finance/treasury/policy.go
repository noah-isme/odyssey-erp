package treasury

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Policy validation errors are deliberately typed so callers can distinguish
// a policy rejection from a repository or provider failure without exposing
// policy details in an HTTP response.
var (
	ErrInvalidPaymentPolicy          = errors.New("treasury: invalid payment policy")
	ErrPaymentBatchLimit             = errors.New("treasury: payment batch exceeds configured limit")
	ErrPaymentItemLimit              = errors.New("treasury: payment item exceeds configured limit")
	ErrPaymentBatchTotal             = errors.New("treasury: payment batch total does not match active items")
	ErrPaymentInvoiceAllocationLimit = errors.New("treasury: payment allocation exceeds invoice balance")
	ErrPaymentApprovalRequired       = errors.New("treasury: payment batch is missing independent approval")
	ErrBankFormatMismatch            = errors.New("treasury: payment export does not match configured bank format")
	ErrPaymentEvidenceRequired       = errors.New("treasury: supplier bank evidence reference is required")
	ErrPaymentBatchRevisionConflict  = errors.New("treasury: payment batch changed during export")
)

const paymentDay = 24 * time.Hour

// Validate checks values read from treasury_payment_policies before they are
// used as controls. A NULL amount/format/cut-off is represented by its zero
// value and means that the corresponding optional control is not configured.
func (p PaymentPolicy) Validate() error {
	if value := strings.TrimSpace(p.MaxBatchAmount.String()); value != "0" || strings.TrimSpace(string(p.MaxBatchAmount)) != "" {
		if err := p.MaxBatchAmount.Validate(); err != nil || !p.MaxBatchAmount.IsPositive() {
			return fmt.Errorf("%w: max batch amount", ErrInvalidPaymentPolicy)
		}
	}
	if value := strings.TrimSpace(p.MaxItemAmount.String()); value != "0" || strings.TrimSpace(string(p.MaxItemAmount)) != "" {
		if err := p.MaxItemAmount.Validate(); err != nil || !p.MaxItemAmount.IsPositive() {
			return fmt.Errorf("%w: max item amount", ErrInvalidPaymentPolicy)
		}
	}
	if p.CalendarID != nil && *p.CalendarID <= 0 {
		return fmt.Errorf("%w: calendar id", ErrInvalidPaymentPolicy)
	}
	if p.CutOffTime != nil && (*p.CutOffTime < 0 || *p.CutOffTime >= paymentDay) {
		return fmt.Errorf("%w: cut-off time", ErrInvalidPaymentPolicy)
	}
	if format := strings.TrimSpace(p.BankFormat); len(format) > 100 {
		return fmt.Errorf("%w: bank format is too long", ErrInvalidPaymentPolicy)
	}
	return nil
}

// ValidateBatch enforces the controls that are meaningful at approval/export
// time. The caller must provide the authoritative batch total returned by its
// repository; the active item sum is checked as an additional integrity
// boundary before an executable artifact is created.
func (p PaymentPolicy) ValidateBatch(batch PaymentBatch, items []PaymentBatchItem) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if batch.ID <= 0 || batch.CompanyID <= 0 || len(items) == 0 {
		return fmt.Errorf("%w: invalid batch", ErrInvalidPaymentPolicy)
	}
	if err := batch.TotalAmount.Validate(); err != nil {
		return fmt.Errorf("%w: batch total: %v", ErrInvalidPaymentPolicy, err)
	}
	total := MustParseAmount("0")
	for _, item := range items {
		if item.ID <= 0 || item.BatchID != batch.ID || !item.Amount.IsPositive() {
			return fmt.Errorf("%w: invalid item %d", ErrInvalidPaymentPolicy, item.ID)
		}
		if err := item.Amount.Validate(); err != nil {
			return fmt.Errorf("%w: item %d amount: %v", ErrInvalidPaymentPolicy, item.ID, err)
		}
		if strings.TrimSpace(string(p.MaxItemAmount)) != "" {
			if exceeds, err := item.Amount.Cmp(p.MaxItemAmount); err != nil {
				return fmt.Errorf("%w: item %d amount: %v", ErrInvalidPaymentPolicy, item.ID, err)
			} else if exceeds > 0 {
				return fmt.Errorf("%w: item %d", ErrPaymentItemLimit, item.ID)
			}
		}
		var err error
		total, err = total.Add(item.Amount)
		if err != nil {
			return fmt.Errorf("%w: item %d total: %v", ErrInvalidPaymentPolicy, item.ID, err)
		}
	}
	if equal, err := total.Cmp(batch.TotalAmount); err != nil {
		return fmt.Errorf("%w: batch total: %v", ErrInvalidPaymentPolicy, err)
	} else if equal != 0 {
		return ErrPaymentBatchTotal
	}
	if strings.TrimSpace(string(p.MaxBatchAmount)) != "" {
		if exceeds, err := batch.TotalAmount.Cmp(p.MaxBatchAmount); err != nil {
			return fmt.Errorf("%w: batch amount: %v", ErrInvalidPaymentPolicy, err)
		} else if exceeds > 0 {
			return ErrPaymentBatchLimit
		}
	}
	return nil
}

// BankFormat reports the normalized identifier used by the export encoder.
// Punctuation is normalized so configuration values such as ISO-20022 and
// iso_20022 cannot accidentally describe different formats.
func normalizeBankFormat(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	return strings.ReplaceAll(value, " ", "_")
}

// BankFormatEncoderIdentity is optional for compatibility with existing
// custom encoders. Once a policy specifies a format, an encoder must identify
// itself so an unrelated payload cannot be exported under that policy.
type BankFormatEncoderIdentity interface {
	BankFormat() string
}

func validateBankFormat(policy PaymentPolicy, encoder BankFormatEncoder) error {
	wanted := normalizeBankFormat(policy.BankFormat)
	if wanted == "" {
		return nil
	}
	identity, ok := encoder.(BankFormatEncoderIdentity)
	if !ok || normalizeBankFormat(identity.BankFormat()) == "" {
		return fmt.Errorf("%w: encoder identity is missing", ErrBankFormatMismatch)
	}
	if normalizeBankFormat(identity.BankFormat()) != wanted {
		return fmt.Errorf("%w: configured=%q encoder=%q", ErrBankFormatMismatch, wanted, normalizeBankFormat(identity.BankFormat()))
	}
	return nil
}
