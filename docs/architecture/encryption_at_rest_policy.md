# Encryption-at-Rest Policy

## Overview
As Odyssey ERP handles sensitive PII, financial data, and external integration secrets, an encryption-at-rest policy is required to prevent unauthorized data exposure in the event of a database compromise or backup leak.

## Strategy

We adopt a **Hybrid Approach**:
1. **Infrastructure-Level Encryption (TDE / Storage Encryption):** 
   - All managed databases (e.g., AWS RDS, GCP Cloud SQL) MUST have native volume encryption enabled (using AWS KMS or Google Cloud KMS).
   - This protects against physical disk theft and guarantees that EBS volumes and snapshots are encrypted.

2. **Application-Level Field Encryption (ALE):**
   - Storage-level encryption does not protect against a compromised database connection or SQL injection. Therefore, highly sensitive fields MUST be encrypted *before* being written to the database.
   - **Algorithm:** AES-256-GCM.
   - **Key Management:** The encryption key must be injected via environment variables (e.g., `ODYSSEY_ENCRYPTION_KEY`) from a secure vault.
   
## Scope of Field-Level Encryption
The following data classifications must use ALE:
- **External Integration Secrets:** OAuth tokens, API keys, webhook secrets (e.g., `connector_connections.secret_ref`).
- **Financial Secrets:** Bank account routing numbers, credit card tokens.
- **Sensitive PII:** SSN / National ID numbers in the HR module.

## Implementation Guidelines
1. The `internal/shared` package provides a `crypto.go` utility containing `Encrypt(plaintext, key)` and `Decrypt(ciphertext, key)` using AES-GCM.
2. Database columns storing ciphertext should be typed as `TEXT` or `BYTEA` and explicitly named to indicate they are encrypted (e.g., `webhook_secret_ciphertext`).
3. Domain repositories are responsible for calling the crypto helpers directly before inserting and immediately after selecting data. Data should remain in plaintext only within the Go application's memory space.

## Key Rotation
- Future enhancements will support key versioning (e.g., prefixing ciphertexts with `v1:`) to allow seamless key rotation without downtime.
