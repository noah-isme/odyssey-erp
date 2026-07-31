package fx

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// OutstandingBalance is the minimal cross-module view needed for revaluation.
// AR/AP services do not own or perform this workflow.
type OutstandingBalance struct {
	DocumentType       DocumentType
	DocumentID         int64
	Currency           string
	BaseCurrency       string
	OriginalBalance    Decimal
	PreviousBaseAmount Decimal
}

type RevaluationRepository interface {
	ListOutstandingBalances(context.Context, time.Time) ([]OutstandingBalance, error)
	PeriodLocked(context.Context, int64) (bool, error)
	InsertRevaluation(context.Context, RevaluationRecord) error
	GetRevaluation(context.Context, int64, DocumentType, int64) (RevaluationRecord, error)
	InsertReversal(context.Context, RevaluationReversal) error
	MarkReversed(context.Context, int64, int64) error
}

type RevaluationTxRepository interface {
	ClaimRevaluation(context.Context, int64, DocumentType, int64) (bool, error)
	InsertRevaluation(context.Context, RevaluationRecord) error
	MarkRevaluationJournal(context.Context, int64, DocumentType, int64, int64) error
	ClaimReversal(context.Context, int64) (bool, error)
	InsertReversal(context.Context, RevaluationReversal) error
	MarkReversed(context.Context, int64, int64) error
}
type TransactionalRevaluationRepository interface {
	WithTx(context.Context, func(context.Context, RevaluationTxRepository) error) error
}

type RevaluationLedger interface {
	CreateRevaluationJournal(context.Context, RevaluationJournal) (int64, error)
}
type TransactionalRevaluationLedger interface {
	CreateRevaluationJournalTx(context.Context, pgx.Tx, RevaluationJournal) (int64, error)
}

type FXRateResolver interface {
	Resolve(context.Context, string, string, time.Time) (FXQuote, error)
}

type RevaluationRecord struct {
	ID, PeriodID, DocumentID, JournalEntryID, ActorID                               int64
	DocumentType                                                                    DocumentType
	Currency, BaseCurrency                                                          string
	OriginalBalance, PreviousBaseAmount, ClosingBaseAmount, Difference, ClosingRate Decimal
	RateDate                                                                        time.Time
	RateSource                                                                      string
}
type RevaluationReversal struct{ RevaluationID, NextPeriodID, JournalEntryID, ActorID int64 }

type RevaluationJournal struct {
	PeriodID, DocumentID, ActorID int64
	DocumentType                  DocumentType
	Amount                        Decimal
	Reversal                      bool
	Date                          time.Time
}

type RevaluationService struct {
	Repo     RevaluationRepository
	Resolver FXRateResolver
	Ledger   RevaluationLedger
}

// PaymentFXSourceKey is shared by AR/AP payment journal writers so retries
// address the same unique database key.
func PaymentFXSourceKey(module string, paymentID, allocationID int64) string {
	return module + "_PAYMENT_FX:" + strconv.FormatInt(paymentID, 10) + ":" + strconv.FormatInt(allocationID, 10)
}

