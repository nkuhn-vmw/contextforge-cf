#!/usr/bin/env bash
# cf_env.sh - Parse VCAP_SERVICES and start the application
#
# Cloud Foundry injects service credentials via the VCAP_SERVICES environment
# variable. This script extracts DATABASE_URL and REDIS_URL from that JSON
# and exports them before starting Gunicorn.

set -euo pipefail

if [ -n "${VCAP_SERVICES:-}" ]; then
    echo "Parsing VCAP_SERVICES for service credentials..."

    # Extract DATABASE_URL from PostgreSQL service binding
    export DATABASE_URL=$(python3 -c "
import json, os, sys
vcap = json.loads(os.environ['VCAP_SERVICES'])
# Try common PostgreSQL service labels
for label in ['postgresql', 'elephantsql', 'postgres', 'crunchy-bridge',
              'user-provided']:
    if label in vcap:
        for svc in vcap[label]:
            creds = svc.get('credentials', {})
            uri = creds.get('uri') or creds.get('database_url') or creds.get('DATABASE_URL', '')
            if uri and ('postgres' in uri):
                # Ensure we use postgresql:// scheme (not postgres://)
                print(uri.replace('postgres://', 'postgresql://', 1))
                sys.exit(0)
print('', end='')
")

    # Extract REDIS_URL from Redis service binding
    export REDIS_URL=$(python3 -c "
import json, os, sys
vcap = json.loads(os.environ['VCAP_SERVICES'])
for label in ['redis', 'rediscloud', 'p-redis', 'p.redis', 'user-provided']:
    if label in vcap:
        for svc in vcap[label]:
            creds = svc.get('credentials', {})
            # Try uri field first
            uri = creds.get('uri') or creds.get('redis_url') or creds.get('REDIS_URL', '')
            if uri and ('redis' in uri):
                print(uri)
                sys.exit(0)
            # Construct from host/port/password (e.g., p-redis shared-vm)
            host = creds.get('host', '')
            port = creds.get('port', 6379)
            password = creds.get('password', '')
            if host:
                if password:
                    print(f'redis://:{password}@{host}:{port}/0')
                else:
                    print(f'redis://{host}:{port}/0')
                sys.exit(0)
print('', end='')
")

    if [ -n "$DATABASE_URL" ]; then
        echo "DATABASE_URL configured from VCAP_SERVICES"
    else
        echo "WARNING: Could not extract DATABASE_URL from VCAP_SERVICES"
    fi

    if [ -n "$REDIS_URL" ]; then
        echo "REDIS_URL configured from VCAP_SERVICES"
    else
        echo "WARNING: Could not extract REDIS_URL from VCAP_SERVICES"
    fi
else
    echo "No VCAP_SERVICES found (running outside CF?). Using existing environment variables."
fi

# Use CF-assigned PORT (defaults to 8080 for local testing)
PORT="${PORT:-8080}"

echo "Starting ContextForge on 0.0.0.0:${PORT} with 2 workers..."

exec gunicorn \
    --worker-class uvicorn.workers.UvicornWorker \
    --bind "0.0.0.0:${PORT}" \
    --workers 2 \
    --timeout 600 \
    --access-logfile - \
    --error-logfile - \
    mcpgateway.main:app
