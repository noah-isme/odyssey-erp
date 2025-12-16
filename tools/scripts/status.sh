#!/bin/bash
# Odyssey ERP - Status Check Script

PID_FILE="/tmp/odyssey-erp.pid"
LOG_FILE="/tmp/odyssey-erp.log"

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}=== Odyssey ERP Status ===${NC}"
echo ""

# Check main application
echo -e "${YELLOW}Application:${NC}"
if [ -f "$PID_FILE" ]; then
    PID=$(cat "$PID_FILE")
    if ps -p "$PID" > /dev/null 2>&1; then
        echo -e "  Status: ${GREEN}RUNNING${NC}"
        echo -e "  PID: ${GREEN}$PID${NC}"
    else
        echo -e "  Status: ${RED}STOPPED${NC} (stale PID file)"
        rm -f "$PID_FILE"
    fi
else
    # Check without PID file
    PIDS=$(pgrep -f "odyssey/main.go")
    if [ ! -z "$PIDS" ]; then
        echo -e "  Status: ${GREEN}RUNNING${NC}"
        echo -e "  PIDs: ${GREEN}$PIDS${NC}"
    else
        echo -e "  Status: ${RED}STOPPED${NC}"
    fi
fi
echo ""

# Check Redis
echo -e "${YELLOW}Redis:${NC}"
if redis-cli -h localhost -p 6379 ping > /dev/null 2>&1; then
    echo -e "  Status: ${GREEN}RUNNING${NC}"
    echo -e "  Port: ${GREEN}6379${NC}"
else
    echo -e "  Status: ${RED}STOPPED${NC}"
fi
echo ""

# Check PostgreSQL
echo -e "${YELLOW}PostgreSQL:${NC}"
if pg_isready -h localhost -p 5432 > /dev/null 2>&1; then
    echo -e "  Status: ${GREEN}RUNNING${NC}"
    echo -e "  Port: ${GREEN}5432${NC}"
else
    echo -e "  Status: ${RED}STOPPED${NC}"
fi
echo ""

# Check port 8080
echo -e "${YELLOW}HTTP Server:${NC}"
if ss -tlnp 2>/dev/null | grep -q ":8080"; then
    echo -e "  Port 8080: ${GREEN}LISTENING${NC}"
    
    # Try to access
    if curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/ | grep -q "200"; then
        echo -e "  HTTP Check: ${GREEN}OK${NC}"
        echo -e "  URL: ${GREEN}http://localhost:8080${NC}"
    else
        echo -e "  HTTP Check: ${RED}FAILED${NC}"
    fi
else
    echo -e "  Port 8080: ${RED}NOT LISTENING${NC}"
fi
echo ""

# Show recent logs if available
if [ -f "$LOG_FILE" ]; then
    echo -e "${YELLOW}Recent Logs:${NC}"
    echo -e "${BLUE}----------------------------------------${NC}"
    tail -10 "$LOG_FILE"
    echo -e "${BLUE}----------------------------------------${NC}"
    echo -e "Full logs: ${GREEN}$LOG_FILE${NC}"
fi
