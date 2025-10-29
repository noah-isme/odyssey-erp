package ic

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

const (
	// SourcePrefix is used to build deterministic source_link keys.
	SourcePrefix = "IC_ARAP"
	// AuditAction identifies audit log entries emitted by the engine.
	AuditAction = "ic_eliminate"
	// AuditEntity describes the audit entity for elimination headers.
	AuditEntity = "elimination_journal_headers"
)

// RepositoryProvider describes the persistence operations required by the engine.
type RepositoryProvider interface {
	ResolvePeriodID(ctx context.Context, code string) (int64, error)
	ListPairExposures(ctx context.Context, groupID, periodID int64, periodCode string) ([]PairExposure, error)
	UpsertElimination(ctx context.Context, params UpsertParams) (UpsertResult, error)
}

// AuditRecorder captures audit events.
type AuditRecorder interface {
	Record(ctx context.Context, log shared.AuditLog) error
}

// Engine orchestrates AR/AP intercompany eliminations.
type Engine struct {
	repo    RepositoryProvider
	audit   AuditRecorder
	logger  *slog.Logger
	actorID int64
	now     func() time.Time
}

// EngineConfig configures optional behaviour for the engine.
type EngineConfig struct {
	ActorID int64
}

// NewEngine wires required dependencies for the elimination engine.
func NewEngine(repo RepositoryProvider, audit AuditRecorder, logger *slog.Logger, cfg EngineConfig) *Engine {
	eng := &Engine{
		repo:   repo,
		audit:  audit,
		logger: logger,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	if cfg.ActorID > 0 {
		eng.actorID = cfg.ActorID
	}
	return eng
}

// Result summarises the engine execution outcome.
type Result struct {
	PeriodID       int64
	PeriodCode     string
	GroupID        int64
	Eliminated     int
	TotalAmount    float64
	HeadersCreated int
	HeadersUpdated int
}

// Run executes the elimination routine for the provided group and period code.
func (e *Engine) Run(ctx context.Context, groupID int64, periodCode string) (Result, error) {
	if e == nil || e.repo == nil {
		return Result{}, fmt.Errorf("ic engine not initialised")
	}
	if groupID <= 0 {
		return Result{}, fmt.Errorf("group id is required")
	}
	if periodCode == "" {
		return Result{}, fmt.Errorf("period code is required")
	}

	periodID, err := e.repo.ResolvePeriodID(ctx, periodCode)
	if err != nil {
		return Result{}, err
	}

	exposures, err := e.repo.ListPairExposures(ctx, groupID, periodID, periodCode)
	if err != nil {
		return Result{}, err
	}
	if len(exposures) == 0 {
		e.log().Info("no intercompany exposures discovered", slog.Int64("group_id", groupID), slog.String("period", periodCode))
		return Result{GroupID: groupID, PeriodID: periodID, PeriodCode: periodCode}, nil
	}

	result := Result{GroupID: groupID, PeriodID: periodID, PeriodCode: periodCode}
	for _, pair := range exposures {
		amount := eliminationAmount(pair.ARAmount, pair.APAmount)
		if amount <= 0 {
			continue
		}
		lines := []UpsertLine{
			{
				GroupAccountID: pair.APGroupAccountID,
				Debit:          round(amount),
				Memo:           fmt.Sprintf("IC elimination AP %s -> %s", pair.CompanyBName, pair.CompanyAName),
			},
			{
				GroupAccountID: pair.ARGroupAccountID,
				Credit:         round(amount),
				Memo:           fmt.Sprintf("IC elimination AR %s -> %s", pair.CompanyAName, pair.CompanyBName),
			},
		}
		source := buildSourceLink(periodCode, pair.CompanyAID, pair.CompanyBID)
		params := UpsertParams{
			GroupID:    groupID,
			PeriodID:   periodID,
			SourceLink: source,
			CreatedBy:  e.actorID,
			Lines:      lines,
		}
		upsert, err := e.repo.UpsertElimination(ctx, params)
		if err != nil {
			return result, err
		}
		result.Eliminated++
		result.TotalAmount += amount
		if upsert.Created {
			result.HeadersCreated++
		} else {
			result.HeadersUpdated++
		}
		e.recordAudit(ctx, upsert.HeaderID, source, pair, amount)
	}

	e.log().Info("completed intercompany eliminations",
		slog.Int64("group_id", groupID),
		slog.String("period", periodCode),
		slog.Int("pairs", result.Eliminated),
		slog.Float64("total_amount", round(result.TotalAmount)))
	return result, nil
}

func (e *Engine) recordAudit(ctx context.Context, headerID int64, source string, pair PairExposure, amount float64) {
	if e == nil || e.audit == nil {
		return
	}
	meta := map[string]any{
		"source_link":    source,
		"group_id":       pair.GroupID,
		"period_id":      pair.PeriodID,
		"period":         pair.PeriodCode,
		"company_a_id":   pair.CompanyAID,
		"company_a_name": pair.CompanyAName,
		"company_b_id":   pair.CompanyBID,
		"company_b_name": pair.CompanyBName,
		"amount":         round(amount),
		"actor":          "system/job",
		"recorded_at":    e.now(),
	}
	_ = e.audit.Record(ctx, shared.AuditLog{
		ActorID:  e.actorID,
		Action:   AuditAction,
		Entity:   AuditEntity,
		EntityID: fmt.Sprintf("%d", headerID),
		Meta:     meta,
		At:       e.now(),
	})
}

func (e *Engine) log() *slog.Logger {
	if e != nil && e.logger != nil {
		return e.logger.With(slog.String("component", "ic_engine"))
	}
	return slog.Default().With(slog.String("component", "ic_engine"))
}

func eliminationAmount(ar, ap float64) float64 {
	ar = math.Abs(ar)
	ap = math.Abs(ap)
	if ar == 0 || ap == 0 {
		return 0
	}
	if ar < ap {
		return ar
	}
	return ap
}

func round(v float64) float64 {
	return math.Round(v*100) / 100
}

func buildSourceLink(period string, companyA, companyB int64) string {
	return fmt.Sprintf("%s|%s|%d|%d", SourcePrefix, period, companyA, companyB)
}
