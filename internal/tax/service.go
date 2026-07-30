package tax

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

type Service struct {
	store     Store
	validator SchemaValidator
}

func NewService(store Store, validator SchemaValidator) *Service {
	return &Service{store: store, validator: validator}
}

func (s *Service) RecordARInvoice(ctx context.Context, id, actorID int64) error {
	if id <= 0 || actorID <= 0 {
		return ErrInvalidInput
	}
	_, err := s.store.CaptureARInvoice(ctx, id, actorID)
	return err
}
func (s *Service) RecordARCreditNote(ctx context.Context, id, actorID int64) error {
	if id <= 0 || actorID <= 0 {
		return ErrInvalidInput
	}
	_, err := s.store.CaptureARCreditNote(ctx, id, actorID)
	return err
}
func (s *Service) RecordAPInvoice(ctx context.Context, id, actorID int64) error {
	if id <= 0 || actorID <= 0 {
		return ErrInvalidInput
	}
	_, err := s.store.CaptureAPInvoice(ctx, id, actorID)
	return err
}
func (s *Service) RecordAPDebitNote(ctx context.Context, id, actorID int64) error {
	if id <= 0 || actorID <= 0 {
		return ErrInvalidInput
	}
	_, err := s.store.CaptureAPDebitNote(ctx, id, actorID)
	return err
}
func (s *Service) RecordAPPayment(ctx context.Context, id, actorID int64) error {
	if id <= 0 || actorID <= 0 {
		return ErrInvalidInput
	}
	_, err := s.store.CaptureAPPayment(ctx, id, actorID)
	return err
}
func (s *Service) Cancel(ctx context.Context, documentID, actorID int64, reason string) error {
	if documentID <= 0 || actorID <= 0 || reason == "" {
		return ErrInvalidInput
	}
	return s.store.CancelDocument(ctx, documentID, actorID, reason)
}
func (s *Service) CancelARInvoice(ctx context.Context, invoiceID, actorID int64, reason string) error {
	if invoiceID <= 0 || actorID <= 0 || reason == "" {
		return ErrInvalidInput
	}
	return s.store.CancelSource(ctx, "AR_INVOICE", invoiceID, actorID, reason)
}
func (s *Service) CancelAPInvoice(ctx context.Context, invoiceID, actorID int64, reason string) error {
	if invoiceID <= 0 || actorID <= 0 || reason == "" {
		return ErrInvalidInput
	}
	return s.store.CancelSource(ctx, "AP_INVOICE", invoiceID, actorID, reason)
}
func (s *Service) Replace(ctx context.Context, originalID, replacementID, actorID int64, reason string) error {
	if originalID <= 0 || replacementID <= 0 || originalID == replacementID || actorID <= 0 || reason == "" {
		return ErrInvalidInput
	}
	return s.store.ReplaceDocument(ctx, originalID, replacementID, actorID, reason)
}
func (s *Service) Documents(ctx context.Context, companyID, periodID int64) ([]Document, error) {
	if companyID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListDocuments(ctx, companyID, periodID)
}
func (s *Service) Periods(ctx context.Context, companyID int64) ([]Period, error) {
	if companyID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListPeriods(ctx, companyID)
}
func (s *Service) BuildPeriod(ctx context.Context, companyID, periodID, actorID int64) error {
	if companyID <= 0 || periodID <= 0 || actorID <= 0 {
		return ErrInvalidInput
	}
	sources, err := s.store.ListPostedSources(ctx, companyID, periodID)
	if err != nil {
		return err
	}
	var failures []error
	for _, source := range sources {
		switch source.Type {
		case "AR_INVOICE":
			err = s.RecordARInvoice(ctx, source.ID, actorID)
		case "AR_CREDIT_NOTE":
			err = s.RecordARCreditNote(ctx, source.ID, actorID)
		case "AP_INVOICE":
			err = s.RecordAPInvoice(ctx, source.ID, actorID)
		case "AP_DEBIT_NOTE":
			err = s.RecordAPDebitNote(ctx, source.ID, actorID)
		case "AP_PAYMENT":
			err = s.RecordAPPayment(ctx, source.ID, actorID)
		default:
			err = ErrInvalidInput
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s *Service) ProcessPending(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	items, err := s.store.PendingCaptures(ctx, limit)
	if err != nil {
		return err
	}
	var failures []error
	for _, item := range items {
		switch item.SourceType {
		case "AR_INVOICE":
			err = s.RecordARInvoice(ctx, item.SourceID, item.ActorID)
		case "AR_CREDIT_NOTE":
			err = s.RecordARCreditNote(ctx, item.SourceID, item.ActorID)
		case "AP_INVOICE":
			err = s.RecordAPInvoice(ctx, item.SourceID, item.ActorID)
		case "AP_DEBIT_NOTE":
			err = s.RecordAPDebitNote(ctx, item.SourceID, item.ActorID)
		case "AP_PAYMENT":
			err = s.RecordAPPayment(ctx, item.SourceID, item.ActorID)
		default:
			err = ErrInvalidInput
		}
		if err == nil {
			err = s.store.CompleteCapture(ctx, item.ID)
		} else {
			_ = s.store.FailCapture(ctx, item.ID, err)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
func (s *Service) Recap(ctx context.Context, companyID, periodID int64) ([]RecapLine, error) {
	if companyID <= 0 || periodID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.Recap(ctx, companyID, periodID)
}
func (s *Service) Lock(ctx context.Context, companyID, periodID, actorID int64) error {
	if companyID <= 0 || periodID <= 0 || actorID <= 0 {
		return ErrInvalidInput
	}
	lines, err := s.store.Recap(ctx, companyID, periodID)
	if err != nil {
		return err
	}
	for _, line := range lines {
		if line.Difference != 0 {
			return ErrReconciliation
		}
	}
	return s.store.LockPeriod(ctx, companyID, periodID, actorID)
}

func (s *Service) Export(ctx context.Context, companyID, periodID, actorID int64, kind string) (ExportResult, error) {
	if companyID <= 0 || periodID <= 0 || actorID <= 0 || kind == "" {
		return ExportResult{}, ErrInvalidInput
	}
	schema, records, err := s.store.LoadExport(ctx, companyID, periodID, kind)
	if err != nil {
		return ExportResult{}, err
	}
	payload, base, amount, err := renderCoretaxXML(schema, records)
	if err != nil {
		return ExportResult{}, err
	}
	if s.validator == nil {
		return ExportResult{}, ErrConfiguration
	}
	if err = s.validator.Validate(schema, payload); err != nil {
		return ExportResult{}, err
	}
	digest := sha256.Sum256(payload)
	hash := hex.EncodeToString(digest[:])
	id, err := s.store.RecordExport(ctx, companyID, periodID, schema.ID, hash, len(records), base, amount, actorID)
	if err != nil {
		return ExportResult{}, err
	}
	return ExportResult{ID: id, Count: int64(len(records)), Content: string(payload), MediaType: schema.MediaType, Hash: hash, SchemaVersion: schema.Version, TaxableBase: base, TaxAmount: amount}, nil
}

func IsConfigurationError(err error) bool { return errors.Is(err, ErrConfiguration) }
