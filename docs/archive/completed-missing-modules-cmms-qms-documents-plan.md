# CMMS, QMS, and Document Management Execution Plan

**Status:** Completed. The CMMS, QMS, and Documents modules have been fully implemented with shared governance foundations, outbox integration, and electronic signatures.

**Note:** This execution plan has been moved to the archive as all implementation tasks are finished.

## Summary

Deliver three distinct modules with shared governance foundations:

- The proposed Document Management module owns managed binary storage, immutable versions, controlled
  review, electronic signatures, retention, and document-level access.
- The proposed CMMS module owns maintainable equipment, preventive and corrective work,
  downtime, labor, parts, vendors, and maintenance cost.
- The proposed QMS module owns enterprise quality plans, inspections, quality events, holds,
  NCRs, CAPAs, audits, complaints, supplier quality, and controlled dispositions.

Fixed Assets remains the financial asset ledger, MRP remains the production-execution
system, and Procurement remains the owner of commercial supplier ratings. The new
modules integrate with those authorities rather than duplicating them.

## 1. Architecture and ownership

Use these source-of-truth boundaries:

| Concern | Authoritative module |
|---|---|
| Financial asset cost, depreciation, disposal | Fixed Assets |
| Operational asset condition and maintenance history | CMMS |
| Production orders, operations, WIP, and receipts | MRP |
| Enterprise quality definitions and controlled quality decisions | QMS |
| Commercial supplier performance and published overall rating | Procurement |
| Managed binary, document version, signature, and retention state | Document Management |

A CMMS asset may reference a fixed asset but need not be capitalized. QMS publishes a
quality component to Procurement without replacing Procurement's overall supplier
rating. MRP calls QMS for governed quality execution and blocking status after a
company is migrated.

Cross-module operations must use module services or outbox events. A module must not
write another module's tables directly.

## 2. Shared foundations

Apply these controls to all three modules:

- Company-scoped tables, indexes, queries, storage keys, jobs, and exports.
- Exact PostgreSQL `NUMERIC` values and Odyssey's exact money types for costs. Do not
  introduce new `float64` monetary boundaries.
- Optimistic versions and idempotency keys on important state transitions and worker
  jobs.
- The shared approval engine for controlled document activation/disposal, maintenance
  overrides, cost exceptions, quality dispositions, and CAPA verification.
- The existing audit system for lifecycle changes, downloads, signatures, evidence,
  overrides, integration events, and accounting actions.
- Immutable finalized records. Corrections use a superseding version, disposition,
  or reversal event rather than in-place mutation.
- Permissions seeded only to administrator roles; all other assignments remain
  explicit.
- Per-company feature flags, migration state, and enforcement mode.
- Transactional outbox delivery for cross-module facts and retry-safe consumers.

Define correlation and causation IDs for document, maintenance, and quality events so
one business action can be reconstructed across modules.

## 3. General document management

### 3.1 Storage service

Extract the local/S3-compatible storage pattern from Board Pack into a shared object
storage package. The interface must support put, open, stat, and controlled delete;
Document Management remains the only caller authorized to dispose managed objects.

- Store metadata in PostgreSQL and binary content in private object storage.
- Use S3-compatible storage in production and local storage in development/tests.
- Record SHA-256 checksum, byte size, declared and detected MIME type, storage key,
  encryption metadata, and malware-scan status.
- Generate opaque storage keys; never derive filesystem paths from uploaded names.
- Enter uploads as `QUARANTINED` until type validation and malware scanning complete.
- Reject size, content-type, checksum, or scan failures before a version is available.
- Stream authorized downloads or return short-lived signed URLs. Do not expose public
  permanent object URLs.
- Deduplicate only within a company. Physically delete a blob only when no retained
  version or legal hold references it.

Do not create another general-purpose `BYTEA` file boundary.

### 3.2 Core records

Add company-scoped tables for:

- Documents and immutable document versions.
- Binary blobs and their storage/scan state.
- Classifications, categories, numbering rules, and metadata schemas.
- Allow-listed links to domain records such as assets, work orders, suppliers, POs,
  inspections, NCRs, and CAPAs.
- Role/user ACL entries and classification defaults.
- Review and approval steps.
- Signature challenges and signatures.
- Retention policies, legal holds, disposition requests, and disposition evidence.
- Access events for preview, download, export, and authorized sharing.

