# Security Checklist – Phase 9 (Sales & AR)

## Overview

Dokumen ini mencakup security controls yang harus diverifikasi sebelum Phase 9 dinyatakan production-ready. Fokus pada RBAC, data validation, audit trail, dan financial transaction security.

---

## 1. Authentication & Authorization

### RBAC Permissions

- [ ] **Quotation Permissions** – verify granular permissions:
  - `sales.quotation.view` – list & read access
  - `sales.quotation.create` – create new quotations
  - `sales.quotation.edit` – edit DRAFT quotations only
  - `sales.quotation.approve` – approve/reject submitted quotations
  - `sales.quotation.delete` – soft delete (audit trail maintained)

- [ ] **Sales Order Permissions**:
  - `sales.order.view`
  - `sales.order.create`
  - `sales.order.edit` – DRAFT only
  - `sales.order.confirm` – confirm orders
  - `sales.order.cancel` – cancel orders with reason
  - `sales.order.delete`

- [ ] **Delivery Permissions**:
  - `sales.delivery.view`
  - `sales.delivery.create`
  - `sales.delivery.confirm` – confirm DO & reduce stock
  - `sales.delivery.cancel`
  - `sales.delivery.download_pdf` – packing list PDF

- [ ] **AR Invoice Permissions**:
  - `finance.ar.invoice.view`
  - `finance.ar.invoice.create`
  - `finance.ar.invoice.edit` – DRAFT only
  - `finance.ar.invoice.post` – post invoice & create journal
  - `finance.ar.invoice.cancel` – cancel with audit
  - `finance.ar.invoice.download_pdf`

- [ ] **AR Payment Permissions**:
  - `finance.ar.payment.view`
  - `finance.ar.payment.create`
  - `finance.ar.payment.post` – post payment & allocate
  - `finance.ar.payment.cancel`

- [ ] **AR Reporting Permissions**:
  - `finance.ar.aging.view` – view aging report
  - `finance.ar.aging.export` – export to PDF/CSV

### Authorization Enforcement

- [ ] **Handler-Level Checks** – every HTTP handler enforces permission via middleware
- [ ] **Service-Level Checks** – service layer validates user permissions before business logic
- [ ] **Row-Level Security** – users can only access data for their assigned companies/branches (if multi-tenant)
- [ ] **403 Forbidden** – proper error responses for unauthorized access
- [ ] **Audit Unauthorized Attempts** – log 403 responses to `audit_logs`

---

## 2. Input Validation & Sanitization

### Form Validation

- [ ] **Quotation Form**:
  - Customer ID exists & is active
  - Quote date <= valid_until date
  - Line items: quantity > 0, unit_price >= 0, discount 0-100%
  - Total calculations match line item sums

- [ ] **Sales Order Form**:
  - All validations from quotation
  - Delivery date >= order date (if provided)
  - Cannot confirm without at least one line item

- [ ] **Delivery Order Form**:
  - Sales order must be in CONFIRMED or PROCESSING status
  - Quantity to deliver <= remaining quantity on SO line
  - Warehouse has sufficient stock (validate before confirm)
  - Driver name & vehicle number sanitized (no SQL injection)

- [ ] **AR Invoice Form**:
  - Invoice date <= due date
  - Amount calculations correct (subtotal + tax = total)
  - GL account codes valid (if manual entry allowed)
  - Cannot post invoice with zero amount

- [ ] **AR Payment Form**:
  - Payment date <= current date (no future payments)
  - Amount > 0
  - Payment method from whitelist (CASH, BANK_TRANSFER, CHECK, etc)
  - Reference number sanitized
  - Total allocations <= payment amount

### SQL Injection Prevention

- [ ] **Parameterized Queries** – all DB queries use `$1, $2` placeholders (sqlc/pgx)
- [ ] **No String Concatenation** – never build SQL via string concat
- [ ] **Whitelist Filters** – filter/sort columns validated against whitelist

### XSS Prevention

