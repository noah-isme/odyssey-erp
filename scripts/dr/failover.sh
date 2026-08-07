#!/bin/bash
# Odyssey ERP - DR Automated Failover Script
# Pulls latest backups from S3, decrypts, and restores to the standby cluster

set -e

BACKUP_DIR="/var/backups/odyssey"
DB_CONTAINER="odyssey_db"

echo "Initiating Odyssey ERP Disaster Recovery Failover..."

if [ -z "$DR_ENCRYPTION_KEY" ]; then
    echo "ERROR: DR_ENCRYPTION_KEY is required"
    exit 1
fi

mkdir -p "$BACKUP_DIR"

# 1. Fetch latest encrypted backup
echo "Fetching latest backup from S3..."
LATEST_BACKUP=$(aws s3 ls s3://$DR_S3_BUCKET/backups/ | sort | tail -n 1 | awk '{print $4}')
if [ -z "$LATEST_BACKUP" ]; then
    echo "ERROR: No backups found in S3."
    exit 1
fi
aws s3 cp "s3://$DR_S3_BUCKET/backups/$LATEST_BACKUP" "$BACKUP_DIR/$LATEST_BACKUP"

# 2. Decrypt
echo "Decrypting backup..."
DECRYPTED_FILE="${LATEST_BACKUP%.enc}"
openssl enc -d -aes-256-cbc -in "$BACKUP_DIR/$LATEST_BACKUP" -out "$BACKUP_DIR/$DECRYPTED_FILE" -pass pass:$DR_ENCRYPTION_KEY

# 3. Restore PostgreSQL
echo "Restoring Database..."
gunzip -c "$BACKUP_DIR/$DECRYPTED_FILE" | docker exec -i $DB_CONTAINER psql -U postgres

# Cleanup
rm "$BACKUP_DIR/$LATEST_BACKUP" "$BACKUP_DIR/$DECRYPTED_FILE"

echo "Failover restoration complete! Please verify data integrity."
