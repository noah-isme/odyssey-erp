package boardpack

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/reports"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics"
	"github.com/odyssey-erp/odyssey-erp/internal/variance"
)

// VarianceProvider exposes the variance payload loader used by the builder.
type VarianceProvider interface {
	LoadSnapshotPayload(ctx context.Context, id int64) ([]variance.VarianceRow, error)
}

// KPIProvider exposes KPI summary queries used in the exec summary.
type KPIProvider interface {
	GetKPISummary(ctx context.Context, filter analytics.KPIFilter) (analytics.KPISummary, error)
}

type dataRepository interface {
	AggregateAccountBalances(ctx context.Context, companyID, periodID int64) ([]reports.AccountBalance, error)
	GetTemplate(ctx context.Context, id int64) (Template, error)
}

// Builder assembles the board pack view model prior to rendering.
type Builder struct {
	repo     dataRepository
	variance VarianceProvider
	kpi      KPIProvider
	now      func() time.Time
	topLimit int
}

// NewBuilder constructs a Builder instance.
func NewBuilder(repo dataRepository, variance VarianceProvider, kpi KPIProvider) *Builder {
	return &Builder{repo: repo, variance: variance, kpi: kpi, now: time.Now, topLimit: 10}
}

// WithTopVarianceLimit overrides the maximum number of rows rendered in the variance section.
func (b *Builder) WithTopVarianceLimit(limit int) {
	if limit > 0 {
		b.topLimit = limit
	}
}

// Build constructs the document view-model for the supplied board pack.
func (b *Builder) Build(ctx context.Context, pack BoardPack) (DocumentData, error) {
	if pack.Template == nil {
		tpl, err := b.repo.GetTemplate(ctx, pack.TemplateID)
		if err != nil {
			return DocumentData{}, err
		}
		pack.Template = &tpl
	}
	company := Company{ID: pack.CompanyID, Code: pack.CompanyCode, Name: pack.CompanyName}
	period := Period{ID: pack.PeriodID, Name: pack.PeriodName, StartDate: pack.PeriodStart, EndDate: pack.PeriodEnd, Status: pack.PeriodStatus, CompanyID: pack.CompanyID}

	balances, err := b.repo.AggregateAccountBalances(ctx, pack.CompanyID, pack.PeriodID)
	if err != nil {
		return DocumentData{}, err
	}
	pl := reports.BuildProfitAndLoss(balances)
	bs := reports.BuildBalanceSheet(balances)

	warnings := make([]string, 0)
	kpiSummary := KPISummary{}
	if b.kpi != nil {
		summary, err := b.kpi.GetKPISummary(ctx, analytics.KPIFilter{Period: period.Name, CompanyID: company.ID, AsOf: period.EndDate})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("KPI summary gagal dimuat: %v", err))
		} else {
			kpiSummary = KPISummary{
				NetProfit:     summary.NetProfit,
				Revenue:       summary.Revenue,
				Opex:          summary.Opex,
				COGS:          summary.COGS,
				CashIn:        summary.CashIn,
				CashOut:       summary.CashOut,
				AROutstanding: summary.AROutstanding,
				APOutstanding: summary.APOutstanding,
			}
		}
	}

	sections := make([]SectionData, 0, len(pack.Template.Sections))
	generatedAt := b.now()
	requestedBy := metadataInt64(pack.Metadata, "requested_by")
	varianceLabel := metadataString(pack.Metadata, "variance_rule")

	for _, section := range pack.Template.Sections {
		switch section.Type {
		case SectionExecSummary:
			exec := &ExecSummaryData{
				Company:       company,
				Period:        period,
				RequestedBy:   requestedBy,
				VarianceLabel: varianceLabel,
				KPISummary:    kpiSummary,
				Status:        pack.Status,
			}
			sections = append(sections, SectionData{Type: SectionExecSummary, Title: section.Title, Exec: exec, HasContent: true})
		case SectionPLSummary:
			sections = append(sections, SectionData{Type: SectionPLSummary, Title: section.Title, Payload: pl, HasContent: len(pl.Revenue.Accounts)+len(pl.Expense.Accounts) > 0})
		case SectionBSSummary:
			sections = append(sections, SectionData{Type: SectionBSSummary, Title: section.Title, Payload: bs, HasContent: len(bs.Assets.Accounts)+len(bs.Liabilities.Accounts)+len(bs.Equity.Accounts) > 0})
		case SectionCashflow:
			cf := &CashflowSummary{CashIn: kpiSummary.CashIn, CashOut: kpiSummary.CashOut, Net: kpiSummary.CashIn - kpiSummary.CashOut}
			sections = append(sections, SectionData{Type: SectionCashflow, Title: section.Title, Cashflow: cf, HasContent: cf.CashIn != 0 || cf.CashOut != 0})
		case SectionTopVariances:
			limit := b.limitFromOptions(section.Options)
			rows, warn := b.loadTopVariances(ctx, pack, limit)
			if warn != "" {
				warnings = append(warnings, warn)
			}
			sections = append(sections, SectionData{Type: SectionTopVariances, Title: section.Title, Payload: rows, Limit: limit, HasContent: len(rows) > 0})
		default:
			warnings = append(warnings, fmt.Sprintf("Section %s tidak dikenal", section.Type))
		}
	}

	return DocumentData{
		Pack:        pack,
		Company:     company,
		Period:      period,
		Template:    *pack.Template,
		Sections:    sections,
		GeneratedAt: generatedAt,
		Warnings:    warnings,
	}, nil
}

func (b *Builder) limitFromOptions(opts map[string]any) int {
	if opts == nil {
		return b.topLimit
	}
	if raw, ok := opts["limit"]; ok {
		switch v := raw.(type) {
		case float64:
			if v > 0 && v < 100 {
				return int(v)
			}
		case int:
			if v > 0 && v < 100 {
				return v
			}
		}
	}
	return b.topLimit
}

func (b *Builder) loadTopVariances(ctx context.Context, pack BoardPack, limit int) ([]variance.VarianceRow, string) {
	if pack.VarianceSnapshotID == nil || b.variance == nil {
		return nil, ""
	}
	rows, err := b.variance.LoadSnapshotPayload(ctx, *pack.VarianceSnapshotID)
	if err != nil {
		return nil, fmt.Sprintf("Gagal memuat variance snapshot #%d: %v", *pack.VarianceSnapshotID, err)
	}
	sort.Slice(rows, func(i, j int) bool {
		return math.Abs(rows[i].Variance) > math.Abs(rows[j].Variance)
	})
	if limit > len(rows) {
		limit = len(rows)
	}
	return rows[:limit], ""
}

func metadataInt64(meta map[string]any, key string) *int64 {
	if meta == nil {
		return nil
	}
	if raw, ok := meta[key]; ok {
		switch v := raw.(type) {
		case float64:
			val := int64(v)
			if val != 0 {
				return &val
			}
		case int:
			val := int64(v)
			return &val
		case int64:
			if v != 0 {
				return &v
			}
		}
	}
	return nil
}

func metadataString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	if raw, ok := meta[key]; ok {
		switch v := raw.(type) {
		case string:
			return strings.TrimSpace(v)
		}
	}
	return ""
}
