# Database Setup Guide - Odyssey ERP

Ada 3 cara untuk setup database:

## Option 1: Otomatis (Recommended - Butuh sudo password)

```bash
./setup-db.sh
```

Jika gagal, lanjut ke Option 2.

---

## Option 2: Manual dengan sudo (Paling Mudah)

### Step 1: Create PostgreSQL user

```bash
sudo -u postgres createuser -P odyssey
```

Saat diminta password, ketik: **odyssey** (2x)

### Step 2: Create database

```bash
sudo -u postgres createdb -O odyssey odyssey
```

### Step 3: Verify connection

```bash
PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey -c "SELECT version();"
```

Jika berhasil, lanjut ke Step 4.

### Step 4: Run migrations

```bash
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
make migrate-up
```

Atau jika `migrate` tool belum terinstall:

```bash
# Install migrate tool dulu
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Pastikan $GOPATH/bin ada di PATH
export PATH=$PATH:$(go env GOPATH)/bin

# Run migrations
make migrate-up
```

### Step 5: Seed test data

```bash
make seed
```

**Done!** Test account sudah dibuat:
- Email: `admin@odyssey.local`
- Password: `admin123`

---

## Option 3: Manual via psql

### Connect sebagai postgres user

```bash
sudo -u postgres psql
```

### Jalankan SQL commands

```sql
-- Create user
CREATE USER odyssey WITH PASSWORD 'odyssey';

-- Create database
CREATE DATABASE odyssey OWNER odyssey;

-- Grant privileges
GRANT ALL PRIVILEGES ON DATABASE odyssey TO odyssey;

-- Exit
\q
```

### Verify & Continue

Test connection:
```bash
PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey
```

Jika berhasil connect, keluar dengan `\q` lalu lanjut:

```bash
# Run migrations
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
make migrate-up

# Seed data
make seed
```

---

## Troubleshooting

### Error: "password authentication failed"

PostgreSQL tidak mengizinkan password authentication. Edit `pg_hba.conf`:

```bash
# Find pg_hba.conf
sudo -u postgres psql -c "SHOW hba_file;"

# Edit file (contoh path)
sudo nano /var/lib/postgres/data/pg_hba.conf
```

Tambahkan/ubah baris:
```
# TYPE  DATABASE        USER            ADDRESS                 METHOD
host    all             all             127.0.0.1/32            md5
host    all             all             ::1/128                 md5
```

Reload PostgreSQL:
```bash
sudo -u postgres pg_ctl reload
# atau
sudo systemctl reload postgresql
```

### Error: "database does not exist"

Database belum dibuat. Jalankan:
```bash
sudo -u postgres createdb -O odyssey odyssey
```

### Error: "role odyssey does not exist"

User belum dibuat. Jalankan:
```bash
sudo -u postgres createuser -P odyssey
```

### Error: "migrate: command not found"

Install migrate tool:
```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Add to PATH
export PATH=$PATH:$(go env GOPATH)/bin

# Make it permanent (add to ~/.bashrc atau ~/.zshrc)
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc
```

### Verify Migration Status

```bash
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
migrate -path migrations -database "$PG_DSN" version
```

### Reset Database (Clean Start)

⚠️ **WARNING: This will delete all data!**

```bash
# Drop database
sudo -u postgres dropdb odyssey

# Recreate
sudo -u postgres createdb -O odyssey odyssey

# Run migrations
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
make migrate-up

# Seed data
make seed
```

---

## Quick Commands Summary

```bash
# Create user & database
sudo -u postgres createuser -P odyssey
sudo -u postgres createdb -O odyssey odyssey

# Run migrations
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
make migrate-up

# Seed test data
make seed

# Verify
PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey -c "\dt"
```

---

## Next Steps

Setelah database setup:

1. Start aplikasi: `./run-background.sh`
2. Buka browser: http://localhost:8080
3. Login dengan `admin@odyssey.local` / `admin123`

Happy coding! 🚀
