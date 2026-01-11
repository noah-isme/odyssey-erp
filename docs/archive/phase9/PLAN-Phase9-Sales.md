# Phase 9 – Sales & Accounts Receivable (AR)

## Executive Summary

Phase 9 melengkapi siklus revenue dengan membangun modul Sales dan Accounts Receivable (AR) sebagai counterpart dari Procurement/AP yang sudah ada di Phase 3. Fokus utama adalah quotation management, sales order processing, delivery fulfillment, invoicing, AR aging, dan payment collection—semuanya terintegrasi dengan inventory, accounting, dan RBAC.

## Business Context

- **Why Now**: Procurement (AP) dan Inventory sudah GA; bisnis butuh mencatat penjualan, revenue recognition, dan piutang pelanggan.
- **Value Proposition**: Single source of truth untuk sales pipeline, automated AR journals, real-time aging reports, dan delivery tracking tanpa spreadsheet manual.
- **Stakeholders**: Sales team, finance (AR clerk, controller), warehouse (fulfillment), management (revenue analytics).

## Scope Overview

Phase 9 dibagi menjadi **3 cycles** dengan total estimasi 15–20 hari kerja:

| Cycle | Focus | Deliverables | Estimasi |
|-------|-------|--------------|----------|
| **9.1** | Quotation & Sales Order | Domain, CRUD, approval flow, SSR UI | 5–6 hari |
| **9.2** | Delivery & Fulfillment | Delivery order, stock reduction, packing list PDF | 4–5 hari |
| **9.3** | AR Invoice & Payment | AR invoice, payment allocation, aging report, integrations | 6–7 hari |

---

## Cycle 9.1 – Quotation & Sales Order

### Objectives

- Bangun domain model untuk quotation (penawaran) dan sales order (SO).
- Implementasi approval workflow sederhana (draft → approved → confirmed).
- SSR UI untuk list, create, edit, approve, dan convert quotation → SO.
- Integrasi dengan master data (customers, products) dan RBAC.

### Database Schema (`000011_phase9_sales_quotation_so.up.sql`)

```sql
-- Sales Quotation
CREATE TYPE quotation_status AS ENUM ('DRAFT','SUBMITTED','APPROVED','REJECTED','CONVERTED');

CREATE TABLE quotations (
    id BIGSERIAL PRIMARY KEY,
    doc_number TEXT NOT NULL UNIQUE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    quote_date DATE NOT NULL,
    valid_until DATE NOT NULL,
    status quotation_status NOT NULL DEFAULT 'DRAFT',
    subtotal NUMERIC(18,2) NOT NULL DEFAULT 0,
    tax_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
    total_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
    notes TEXT,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    approved_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    approved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_quotation_dates CHECK (quote_date <= valid_until)
);

CREATE TABLE quotation_lines (
    id BIGSERIAL PRIMARY KEY,
    quotation_id BIGINT NOT NULL REFERENCES quotations(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    quantity NUMERIC(14,4) NOT NULL CHECK (quantity > 0),
    unit_price NUMERIC(18,2) NOT NULL CHECK (unit_price >= 0),
    discount_percent NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (discount_percent >= 0 AND discount_percent <= 100),
    tax_percent NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (tax_percent >= 0),
    line_total NUMERIC(18,2) NOT NULL,
    notes TEXT,
    line_order INT NOT NULL DEFAULT 0
);

-- Sales Order
CREATE TYPE sales_order_status AS ENUM ('DRAFT','CONFIRMED','PROCESSING','COMPLETED','CANCELLED');

CREATE TABLE sales_orders (
    id BIGSERIAL PRIMARY KEY,
    doc_number TEXT NOT NULL UNIQUE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    quotation_id BIGINT REFERENCES quotations(id) ON DELETE SET NULL,
    order_date DATE NOT NULL,
    delivery_date DATE,
    status sales_order_status NOT NULL DEFAULT 'DRAFT',
    subtotal NUMERIC(18,2) NOT NULL DEFAULT 0,
    tax_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
    total_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
    notes TEXT,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    confirmed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    confirmed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE sales_order_lines (
    id BIGSERIAL PRIMARY KEY,
    sales_order_id BIGINT NOT NULL REFERENCES sales_orders(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    quantity NUMERIC(14,4) NOT NULL CHECK (quantity > 0),
    quantity_delivered NUMERIC(14,4) NOT NULL DEFAULT 0 CHECK (quantity_delivered >= 0),
    unit_price NUMERIC(18,2) NOT NULL CHECK (unit_price >= 0),
    discount_percent NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (discount_percent >= 0 AND discount_percent <= 100),
    tax_percent NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (tax_percent >= 0),
    line_total NUMERIC(18,2) NOT NULL,
    notes TEXT,
    line_order INT NOT NULL DEFAULT 0,
    CONSTRAINT chk_qty_delivered CHECK (quantity_delivered <= quantity)
);

CREATE INDEX idx_quotations_company_status ON quotations(company_id, status);
CREATE INDEX idx_quotations_customer ON quotations(customer_id);
CREATE INDEX idx_quotations_created_by ON quotations(created_by);
CREATE INDEX idx_quotation_lines_quotation ON quotation_lines(quotation_id);
CREATE INDEX idx_quotation_lines_product ON quotation_lines(product_id);

CREATE INDEX idx_sales_orders_company_status ON sales_orders(company_id, status);
CREATE INDEX idx_sales_orders_customer ON sales_orders(customer_id);
CREATE INDEX idx_sales_orders_quotation ON sales_orders(quotation_id);
CREATE INDEX idx_sales_order_lines_so ON sales_order_lines(sales_order_id);
CREATE INDEX idx_sales_order_lines_product ON sales_order_lines(product_id);
```

