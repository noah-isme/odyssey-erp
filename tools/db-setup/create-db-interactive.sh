#!/bin/bash
# Interactive Database Creation Script

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}╔═══════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║       Odyssey ERP - Interactive Database Setup                   ║${NC}"
echo -e "${BLUE}╚═══════════════════════════════════════════════════════════════════╝${NC}"
echo ""

echo -e "${YELLOW}Saya akan membantu Anda connect ke PostgreSQL dan create database.${NC}"
echo ""

echo -e "${GREEN}Step 1: Connect ke PostgreSQL${NC}"
echo ""
echo "Jalankan command ini:"
echo ""
echo -e "${BLUE}    psql -h localhost -U postgres${NC}"
echo ""
echo "Saat diminta password, coba salah satu:"
echo "  - Ketik: postgres (lalu Enter)"
echo "  - Atau langsung Enter (password kosong)"
echo ""
read -p "Tekan Enter setelah Anda siap mencoba..."

echo ""
echo -e "${GREEN}Step 2: Setelah masuk psql, copy-paste SQL ini:${NC}"
echo ""
echo -e "${BLUE}CREATE USER odyssey WITH PASSWORD 'odyssey';${NC}"
echo -e "${BLUE}CREATE DATABASE odyssey OWNER odyssey;${NC}"
echo -e "${BLUE}GRANT ALL PRIVILEGES ON DATABASE odyssey TO odyssey;${NC}"
echo -e "${BLUE}\\q${NC}"
echo ""
echo "Lalu tekan Enter untuk keluar dari psql"
echo ""
read -p "Tekan Enter setelah Anda selesai..."

# Test koneksi
echo ""
echo -e "${GREEN}Step 3: Test koneksi ke database odyssey${NC}"
echo ""
if PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey -c "SELECT version();" > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Koneksi berhasil!${NC}"
else
    echo -e "${RED}✗ Koneksi gagal${NC}"
    echo ""
    echo "Kemungkinan:"
    echo "1. Database belum dibuat (coba lagi Step 1-2)"
    echo "2. Password salah"
    echo "3. PostgreSQL belum allow koneksi dari localhost"
    echo ""
    read -p "Coba lagi? (y/n): " retry
    if [ "$retry" = "y" ]; then
        exec "$0"
    else
        exit 1
    fi
fi

# Migrations
echo ""
echo -e "${GREEN}Step 4: Run migrations${NC}"
echo ""
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'

if ! command -v migrate &> /dev/null; then
    echo -e "${YELLOW}Installing migrate tool...${NC}"
    go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
    export PATH=$PATH:$(go env GOPATH)/bin
fi

if command -v migrate &> /dev/null; then
    echo "Running migrations..."
    migrate -path migrations -database "$PG_DSN" up
    echo -e "${GREEN}✓ Migrations complete${NC}"
else
    echo -e "${RED}migrate tool not found${NC}"
    echo "Please install or run: make migrate-up"
    exit 1
fi

# Seed
echo ""
echo -e "${GREEN}Step 5: Create test account${NC}"
echo ""
PG_DSN="$PG_DSN" go run ./scripts/seed/main.go

echo ""
echo -e "${GREEN}╔═══════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║                    ✓ SETUP COMPLETE!                              ║${NC}"
echo -e "${GREEN}╚═══════════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "Test Account:"
echo -e "  ${GREEN}Email:    admin@odyssey.local${NC}"
echo -e "  ${GREEN}Password: admin123${NC}"
echo ""
echo -e "Next steps:"
echo -e "  ${YELLOW}./run-background.sh${NC}  → Start application"
echo -e "  ${YELLOW}./status.sh${NC}          → Check status"
echo ""
echo -e "Access: ${GREEN}http://localhost:8080${NC}"
echo ""
