#!/usr/bin/env bash
set -euo pipefail

# ContextForge CF Deployment Script
#
# Usage:
#   ./deploy.sh                                    # interactive — prompts for service plans
#   ./deploy.sh postgres/on-demand-postgres-db p-redis/shared-vm   # non-interactive

VARS_FILE="vars.yml"

echo "=== ContextForge CF Deployment ==="
echo ""

# --- Prerequisites -----------------------------------------------------------

if ! command -v cf &>/dev/null; then
    echo "ERROR: CF CLI not found. Install from https://docs.cloudfoundry.org/cf-cli/install-go-cli.html"
    exit 1
fi

if ! cf target &>/dev/null; then
    echo "ERROR: Not logged in to CF. Run 'cf login' first."
    exit 1
fi

if [ ! -f "$VARS_FILE" ]; then
    echo "ERROR: $VARS_FILE not found. Copy vars.yml and fill in your secrets."
    exit 1
fi

if grep -q "CHANGE_ME" "$VARS_FILE"; then
    echo "WARNING: vars.yml still contains placeholder values (CHANGE_ME)."
    echo "Please update all secrets in vars.yml before deploying to production."
    read -rp "Continue anyway? (y/N) " confirm
    if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
        echo "Aborted."
        exit 1
    fi
fi

# --- Determine service plans --------------------------------------------------

if [ $# -ge 2 ]; then
    # Non-interactive: parse service/plan from positional args
    PG_SERVICE="${1%%/*}"
    PG_PLAN="${1##*/}"
    REDIS_SERVICE="${2%%/*}"
    REDIS_PLAN="${2##*/}"
else
    echo ""
    echo "Available PostgreSQL services:"
    cf marketplace -e postgres 2>/dev/null || echo "  (could not list — specify manually)"
    echo ""
    read -rp "Enter PostgreSQL service/plan (e.g., postgres/on-demand-postgres-db): " pg_input
    PG_SERVICE="${pg_input%%/*}"
    PG_PLAN="${pg_input##*/}"

    echo ""
    echo "Available Redis services:"
    cf marketplace -e p-redis 2>/dev/null || cf marketplace -e p.redis 2>/dev/null || echo "  (could not list — specify manually)"
    echo ""
    read -rp "Enter Redis service/plan (e.g., p-redis/shared-vm): " redis_input
    REDIS_SERVICE="${redis_input%%/*}"
    REDIS_PLAN="${redis_input##*/}"
fi

# --- Create services (idempotent) --------------------------------------------

echo ""
echo "--- Creating PostgreSQL service: contextforge-db ---"
cf create-service "${PG_SERVICE}" "${PG_PLAN}" contextforge-db 2>/dev/null \
    && echo "Created contextforge-db" \
    || echo "Service contextforge-db already exists (or creation pending)"

echo ""
echo "--- Creating Redis service: contextforge-cache ---"
cf create-service "${REDIS_SERVICE}" "${REDIS_PLAN}" contextforge-cache 2>/dev/null \
    && echo "Created contextforge-cache" \
    || echo "Service contextforge-cache already exists (or creation pending)"

# --- Wait for services -------------------------------------------------------

echo ""
echo "--- Waiting for services to be ready ---"
for svc in contextforge-db contextforge-cache; do
    echo -n "Waiting for $svc..."
    for i in $(seq 1 60); do
        svc_status=$(cf service "$svc" 2>&1 | grep "status:" | head -1 || true)
        if echo "$svc_status" | grep -q "create succeeded"; then
            echo " ready!"
            break
        fi
        if echo "$svc_status" | grep -q "create failed"; then
            echo " FAILED"
            echo "ERROR: Service $svc creation failed. Check with: cf service $svc"
            exit 1
        fi
        echo -n "."
        sleep 5
    done
done

# --- Push ---------------------------------------------------------------------

echo ""
echo "--- Pushing ContextForge ---"
cf push --vars-file "$VARS_FILE"

# --- Results ------------------------------------------------------------------

echo ""
echo "=== Deployment Complete ==="
echo ""

APP_URL=$(cf app contextforge | grep "routes:" | awk '{print $2}')
echo "App URL:    https://${APP_URL}"
echo "Health:     https://${APP_URL}/health"
echo "Admin UI:   https://${APP_URL}/admin/"
echo ""

echo "--- Health Check ---"
curl -sf "https://${APP_URL}/health" 2>/dev/null && echo "" || echo "(health endpoint not yet reachable — app may still be starting)"

echo ""
echo "--- Next Steps ---"
echo "1. Check logs:    cf logs contextforge --recent"
echo "2. Open admin UI: https://${APP_URL}/admin/login"
echo "3. Log in with:   <admin_email> / <admin_password> from vars.yml"
echo "   Default email:  admin@example.com"
echo "4. Scale:         cf scale contextforge -i 2"
echo ""
