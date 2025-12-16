#!/bin/bash
# Setup Odyssey ERP Database

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}=== Odyssey ERP Database Setup ===${NC}"

# Check if PostgreSQL is running
if ! pg_isready -h localhost -p 5432 > /dev/null 2>&1; then
    echo -e "${RED}PostgreSQL is not running!${NC}"
    echo "Please start PostgreSQL first"
    exit 1
fi

echo -e "${YELLOW}Creating database and user...${NC}"

# Create SQL script
cat > /tmp/odyssey_setup.sql << 'EOSQL'
-- Create user if not exists
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_user WHERE usename = 'odyssey') THEN
        CREATE USER odyssey WITH PASSWORD 'odyssey';
    END IF;
END
$$;

-- Create database if not exists
SELECT 'CREATE DATABASE odyssey OWNER odyssey'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'odyssey')\gexec

-- Grant privileges
GRANT ALL PRIVILEGES ON DATABASE odyssey TO odyssey;
EOSQL

# Try multiple connection methods
echo "Attempting to connect to PostgreSQL..."

# Method 1: sudo -u postgres
if sudo -n -u postgres psql -f /tmp/odyssey_setup.sql > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Connected via sudo -u postgres${NC}"
# Method 2: psql without auth (peer/trust)
elif psql -U postgres -f /tmp/odyssey_setup.sql > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Connected as postgres user${NC}"
# Method 3: Use current user if superuser
elif psql -f /tmp/odyssey_setup.sql > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Connected as current user${NC}"
# Method 4: Connect via TCP with localhost
elif PGPASSWORD=postgres psql -h localhost -U postgres -f /tmp/odyssey_setup.sql > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Connected via TCP localhost${NC}"
else
    echo -e "${RED}✗ Failed to connect to PostgreSQL${NC}"
    echo ""
    echo -e "${YELLOW}Please run manually:${NC}"
    echo "  sudo -u postgres psql -f /tmp/odyssey_setup.sql"
    echo ""
    echo "Or create database manually:"
    echo "  sudo -u postgres createuser -P odyssey"
    echo "  sudo -u postgres createdb -O odyssey odyssey"
    rm /tmp/odyssey_setup.sql
    exit 1
fi

rm /tmp/odyssey_setup.sql

echo -e "${GREEN}✓ Database and user created${NC}"

# Verify connection
echo -e "${YELLOW}Verifying database connection...${NC}"
if PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey -c "SELECT 1;" > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Connection verified${NC}"
else
    echo -e "${RED}✗ Cannot connect to database${NC}"
    echo ""
    echo -e "${YELLOW}Database may exist but connection failed.${NC}"
    echo "Try connecting manually:"
    echo "  PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey"
    echo ""
    echo "If connection fails, check pg_hba.conf for authentication settings."
    exit 1
fi

# Run migrations
echo -e "${YELLOW}Running migrations...${NC}"
if command -v migrate &> /dev/null; then
    export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
    migrate -path migrations -database "$PG_DSN" up
    echo -e "${GREEN}✓ Migrations completed${NC}"
else
    echo -e "${YELLOW}⚠ 'migrate' tool not found. Skipping migrations.${NC}"
    echo "Install: https://github.com/golang-migrate/migrate"
    echo "Or use: make migrate-up"
fi

# Seed data
echo -e "${YELLOW}Seeding test data...${NC}"
PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable' go run ./scripts/seed/main.go

echo ""
echo -e "${GREEN}✓ Database setup complete!${NC}"
echo ""
echo -e "Test Account:"
echo -e "  Email: ${GREEN}admin@odyssey.local${NC}"
echo -e "  Password: ${GREEN}admin123${NC}"
echo ""
