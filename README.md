# ContextForge MCP Gateway Service Broker

Open Service Broker API implementation for the ContextForge MCP Gateway. Allows Cloud Foundry developers to provision gateway access and receive JWT credentials via `VCAP_SERVICES`.

## Usage

```bash
cf create-service contextforge-mcp-gateway standard my-gateway
cf bind-service my-app my-gateway
cf restage my-app
```

Your app receives credentials in `VCAP_SERVICES`:

```json
{
  "contextforge-mcp-gateway": [{
    "credentials": {
      "url": "https://contextforge.apps.tas-ndc.kuhn-labs.com",
      "mcp_url": "https://contextforge.apps.tas-ndc.kuhn-labs.com/mcp",
      "username": "cf-binding-abc123...",
      "jwt_token": "eyJhbG...",
      "uri": "https://contextforge.apps.tas-ndc.kuhn-labs.com"
    }
  }]
}
```

## Deploy

1. Copy `vars.yml` and fill in secrets:
   ```bash
   cp vars.yml my-vars.yml
   # Edit my-vars.yml with real values
   ```

2. Push the broker:
   ```bash
   cf push --vars-file my-vars.yml
   ```

3. Register the broker:
   ```bash
   ./register-broker.sh broker-admin <password>
   ```

4. Verify:
   ```bash
   cf marketplace | grep contextforge
   ```

## E2E Verification

```bash
./verify.sh https://contextforge-broker.apps.tas-ndc.kuhn-labs.com broker-admin <password>
```

## Configuration

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
| `PORT` | Listen port (set by CF) |