### Module Structure (`internal/sales/`)

```
internal/sales/
├── domain.go          // Quotation, QuotationLine, SalesOrder, SalesOrderLine entities
├── repository.go      // CRUD + queries (list, by status, by customer, etc)
├── service.go         // business logic (create, approve, convert, cancel)
├── http/
│   └── handler.go     // SSR handlers untuk quotation & SO CRUD
└── service_test.go
```

### Key Business Rules

1. **Quotation Approval**:
   - Draft → Submitted (by creator)
   - Submitted → Approved/Rejected (by user dengan permission `sales.quotation.approve`)
   - Approved quotation dapat di-convert menjadi Sales Order
   - Rejected/Converted quotation tidak dapat diedit lagi

2. **Sales Order Confirmation**:
   - Draft → Confirmed (by user dengan permission `sales.order.confirm`)
   - Confirmed order tidak dapat diedit, hanya bisa cancel atau proceed ke delivery

3. **Convert Quotation → SO**:
   - Copy semua line items dari quotation
   - Tandai quotation sebagai CONVERTED
   - SO baru dalam status DRAFT

4. **Stock Reservation** (opsional di 9.1, implement penuh di 9.2):
   - Saat SO confirmed, sistem bisa optional check available stock
   - Jika stock tidak cukup, warning tapi tidak blocking (soft check)

### RBAC Permissions

```
sales.quotation.view
sales.quotation.create
sales.quotation.edit
sales.quotation.approve
sales.quotation.delete

sales.order.view
sales.order.create
sales.order.edit
sales.order.confirm
sales.order.cancel
sales.order.delete
```

### SSR UI Pages

1. **`/sales/quotations`** – List dengan filter (status, customer, date range), pagination
2. **`/sales/quotations/new`** – Form create quotation (customer picker, line items table)
3. **`/sales/quotations/{id}`** – Detail view dengan action buttons (submit, approve, reject, convert)
4. **`/sales/quotations/{id}/edit`** – Edit form (hanya untuk DRAFT)
5. **`/sales/orders`** – List SO dengan filter
6. **`/sales/orders/new`** – Form create SO manual (atau via convert)
7. **`/sales/orders/{id}`** – Detail SO dengan action buttons (confirm, cancel)
8. **`/sales/orders/{id}/edit`** – Edit SO (hanya untuk DRAFT)

### Testing Strategy

- **Unit tests**: service logic (create, approve, convert, validation)
- **Integration tests**: repository queries dengan test DB
- **HTTP tests**: handler responses (200, 403 RBAC, 422 validation)
- **E2E scenario**: create quotation → approve → convert to SO → confirm

### Documentation

- `docs/howto-sales-quotation.md` – panduan user untuk quotation flow
- `docs/runbook-sales.md` – operational runbook
- Update `docs/CHANGELOG.md` untuk Cycle 9.1

### Acceptance Criteria

- ✅ Migration 000011 dapat dijalankan tanpa error
- ✅ CRUD quotation dan SO via SSR UI berfungsi
- ✅ Approval workflow terimplementasi dengan RBAC
- ✅ Convert quotation → SO bekerja dengan benar
- ✅ Unit tests coverage ≥ 70%
- ✅ Dokumentasi lengkap

---

## Cycle 9.2 – Delivery & Fulfillment

### Objectives

- Bangun delivery order (DO) untuk fulfill sales order
- Integrasi dengan inventory untuk stock reduction
- Generate packing list PDF
- Track partial delivery dan completion status

### Database Schema (`000012_phase9_delivery.up.sql`)

