# Quick Start - Odyssey ERP

Panduan cepat untuk menjalankan Odyssey ERP.

## 🐳 Docker (Recommended)

```bash
# 1. Start all services
docker-compose up -d

# 2. Run migrations & seed
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
make migrate-up
make seed

# 3. Access application
open http://localhost:8080
```

**Login:**
- Email: `admin@odyssey.local`
- Password: `admin123`

---

## 🔧 Native Installation

Untuk setup tanpa Docker, lihat:
- [Native Setup Guide](native-setup.md) - PostgreSQL lokal
- [Docker Setup Guide](docker-setup.md) - PostgreSQL via Docker container

---

## ✅ Verify Installation

```bash
# Check health endpoint
curl http://localhost:8080/healthz
```

### Verify Seed Data

Confirm the admin account was created:

```bash
PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey \
  -c "SELECT email, is_active FROM users WHERE email='admin@odyssey.local';"
```

Expected output:

```
 email | is_active
-------+-----------
 admin@odyssey.local | t
```

Then test login:

```bash
curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@odyssey.local","password":"admin123"}' | head
```

---

## 🆘 Troubleshooting

**Port 8080 in use:**
```bash
docker compose down
docker-compose down
```

**Database connection error:**
```bash
make migrate-up && make seed
```

**Cannot login:**
```bash
make seed  # Recreate test account
```

Untuk masalah lainnya, lihat [Troubleshooting Guide](troubleshooting.md).

---

## 📚 Next Steps

1. ✅ Explore the application
2. 📖 Read [Architecture Overview](../architecture/arsitektur.md)
3. 🔐 Setup [RBAC](../reference/rbac.md)
4. 🧪 Run [Tests](../guides/testing-runbook.md)
