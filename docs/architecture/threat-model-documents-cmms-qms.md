# Threat Model: Documents, CMMS, and QMS Modules

## Overview
This threat model covers the three new modules introduced in the CMMS/QMS/Documents initiative. It follows STRIDE methodology and assumes the existing Odyssey security baseline (authentication, authorization, audit, encryption).

## Assets

| Asset | Classification | Description |
|-------|----------------|-------------|
| Managed binary blobs | Confidential | Documents, drawings, certificates, evidence files |
| Document metadata & versions | Confidential | Classification, ACLs, retention, signatures |
| Maintenance work orders & costs | Confidential | Labor, parts, vendor costs, downtime data |
| Quality records (inspections, NCRs, CAPAs) | Confidential | Nonconformances, root cause, corrective actions |
| Electronic signatures | High Integrity | Legally relevant attestations |
| Retention/disposition records | High Integrity | Legal hold, disposition evidence, audit trail |

## Trust Boundaries

```
┌─────────────────────────────────────────────────────────────┐
│                    External Users                           │
│  (Customers, Suppliers, Employees via Portals)              │
└──────────────────────────┬──────────────────────────────────┘
                           │ HTTPS / mTLS
┌──────────────────────────▼──────────────────────────────────┐
│                    API Gateway / Load Balancer              │
│                    (Rate limit, WAF, TLS termination)       │
└──────────────────────────┬──────────────────────────────────┘
                           │ Internal network
┌──────────────────────────▼──────────────────────────────────┐
│                   Odyssey Application                        │
│  ┌────────────┐ ┌────────┐ ┌────────┐ ┌──────────────────┐  │
│  │  Documents │ │  CMMS  │ │  QMS   │ │  Existing Modules │  │
│  └────────────┘ └────────┘ └────────┘ └──────────────────┘  │
│           │          │          │              │             │
│           └──────────┼──────────┼──────────────┘             │
│                      ▼          ▼                             │
│            ┌─────────────────────────┐                        │
│            │   Shared Foundations    │                        │
│            │  Authz | Audit | Outbox │                        │
│            │  Approval | Storage     │                        │
│            └────────────┬────────────┘                        │
│                         │                                      │
└─────────────────────────┼──────────────────────────────────────┘
                          │
         ┌────────────────┼────────────────┐
         ▼                ▼                ▼
    ┌─────────┐      ┌──────────┐    ┌──────────┐
    │PostgreSQL│      │  S3/MinIO │    │  Redis   │
    │ (Metadata)│     │  (Blobs)  │    │ (Cache/  │
    │           │      │           │    │  Queue)  │
    └─────────┘      └──────────┘    └──────────┘
```

## STRIDE Analysis

### Spoofing

| Threat | Mitigation |
|--------|------------|
| Attacker impersonates another user to sign documents | Short-lived reauthentication challenges, server-generated canonical snapshots, challenge verification |
| Attacker forges maintenance work order completion | Optimistic locking, idempotency keys, RBAC on `cmms.work_order.execute`/`close` |
| Attacker spoofs quality inspection results | Server-side validation, immutable result snapshots, separation of duties on disposition |
| Service-to-service impersonation | mTLS between services, signed outbox events |

### Tampering

| Threat | Mitigation |
|--------|------------|
| Modify document binary after approval | SHA-256 checksums stored at version creation, immutable versions, object storage versioning |
| Alter retention expiry dates | Append-only audit events, retention calculated from policy-defined trigger events, legal holds block disposition |
| Change work order costs after approval | Exact `NUMERIC` costs, snapshot at completion, approval workflow for cost exceptions |
| Modify quality disposition after closure | Immutable finalized records, superseding versions for corrections, audit trail |
| SQL injection in dynamic queries | Parameterized queries only, sqlc-generated code, no dynamic SQL |

### Repudiation

| Threat | Mitigation |
|--------|------------|
| User denies signing a document | Electronic signature binds: record version, SHA-256 hash, meaning, signer, timestamp, policy version, auth method |
| Maintenance tech denies completing work | Immutable work order events, labor entries with approvals, completion readings |
| Quality engineer denies disposition decision | CAPA verification requires independent verifier, signatures on disposition, audit events with correlation IDs |
| System admin denies deleting a blob | Audit events for all deletions, disposition evidence required, metadata tombstones survive |

### Information Disclosure

