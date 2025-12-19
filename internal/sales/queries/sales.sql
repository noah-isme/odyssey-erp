-- =============================================================================
-- CUSTOMERS
-- =============================================================================

-- name: GetCustomer :one
SELECT id, code, name, company_id, email, phone, tax_id,
       credit_limit, payment_terms_days, address_line1, address_line2,
       city, state, postal_code, country, is_active, notes,
       created_by, created_at, updated_at
FROM customers
WHERE id = $1;

-- name: GetCustomerByCode :one
SELECT id, code, name, company_id, email, phone, tax_id,
       credit_limit, payment_terms_days, address_line1, address_line2,
       city, state, postal_code, country, is_active, notes,
       created_by, created_at, updated_at
FROM customers
WHERE company_id = $1 AND code = $2;

-- name: CreateCustomer :one
INSERT INTO customers (
    code, name, company_id, email, phone, tax_id,
    credit_limit, payment_terms_days, address_line1, address_line2,
    city, state, postal_code, country, is_active, notes, created_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
RETURNING id;

-- name: UpdateCustomer :exec
UPDATE customers SET 
    name = COALESCE(sqlc.narg('name'), name),
    email = COALESCE(sqlc.narg('email'), email),
    phone = COALESCE(sqlc.narg('phone'), phone),
    address_line1 = COALESCE(sqlc.narg('address_line1'), address_line1),
    address_line2 = COALESCE(sqlc.narg('address_line2'), address_line2),
    city = COALESCE(sqlc.narg('city'), city),
    state = COALESCE(sqlc.narg('state'), state),
    postal_code = COALESCE(sqlc.narg('postal_code'), postal_code),
    country = COALESCE(sqlc.narg('country'), country),
    is_active = COALESCE(sqlc.narg('is_active'), is_active),
    notes = COALESCE(sqlc.narg('notes'), notes),
    updated_at = NOW()
WHERE id = sqlc.arg('id');

-- =============================================================================
-- QUOTATIONS
-- =============================================================================

-- name: GetQuotation :one
SELECT id, doc_number, company_id, customer_id, quote_date, valid_until,
       status, currency, subtotal, tax_amount, total_amount, notes,
       created_by, approved_by, approved_at, rejected_by, rejected_at,
       rejection_reason, created_at, updated_at
FROM quotations
WHERE id = $1;

-- name: GetQuotationByDocNumber :one
SELECT id, doc_number, company_id, customer_id, quote_date, valid_until,
       status, currency, subtotal, tax_amount, total_amount, notes,
       created_by, approved_by, approved_at, rejected_by, rejected_at,
       rejection_reason, created_at, updated_at
FROM quotations
WHERE doc_number = $1;

-- name: CreateQuotation :one
INSERT INTO quotations (
    doc_number, company_id, customer_id, quote_date, valid_until,
    status, currency, subtotal, tax_amount, total_amount, notes, created_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id;

-- name: InsertQuotationLine :one
INSERT INTO quotation_lines (
    quotation_id, product_id, description, quantity, uom,
    unit_price, discount_percent, discount_amount, tax_percent,
    tax_amount, line_total, notes, line_order
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id;

-- name: GetQuotationLines :many
SELECT id, quotation_id, product_id, description, quantity, uom,
       unit_price, discount_percent, discount_amount, tax_percent,
       tax_amount, line_total, notes, line_order, created_at, updated_at
FROM quotation_lines
WHERE quotation_id = $1
ORDER BY line_order, id;

-- name: UpdateQuotationStatus :exec
UPDATE quotations SET 
    status = $1, 
    approved_by = sqlc.narg('approved_by'),
    approved_at = sqlc.narg('approved_at'),
    rejected_by = sqlc.narg('rejected_by'),
    rejected_at = sqlc.narg('rejected_at'),
    rejection_reason = sqlc.narg('rejection_reason'),
    updated_at = NOW()
WHERE id = $2;

-- name: DeleteQuotationLines :exec
DELETE FROM quotation_lines WHERE quotation_id = $1;

-- =============================================================================
-- SALES ORDERS
-- =============================================================================

-- name: GetSalesOrder :one
SELECT id, doc_number, company_id, customer_id, quotation_id, order_date,
       expected_delivery_date, status, currency, subtotal, tax_amount, total_amount,
       notes, created_by, confirmed_by, confirmed_at, cancelled_by, cancelled_at,
       cancellation_reason, created_at, updated_at
FROM sales_orders
WHERE id = $1;

-- name: GetSalesOrderByDocNumber :one
SELECT id, doc_number, company_id, customer_id, quotation_id, order_date,
       expected_delivery_date, status, currency, subtotal, tax_amount, total_amount,
       notes, created_by, confirmed_by, confirmed_at, cancelled_by, cancelled_at,
       cancellation_reason, created_at, updated_at
FROM sales_orders
WHERE doc_number = $1;

-- name: CreateSalesOrder :one
INSERT INTO sales_orders (
    doc_number, company_id, customer_id, quotation_id, order_date,
    expected_delivery_date, status, currency, subtotal, tax_amount,
    total_amount, notes, created_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id;

-- name: InsertSalesOrderLine :one
INSERT INTO sales_order_lines (
    sales_order_id, product_id, description, quantity,
    quantity_delivered, quantity_invoiced, uom, unit_price,
    discount_percent, discount_amount, tax_percent, tax_amount,
    line_total, notes, line_order
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING id;

-- name: GetSalesOrderLines :many
SELECT id, sales_order_id, product_id, description, quantity,
       quantity_delivered, quantity_invoiced, uom, unit_price,
       discount_percent, discount_amount, tax_percent, tax_amount,
       line_total, notes, line_order, created_at, updated_at
FROM sales_order_lines
WHERE sales_order_id = $1
ORDER BY line_order, id;

-- name: UpdateSalesOrderStatus :exec
UPDATE sales_orders SET 
    status = $1, 
    confirmed_by = sqlc.narg('confirmed_by'),
    confirmed_at = sqlc.narg('confirmed_at'),
    cancelled_by = sqlc.narg('cancelled_by'),
    cancelled_at = sqlc.narg('cancelled_at'),
    cancellation_reason = sqlc.narg('cancellation_reason'),
    updated_at = NOW()
WHERE id = $2;

-- name: DeleteSalesOrderLines :exec
DELETE FROM sales_order_lines WHERE sales_order_id = $1;

-- name: UpdateSalesOrderLineDelivered :exec
UPDATE sales_order_lines SET quantity_delivered = $1, updated_at = NOW() WHERE id = $2;
