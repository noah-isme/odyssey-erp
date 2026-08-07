#!/bin/bash
# disaster_recovery_backup.sh
# 
# This script performs a full database backup of Odyssey ERP
# and synchronizes it to a designated secure S3 bucket.
#
# Requirements:
# - pg_dump (PostgreSQL client)
# - aws-cli configured with appropriate IAM permissions

set -euo pipefail

# Configuration
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-odyssey}"
DB_NAME="${DB_NAME:-odyssey}"
S3_BUCKET="${S3_BUCKET:-s3://odyssey-erp-backups}"
BACKUP_DIR="${BACKUP_DIR:-/tmp/odyssey_backups}"
DATE=$(date +"%Y%m%d_%H%M%S")
BACKUP_FILE="${BACKUP_DIR}/odyssey_db_${DATE}.sql.gz"

echo "Starting Disaster Recovery Backup at $(date)"

# Ensure backup directory exists
mkdir -p "${BACKUP_DIR}"

# 1. Perform database dump
echo "Dumping database ${DB_NAME} to ${BACKUP_FILE}..."
# Note: PGPASSWORD should be set in the environment
pg_dump -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -F c -Z 9 -f "${BACKUP_FILE}"

# 2. Upload to S3
echo "Uploading to ${S3_BUCKET}..."
aws s3 cp "${BACKUP_FILE}" "${S3_BUCKET}/db_backups/odyssey_db_${DATE}.sql.gz" \
  --storage-class STANDARD_IA \
  --server-side-encryption AES256

# 3. Cleanup local backup
echo "Cleaning up local file ${BACKUP_FILE}..."
rm "${BACKUP_FILE}"

echo "Disaster Recovery Backup completed successfully at $(date)."
