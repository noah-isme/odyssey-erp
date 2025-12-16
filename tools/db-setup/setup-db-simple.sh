#!/bin/bash
# Simple Database Setup - Manual Execution

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}=== Odyssey ERP Database Setup (System PostgreSQL) ===${NC}"
echo ""

# Create user and database using sudo
echo -e "${YELLOW}Creating database user and database...${NC}"
echo "You will be prompted for your sudo password."
echo ""

sudo -u postgres psql -h localhost << 'EOFSQL'
-- Create user if not exists
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_user WHERE usename = 'odyssey') THEN
        CREATE USER odyssey WITH PASSWORD 'odyssey';
        RAISE NOTICE 'User odyssey created';
    ELSE
        RAISE NOTICE 'User odyssey already exists';
    END IF;
END
$$;

-- Create database if not exists
SELECT 'Database already exists' 
WHERE EXISTS (SELECT FROM pg_database WHERE datname = 'odyssey')
UNION ALL
SELECT 'CREATE DATABASE odyssey OWNER odyssey'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'odyssey')
\gexec

-- Grant privileges
GRANT ALL PRIVILEGES ON DATABASE odyssey TO odyssey;

-- Show created objects
\echo ''
\echo '=== Database User ==='
\du odyssey

\echo ''
\echo '=== Database ==='
\l odyssey
EOFSQL

echo ""
echo -e "${GREEN}✓ Database and user created${NC}"
echo ""

# Verify connection
echo -e "${YELLOW}Verifying connection...${NC}"
if PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey -c "SELECT 'Connection OK' as status;" > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Connection verified${NC}"
else
    echo -e "${RED}✗ Cannot connect${NC}"
    echo ""
    echo "PostgreSQL might not accept TCP connections."
    echo "Checking pg_hba.conf..."
    sudo -u postgres psql -c "SHOW hba_file;"
    exit 1
fi

# Run migrations
echo ""
echo -e "${YELLOW}Running migrations...${NC}"
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'

if command -v migrate &> /dev/null; then
    migrate -path migrations -database "$PG_DSN" up
    echo -e "${GREEN}✓ Migrations completed${NC}"
else
    echo -e "${YELLOW}⚠ 'migrate' tool not found${NC}"
    echo "Installing migrate tool..."
    go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
    
    # Add to PATH if needed
    export PATH=$PATH:$(go env GOPATH)/bin
    
    # Retry
    if command -v migrate &> /dev/null; then
        migrate -path migrations -database "$PG_DSN" up
        echo -e "${GREEN}✓ Migrations completed${NC}"
    else
        echo -e "${RED}Failed to install migrate tool${NC}"
        echo "Please install manually or run: make migrate-up"
        exit 1
    fi
fi

# Seed data
echo ""
echo -e "${YELLOW}Creating test account...${NC}"
PG_DSN="$PG_DSN" go run ./scripts/seed/main.go

echo ""
echo -e "${GREEN}═══════════════════════════════════════════════${NC}"
echo -e "${GREEN}✓ Database Setup Complete!${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════${NC}"
echo ""
echo -e "Test Account:"
echo -e "  Email:    ${GREEN}admin@odyssey.local${NC}"
echo -e "  Password: ${GREEN}admin123${NC}"
echo ""
echo -e "Next steps:"
echo -e "  ${YELLOW}./run-background.sh${NC}  → Start application"
echo -e "  ${YELLOW}./status.sh${NC}          → Check status"
echo ""
echo -e "Access: ${GREEN}http://localhost:8080${NC}"
echo ""