- [ ] **HTML Escaping** – all user input escaped in templates (`{{.Field}}` not `{{.Field | raw}}`)
- [ ] **Rich Text** – if notes/comments allow formatting, use sanitizer (bluemonday)
- [ ] **PDF Generation** – user input sanitized before passing to Gotenberg

---

## 3. CSRF Protection

- [ ] **CSRF Tokens** – all POST/PUT/DELETE forms include CSRF token
- [ ] **Token Verification** – middleware verifies token on submit
- [ ] **Token Rotation** – tokens rotated after critical actions (posting invoices/payments)
- [ ] **AJAX Requests** – CSRF token in custom header (`X-CSRF-Token`)

---

## 4. Data Integrity & Consistency

### Transaction Integrity

- [ ] **Database Transactions** – all multi-table operations wrapped in DB transactions
- [ ] **Rollback on Error** – errors trigger rollback to prevent partial updates
- [ ] **Idempotency** – posting operations idempotent (prevent double-posting)
  - Use `idempotency_key` or check existing `journal_entry_id`
  - Return success if already processed

### Financial Controls

- [ ] **Immutability** – posted invoices/payments cannot be edited (only cancel & recreate)
- [ ] **Audit Trail** – all financial transactions logged to `audit_logs`
- [ ] **Journal Entry Linking** – every invoice/payment links to journal entry
- [ ] **Balance Validation** – journal entry debits = credits (enforced in service layer)
- [ ] **Amount Constraints** – DB constraints on amounts (paid_amount <= total_amount, etc)

### Stock Consistency

- [ ] **Stock Validation** – delivery confirm validates available stock before reduction
- [ ] **Negative Stock Prevention** – DB constraint or service check prevents negative stock
- [ ] **Inventory Transaction** – every stock movement creates `inventory_tx` record
- [ ] **Reconciliation** – periodic job reconciles stock balances vs transactions

---

## 5. Access Control & Segregation of Duties

### Separation of Concerns

- [ ] **Sales vs Finance** – sales team cannot post invoices/payments (finance-only)
- [ ] **Warehouse vs Sales** – warehouse cannot edit sales orders (read-only)
- [ ] **Create vs Approve** – users cannot approve their own quotations
- [ ] **Post vs Cancel** – separate permissions for posting and canceling

### Data Isolation

- [ ] **Company-Level Filtering** – queries filter by `company_id` based on user's assigned company
- [ ] **Customer Access** – users can only view customers assigned to their branch (if applicable)
- [ ] **Document Ownership** – users can only edit their own drafts (unless manager)

---

## 6. Audit Logging

### Required Audit Events

- [ ] **Quotation Events**:
  - Create, update, submit, approve, reject, convert to SO, delete

- [ ] **Sales Order Events**:
  - Create, update, confirm, cancel, delete

- [ ] **Delivery Order Events**:
  - Create, update, confirm, cancel, stock reduction

- [ ] **Invoice Events**:
  - Create, update, post, cancel, payment allocation

- [ ] **Payment Events**:
  - Create, post, allocate, cancel

### Audit Log Fields

- [ ] **actor_id** – user who performed action
- [ ] **action** – CREATE, UPDATE, DELETE, APPROVE, POST, etc
- [ ] **entity** – quotation, sales_order, delivery_order, ar_invoice, ar_payment
- [ ] **entity_id** – ID of the record
- [ ] **meta** – JSON with old/new values, reason for cancel, etc
- [ ] **timestamp** – when action occurred
- [ ] **ip_address** – client IP (optional)

### Audit Queries

- [ ] **Who Posted Invoice** – query audit logs by entity=ar_invoice, action=POST
- [ ] **Payment History** – trace all allocations for an invoice
- [ ] **Cancelled Orders** – query cancelled orders with reasons
- [ ] **Approval Chain** – who approved quotations that became orders

---

## 7. Rate Limiting

