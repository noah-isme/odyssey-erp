# Quick Fix - PostgreSQL Setup

PostgreSQL sistem Anda tidak mengizinkan koneksi tanpa password. Ini cara termudah:

## Opsi 1: Set Password PostgreSQL User (Paling Cepat)

```bash
# 1. Set password untuk user postgres
sudo -u postgres psql -h localhost

# Akan muncul prompt password, tekan ENTER (kosong) atau coba password umum:
# - postgres
# - (kosong/enter saja)
```

Jika masuk, jalankan SQL:
```sql
ALTER USER postgres WITH PASSWORD 'postgres';
\q
```

Lalu jalankan:
```bash
export PGPASSWORD=postgres
psql -h localhost -U postgres << 'EOF'
CREATE USER odyssey WITH PASSWORD 'odyssey';
CREATE DATABASE odyssey OWNER odyssey;
GRANT ALL PRIVILEGES ON DATABASE odyssey TO odyssey;
\q
EOF

# Lanjut migrations & seed
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
make migrate-up
make seed
```

---

## Opsi 2: Manual SQL Execution (Copy-Paste)

Jika tahu password postgres, jalankan:

```bash
# Ganti 'PASSWORDNYA' dengan password postgres yang benar
export PGPASSWORD=PASSWORDNYA

psql -h localhost -U postgres << 'EOF'
CREATE USER odyssey WITH PASSWORD 'odyssey';
CREATE DATABASE odyssey OWNER odyssey;
GRANT ALL PRIVILEGES ON DATABASE odyssey TO odyssey;
EOF

# Migrations & seed
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
make migrate-up
make seed
```

---

## Opsi 3: Menggunakan psql Interaktif

```bash
# 1. Connect ke PostgreSQL (coba dengan user postgres)
psql -h localhost -U postgres

# Masukkan password postgres jika diminta

# 2. Jalankan SQL ini di psql:
CREATE USER odyssey WITH PASSWORD 'odyssey';
CREATE DATABASE odyssey OWNER odyssey;
GRANT ALL PRIVILEGES ON DATABASE odyssey TO odyssey;

# 3. Keluar
\q

# 4. Test koneksi
PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey -c "SELECT version();"

# 5. Migrations & seed
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
make migrate-up
make seed
```

---

## Opsi 4: Reset PostgreSQL Config (Advanced)

Jika tidak tahu password postgres sama sekali:

```bash
# 1. Edit pg_hba.conf untuk allow trust
sudo nano /var/lib/postgres/data/pg_hba.conf

# 2. Ubah baris yang ada 'md5' atau 'password' menjadi 'trust':
# Contoh:
# host    all             all             127.0.0.1/32            trust
# host    all             all             ::1/128                 trust

# 3. Reload PostgreSQL
sudo systemctl reload postgresql

# 4. Sekarang bisa connect tanpa password
psql -h localhost -U postgres

# 5. Set password baru
ALTER USER postgres WITH PASSWORD 'postgres';

# 6. Create odyssey user & database
CREATE USER odyssey WITH PASSWORD 'odyssey';
CREATE DATABASE odyssey OWNER odyssey;
GRANT ALL PRIVILEGES ON DATABASE odyssey TO odyssey;
\q

# 7. Kembalikan pg_hba.conf ke md5
sudo nano /var/lib/postgres/data/pg_hba.conf
# Ubah 'trust' kembali ke 'md5'

# 8. Reload lagi
sudo systemctl reload postgresql
```

---

## Verifikasi Setup Berhasil

```bash
# Test koneksi
PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey -c "\dt"

# Jika berhasil, lanjut:
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
make migrate-up
make seed

# Start aplikasi
./run-background.sh
```

---

## Cek Password PostgreSQL Default

Coba password ini untuk user postgres:
- `postgres`
- `password`
- (kosong - tekan Enter saja)
- `admin`

---

## Jika Masih Stuck

Langsung edit pg_hba.conf untuk allow localhost:

```bash
sudo nano /var/lib/postgres/data/pg_hba.conf
```

Pastikan ada baris ini:
```
# IPv4 local connections:
host    all             all             127.0.0.1/32            md5
```

Jika tidak ada, tambahkan. Lalu reload:
```bash
sudo systemctl reload postgresql
```

Lalu coba lagi connect dengan password.
