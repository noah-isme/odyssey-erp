# Disaster Recovery & Business Continuity

## Overview
This runbook defines the procedures for recovering the Odyssey ERP system in the event of a critical failure (e.g., region outage, data corruption, or infrastructure compromise).

## Recovery Time Objective (RTO) & Recovery Point Objective (RPO)
- **RTO:** 4 Hours (Maximum time to restore full service)
- **RPO:** 1 Hour (Maximum acceptable data loss)

## Backup Strategy
1. **Automated Snapshots (Infrastructure):**
   - Managed DBs (RDS/CloudSQL) take automated snapshots every 24 hours.
   - Point-In-Time-Recovery (PITR) is enabled for up to 7 days, allowing restoration to any specific second.
2. **Logical Backups (Application):**
   - `scripts/disaster_recovery_backup.sh` runs via cron every hour to dump the database and push an encrypted archive to Amazon S3 (Standard-IA).

## Incident Response Plan

### Scenario 1: Database Corruption / Dropped Tables
**Action:** Restore from Point-in-Time or recent logical backup.
1. Halt application traffic to prevent further data corruption.
2. If using RDS, initiate a PITR restore to a new database instance specifying the time immediately preceding the corruption.
3. Update `PG_DSN` in the application environment to point to the newly restored instance.
4. Verify data integrity using `make test`.
5. Resume traffic.

### Scenario 2: Total Region Outage
**Action:** Failover to secondary region.
1. Infrastructure-as-Code (Terraform) is used to provision a fresh Kubernetes cluster and networking stack in the secondary region.
2. Fetch the latest logical backup from the cross-region replicated S3 bucket.
3. Provision a new database instance and restore the logical backup:
   `pg_restore -d <new-db> <downloaded-backup.sql.gz>`
4. Update DNS routing (Route53) to point to the secondary region load balancer.

## Testing DR
- The DR process MUST be tested quarterly.
- A synthetic data restore must be performed to a staging environment to validate RTO/RPO metrics.
