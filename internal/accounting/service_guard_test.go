package accounting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	closepkg "github.com/odyssey-erp/odyssey-erp/internal/close"
)

type stubRepo struct {
	period Period
}

func (r stubRepo) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	return fn(ctx, stubTx{period: r.period})
}

type stubTx struct {
	period Period
}

func (tx stubTx) InsertJournalEntry(ctx context.Context, in PostingInput) (JournalEntry, error) {
	return JournalEntry{
		ID:           1,
		PeriodID:     in.PeriodID,
		Date:         in.Date,
		SourceModule: in.SourceModule,
		SourceID:     in.SourceID,
		Memo:         in.Memo,
		PostedBy:     in.PostedBy,
		Status:       JournalStatusPosted,
	}, nil
}

func (tx stubTx) InsertJournalLines(ctx context.Context, entryID int64, lines []PostingLineInput) error {
	return nil
}

func (tx stubTx) LinkSource(ctx context.Context, module string, ref uuid.UUID, entryID int64) error {
	return nil
}

func (tx stubTx) GetPeriodForUpdate(ctx context.Context, periodID int64) (Period, error) {
	return tx.period, nil
}

func (tx stubTx) GetNextOpenPeriodAfter(ctx context.Context, date time.Time) (Period, error) {
	return Period{}, errors.New("not implemented")
}

func (tx stubTx) GetJournalWithLines(ctx context.Context, entryID int64) (JournalEntry, []JournalLine, error) {
	return JournalEntry{}, nil, errors.New("not implemented")
}

func (tx stubTx) UpdateJournalStatus(ctx context.Context, entryID int64, status JournalStatus) error {
	return nil
}

func (tx stubTx) ListAccounts(ctx context.Context) ([]Account, error) {
	return nil, nil
}

type stubGuard struct {
	err error
}

func (g stubGuard) EnsurePeriodOpenForPosting(ctx context.Context, periodID int64) error {
	return g.err
}

func TestPostJournalRejectsHardClosedGuard(t *testing.T) {
	repo := stubRepo{period: Period{
		ID:        1,
		Status:    PeriodStatusOpen,
		StartDate: time.Now().Add(-time.Hour),
		EndDate:   time.Now().Add(time.Hour),
	}}
	service := NewService(repo, nil, stubGuard{err: closepkg.ErrPeriodHardClosed})
	input := PostingInput{
		PeriodID:     1,
		Date:         time.Now(),
		SourceModule: "TEST",
		SourceID:     uuid.New(),
		Memo:         "guard",
		PostedBy:     10,
		Lines: []PostingLineInput{
			{AccountID: 1, Debit: 100},
			{AccountID: 2, Credit: 100},
		},
	}
	if _, err := service.PostJournal(context.Background(), input); !errors.Is(err, ErrPeriodLocked) {
		t.Fatalf("expected ErrPeriodLocked, got %v", err)
	}
}

func TestPostJournalAllowsSoftClosedPeriod(t *testing.T) {
	repo := stubRepo{period: Period{
		ID:        1,
		Status:    PeriodStatusClosed,
		StartDate: time.Now().Add(-time.Hour),
		EndDate:   time.Now().Add(time.Hour),
	}}
	service := NewService(repo, nil, nil)
	input := PostingInput{
		PeriodID:     1,
		Date:         time.Now(),
		SourceModule: "TEST",
		SourceID:     uuid.New(),
		Memo:         "soft",
		PostedBy:     10,
		Lines: []PostingLineInput{
			{AccountID: 1, Debit: 200},
			{AccountID: 2, Credit: 200},
		},
	}
	if _, err := service.PostJournal(context.Background(), input); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}