A document has a stable identity and multiple immutable versions. The link service
validates company ownership and target existence before creating a domain link.
Target deletion never cascades into document deletion.

### 3.3 Version lifecycle

Document versions use:

`DRAFT → IN_REVIEW → APPROVED → EFFECTIVE → SUPERSEDED/OBSOLETE → ARCHIVED`

With `REJECTED`, `QUARANTINED`, and terminal `DISPOSED` states where applicable.

- Only one effective version is permitted for a controlled document.
- Submission freezes the binary, checksum, classification, and review snapshot.
- Rejected drafts may be revised into a new version; submitted content is never
  overwritten.
- Effective documents are replaced only by an approved successor.
- Disposition preserves a metadata tombstone, signatures, approvals, and audit trail.

### 3.4 Document permissions

Introduce:

- `documents.view`
- `documents.upload`
- `documents.version`
- `documents.review`
- `documents.approve`
- `documents.sign`
- `documents.share`
- `documents.retention.manage`
- `documents.hold.manage`
- `documents.dispose`
- `documents.admin`

Authorization requires both a module permission and record-level access. ACLs support
role grants, user grants, classification defaults, and explicit deny rules. Explicit
deny wins, and no document crosses a company boundary.

### 3.5 Electronic signatures

Implement regulator-neutral internal electronic attestations:

1. Create a short-lived, one-time reauthentication challenge.
2. Generate the canonical record/version snapshot on the server.
3. Verify signer permission, workflow step, separation of duties, and challenge.
4. Store the record version, SHA-256 hash, signature meaning, signer, timestamp,
   policy version, and authentication method.
5. Complete the governed transition and append its audit event transactionally.

Reject expired, replayed, wrong-record, wrong-version, or tampered challenges. Never
store passwords or client-supplied reauthentication evidence. This feature must not
be represented as a qualified or regulator-certified digital signature.

### 3.6 Retention and disposition

Retention begins from a policy-defined event such as approval, supersession,
work-order closure, quality-case closure, or contract expiry.

- Calculate and persist the retention trigger and expiry date.
- Legal holds block disposition regardless of expiry.
- Controlled classifications require an approved disposition request.
- A retry-safe worker executes approved dispositions and records evidence.
- Metadata tombstones and audit history survive physical binary deletion.
- Retention changes affect future calculations unless an approved migration explicitly
  recalculates existing records.

### 3.7 Existing document migration

- Migrate `portal_documents` binaries into managed object storage. (Completed)
- Create document identities and initial versions from existing ownership and file
  metadata.
- Use dual-read compatibility while checksums, counts, ownership, and downloads are
  reconciled.
- Stop new legacy writes only after the portal uses the document service.
- Retain the legacy table until migration evidence is accepted.
- Move Board Pack behind the shared storage interface without changing current board-
  pack lifecycle or access behavior.

## 4. CMMS

### 4.1 Asset and maintenance masters

Add:

- Maintainable assets and parent-child equipment assemblies.
- Optional links to fixed assets, products, work centers, and locations.
- Site, location, department, custodian, manufacturer, model, serial, tag, criticality,
  and operational condition.
- Meters, units, readings, correction history, and rollover rules.
- Failure, cause, remedy, downtime, task, labor-skill, and safety-code masters.
- Warranty, commissioning, manuals, certificates, and drawings through Document
  Management.

Keep financial asset status separate from operational condition. Prevent disposed
fixed assets from receiving new maintenance work.

### 4.2 Maintenance execution

Add:

- Maintenance requests and triage.
- Versioned preventive-maintenance strategies.
- Calendar-, meter-, and usage-based schedules.
- Corrective, preventive, inspection, emergency, and calibration work orders.
- Task checklists, permits, lockout/tagout evidence, and completion readings.
- Labor entries and approvals.
- Parts reservations, issues, returns, consumption, and exact cost snapshots.
- External service requests, vendor references, and exact service costs.
- Planned and actual downtime.
- Failure analysis, completion evidence, and next-due calculation.
- Immutable work-order events.

Work orders use:

`REQUESTED → TRIAGED → PLANNED → APPROVAL → RELEASED → IN_PROGRESS → ON_HOLD → COMPLETED → CLOSED`

With `REJECTED` and `CANCELLED`.

