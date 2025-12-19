package variance

import (
	"context"
	"fmt"
	"time"
)

// Service coordinates variance rules and snapshot processing.
type Service struct {
	repo *Repository
	now  func() time.Time
}

// NewService builds the service.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// CreateRule validates and stores a rule.
func (s *Service) CreateRule(ctx context.Context, input CreateRuleInput) (Rule, error) {
	if err := input.Validate(); err != nil {
		return Rule{}, err
	}
	if _, err := s.repo.LoadAccountingPeriod(ctx, input.BasePeriodID); err != nil {
		return Rule{}, err
	}
	if input.ComparePeriodID != nil {
		if _, err := s.repo.LoadAccountingPeriod(ctx, *input.ComparePeriodID); err != nil {
			return Rule{}, err
		}
	}
	return s.repo.InsertRule(ctx, input)
}

// ListRules enumerates rules by company.
func (s *Service) ListRules(ctx context.Context, companyID int64) ([]Rule, error) {
	return s.repo.ListRules(ctx, companyID)
}

// TriggerSnapshot inserts a pending snapshot.
func (s *Service) TriggerSnapshot(ctx context.Context, req SnapshotRequest) (Snapshot, error) {
	if err := req.Validate(); err != nil {
		return Snapshot{}, err
	}
	if _, err := s.repo.GetRule(ctx, req.RuleID); err != nil {
		return Snapshot{}, err
	}
	if _, err := s.repo.LoadAccountingPeriod(ctx, req.PeriodID); err != nil {
		return Snapshot{}, err
	}
	return s.repo.InsertSnapshot(ctx, req)
}

// ListSnapshots fetches latest snapshots.
func (s *Service) ListSnapshots(ctx context.Context, filters ListFilters) ([]Snapshot, int, error) {
	return s.repo.ListSnapshots(ctx, filters)
}

// GetSnapshot returns metadata by id.
func (s *Service) GetSnapshot(ctx context.Context, id int64) (Snapshot, error) {
	return s.repo.GetSnapshot(ctx, id)
}

// LoadSnapshotPayload returns previously generated variance rows.
func (s *Service) LoadSnapshotPayload(ctx context.Context, id int64) ([]VarianceRow, error) {
	return s.repo.LoadPayload(ctx, id)
}

// ProcessSnapshot performs aggregation and persists payload.
func (s *Service) ProcessSnapshot(ctx context.Context, snapshotID int64) error {
	snap, err := s.repo.GetSnapshot(ctx, snapshotID)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateStatus(ctx, snap.ID, SnapshotInProgress); err != nil {
		return err
	}
	rule := snap.Rule
	if rule == nil {
		fetched, err := s.repo.GetRule(ctx, snap.RuleID)
		if err != nil {
			_ = s.repo.UpdateStatus(ctx, snap.ID, SnapshotFailed)
			return err
		}
		rule = &fetched
	}
	base, err := s.repo.AggregateBalances(ctx, rule.BasePeriodID, rule.CompanyID)
	if err != nil {
		_ = s.repo.SavePayload(ctx, snap.ID, nil, err.Error())
		_ = s.repo.UpdateStatus(ctx, snap.ID, SnapshotFailed)
		return err
	}
	comparePeriod := rule.BasePeriodID
	if rule.ComparePeriodID != nil {
		comparePeriod = *rule.ComparePeriodID
	}
	compare, err := s.repo.AggregateBalances(ctx, comparePeriod, rule.CompanyID)
	if err != nil {
		_ = s.repo.SavePayload(ctx, snap.ID, nil, err.Error())
		_ = s.repo.UpdateStatus(ctx, snap.ID, SnapshotFailed)
		return err
	}
	rows := ComputeVariance(base, compare, rule.ThresholdAmount, rule.ThresholdPercent)
	if err := s.repo.SavePayload(ctx, snap.ID, rows, ""); err != nil {
		_ = s.repo.UpdateStatus(ctx, snap.ID, SnapshotFailed)
		return err
	}
	if err := s.repo.UpdateStatus(ctx, snap.ID, SnapshotReady); err != nil {
		return err
	}
	return nil
}

// ExportRows formats rows into CSV-ready strings.
func ExportRows(rows []VarianceRow) [][]string {
	out := make([][]string, 0, len(rows)+1)
	header := []string{"Account", "Name", "Base", "Compare", "Variance", "Variance %", "Flagged"}
	out = append(out, header)
	for _, row := range rows {
		out = append(out, []string{
			row.AccountCode,
			row.AccountName,
			fmt.Sprintf("%.2f", row.BaseAmount),
			fmt.Sprintf("%.2f", row.CompareAmount),
			fmt.Sprintf("%.2f", row.Variance),
			fmt.Sprintf("%.2f", row.VariancePct),
			fmt.Sprintf("%t", row.Flagged),
		})
	}
	return out
}
