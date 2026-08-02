# Security Guide

Panduan keamanan untuk Odyssey ERP.

## Security Stack

| Component | Technology | Purpose |
|-----------|------------|---------|
| Rate Limiting | `httprate` | Request throttling |
| CSRF Protection | Token per form | Prevent CSRF attacks |
| Secure Headers | `unrolled/secure` | HTTP hardening |
| Session | Redis + HttpOnly cookie | Secure session storage |
| Password Hashing | bcrypt | Password storage |

## Security Checklist

### Authentication
- [ ] Session cookie dengan `HttpOnly` flag
- [ ] CSRF token di setiap form
- [ ] Password hashing dengan bcrypt (cost 12+)
- [ ] Session rotation setelah login
- [ ] Logout clears session completely

### Authorization
- [ ] RBAC permission check setiap request
- [ ] No direct object references exposed
- [ ] Role assignment hanya oleh admin

### HTTP Security
- [ ] X-Frame-Options: DENY
- [ ] X-Content-Type-Options: nosniff
- [ ] X-XSS-Protection enabled
- [ ] Content-Security-Policy configured
- [ ] HTTPS only di production

### Database
- [ ] Prepared statements (via sqlc)
- [ ] Input validation sebelum query
- [ ] Connection string tidak di-log
- [ ] Database user dengan minimal privileges

### Rate Limiting

| Endpoint | Limit | Code |
|----------|-------|------|
| `POST /auth/login` | 5 requests/minute per IP | `internal/auth/handler.go:50` |
| General API | 60 requests/minute per IP | `internal/app/middleware.go:159` |
| Export (PDF/CSV) — Balance Sheet, P&L, Trial Balance | 10 requests/minute per IP | `internal/consol/http/handlers_bs.go:44`, `handlers_pl.go:44`, `handlers_tb.go:55` |
| Audit log export | 10 requests/minute per IP | `internal/audit/http/routes.go:22` |
| Analytics export (PDF/CSV) | 10 requests/minute per IP | `internal/analytics/http/routes.go:19` |

## RBAC Implementation

Lihat [RBAC Reference](../reference/rbac.md) untuk detail lengkap.

### Permission Check Flow
```
Request → Auth Middleware → Permission Check → Handler
                ↓
           Session Valid?
                ↓
           Has Permission?
                ↓
           Allow/Deny
```

## Security Testing

```bash
# Run security-focused tests
go test -v ./internal/auth/...

# Check for vulnerabilities
govulncheck ./...
```

## Enterprise control status

The following table is the current security boundary. A control is not considered
implemented merely because the application has a related concept such as RBAC or
sessions.

| Control | Status | Current evidence / boundary |
|---|---|---|
| RBAC | Implemented | Permission middleware and module-scoped roles; see [`reference/rbac.md`](../reference/rbac.md) |
| Audit logs | Implemented | Audit timeline and protected exports are available; retention and tamper-evidence policy remain to be formalized |
| 2FA | Unsupported | No TOTP, WebAuthn, recovery-code, enrollment, or enforcement flow is documented |
| SSO | Planned | OIDC-first federation, identity linking, role mapping, and break-glass controls are specified in the [`External Integrations Plan`](external-integrations-plan.md); no provider or session handoff is implemented |
| Encryption in transit | Partial | HTTPS is a production requirement; certificate, TLS policy, and reverse-proxy ownership are deployment concerns |
| Encryption at rest | Unsupported | No database, backup, object-storage, or key-management policy is documented |
| IP restrictions | Unsupported | Rate limiting is documented, but allowlists/denylists and trusted-network policy are not |
| Device/session management | Partial | Redis-backed sessions and logout exist; device inventory, session listing, revocation UI, and anomaly detection are not documented |
| Secrets management | Partial | Secrets are environment-configured and should not be logged; a managed vault/KMS policy is not documented |
| Backup and restore | Planned | Manual backup/restore procedures exist in deployment material; automated backup verification is a roadmap item |
| Disaster recovery | Planned | RPO/RTO, restore drills, failover, and regional recovery procedures are not yet defined |
| Data retention | Planned | Audit/tax immutability exists, but retention periods, legal holds, deletion/anonymization, and purge ownership are not defined |
| GDPR/privacy | Unsupported | No data inventory, subject-access, erasure, consent, breach, or processor-control procedure is documented |
| ISO 27001 controls | Unsupported | No control mapping, evidence owner, risk register, or statement of applicability is documented |

### Minimum production baseline

Before claiming enterprise security readiness, document and test: TLS termination and
certificate rotation; encrypted database and backup storage; secret rotation; session
revocation; backup restore drills; stated RPO/RTO; audit-log retention and access;
incident severity and notification timelines; and privacy/compliance ownership.

### Backup and restore boundary

The application depends on PostgreSQL, Redis, and externally configured services. A
database dump alone is not a disaster-recovery plan: restore procedures must include
schema migrations, secret/config recovery, Redis rebuild expectations, worker queues,
uploaded/generated artifacts, and post-restore integrity checks. Until those checks are
automated and exercised, backup/DR remains **Planned** in the module catalog.

## Incident Response

1. **Detection**: Monitor logs untuk anomalies
2. **Containment**: Disable compromised account
3. **Investigation**: Review audit logs
4. **Recovery**: Reset credentials
5. **Post-mortem**: Document dan improve

## Historical Docs

Security checklists untuk phases sebelumnya ada di [archive/](../archive/).
