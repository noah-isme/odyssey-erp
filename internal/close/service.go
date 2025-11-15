package close

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Service orchestrates accounting period lifecycle and close runs.
type Service struct {
	repo *Repository
	now  func() time.Time
}

// NewService constructs a Service instance.
func NewService(repo *Repository) *Service {
	return &Service{
		repo: repo,
		now:  time.Now,
	}
}

// WithNow overrides the clock for deterministic tests.
func (s *Service) WithNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// ListPeriods returns paginated periods for the specified company.
func (s *Service) ListPeriods(ctx context.Context, companyID int64, limit, offset int) ([]Period, error) {
	return s.repo.ListPeriods(ctx, companyID, limit, offset)
}

// CreatePeriod inserts a new period after validating overlap.
func (s *Service) CreatePeriod(ctx context.Context, in CreatePeriodInput) (Period, error) {
	if err := in.Validate(); err != nil {
		return Period{}, err
	}
	conflict, err := s.repo.PeriodRangeConflict(ctx, in.CompanyID, in.StartDate, in.EndDate)
	if err != nil {
		return Period{}, err
	}
	if conflict {
		return Period{}, ErrPeriodOverlap
	}
	var period Period
	err = s.repo.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var e error
		period, e = s.repo.InsertPeriod(ctx, tx, in, PeriodStatusOpen)
		return e
	})
	if err != nil {
		return Period{}, err
	}
	return period, nil
}

// StartCloseRun creates a new close run for a period and seeds default checklist items.
func (s *Service) StartCloseRun(ctx context.Context, in StartCloseRunInput) (CloseRun, error) {
	if in.CompanyID == 0 || in.PeriodID == 0 || in.ActorID == 0 {
		return CloseRun{}, errors.New("close: company, period, and actor are required")
	}
	var run CloseRun
	err := s.repo.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		period, err := s.repo.LoadPeriodForUpdate(ctx, tx, in.PeriodID)
		if err != nil {
			return err
		}
		if period.CompanyID != 0 && period.CompanyID != in.CompanyID {
			return fmt.Errorf("close: period does not belong to company")
		}
		if period.Status == PeriodStatusHardClosed {
			return ErrPeriodHardClosed
		}
		active, err := s.repo.PeriodHasActiveRun(ctx, tx, period.ID)
		if err != nil {
			return err
		}
		if active {
			return ErrActiveRunExists
		}
		run, err = s.repo.InsertCloseRun(ctx, tx, in)
		if err != nil {
			return err
		}
		items, err := s.repo.InsertChecklistItems(ctx, tx, run.ID, defaultChecklist)
		if err != nil {
			return err
		}
		run.Checklist = items
		return nil
	})
	if err != nil {
		return CloseRun{}, err
	}
	return run, nil
}

// GetCloseRun returns a run with checklist details.
func (s *Service) GetCloseRun(ctx context.Context, id int64) (CloseRun, error) {
	return s.repo.LoadCloseRunWithChecklist(ctx, id)
}

// GetPeriod returns a single accounting period by identifier.
func (s *Service) GetPeriod(ctx context.Context, id int64) (Period, error) {
	return s.repo.LoadPeriod(ctx, id)
}

// UpdateChecklist updates a checklist item status, automatically completing the run if applicable.
func (s *Service) UpdateChecklist(ctx context.Context, in ChecklistUpdateInput) (ChecklistItem, error) {
	if in.ItemID == 0 || in.ActorID == 0 {
		return ChecklistItem{}, errors.New("close: checklist item id and actor required")
	}
	if !isAllowedChecklistStatus(in.Status) {
		return ChecklistItem{}, ErrInvalidChecklistStatus
	}
	var item ChecklistItem
	err := s.repo.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		runID, err := s.repo.LockChecklistItemRun(ctx, tx, in.ItemID)
		if err != nil {
			return err
		}
		run, err := s.repo.LoadCloseRunForUpdate(ctx, tx, runID)
		if err != nil {
			return err
		}
		if run.Status == RunStatusCancelled {
			return ErrChecklistLocked
		}
		item, err = s.repo.UpdateChecklistStatus(ctx, tx, in)
		if err != nil {
			return err
		}
		if run.Status != RunStatusCompleted {
			done, err := s.repo.ChecklistCompletionState(ctx, tx, run.ID)
			if err != nil {
				return err
			}
			if done {
				if err := s.repo.UpdateRunStatus(ctx, tx, run.ID, RunStatusCompleted); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return ChecklistItem{}, err
	}
	return item, nil
}

// SoftClose marks the period as soft-closed.
func (s *Service) SoftClose(ctx context.Context, runID, actorID int64) (Period, error) {
	if actorID == 0 {
		return Period{}, errors.New("close: actor required")
	}
	var periodID int64
	err := s.repo.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		run, err := s.repo.LoadCloseRunForUpdate(ctx, tx, runID)
		if err != nil {
			return err
		}
		if run.Status == RunStatusCancelled {
			return ErrChecklistLocked
		}
		periodID = run.PeriodID
		if err := s.repo.UpdatePeriodStatus(ctx, tx, run.PeriodID, PeriodStatusSoftClosed, actorID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return Period{}, err
	}
	return s.repo.LoadPeriod(ctx, periodID)
}

// HardClose locks the period and enforces checklist completion.
func (s *Service) HardClose(ctx context.Context, runID, actorID int64) (Period, error) {
	if actorID == 0 {
		return Period{}, errors.New("close: actor required")
	}
	var periodID int64
	err := s.repo.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		run, err := s.repo.LoadCloseRunForUpdate(ctx, tx, runID)
		if err != nil {
			return err
		}
		if run.Status == RunStatusCancelled {
			return ErrChecklistLocked
		}
		done, err := s.repo.ChecklistCompletionState(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		if !done {
			return ErrChecklistIncomplete
		}
		if err := s.repo.UpdatePeriodStatus(ctx, tx, run.PeriodID, PeriodStatusHardClosed, actorID); err != nil {
			return err
		}
		if err := s.repo.UpdateRunStatus(ctx, tx, run.ID, RunStatusCompleted); err != nil {
			return err
		}
		periodID = run.PeriodID
		return nil
	})
	if err != nil {
		return Period{}, err
	}
	return s.repo.LoadPeriod(ctx, periodID)
}

// EnsurePeriodOpenForPosting validates that the ledger period is not hard closed.
func (s *Service) EnsurePeriodOpenForPosting(ctx context.Context, ledgerPeriodID int64) error {
	period, err := s.repo.LoadPeriodByLedgerID(ctx, ledgerPeriodID)
	if err != nil {
		return err
	}
	if period.Status == PeriodStatusHardClosed {
		return ErrPeriodHardClosed
	}
	return nil
}

func isAllowedChecklistStatus(status ChecklistStatus) bool {
	switch status {
	case ChecklistStatusPending, ChecklistStatusInProgress, ChecklistStatusDone, ChecklistStatusSkipped:
		return true
	default:
		return false
	}
}

var defaultChecklist = []ChecklistDefinition{
	{Code: "BANK_RECON", Label: "Bank reconciliation completed"},
	{Code: "AP_SUBLEDGER", Label: "AP subledger reconciled"},
	{Code: "AR_SUBLEDGER", Label: "AR subledger reconciled"},
}
