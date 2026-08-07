# Regulated Manufacturing Compliance (21 CFR Part 11)

## Overview
Odyssey ERP supports deployment in regulated environments such as pharmaceuticals, medical devices, and food & beverage manufacturing. To comply with FDA **21 CFR Part 11** (Electronic Records; Electronic Signatures), the system implements strict controls around audit trails and non-repudiation.

## Electronic Signatures
To ensure non-repudiation, electronic signatures are required for critical manufacturing actions (e.g., releasing a batch, approving a formulation, overriding a quality hold).

### Requirements
1. **Re-authentication:** The user must explicitly re-enter their password or a TOTP MFA token at the exact moment of signing.
2. **Intent:** The signature must capture the exact intent (e.g., "Approved", "Authored", "Reviewed").
3. **Immutability:** Signatures are immutably tied to the exact version of the record (AggregateVersion) and stored in `compliance_esignatures`.

### Implementation (Application Layer)
Use `shared.ESignatureManager` during a domain transaction:

```go
func (s *Service) ApproveBatchRecord(ctx context.Context, batchID string, userID int64, password string) error {
	return s.db.BeginTx(ctx, func(tx pgx.Tx) error {
		// 1. Business Logic
		batch := getBatch(tx, batchID)
		batch.Approve()
		
		// 2. Electronic Signature
		err := s.esignManager.VerifyAndSign(ctx, tx, shared.ElectronicSignature{
			CompanyID: 1,
			ActorID:   userID,
			Entity:    "ManufacturingBatch",
			EntityID:  batchID,
			Intent:    "Approved",
		}, func() bool {
			// Validate password/MFA synchronously
			return s.authService.VerifyPassword(userID, password)
		})
		
		return err
	})
}
```

## Audit Trails
All state transitions in the system are logged to `audit_logs` using the `shared.AuditLogger`. These logs capture:
- The exact user (`actor_id`)
- The action taken (`action`)
- The affected entity (`entity`, `entity_id`)
- Contextual metadata (`meta`)

**Tamper Evidence:** In regulated deployments, `audit_logs` are periodically hashed (Merkle Tree approach) and the root hash is published to an external immutable ledger or WORM (Write-Once-Read-Many) storage like AWS S3 Object Lock.
