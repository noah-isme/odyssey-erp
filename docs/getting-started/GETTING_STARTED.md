# Getting Started - Odyssey ERP

Quick guide untuk menjalankan Odyssey ERP tanpa Docker.

## 🚀 Quick Start (3 Steps)

### 1. Setup Database & Test Account

Jalankan 5 perintah ini:

```bash
# 1. Create PostgreSQL user (password: odyssey)
sudo -u postgres createuser -P odyssey

# 2. Create database
sudo -u postgres createdb -O odyssey odyssey

# 3. Set connection string
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'

# 4. Run migrations
make migrate-up

# 5. Create test account
make seed
```

✅ Test account dibuat:
- Email: `admin@odyssey.local`
- Password: `admin123`

💡 **Troubleshooting:** Lihat `SETUP_DATABASE.md` untuk panduan lengkap

### 2. Start Application

```bash
./run-background.sh
```

Server akan berjalan di background pada port 8080.

### 3. Access Web UI

Buka browser ke: **http://localhost:8080**

Login dengan:
- **Email:** `admin@odyssey.local`
- **Password:** `admin123`

---

## 📋 Available Commands

```bash
./run.sh                # Run in foreground (interactive)
./run-background.sh     # Run in background (daemon)
./stop.sh               # Stop application
./status.sh             # Check status
./setup-db.sh           # Setup database
```

---

## 📖 Documentation

- **SCRIPTS_USAGE.txt** - Quick command reference
- **RUN_WITHOUT_DOCKER.md** - Complete guide
- **TEST_ACCOUNTS.md** - Test account & database setup details

---

## ⚠️ Prerequisites

Pastikan sudah terinstall:
- Go 1.24+
- PostgreSQL
- Redis (akan auto-start)

---

## 🔍 Verify Installation

```bash
# Check all services
./status.sh

# View logs
tail -f /tmp/odyssey-erp.log

# Test HTTP endpoint
curl http://localhost:8080/
```

---

## 🆘 Troubleshooting

### Database connection error

```bash
./setup-db.sh
```

### Port 8080 in use

```bash
./stop.sh
./run-background.sh
```

### Cannot login

Verify test account exists:
```bash
make seed
```

---

## 📚 Next Steps

1. ✅ Setup database: `./setup-db.sh`
2. ✅ Start app: `./run-background.sh`  
3. ✅ Login: http://localhost:8080
4. 📖 Read full documentation
5. 🧪 Explore features
6. 💻 Start developing

Happy coding! 🎉