```sql
CREATE TYPE delivery_order_status AS ENUM ('DRAFT','CONFIRMED','IN_TRANSIT','DELIVERED','CANCELLED');

CREATE TABLE delivery_orders (
    id BIGSERIAL PRIMARY KEY,
    doc_number TEXT NOT NULL UNIQUE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    sales_order_id BIGINT NOT NULL REFERENCES sales_orders(id) ON DELETE CASCADE,
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE RESTRICT,
    delivery_date DATE NOT NULL,
    status delivery_order_status NOT NULL DEFAULT 'DRAFT',
    driver_name TEXT,
    vehicle_number TEXT,
    notes TEXT,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    confirmed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    confirmed_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE delivery_order_lines (
    id BIGSERIAL PRIMARY KEY,
    delivery_order_id BIGINT NOT NULL REFERENCES delivery_orders(id) ON DELETE CASCADE,
    sales_order_line_id BIGINT NOT NULL REFERENCES sales_order_lines(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    quantity_to_deliver NUMERIC(14,4) NOT NULL CHECK (quantity_to_deliver > 0),
    quantity_delivered NUMERIC(14,4) NOT NULL DEFAULT 0,
    notes TEXT,
    line_order INT NOT NULL DEFAULT 0
);

CREATE INDEX idx_delivery_orders_company_status ON delivery_orders(company_id, status);
CREATE INDEX idx_delivery_orders_so ON delivery_orders(sales_order_id);
CREATE INDEX idx_delivery_orders_warehouse ON delivery_orders(warehouse_id);
CREATE INDEX idx_delivery_order_lines_do ON delivery_order_lines(delivery_order_id);
CREATE INDEX idx_delivery_order_lines_sol ON delivery_order_lines(sales_order_line_id);
```

### Module Structure (`internal/delivery/`)

```
internal/delivery/
├── domain.go
├── repository.go
├── service.go         // create DO from SO, confirm DO, update inventory
├── job.go             // async inventory reduction job
├── http/
│   └── handler.go
├── pdf_generator.go   // packing list PDF via Gotenberg
└── service_test.go
```

### Key Business Rules

1. **Create Delivery Order**:
   - User memilih SO yang status = CONFIRMED atau PROCESSING
   - System populate DO lines dari SO lines yang belum fully delivered
   - User dapat adjust quantity_to_deliver (partial delivery support)

2. **Confirm Delivery Order**:
   - Validate stock availability di warehouse
   - Create inventory transaction (SALES_OUT) untuk reduce stock
   - Update `quantity_delivered` di `sales_order_lines`
   - Jika semua SO lines fully delivered → update SO status = COMPLETED
   - Jika partial → SO status = PROCESSING

3. **Stock Integration**:
   - Reuse `inventory_tx` table dari Phase 3
   - Transaction type = 'SALES_OUT'
   - Reference DO ID
   - Update product stock balances

4. **PDF Packing List**:
   - Generate via Gotenberg (reuse pattern dari boardpack)
   - Include: DO number, customer info, product list, quantities, driver info
   - Downloadable dari DO detail page

### Background Jobs

- **`delivery:confirm`** – Async job untuk confirm DO + inventory reduction
- **`delivery:pdf-generate`** – Generate packing list PDF (opsional, bisa sync juga)

### RBAC Permissions

```
sales.delivery.view
sales.delivery.create
sales.delivery.confirm
sales.delivery.cancel
sales.delivery.download_pdf
```

### SSR UI Pages

1. **`/sales/deliveries`** – List DO dengan filter
2. **`/sales/deliveries/new?so_id={id}`** – Create DO from SO
3. **`/sales/deliveries/{id}`** – Detail DO dengan action buttons (confirm, cancel, download PDF)
4. **`/sales/deliveries/{id}/edit`** – Edit DO (hanya DRAFT)

### Testing Strategy

- **Unit tests**: DO creation logic, stock validation, partial delivery calculation
- **Integration tests**: inventory_tx creation, SO status updates
- **Job tests**: async confirm job dengan retry scenarios
- **PDF tests**: verify Gotenberg integration

### Documentation

- `docs/howto-sales-delivery.md`
- `docs/runbook-sales-delivery.md`
- Update CHANGELOG

### Acceptance Criteria

- ✅ DO dapat dibuat dari SO
- ✅ Partial delivery support
- ✅ Stock reduction terintegrasi dengan inventory
- ✅ SO status auto-update berdasarkan delivery progress
- ✅ Packing list PDF dapat diunduh
- ✅ RBAC enforced
- ✅ Tests coverage ≥ 70%

---

## Cycle 9.3 – AR Invoice & Payment

### Objectives

- Bangun AR invoice dari delivery order atau sales order
- Payment allocation & matching
- AR aging report (30/60/90/90+ days)
- Integration dengan accounting (auto journal entries)
- Customer statement PDF

### Database Schema (`000013_phase9_ar_invoice_payment.up.sql`)

