# Setup Database dengan Docker (Alternatif Mudah)

Jika setup PostgreSQL lokal sulit karena masalah permission/socket, gunakan PostgreSQL via Docker:

## Option 1: PostgreSQL via Docker (Paling Mudah)

### 1. Start PostgreSQL Container

```bash
docker run -d \
  --name odyssey-postgres \
  -e POSTGRES_USER=odyssey \
  -e POSTGRES_PASSWORD=odyssey \
  -e POSTGRES_DB=odyssey \
  -p 5432:5432 \
  postgres:15-alpine
```

### 2. Verify Running

```bash
docker ps | grep odyssey-postgres
```

### 3. Test Connection

```bash
PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey -c "SELECT version();"
```

### 4. Run Migrations

```bash
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
make migrate-up
```

### 5. Seed Data

```bash
make seed
```

### 6. Start Application

```bash
./run-background.sh
```

---

## Option 2: PostgreSQL Local (Jika password postgres diketahui)

Jika Anda tahu password postgres:

```bash
# Set password
export PGPASSWORD=your_postgres_password

# Create user
psql -h localhost -U postgres -c "CREATE USER odyssey WITH PASSWORD 'odyssey';"

# Create database
psql -h localhost -U postgres -c "CREATE DATABASE odyssey OWNER odyssey;"

# Continue with migrations
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
make migrate-up
make seed
```

---

## Option 3: Reset PostgreSQL Password

### Arch Linux

```bash
# Stop PostgreSQL
sudo systemctl stop postgresql

# Edit pg_hba.conf untuk trust local
sudo sed -i 's/^local.*all.*postgres.*$/local all postgres trust/' /var/lib/postgres/data/pg_hba.conf

# Start PostgreSQL
sudo systemctl start postgresql

# Set new password
sudo -u postgres psql -c "ALTER USER postgres PASSWORD 'newpassword';"

# Revert pg_hba.conf to md5
sudo sed -i 's/^local.*all.*postgres.*trust$/local all postgres md5/' /var/lib/postgres/data/pg_hba.conf

# Reload
sudo systemctl reload postgresql
```

---

## Manage PostgreSQL Docker Container

### Stop Container
```bash
docker stop odyssey-postgres
```

### Start Container
```bash
docker start odyssey-postgres
```

### Remove Container
```bash
docker rm -f odyssey-postgres
```

### View Logs
```bash
docker logs odyssey-postgres
```

### Connect to Database
```bash
docker exec -it odyssey-postgres psql -U odyssey -d odyssey
```

---

## Troubleshooting

### Port 5432 already in use

Ada PostgreSQL lokal yang berjalan. Stop dulu:

```bash
# Stop local PostgreSQL
sudo systemctl stop postgresql

# Or use different port for Docker
docker run -d \
  --name odyssey-postgres \
  -e POSTGRES_USER=odyssey \
  -e POSTGRES_PASSWORD=odyssey \
  -e POSTGRES_DB=odyssey \
  -p 5433:5432 \
  postgres:15-alpine

# Update DSN
export PG_DSN='postgres://odyssey:odyssey@localhost:5433/odyssey?sslmode=disable'
```

### Docker not installed

Install Docker:

**Arch Linux:**
```bash
sudo pacman -S docker
sudo systemctl start docker
sudo systemctl enable docker
sudo usermod -aG docker $USER
# Logout and login again
```

**Ubuntu/Debian:**
```bash
sudo apt update
sudo apt install docker.io
sudo systemctl start docker
sudo systemctl enable docker
sudo usermod -aG docker $USER
# Logout and login again
```

---

## Recommended: Docker Compose

Buat file `docker-compose.db.yml`:

```yaml
version: '3.8'

services:
  postgres:
    image: postgres:15-alpine
    container_name: odyssey-postgres
    environment:
      POSTGRES_USER: odyssey
      POSTGRES_PASSWORD: odyssey
      POSTGRES_DB: odyssey
    ports:
      - "5432:5432"
    volumes:
      - odyssey-pgdata:/var/lib/postgresql/data
    restart: unless-stopped

volumes:
  odyssey-pgdata:
```

Start:
```bash
docker-compose -f docker-compose.db.yml up -d
```

Stop:
```bash
docker-compose -f docker-compose.db.yml down
```

---

## Summary

**Paling Mudah:**
```bash
# 1. Start PostgreSQL Docker
docker run -d --name odyssey-postgres \
  -e POSTGRES_USER=odyssey -e POSTGRES_PASSWORD=odyssey \
  -e POSTGRES_DB=odyssey -p 5432:5432 postgres:15-alpine

# 2. Run migrations
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
make migrate-up

# 3. Seed data
make seed

# 4. Start app
./run-background.sh
```

Done! 🎉