func (s *RevaluationService) Revalue(ctx context.Context, periodID int64, asOf time.Time, actorID int64) error {
	if s == nil || s.Repo == nil || s.Resolver == nil || s.Ledger == nil {
		return fmt.Errorf("fx revaluation: repository, resolver, and ledger are required")
	}
	locked, err := s.Repo.PeriodLocked(ctx, periodID)
	if err != nil {
		return err
	}
	if locked {
		return fmt.Errorf("fx revaluation: period %d is locked", periodID)
	}
	txRepo, txOK := s.Repo.(TransactionalRevaluationRepository)
	ledger, ledgerOK := s.Ledger.(TransactionalRevaluationLedger)
	if !txOK || !ledgerOK {
		return fmt.Errorf("fx revaluation: transactional repository and ledger are required")
	}
	balances, err := s.Repo.ListOutstandingBalances(ctx, asOf)
	if err != nil {
		return err
	}
	for _, balance := range balances {
		quote, err := s.Resolver.Resolve(ctx, balance.BaseCurrency, balance.Currency, asOf)
		if err != nil {
			return err
		}
		result, err := CalculateRevaluation(RevaluationInput{DocumentType: balance.DocumentType, OriginalBalance: balance.OriginalBalance, PreviousBaseAmount: balance.PreviousBaseAmount, ClosingRate: quote.Rate})
		if err != nil {
			return err
		}
		if result.Difference.IsZero() {
			continue
		}
		record := RevaluationRecord{PeriodID: periodID, DocumentID: balance.DocumentID, ActorID: actorID, DocumentType: balance.DocumentType, Currency: balance.Currency, BaseCurrency: balance.BaseCurrency, OriginalBalance: balance.OriginalBalance, PreviousBaseAmount: balance.PreviousBaseAmount, ClosingBaseAmount: result.ClosingBaseAmount, Difference: result.Difference, ClosingRate: quote.Rate, RateDate: quote.RateDate, RateSource: quote.Source}
		err = txRepo.WithTx(ctx, func(txctx context.Context, tx RevaluationTxRepository) error {
			claimed, err := tx.ClaimRevaluation(txctx, periodID, balance.DocumentType, balance.DocumentID)
			if err != nil || !claimed {
				return err
			}
			handle, ok := tx.(interface{ PGXTx() pgx.Tx })
			if !ok {
				return fmt.Errorf("fx revaluation: transaction handle unavailable")
			}
			journalID, err := ledger.CreateRevaluationJournalTx(txctx, handle.PGXTx(), RevaluationJournal{PeriodID: periodID, DocumentID: balance.DocumentID, DocumentType: balance.DocumentType, Amount: result.Difference, ActorID: actorID, Date: quote.RateDate})
			if err != nil {
				return err
			}
			record.JournalEntryID = journalID
			if err := tx.InsertRevaluation(txctx, record); err != nil {
				return err
			}
			return tx.MarkRevaluationJournal(txctx, periodID, balance.DocumentType, balance.DocumentID, journalID)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Reverse creates the next-period reversal through the same ledger port.
func (s *RevaluationService) Reverse(ctx context.Context, periodID, nextPeriodID, documentID, actorID int64, documentType DocumentType, amount Decimal) (int64, error) {
	if s == nil || s.Ledger == nil {
		return 0, fmt.Errorf("fx revaluation: ledger is required")
	}
	original, err := s.Repo.GetRevaluation(ctx, periodID, documentType, documentID)
	if err != nil {
		return 0, err
	}
	txRepo, ok := s.Repo.(TransactionalRevaluationRepository)
	if !ok {
		return 0, fmt.Errorf("fx revaluation: transactional repository is required")
	}
	ledger, ok := s.Ledger.(TransactionalRevaluationLedger)
	if !ok {
		return 0, fmt.Errorf("fx revaluation: transactional ledger is required")
	}
	var journalID int64
	err = txRepo.WithTx(ctx, func(txctx context.Context, tx RevaluationTxRepository) error {
		claimed, err := tx.ClaimReversal(txctx, original.ID)
		if err != nil || !claimed {
			return err
		}
		handle, ok := tx.(interface{ PGXTx() pgx.Tx })
		if !ok {
			return fmt.Errorf("fx revaluation: transaction handle unavailable")
		}
		journalID, err = ledger.CreateRevaluationJournalTx(txctx, handle.PGXTx(), RevaluationJournal{PeriodID: nextPeriodID, DocumentID: documentID, ActorID: actorID, DocumentType: documentType, Amount: amount, Reversal: true})
		if err != nil {
			return err
		}
		if err := tx.InsertReversal(txctx, RevaluationReversal{RevaluationID: original.ID, NextPeriodID: nextPeriodID, JournalEntryID: journalID, ActorID: actorID}); err != nil {
			return err
		}
		return tx.MarkReversed(txctx, original.ID, journalID)
	})
	return journalID, err
}