```sql
CREATE TYPE ar_invoice_status AS ENUM ('DRAFT','ISSUED','PARTIALLY_PAID','PAID','OVERDUE','CANCELLED');
CREATE TYPE ar_payment_status AS ENUM ('DRAFT','POSTED','CANCELLED');

CREATE TABLE ar_invoices (
    id BIGSERIAL PRIMARY KEY,
    doc_number TEXT NOT NULL UNIQUE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    sales_order_id BIGINT REFERENCES sales_orders(id) ON DELETE SET NULL,
    delivery_order_id BIGINT REFERENCES delivery_orders(id) ON DELETE SET NULL,
    invoice_date DATE NOT NULL,
    due_date DATE NOT NULL,
    status ar_invoice_status NOT NULL DEFAULT 'DRAFT',
    subtotal NUMERIC(18,2) NOT NULL DEFAULT 0,
    tax_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
    total_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
    paid_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
    outstanding_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
    currency_code TEXT NOT NULL DEFAULT 'IDR',
    payment_terms TEXT,
    notes TEXT,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    posted_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    posted_at TIMESTAMPTZ,
    journal_entry_id BIGINT REFERENCES journal_entries(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_ar_invoice_dates CHECK (invoice_date <= due_date),
    CONSTRAINT chk_ar_amounts CHECK (paid_amount <= total_amount AND outstanding_amount >= 0)
);

CREATE TABLE ar_invoice_lines (
    id BIGSERIAL PRIMARY KEY,
    ar_invoice_id BIGINT NOT NULL REFERENCES ar_invoices(id) ON DELETE CASCADE,
    product_id BIGINT REFERENCES products(id) ON DELETE SET NULL,
    description TEXT NOT NULL,
    quantity NUMERIC(14,4) NOT NULL DEFAULT 1,
    unit_price NUMERIC(18,2) NOT NULL,
    discount_percent NUMERIC(5,2) NOT NULL DEFAULT 0,
    tax_percent NUMERIC(5,2) NOT NULL DEFAULT 0,
    line_total NUMERIC(18,2) NOT NULL,
    gl_account_code TEXT,
    line_order INT NOT NULL DEFAULT 0
);

CREATE TABLE ar_payments (
    id BIGSERIAL PRIMARY KEY,
    doc_number TEXT NOT NULL UNIQUE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    payment_date DATE NOT NULL,
    amount NUMERIC(18,2) NOT NULL CHECK (amount > 0),
    payment_method TEXT NOT NULL,
    reference_number TEXT,
    bank_account TEXT,
    status ar_payment_status NOT NULL DEFAULT 'DRAFT',
    notes TEXT,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    posted_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    posted_at TIMESTAMPTZ,
    journal_entry_id BIGINT REFERENCES journal_entries(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE ar_payment_allocations (
    id BIGSERIAL PRIMARY KEY,
    ar_payment_id BIGINT NOT NULL REFERENCES ar_payments(id) ON DELETE CASCADE,
    ar_invoice_id BIGINT NOT NULL REFERENCES ar_invoices(id) ON DELETE CASCADE,
    allocated_amount NUMERIC(18,2) NOT NULL CHECK (allocated_amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ar_invoices_company_status ON ar_invoices(company_id, status);
CREATE INDEX idx_ar_invoices_customer ON ar_invoices(customer_id);
CREATE INDEX idx_ar_invoices_due_date ON ar_invoices(due_date);
CREATE INDEX idx_ar_invoices_so ON ar_invoices(sales_order_id);
CREATE INDEX idx_ar_invoices_do ON ar_invoices(delivery_order_id);
CREATE INDEX idx_ar_invoice_lines_invoice ON ar_invoice_lines(ar_invoice_id);

CREATE INDEX idx_ar_payments_company_status ON ar_payments(company_id, status);
CREATE INDEX idx_ar_payments_customer ON ar_payments(customer_id);
CREATE INDEX idx_ar_payments_date ON ar_payments(payment_date);
CREATE INDEX idx_ar_payment_allocations_payment ON ar_payment_allocations(ar_payment_id);
CREATE INDEX idx_ar_payment_allocations_invoice ON ar_payment_allocations(ar_invoice_id);

-- Materialized view untuk AR aging
CREATE MATERIALIZED VIEW mv_ar_aging AS
SELECT
    inv.company_id,
    inv.customer_id,
    cust.name AS customer_name,
    inv.id AS invoice_id,
    inv.doc_number,
    inv.invoice_date,
    inv.due_date,
    inv.total_amount,
    inv.outstanding_amount,
    CURRENT_DATE - inv.due_date AS days_overdue,
    CASE
        WHEN inv.status = 'PAID' THEN 'paid'
        WHEN CURRENT_DATE <= inv.due_date THEN 'current'
        WHEN CURRENT_DATE - inv.due_date BETWEEN 1 AND 30 THEN '1-30'
        WHEN CURRENT_DATE - inv.due_date BETWEEN 31 AND 60 THEN '31-60'
        WHEN CURRENT_DATE - inv.due_date BETWEEN 61 AND 90 THEN '61-90'
        ELSE '90+'
    END AS aging_bucket
FROM ar_invoices inv
JOIN customers cust ON inv.customer_id = cust.id
WHERE inv.status IN ('ISSUED','PARTIALLY_PAID','OVERDUE');

CREATE INDEX idx_mv_ar_aging_company ON mv_ar_aging(company_id);
CREATE INDEX idx_mv_ar_aging_customer ON mv_ar_aging(customer_id);
CREATE INDEX idx_mv_ar_aging_bucket ON mv_ar_aging(aging_bucket);
```