- [ ] **Export Endpoints** – PDF/CSV exports limited to 10 req/min per user
- [ ] **Posting Endpoints** – invoice/payment posting limited to 30 req/min per user
- [ ] **List Endpoints** – list queries limited to 100 req/min per user
- [ ] **429 Responses** – proper error message & retry-after header

---

## 8. Secure Communication

- [ ] **HTTPS Only** – enforce HTTPS in production (`secure` flag on cookies)
- [ ] **HTTP Security Headers**:
  - `Strict-Transport-Security: max-age=31536000`
  - `X-Content-Type-Options: nosniff`
  - `X-Frame-Options: DENY`
  - `Content-Security-Policy: default-src 'self'`
- [ ] **Session Cookies** – `HttpOnly`, `Secure`, `SameSite=Lax` flags

---

## 9. Secrets Management

- [ ] **No Hardcoded Secrets** – no API keys, DB passwords in code
- [ ] **Environment Variables** – secrets loaded from env vars or secret manager
- [ ] **Gotenberg URL** – Gotenberg endpoint URL in config (not hardcoded)
- [ ] **Database URL** – DB connection string in env (not in repo)
- [ ] **Redis URL** – Redis connection string in env

---

## 10. Error Handling & Information Disclosure

- [ ] **Generic Error Messages** – no stack traces or DB errors exposed to users
- [ ] **Detailed Logging** – errors logged with full context (but not shown to user)
- [ ] **404 vs 403** – distinguish between "not found" and "forbidden"
- [ ] **Validation Errors** – specific validation errors OK (e.g., "quantity must be > 0")
- [ ] **SQL Errors** – DB errors logged but user sees "internal server error"

---

## 11. File Upload & Download Security

### PDF Generation

- [ ] **Gotenberg Timeout** – set timeout (10s) to prevent DoS
- [ ] **Size Limits** – reject PDFs > 10MB (sanity check)
- [ ] **Virus Scanning** – optional: scan generated PDFs (ClamAV integration)
- [ ] **User Input Sanitization** – escape HTML in user-provided text before PDF generation

### File Downloads

- [ ] **Authorization** – verify user has permission before serving file
- [ ] **Path Traversal Prevention** – validate file path (no `../` attacks)
- [ ] **Content-Disposition** – set `attachment; filename="safe-name.pdf"`
- [ ] **Content-Type** – set correct MIME type (`application/pdf`)

---

## 12. Background Jobs Security

### Job Queue

- [ ] **Job Payload Validation** – validate job params before processing
- [ ] **Retry Limits** – max 3 retries with exponential backoff
- [ ] **Timeout** – job timeout to prevent runaway jobs
- [ ] **Dead Letter Queue** – failed jobs moved to DLQ for investigation
- [ ] **Job Context** – include user_id, company_id in job payload for audit

### Sensitive Data in Jobs

- [ ] **No Credentials in Payload** – never pass passwords/API keys in job data
- [ ] **Encrypted Payloads** – encrypt sensitive job data if stored in Redis
- [ ] **Short TTL** – job payloads expire after 24h

---

## 13. Multi-Tenancy (if applicable)

- [ ] **Company ID Filter** – all queries filter by user's company
- [ ] **Shared Resources** – customers/products scoped to company
- [ ] **Data Leakage Prevention** – user cannot view other company's data
- [ ] **Tenant Isolation** – physical or logical DB separation (if required)

---

## 14. Dependency Vulnerabilities

- [ ] **Go Modules** – run `go mod tidy` and `govulncheck`
- [ ] **Gotenberg** – use latest stable version (v8.x)
- [ ] **PostgreSQL** – keep PG updated (latest minor version)
- [ ] **Redis** – use Redis 7.x with security patches
- [ ] **Dependabot** – enable Dependabot alerts on GitHub

---

## 15. Penetration Testing Scenarios

### Manual Tests

