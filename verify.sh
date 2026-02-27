#!/usr/bin/env bash
set -euo pipefail

# E2E verification of the ContextForge MCP Gateway service broker
# Usage: ./verify.sh <broker-url> <broker-username> <broker-password> [contextforge-url]

BROKER_URL="${1:?Usage: $0 <broker-url> <broker-username> <broker-password> [contextforge-url]}"
USERNAME="${2:?Usage: $0 <broker-url> <broker-username> <broker-password> [contextforge-url]}"
PASSWORD="${3:?Usage: $0 <broker-url> <broker-username> <broker-password> [contextforge-url]}"
CF_URL="${4:-}"

# Strip trailing slash
BROKER_URL="${BROKER_URL%/}"

INSTANCE_ID="test-instance-$(date +%s)"
BINDING_ID="test-binding-$(date +%s)"
SERVICE_ID="contextforge-mcp-gateway-service"
PLAN_ID="contextforge-mcp-gateway-standard"

PASS=0
FAIL=0

check() {
  local desc="$1"
  local expected_code="$2"
  shift 2
  local actual_code
  local body

  body=$(mktemp)
  actual_code=$(curl -s -o "$body" -w "%{http_code}" "$@")

  if [[ "$actual_code" == "$expected_code" ]]; then
    echo "PASS: $desc (HTTP $actual_code)"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $desc (expected HTTP $expected_code, got $actual_code)"
    cat "$body"
    echo ""
    FAIL=$((FAIL + 1))
  fi

  cat "$body"
  rm -f "$body"
  echo ""
}

echo "=== ContextForge Broker E2E Verification ==="
echo "Broker: $BROKER_URL"
echo ""

# 1. Health check
echo "--- Health Check ---"
check "Health endpoint" "200" \
  "${BROKER_URL}/health"

# 2. Catalog
echo "--- Catalog ---"
check "GET catalog" "200" \
  -u "${USERNAME}:${PASSWORD}" \
  -H "X-Broker-API-Version: 2.17" \
  "${BROKER_URL}/v2/catalog"

# 3. Provision
echo "--- Provision ---"
check "PUT provision instance" "201" \
  -X PUT \
  -u "${USERNAME}:${PASSWORD}" \
  -H "X-Broker-API-Version: 2.17" \
  -H "Content-Type: application/json" \
  -d "{\"service_id\":\"${SERVICE_ID}\",\"plan_id\":\"${PLAN_ID}\",\"organization_guid\":\"test-org\",\"space_guid\":\"test-space\"}" \
  "${BROKER_URL}/v2/service_instances/${INSTANCE_ID}"

# 4. Get instance
echo "--- Get Instance ---"
check "GET instance" "200" \
  -u "${USERNAME}:${PASSWORD}" \
  -H "X-Broker-API-Version: 2.17" \
  "${BROKER_URL}/v2/service_instances/${INSTANCE_ID}"

# 5. Last operation
echo "--- Last Operation ---"
check "GET last operation" "200" \
  -u "${USERNAME}:${PASSWORD}" \
  -H "X-Broker-API-Version: 2.17" \
  "${BROKER_URL}/v2/service_instances/${INSTANCE_ID}/last_operation"

# 6. Bind
echo "--- Bind ---"
BIND_RESPONSE=$(mktemp)
curl -s -o "$BIND_RESPONSE" -w "%{http_code}" \
  -X PUT \
  -u "${USERNAME}:${PASSWORD}" \
  -H "X-Broker-API-Version: 2.17" \
  -H "Content-Type: application/json" \
  -d "{\"service_id\":\"${SERVICE_ID}\",\"plan_id\":\"${PLAN_ID}\",\"app_guid\":\"test-app\"}" \
  "${BROKER_URL}/v2/service_instances/${INSTANCE_ID}/service_bindings/${BINDING_ID}" > /dev/null

BIND_BODY=$(cat "$BIND_RESPONSE")
rm -f "$BIND_RESPONSE"

echo "Bind response:"
echo "$BIND_BODY" | python3 -m json.tool 2>/dev/null || echo "$BIND_BODY"
echo ""

# Extract JWT token
JWT_TOKEN=$(echo "$BIND_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)['credentials']['jwt_token'])" 2>/dev/null || echo "")
if [[ -n "$JWT_TOKEN" ]]; then
  echo "PASS: JWT token extracted from binding"
  PASS=$((PASS + 1))

  # Decode JWT payload (base64url decode the middle segment)
  PAYLOAD=$(echo "$JWT_TOKEN" | cut -d. -f2 | tr '_-' '/+' | base64 -d 2>/dev/null || echo "decode-failed")
  echo "JWT payload: $PAYLOAD"
  echo ""

  # Test JWT against ContextForge if URL provided
  if [[ -n "$CF_URL" ]]; then
    echo "--- Test JWT against ContextForge ---"
    check "JWT auth against ContextForge /health" "200" \
      -H "Authorization: Bearer ${JWT_TOKEN}" \
      "${CF_URL}/health"
  fi
else
  echo "FAIL: Could not extract JWT token from binding response"
  FAIL=$((FAIL + 1))
fi

# 7. Get binding
echo "--- Get Binding ---"
check "GET binding" "200" \
  -u "${USERNAME}:${PASSWORD}" \
  -H "X-Broker-API-Version: 2.17" \
  "${BROKER_URL}/v2/service_instances/${INSTANCE_ID}/service_bindings/${BINDING_ID}"

# 8. Unbind
echo "--- Unbind ---"
check "DELETE unbind" "200" \
  -X DELETE \
  -u "${USERNAME}:${PASSWORD}" \
  -H "X-Broker-API-Version: 2.17" \
  "${BROKER_URL}/v2/service_instances/${INSTANCE_ID}/service_bindings/${BINDING_ID}?service_id=${SERVICE_ID}&plan_id=${PLAN_ID}"

# 9. Deprovision
echo "--- Deprovision ---"
check "DELETE deprovision" "200" \
  -X DELETE \
  -u "${USERNAME}:${PASSWORD}" \
  -H "X-Broker-API-Version: 2.17" \
  "${BROKER_URL}/v2/service_instances/${INSTANCE_ID}?service_id=${SERVICE_ID}&plan_id=${PLAN_ID}"

# 10. Verify instance is gone
echo "--- Verify Cleanup ---"
check "GET deprovisioned instance returns 404" "404" \
  -u "${USERNAME}:${PASSWORD}" \
  -H "X-Broker-API-Version: 2.17" \
  "${BROKER_URL}/v2/service_instances/${INSTANCE_ID}"

echo ""
echo "=== Results: ${PASS} passed, ${FAIL} failed ==="
if [[ "$FAIL" -gt 0 ]]; then
  exit 1
fi