### Module Structure (`internal/ar/`)

```
internal/ar/
├── domain.go          // ARInvoice, ARPayment, ARPaymentAllocation
├── repository.go      // CRUD + aging queries
├── service.go         // invoice creation, payment posting, allocation
├── job.go             // async invoice posting, journal creation
├── http/
│   └── handler.go     // SSR handlers + PDF/CSV exports
├── aging_report.go    // AR aging report logic
└── service_test.go
```

### Key Business Rules

1. **Invoice Creation**:
   - Can be created from DO (delivered items) or SO directly
   - Auto-populate lines from DO/SO
   - Status starts as DRAFT
   - User can edit before issuing

2. **Invoice Posting**:
   - DRAFT → ISSUED
   - Create journal entry:
     - DR: Accounts Receivable (customer control account)
     - CR: Revenue (per line item GL account or default revenue account)
     - CR: Tax Payable (if applicable)
   - Cannot edit after posting

3. **Payment Recording**:
   - Create AR Payment record
   - Allocate to one or multiple invoices
   - Update invoice `paid_amount` and `outstanding_amount`
   - Auto-update invoice status (PARTIALLY_PAID / PAID)
   - Create journal entry:
     - DR: Cash/Bank account
     - CR: Accounts Receivable

4. **AR Aging**:
   - Materialized view refresh daily via cron job
   - Buckets: Current, 1-30, 31-60, 61-90, 90+ days
   - Export to PDF/CSV

5. **Overdue Detection**:
   - Daily job scan invoices with due_date < today
   - Auto-update status to OVERDUE
   - Optional: send email reminders (future enhancement)

### Background Jobs

- **`ar:post-invoice`** – Async invoice posting + journal creation
- **`ar:post-payment`** – Async payment posting + journal creation
- **`ar:refresh-aging`** – Daily refresh of mv_ar_aging (cron)
- **`ar:check-overdue`** – Daily check for overdue invoices (cron)

### RBAC Permissions

```
finance.ar.invoice.view
finance.ar.invoice.create
finance.ar.invoice.edit
finance.ar.invoice.post
finance.ar.invoice.cancel
finance.ar.invoice.download_pdf

finance.ar.payment.view
finance.ar.payment.create
finance.ar.payment.post
finance.ar.payment.cancel

finance.ar.aging.view
finance.ar.aging.export
```

### SSR UI Pages

1. **`/finance/ar/invoices`** – List invoices dengan filter
2. **`/finance/ar/invoices/new`** – Create invoice (manual atau from DO/SO)
3. **`/finance/ar/invoices/{id}`** – Detail invoice dengan payment history
4. **`/finance/ar/invoices/{id}/edit`** – Edit (DRAFT only)
5. **`/finance/ar/payments`** – List payments
6. **`/finance/ar/payments/new`** – Record payment dengan allocation UI
7. **`/finance/ar/payments/{id}`** – Payment detail
8. **`/finance/ar/aging`** – AR aging report dengan pivot table (SSR)
9. **`/finance/ar/customer-statement/{customer_id}`** – Customer statement PDF

### Accounting Integration

**GL Accounts Required** (add to default chart of accounts):
- `1200` – Accounts Receivable (asset)
- `4000` – Sales Revenue (revenue)
- `4100` – Sales Discounts (contra-revenue)
- `2100` – Sales Tax Payable (liability)
- `1100` – Cash/Bank (asset)

**Auto Journal Templates**:

*Invoice Posting*:
```
DR Accounts Receivable       $total_amount
  CR Sales Revenue             $subtotal
  CR Sales Tax Payable         $tax_amount
```

*Payment Posting*:
```
DR Cash/Bank                  $payment_amount
  CR Accounts Receivable       $payment_amount
```

### Testing Strategy

