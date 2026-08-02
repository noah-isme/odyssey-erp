# ODYSSEY ERP: PRODUCTION DEPLOYMENT GUIDE

**Version:** 1.0.0  
**Date:** 2026-08-02  
**Status:** Production Ready  

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

Create `.env.production`:

```bash
# Application
ENVIRONMENT=production
PORT=8080
LOG_LEVEL=info
ENABLE_PROFILING=false

# Database
DB_DSN=postgres://user:password@db-host:5432/odyssey?sslmode=require
DB_MAX_CONNECTIONS=25
DB_CONN_TIMEOUT=30s

# Session Management
SESSION_TIMEOUT=1h
SESSION_SECURE_COOKIE=true
SESSION_SAME_SITE=Strict

# TLS/HTTPS
TLS_ENABLED=true
TLS_CERT_FILE=/etc/odyssey/certs/server.crt
TLS_KEY_FILE=/etc/odyssey/certs/server.key

# Cache Configuration
CACHE_ENABLED=true
CACHE_TTL_SHORT=15m
CACHE_TTL_MEDIUM=1h
CACHE_TTL_LONG=6h

# Monitoring
METRICS_ENABLED=true
TRACES_ENABLED=true
JAEGER_ENDPOINT=http://jaeger:14268/api/traces

# Security
CORS_ALLOWED_ORIGINS=https://app.odyssey.com
CSRF_TOKEN_LENGTH=32
RATE_LIMIT_REQUESTS=100
RATE_LIMIT_WINDOW=1m
```

### Configuration File

`config/production.yaml`:

```yaml
app:
  name: Odyssey ERP
  environment: production
  version: 1.0.0

database:
  driver: postgres
  connection:
    host: db-host
    port: 5432
    database: odyssey
    ssl: require
  pool:
    min_connections: 5
    max_connections: 25
    idle_timeout: 5m
    lifetime: 30m
  query:
    timeout: 30s
    slow_query_threshold: 100ms

server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s
  max_header_bytes: 1048576

tls:
  enabled: true
  cert_file: /etc/odyssey/certs/server.crt
  key_file: /etc/odyssey/certs/server.key
  min_version: "1.2"

logging:
  level: info
  format: json
  output: stdout

cache:
  enabled: true
  ttl:
    short: 15m
    medium: 1h
    long: 6h
  max_size: 1000

security:
  session_timeout: 1h
  secure_cookie: true
  csrf_protection: true
  rate_limiting: true
```

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

# Verify migration status
make migrate-status

# Expected output: All migrations applied successfully
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

### 1. Build Binary

```bash
# Build for production
make build

# Verify binary
./odyssey-erp version

# Expected: Version 1.0.0 (production)
```

### 2. Prepare Application Directory

```bash
# Create application directories
mkdir -p /opt/odyssey/{bin,config,certs,logs}

# Copy binary
cp odyssey-erp /opt/odyssey/bin/
chmod +x /opt/odyssey/bin/odyssey-erp

# Copy configuration
cp config/production.yaml /opt/odyssey/config/
cp .env.production /opt/odyssey/

# Copy TLS certificates
cp /path/to/certs/* /opt/odyssey/certs/
chmod 600 /opt/odyssey/certs/*

# Set permissions
chown -R odyssey:odyssey /opt/odyssey
chmod 755 /opt/odyssey/bin/odyssey-erp
```

### 3. Create Systemd Service

Create `/etc/systemd/system/odyssey.service`:

```ini
[Unit]
Description=Odyssey ERP Application
After=network.target postgresql.service
Requires=postgresql.service

[Service]
Type=simple
User=odyssey
Group=odyssey
WorkingDirectory=/opt/odyssey

# Load environment
EnvironmentFile=/opt/odyssey/.env.production

# Start command
ExecStart=/opt/odyssey/bin/odyssey-erp \
  -config /opt/odyssey/config/production.yaml

# Restart policy
Restart=on-failure
RestartSec=10
StartLimitInterval=60
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

### 4. Start Service

```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable on boot
sudo systemctl enable odyssey

# Start service
sudo systemctl start odyssey

# Check status
sudo systemctl status odyssey

# View logs
sudo journalctl -u odyssey -f

# Expected: Service started successfully, listening on :8080
```

### 5. Configure Reverse Proxy (Nginx)

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
curl -s https://app.odyssey.com/health | jq .

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
sudo journalctl -u odyssey -n 100

# Tail logs in real-time
sudo journalctl -u odyssey -f

# Filter by log level
sudo journalctl -u odyssey --priority err

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
# Stop current version
sudo systemctl stop odyssey

# Restore previous binary
cp /opt/odyssey/backups/odyssey-erp.previous /opt/odyssey/bin/odyssey-erp

# Start previous version
sudo systemctl start odyssey

# Verify health
curl -s https://app.odyssey.com/health
```

### Database Rollback (if migrations failed)

```bash
# List applied migrations
make migrate-status

# Rollback specific migration
make migrate-down steps=1

# Verify state
psql $PG_DSN -c "\d"
```

### Full Rollback to Previous Release

```bash
# Check backup availability
ls -la /opt/odyssey/backups/

# Restore database from backup (if needed)
pg_restore -d odyssey /opt/odyssey/backups/odyssey.sql.gz

# Restore application binary and config
rm -rf /opt/odyssey/bin /opt/odyssey/config
cp -r /opt/odyssey/backups/previous/* /opt/odyssey/

# Restart
sudo systemctl restart odyssey
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
2. **Verify health:** `curl https://app.odyssey.com/health`
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

**Deployment Status: READY FOR PRODUCTION** ✅
