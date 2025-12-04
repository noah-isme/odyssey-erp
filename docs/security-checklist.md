# Security Checklist – Phase 1

- [x] HTTP secure headers via `unrolled/secure` (X-Frame-Options, X-Content-Type-Options, Referrer-Policy).
- [x] Rate limiting 60 rpm per IP using `httprate`.
- [x] Global request timeout 30s.
- [x] Session cookie HttpOnly + SameSite=Strict; secure flag in production.
- [x] Session storage Redis dengan TTL dapat dikonfigurasi.
- [x] CSRF token wajib untuk POST (middleware + form hidden input).
- [x] Password diverifikasi menggunakan `bcrypt`.
- [x] Audit log tabel disiapkan (`audit_logs`).
- [x] Input login divalidasi `validator/v10`.
- [x] Sorting/pagination helper whitelist (skeleton `internal/shared/pagination.go`).