- **Unit tests**: invoice creation, payment allocation logic, aging calculation
- **Integration tests**: journal entry creation, status updates
- **Job tests**: async posting with rollback on error
- **Report tests**: aging view accuracy, PDF generation
- **E2E scenario**: create invoice → post → record payment → verify journal & status

### Documentation

- `docs/howto-ar-invoice.md` – user guide untuk invoice & payment
- `docs/howto-ar-aging.md` – aging report guide
- `docs/runbook-ar.md` – operational runbook (jobs, aging refresh)
- `docs/accounting-integration-ar.md` – journal entry mapping
- Update CHANGELOG

### Acceptance Criteria

- ✅ Invoice dapat dibuat dari DO/SO atau manual
- ✅ Invoice posting creates proper journal entries
- ✅ Payment allocation & matching bekerja
- ✅ AR aging report akurat dengan buckets
- ✅ Auto overdue detection berjalan via cron
- ✅ PDF invoice dan customer statement dapat diunduh
- ✅ RBAC enforced
- ✅ Tests coverage ≥ 70%
- ✅ Integration dengan accounting module verified

---

## Cross-Cutting Concerns

### Security

- **RBAC**: semua endpoints harus enforce permissions
- **CSRF**: semua form POST/PUT/DELETE harus pakai CSRF token
- **Audit Trail**: log semua create/update/delete operations ke `audit_logs`
- **Rate Limiting**: apply rate limits pada export endpoints (PDF/CSV)

### Performance

- **Pagination**: semua list endpoints harus support pagination (default 50 items)
- **Indexing**: pastikan semua foreign keys dan filter columns punya index
- **Caching**: consider caching untuk customer/product lookups
- **Async Jobs**: operasi berat (posting, PDF generation) harus async via Asynq

### Observability

- **Metrics**: tambahkan Prometheus metrics untuk:
  - `odyssey_sales_orders_total{status}` – counter per status
  - `odyssey_ar_invoices_total{status}` – counter per status
  - `odyssey_ar_payments_posted_total` – payment counter
  - `odyssey_ar_overdue_amount` – gauge untuk total overdue
- **Logging**: structured logs dengan correlation IDs
- **Tracing**: optional distributed tracing untuk complex flows

### Data Quality

- **Validation**: strict validation di service layer (amounts, dates, references)
- **Idempotency**: posting operations harus idempotent (prevent double-posting)
- **Consistency**: enforce referential integrity dengan FK constraints
- **Audit**: semua financial transactions harus immutable setelah posting

---

## Dependencies & Integration Points

### With Existing Modules

| Module | Integration Point | Type |
|--------|-------------------|------|
| **Inventory** | Stock reduction via inventory_tx | Write |
| **Accounting** | Journal entry creation for invoices & payments | Write |
| **Auth/RBAC** | Permission checks | Read |
| **Master Data** | Customer, product lookups | Read |
| **Jobs** | Asynq background jobs | Write |
| **Audit** | Audit log entries | Write |

### External Dependencies

- **Gotenberg** – PDF generation (reuse existing setup)
- **Redis** – Session, rate limiting, job queue
- **PostgreSQL** – Primary data store

---

## Migration Strategy

### Database Migrations

- `000011_phase9_sales_quotation_so.up.sql` – Cycle 9.1
- `000012_phase9_delivery.up.sql` – Cycle 9.2
- `000013_phase9_ar_invoice_payment.up.sql` – Cycle 9.3

### Rollback Plan

- Each migration must have corresponding `.down.sql`
- Test rollback in staging before production
- Document rollback steps in runbook

### Data Seeding

Create seed data script (`scripts/seed-phase9.go`):
- Sample customers (if not exist)
- Sample products (if not exist)
- RBAC permissions for sales & AR
- Default GL accounts for AR
- Sample quotations & orders for testing

---

## Testing Strategy (Phase-Level)

### Unit Tests

- **Coverage target**: ≥ 70% per cycle
- **Focus areas**: business logic, calculations, validations
- **Tools**: `testing`, `testify`, `sqlmock` untuk repo tests

### Integration Tests

- **Database**: use testcontainers or Docker Compose PostgreSQL
- **Scenarios**: end-to-end flows (quotation → SO → DO → invoice → payment)
- **Fixtures**: reusable test data builders

### E2E Tests

- **HTTP tests**: `httpexpect` untuk test handlers
- **RBAC tests**: verify 403 for unauthorized users
- **Validation tests**: verify 422 for invalid inputs

### Performance Tests

- **Load testing**: simulate 100 concurrent users creating orders
- **Aging report**: verify query performance with 10K+ invoices
- **PDF generation**: ensure <5s generation time

### Manual QA Checklist

