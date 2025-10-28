package shared

import "errors"

// Period statuses reused outside accounting module.
const (
	PeriodStatusOpen   = "OPEN"
	PeriodStatusClosed = "CLOSED"
	PeriodStatusLocked = "LOCKED"
)

// ErrInvalidPeriodTransition indicates status change not allowed.
var ErrInvalidPeriodTransition = errors.New("period transition invalid")

// ValidatePeriodTransition checks transitions according to policy.
func ValidatePeriodTransition(current, target string, hasOverride bool) error {
	if current == target {
		return nil
	}
	switch current {
	case PeriodStatusOpen:
		if target == PeriodStatusClosed || target == PeriodStatusLocked {
			return nil
		}
	case PeriodStatusClosed:
		if target == PeriodStatusOpen {
			return nil
		}
		if target == PeriodStatusLocked {
			return nil
		}
	case PeriodStatusLocked:
		if target == PeriodStatusClosed && hasOverride {
			return nil
		}
	}
	return ErrInvalidPeriodTransition
}
