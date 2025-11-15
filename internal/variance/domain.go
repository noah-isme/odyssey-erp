package variance

import (
	"errors"
	"strings"
	"time"
)

// SnapshotStatus enumerates async job lifecycle values.
type SnapshotStatus string

const (
	// SnapshotPending indicates waiting to be processed.
	SnapshotPending SnapshotStatus = "PENDING"
	// SnapshotInProgress indicates job executing.
	SnapshotInProgress SnapshotStatus = "IN_PROGRESS"
	// SnapshotReady indicates payload ready for consumption.
	SnapshotReady SnapshotStatus = "READY"
	// SnapshotFailed indicates error occurred.
	SnapshotFailed SnapshotStatus = "FAILED"
)

// RuleComparison enumerates supported variance comparisons.
type RuleComparison string

const (
	// ComparisonActualVsPrior compares two periods of actuals.
	ComparisonActualVsPrior RuleComparison = "ACTUAL_VS_ACTUAL"
)

// Rule defines a variance configuration per company or group.
type Rule struct {
	ID               int64
	CompanyID        int64
	Name             string
	ComparisonType   RuleComparison
	BasePeriodID     int64
	ComparePeriodID  *int64
	DimensionFilter  map[string]any
	ThresholdAmount  *float64
	ThresholdPercent *float64
	Active           bool
	CreatedBy        int64
	CreatedAt        time.Time
}

// Snapshot stores metadata + payload for a single calculation.
type Snapshot struct {
	ID          int64
	RuleID      int64
	PeriodID    int64
	Status      SnapshotStatus
	GeneratedAt *time.Time
	GeneratedBy *int64
	Error       string
	Payload     []VarianceRow
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Rule        *Rule
}

// VarianceRow describes UI row entry.
type VarianceRow struct {
	AccountCode   string  `json:"account_code"`
	AccountName   string  `json:"account_name"`
	BaseAmount    float64 `json:"base_amount"`
	CompareAmount float64 `json:"compare_amount"`
	Variance      float64 `json:"variance"`
	VariancePct   float64 `json:"variance_pct"`
	Flagged       bool    `json:"flagged"`
}

// CreateRuleInput captures rule creation input.
type CreateRuleInput struct {
	CompanyID        int64
	Name             string
	ComparisonType   RuleComparison
	BasePeriodID     int64
	ComparePeriodID  *int64
	ThresholdAmount  *float64
	ThresholdPercent *float64
	ActorID          int64
}

// Validate ensures correctness.
func (in CreateRuleInput) Validate() error {
	if in.CompanyID == 0 {
		return errors.New("variance: company required")
	}
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("variance: name required")
	}
	if in.BasePeriodID == 0 {
		return errors.New("variance: base period required")
	}
	if in.ActorID == 0 {
		return errors.New("variance: actor required")
	}
	if in.ComparisonType == ComparisonActualVsPrior && (in.ComparePeriodID == nil || *in.ComparePeriodID == 0) {
		return errors.New("variance: compare period required")
	}
	return nil
}

// SnapshotRequest configures a trigger for variance computation.
type SnapshotRequest struct {
	RuleID   int64
	PeriodID int64
	ActorID  int64
}

// Validate ensures request is valid.
func (r SnapshotRequest) Validate() error {
	if r.RuleID == 0 || r.PeriodID == 0 {
		return errors.New("variance: rule and period required")
	}
	if r.ActorID == 0 {
		return errors.New("variance: actor required")
	}
	return nil
}

var (
	// ErrRuleNotFound occurs when rule missing.
	ErrRuleNotFound = errors.New("variance: rule not found")
	// ErrSnapshotNotFound occurs when snapshot missing.
	ErrSnapshotNotFound = errors.New("variance: snapshot not found")
)
