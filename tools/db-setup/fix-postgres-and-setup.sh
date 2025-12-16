#!/bin/bash
# Fix PostgreSQL dan Setup Database

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}=== Fix PostgreSQL & Setup Database ===${NC}"
echo ""
echo "Script ini akan:"
echo "1. Backup pg_hba.conf"
echo "2. Set trust authentication sementara"
echo "3. Restart PostgreSQL"
echo "4. Create database & user"
echo "5. Restore authentication"
echo ""
read -p "Lanjutkan? (y/n): " confirm
if [ "$confirm" != "y" ]; then
    exit 0
fi

PG_HBA="/var/lib/postgres/data/pg_hba.conf"

# Backup
echo -e "${YELLOW}Backup pg_hba.conf...${NC}"
sudo cp $PG_HBA ${PG_HBA}.backup.$(date +%Y%m%d_%H%M%S)

# Create new pg_hba.conf with trust
echo -e "${YELLOW}Setting trust authentication...${NC}"
sudo tee $PG_HBA > /dev/null << 'EOF'
# TYPE  DATABASE        USER            ADDRESS                 METHOD
local   all             all                                     trust
host    all             all             127.0.0.1/32            trust
host    all             all             ::1/128                 trust
local   replication     all                                     trust
host    replication     all             127.0.0.1/32            trust
host    replication     all             ::1/128                 trust
EOF

echo -e "${GREEN}✓ pg_hba.conf updated${NC}"

# Kill current postgres processes
echo -e "${YELLOW}Stopping PostgreSQL...${NC}"
sudo pkill -u postgres postgres || true
sleep 2

# Start PostgreSQL manually
echo -e "${YELLOW}Starting PostgreSQL...${NC}"
sudo -u postgres /usr/bin/postgres -D /var/lib/postgres/data > /tmp/postgres.log 2>&1 &
sleep 3

# Check if running
if pgrep -u postgres postgres > /dev/null; then
    echo -e "${GREEN}✓ PostgreSQL started${NC}"
else
    echo -e "${RED}✗ Failed to start PostgreSQL${NC}"
    echo "Check: tail /tmp/postgres.log"
    exit 1
fi

# Create database and user (no password needed now)
echo -e "${YELLOW}Creating database and user...${NC}"
psql -h localhost -U postgres << 'EOSQL'
-- Set password for postgres
ALTER USER postgres WITH PASSWORD 'postgres';

-- Create odyssey user
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_user WHERE usename = 'odyssey') THEN
        CREATE USER odyssey WITH PASSWORD 'odyssey';
    END IF;
END
$$;

-- Create database
SELECT 'CREATE DATABASE odyssey OWNER odyssey'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'odyssey')
\gexec

-- Grant privileges
GRANT ALL PRIVILEGES ON DATABASE odyssey TO odyssey;

\q
EOSQL

echo -e "${GREEN}✓ Database created${NC}"

# Restore secure authentication
echo -e "${YELLOW}Restoring secure authentication...${NC}"
sudo tee $PG_HBA > /dev/null << 'EOF'
# TYPE  DATABASE        USER            ADDRESS                 METHOD
local   all             postgres                                peer
local   all             all                                     peer
host    all             all             127.0.0.1/32            md5
host    all             all             ::1/128                 md5
local   replication     all                                     peer
host    replication     all             127.0.0.1/32            md5
host    replication     all             ::1/128                 md5
EOF

# Reload PostgreSQL
echo -e "${YELLOW}Reloading PostgreSQL...${NC}"
psql -h localhost -U postgres -c "SELECT pg_reload_conf();" > /dev/null

echo -e "${GREEN}✓ Authentication restored${NC}"

# Test connection
echo -e "${YELLOW}Testing connection...${NC}"
if PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey -c "SELECT version();" > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Connection successful${NC}"
else
    echo -e "${RED}✗ Connection failed${NC}"
    exit 1
fi

# Migrations
echo ""
echo -e "${YELLOW}Running migrations...${NC}"
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'

if ! command -v migrate &> /dev/null; then
    echo "Installing migrate tool..."
    go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
    export PATH=$PATH:$(go env GOPATH)/bin
fi

migrate -path migrations -database "$PG_DSN" up
echo -e "${GREEN}✓ Migrations complete${NC}"

# Seed
echo ""
echo -e "${YELLOW}Creating test account...${NC}"
PG_DSN="$PG_DSN" go run ./scripts/seed/main.go

echo ""
echo -e "${GREEN}═══════════════════════════════════════════════${NC}"
echo -e "${GREEN}✓ SETUP COMPLETE!${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════${NC}"
echo ""
echo -e "PostgreSQL password has been set to: ${GREEN}postgres${NC}"
echo ""
echo -e "Test Account:"
echo -e "  Email:    ${GREEN}admin@odyssey.local${NC}"
echo -e "  Password: ${GREEN}admin123${NC}"
echo ""
echo -e "Next steps:"
echo -e "  ${YELLOW}./run-background.sh${NC}  → Start application"
echo ""
echo -e "Access: ${GREEN}http://localhost:8080${NC}"
echo ""