- [ ] **Privilege Escalation** – attempt to approve own quotation (should fail)
- [ ] **IDOR** – try accessing other user's invoices by ID (should 403)
- [ ] **SQL Injection** – inject SQL in filter fields (should be parameterized)
- [ ] **XSS** – inject `<script>` in notes field (should be escaped)
- [ ] **CSRF** – submit form without CSRF token (should fail)
- [ ] **Rate Limit Bypass** – exceed rate limits (should 429)
- [ ] **Path Traversal** – download file with `../../etc/passwd` path (should fail)

### Automated Scans

- [ ] **OWASP ZAP** – run automated security scan
- [ ] **Burp Suite** – manual penetration testing
- [ ] **SQLMap** – test for SQL injection vulnerabilities

---

## 16. Compliance & Privacy

### Data Privacy

- [ ] **Customer Data** – PII (name, email, phone) handled per GDPR/local laws
- [ ] **Data Retention** – policy for archiving old invoices/payments
- [ ] **Right to Deletion** – mechanism to anonymize customer data on request
- [ ] **Data Export** – customer can export their invoice history

### Financial Compliance

- [ ] **Audit Trail** – immutable audit logs for regulators
- [ ] **Tax Compliance** – tax calculations accurate per jurisdiction
- [ ] **Invoice Numbering** – sequential, no gaps (for tax authority)
- [ ] **Journal Entry Integrity** – debits = credits enforced

---

## 17. Monitoring & Alerting

### Security Metrics

- [ ] **Failed Login Attempts** – alert on >10 failed logins in 5 min
- [ ] **403 Spike** – alert on sudden increase in 403 responses
- [ ] **Suspicious Activity** – alert on unusual patterns (e.g., 100 invoices posted in 1 min)
- [ ] **Job Failures** – alert on job failure rate >10%

### Audit Dashboards

- [ ] **Grafana Dashboard** – panel for audit events by action type
- [ ] **Alert Rules** – Prometheus alerts for security events

---

## 18. Production Deployment Checklist

### Pre-Deployment

- [ ] **Security Review** – this checklist completed
- [ ] **Pen Test Results** – no critical/high vulnerabilities
- [ ] **Code Review** – all PRs reviewed by at least 2 developers
- [ ] **Secrets Rotated** – rotate DB passwords, API keys before go-live

### Deployment

- [ ] **HTTPS Enforced** – redirect HTTP to HTTPS
- [ ] **Firewall Rules** – restrict DB/Redis access to app servers only
- [ ] **Backup Strategy** – automated daily backups with encryption
- [ ] **Rollback Plan** – tested rollback procedure

### Post-Deployment

- [ ] **Security Monitoring** – SIEM/log aggregation active
- [ ] **Incident Response Plan** – documented procedure for security incidents
- [ ] **Bug Bounty** – consider bug bounty program (if public-facing)

---

## 19. Known Limitations & Future Work

### Phase 9.1 Limitations

- Approval workflow is single-level (no multi-step approval)
- Stock reservation is soft check (no hard lock)

### Phase 9.2 Limitations

- Delivery tracking is basic (no GPS/real-time tracking)
- Packing list PDF is simple template (no custom branding)

### Phase 9.3 Limitations

- Payment allocation is manual (no auto-matching)
- No dunning/reminder emails (manual follow-up)
- No credit limit enforcement (future enhancement)

### Future Security Enhancements

- [ ] **2FA** – two-factor authentication for finance users
- [ ] **IP Whitelisting** – restrict admin access to office IPs
- [ ] **E-signature** – digital signatures for invoices (legal compliance)
- [ ] **Blockchain Audit Trail** – immutable audit log on blockchain (optional)

---

## Sign-Off

| Role | Name | Signature | Date |
|------|------|-----------|------|
| **Tech Lead** | | | |
| **Security Engineer** | | | |
| **Finance Manager** | | | |
| **QA Lead** | | | |

---

**Document Version**: 1.0  
**Last Updated**: 2025-01-16  
**Next Review**: Before Phase 9 Production Deployment