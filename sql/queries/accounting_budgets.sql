-- name: GetBudget :one
SELECT * FROM accounting_budgets
WHERE account_id = $1 AND period_year = $2 AND period_month = $3;

-- name: ListBudgetsByPeriod :many
SELECT * FROM accounting_budgets
WHERE period_year = $1 AND period_month = $2;

-- name: ListBudgetsByYear :many
SELECT * FROM accounting_budgets
WHERE period_year = $1;

-- name: UpsertBudget :one
INSERT INTO accounting_budgets (account_id, period_year, period_month, amount)
VALUES ($1, $2, $3, $4)
ON CONFLICT (account_id, period_year, period_month)
DO UPDATE SET amount = EXCLUDED.amount, updated_at = NOW()
RETURNING *;
