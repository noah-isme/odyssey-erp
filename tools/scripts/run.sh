#!/bin/bash
# Odyssey ERP - Startup Script
# Run without Docker

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Odyssey ERP Startup ===${NC}"

# Check if Redis is running
echo -n "Checking Redis... "
if redis-cli -h localhost -p 6379 ping > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${RED}✗${NC}"
    echo -e "${YELLOW}Starting Redis...${NC}"
    redis-server --port 6379 --daemonize yes
    sleep 2
    if redis-cli -h localhost -p 6379 ping > /dev/null 2>&1; then
        echo -e "${GREEN}Redis started${NC}"
    else
        echo -e "${RED}Failed to start Redis${NC}"
        exit 1
    fi
fi

# Check if PostgreSQL is running
echo -n "Checking PostgreSQL... "
if pg_isready -h localhost -p 5432 > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${RED}✗${NC}"
    echo -e "${RED}PostgreSQL is not running. Please start PostgreSQL first.${NC}"
    exit 1
fi

# Environment variables
export ODYSSEY_TEST_MODE=0
export APP_ENV=development
export APP_ADDR=:8080
export APP_READ_TIMEOUT=15s
export APP_WRITE_TIMEOUT=15s
export APP_REQUEST_TIMEOUT=30s
export LOG_FORMAT=pretty
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
export REDIS_ADDR=localhost:6379
export SESSION_SECRET=local-dev-session-secret-change-in-production
export SESSION_TTL=720h
export CSRF_SECRET=local-dev-csrf-secret-change-in-production
export SMTP_HOST=localhost
export SMTP_PORT=1025
export SMTP_FROM=no-reply@odyssey.local
export GOTENBERG_URL=http://localhost:3000

echo -e "${GREEN}Starting Odyssey ERP...${NC}"
echo -e "Access URL: ${GREEN}http://localhost:8080${NC}"
echo -e "Press ${YELLOW}Ctrl+C${NC} to stop"
echo ""

# Run the application
go run ./cmd/odyssey/main.go
