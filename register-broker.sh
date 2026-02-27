#!/usr/bin/env bash
set -euo pipefail

# Register (or update) the ContextForge MCP Gateway service broker
# Usage: ./register-broker.sh <broker-username> <broker-password>

BROKER_NAME="contextforge-broker"

USERNAME="${1:?Usage: $0 <broker-username> <broker-password>}"
PASSWORD="${2:?Usage: $0 <broker-username> <broker-password>}"

# Detect broker URL from cf app
BROKER_URL=$(cf app "$BROKER_NAME" | grep routes | awk '{print $2}')
if [[ -z "$BROKER_URL" ]]; then
  echo "ERROR: Could not detect route for app '$BROKER_NAME'. Is it pushed?"
  exit 1
fi
BROKER_URL="https://${BROKER_URL}"

echo "Broker URL: $BROKER_URL"

# Check if broker already exists
if cf service-brokers | grep -q "$BROKER_NAME"; then
  echo "Updating existing service broker..."
  cf update-service-broker "$BROKER_NAME" "$USERNAME" "$PASSWORD" "$BROKER_URL"
else
  echo "Creating space-scoped service broker..."
  cf create-service-broker "$BROKER_NAME" "$USERNAME" "$PASSWORD" "$BROKER_URL" --space-scoped
fi

echo ""
echo "Service broker registered. Checking marketplace..."
echo ""
cf marketplace | grep -i contextforge || echo "NOTE: Broker registered but not yet visible in marketplace."
