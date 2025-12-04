# Phase 9 Kickoff – Sales & Accounts Receivable

## 🎯 Mission Statement

**"Melengkapi siklus bisnis Odyssey ERP dengan modul Sales & AR yang terintegrasi penuh, memberikan visibility real-time atas pipeline penjualan, fulfillment, dan kesehatan piutang."**

---

## 📋 Executive Summary

Phase 9 adalah kelanjutan natural dari Phase 1-8 yang melengkapi sisi **revenue** setelah **procurement (AP)** selesai di Phase 3. Dengan Sales & AR, bisnis dapat:

- 📝 Mengelola quotation & sales order tanpa spreadsheet manual
- 📦 Track delivery & fulfillment dengan automatic stock reduction
- 💰 Record AR invoice & payment dengan auto journal entries
- 📊 Monitor AR aging untuk proactive collection
- 🔄 Full integration dengan inventory, accounting, dan RBAC

---

## 🎪 Phase Structure

| Cycle | Focus | Duration | Key Deliverables |
|-------|-------|----------|------------------|
| **9.1** | Quotation & Sales Order | 5-6 hari | Domain, approval workflow, SSR UI, RBAC |
| **9.2** | Delivery & Fulfillment | 4-5 hari | DO management, stock integration, packing list PDF |
| **9.3** | AR Invoice & Payment | 6-7 hari | Invoice posting, payment allocation, aging report, GL integration |
| **Buffer** | Integration & Hardening | 2-3 hari | E2E testing, docs finalization, staging deployment |
| **Total** | **17-21 hari** | ~4 weeks | Complete Sales & AR module |

---

## 🚀 What's New in Phase 9

### Cycle 9.1 – Quotation & Sales Order

**Database**: 4 new tables
- `quotations` – sales quotations with approval workflow
- `quotation_lines` – line items per quotation
- `sales_orders` – confirmed sales orders
- `sales_order_lines` – line items with delivery tracking

**Features**:
- ✅ Create quotation with customer & product selection
- ✅ Submit → Approve → Convert to Sales Order
- ✅ Rejection flow with reasons
- ✅ SO confirmation with soft stock check
- ✅ RBAC: separate create, approve, confirm permissions

**UI Pages**: 8 new SSR pages
- Quotation list, detail, create, edit
- Sales Order list, detail, create, edit

---

### Cycle 9.2 – Delivery & Fulfillment

**Database**: 2 new tables
- `delivery_orders` – delivery documents
- `delivery_order_lines` – items to deliver

**Features**:
- ✅ Create DO from confirmed SO
- ✅ Partial delivery support (multiple DOs per SO)
- ✅ Stock reduction via `inventory_tx` (reuse Phase 3)
- ✅ Packing list PDF generation (Gotenberg)
- ✅ Auto-update SO status (PROCESSING → COMPLETED)

**Integration Points**:
- **Inventory Module**: Stock reduction on DO confirm
- **Background Jobs**: Async DO confirmation via Asynq

---

### Cycle 9.3 – AR Invoice & Payment

**Database**: 5 new tables + 1 materialized view
- `ar_invoices` – customer invoices
- `ar_invoice_lines` – invoice line items
- `ar_payments` – payment records
- `ar_payment_allocations` – payment-to-invoice matching
- `mv_ar_aging` – aging buckets (current, 1-30, 31-60, 61-90, 90+)

**Features**:
- ✅ Create invoice from DO/SO or manual entry
- ✅ Post invoice → create journal entry (DR AR, CR Revenue/Tax)
- ✅ Record payment → allocate to invoice(s)
- ✅ Auto-update invoice status (ISSUED → PARTIALLY_PAID → PAID)
- ✅ AR aging report with pivot table
- ✅ Customer statement PDF
- ✅ Daily cron: overdue detection & aging refresh

**Integration Points**:
- **Accounting Module**: Auto journal entries for invoice & payment
- **Jobs**: Async posting, daily overdue checks, aging refresh

---

## 🏗️ Architecture Highlights

### Hexagonal Architecture (Consistent with Phase 1-8)

