# WO-A3 (S105) — amsclient: send ProxyAuthorization for management-scope endpoints

**Review finding (Issue 3, P1, verified):** with `PULSE_AMS_AUTH_TOKEN` (static-token mode),
Pulse sends `Authorization: Bearer <token>` on every request. That is correct for app-scope
REST (`/{app}/rest/v2/...`, AMS `jwtControlEnabled`), but the AMS **web-panel/management** REST
API (`server.jwtServerControlEnabled=true`) expects the JWT in a **`ProxyAuthorization`** header.
Pulse calls five management-scope endpoints every poll (`/rest/v2/applications`,
`/rest/v2/cluster/nodes`, `/rest/v2/cluster/nodes/{id}`, `/rest/v2/system-status`,
`/rest/v2/version`), so token-only mode fails exactly where fleet data comes from. The
cookie-session path is unaffected.

## Scope — you may ONLY edit these files
- `server/pkg/amsclient/client.go`
- `server/pkg/amsclient/*_test.go` (extend an existing test file or add `auth_header_test.go`)

Do NOT touch docs (a parallel agent owns them), do NOT run git commands.

## Work items

1. In `doGet` (client.go ~lines 333-348), when `c.authHeader != ""`, keep setting
   `Authorization: <authHeader>` AND also set
   `ProxyAuthorization: <token without the "Bearer " prefix>` (use `strings.TrimPrefix`).
   Comment: app-scope REST reads Authorization Bearer (jwtControlEnabled); management/web-panel
   REST reads ProxyAuthorization (server.jwtServerControlEnabled); sending both is harmless —
   each side ignores the header it doesn't use.

2. Tests (httptest): with AuthToken configured, a GET carries BOTH headers with correct values
   (ProxyAuthorization has no Bearer prefix); with no AuthToken, NEITHER header is present;
   cookie-session mode unaffected.

## Definition of done
- `gofmt -l` clean; `cd server && go build ./... && go test ./pkg/amsclient/...` green.
- Return: files changed, test results, any deviation.
