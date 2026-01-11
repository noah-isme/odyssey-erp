# Odyssey ERP — Future Roadmap & Recommendations

**Prepared:** 2026-01-11  
**Current Version:** v0.9.0

## Executive Summary

Odyssey ERP telah menyelesaikan Phase 9 (Sales & AR). Dokumen ini berisi rekomendasi fitur untuk pengembangan selanjutnya, diprioritaskan berdasarkan business value dan technical foundation.

---

## Phase 10: Accounts Payable (AP)

**Priority:** 🔴 High  
**Estimated Effort:** 3-4 weeks

### Features
| Feature | Description | Priority |
|---------|-------------|----------|
| AP Invoice | Create invoices from GRN/PO | High |
| AP Invoice Lines | Line items with product/service details | High |
| AP Payment | Record vendor payments | High |
| Payment Allocation | Allocate payments across invoices | Medium |
| AP Aging Report | Outstanding payables by vendor | High |
| Vendor Statement | Statement reconciliation | Medium |

### Technical Notes
- Mirror AR structure (`ap_invoices`, `ap_invoice_lines`, `ap_payment_allocations`)
- Link to `suppliers` and `purchase_orders`
- Add `finance.ap.*` permissions

---

## Phase 11: Bank & Cash Management

**Priority:** 🔴 High  
**Estimated Effort:** 2-3 weeks

### Features
| Feature | Description | Priority |
|---------|-------------|----------|
| Bank Accounts | Multiple bank account management | High |
| Bank Transactions | Record deposits, withdrawals, transfers | High |
| Bank Reconciliation | Match transactions with bank statement | High |
| Cash Flow Report | Actual cash flow from transactions | Medium |
| Auto Bank Feed | Import bank statements (CSV/OFX) | Low |

### Technical Notes
- New entity `bank_accounts` with `company_id`
- `bank_transactions` with type (deposit/withdrawal/transfer)
- Reconciliation status tracking

---

## Phase 12: Inventory Enhancements

**Priority:** 🟡 Medium  
**Estimated Effort:** 3-4 weeks

### Features
| Feature | Description | Priority |
|---------|-------------|----------|
| Stock Valuation | FIFO/LIFO/Average costing methods | High |
| Stock Reorder | Auto-generate PO when below min | Medium |
| Batch/Lot Tracking | Track items by batch number | Medium |
| Serial Number | Track individual items | Low |
| Stock Take | Physical inventory count | High |
| Stock Adjustment | Adjust stock with audit trail | High |

### Technical Notes
- Add `costing_method` to products
- `inventory_lots` for batch tracking
- `stock_takes` and `stock_take_lines`

---

## Phase 13: Fixed Assets

**Priority:** 🟡 Medium  
**Estimated Effort:** 2-3 weeks

### Features
| Feature | Description | Priority |
|---------|-------------|----------|
| Asset Register | List of company assets | High |
| Depreciation | Auto-calculate depreciation | High |
| Asset Categories | Group assets by type | Medium |
| Asset Disposal | Record sale/disposal | Medium |
| Asset Transfer | Transfer between branches | Low |

### Technical Notes
- `fixed_assets` with depreciation method, useful life
- Monthly depreciation job
- Journal entries for depreciation expense

---

## Phase 14: Multi-Currency Enhancement

**Priority:** 🟡 Medium  
**Estimated Effort:** 2 weeks

### Features
| Feature | Description | Priority |
|---------|-------------|----------|
| Auto FX Rate | Fetch daily rates from API | Medium |
| Realized Gain/Loss | Calculate on payment | High |
| Unrealized Gain/Loss | Revalue outstanding invoices | High |
| Currency Revaluation | Month-end revaluation job | High |

### Technical Notes
- Integrate with free FX API (exchangerate-api.com)
- Add `original_currency_amount` to AR/AP
- Create revaluation journal entries

---

## Phase 15: Reporting & Analytics

**Priority:** 🟡 Medium  
**Estimated Effort:** 3-4 weeks

### Features
| Feature | Description | Priority |
|---------|-------------|----------|
| Custom Reports | Build reports with drag-drop | Low |
| Dashboard Widgets | Customizable dashboard | Medium |
| Budget vs Actual | Compare to budget | High |
| Department Reporting | P&L by department/cost center | Medium |
| Export to Excel | Native Excel export | High |
| Scheduled Reports | Email reports on schedule | Medium |

### Technical Notes
- Consider Go templating or external BI tool
- `budgets` table with period, account, amount
- Add `department_id` to transactions

---

## Phase 16: Audit & Compliance

**Priority:** 🟢 Low  
**Estimated Effort:** 2 weeks

### Features
| Feature | Description | Priority |
|---------|-------------|----------|
| Audit Log Viewer | UI for viewing audit logs | High |
| Data Export | Export all data for audit | Medium |
| Document Attachment | Attach files to transactions | Medium |
| E-Signature | Digital approval signatures | Low |
| Change History | View change history per record | Medium |

---

## Phase 17: Integration

**Priority:** 🟢 Low  
**Estimated Effort:** Varies

### Potential Integrations
| Integration | Description | Effort |
|-------------|-------------|--------|
| E-Faktur | Indonesian tax invoice | 2 weeks |
| Payment Gateway | Online payment (Midtrans, etc) | 2 weeks |
| E-Commerce | Tokopedia, Shopee integration | 3 weeks |
| Shipping | JNE, J&T, SiCepat API | 2 weeks |
| WhatsApp | Invoice delivery via WA | 1 week |

---

## Quick Wins (Low Effort, High Impact)

These can be implemented in 1-2 days each:

1. **PDF Invoice Email** — Send invoice PDF via email
2. **Duplicate Invoice Check** — Prevent duplicate PO numbers
3. **Dashboard KPIs** — AR/AP totals on dashboard
4. **Keyboard Shortcuts** — Quick navigation
5. **Bulk Actions** — Multi-select for status updates
6. **Search Everywhere** — Global search bar
7. **Recent Activity** — User activity feed
8. **Export to CSV** — All list views exportable

---

## Technical Debt & Improvements

### Code Quality
- [ ] Fix template embedding issue (new templates not embedded)
- [ ] Add comprehensive unit tests for AR module
- [ ] Add integration tests for AR workflows
- [ ] Refactor handler error responses to be consistent

### Performance
- [ ] Add database indexes for AR queries
- [ ] Cache frequently accessed data (company settings)
- [ ] Optimize aging report query

### Security
- [ ] Add rate limiting on login
- [ ] Implement password complexity rules
- [ ] Add 2FA support
- [ ] Session timeout configuration

### DevOps
- [ ] Add CI/CD pipeline
- [ ] Add staging environment
- [ ] Implement blue-green deployment
- [ ] Add automated database backups

---

## Recommended Next Steps

1. **Immediate (Next 2 weeks)**
   - Fix template embedding issue
   - Add AR unit tests
   - Start Phase 10 (AP) planning

2. **Short-term (1 month)**
   - Complete Phase 10 (AP)
   - Implement Bank Management basics
   - Add PDF invoice email

3. **Medium-term (3 months)**
   - Complete Bank Reconciliation
   - Implement Stock Valuation
   - Add Budget tracking

---

## Conclusion

Prioritas utama adalah menyelesaikan **Accounts Payable (Phase 10)** untuk melengkapi siklus finance dasar. Setelah itu, **Bank Management** dan **Inventory Enhancements** akan memberikan nilai bisnis tertinggi.

Quick wins seperti PDF email dan dashboard KPIs dapat diimplementasikan secara paralel untuk meningkatkan user experience.
