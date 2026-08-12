# ODYSSEY ERP: PRODUCTION DEPLOYMENT GUIDE

**Version:** 1.1.0
**Date:** 2026-08-12
**Deployment target:** Self-managed VPS
**Status:** Production runbook; operator sign-off required

For the bounded v0.10.0 release, use the [v0.10-core staging certification
record](releases/v0.10-core-staging-certification.md) before promotion. The
release gate requires an explicit `RELEASE_PROFILE`; production promotion uses
`RELEASE_PROFILE=v0.10-core` unless a separately approved `full` profile has
certified every matrix row.

The cumulative `v0.11-finance` profile is documented for the next finance
automation certification cycle, but it is not production-certified and must not
be used for promotion until its matrix evidence is complete.

> This runbook is production-only. The `staging` branch deploys through the
> isolated staging contract in [STAGING_DEPLOYMENT.md](STAGING_DEPLOYMENT.md);
> it does not use the production host, paths, services, or secrets described
> below.

---

## Table of Contents

1. [Pre-Deployment Checklist](#pre-deployment-checklist)
2. [Environment Configuration](#environment-configuration)
3. [Database Setup](#database-setup)
4. [Deployment Steps](#deployment-steps)
5. [Health Checks](#health-checks)
6. [Monitoring and Logging](#monitoring-and-logging)
7. [Rollback Procedures](#rollback-procedures)
8. [Performance Tuning](#performance-tuning)
9. [Security Hardening](#security-hardening)

---

## Pre-Deployment Checklist

### Infrastructure Requirements
- [ ] **Web Server:** 2+ CPU cores, 4GB+ RAM
- [ ] **Database Server:** Dedicated PostgreSQL 13+, 8GB+ RAM
- [ ] **Load Balancer:** Optional but recommended (HAProxy/AWS ALB)
- [ ] **Storage:** 50GB+ SSD for database, 10GB+ for application
- [ ] **Network:** TLS/SSL certificates, outbound HTTPS access

### Code Review
- [ ] All commits reviewed and tested
- [ ] Build passes without warnings
- [ ] Unit tests passing (100% success)
- [ ] Integration tests passing
- [ ] E2E tests passing on target environment
- [ ] Security scan completed (no critical issues)
- [ ] Performance benchmark baseline established

### Documentation
- [ ] Runbooks created for common operations
- [ ] Disaster recovery plan documented
- [ ] Team training completed
- [ ] Incident response procedures defined

---

## Environment Configuration

### Environment Variables

Create `/opt/odyssey/.env` on the VPS. This file stays outside release
directories, is never committed, and is sourced by both systemd services and
the native deployment workflow:

```bash
# Application
APP_ENV=production
RELEASE_PROFILE=v0.10-core
APP_ADDR=:8080
LOG_FORMAT=json

# Database
PG_DSN=postgres://user:password@db-host:5432/odyssey?sslmode=require

# Cache / job queue
REDIS_ADDR=redis-host:6379

# Session Management
SESSION_SECRET=replace-with-a-long-random-secret
SESSION_TTL=720h
CSRF_SECRET=replace-with-a-different-long-random-secret

# Production connector policy and PDF service
CONNECTORS_DEVELOPMENT_MODE=false
GOTENBERG_URL=http://127.0.0.1:3000
```

The application reads runtime configuration from environment variables through
`internal/app/config.go`; there is no checked-in `config/production.yaml` to
copy into a release. Set `SESSION_SECRET` and `CSRF_SECRET` to unique secret
values and keep `CONNECTORS_DEVELOPMENT_MODE=false` in production. Accepted
release profiles are `v0.10-core`, `v0.11-finance`, and `full`; do not omit the
profile or invent another value. `v0.11-finance` is not currently certified for
production. The selected profile must match the scope and evidence recorded in
the [authoritative feature matrix](reference/feature-matrix.md).

---

## Database Setup

### 1. Create Database

```sql
CREATE DATABASE odyssey
  OWNER odyssey_user
  ENCODING 'UTF8'
  LC_COLLATE 'en_US.UTF-8'
  LC_CTYPE 'en_US.UTF-8';

-- Create application user
CREATE ROLE odyssey_user WITH
  LOGIN
  PASSWORD 'secure_password_here'
  NOSUPERUSER
  NOCREATEDB
  NOCREATEROLE
  NOINHERIT;

-- Grant permissions
GRANT CONNECT ON DATABASE odyssey TO odyssey_user;
GRANT USAGE ON SCHEMA public TO odyssey_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO odyssey_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO odyssey_user;
```

### 2. Run Migrations

```bash
# Connect to production database
export PG_DSN="postgres://odyssey_user:password@db-host:5432/odyssey?sslmode=require"

# Run all migrations
make migrate-up

# Verify the current migration version; this fails if the database is unreachable
make migrate-status

# Run the repository's migration safety tests before a staging rehearsal
make test-migrate

# Expected output: the latest migration version and passing migration tests
```

### 3. Create Indexes

```bash
# Automatic index creation via migrations
# Manual verification:
psql $PG_DSN -c "SELECT * FROM pg_indexes WHERE schemaname = 'public';"

# Should show indexes on:
# - loads (company_id, status)
# - delivery_routes (load_id, status)
# - route_stops (route_id, stop_sequence)
# - transfer_orders (company_id, status)
# - supplier_contracts (supplier_id, status)
# etc.
```

### 4. Load Initial Data

```bash
# Create default master data
make seed-production

# Verify data loaded
psql $PG_DSN -c "SELECT COUNT(*) as company_count FROM companies;"
psql $PG_DSN -c "SELECT COUNT(*) as warehouse_count FROM warehouses;"
```

---

## Deployment Steps

### 1. Native release layout

Production uses the following native release layout. The
`.github/workflows/deploy-native.yml` file on the `staging` branch is
intentionally a staging workflow; follow
[STAGING_DEPLOYMENT.md](STAGING_DEPLOYMENT.md) for that branch. Do not point
the staging workflow at the production paths below.

```bash
sudo install -d -o odyssey -g odyssey -m 0755 /opt/odyssey/releases
sudo install -d -o odyssey -g odyssey -m 0755 /opt/odyssey
sudo install -o odyssey -g odyssey -m 0600 /dev/null /opt/odyssey/.env
```

Each release is uploaded to `/opt/odyssey/releases/<short-sha>` and contains
`odyssey`, `worker`, `bootstrap-admin`, `migrate`, `migrations/`, `web/`, and
checksum/revision metadata. The workflow switches the atomic
`/opt/odyssey/current` symlink only after migrations and checksum verification.
Configure these repository secrets before enabling the workflow:
`PRODUCTION_HOST`, `PRODUCTION_USER`, and `PRODUCTION_SSH_KEY`. Configure a
GitHub `production` environment with required reviewers if deployment approval
is needed. `PRODUCTION_USER` must be able to write the release/current paths
and run the two systemd commands with passwordless `sudo -n`.

### 2. Create systemd services

Create `/etc/systemd/system/odyssey.service`:

```ini
[Unit]
Description=Odyssey ERP Application
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=odyssey
Group=odyssey
WorkingDirectory=/opt/odyssey/current

# Load environment
EnvironmentFile=/opt/odyssey/.env

# Start command
ExecStart=/opt/odyssey/current/odyssey

# Restart policy
Restart=on-failure
RestartSec=10
StartLimitIntervalSec=60
StartLimitBurst=3

# Process management
KillMode=process
KillSignal=SIGTERM
TimeoutStopSec=30

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=odyssey

# Security
PrivateTmp=yes
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

Create `/etc/systemd/system/odyssey-worker.service` with the same unit policy
and environment, but run the background worker:

```ini
[Unit]
Description=Odyssey ERP Worker
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=odyssey
Group=odyssey
WorkingDirectory=/opt/odyssey/current
EnvironmentFile=/opt/odyssey/.env
ExecStart=/opt/odyssey/current/worker
Restart=on-failure
RestartSec=10
StartLimitIntervalSec=60
StartLimitBurst=3
KillMode=process
KillSignal=SIGTERM
TimeoutStopSec=30
StandardOutput=journal
StandardError=journal
SyslogIdentifier=odyssey-worker
PrivateTmp=yes
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

### 3. Enable services

```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable and start both processes
sudo systemctl enable --now odyssey.service odyssey-worker.service

# Check status
sudo systemctl status odyssey.service odyssey-worker.service

# View logs
sudo journalctl -u odyssey.service -u odyssey-worker.service -f

# Expected: Service started successfully, listening on :8080
```

### 4. Configure Reverse Proxy (Nginx)

Create `/etc/nginx/sites-available/odyssey`:

```nginx
upstream odyssey {
    server localhost:8080;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name app.odyssey.com;

    # SSL configuration
    ssl_certificate /etc/letsencrypt/live/app.odyssey.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/app.odyssey.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;

    # Security headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options SAMEORIGIN always;
    add_header X-Content-Type-Options nosniff always;
    add_header X-XSS-Protection "1; mode=block" always;

    # Compression
    gzip on;
    gzip_types text/plain text/css application/json application/javascript;
    gzip_min_length 1000;

    # Proxy settings
    location / {
        proxy_pass http://odyssey;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;

        # Timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    # Static assets (cache longer)
    location ~* \.(css|js|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        proxy_pass http://odyssey;
        proxy_cache_valid 200 1d;
        add_header Cache-Control "public, max-age=86400";
    }
}

# Redirect HTTP to HTTPS
server {
    listen 80;
    server_name app.odyssey.com;
    return 301 https://$server_name$request_uri;
}
```

Enable and test:

```bash
sudo ln -s /etc/nginx/sites-available/odyssey /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

---

## Health Checks

### 1. Application Health

```bash
# Basic health check
curl -s https://app.odyssey.com/healthz | jq .

# Expected response:
# {
#   "status": "healthy",
#   "version": "1.0.0",
#   "timestamp": "2026-08-02T19:30:00Z"
# }
```

### 2. Database Connectivity

```bash
# Check database connection
curl -s https://app.odyssey.com/health/db | jq .

# Expected: "database": "connected"
```

### 3. Service Verification

```bash
# Login test
curl -X POST https://app.odyssey.com/login \
  -d "email=admin@odyssey.local&password=password"

# List contracts (basic API test)
curl -H "Authorization: Bearer $TOKEN" \
  https://app.odyssey.com/api/procurement/contracts

# Expected: 200 OK with JSON response
```

---

## Monitoring and Logging

### 1. Logging Setup

```bash
# View application logs
sudo journalctl -u odyssey.service -u odyssey-worker.service -n 100

# Tail logs in real-time
sudo journalctl -u odyssey.service -u odyssey-worker.service -f

# Filter by log level
sudo journalctl -u odyssey.service -u odyssey-worker.service --priority err

# Set retention
echo "SystemMaxUse=1G" | sudo tee -a /etc/systemd/journald.conf
sudo systemctl restart systemd-journald
```

### 2. Metrics Collection

Enable in application:

```bash
# Access metrics endpoint
curl -s https://app.odyssey.com/metrics

# Prometheus format output with:
# - HTTP request counts
# - Response time histogram
# - Database query times
# - Cache hit ratio
# - Active connections

# The worker exposes reconciliation and connector recovery counters separately.
curl -s http://127.0.0.1:9091/metrics
```

### 3. Alerting Rules

Configure alerts for:

```yaml
# High error rate (>1% in 5m)
alert: HighErrorRate
expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.01

# Slow response time (p95 > 500ms)
alert: SlowResponseTime
expr: histogram_quantile(0.95, http_request_duration_seconds) > 0.5

# Database connection pool exhausted
alert: DBConnectionPoolExhausted
expr: db_connections_active / db_connections_max > 0.9

# Cache hit ratio too low (<70%)
alert: LowCacheHitRatio
expr: cache_hit_ratio < 0.7
```

---

## Rollback Procedures

### Quick Rollback (< 5 minutes)

```bash
# Point the current symlink at a previously verified release
sudo ln -sfn /opt/odyssey/releases/<previous-short-sha> /opt/odyssey/current

# Restart both processes
sudo systemctl restart odyssey.service odyssey-worker.service

# Verify health
curl -fsS https://app.odyssey.com/healthz
```

### Database Rollback (if migrations failed)

```bash
# List applied migrations
make migrate-status

# Rollback specific migration
MIGRATION_STEPS=1 make migrate-down

# MIGRATION_STEPS defaults to 1; increase it only after reviewing the recovery plan

# Verify state
psql $PG_DSN -c "\d"
```

### Full Rollback to Previous Release

```bash
# Check backup availability
ls -la /opt/odyssey/backups/

# Restore database from backup (if needed)
pg_restore -d odyssey /opt/odyssey/backups/odyssey.sql.gz

# Restore the application symlink and restart both processes
sudo ln -sfn /opt/odyssey/releases/<previous-short-sha> /opt/odyssey/current

# Restart
sudo systemctl restart odyssey.service odyssey-worker.service
```

---

## Performance Tuning

### 1. Database Tuning

```sql
-- Adjust PostgreSQL settings for production
ALTER SYSTEM SET shared_buffers = '4GB';
ALTER SYSTEM SET effective_cache_size = '12GB';
ALTER SYSTEM SET maintenance_work_mem = '1GB';
ALTER SYSTEM SET checkpoint_completion_target = 0.9;
ALTER SYSTEM SET wal_buffers = '16MB';
ALTER SYSTEM SET default_statistics_target = 100;

-- Apply changes
SELECT pg_reload_conf();
```

### 2. Connection Pool Optimization

```yaml
# Update config
database:
  pool:
    min_connections: 10
    max_connections: 50
    idle_timeout: 10m
    lifetime: 30m
```

### 3. Cache Configuration

```bash
# Monitor cache performance
curl -s https://app.odyssey.com/metrics/cache

# If hit ratio < 70%, increase TTL values
# If memory usage high, reduce TTL or max_size
```

---

## Security Hardening

### 1. Enable RBAC Enforcement

Verify permissions are enforced:

```bash
# Test permission denied
curl -H "Authorization: Bearer $READONLY_TOKEN" \
  -X POST https://app.odyssey.com/api/procurement/contracts

# Expected: 403 Forbidden
```

### 2. Audit Logging

Enable and monitor:

```bash
# View audit logs
sudo journalctl -u odyssey | grep "permission_check"

# Expected: Logs show who accessed what, when
```

### 3. Secrets Management

```bash
# Ensure secrets are not in git
git log --all --source --remotes | grep -i password

# Store in environment variables or secrets manager
# Never commit: passwords, keys, tokens

# Rotate credentials quarterly
# Update in: .env, database, SSL certs
```

---

## Production Support

### Common Operations

```bash
# View live metrics
curl -s https://app.odyssey.com/metrics

# Create database backup
pg_dump odyssey | gzip > /backups/odyssey_$(date +%Y%m%d).sql.gz

# Check system resources
free -h
df -h
top -b -n 1

# Monitor active connections
psql -c "SELECT count(*) FROM pg_stat_activity WHERE state = 'active';"
```

### Incident Response

1. **Check logs:** `sudo journalctl -u odyssey -f`
2. **Verify health:** `curl https://app.odyssey.com/healthz`
3. **Check database:** `psql $PG_DSN -c "SELECT 1"`
4. **Restart if needed:** `sudo systemctl restart odyssey`
5. **Document in runbook**

---

## Post-Deployment Validation

### 1-Hour After Launch

- [ ] Application responding to requests
- [ ] No error spike in logs
- [ ] Database performing normally
- [ ] Cache hit ratio > 70%
- [ ] Response times < 200ms (p95)
- [ ] All health checks passing
- [ ] User logins working
- [ ] API endpoints responding

### First Week

- [ ] Monitor error rates (target: <0.1%)
- [ ] Collect performance baselines
- [ ] Verify backups working
- [ ] Test rollback procedure
- [ ] Gather user feedback
- [ ] Review security logs

---

## Support and Escalation

**On-Call:** [Contact info]  
**Emergency Hotline:** [Phone]  
**Incident Channel:** #odyssey-incidents  
**Documentation:** /docs/runbooks  

---

**Deployment Status:** VPS runbook; operator sign-off and production evidence required.
