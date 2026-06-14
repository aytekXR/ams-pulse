# ADR 0005: Beacon ingest security posture

**Status:** Accepted · **Date:** 2026-06-14 (Wave 2)

## Context

The beacon ingest endpoint (`POST /ingest/beacon`) is the only Pulse surface
that receives data from the open internet (player clients can be on any network).
It is distinct from the admin API (internal use only) and must resist:

- Credential brute-force (ingest token enumeration)
- Resource exhaustion (large payloads, request floods)
- Data poisoning (malformed or oversized event batches)
- Token disclosure (leaking tokens in error responses)

The endpoint is served on a dedicated listen address (`PULSE_INGEST_LISTEN_ADDR`,
default `:8091`) to allow separate DMZ/firewall treatment from the admin API.

## Decision

The beacon ingest endpoint implements four independent security layers:

1. **Token authentication (constant-time)**  
   Incoming `X-Pulse-Ingest-Token` headers are hashed with SHA-256 and compared
   against the stored hash using a constant-time map lookup. The raw token is
   never echoed in any response path — 401 responses return `{"error":"unauthorized"}`
   with no further detail.

2. **Tokens at rest (SHA-256)**  
   Ingest tokens are stored as SHA-256 hex in the meta store, never as plaintext.
   The `meta.Store` never writes the raw token value.

3. **Rate limiting (token bucket per token ID)**  
   Each ingest token has an independent token bucket limiter. Default: 100 req/s,
   burst 200. Requests exceeding the burst return HTTP 429.

4. **Body size cap**  
   `http.MaxBytesReader(64 KB)` is applied before any JSON decoding. Requests
   exceeding 64 KB return HTTP 413 without reading the body.

5. **Schema validation**  
   All event batches are validated against the `beacon-event.schema.json` rules
   before being accepted. Invalid payloads return HTTP 422 with a structured
   error array.

6. **Async write**  
   The accepted response (202) is returned before the event is written to
   ClickHouse: `go sink.WriteBeaconEvent(evt)`. This prevents ClickHouse latency
   from affecting the player.

7. **CORS**  
   Any origin is allowed for browser SDKs. No cookies or credentials are involved
   (token is in a custom header, not `Authorization`).

## Rationale

- **SHA-256 for token auth** (not bcrypt): beacon auth does not require bcrypt's
  slow hash because we are not defending against offline dictionary attacks on a
  leaked database. The token is a 128-bit random value; SHA-256 provides a fast
  constant-time lookup with sufficient entropy. bcrypt is used for user password
  hashing (admin API, Wave 2).

- **Constant-time comparison**: using a hash-keyed map lookup avoids substring
  comparison timing side-channels that could leak token prefix length.

- **Per-token rate limit** (not per-IP): player deployments use CDNs and NAT;
  per-IP limits would penalize legitimate players behind shared IPs. Per-token
  limits accurately target abusing integrations.

- **64 KB body cap**: the `BeaconBatch` schema allows up to 25 events per batch.
  Typical batches are < 2 KB. 64 KB is a generous cap that rejects only clearly
  malicious payloads while allowing large error payloads from high-event-rate
  players.

- **Dedicated listen address**: separating the ingest listener (`PULSE_INGEST_LISTEN_ADDR`)
  from the admin API (`PULSE_LISTEN_ADDR`) allows Kubernetes / firewall operators to
  expose only `:8091` to the internet while keeping `:8090` private. In the Helm chart,
  the ingest Service is annotated `pulse.io/internet-facing: "true"`.

## Consequences

- Ingest token management is separate from admin API token management. Tokens of
  `kind: "ingest"` are issued via `POST /api/v1/admin/tokens` with `kind: "ingest"`.
- The per-token rate limit default (100 req/s burst 200) is configurable at compile
  time but not via env var in Wave 2. Adjust in `beacon.go` for very high-volume
  deployments (Wave 3: expose as `PULSE_INGEST_RATE_LIMIT`).
- Free-tier beacon write gating is not yet enforced per-token (GAP-2-004). Any
  valid ingest token currently works regardless of tier. Tier check planned Wave 3.
