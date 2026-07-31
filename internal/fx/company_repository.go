package fx

import (
	"context"
	"fmt"
)

func (r *SQLRepository) CompanyBaseCurrencies(ctx context.Context) ([]string, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("fx repository: pool is required")
	}
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT base_currency FROM companies ORDER BY base_currency`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var currency string
		if err := rows.Scan(&currency); err != nil {
			return nil, err
		}
		result = append(result, currency)
	}
	return result, rows.Err()
}