```
┌─────────────────────────────────────────────────────┐
│                    HTTP Handler                     │
│  (SSR forms, RBAC middleware, CSRF protection)      │
└──────────────────┬──────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────┐
│                  Service Layer                      │
│  (Business logic, validations, orchestration)       │
└──────┬────────────────────────────────┬─────────────┘
       │                                │
       ▼                                ▼
┌─────────────────┐            ┌──────────────────────┐
│   Repository    │            │  Background Jobs     │
│  (DB queries)   │            │  (Asynq workers)     │
└─────────────────┘            └──────────────────────┘
       │                                │
       ▼                                ▼
┌─────────────────────────────────────────────────────┐
│              PostgreSQL Database                    │
│  (Transactions, constraints, materialized views)    │
└─────────────────────────────────────────────────────┘
```

### Integration Map

```
                    ┌─────────────┐
                    │   Sales     │
                    │   Module    │
                    └──────┬──────┘
                           │
       ┌───────────────────┼───────────────────┐
       │                   │                   │
       ▼                   ▼                   ▼
┌─────────────┐    ┌──────────────┐    ┌────────────┐
│  Inventory  │    │  Accounting  │    │ Auth/RBAC  │
│   (Stock)   │    │   (Journals) │    │ (Perms)    │
└─────────────┘    └──────────────┘    └────────────┘
       │                   │
       └───────────┬───────┘
                   │
                   ▼
           ┌──────────────┐
           │  Audit Logs  │
           └──────────────┘
```

---

## 📊 Key Metrics & KPIs

### Technical Metrics

- **Test Coverage**: ≥ 70% per cycle
- **API Response Time**: p95 <500ms (reads), <1s (writes)
- **PDF Generation**: <5s per document
- **Job Success Rate**: >99%
- **Zero Critical Bugs**: In production

### Business Metrics

- **Sales Pipeline Visibility**: 100% quotation → SO → delivery tracked in system
- **AR Accuracy**: 100% invoice & payment reconciled with GL
- **Aging Report**: Real-time aging buckets vs 1-week delay (manual)
- **Fulfillment Speed**: Reduce DO processing time by 50%
- **Manual Work Reduction**: 80% reduction in spreadsheet tracking

---

## 🔐 Security & Compliance

### RBAC Permissions (16 new permissions)

**Sales**:
- `sales.quotation.*` (view, create, edit, approve, delete)
- `sales.order.*` (view, create, edit, confirm, cancel, delete)
- `sales.delivery.*` (view, create, confirm, cancel, download_pdf)

**Finance**:
- `finance.ar.invoice.*` (view, create, edit, post, cancel, download_pdf)
- `finance.ar.payment.*` (view, create, post, cancel)
- `finance.ar.aging.*` (view, export)

### Security Controls

- ✅ CSRF protection on all forms
- ✅ SQL injection prevention (parameterized queries)
- ✅ XSS prevention (HTML escaping)
- ✅ Rate limiting (10 req/min on exports)
- ✅ Audit trail for all financial transactions
- ✅ Immutable posted invoices/payments
- ✅ HTTPS enforced in production

See: `docs/security-checklist-phase9.md`

---

## 📚 Documentation Deliverables

### Planning & Architecture
- ✅ `docs/PLAN-Phase9-Sales.md` – 992 lines, comprehensive plan
- ✅ `docs/TESTING-PHASE9.md` – 856 lines, full testing strategy
- ✅ `docs/security-checklist-phase9.md` – 416 lines, security requirements

### User Guides (To be created during implementation)
- [ ] `docs/howto-sales-quotation.md`
- [ ] `docs/howto-sales-delivery.md`
- [ ] `docs/howto-ar-invoice.md`
- [ ] `docs/howto-ar-aging.md`

### Operations (To be created during implementation)
- [ ] `docs/runbook-sales.md`
- [ ] `docs/runbook-ar.md`
- [ ] `docs/troubleshooting-phase9.md`

---

## 🧪 Testing Strategy

### Test Pyramid Distribution

