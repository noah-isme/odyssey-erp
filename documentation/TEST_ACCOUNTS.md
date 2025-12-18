# Test Accounts - Odyssey ERP

## Default Test Account

Odyssey ERP sudah menyediakan akun test default yang dibuat melalui script seed.

### Admin Account

```
Email:    admin@odyssey.local
Password: admin123
```

**Akses:** Full access ke semua modul

## Setup Database & Create Test Account

### Option 1: Automatic Setup (Recommended)

```bash
./setup-db.sh
```

Script ini akan:
1. Create PostgreSQL user `odyssey` dengan password `odyssey`
2. Create database `odyssey`
3. Run migrations
4. Seed test data termasuk akun admin

### Option 2: Manual Setup

#### 1. Create PostgreSQL User & Database

```bash
sudo -u postgres psql
```

Kemudian jalankan SQL:

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

#### 2. Run Migrations

```bash
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
make migrate-up
```

Atau jika tidak ada `migrate` tool:

```bash
# Install migrate tool
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Run migrations
migrate -path migrations -database "$PG_DSN" up
```

#### 3. Seed Test Data

```bash
make seed
```

Atau langsung:

```bash
PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable' \
  go run ./scripts/seed/main.go
```

## Verify Account Created

### Check in Database

```bash
PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey \
  -c "SELECT id, email, is_active, created_at FROM users;"
```

Expected output:
```
 id |         email          | is_active |         created_at
----+------------------------+-----------+----------------------------
  1 | admin@odyssey.local    | t         | 2025-12-09 12:00:00.000000
```

### Test Login via API

```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "admin@odyssey.local",
    "password": "admin123"
  }'
```

## Roles & Permissions

Akun `admin@odyssey.local` memiliki role `admin` dengan permissions:

- ✓ Core platform (`users.view`, `users.edit`, `roles.view`, `roles.edit`, `permissions.view`)
- ✓ Organization management (`org.view`, `org.edit`)
- ✓ Master data (`master.view`, `master.edit`, `master.import`)
- ✓ RBAC configuration (`rbac.view`, `rbac.edit`)
- ✓ Reports (`report.view`)
- ✓ Inventory (`inventory.view`, `inventory.edit`)
- ✓ Procurement (`procurement.view`, `procurement.edit`)
- ✓ Finance AP (`finance.ap.view`, `finance.ap.edit`)
- ✓ Board Pack (`finance.boardpack`)
- ✓ Sales (`sales.*`)
- ✓ Delivery (`delivery.*`)
- ✓ Consolidation (`consol.*`)
- ✓ Analytics (`analytics.*`)
- ✓ Audit (`audit.*`)

Catatan: sebagian database lama hanya memiliki permission `rbac.view` / `rbac.edit`; rute `/users`, `/roles`, dan `/permissions` tetap mengizinkan akses via permission tersebut untuk kompatibilitas.

## Create Additional Test Users

### Via Seed Script (Edit `scripts/seed/main.go`)

Add more users in `seedUsers` function:

```go
func seedUsers(ctx context.Context, pool *pgxpool.Pool) error {
    users := []struct {
        email    string
        password string
    }{
        {"admin@odyssey.local", "admin123"},
        {"user@odyssey.local", "user123"},
        {"manager@odyssey.local", "manager123"},
    }
    
    for _, u := range users {
        password, _ := bcrypt.GenerateFromPassword([]byte(u.password), bcrypt.DefaultCost)
        _, err := pool.Exec(ctx, `INSERT INTO users (email, password_hash, is_active, created_at, updated_at)
        VALUES ($1, $2, TRUE, NOW(), NOW())
        ON CONFLICT (email) DO NOTHING`, u.email, string(password))
        if err != nil {
            return err
        }
    }
    return nil
}
```

Then run:
```bash
make seed
```

### Via SQL

```sql
-- Generate bcrypt hash for password (use bcrypt tool or Go)
-- bcrypt hash for "password123": $2a$10$...

INSERT INTO users (email, password_hash, is_active, created_at, updated_at)
VALUES (
    'testuser@odyssey.local',
    '$2a$10$YourBcryptHashHere',
    TRUE,
    NOW(),
    NOW()
);
```

## Troubleshooting

### "password authentication failed"

Database user belum dibuat. Jalankan:
```bash
./setup-db.sh
```

### "database does not exist"

Database belum dibuat. Jalankan:
```bash
sudo -u postgres createdb -O odyssey odyssey
```

### Cannot login with admin account

1. Verify user exists:
   ```bash
   PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey \
     -c "SELECT * FROM users WHERE email='admin@odyssey.local';"
   ```

2. Re-run seed script:
   ```bash
   make seed
   ```

3. Check application logs:
   ```bash
   tail -f /tmp/odyssey-erp.log
   ```

### Reset admin password

```sql
-- Connect to database
PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey

-- Update password (bcrypt hash for "admin123")
UPDATE users 
SET password_hash = '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy'
WHERE email = 'admin@odyssey.local';
```

Or run seed script again (safe, uses ON CONFLICT DO NOTHING):
```bash
make seed
```

## Security Notes

⚠️ **For Development Only**

These test accounts are for **development and testing only**. 

For production:
- Change all default passwords
- Use strong passwords (min 12 characters)
- Enable 2FA if available
- Regularly rotate passwords
- Audit user access logs
- Remove unused accounts

## Next Steps

After login:
1. Navigate to http://localhost:8080/
2. Login with `admin@odyssey.local` / `admin123`
3. Change password in user settings
4. Create additional users with appropriate roles
5. Configure organization and master data
