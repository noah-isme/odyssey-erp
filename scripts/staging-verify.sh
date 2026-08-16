#!/bin/bash
# staging-verify.sh
# Performs staging certification checks for Odyssey ERP v0.10-core

STAGING_URL="${1:-$ODYSSEY_E2E_URL}"
PG_DSN="${2:-$PG_DSN}"

TOTAL=0
PASSED=0
FAILED=0
SKIPPED=0

print_pass() {
  echo "[PASS] $1"
  ((TOTAL++))
  ((PASSED++))
}

print_fail() {
  echo "[FAIL] $1"
  ((TOTAL++))
  ((FAILED++))
}

print_skip() {
  echo "[SKIP] $1"
  ((TOTAL++))
  ((SKIPPED++))
}

echo "=== Odyssey v0.10-core Staging Verification ==="

# ENV-001
if [ "$RELEASE_PROFILE" = "v0.10-core" ]; then
  print_pass "ENV-001: RELEASE_PROFILE = v0.10-core"
else
  print_fail "ENV-001: RELEASE_PROFILE is '$RELEASE_PROFILE' (expected 'v0.10-core')"
fi

# ENV-002
if [ -z "$CONNECTORS_DEVELOPMENT_MODE" ] || [ "$CONNECTORS_DEVELOPMENT_MODE" = "false" ]; then
  print_pass "ENV-002: CONNECTORS_DEVELOPMENT_MODE is false or unset"
else
  print_fail "ENV-002: CONNECTORS_DEVELOPMENT_MODE is true (expected false or unset)"
fi

if ! command -v curl >/dev/null 2>&1; then
  print_skip "ENV-003: curl not found"
  print_skip "ENV-004: curl not found"
  print_skip "REL-004: curl not found"
  print_skip "REL-005: curl not found"
  print_skip "OPS-002: curl not found"
else
  if [ -n "$STAGING_URL" ]; then
    # ENV-003
    HEALTH_HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$STAGING_URL/healthz")
    if [ "$HEALTH_HTTP_CODE" = "200" ]; then
      print_pass "ENV-003: /healthz returns 200"
    else
      print_fail "ENV-003: /healthz returns $HEALTH_HTTP_CODE (expected 200)"
    fi

    # ENV-004
    EXPECTED_COMMIT="d8b02b8"
    if [ -n "$DEPLOYED_COMMIT" ]; then
      if [ "${DEPLOYED_COMMIT:0:7}" = "$EXPECTED_COMMIT" ]; then
        print_pass "ENV-004: DEPLOYED_COMMIT matches $EXPECTED_COMMIT"
      else
        print_fail "ENV-004: DEPLOYED_COMMIT is $DEPLOYED_COMMIT (expected $EXPECTED_COMMIT)"
      fi
    else
      HEALTH_OUTPUT=$(curl -s "$STAGING_URL/healthz")
      if echo "$HEALTH_OUTPUT" | grep -q "$EXPECTED_COMMIT"; then
        print_pass "ENV-004: Deployed commit matches $EXPECTED_COMMIT in /healthz"
      else
        print_fail "ENV-004: Deployed commit $EXPECTED_COMMIT not found in /healthz or DEPLOYED_COMMIT"
      fi
    fi

    # REL-004
    V11_FAIL=0
    for route in "/finance/payments/execute" "/finance/treasury/settlement"; do
      CODE=$(curl -s -o /dev/null -w "%{http_code}" "$STAGING_URL$route")
      if [ "$CODE" != "404" ]; then
         V11_FAIL=1
      fi
    done
    if [ "$V11_FAIL" -eq 0 ]; then
      print_pass "REL-004: v0.11-finance routes return 404"
    else
      print_fail "REL-004: v0.11-finance routes did not return 404"
    fi

    # REL-005
    V10_FAIL=0
    for route in "/accounting/pnl" "/sales/orders" "/inventory/movements" "/documents" "/cmms/work-orders"; do
      CODE=$(curl -s -o /dev/null -w "%{http_code}" -L "$STAGING_URL$route")
      if [ "$CODE" != "200" ] && [ "$CODE" != "302" ]; then
         V10_FAIL=1
      fi
    done
    if [ "$V10_FAIL" -eq 0 ]; then
      print_pass "REL-005: Core v0.10 routes are accessible (200/302)"
    else
      print_fail "REL-005: Core v0.10 routes failed to return 200/302"
    fi

    # OPS-002
    HEADERS=$(curl -s -I "$STAGING_URL")
    HTML_BODY=$(curl -s "$STAGING_URL")
    
    OPS_FAIL=0
    if ! echo "$HEADERS" | grep -iqE "(x-frame-options|content-security-policy.*frame-ancestors)"; then
      OPS_FAIL=1
    fi
    if ! echo "$HEADERS" | grep -iq "x-content-type-options: nosniff"; then
      OPS_FAIL=1
    fi
    # Only checking headers if there's a set-cookie
    if echo "$HEADERS" | grep -iq "set-cookie:"; then
      if ! echo "$HEADERS" | grep -iq "secure"; then
        OPS_FAIL=1
      fi
    fi
    # Simplistic CSRF check in forms
    if echo "$HTML_BODY" | grep -iq "<form"; then
      if ! echo "$HTML_BODY" | grep -iq "csrf"; then
        OPS_FAIL=1
      fi
    fi
    
    if [ "$OPS_FAIL" -eq 0 ]; then
      print_pass "OPS-002: Security headers and CSRF tokens present"
    else
      print_fail "OPS-002: Security headers or CSRF tokens missing"
    fi

  else
    print_skip "ENV-003: STAGING_URL not set"
    print_skip "ENV-004: STAGING_URL not set"
    print_skip "REL-004: STAGING_URL not set"
    print_skip "REL-005: STAGING_URL not set"
    print_skip "OPS-002: STAGING_URL not set"
  fi
