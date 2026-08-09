-- name: SupplierBelongsToCompany :one
SELECT EXISTS(
    SELECT 1 FROM suppliers WHERE id = $1 AND company_id = $2
);

-- name: CreateTreasurySupplierBankAccount :one
INSERT INTO treasury_supplier_bank_accounts (
    company_id, supplier_id, bank_name, account_number, routing_number, 
    currency, effective_from, effective_to, verification_status, evidence_ref, 
    hold_payments, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, 'PENDING_APPROVAL', $9, TRUE, $10
) RETURNING *;

-- name: UpdateTreasurySupplierBankAccountVerification :one
UPDATE treasury_supplier_bank_accounts
SET verification_status = $2,
    hold_payments = $3,
    approved_by = $4,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: GetTreasurySupplierBankAccount :one
SELECT * FROM treasury_supplier_bank_accounts WHERE id = $1;

-- name: ListTreasurySupplierBankAccounts :many
SELECT * FROM treasury_supplier_bank_accounts 
WHERE supplier_id = $1 AND company_id = $2
ORDER BY effective_from DESC;

-- name: GetTreasuryPaymentPolicy :one
SELECT * FROM treasury_payment_policies WHERE company_id = $1 LIMIT 1;

-- name: APInvoiceEligibleForTreasuryPayment :one
SELECT EXISTS(
    SELECT 1
    FROM ap_invoices i
    JOIN suppliers s ON s.id = i.supplier_id
    WHERE i.id = $1
      AND i.supplier_id = $2
      AND s.company_id = $3
      AND i.currency = $4
      AND i.status = 'POSTED'
      AND i.total > COALESCE((
          SELECT SUM(pa.amount)
          FROM ap_payment_allocations pa
          WHERE pa.ap_invoice_id = i.id
      ), 0)
);
