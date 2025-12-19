package elimination

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ListFilters captures pagination and sorting parameters.
type ListFilters struct {
	Page    int
	Limit   int
	SortBy  string
	SortDir string
}

// RunStatus captures lifecycle of an elimination run.
type RunStatus string

const (
	// RunStatusDraft indicates the run has been created but not simulated.
	RunStatusDraft RunStatus = "DRAFT"
	// RunStatusSimulated indicates balances have been analysed.
	RunStatusSimulated RunStatus = "SIMULATED"
	// RunStatusPosted indicates the run has posted a journal entry.
	RunStatusPosted RunStatus = "POSTED"
	// RunStatusFailed indicates the run failed during processing.
	RunStatusFailed RunStatus = "FAILED"
)

// Rule describes how to eliminate intercompany balances between two companies.
type Rule struct {
	ID              int64
	GroupID         *int64
	Name            string
	SourceCompanyID int64
	TargetCompanyID int64
	AccountSource   string
	AccountTarget   string
	MatchCriteria   map[string]any
	Active          bool
	CreatedBy       int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Run aggregates simulation/posting metadata for a rule + period.
type Run struct {
	ID           int64
	PeriodID     int64
	RuleID       int64
	Status       RunStatus
	CreatedBy    int64
	CreatedAt    time.Time
	SimulatedAt  *time.Time
	PostedAt     *time.Time
	JournalEntry *int64
	Summary      *SimulationSummary
	Rule         *Rule
}

// SimulationSummary stores computed balances to be rendered on UI.
type SimulationSummary struct {
	SourceBalance float64 `json:"source_balance"`
	TargetBalance float64 `json:"target_balance"`
	Eliminated    float64 `json:"eliminated"`
}

// PeriodView represents minimal accounting period metadata for UI forms.
type PeriodView struct {
	ID        int64
	LedgerID  int64
	Name      string
	StartDate time.Time
	EndDate   time.Time
}

// CreateRuleInput validates new elimination configuration.
type CreateRuleInput struct {
	GroupID         *int64
	Name            string
	SourceCompanyID int64
	TargetCompanyID int64
	AccountSource   string
	AccountTarget   string
	MatchCriteria   map[string]any
	ActorID         int64
}

// Validate ensures the request is coherent.
func (in CreateRuleInput) Validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("elimination: name required")
	}
	if in.SourceCompanyID == 0 || in.TargetCompanyID == 0 {
		return errors.New("elimination: company pair required")
	}
	if in.SourceCompanyID == in.TargetCompanyID {
		return errors.New("elimination: companies must differ")
	}
	if strings.TrimSpace(in.AccountSource) == "" || strings.TrimSpace(in.AccountTarget) == "" {
		return errors.New("elimination: account codes required")
	}
	if in.ActorID == 0 {
		return errors.New("elimination: actor required")
	}
	return nil
}

// UpdateRuleInput mutates existing rule metadata.
type UpdateRuleInput struct {
	Name          string
	AccountSource string
	AccountTarget string
	Active        bool
}

// Validate ensures update input remains valid.
func (in UpdateRuleInput) Validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("elimination: name required")
	}
	if strings.TrimSpace(in.AccountSource) == "" || strings.TrimSpace(in.AccountTarget) == "" {
		return errors.New("elimination: account codes required")
	}
	return nil
}

// CreateRunInput captures fields necessary to prepare a run.
type CreateRunInput struct {
	PeriodID int64
	RuleID   int64
	ActorID  int64
}

// Validate ensures identifiers are supplied.
func (in CreateRunInput) Validate() error {
	if in.PeriodID == 0 || in.RuleID == 0 {
		return errors.New("elimination: period and rule required")
	}
	if in.ActorID == 0 {
		return errors.New("elimination: actor required")
	}
	return nil
}

// ErrRuleNotFound occurs when rule lookup fails.
var ErrRuleNotFound = errors.New("elimination: rule not found")

// ErrRunNotFound occurs when run lookup fails.
var ErrRunNotFound = errors.New("elimination: run not found")

// ErrNoElimination occurs when balances net to zero.
var ErrNoElimination = errors.New("elimination: nothing to eliminate")

// ErrInvalidStatus indicates that the transition isn't allowed.
var ErrInvalidStatus = errors.New("elimination: invalid run status")

// ErrAccountNotFound indicates the referenced account code is missing.
var ErrAccountNotFound = errors.New("elimination: account code not found")

// ErrPeriodNotFound indicates the accounting period is missing.
var ErrPeriodNotFound = errors.New("elimination: accounting period not found")

// String implements fmt.Stringer for debugging.
func (s RunStatus) String() string {
	return string(s)
}

// FormatMemo renders a consistent memo for posted journals.
func FormatMemo(rule Rule, period PeriodView) string {
	return fmt.Sprintf("Elimination %s - %s", rule.Name, period.Name)
}