Preventive plans use `DRAFT → ACTIVE → SUSPENDED → RETIRED`. A retry-safe
worker generates each scheduled work order exactly once. (Completed)

### 4.3 CMMS permissions

Introduce:

- `cmms.asset.view` and `cmms.asset.manage`
- `cmms.request.create` and `cmms.request.triage`
- `cmms.plan.view` and `cmms.plan.manage`
- `cmms.work_order.view`, `cmms.work_order.release`,
  `cmms.work_order.execute`, and `cmms.work_order.close`
- `cmms.cost.view` and `cmms.cost.approve`
- `cmms.admin`

Separate work execution, work closure, and cost approval so companies can configure
separation of duties.

### 4.4 CMMS integrations

- **Fixed Assets:** identity, disposal restrictions, warranty, capitalization lineage,
  and asset history.
- **Inventory:** spare-part reservation, issue, return, and valuation.
- **Procurement:** maintenance PRs, POs, external services, and supplier references.
- **AP/Accounting:** vendor invoices and maintenance expense posting.
- **MRP:** planned and actual work-center downtime updates capacity calendars.
- **QMS:** calibration requirements and failed-calibration dispositions.
- **Documents:** manuals, permits, photographs, reports, and certificates.

Expense maintenance by default. Capital improvements require an explicit approved
capitalization workflow; they are never activated by a work-order checkbox.

### 4.5 CMMS reporting

Provide overdue preventive work, schedule compliance, asset availability, downtime,
MTBF, MTTR, planned-versus-corrective work, maintenance cost by asset/location/failure,
parts usage, vendor spend, and warranty recovery.

## 5. Standalone QMS

### 5.1 Quality masters and records

Add:

- Versioned specifications, inspection plans, characteristics, tolerances, sampling
  rules, defect codes, and disposition codes.
- Inspections and immutable result snapshots.
- Quality events, deviations, holds, and dispositions.
- Nonconformance reports.
- CAPAs, action items, effectiveness checks, and independent verification.
- Internal and supplier audits, findings, and follow-up actions.
- Customer complaints and response evidence.
- Supplier-quality cases and quality-scoring evidence.
- Controlled-procedure training acknowledgements.
- Quality decisions, approvals, signatures, and evidence links.

### 5.2 QMS lifecycles

- Specification/plan: `DRAFT → REVIEW → APPROVED → EFFECTIVE → RETIRED`.
- Inspection: `PLANNED → IN_PROGRESS → PASSED/FAILED/HOLD → CLOSED`.
- Quality event: `OPEN → TRIAGED → INVESTIGATING → DISPOSITIONED → CLOSED`.
- NCR: `OPEN → INVESTIGATING → DISPOSITION_APPROVAL → IMPLEMENTED → CLOSED`.
- CAPA: `OPEN → ROOT_CAUSE → ACTION_PLAN → IMPLEMENTATION → VERIFICATION → CLOSED`.
- Audit: `PLANNED → IN_PROGRESS → REPORTING → FOLLOW_UP → CLOSED`.
- Complaint: `RECEIVED → TRIAGED → INVESTIGATING → RESPONDED → CLOSED`.

Every controlled transition validates prerequisites, permissions, effective policy,
and optimistic version in the service layer.

### 5.3 QMS permissions

Introduce separate permission families:

- `qms.specification.*`
- `qms.inspection.*`
- `qms.hold.*`
- `qms.ncr.*`
- `qms.capa.*`
- `qms.audit.*`
- `qms.complaint.*`
- `qms.supplier_quality.*`
- `qms.admin`

Inspection execution, disposition approval, CAPA ownership, and CAPA verification use
different permissions to support configurable separation of duties.

### 5.4 MRP quality migration

Existing `mrp_inspection_plans`, inspections, holds, NCRs, and CAPAs are migration
sources rather than a second permanent quality authority.

1. Create QMS records with immutable legacy source identifiers.
2. Backfill plans, inspections, holds, NCRs, CAPAs, owners, links, and lifecycle state.
3. Reconcile company ownership, counts, status mappings, and evidence.
4. Switch each company's MRP quality operations to QMS services.
5. Make migrated MRP quality tables read-only for that company.
6. Preserve compatibility views until all readers use QMS.
7. Retain historical MRP rows throughout the initial rollout.

