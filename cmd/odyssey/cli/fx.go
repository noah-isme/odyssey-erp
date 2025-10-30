package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/consol"
	"github.com/odyssey-erp/odyssey-erp/internal/consol/fx"
)

// FXRepository exposes the subset of consolidation repository features needed by the FX CLI.
type FXRepository interface {
	GroupReportingCurrency(ctx context.Context, groupID int64) (string, error)
	MemberCurrencies(ctx context.Context, groupID int64) (map[int64]string, error)
	FxRateForPeriod(ctx context.Context, asOf time.Time, pair string) (fx.Quote, error)
	UpsertFxRates(ctx context.Context, rows []consol.FxRateInput) error
}

// FXOpsCLI offers operational helpers to manage FX rates used by consolidation.
type FXOpsCLI struct {
	repo FXRepository
}

// NewFXOpsCLI constructs a new helper instance wired to the provided repository.
func NewFXOpsCLI(repo FXRepository) (*FXOpsCLI, error) {
	if repo == nil {
		return nil, fmt.Errorf("fx cli: repository is required")
	}
	return &FXOpsCLI{repo: repo}, nil
}

// ErrFXOpsNotImplemented indicates that the helper is pending implementation.
var ErrFXOpsNotImplemented = errors.New("consol: fx ops cli not implemented")

// ValidateParams captures the scope for FX validation runs.
type ValidateParams struct {
	GroupID int64
	Period  time.Time
	Pairs   []string
}

// ValidateResult bundles the validator outcome together with reporting metadata.
type ValidateResult struct {
	GroupID            int64
	Result             fx.Result
	ReportingCurrency  string
	ConsideredPairs    []string
	RequestedPairNames []string
}

// ValidateGaps inspects FX rate gaps for the configured policy.
func (c *FXOpsCLI) ValidateGaps(ctx context.Context, params ValidateParams) (ValidateResult, error) {
	var empty ValidateResult
	if c == nil || c.repo == nil {
		return empty, fmt.Errorf("fx cli: client not configured")
	}
	if params.GroupID <= 0 {
		return empty, fmt.Errorf("fx cli: group id is required")
	}
	if params.Period.IsZero() {
		return empty, fmt.Errorf("fx cli: period is required")
	}
	period := time.Date(params.Period.Year(), params.Period.Month(), 1, 0, 0, 0, 0, time.UTC)
	reportingCurrency, err := c.repo.GroupReportingCurrency(ctx, params.GroupID)
	if err != nil {
		return empty, err
	}
	reportingCurrency = strings.ToUpper(strings.TrimSpace(reportingCurrency))
	if reportingCurrency == "" {
		reportingCurrency = "IDR"
	}
	currencies, err := c.repo.MemberCurrencies(ctx, params.GroupID)
	if err != nil {
		return empty, err
	}
	requiredPairs := make(map[string]struct{})
	requestedPairs := make([]string, 0, len(params.Pairs))
	for _, pair := range params.Pairs {
		normalised := strings.ToUpper(strings.TrimSpace(pair))
		if normalised == "" {
			continue
		}
		requiredPairs[normalised] = struct{}{}
		requestedPairs = append(requestedPairs, normalised)
	}
	for _, currency := range currencies {
		local := strings.ToUpper(strings.TrimSpace(currency))
		if local == "" || local == reportingCurrency {
			continue
		}
		pair := local + reportingCurrency
		requiredPairs[pair] = struct{}{}
	}
	pairs := make([]string, 0, len(requiredPairs))
	for pair := range requiredPairs {
		pairs = append(pairs, pair)
	}
	sort.Strings(pairs)
	requirements := make([]fx.Requirement, 0, len(pairs))
	for _, pair := range pairs {
		requirements = append(requirements, fx.Requirement{Pair: pair, Methods: []fx.Method{fx.MethodAverage, fx.MethodClosing}})
	}
	provider := repoQuoteProvider{repo: c.repo}
	result, err := fx.Validate(ctx, provider, period, requirements)
	if err != nil {
		return empty, err
	}
	return ValidateResult{
		GroupID:            params.GroupID,
		Result:             result,
		ReportingCurrency:  reportingCurrency,
		ConsideredPairs:    pairs,
		RequestedPairNames: requestedPairs,
	}, nil
}

// ImportRates ingests FX rates into the system.
func (c *FXOpsCLI) ImportRates(path string) error {
	return ErrFXOpsNotImplemented
}

// repoQuoteProvider satisfies fx.QuoteProvider using the repository lookup.
type repoQuoteProvider struct {
	repo FXRepository
}

func (p repoQuoteProvider) QuoteForPeriod(ctx context.Context, asOf time.Time, pair string) (fx.Quote, bool, error) {
	if p.repo == nil {
		return fx.Quote{}, false, fmt.Errorf("fx cli: repository not configured")
	}
	quote, err := p.repo.FxRateForPeriod(ctx, asOf, pair)
	if err != nil {
		if errors.Is(err, consol.ErrFxRateNotFound) {
			return fx.Quote{}, false, nil
		}
		return fx.Quote{}, false, err
	}
	return quote, true, nil
}
