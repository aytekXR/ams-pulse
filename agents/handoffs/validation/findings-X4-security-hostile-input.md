# X4-security-hostile-input Validation Findings

Verifier: QA-01 subagent (adversarial), 2026-06-14, branch main

## FINDING-S1 [CRITICAL] Duplicate beacon handler on main port has no rate limit, no schema validation, and silently discards all events

The package comment in server.go line 4 says "beacon ingest route (delegating to collector/beacon)" but this is false. The main API server registers its own `handleIngestBeacon` (api/server.go:1262) that has no rate limiting, no schema validation, and no EventSink write. The hardened beacon.Handler (collector/beacon/beacon.go) is only instantiated when PULSE_INGEST_LISTEN_ADDR is set. In default deployment the secure path is never active. All beacon data sent to :8090 is silently discarded.

## FINDING-S2 [CRITICAL] SDK sends X-Pulse-Token but server reads X-Pulse-Ingest-Token

sdk/beacon-js/src/transport.ts:138 sends 'X-Pulse-Token'. Server reads X-Pulse-Ingest-Token (beacon.go:188, api/server.go:1263). CORS allow-headers only include X-Pulse-Ingest-Token. Every SDK request gets 401. sendBeacon path sends no auth header at all (Blob limitation). No transport test verifies header names.

## FINDING-S3 [MAJOR] /metrics token compared with non-constant-time != operator

api/server.go:462: `extractBearerToken(r) != s.cfg.MetricsToken` — Go string != is not constant-time. ARCH §6 states constant-time auth. Metrics token is compared raw (not hashed), enabling timing oracle.

## FINDING-S4 [MAJOR] WebSocket upgrade uses InsecureSkipVerify=true — disables origin enforcement

api/server.go:567: `websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})`. This disables nhooyr/websocket's built-in cross-origin rejection. Combined with ACAO: * on the main router, a cross-origin page with a captured token can subscribe to live dashboard WebSocket from any domain.

## FINDING-S5 [MAJOR] GAP-2-004 confirmed: beacon ingest has no license tier gate (acknowledged)

handleIngestBeacon in api/server.go has no s.lic.CheckTier() call. PRD §7.11: beacons are Pro+. Already filed as GAP-2-004. Confirming it is still open.

## FINDING-S6 [MINOR] Body size cap mismatch: beacon.go uses 64 KB, OpenAPI contract specifies 256 KB

beacon.go:38 `maxBodyBytes = 64 * 1024` with comment "per spec". OpenAPI contract (pulse-api.yaml:820) says 256 KB. TestBeacon_OverSize_413 uses 70 KB body and expects 413 — would be a false rejection per contract.

## FINDING-S7 [MINOR] TestBeacon_CORS_Headers is vacuous — CORS headers never verified

test calls h.Handle() directly bypassing corsMiddlewareBeacon, unconditionally logs PASS. No assertion on response headers. corsMiddlewareBeacon also missing Vary: Origin when echoing Origin.

## FINDING-S8 [MINOR] TestBeacon_SinkReceivesEvent spin-polls without sleep, masks broken sink writes

Spin loop with no sleep; if goroutine hasn't run, logs NOTE and passes. Does not catch FINDING-S1 (main API discards all events).

## Verified Correct

- AES-256-GCM encryption at rest for secrets (meta.go). pulse_secret.key files on disk but NOT tracked by git (git ls-files returns empty). 
- Tokens never stored plaintext: SHA-256 hash stored, raw token printed to stderr once on bootstrap/creation.
- User passwords use bcrypt (hashPassword with DefaultCost=10). Legacy sha256 constant-time fallback in checkPassword.
- Token not echoed in error responses: verified by TestBeacon_InvalidToken_401.
- Body cap + rate limit + schema validation all correct in beacon.Handler (only active on dedicated port).
- Alert channel and AMS source credentials encrypted via store.Encrypt() before write.
