#!/bin/bash
# Odyssey ERP - Automated DR Backup Script
# Performs encrypted backups of PostgreSQL and Redis state, and pushes to remote S3

set -e

BACKUP_DIR="/var/backups/odyssey"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
DB_CONTAINER="odyssey_db"
REDIS_CONTAINER="odyssey_redis"

echo "Starting Odyssey ERP Disaster Recovery Backup at $TIMESTAMP..."

mkdir -p "$BACKUP_DIR"

# 1. Backup PostgreSQL
echo "Backing up PostgreSQL..."
docker exec $DB_CONTAINER pg_dumpall -U postgres | gzip > "$BACKUP_DIR/db_$TIMESTAMP.sql.gz"

# 2. Backup Redis (optional if persistence is critical for jobs)
echo "Backing up Redis dump.rdb..."
docker cp $REDIS_CONTAINER:/data/dump.rdb "$BACKUP_DIR/redis_$TIMESTAMP.rdb" || echo "Redis backup skipped."

# 3. Encrypt backups using AES-256 (symmetric)
echo "Encrypting backups at rest..."
openssl enc -aes-256-cbc -salt -in "$BACKUP_DIR/db_$TIMESTAMP.sql.gz" -out "$BACKUP_DIR/db_$TIMESTAMP.sql.gz.enc" -pass pass:$DR_ENCRYPTION_KEY
openssl enc -aes-256-cbc -salt -in "$BACKUP_DIR/redis_$TIMESTAMP.rdb" -out "$BACKUP_DIR/redis_$TIMESTAMP.rdb.enc" -pass pass:$DR_ENCRYPTION_KEY

# Cleanup unencrypted files
rm "$BACKUP_DIR/db_$TIMESTAMP.sql.gz" "$BACKUP_DIR/redis_$TIMESTAMP.rdb"

# 4. Push to remote storage (e.g. AWS S3)
echo "Syncing encrypted backups to offsite DR storage..."
if [ -x "$(command -v aws)" ]; then
    aws s3 cp "$BACKUP_DIR/" s3://$DR_S3_BUCKET/backups/ --recursive --exclude "*" --include "*.enc"
else
    echo "Warning: AWS CLI not found, local backups stored at $BACKUP_DIR"
fi

echo "DR Backup complete."
