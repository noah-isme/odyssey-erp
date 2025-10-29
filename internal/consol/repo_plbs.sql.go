package consol

import (
	"context"
	"errors"
)

// ConsolBalanceByTypeQueryRow mirrors the SQL response when aggregating balances by account type.
type ConsolBalanceByTypeQueryRow struct {
	GroupAccountID int64
	AccountType    string
	LocalAmount    float64
	GroupAmount    float64
}

// ConsolBalancesByType fetches balances grouped by their account type classification.
func (r *Repository) ConsolBalancesByType(ctx context.Context, groupID int64, periodCode string, entities []int64) ([]ConsolBalanceByTypeQueryRow, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("consol repo not initialised")
	}
	// Implementation will be provided in subsequent commits.
	return nil, errors.New("consol: ConsolBalancesByType not implemented")
}
