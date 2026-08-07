# Permission Matrix: Documents, CMMS, and QMS Modules

## Overview
This document defines the complete permission matrix for the three new modules. Permissions follow Odyssey's RBAC pattern: module-scoped, action-based, seeded only to administrator roles.

## Document Management Permissions

| Permission | Description | Seeded To | Separation of Duties |
|------------|-------------|-----------|---------------------|
| `documents.view` | View document metadata and list documents | All internal roles | — |
| `documents.upload` | Upload new documents (create draft) | Document Controllers | — |
| `documents.version` | Create new versions of existing documents | Document Controllers | Cannot approve own version |
| `documents.review` | Review documents in review workflow | Reviewers, QA | Cannot approve own review |
| `documents.approve` | Approve documents for effectiveness | Approvers, Managers | Cannot approve own draft/review |
| `documents.sign` | Apply electronic signature to document versions | Authorized Signers | Reauthentication challenge required |
| `documents.share` | Generate authorized sharing links | Document Controllers, Managers | Explicit deny on classification |
| `documents.retention.manage` | Create/modify retention policies | Records Managers, Compliance | — |
| `documents.hold.manage` | Place/remove legal holds | Legal, Compliance | — |
| `documents.dispose` | Execute approved disposition requests | Records Managers | Requires approved disposition request |
| `documents.admin` | Full administrative access to document module | System Admins | — |

### Document ACL Model
- **Role grants**: Permission granted to role for classification/category
- **User grants**: Explicit permission to user for specific document
- **Classification defaults**: Default ACL per classification (e.g., "Confidential" = deny all except explicit grants)
- **Explicit deny**: Always wins over grants
- **Company boundary**: No document crosses company boundary

## CMMS Permissions

| Permission | Description | Seeded To | Separation of Duties |
|------------|-------------|-----------|---------------------|
| `cmms.asset.view` | View maintainable assets and hierarchy | Maintenance Techs, Planners | — |
| `cmms.asset.manage` | Create/modify assets, meters, criticality | Maintenance Managers | — |
| `cmms.request.create` | Create maintenance requests | All Internal, Operators | — |
| `cmms.request.triage` | Triage and prioritize requests | Maintenance Planners, Supervisors | — |
| `cmms.plan.view` | View preventive maintenance plans | Maintenance Techs, Planners | — |
| `cmms.plan.manage` | Create/modify/activate PM strategies | Maintenance Managers, Planners | — |
| `cmms.work_order.view` | View work orders and details | All Maintenance | — |
| `cmms.work_order.release` | Release planned work orders | Maintenance Supervisors | Cannot release own planned WO |
| `cmms.work_order.execute` | Execute work (labor, readings, checklists) | Maintenance Techs | Cannot close own WO |
| `cmms.work_order.close` | Close completed work orders | Maintenance Supervisors, QA | Cannot close own executed WO |
| `cmms.cost.view` | View maintenance costs (labor, parts, vendor) | Maintenance Managers, Finance | — |
| `cmms.cost.approve` | Approve cost exceptions, capital improvements | Maintenance Managers, Plant Manager | Cannot approve own WO costs |
| `cmms.admin` | Full administrative access to CMMS | System Admins | — |

### CMMS Separation of Duties Matrix

| Action | Execute | Release | Close | Cost Approve |
|--------|---------|---------|-------|--------------|
| `cmms.work_order.execute` | ✓ | ✗ | ✗ | ✗ |
| `cmms.work_order.release` | ✗ | ✓ | ✗ | ✗ |
| `cmms.work_order.close` | ✗ | ✗ | ✓ | ✗ |
| `cmms.cost.approve` | ✗ | ✗ | ✗ | ✓ |

**Rule**: A single user cannot hold more than one of {execute, release, close, cost.approve} for the same work order without explicit override approval.

## QMS Permissions

### Specification & Inspection Plan Family
| Permission | Description | Seeded To | Separation of Duties |
|------------|-------------|-----------|---------------------|
| `qms.specification.view` | View specifications, inspection plans | Quality, Production | — |
| `qms.specification.manage` | Create/modify specifications, plans | Quality Engineers, QA Managers | — |
| `qms.specification.approve` | Approve specifications for effectiveness | QA Managers | Cannot approve own draft |

### Inspection Family
| Permission | Description | Seeded To | Separation of Duties |
|------------|-------------|-----------|---------------------|
| `qms.inspection.view` | View inspections and results | Quality, Production | — |
| `qms.inspection.execute` | Perform inspections, record results | Quality Inspectors | Cannot disposition own inspection |
| `qms.inspection.disposition` | Approve inspection disposition (pass/fail/hold) | Quality Engineers, QA | Cannot disposition own execution |

### Hold Family
| Permission | Description | Seeded To | Separation of Duties |
|------------|-------------|-----------|---------------------|
| `qms.hold.view` | View quality holds | Quality, Production | — |
| `qms.hold.create` | Place quality holds | Quality Inspectors, QA | — |
| `qms.hold.release` | Release quality holds | Quality Engineers, QA Managers | Cannot release own hold |

### NCR Family
| Permission | Description | Seeded To | Separation of Duties |
|------------|-------------|-----------|---------------------|
| `qms.ncr.view` | View NCRs | Quality, Production | — |
| `qms.ncr.create` | Create NCRs | Quality Inspectors, Production | — |
| `qms.ncr.investigate` | Investigate and propose disposition | Quality Engineers | — |
| `qms.ncr.disposition` | Approve NCR disposition | QA Managers | Cannot disposition own investigation |