- [ ] Create quotation → approve → convert to SO → confirm
- [ ] Create DO from SO → confirm → verify stock reduction
- [ ] Create invoice from DO → post → verify journal entry
- [ ] Record payment → allocate to invoice → verify status update
- [ ] View AR aging report → export to PDF & CSV
- [ ] Verify RBAC enforcement on all pages
- [ ] Test partial delivery and partial payment scenarios
- [ ] Verify audit log entries for all operations

---

## Documentation Deliverables

### Technical Docs

- [ ] `docs/PLAN-Phase9-Sales.md` – this document ✅
- [ ] `docs/architecture-sales-ar.md` – architecture decisions
- [ ] `docs/database-schema-phase9.md` – ER diagram & schema docs
- [ ] `docs/api-sales-ar.md` – API/handler documentation

### User Guides

- [ ] `docs/howto-sales-quotation.md`
- [ ] `docs/howto-sales-delivery.md`
- [ ] `docs/howto-ar-invoice.md`
- [ ] `docs/howto-ar-aging.md`

### Operations

- [ ] `docs/runbook-sales.md` – operations runbook
- [ ] `docs/runbook-ar.md` – AR operations runbook
- [ ] `docs/troubleshooting-phase9.md` – common issues & solutions

### Testing

- [ ] `docs/TESTING-PHASE9-C1.md` – Cycle 9.1 test plan
- [ ] `docs/TESTING-PHASE9-C2.md` – Cycle 9.2 test plan
- [ ] `docs/TESTING-PHASE9-C3.md` – Cycle 9.3 test plan

---

## Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| **Stock sync issues** | High | Medium | Use DB transactions, add retry logic, implement reconciliation job |
| **Journal entry errors** | High | Low | Extensive testing, idempotency checks, audit trail |
| **Performance degradation** | Medium | Medium | Add indexes, use materialized views, implement caching |
| **Complex partial scenarios** | Medium | Medium | Start with simple scenarios, iterate based on user feedback |
| **RBAC complexity** | Low | Low | Reuse existing RBAC patterns, comprehensive testing |
| **PDF generation timeout** | Medium | Low | Reuse Gotenberg setup from Phase 8, add retry logic |

---

## Success Metrics

### Technical Metrics

- ✅ All migrations run successfully
- ✅ Test coverage ≥ 70% per cycle
- ✅ Zero critical bugs in production
- ✅ API response time <500ms (p95)
- ✅ PDF generation <5s

### Business Metrics

- ✅ Sales team can create and manage quotations without manual tracking
- ✅ Delivery team can process DO and update stock in real-time
- ✅ Finance team can track AR aging and payments accurately
- ✅ Management can view sales pipeline and revenue reports
- ✅ Reduction in manual reconciliation time by 80%

---

## Timeline Estimate

| Cycle | Duration | Key Milestones |
|-------|----------|----------------|
| **9.1 – Quotation & SO** | 5-6 days | Day 1-2: Schema + domain; Day 3-4: Service + UI; Day 5-6: Tests + docs |
| **9.2 – Delivery** | 4-5 days | Day 1-2: Schema + inventory integration; Day 3-4: UI + PDF; Day 5: Tests |
| **9.3 – AR Invoice & Payment** | 6-7 days | Day 1-2: Schema + domain; Day 3-4: Accounting integration; Day 5-6: Aging + UI; Day 7: Tests |
| **Integration & Hardening** | 2-3 days | E2E testing, documentation finalization, staging deployment |
| **Total** | **17-21 days** | Approximately 3.5-4 weeks with buffer |

---

## Phase 9 Exit Criteria

Before declaring Phase 9 complete, verify:

- ✅ All 3 cycles completed and merged to `main`
- ✅ All migrations applied to staging and production
- ✅ All tests passing (unit, integration, E2E)
- ✅ All documentation updated
- ✅ RBAC permissions seeded
- ✅ User acceptance testing (UAT) completed
- ✅ Performance benchmarks met
- ✅ Security checklist completed
- ✅ Operations runbooks reviewed by ops team
- ✅ Production deployment plan approved

---

## Post-Phase 9 Enhancements (Future)

Ideas for Phase 10 or future iterations:

1. **Sales Analytics Dashboard**
   - Revenue trends, top customers, product performance
   - Sales funnel conversion rates (quotation → order → delivery)

2. **Credit Management**
   - Customer credit limits
   - Auto-hold orders if credit limit exceeded
   - Credit approval workflow

3. **Advanced AR Features**
   - Auto-dunning (automated reminder emails)
   - Dispute management
   - Write-offs & bad debt provisioning
   - Installment payments

4. **Multi-Currency Support**
   - Foreign currency invoices
   - Exchange rate management
   - Multi-currency AR aging

5. **Sales Commissions**
   - Commission calculation based on sales
   - Commission reports for sales team

