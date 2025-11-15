package boardpack

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Service orchestrates board pack creation and status transitions.
type Service struct {
	repo *Repository
	now  func() time.Time
}

// NewService constructs a Service instance.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// WithNow overrides the clock for deterministic tests.
func (s *Service) WithNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// Create inserts a new board pack request after validating inputs.
func (s *Service) Create(ctx context.Context, req CreateRequest) (BoardPack, error) {
	if err := req.Validate(); err != nil {
		return BoardPack{}, err
	}
	company, err := s.repo.GetCompany(ctx, req.CompanyID)
	if err != nil {
		return BoardPack{}, err
	}
	period, err := s.repo.GetPeriod(ctx, req.PeriodID)
	if err != nil {
		return BoardPack{}, err
	}
	if period.CompanyID != 0 && period.CompanyID != company.ID {
		return BoardPack{}, fmt.Errorf("boardpack: periode tidak dimiliki company")
	}
	tpl, err := s.repo.GetTemplate(ctx, req.TemplateID)
	if err != nil {
		return BoardPack{}, err
	}
	if !tpl.IsActive {
		return BoardPack{}, fmt.Errorf("boardpack: template nonaktif")
	}
	var varianceMeta map[string]any
	if req.VarianceSnapshotID != nil {
		snap, err := s.repo.GetVarianceSnapshot(ctx, *req.VarianceSnapshotID)
		if err != nil {
			return BoardPack{}, err
		}
		if snap.CompanyID != company.ID {
			return BoardPack{}, fmt.Errorf("boardpack: variance snapshot bukan milik company")
		}
		if strings.ToUpper(strings.TrimSpace(snap.Status)) != "READY" {
			return BoardPack{}, fmt.Errorf("boardpack: variance snapshot belum siap")
		}
		varianceMeta = map[string]any{
			"variance_rule":        snap.RuleName,
			"variance_snapshot_id": snap.ID,
		}
	}
	baseMeta := map[string]any{
		"company_name": company.Name,
		"company_code": company.Code,
		"period_name":  period.Name,
		"template":     tpl.Name,
	}
	req.Metadata = mergeMetadata(baseMeta, varianceMeta, req.Metadata)
	pack, err := s.repo.InsertBoardPack(ctx, req)
	if err != nil {
		return BoardPack{}, err
	}
	return pack, nil
}

// List returns board packs filtered by company/period/status.
func (s *Service) List(ctx context.Context, filter ListFilter) ([]BoardPack, error) {
	return s.repo.ListBoardPacks(ctx, filter)
}

// Get loads a single board pack.
func (s *Service) Get(ctx context.Context, id int64) (BoardPack, error) {
	return s.repo.GetBoardPack(ctx, id)
}

// ListTemplates enumerates available templates for UI consumption.
func (s *Service) ListTemplates(ctx context.Context) ([]Template, error) {
	return s.repo.ListTemplates(ctx, false)
}

// ListCompanies enumerates companies for the dropdown.
func (s *Service) ListCompanies(ctx context.Context) ([]Company, error) {
	return s.repo.ListCompanies(ctx)
}

// ListPeriods returns recent accounting periods.
func (s *Service) ListPeriods(ctx context.Context, companyID int64, limit int) ([]Period, error) {
	return s.repo.ListRecentPeriods(ctx, companyID, limit)
}

// ListVarianceSnapshots enumerates latest ready variance snapshots for the company.
func (s *Service) ListVarianceSnapshots(ctx context.Context, companyID int64, limit int) ([]VarianceSnapshot, error) {
	return s.repo.ListVarianceSnapshots(ctx, companyID, limit)
}

// MarkInProgress transitions a pack to in-progress.
func (s *Service) MarkInProgress(ctx context.Context, id int64) error {
	return s.repo.MarkInProgress(ctx, id)
}

// MarkReady persists the generated artefact and refreshed metadata.
func (s *Service) MarkReady(ctx context.Context, pack BoardPack, filePath string, fileSize int64, pageCount *int, extraMeta map[string]any) (BoardPack, error) {
	merged := mergeMetadata(pack.Metadata, extraMeta)
	if err := s.repo.MarkReady(ctx, pack.ID, filePath, fileSize, pageCount, s.now(), merged); err != nil {
		return BoardPack{}, err
	}
	return s.repo.GetBoardPack(ctx, pack.ID)
}

// MarkFailed updates the record when generation fails.
func (s *Service) MarkFailed(ctx context.Context, id int64, errMessage string) error {
	errMessage = strings.TrimSpace(errMessage)
	if errMessage == "" {
		errMessage = "unknown error"
	}
	return s.repo.MarkFailed(ctx, id, errMessage)
}