fi

# DB-001
if ! command -v psql >/dev/null 2>&1; then
  print_skip "DB-001: psql not found"
else
  if [ -n "$PG_DSN" ]; then
    LATEST_MIGRATION=$(psql "$PG_DSN" -t -c "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1;" 2>/dev/null | xargs)
    HAS_125=$(psql "$PG_DSN" -t -c "SELECT COUNT(*) FROM schema_migrations WHERE version >= 125;" 2>/dev/null | xargs)
    COUNT_MIGRATIONS=$(psql "$PG_DSN" -t -c "SELECT COUNT(*) FROM schema_migrations;" 2>/dev/null | xargs)
    
    DB1_FAIL=0
    if [ "$LATEST_MIGRATION" != "124" ] && [ "$LATEST_MIGRATION" != "000124" ]; then
      DB1_FAIL=1
    fi
    if [ "$HAS_125" != "0" ]; then
      DB1_FAIL=1
    fi
    
    if [ "$DB1_FAIL" -eq 0 ]; then
      print_pass "DB-001: Migration ceiling verified (latest: 000124, total applied: $COUNT_MIGRATIONS)"
    else
      print_fail "DB-001: Migration ceiling failed (latest: $LATEST_MIGRATION, has_125: $HAS_125)"
    fi
  else
    print_skip "DB-001: PG_DSN not set"
  fi
fi

# DB-002: Verify the rc.6 candidate has the correct migration ceiling
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# Check the tagged commit's migration list, not the working tree (which may
# contain post-v0.10 migrations on a development branch).
if git -C "$REPO_ROOT" rev-parse v0.10.0-rc.6 >/dev/null 2>&1; then
  MAX_RC6=$(git -C "$REPO_ROOT" ls-tree --name-only v0.10.0-rc.6:migrations/ \
    | grep -oE '^[0-9]+' | sort -n | tail -n 1)
  if [ "$MAX_RC6" = "124" ] || [ "$MAX_RC6" = "000124" ]; then
    print_pass "DB-002: rc.6 tag migration ceiling is 000124"
  else
    print_fail "DB-002: rc.6 tag migration ceiling is $MAX_RC6 (expected 000124)"
  fi
else
  # Fallback: check working tree
  MIGRATION_DIR="$REPO_ROOT/migrations"
  if [ -d "$MIGRATION_DIR" ]; then
    MAX_LOCAL=$(ls "$MIGRATION_DIR" | grep -oE '^[0-9]+' | sort -n | tail -n 1)
    print_fail "DB-002: No rc.6 tag found; working tree max migration is $MAX_LOCAL"
  else
    print_fail "DB-002: migrations directory not found and no rc.6 tag"
  fi
fi

echo "=== Summary: $PASSED/$TOTAL checks passed, $FAILED failed ==="
if [ "$SKIPPED" -gt 0 ]; then
  echo "($SKIPPED checks skipped)"
fi

if [ "$FAILED" -gt 0 ]; then
  exit 1
fi
exit 0
