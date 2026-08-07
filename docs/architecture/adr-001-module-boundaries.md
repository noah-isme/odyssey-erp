# ADR-001: Module Boundaries for Documents, CMMS, and QMS

## Status
Accepted

## Context
Odyssey needs to introduce three new top-level modules: Document Management, CMMS (Computerized Maintenance Management System), and QMS (Quality Management System). These modules must integrate with existing modules (Fixed Assets, MRP, Procurement, Inventory, Accounting) without duplicating their authoritative responsibilities.

## Decision

### Source of Truth Boundaries

| Concern | Authoritative Module | Integration Pattern |
|---------|---------------------|---------------------|
| Financial asset cost, depreciation, disposal | Fixed Assets | CMMS references fixed asset ID; disposed assets blocked from new work |
| Operational asset condition and maintenance history | CMMS | Fixed Assets receives capitalization requests via approval workflow |
| Production orders, operations, WIP, and receipts | MRP | QMS called for inspections/holds; CMMS provides downtime to MRP capacity |
| Enterprise quality definitions and controlled quality decisions | QMS | MRP requests inspections/holds; Procurement receives quality component |
| Commercial supplier performance and published overall rating | Procurement | QMS publishes quality score component; Procurement aggregates overall |
| Managed binary, document version, signature, and retention state | Document Management | All modules link documents; Doc Mgmt owns storage lifecycle |

### Cross-Module Rules
1. **No direct table writes**: Modules communicate via services or transactional outbox events
2. **Company-scoped isolation**: All tables, indexes, queries, storage keys, jobs, and exports are company-scoped
3. **Correlation/causation IDs**: Every cross-module event carries correlation and causation UUIDs for traceability
4. **Immutable finalized records**: Corrections use superseding versions, dispositions, or reversals—not in-place mutations
5. **Feature flags**: Per-company feature flags, migration state, and enforcement mode for each module

### Shared Foundations
- Exact `NUMERIC` for money, no `float64` monetary boundaries
- Optimistic versions and idempotency keys on state transitions
- Shared approval engine (existing `approvals` module) for governed transitions
- Existing audit system for lifecycle changes, downloads, signatures, evidence
- Permissions seeded only to admin roles; all other assignments explicit
- Transactional outbox for cross-module facts with retry-safe consumers

## Consequences
- Clear ownership boundaries prevent feature creep and data inconsistency
- Integration via services/outbox adds slight latency but ensures consistency
- Migration path: Phase 0 establishes boundaries, Phase 1+ builds on them
- Testing must verify company isolation at every layer

## References
- [CMMS Guide](../guides/cmms.md)
- [QMS Guide](../guides/qms.md)
- [Document Management Guide](../guides/documents.md)