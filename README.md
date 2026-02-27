# ContextForge on Cloud Foundry

Deploy [ContextForge](https://github.com/IBM/mcp-context-forge) MCP Gateway to Cloud Foundry with a service broker that lets developers self-service gateway credentials.

## Overview

This repo contains two components:

| Directory | What it does |
|-----------|-------------|
| **`contextforge-cf-deploy/`** | Deploys the ContextForge MCP Gateway app (Python/FastAPI) with PostgreSQL and Redis |
| **Root (`./`)** | Open Service Broker that provisions JWT credentials for bound apps |

Once both are deployed, CF developers can:

```bash
cf create-service contextforge-mcp-gateway standard my-gateway
cf bind-service my-app my-gateway
cf restage my-app
```

Their app receives credentials in `VCAP_SERVICES`:

```json
{
  "contextforge-mcp-gateway": [{
    "credentials": {
      "url": "https://contextforge.<apps-domain>",
      "mcp_url": "https://contextforge.<apps-domain>/mcp",
      "username": "cf-binding-abc123...",
      "jwt_token": "eyJhbG...",
      "uri": "https://contextforge.<apps-domain>"
    }
  }]
}
```

## Prerequisites

- CF CLI v8+
- A CF foundation with Python buildpack, Go buildpack, PostgreSQL, and Redis in the marketplace

## Deploy

### 1. Generate secrets

```bash
openssl rand -base64 32   # → admin_password / broker_password
openssl rand -hex 64      # → jwt_secret_key
openssl rand -hex 32      # → encryption_secret
```

### 2. Configure

Copy the vars templates and fill in real values. The `jwt_secret_key` must match across both files.

```bash
cp contextforge-cf-deploy/vars.yml contextforge-cf-deploy/vars-local.yml
cp vars.yml vars-local.yml
# Edit both vars-local.yml files with generated secrets
```

### 3. Deploy ContextForge

```bash
cd contextforge-cf-deploy
./deploy.sh postgres/on-demand-postgres-db p-redis/shared-vm
# Or interactively: ./deploy.sh
```

Or manually:

```bash
cf create-service postgres on-demand-postgres-db contextforge-db
cf create-service p-redis shared-vm contextforge-cache
cf push --vars-file vars-local.yml
```

Verify: `curl https://contextforge.<apps-domain>/health`

### 4. Deploy the service broker

```bash
cd ..   # back to repo root
cf push --vars-file vars-local.yml
```

### 5. Register the broker

```bash
./register-broker.sh broker-admin <broker_password>
```

### 6. Verify

```bash
cf marketplace | grep contextforge
cf create-service contextforge-mcp-gateway standard test-gw
cf bind-service contextforge test-gw
cf env contextforge | grep jwt_token
cf unbind-service contextforge test-gw
cf delete-service test-gw -f
```

Or run the E2E script:

```bash
./verify.sh https://contextforge-broker.<apps-domain> broker-admin <broker_password>
```

## Architecture

```
┌─────────────────┐     cf create-service     ┌──────────────────────┐
│  CF Developer   │ ──────────────────────────▶│  contextforge-broker │
│                 │     cf bind-service        │  (Go, OSB API)       │
│                 │◀────────────────────────── │  Generates JWT       │
│                 │     VCAP_SERVICES creds    └──────────────────────┘
│                 │                                     │
│  my-app         │     Authorization: Bearer <jwt>     │ signs with
│  (reads creds)  │ ──────────────────────────────┐     │ JWT_SECRET_KEY
└─────────────────┘                               ▼     ▼
                                           ┌──────────────────┐
                                           │   contextforge    │
                                           │   (Python/FastAPI)│
                                           │   MCP Gateway     │
                                           └──────────────────┘
                                             │              │
                                        ┌────┴───┐    ┌────┴────┐
                                        │Postgres│    │  Redis  │
                                        └────────┘    └─────────┘
```

## Broker Configuration

The broker reads `broker-config.yml` and applies environment variable overrides:

| Env Var | Description |
|---------|-------------|
| `BROKER_USERNAME` | Broker basic auth username |
| `BROKER_PASSWORD` | Broker basic auth password |
| `CONTEXTFORGE_URL` | ContextForge gateway URL |
| `CONTEXTFORGE_MCP_URL` | MCP endpoint URL (defaults to URL + /mcp) |
| `CONTEXTFORGE_ADMIN_USER` | ContextForge admin username |
| `CONTEXTFORGE_ADMIN_PASSWORD` | ContextForge admin password |
| `CONTEXTFORGE_JWT_SECRET_KEY` | JWT signing secret (must match ContextForge) |

## ContextForge Configuration

See [`contextforge-cf-deploy/README.md`](contextforge-cf-deploy/README.md) for environment variables, scaling, and troubleshooting.

## File Structure

```
.
├── main.go                         # Broker entry point
├── go.mod / go.sum
├── config/config.go                # YAML config + env var overlay
├── broker/
│   ├── broker.go                   # OSB handlers + JWT generation
│   ├── icon.go                     # go:embed for marketplace icon
│   └── icon.png                    # ContextForge logo (128x128)
├── store/store.go                  # File-based JSON state store
├── manifest.yml                    # CF manifest for the broker
├── broker-config.yml               # Catalog + config template
├── Procfile
├── vars.yml                        # Broker secrets template
├── register-broker.sh              # cf create-service-broker helper
├── verify.sh                       # E2E curl tests
├── .cfignore
├── .gitignore
└── contextforge-cf-deploy/
    ├── manifest.yml                # CF manifest for ContextForge
    ├── requirements.txt            # Python deps (PyPI install)
    ├── runtime.txt                 # Python 3.12.x
    ├── Procfile
    ├── cf_env.sh                   # VCAP_SERVICES parser + Gunicorn
    ├── deploy.sh                   # One-command deploy script
    ├── vars.yml                    # ContextForge secrets template
    └── README.md                   # ContextForge-specific docs
```
