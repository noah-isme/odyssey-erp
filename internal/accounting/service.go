package accounting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	closepkg "github.com/odyssey-erp/odyssey-erp/internal/close"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// RepositoryPort abstracts transactional repository behaviour.
type RepositoryPort interface {
	WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error
}

// AuditPort records ledger events for compliance.
type AuditPort interface {
	Record(ctx context.Context, log shared.AuditLog) error
}

// PeriodGuard blocks journal postings for hard closed periods.
type PeriodGuard interface {
	EnsurePeriodOpenForPosting(ctx context.Context, periodID int64) error
}

// Service coordinates posting, voiding, and reversing journal entries.
type Service struct {
	repo  RepositoryPort
	audit AuditPort
	guard PeriodGuard
	now   func() time.Time
}

// NewService constructs the ledger service.
func NewService(repo RepositoryPort, audit AuditPort, guard PeriodGuard) *Service {
	return &Service{repo: repo, audit: audit, guard: guard, now: time.Now}
}

// WithNow overrides the clock for testing.
func (s *Service) WithNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// PostJournal validates and persists a new journal entry.
func (s *Service) PostJournal(ctx context.Context, input PostingInput) (JournalEntry, error) {
	if err := input.Validate(); err != nil {
		return JournalEntry{}, err
	}
	var entry JournalEntry
	err := s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if s.guard != nil {
			if err := s.guard.EnsurePeriodOpenForPosting(ctx, input.PeriodID); err != nil {
				if errors.Is(err, closepkg.ErrPeriodHardClosed) {
					return ErrPeriodLocked
				}
				return err
			}
		}
		period, err := tx.GetPeriodForUpdate(ctx, input.PeriodID)
		if err != nil {
			return err
		}
		if period.Status == PeriodStatusLocked {
			return ErrPeriodLocked
		}
		if period.Status != PeriodStatusOpen && period.Status != PeriodStatusClosed {
			return ErrInvalidPeriod
		}
		if input.Date.Before(period.StartDate) || input.Date.After(period.EndDate) {
			return ErrDateOutOfRange
		}
		inserted, err := tx.InsertJournalEntry(ctx, input)
		if err != nil {
			return err
		}
		if err := tx.InsertJournalLines(ctx, inserted.ID, input.Lines); err != nil {
			return err
		}
		if err := tx.LinkSource(ctx, input.SourceModule, input.SourceID, inserted.ID); err != nil {
			if errors.Is(err, ErrSourceConflict) {
				return ErrSourceAlreadyLinked
			}
			return err
		}
		inserted.Lines = toJournalLines(inserted.ID, input.Lines, s.now())
		entry = inserted
		return nil
	})
	if err != nil {
		return JournalEntry{}, err
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, shared.AuditLog{
			ActorID:  input.PostedBy,
			Action:   "journal.post",
			Entity:   "journal_entry",
			EntityID: fmt.Sprintf("%d", entry.ID),
			Meta: map[string]any{
				"number":        entry.Number,
				"source_module": input.SourceModule,
				"source_id":     input.SourceID.String(),
			},
			At: s.now(),
		})
	}
	return entry, nil
}

// VoidJournal marks an existing journal as VOID.
func (s *Service) VoidJournal(ctx context.Context, input VoidInput) (JournalEntry, error) {
	if input.EntryID == 0 {
		return JournalEntry{}, errors.New("accounting: entry id required")
	}
	var entry JournalEntry
	var lines []JournalLine
	err := s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		current, currLines, err := tx.GetJournalWithLines(ctx, input.EntryID)
		if err != nil {
			return err
		}
		period, err := tx.GetPeriodForUpdate(ctx, current.PeriodID)
		if err != nil {
			return err
		}
		if period.Status == PeriodStatusLocked {
			return ErrPeriodLocked
		}
		if period.Status == PeriodStatusClosed {
			return ErrInvalidPeriod
		}
		if current.Status != JournalStatusPosted {
			return ErrInvalidStatus
		}
		if err := tx.UpdateJournalStatus(ctx, current.ID, JournalStatusVoid); err != nil {
			return err
		}
		entry = current
		entry.Status = JournalStatusVoid
		lines = currLines
		return nil
	})
	if err != nil {
		return JournalEntry{}, err
	}
	entry.Lines = lines
	if s.audit != nil {
		_ = s.audit.Record(ctx, shared.AuditLog{
			ActorID:  input.ActorID,
			Action:   "journal.void",
			Entity:   "journal_entry",
			EntityID: fmt.Sprintf("%d", entry.ID),
			Meta: map[string]any{
				"reason": input.Reason,
			},
			At: s.now(),
		})
	}
	return entry, nil
}