- **Unit Tests (60%)**: Business logic, validations, calculations
- **Integration Tests (30%)**: Repository + DB, service + dependencies
- **E2E Tests (10%)**: Critical user journeys

### Key Test Scenarios

1. **Quotation → SO → DO → Invoice → Payment** (happy path)
2. **Partial Delivery & Payment** (complex scenario)
3. **Approval Rejection & Resubmit** (workflow)
4. **Stock Validation on Delivery** (inventory integration)
5. **AR Aging Accuracy** (reporting)

### Tools

- `testing` + `testify` – unit tests
- Docker testcontainer – integration tests with PostgreSQL
- `httpexpect` – HTTP handler tests
- Manual QA – UI/UX verification

---

## 🚦 Success Criteria

Before declaring Phase 9 complete:

### Functional
- ✅ All 3 cycles implemented and merged to `main`
- ✅ All user stories completed
- ✅ All migrations applied successfully

### Quality
- ✅ Test coverage ≥ 70% per module
- ✅ Zero critical/high bugs in production
- ✅ All E2E scenarios passing
- ✅ Performance benchmarks met

### Security
- ✅ Security checklist 100% complete
- ✅ Penetration test passed
- ✅ RBAC enforced on all endpoints
- ✅ Audit trail verified

### Documentation
- ✅ All docs updated (howto, runbook, architecture)
- ✅ User guides reviewed by stakeholders
- ✅ Operations runbooks reviewed by ops team

### Production Readiness
- ✅ Staging deployment successful
- ✅ UAT completed by business users
- ✅ Rollback plan tested
- ✅ Monitoring dashboards configured
- ✅ Alerting rules deployed

---

## 📅 Timeline & Milestones

### Week 1 – Cycle 9.1 (Quotation & SO)
- **Day 1-2**: Schema migration, domain model, repository
- **Day 3-4**: Service layer, approval workflow, RBAC
- **Day 5-6**: SSR UI, tests, documentation

**Milestone**: ✅ Can create, approve, and convert quotations to SO

---

### Week 2 – Cycle 9.2 (Delivery)
- **Day 1-2**: DO schema, inventory integration, service layer
- **Day 3-4**: SSR UI, packing list PDF, background jobs
- **Day 5**: Tests, documentation, integration verification

**Milestone**: ✅ Can fulfill SO via DO with automatic stock reduction

---

### Week 3 – Cycle 9.3 (AR Invoice & Payment)
- **Day 1-2**: AR schema, domain model, accounting integration
- **Day 3-4**: Invoice posting, payment allocation, journal entries
- **Day 5-6**: Aging report, SSR UI, PDFs
- **Day 7**: Tests, documentation

**Milestone**: ✅ Can post invoices, record payments, and view AR aging

---

### Week 4 – Integration & Hardening
- **Day 1-2**: E2E testing, regression testing
- **Day 3**: Documentation finalization, security review
- **Day 4**: Staging deployment, UAT
- **Day 5**: Production deployment, monitoring setup

**Milestone**: ✅ Phase 9 in production, all systems green

---

## 🎯 Team Roles & Responsibilities

### Tech Lead
- Architecture decisions & code reviews
- Coordinate integration points with existing modules
- Daily standup facilitation
- Documentation oversight

### Backend Engineers
- Implement domain, repository, service layers
- Write unit & integration tests
- Background job implementation
- API/handler development

### QA Engineer
- Execute test plans (unit, integration, E2E)
- Manual QA checklist verification
- Bug reporting & regression testing
- Performance & security testing

### Product Owner
- User story validation
- UAT coordination
- Stakeholder communication
- Go/no-go decision

---

## ⚠️ Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| **Stock sync issues** | High | Medium | Use DB transactions, retry logic, reconciliation job |
| **Journal entry errors** | High | Low | Extensive testing, idempotency checks, audit trail |
| **Performance degradation** | Medium | Medium | Indexes, materialized views, caching |
| **Complex partial scenarios** | Medium | Medium | Start simple, iterate based on feedback |
| **Scope creep** | Medium | High | Strict adherence to plan, defer enhancements to Phase 10 |

