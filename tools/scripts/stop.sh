#!/bin/bash
# Odyssey ERP - Stop Script

PID_FILE="/tmp/odyssey-erp.pid"

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}=== Stopping Odyssey ERP ===${NC}"

if [ ! -f "$PID_FILE" ]; then
    echo -e "${YELLOW}No PID file found. Trying to find process...${NC}"
    
    # Try to find by process name
    PIDS=$(pgrep -f "odyssey/main.go")
    if [ -z "$PIDS" ]; then
        echo -e "${RED}Odyssey ERP is not running${NC}"
        exit 1
    fi
    
    for pid in $PIDS; do
        echo -e "Killing process ${GREEN}$pid${NC}"
        kill -9 "$pid" 2>/dev/null || true
    done
else
    PID=$(cat "$PID_FILE")
    
    if ps -p "$PID" > /dev/null 2>&1; then
        echo -e "Stopping Odyssey ERP (PID: ${GREEN}$PID${NC})"
        kill -9 "$PID"
        
        # Wait for process to stop
        for i in {1..5}; do
            if ! ps -p "$PID" > /dev/null 2>&1; then
                break
            fi
            sleep 1
        done
        
        if ps -p "$PID" > /dev/null 2>&1; then
            echo -e "${RED}Failed to stop process $PID${NC}"
            exit 1
        fi
    else
        echo -e "${YELLOW}Process $PID is not running${NC}"
    fi
    
    rm -f "$PID_FILE"
fi

# Clean up any remaining Go processes
GO_PIDS=$(pgrep -f "go run.*odyssey")
if [ ! -z "$GO_PIDS" ]; then
    echo -e "${YELLOW}Cleaning up remaining Go processes...${NC}"
    for pid in $GO_PIDS; do
        kill -9 "$pid" 2>/dev/null || true
    done
fi

# Check if port is free
if ss -tlnp 2>/dev/null | grep -q ":8080"; then
    echo -e "${YELLOW}Port 8080 is still in use. Trying to free it...${NC}"
    fuser -k 8080/tcp 2>/dev/null || true
    sleep 1
fi

echo -e "${GREEN}✓ Odyssey ERP stopped${NC}"