### CAPA Family
| Permission | Description | Seeded To | Separation of Duties |
|------------|-------------|-----------|---------------------|
| `qms.capa.view` | View CAPAs | Quality, Production | — |
| `qms.capa.create` | Create CAPAs from NCRs/audits | Quality Engineers | — |
| `qms.capa.implement` | Implement action plans | Responsible Parties | Cannot verify own implementation |
| `qms.capa.verify` | Verify CAPA effectiveness | QA Managers, Independent Verifiers | Cannot verify own implementation |

### Audit Family
| Permission | Description | Seeded To | Separation of Duties |
|------------|-------------|-----------|---------------------|
| `qms.audit.view` | View audits and findings | Quality, Management | — |
| `qms.audit.plan` | Plan and schedule audits | QA Managers | — |
| `qms.audit.execute` | Conduct audits, record findings | Auditors | Cannot audit own area |
| `qms.audit.followup` | Track and close audit findings | Quality Engineers, QA | — |

### Complaint Family
| Permission | Description | Seeded To | Separation of Duties |
|------------|-------------|-----------|---------------------|
| `qms.complaint.view` | View customer complaints | Quality, Customer Service | — |
| `qms.complaint.manage` | Create, investigate, respond to complaints | Quality Engineers, CS Managers | — |
| `qms.complaint.close` | Close complaints with evidence | QA Managers | Cannot close own investigation |

### Supplier Quality Family
| Permission | Description | Seeded To | Separation of Duties |
|------------|-------------|-----------|---------------------|
| `qms.supplier_quality.view` | View supplier quality cases | Quality, Procurement | — |
| `qms.supplier_quality.manage` | Create/manage supplier quality cases | Quality Engineers, SQE | — |
| `qms.supplier_quality.score` | Publish quality component to Procurement | QA Managers | Automated on case closure |

### QMS Admin
| Permission | Description | Seeded To |
|------------|-------------|-----------|
| `qms.admin` | Full administrative access to QMS | System Admins |

## Cross-Module Permission Dependencies

### Documents → CMMS
- CMMS links documents (manuals, certificates, photos) via `documents.view` + document ACL
- CMMS work orders can require document attachments → `documents.upload`

### Documents → QMS
- QMS links documents (specifications, certificates, evidence) via `documents.view` + document ACL
- QMS inspections/NCRs/CAPAs can require document attachments → `documents.upload`

### CMMS → Fixed Assets
- `cmms.asset.view` + `fixedassets.asset.view` to see linked fixed asset
- Capital improvement workflow requires `fixedassets.capitalization.approve`

### CMMS → Inventory
- `cmms.work_order.execute` + `inventory.stock.issue` for parts issuance
- `cmms.work_order.execute` + `inventory.stock.reserve` for parts reservation

### CMMS → Procurement
- `cmms.work_order.execute` + `procurement.pr.create` for external services
- Vendor references via `procurement.supplier.view`

### CMMS → Accounting
- `cmms.cost.view` + `accounting.journal.view` for expense posting
- `cmms.cost.approve` + `accounting.journal.approve` for capitalization

### CMMS → MRP
- `cmms.work_order.close` publishes downtime → `mrp.capacity.view`
- Requires `mrp.workcenter.view` for work center references

### QMS → MRP
- `qms.inspection.execute` + `mrp.operation.complete` for inspection gates
- `qms.hold.release` required before `mrp.operation.complete` / `mrp.receipt.create`

### QMS → Procurement
- `qms.supplier_quality.score` publishes quality component → `procurement.supplier.quality_component`
- Does NOT grant `procurement.supplier.rating.manage` (commercial rating stays in Procurement)

### QMS → Documents
- All quality records link evidence via Document Management
- Requires `documents.upload` for evidence attachment
- Retention policies via `documents.retention.manage`

## Permission Seeding Rules

1. **Only administrator roles** receive seeded permissions:
   - `documents.admin` → `system_admin`, `company_admin`
   - `cmms.admin` → `system_admin`, `company_admin`
   - `qms.admin` → `system_admin`, `company_admin`

2. **All other permissions** are explicitly assigned via:
   - Role grants in RBAC UI
   - User grants for specific records
   - Classification/category defaults (Documents)

3. **No implicit grants** from existing roles (e.g., `mrp.operator` does not get `qms.inspection.execute`)

4. **Feature flag gating**: Module permissions only effective when module enabled for company

## Migration Notes

### Existing Permissions Mapping
| Legacy Permission | Module | New Permissions |
|-------------------|--------|-----------------|
| `portal.manage` | Portal | `documents.upload` (for portal docs) |
| `mrp.quality_*` | MRP | Mapped to `qms.*` per family |
| `fixedassets.maintenance_*` | Fixed Assets | Mapped to `cmms.*` |

### Dual-Read Period
During MRP→QMS migration:
- `mrp.quality_*` permissions remain functional for legacy tables
- `qms.*` permissions control new QMS tables
- Migration tooling reconciles permission assignments

## Validation Checklist

- [ ] All permissions documented in this matrix
- [ ] Permissions follow `module.action` naming convention
- [ ] Separation of duties defined for all critical transitions
- [ ] No permission crosses company boundary
- [ ] Seeded only to admin roles
- [ ] Cross-module dependencies documented
- [ ] Migration mapping complete for legacy permissions
- [ ] RBAC UI supports all new permissions

## References
- [ADR-001: Module Boundaries](./adr-001-module-boundaries.md)
- [Threat Model](./threat-model-documents-cmms-qms.md)
- [Missing Modules Plan](../guides/cmms.md)
- [RBAC Documentation](../reference/rbac.md)