---

## 🔄 Dependencies

### External Services
- **Gotenberg** – PDF generation (reuse from Phase 8)
- **PostgreSQL** – Primary database
- **Redis** – Session, rate limiting, job queue
- **Asynq** – Background job processing

### Internal Modules
- **Inventory** – Stock reduction integration (Phase 3)
- **Accounting** – Journal entry creation (Phase 4)
- **Auth/RBAC** – Permissions & access control (Phase 1)
- **Audit** – Audit logging (Phase 2)

---

## 📞 Communication Plan

### Daily Standups
- **Time**: Every morning, 9:00 AM (15 min)
- **Format**: What I did, what I'm doing, blockers
- **Tool**: Slack standup bot or video call

### Weekly Sync
- **Time**: Every Friday, 3:00 PM (30 min)
- **Agenda**: Progress review, risk assessment, next week planning
- **Attendees**: Tech lead, engineers, QA, product owner

### Stakeholder Demos
- **When**: End of each cycle
- **Format**: Live demo + Q&A (45 min)
- **Attendees**: Product owner, sales team, finance team, management

---

## 🎉 What Success Looks Like

### For Sales Team
- 📝 Create quotations in system (no Excel)
- ✅ Track approval status in real-time
- 🚀 Convert approved quotes to orders in 1 click
- 📊 View sales pipeline at a glance

### For Warehouse Team
- 📦 Receive delivery orders from confirmed SOs
- ✅ Confirm deliveries → stock auto-updates
- 📄 Print packing lists with 1 click
- 🔍 Track fulfillment status per SO

### For Finance Team
- 💰 Create invoices from deliveries (no manual entry)
- ✅ Post invoices → journal entries auto-created
- 💵 Record payments & allocate to invoices
- 📊 View AR aging report in real-time (no monthly Excel)
- 📧 Track overdue invoices automatically

### For Management
- 📈 Real-time revenue pipeline visibility
- 💰 Accurate AR aging for cash flow planning
- ✅ Reduced manual errors & reconciliation time
- 🚀 Faster quote-to-cash cycle time

---

## 🚀 Next Steps

### Immediate Actions
1. ✅ **Review & approve this plan** – Tech lead + product owner
2. ✅ **Create GitHub project board** – Track tasks per cycle
3. ✅ **Schedule kickoff meeting** – Align team on goals & timeline
4. ⏭️ **Begin Cycle 9.1 implementation** – Schema migration + domain model

### Pre-Implementation Checklist
- [ ] All team members read planning docs
- [ ] Development environment set up (Gotenberg, PostgreSQL, Redis)
- [ ] Test fixtures prepared
- [ ] RBAC permissions design reviewed
- [ ] GL account mapping confirmed with finance team

---

## 📖 Reference Documents

- **Planning**: `docs/PLAN-Phase9-Sales.md`
- **Testing**: `docs/TESTING-PHASE9.md`
- **Security**: `docs/security-checklist-phase9.md`
- **Main README**: `docs/README.md` (updated with Phase 9 scope)
- **Changelog**: `docs/CHANGELOG.md` (Phase 9 placeholder added)

---

## 💬 Questions & Support

### Got Questions?
- **Technical**: Ask in #phase9-tech Slack channel
- **Product**: Ask product owner directly
- **Blockers**: Escalate to tech lead

### Need Help?
- **Code Review**: Tag tech lead in PR
- **Testing**: Consult `TESTING-PHASE9.md`
- **Security**: Refer to `security-checklist-phase9.md`

---

## 🎯 Remember

> **"Phase 9 bukan hanya tentang menambahkan fitur baru, tapi melengkapi siklus bisnis end-to-end. Setiap line of code kita tulis membantu bisnis berjalan lebih efisien, lebih akurat, dan lebih cepat."**

**Let's ship Phase 9! 🚀**

---

**Document Version**: 1.0  
**Created**: 2025-01-16  
**Author**: Technical Lead  
**Status**: ✅ Ready for Kickoff