After cutover, MRP requests inspections from QMS and queries blocking holds before
operation completion or finished-goods receipt. QMS publishes quality performance to
Procurement without replacing commercial supplier ratings.

### 5.5 Calibration boundary

QMS defines calibration requirements, tolerances, acceptance, and the disposition of
failed calibration. CMMS owns equipment scheduling, work execution, labor, parts,
vendors, downtime, and cost. Both modules link the same certificates and evidence
through Document Management.

## 6. Delivery sequence

| Phase | Deliverable | Indicative duration |
|---|---|---:|
| 0 | Ownership ADRs, threat model, permission matrix, migration rehearsal | 2–3 weeks |
| 1 | Shared object storage and document upload/version foundation | 4–5 weeks |
| 2 | Document approval, ACL, signatures, retention, and portal migration | 4–6 weeks |
| 3 | CMMS asset registry, requests, preventive plans, and work orders | 5–6 weeks |
| 4 | CMMS inventory, procurement, accounting, MRP, and reporting integration | 4–5 weeks |
| 5 | QMS specifications, inspections, events, holds, NCR, and CAPA | 6–8 weeks |
| 6 | MRP migration, audits, complaints, supplier quality, and calibration | 4–6 weeks |
| 7 | Security, performance, recovery, staging certification, and rollout | 3–4 weeks |

CMMS and QMS may run as parallel streams after the document foundation is stable.
Each phase must leave a deployable, disabled-by-default vertical slice.

## 7. Rollout and migration

1. Add schemas and services without enabling routes or new writes.
2. Enable Document Management for an internal staging company.
3. Migrate portal documents and reconcile every checksum and owner.
4. Pilot CMMS with one site and a bounded asset set.
5. Pilot QMS with one product family while legacy MRP quality remains readable.
6. Run dual-read reconciliation; do not dual-write quality decisions.
7. Obtain document owner, maintenance owner, quality owner, security, finance, and
   operations sign-off.
8. Cut over per company with a recorded migration checkpoint and rollback procedure.
9. Remove compatibility paths only after all enabled companies pass reconciliation.

## 8. Mandatory validation

- Fresh-schema, upgrade, and rollback migration rehearsals.
- Company isolation for metadata, binaries, search, links, signatures, and exports.
- File-size, MIME spoofing, checksum, malware, path traversal, and storage-failure
  tests.
- ACL, explicit-deny, linked-record, and download authorization tests.
- Signature expiry, replay, tampering, wrong-record, and wrong-version tests.
- Retention calculation, legal hold, approved disposition, and retry tests.
- CMMS schedule, meter correction, concurrent completion, parts, downtime, and exact
  cost tests.
- QMS lifecycle, separation-of-duties, blocking hold, disposition, and CAPA
  verification tests.
- MRP migration count, ownership, link, and status reconciliation.
- Idempotent workers, duplicate requests, and optimistic conflict tests.
- Transaction rollback proving no partial inventory or accounting changes.
- Immutable audit and complete evidence-export tests.
- Full verification with `go test ./...`, `go vet ./...`, `make lint`, and
  `make docs-check`.

## 9. Exit criteria

- Managed binaries no longer depend on database `BYTEA` storage.
- Every managed document version is traceable, permission-controlled, and governed by
  retention policy.
- Electronic signatures bind an authenticated signer to an immutable record version.
- CMMS provides complete maintenance, labor, parts, vendor, downtime, and cost history.
- Preventive work is generated exactly once and overdue work is visible.
- QMS is authoritative for new enterprise quality records in enabled companies.
- MRP cannot bypass blocking QMS holds or mandatory inspections after cutover.
- Portal documents and migrated MRP records reconcile with their replacements.
- Each module can be enabled per company only after staging sign-off.

## 10. Deferred scope

- Qualified legal digital signatures and external trust providers.
- OCR, AI classification, and collaborative office editing.
- CAD/PDM and product lifecycle management.
- Predictive maintenance and native IoT ingestion.
- Full LIMS, laboratory-instrument integration, and advanced SPC.
- Regulator-specific certification claims.

## Assumptions

- Password reauthentication is the first signature method; 2FA/SSO may replace it
  later.
- Object storage provides encryption at rest and transport security in production.
- Compliance policies are configurable and regulator-neutral.
- CMMS and QMS are top-level modules; manufacturing quality remains embedded only
  until each company completes its controlled QMS migration.
