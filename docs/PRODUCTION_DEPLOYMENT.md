# ODYSSEY ERP: PRODUCTION DEPLOYMENT GUIDE

**Status:** Production Ready  
**Last Updated:** 2026-08-02 19:24 UTC  
**Phases:** 1-5 Complete (75% Procurement-Logistics)

---

## Pre-Deployment Checklist

### Infrastructure Requirements

- [ ] PostgreSQL 14+ (production instance)
- [ ] Redis 7+ (caching, sessions)
- [ ] Go 1.21+ runtime
- [ ] Docker/Kubernetes (optional, for containerization)
- [ ] Load balancer (for HA)
- [ ] Monitoring stack (Prometheus, Grafana)
- [ ] Log aggregation (ELK or similar)

### Security Prerequisites

- [ ] SSL/TLS certificates (domain)
- [ ] CORS configuration
- [ ] Rate limiting configured
- [ ] RBAC roles and permissions initialized
- [ ] Admin user account created
- [ ] Audit logging enabled
- [ ] `APP_MASTER_KEY` provisioned from a secret manager (never committed to configuration)
- [ ] `CONNECTORS_DEVELOPMENT_MODE=false` (or unset)
- [ ] Provider `Connection.SecretRef` values created through the vault-backed administration flow

### Database Setup

```bash
# Create production database
createdb odyssey_erp_prod

# Run migrations
make migrate-up ENV=prod

# Seed initial data (optional)
make seed ENV=prod
```

### Environment Configuration

Create `.env.prod`:

```bash
# Server
APP_ENV=production
APP_PORT=8080
APP_HOST=0.0.0.0

# Database
PG_DSN=postgresql://user:pass@prod-db:5432/odyssey_erp_prod
PG_MAX_CONN=50
PG_MIN_CONN=10

# Redis
REDIS_ADDR=prod-redis:6379
REDIS_PASSWORD=secure_password

# Auth
SESSION_KEY=<generate_secure_key>
CSRF_KEY=<generate_secure_key>

# Security
SSL_REDIRECT=true
SECURE_COOKIES=true
```

---

## Deployment Steps

### 1. Build Production Binary

```bash
make build ENV=prod
# Outputs: ./bin/odyssey (HTTP server), ./bin/worker (Asynq jobs)
```

### 2. Health Checks

```bash
# Before deployment, verify:
./bin/odyssey --version
./bin/odyssey --health-check
```

### 3. Database Validation

```bash
# Run migrations
./bin/odyssey migrate-up

# Verify schema
./bin/odyssey migrate-status

# Run tests
make test ENV=prod
```

### 4. Start Services

```bash
# Option A: Systemd (Linux)
systemctl start odyssey-web
systemctl start odyssey-worker

# Option B: Docker Compose
docker-compose -f docker-compose.prod.yml up -d

# Option C: Kubernetes
kubectl apply -f k8s/prod/
```

### 5. Verify Deployment

```bash
# Health check
curl https://api.odyssey.example.com/health

# API connectivity
curl -H "Authorization: Bearer TOKEN" https://api.odyssey.example.com/api/dashboard

# Check logs
journalctl -u odyssey-web -f
docker logs -f odyssey-web
```

---

## Performance Optimization

### Database Optimization

**Query Indexes** (already in migrations):
- company_id on all main tables
- status columns for filtering
- created_at for time-range queries
- Foreign keys for joins

**Connection Pooling** (configured):
```go
// pgxpool.Config
MaxConns:             50
MinConns:             10
AcquireTimeout:       30s
IdleInTransactionSessionTimeout: 5m
```

**Query Optimization**:
```sql
-- Use indexed columns
SELECT * FROM contracts WHERE company_id = $1 AND status = $2 ORDER BY created_at DESC
-- Uses: idx_contracts_company_status, idx_contracts_created_at

-- Pagination for large result sets
SELECT * FROM loads LIMIT 50 OFFSET (page-1)*50
```

### Caching Strategy

**Redis Caching** (implemented):
```go
// Cache permission checks (5 min TTL)
permissionKey := fmt.Sprintf("perms:%s:%s", userID, permission)
cache.Set(permissionKey, "true", 5*time.Minute)

// Cache planning rules (hourly)
rulesKey := fmt.Sprintf("rules:%d", warehouseID)
cache.Set(rulesKey, rules, 1*time.Hour)
```

**HTTP Caching**:
```go
// Cache GET responses (public data)
w.Header().Set("Cache-Control", "public, max-age=300")

// No cache for sensitive data
w.Header().Set("Cache-Control", "no-store, no-cache")
```

### Rate Limiting

**Configured limits**:
- 100 requests/minute per IP
- 1000 requests/minute per authenticated user
- 10 requests/second for batch operations

```go
httprate.Limit(100, time.Minute)
```

### Monitoring

**Metrics** (Prometheus):
- Request latency (p50, p95, p99)
- Error rates by endpoint
- Database connection pool usage
- Cache hit/miss rates
- Queue depth (Asynq jobs)
- Payment reconciliation, refund recovery, and connector dead-letter counters at
  the worker metrics endpoint (`WORKER_METRICS_ADDR`, default `:9091`)

**Health Checks**:
- Database connectivity
- Redis connectivity
- Disk space
- Memory usage

---

## Operational Procedures

### Database Migrations

