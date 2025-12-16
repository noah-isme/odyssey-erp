#!/bin/bash
# Manual Database Setup for Odyssey ERP
# Use this if automatic setup fails

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${YELLOW}=== Manual Database Setup ===${NC}"
echo ""
echo "This script will guide you through manual database setup."
echo ""

# Step 1: Create user
echo -e "${YELLOW}Step 1: Create PostgreSQL User${NC}"
echo "Run this command:"
echo ""
echo -e "${GREEN}sudo -u postgres createuser -P odyssey${NC}"
echo ""
echo "When prompted, enter password: odyssey"
echo ""
read -p "Press Enter after completing this step..."

# Step 2: Create database
echo ""
echo -e "${YELLOW}Step 2: Create Database${NC}"
echo "Run this command:"
echo ""
echo -e "${GREEN}sudo -u postgres createdb -O odyssey odyssey${NC}"
echo ""
read -p "Press Enter after completing this step..."

# Step 3: Verify
echo ""
echo -e "${YELLOW}Step 3: Verify Connection${NC}"
echo "Testing connection..."
if PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey -c "SELECT 1;" > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Connection successful!${NC}"
else
    echo -e "${RED}✗ Connection failed${NC}"
    echo ""
    echo "Possible fixes:"
    echo "1. Check PostgreSQL is running: pg_isready"
    echo "2. Check pg_hba.conf allows password auth:"
    echo "   sudo nano /var/lib/postgres/data/pg_hba.conf"
    echo "   Add line: host all all 127.0.0.1/32 md5"
    echo "3. Reload PostgreSQL: sudo -u postgres pg_ctl reload"
    exit 1
fi

# Step 4: Run migrations
echo ""
echo -e "${YELLOW}Step 4: Run Migrations${NC}"
if command -v migrate &> /dev/null; then
    export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
    migrate -path migrations -database "$PG_DSN" up
    echo -e "${GREEN}✓ Migrations completed${NC}"
else
    echo -e "${YELLOW}⚠ 'migrate' tool not found${NC}"
    echo "Install it:"
    echo "  go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest"
    echo ""
    echo "Or run manually:"
    echo "  export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'"
    echo "  make migrate-up"
fi

# Step 5: Seed data
echo ""
echo -e "${YELLOW}Step 5: Seed Test Data${NC}"
PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable' go run ./scripts/seed/main.go

echo ""
echo -e "${GREEN}✓ Manual setup complete!${NC}"
echo ""
echo -e "Test Account:"
echo -e "  Email: ${GREEN}admin@odyssey.local${NC}"
echo -e "  Password: ${GREEN}admin123${NC}"
echo ""
