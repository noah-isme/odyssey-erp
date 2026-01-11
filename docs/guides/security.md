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
- [ ] Login endpoint: 5 requests/minute
- [ ] API endpoints: 100 requests/minute
- [ ] File upload: 10 requests/minute

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

## Incident Response

1. **Detection**: Monitor logs untuk anomalies
2. **Containment**: Disable compromised account
3. **Investigation**: Review audit logs
4. **Recovery**: Reset credentials
5. **Post-mortem**: Document dan improve

## Historical Docs

Security checklists untuk phases sebelumnya ada di [archive/](../archive/).