// ReverseJournal creates a reversing journal entry.
func (s *Service) ReverseJournal(ctx context.Context, input ReverseInput) (JournalEntry, error) {
	if input.EntryID == 0 {
		return JournalEntry{}, errors.New("accounting: entry id required")
	}
	var reversal JournalEntry
	err := s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		original, lines, err := tx.GetJournalWithLines(ctx, input.EntryID)
		if err != nil {
			return err
		}
		if original.Status != JournalStatusPosted {
			return ErrInvalidStatus
		}
		period, err := tx.GetPeriodForUpdate(ctx, original.PeriodID)
		if err != nil {
			return err
		}
		targetPeriod := period
		targetDate := original.Date
		if input.TargetDate != nil {
			targetDate = *input.TargetDate
		}
		if period.Status != PeriodStatusOpen {
			if period.Status == PeriodStatusLocked && !input.Override {
				return ErrPeriodLocked
			}
			next, err := tx.GetNextOpenPeriodAfter(ctx, period.EndDate.AddDate(0, 0, 1))
			if err != nil {
				return err
			}
			targetPeriod = next
			targetDate = next.StartDate
		}
		if targetDate.Before(targetPeriod.StartDate) || targetDate.After(targetPeriod.EndDate) {
			return ErrDateOutOfRange
		}
		posting := PostingInput{
			PeriodID:     targetPeriod.ID,
			Date:         targetDate,
			SourceModule: original.SourceModule + ":REVERSAL",
			SourceID:     uuid.New(),
			Memo:         defaultReversalMemo(input.Memo, original.Number),
			PostedBy:     input.ActorID,
			Lines:        reverseLines(lines),
		}
		inserted, err := tx.InsertJournalEntry(ctx, posting)
		if err != nil {
			return err
		}
		if err := tx.InsertJournalLines(ctx, inserted.ID, posting.Lines); err != nil {
			return err
		}
		if err := tx.LinkSource(ctx, posting.SourceModule, posting.SourceID, inserted.ID); err != nil {
			return err
		}
		reversal = inserted
		reversal.Lines = toJournalLines(inserted.ID, posting.Lines, s.now())
		return nil
	})
	if err != nil {
		return JournalEntry{}, err
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, shared.AuditLog{
			ActorID:  input.ActorID,
			Action:   "journal.reverse",
			Entity:   "journal_entry",
			EntityID: fmt.Sprintf("%d", input.EntryID),
			Meta: map[string]any{
				"reversal_id":     reversal.ID,
				"reversal_number": reversal.Number,
			},
			At: s.now(),
		})
	}
	return reversal, nil
}

func reverseLines(lines []JournalLine) []PostingLineInput {
	out := make([]PostingLineInput, 0, len(lines))
	for _, line := range lines {
		out = append(out, PostingLineInput{
			AccountID: line.AccountID,
			Debit:     line.Credit,
			Credit:    line.Debit,
			CompanyID: line.DimCompanyID,
			BranchID:  line.DimBranchID,
			Warehouse: line.DimWarehouseID,
		})
	}
	return out
}

func toJournalLines(entryID int64, lines []PostingLineInput, ts time.Time) []JournalLine {
	out := make([]JournalLine, 0, len(lines))
	now := ts
	for _, line := range lines {
		out = append(out, JournalLine{
			JournalID:      entryID,
			AccountID:      line.AccountID,
			Debit:          line.Debit,
			Credit:         line.Credit,
			DimCompanyID:   line.CompanyID,
			DimBranchID:    line.BranchID,
			DimWarehouseID: line.Warehouse,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
	}
	return out
}

func defaultReversalMemo(memo string, number int64) string {
	if memo != "" {
		return memo
	}
	return fmt.Sprintf("Reversal of JE %d", number)
}

// ListAccounts retrieves all chart of accounts entries.
func (s *Service) ListAccounts(ctx context.Context) ([]Account, error) {
	var accounts []Account
	err := s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		var err error
		accounts, err = tx.ListAccounts(ctx)
		return err
	})
	return accounts, err
}

// ListJournalEntries retrieves all journal entries.
func (s *Service) ListJournalEntries(ctx context.Context) ([]JournalEntry, error) {
	var entries []JournalEntry
	err := s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		var err error
		entries, err = tx.ListJournalEntries(ctx)
		return err
	})
	return entries, err
}

// ListGeneralLedger retrieves accounts with their balances for general ledger.
func (s *Service) ListGeneralLedger(ctx context.Context) ([]Account, error) {
	// For MVP, just return all accounts; full GL would include balances
	return s.ListAccounts(ctx)
}
