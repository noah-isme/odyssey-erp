-- name: CreateBankAccount :one
INSERT INTO bank_accounts (
    company_id, name, account_number, currency, gl_account_id, initial_balance, is_active
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: GetBankAccount :one
SELECT * FROM bank_accounts WHERE id = $1;

-- name: ListBankAccounts :many
SELECT * FROM bank_accounts 
WHERE company_id = $1 
ORDER BY name;

-- name: UpdateBankAccount :exec
UPDATE bank_accounts
SET name = $2, account_number = $3, gl_account_id = $4, is_active = $5
WHERE id = $1;

-- name: CreateBankTransaction :one
INSERT INTO bank_transactions (
    id, bank_account_id, date, amount, description, reference, status, gl_journal_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetBankTransaction :one
SELECT * FROM bank_transactions WHERE id = $1;

-- name: ListBankTransactions :many
SELECT * FROM bank_transactions
WHERE bank_account_id = $1
ORDER BY date DESC, created_at DESC;

-- name: UpdateBankTransactionStatus :exec
UPDATE bank_transactions
SET status = $2, gl_journal_id = $3
WHERE id = $1;
