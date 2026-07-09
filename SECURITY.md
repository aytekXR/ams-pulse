# Security Policy

## Reporting a Vulnerability

Please report security vulnerabilities by email to **aytek@beyondkaira.com**.

Include a description of the issue, reproduction steps, and potential impact.
You will receive a response within 5 business days. Please do not open a public
GitHub issue for security vulnerabilities.

## Supported Versions

| Version | Supported |
|---|---|
| v0.1.x | Yes |
| < v0.1.0 | No |

## Security Design Overview

The claims below are code-verified against the live tree. Each claim cites the
file and line number that was inspected.

### Webhook HMAC authentication

Every AMS webhook request is authenticated with HMAC-SHA256:

- The `X-Ams-Signature` header must equal `sha256=` + hex(HMAC-SHA256(secret, body)).
- Comparison uses `hmac.Equal` (constant-time).
- An empty secret always returns `false` from `validateHMAC`; the handler returns
  `401` (fail-closed — not `404`). (Verified: `server/internal/collector/webhook/webhook.go:260-268`.)
- The legacy route (`/webhook/ams`) uses `PULSE_WEBHOOK_SECRET` globally.
- The per-source route (`/webhook/ams/{name}`) uses the named source's secret when present
  with no SharedSecret fallback — cross-source isolation. Unknown names fall back to
  SharedSecret or return `401`. (Verified: `webhook.go:126-163`.)
- `PULSE_WEBHOOK_SECRET` must be set when `PULSE_WEBHOOK_ADDR` is configured; the
  webhook listener is skipped (fail-closed) at startup if the secret is absent.
  (Verified: `server/cmd/pulse/serve.go:278-283`.)

### API token storage

API tokens are stored as **HMAC-SHA256(hmacKey, rawToken)** with `hash_alg='hmac-sha256'`
when `PULSE_SECRET_KEY` is configured. (Verified: `HashToken` in
`server/internal/store/meta/meta.go`; function-name citation used — line numbers
shift as the file grows.)

- The HMAC key is derived from the cipher key via domain-separated SHA-256.
- Legacy rows with `hash_alg='sha256'` (plain SHA-256, created before D-052) still
  authenticate via `LookupToken`'s fallback — upgrade is transparent.
  (Verified: `LookupToken` in `server/internal/store/meta/meta.go`.)
- Dev mode (no `PULSE_SECRET_KEY`, `:memory:` DSN) uses plain SHA-256.
- **Caution:** rotating `PULSE_SECRET_KEY` invalidates `hmac-sha256` tokens; operators must
  re-issue tokens after key rotation.

### Secret environment variables — `_FILE` convention

All secret-bearing environment variables support a `<VAR>_FILE` variant: when set, the
value is read from the named file (file wins over env; missing file is a hard startup error).
This allows secrets to be mounted as Docker tmpfs files rather than injected as env vars.

Supported variables: `PULSE_SECRET_KEY`, `PULSE_WEBHOOK_SECRET`, `PULSE_AMS_LOGIN_PASSWORD`,
`PULSE_METRICS_TOKEN`, `PULSE_AMS_AUTH_TOKEN`, and per-source `PULSE_AMS_<NAME>_TOKEN`.
(Verified: `server/internal/config/secrets.go:27` `GetSecret` implementation.)

**Exception:** `PULSE_LICENSE_KEY` is read via `os.Getenv` directly and does NOT support
the `_FILE` convention. (Verified: `server/cmd/pulse/serve.go:236`.)

### Startup key validation (fail-closed)

For non-`:memory:` meta store DSNs, `PULSE_SECRET_KEY` must be set and at least 16 bytes.
An empty key or a key shorter than 16 bytes causes the server to refuse to start with an
actionable error message. (Verified: `server/cmd/pulse/serve.go:256-260`.)

### Content Security Policy

CSP is enforced by Caddy (not Go middleware). Two Caddyfiles are in use; they carry
**different** policies:

**Development / base overlay** (`deploy/config/Caddyfile:71`):

```
Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self' wss://{$PULSE_DOMAIN:localhost} https://{$PULSE_DOMAIN:localhost}; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
```

**Production overlay** (`deploy/config/Caddyfile.prod:78`, mounted at
`deploy/docker-compose.prod-tls.yml:37`):

```
Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self' wss://{$PULSE_DOMAIN} wss://pulse.{$PULSE_DOMAIN} https://{$PULSE_DOMAIN} https://pulse.{$PULSE_DOMAIN}; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
```

The production policy adds `wss://pulse.{$PULSE_DOMAIN}` and `https://pulse.{$PULSE_DOMAIN}`
(the pulse subdomain) and removes the `:localhost` fallbacks present in the base Caddyfile.

The `csp-e2e` CI job (`deploy/config/Caddyfile.ci:69`) validates that Caddy serves the
CI-specific policy (`connect-src 'self' ws://localhost:18080`) against a live Caddy stack.
It does not assert parity with the base or production Caddyfiles.
(Verified: `grep -n 'Content-Security-Policy' deploy/config/Caddyfile{,.prod,.ci}`, D-061.)

### License gates — fail-closed (403)

Gated features return `403 LICENSE_REQUIRED` when the active license tier is insufficient:

| Feature | Minimum tier | Handler location |
|---|---|---|
| `/metrics` (Prometheus) | Business | `server/internal/api/server.go:688-690`; `license.go:351-357` |
| Usage/billing reports | Business | `license.go:328-333` |
| Multi-tenant billing | Business | `license.go:317-322` |
| QoE beacon ingest | Pro | `license.go:339-344` |

The default tier when no license key is configured is **Free** (not a startup failure).
A license init error is logged as WARN and falls back to Free tier.

### Network exposure

ClickHouse ports 9000 (native) and 8123 (HTTP) are declared with `expose:` in the base
`docker-compose.yml`, not `ports:`. This means they are cluster-internal only and never
bound to the host network. External access to ClickHouse is not possible without explicitly
publishing a port. (Verified: `deploy/docker-compose.yml:100-102`.)

The meta store (SQLite) is a file on the `pulse-data` Docker volume and is not network-accessible.

### Rate limiting

`/metrics`: 10 rps / burst 20, per IP (enforced before token check).
`/ingest/beacon` (main port): 100 rps / burst 200, per token.
API routes: per-user rate limiting via middleware.
(Verified: `server/internal/api/ratelimit.go:20-21`; `server/internal/api/server.go`.)

## License

A `LICENSE` file has not yet been added to this repository. The operator has not selected
a license (operator item O5, deferred). This does not affect the security posture described
above.
