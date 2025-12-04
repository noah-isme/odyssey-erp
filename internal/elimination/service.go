package elimination

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting"
)

const sourceModule = "ELIMINATIONS"

// LedgerPoster abstracts journal posting behaviour.
type LedgerPoster interface {
	PostJournal(ctx context.Context, input accounting.PostingInput) (accounting.JournalEntry, error)
}

// Service orchestrates elimination rules, simulations, and postings.
type Service struct {
	repo   *Repository
	ledger LedgerPoster
	now    func() time.Time
}

// NewService constructs an elimination service instance.
func NewService(repo *Repository, ledger LedgerPoster) *Service {
	return &Service{repo: repo, ledger: ledger, now: time.Now}
}

// ListRules returns configured rules.
func (s *Service) ListRules(ctx context.Context, limit int) ([]Rule, error) {
	return s.repo.ListRules(ctx, limit)
}

// CreateRule validates and persists a new rule.
func (s *Service) CreateRule(ctx context.Context, input CreateRuleInput) (Rule, error) {
	if err := input.Validate(); err != nil {
		return Rule{}, err
	}
	return s.repo.InsertRule(ctx, input)
}

// ListRuns fetches recent elimination runs.
func (s *Service) ListRuns(ctx context.Context, limit int) ([]Run, error) {
	return s.repo.ListRuns(ctx, limit)
}

// CreateRun prepares a run for the provided period and rule.
func (s *Service) CreateRun(ctx context.Context, input CreateRunInput) (Run, error) {
	if err := input.Validate(); err != nil {
		return Run{}, err
	}
	if _, err := s.repo.GetRule(ctx, input.RuleID); err != nil {
		return Run{}, err
	}
	if _, err := s.repo.LoadAccountingPeriod(ctx, input.PeriodID); err != nil {
		return Run{}, err
	}
	return s.repo.InsertRun(ctx, input)
}

// GetRun returns a run with rule context.
func (s *Service) GetRun(ctx context.Context, id int64) (Run, error) {
	return s.repo.GetRun(ctx, id)
}

// SimulateRun aggregates balances and stores them on the run summary.
func (s *Service) SimulateRun(ctx context.Context, id int64) (Run, SimulationSummary, error) {
	run, err := s.repo.GetRun(ctx, id)
	if err != nil {
		return Run{}, SimulationSummary{}, err
	}
	summary, err := s.calculateSummary(ctx, run)
	if err != nil {
		return Run{}, SimulationSummary{}, err
	}
	status := RunStatusSimulated
	if summary.Eliminated <= 0 {
		status = RunStatusFailed
	}
	now := s.now()
	if err := s.repo.SaveRunSimulation(ctx, run.ID, summary, status, now); err != nil {
		return Run{}, SimulationSummary{}, err
	}
	run.Status = status
	run.Summary = &summary
	run.SimulatedAt = &now
	return run, summary, nil
}

// PostRun converts the simulation into a journal entry.
func (s *Service) PostRun(ctx context.Context, id int64, actorID int64) (Run, error) {
	run, err := s.repo.GetRun(ctx, id)
	if err != nil {
		return Run{}, err
	}
	if run.Status != RunStatusSimulated && run.Status != RunStatusDraft && run.Status != RunStatusFailed {
		return Run{}, ErrInvalidStatus
	}
	summary, err := s.calculateSummary(ctx, run)
	if err != nil {
		return Run{}, err
	}
	if summary.Eliminated <= 0 {
		return Run{}, ErrNoElimination
	}
	period, err := s.repo.LoadAccountingPeriod(ctx, run.PeriodID)
	if err != nil {
		return Run{}, err
	}
	rule := run.Rule
	if rule == nil {
		r, err := s.repo.GetRule(ctx, run.RuleID)
		if err != nil {
			return Run{}, err
		}
		rule = &r
	}
	srcAccountID, err := s.repo.LookupAccountID(ctx, rule.AccountSource)
	if err != nil {
		return Run{}, err
	}
	tgtAccountID, err := s.repo.LookupAccountID(ctx, rule.AccountTarget)
	if err != nil {
		return Run{}, err
	}
	lines := buildLines(summary, srcAccountID, tgtAccountID, rule.SourceCompanyID, rule.TargetCompanyID)
	posting := accounting.PostingInput{
		PeriodID:     period.LedgerID,
		Date:         period.EndDate,
		SourceModule: sourceModule,
		SourceID:     uuid.New(),
		Memo:         FormatMemo(*rule, period),
		PostedBy:     actorID,
		Lines:        lines,
	}
	entry, err := s.ledger.PostJournal(ctx, posting)
	if err != nil {
		return Run{}, err
	}
	now := s.now()
	if err := s.repo.SaveRunSimulation(ctx, run.ID, summary, RunStatusSimulated, now); err != nil {
		return Run{}, err
	}
	if err := s.repo.MarkRunPosted(ctx, run.ID, entry.ID, now); err != nil {
		return Run{}, err
	}
	run.Status = RunStatusPosted
	run.PostedAt = &now
	run.JournalEntry = &entry.ID
	run.Summary = &summary
	return run, nil
}

// RecentPeriods exposes the latest accounting periods for UI.
func (s *Service) RecentPeriods(ctx context.Context, limit int) ([]PeriodView, error) {
	return s.repo.ListRecentPeriods(ctx, limit)
}

func (s *Service) calculateSummary(ctx context.Context, run Run) (SimulationSummary, error) {
	rule := run.Rule
	if rule == nil {
		fetched, err := s.repo.GetRule(ctx, run.RuleID)
		if err != nil {
			return SimulationSummary{}, err
		}
		rule = &fetched
	}
	srcBalance, err := s.repo.SumAccountBalance(ctx, run.PeriodID, rule.SourceCompanyID, rule.AccountSource)
	if err != nil {
		return SimulationSummary{}, err
	}
	tgtBalance, err := s.repo.SumAccountBalance(ctx, run.PeriodID, rule.TargetCompanyID, rule.AccountTarget)
	if err != nil {
		return SimulationSummary{}, err
	}
	return ComputeElimination(srcBalance, tgtBalance), nil
}

func buildLines(summary SimulationSummary, srcAccountID, tgtAccountID int64, srcCompany, tgtCompany int64) []accounting.PostingLineInput {
	amount := summary.Eliminated
	companyA := srcCompany
	companyB := tgtCompany
	lineSrc := accounting.PostingLineInput{AccountID: srcAccountID, CompanyID: &companyA}
	lineTgt := accounting.PostingLineInput{AccountID: tgtAccountID, CompanyID: &companyB}
	if summary.SourceBalance >= 0 {
		lineSrc.Credit = amount
		lineTgt.Debit = amount
	} else {
		lineSrc.Debit = amount
		lineTgt.Credit = amount
	}
	return []accounting.PostingLineInput{lineSrc, lineTgt}
}
