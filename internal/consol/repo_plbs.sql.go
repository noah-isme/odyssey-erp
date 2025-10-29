package consol

import (
	"context"
	"errors"
)

// ConsolBalanceByTypeQueryRow mirrors the SQL response when aggregating balances by account type.
type ConsolBalanceByTypeQueryRow struct {
	GroupAccountID   int64
	GroupAccountCode string
	GroupAccountName string
	AccountType      string
	LocalAmount      float64
	GroupAmount      float64
	MembersJSON      []byte
}

// ConsolBalancesByType fetches balances grouped by their account type classification.
func (r *Repository) ConsolBalancesByType(ctx context.Context, groupID int64, periodCode string, entities []int64) ([]ConsolBalanceByTypeQueryRow, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("consol repo not initialised")
	}
	if groupID <= 0 {
		return nil, errors.New("consol: invalid group id")
	}
	if periodCode == "" {
		return nil, errors.New("consol: period code required")
	}

	periodID, err := r.FindPeriodID(ctx, periodCode)
	if err != nil {
		return nil, err
	}

	const query = `
SELECT
    mv.group_account_id,
    ga.code,
    ga.name,
    ga.type,
    mv.local_ccy_amt,
    mv.group_ccy_amt,
    mv.members
FROM mv_consol_balances mv
JOIN consol_group_accounts ga ON ga.id = mv.group_account_id
WHERE mv.group_id = $1 AND mv.period_id = $2
ORDER BY ga.code`

	rows, err := r.pool.Query(ctx, query, groupID, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ConsolBalanceByTypeQueryRow
	for rows.Next() {
		var row ConsolBalanceByTypeQueryRow
		if err := rows.Scan(&row.GroupAccountID, &row.GroupAccountCode, &row.GroupAccountName, &row.AccountType, &row.LocalAmount, &row.GroupAmount, &row.MembersJSON); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
