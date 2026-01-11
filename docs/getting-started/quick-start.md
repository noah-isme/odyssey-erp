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

# Check all services
./tools/scripts/status.sh

# View logs
tail -f /tmp/odyssey-erp.log
```

---

## 🆘 Troubleshooting

**Port 8080 in use:**
```bash
./tools/scripts/stop.sh
docker-compose down
```

**Database connection error:**
```bash
./tools/db-setup/setup-db.sh
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