6. **Returns & Credit Notes**
   - Return authorization (RA)
   - Credit note processing
   - Stock returns to inventory

7. **Mobile App**
   - Mobile delivery app for drivers
   - Mobile payment collection

8. **API Integration**
   - REST API for third-party integrations
   - Webhook notifications for order status changes

---

## Coordination & Communication

### Daily Standups

- **What**: Quick sync (15 min)
- **When**: Every morning
- **Focus**: Progress, blockers, dependencies

### Code Reviews

- **Requirement**: All PRs must be reviewed by at least 1 team member
- **Criteria**: Tests passing, documentation updated, follows coding standards

### Documentation Updates

- **Requirement**: Update docs in the same PR as code changes
- **Review**: Tech lead reviews documentation accuracy

### Stakeholder Demos

- **When**: End of each cycle
- **Audience**: Product owner, sales team, finance team
- **Format**: Live demo + Q&A

---

## Appendix

### A. Sample Workflows

**Workflow 1: Standard Sales Process**
```
1. Sales creates Quotation (DRAFT)
2. Sales submits for approval (SUBMITTED)
3. Manager approves (APPROVED)
4. Sales converts to Sales Order (SO in DRAFT)
5. Sales confirms SO (CONFIRMED)
6. Warehouse creates Delivery Order from SO
7. Warehouse confirms DO → stock reduced (DELIVERED)
8. Finance creates Invoice from DO
9. Finance posts Invoice → journal entry created (ISSUED)
10. Customer pays → Finance records Payment
11. Finance allocates Payment to Invoice (PAID)
```

**Workflow 2: Partial Delivery & Payment**
```
1. SO for 100 units confirmed
2. DO1 for 60 units confirmed → SO status = PROCESSING
3. DO2 for 40 units confirmed → SO status = COMPLETED
4. Invoice for 100 units issued
5. Payment1 for 60% received → Invoice status = PARTIALLY_PAID
6. Payment2 for 40% received → Invoice status = PAID
```

### B. GL Account Mapping Reference

| Transaction | Debit | Credit |
|-------------|-------|--------|
| **Invoice Posting** | 1200 (AR) | 4000 (Revenue), 2100 (Tax) |
| **Payment Posting** | 1100 (Cash) | 1200 (AR) |
| **Sales Return** (future) | 4200 (Sales Returns) | 1200 (AR) |
| **Write-off** (future) | 6100 (Bad Debt Expense) | 1200 (AR) |

### C. Permission Matrix

| Role | Quotation | SO | Delivery | Invoice | Payment | Aging |
|------|-----------|----|---------|---------|---------| ------|
| **Sales Staff** | CRUD (own) | CRUD (own) | View | - | - | - |
| **Sales Manager** | CRUD (all) + Approve | CRUD (all) + Confirm | View | - | - | - |
| **Warehouse Staff** | View | View | CRUD | - | - | - |
| **AR Clerk** | View | View | View | CRUD | CRUD | View |
| **Finance Manager** | View | View | View | All | All | Export |
| **Admin** | All | All | All | All | All | All |

### D. Useful SQL Queries

**Outstanding AR by Customer**:
```sql
SELECT
    c.name,
    SUM(inv.outstanding_amount) AS total_outstanding
FROM ar_invoices inv
JOIN customers c ON inv.customer_id = c.id
WHERE inv.status IN ('ISSUED','PARTIALLY_PAID','OVERDUE')
GROUP BY c.id, c.name
ORDER BY total_outstanding DESC;
```

**Sales Performance by Product**:
```sql
SELECT
    p.name,
    SUM(sol.quantity) AS total_qty_sold,
    SUM(sol.line_total) AS total_revenue
FROM sales_order_lines sol
JOIN products p ON sol.product_id = p.id
JOIN sales_orders so ON sol.sales_order_id = so.id
WHERE so.status = 'COMPLETED'
    AND so.order_date >= CURRENT_DATE - INTERVAL '30 days'
GROUP BY p.id, p.name
ORDER BY total_revenue DESC;
```

---

## Conclusion

Phase 9 melengkapi siklus bisnis Odyssey ERP dengan modul Sales & AR yang robust. Dengan fokus pada quotation management, delivery tracking, invoicing, dan AR aging, sistem akan memberikan visibility penuh atas pipeline penjualan dan kesehatan piutang. Integrasi seamless dengan inventory dan accounting memastikan data consistency dan real-time reporting.

**Next Steps**:
1. Review & approve plan ini dengan stakeholders
2. Kick-off Cycle 9.1 dengan setup meeting
3. Create GitHub project board untuk tracking
4. Begin implementation 🚀

---

**Document Version**: 1.0  
**Created**: 2025-01-16  
**Author**: Technical Lead  
**Status**: Draft - Awaiting Approval