| Threat | Mitigation |
|--------|------------|
| Unauthorized document download | Dual authorization: module permission + record-level ACL, classification defaults, explicit deny wins |
| Cross-company data access | Company-scoped tables, indexes, queries, storage keys, row-level security policies |
| Blob enumeration via predictable keys | Opaque storage keys (UUID), never derived from filenames, per-company deduplication only |
| Quality data leakage to suppliers | Supplier quality cases separate from commercial ratings, QMS publishes only quality component |
| Cost data exposure in CMMS | Separate `cmms.cost.view` and `cmms.cost.approve` permissions |

### Denial of Service

| Threat | Mitigation |
|--------|------------|
| Large file uploads exhaust storage | 10MB default limit (configurable), streaming uploads, quarantine before version creation |
| Malicious retention policies cause mass deletion | Retention changes affect future only unless explicit migration, legal holds block, admin approval for disposition |
| Preventive work order storm | Exactly-once generation worker, schedule limits, overdue visibility not auto-creation |
| Audit event table growth | Partitioning by time, retention policies on audit data, async outbox processing |
| Signature challenge replay | One-time use challenges with expiry, `used` flag, server-side validation |

### Elevation of Privilege

| Threat | Mitigation |
|--------|------------|
| Regular user gains `documents.admin` | Permissions seeded only to admin roles, explicit assignment required, RBAC middleware on all routes |
| CMMS tech approves own cost exception | Separation of duties: `cmms.work_order.execute` ≠ `cmms.cost.approve`, approval workflow |
| Quality inspector verifies own CAPA | CAPA verification requires independent verifier permission (`qms.capa.verify` ≠ `qms.capa.implement`) |
| Document reviewer approves own draft | Separation of duties enforced in approval engine, policy config |
| Storage service accessed directly | Document Management is only authorized disposer, storage interface internal |

## Data Flow Threats

### Document Upload Flow
```
User → API → Virus Scan → Type Validation → Checksum → Quarantine → Version Create → Storage
```
Threats: MIME spoofing (detected type vs declared), size bypass (streaming limit), path traversal (opaque keys), scan bypass (quarantine gate)

### Document Download Flow
```
User → API → Authz (perm + ACL) → Signed URL / Stream → Audit Event
```
Threats: ACL bypass (explicit deny wins, classification defaults), URL leakage (short-lived, single-use), audit gap (transactional write)

### Maintenance Work Order Flow
```
Request → Triage → Plan → Approval → Release → Execute → Complete → Close
                              ↓
                        Approval Engine
```
Threats: State bypass (optimistic version, service-layer validation), cost inflation (snapshot at completion, approval for exceptions), parts diversion (reservation → issue → return tracking)

### Quality Inspection Flow
```
Plan → Schedule → Execute → Result → Disposition → Close
                    ↓              ↓
               Hold (blocks)   NCR/CAPA
```
Threats: Hold bypass (MRP queries QMS before completion/receipt), disposition without approval (separation of duties), result tampering (immutable snapshot, versioned)

## Security Requirements

### Authentication & Authorization
- [ ] All routes protected by RBAC middleware
- [ ] Record-level ACL checked on every data access
- [ ] Explicit deny overrides all grants
- [ ] Separation of duties enforced via approval engine
- [ ] Per-company feature flags gate module access

### Data Protection
- [ ] Encryption at rest (S3 SSE, PostgreSQL TDE)
- [ ] Encryption in transit (mTLS internal, TLS external)
- [ ] SHA-256 checksums on all blobs at ingest
- [ ] Malware scanning before version availability
- [ ] No passwords or client-supplied auth evidence stored

### Audit & Accountability
- [ ] Immutable audit events for all lifecycle changes
- [ ] Correlation/causation IDs on cross-module events
- [ ] Electronic signatures: record version, hash, meaning, signer, timestamp, policy, auth method
- [ ] Metadata tombstones survive physical deletion
- [ ] Complete evidence export for compliance

### Resilience
- [ ] Idempotency keys on all state transitions
- [ ] Optimistic locking on versioned records
- [ ] Retry-safe outbox consumers
- [ ] Exactly-once preventive work generation
- [ ] Transaction rollback proves no partial inventory/accounting changes

## Validation Checklist (Phase 0)

- [ ] Threat model reviewed by security team
- [ ] ADRs document all integration boundaries
- [ ] Permission matrix complete for all three modules
- [ ] Migration rehearsal plan includes rollback procedures
- [ ] Storage extraction design reviewed for security
- [ ] Initial migrations include RLS policies

## References
- [ADR-001: Module Boundaries](./adr-001-module-boundaries.md)
- [Missing Modules Plan](../archive/completed-../archive/completed-missing-modules-cmms-qms-documents-plan.md)
- [OWASP ASVS 4.0](https://owasp.org/www-project-application-security-verification-standard/)