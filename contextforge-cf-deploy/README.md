# ContextForge CF Deployment

Deploy [IBM ContextForge](https://github.com/IBM/mcp-context-forge) (mcp-context-forge) as an enterprise MCP gateway on Cloud Foundry with PostgreSQL and Redis managed services.

This is a **standalone deployment repo** — the ContextForge application is installed from PyPI (`mcp-contextforge-gateway`). No need to clone the IBM repository.

## Prerequisites

- **CF CLI v8+** — [Install guide](https://docs.cloudfoundry.org/cf-cli/install-go-cli.html)
- **CF target** with Python buildpack support
- **PostgreSQL service** in marketplace (e.g., `postgres`, `elephantsql`, `crunchy-bridge`)
- **Redis service** in marketplace (e.g., `p-redis`, `p.redis`, `rediscloud`)

Verify your environment:

```bash
cf login -a https://api.<your-cf-domain>
cf marketplace   # confirm PostgreSQL and Redis are available
```

## Quick Start

1. **`cd` into this directory:**

   ```bash
   cd contextforge-cf-deploy
   ```

2. **Configure secrets** — edit `vars.yml`:

   ```bash
   # Generate strong secrets
   openssl rand -base64 32   # admin_password
   openssl rand -hex 64      # jwt_secret_key
   openssl rand -hex 32      # encryption_secret

   # Paste the generated values into vars.yml
   ```

3. **Deploy** using the helper script:

   ```bash
   chmod +x deploy.sh
   ./deploy.sh
   ```

   Or pass service plans non-interactively:

   ```bash
   ./deploy.sh postgres/on-demand-postgres-db p-redis/shared-vm
   ```

   The script creates both service instances, waits for provisioning, pushes the app, and prints the URL.

4. **Or deploy manually:**

   ```bash
   # Create services (use your marketplace's service/plan names)
   cf create-service postgres on-demand-postgres-db contextforge-db
   cf create-service p-redis shared-vm contextforge-cache

   # Wait for the on-demand PostgreSQL instance to finish provisioning
   watch cf service contextforge-db

   # Push
   cf push --vars-file vars.yml
   ```

5. **Verify:**

   ```bash
   curl https://contextforge.<cf-domain>/health
   # {"status":"healthy"}
   ```

6. **Open the admin UI** at `https://contextforge.<cf-domain>/admin/login` and log in with:
   - Email: the `admin_email` from your `vars.yml` (default: `admin@example.com`)
   - Password: the `admin_password` from your `vars.yml`
   - On first login you will be prompted to change the password

## Environment Variables

ContextForge uses **unprefixed** env var names (via pydantic-settings with no `env_prefix`). The manifest sets these through `vars.yml` substitution and static values.

### Set via `vars.yml`

| Variable | Description |
|----------|-------------|
| `PLATFORM_ADMIN_EMAIL` | Admin UI login email (default: `admin@example.com`) |
| `PLATFORM_ADMIN_PASSWORD` | Admin UI login password |
| `BASIC_AUTH_PASSWORD` | REST API basic auth password (set to same value) |
| `JWT_SECRET_KEY` | JWT signing key (min 32 chars) |
| `AUTH_ENCRYPTION_SECRET` | Encryption key for stored secrets (min 32 chars) |

### Set in `manifest.yml` (static)

| Variable | Value | Description |
|----------|-------|-------------|
| `MCPGATEWAY_UI_ENABLED` | `true` | Enable the HTMX admin dashboard at `/admin/` |
| `MCPGATEWAY_ADMIN_API_ENABLED` | `true` | Mount admin REST API routes |
| `BASIC_AUTH_USER` | `admin` | REST API basic auth username |
| `APP_DOMAIN` | `https://contextforge.<domain>` | CORS origin for admin UI |
| `CACHE_TYPE` | `redis` | Use Redis for caching |
| `SECURE_COOKIES` | `true` | Secure cookie flag (CF terminates TLS) |
| `LOG_LEVEL` | `INFO` | Log level |
| `LOG_FORMAT` | `json` | Structured JSON logging |

### Auto-configured from VCAP_SERVICES

| Variable | Source |
|----------|--------|
| `DATABASE_URL` | Extracted from PostgreSQL service binding by `cf_env.sh` |
| `REDIS_URL` | Extracted from Redis service binding by `cf_env.sh` |

## How It Works

1. **Service bindings** — CF injects `VCAP_SERVICES` JSON with PostgreSQL and Redis credentials.
2. **`cf_env.sh`** — On startup, parses `VCAP_SERVICES` to extract `DATABASE_URL` and `REDIS_URL`, then exec's Gunicorn. Handles multiple service label formats (e.g., `postgres`, `p-redis`) and constructs Redis URLs from `host`/`port`/`password` when no `uri` field is present.
3. **Gunicorn + Uvicorn** — Runs the FastAPI app with 2 async workers on the CF-assigned `$PORT`.
4. **Alembic migrations** — ContextForge runs database migrations automatically on startup.
5. **Health checks** — CF monitors `GET /health` and restarts the app if it becomes unhealthy.

## Scaling

```bash
# Horizontal — add instances
cf scale contextforge -i 3

# Vertical — more memory per instance
cf scale contextforge -m 4G

# Adjust workers per instance
cf set-env contextforge WEB_CONCURRENCY 4
cf restage contextforge
```

**Memory guidelines:**
- 2GB: 2 workers (default, ~215MB idle per instance)
- 4GB: 4 workers

## Troubleshooting

### App won't start

```bash
cf logs contextforge --recent
cf events contextforge
```

Common causes:
- **Service not ready** — on-demand PostgreSQL takes ~5 minutes to provision. Check: `cf service contextforge-db`
- **Missing `psycopg2`** — ensure `psycopg2-binary` is in `requirements.txt` (ContextForge uses psycopg2, not psycopg v3)
- **Out of memory** — increase with `cf scale contextforge -m 4G`

### Database connection errors

```bash
# Verify service binding
cf env contextforge   # look at VCAP_SERVICES

# Check extracted DATABASE_URL
cf ssh contextforge -c "echo \$DATABASE_URL"
```

### Redis connection errors

The `p-redis` shared-vm plan provides `host`/`port`/`password` (no `uri` field). `cf_env.sh` constructs the URL automatically. Verify:

```bash
cf ssh contextforge -c "echo \$REDIS_URL"
```

To fall back to in-memory cache temporarily:

```bash
cf set-env contextforge CACHE_TYPE simple
cf restart contextforge
```

### Admin UI not loading

Ensure these env vars are set:

```bash
cf env contextforge | grep -E "MCPGATEWAY_UI_ENABLED|MCPGATEWAY_ADMIN_API_ENABLED"
```

Both must be `true`. If you changed them, restart:

```bash
cf restart contextforge
```

### Health check timeout on first deploy

Alembic migrations may take time on first deploy. Increase the startup timeout:

```bash
cf set-health-check contextforge http --endpoint /health --invocation-timeout 300
cf restart contextforge
```

### Viewing app environment

```bash
cf env contextforge          # all environment variables
cf ssh contextforge          # SSH into the container
cf logs contextforge -f      # stream live logs
```

## File Structure

```
contextforge-cf-deploy/
├── manifest.yml        # CF application manifest
├── requirements.txt    # Python dependencies (installs from PyPI)
├── Procfile            # Process type declaration
├── runtime.txt         # Python version (3.12.x)
├── cf_env.sh           # VCAP_SERVICES parser + Gunicorn launcher
├── deploy.sh           # One-command deployment script
├── vars.yml            # Secrets template (fill before deploying)
├── .cfignore           # Files excluded from cf push upload
└── README.md           # This file
```