```bash
# Before deploying schema changes
./bin/odyssey migrate-create add_new_table

# Verify migration works on staging
make test-migrate ENV=staging

# Deploy to production
./bin/odyssey migrate-up --force
```

### Backups

```bash
# Daily automated backups
pg_dump odyssey_erp_prod | gzip > /backups/odyssey-$(date +%Y%m%d).sql.gz

# Test restore monthly
pg_restore -d odyssey_erp_test < /backups/odyssey-latest.sql.gz
```

### Rollback Procedures

```bash
# If deployment fails:
1. Stop current version: systemctl stop odyssey-web
2. Revert to previous version: git checkout previous-tag
3. Run migrations down: ./bin/odyssey migrate-down
4. Restart: systemctl start odyssey-web

# Verify: curl https://api.odyssey.example.com/health
```

### Scaling

**Horizontal Scaling** (stateless):
```yaml
# Kubernetes deployment
replicas: 3
```

**Vertical Scaling**:
- Increase DB pool: MaxConns: 100
- Increase worker threads: NumWorkers: 8
- Increase cache: Redis memory: 16GB

---

## Monitoring & Alerting

### Key Metrics to Monitor

1. **Response Time**
   - Alert if p99 > 500ms
   - Alert if p95 > 200ms

2. **Error Rate**
   - Alert if > 1% errors
   - Alert if > 5 errors/minute

3. **Database**
   - Alert if connections > 80%
   - Alert if slow queries > 100ms

4. **Job Queue**
   - Alert if queue depth > 1000
   - Alert if job failure rate > 5%

### Log Analysis

```bash
# Error patterns
grep ERROR logs/odyssey.log | jq '.endpoint' | sort | uniq -c

# Slow queries (> 100ms)
grep "duration.*ms" logs/odyssey.log | grep -E "duration.*[1-9][0-9][0-9]" 

# RBAC denials (audit)
grep "permission_denied" logs/odyssey.log
```

---

## Disaster Recovery

### RTO/RPO Targets
- RTO (Recovery Time Objective): 4 hours
- RPO (Recovery Point Objective): 1 hour

### Failover Procedure

1. **Detect failure**: Monitoring alerts on health checks
2. **Activate standby**: Switch DNS/load balancer to standby instance
3. **Restore database**: Restore from latest backup
4. **Verify**: Run E2E tests on restored instance
5. **Cutover**: Update primary DNS record

### Backup Verification

```bash
# Weekly: Restore backup to test database
pg_restore -d odyssey_erp_test < /backups/odyssey-weekly.sql.gz

# Verify data integrity
SELECT COUNT(*) FROM contracts;
SELECT COUNT(*) FROM shipments;

# Run consistency checks
./bin/odyssey integrity-check
```

---

## Security Hardening

### SSL/TLS

```nginx
# Nginx configuration
ssl_protocols TLSv1.2 TLSv1.3;
ssl_ciphers HIGH:!aNULL:!MD5;
ssl_certificate /etc/ssl/odyssey.crt;
ssl_certificate_key /etc/ssl/odyssey.key;
```

### CORS Policy

```go
// Allow specific origins only
allowedOrigins := []string{
    "https://odyssey.example.com",
    "https://app.odyssey.example.com",
}
```

### Rate Limiting & DDoS Protection

```
- CloudFlare DDoS protection
- Rate limiting: 100 req/min per IP
- Geo-blocking: Restrict to allowed countries
```

### Secrets Management

```bash
# Use environment variables (not hardcoded)
DB_PASSWORD=${PG_PASSWORD}
REDIS_PASSWORD=${REDIS_PASSWORD}

# Or use AWS Secrets Manager
aws secretsmanager get-secret-value --secret-id odyssey-prod
```

---

## Support & Escalation

### Support Tiers

**Tier 1: Operations** (on-call)
- Monitoring alerts
- Health checks
- Restart procedures

**Tier 2: Engineering** (escalation)
- Database issues
- Query optimization
- Code bugs

**Tier 3: Leadership**
- Major incidents
- Feature requests
- Strategic decisions

### Contact Information

- **On-Call:** +1-XXX-XXX-XXXX
- **Slack:** #odyssey-incidents
- **Email:** support@odyssey.example.com

---

## Verification Checklist

Before going live:

- [ ] All E2E tests passing
- [ ] RBAC permissions verified
- [ ] Database backups working
- [ ] Monitoring alerts configured
- [ ] SSL certificates valid
- [ ] Rate limiting configured
- [ ] Logging aggregation working
- [ ] Team trained on procedures
- [ ] Rollback procedure tested
- [ ] Disaster recovery tested

---

## Post-Deployment

### Week 1: Monitoring
- Monitor error rates
- Check slow query logs
- Verify cache hit rates
- Monitor user feedback

### Month 1: Optimization
- Analyze usage patterns
- Optimize hot queries
- Adjust rate limits
- Fine-tune cache TTLs

### Ongoing
- Monthly security audits
- Quarterly performance reviews
- Continuous monitoring
- User feedback incorporation

---

## Support Contact

For production issues:
- **Email:** ops@odyssey.example.com
- **Phone:** +1-XXX-XXX-XXXX
- **Slack:** #odyssey-incidents (24/7)

**Deployment completed: [DATE]**
**Last verified: [DATE]**
**Next review: [DATE + 30 days]**
