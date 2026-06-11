/**
 * Typed API client for the Pulse REST API and live WebSocket.
 *
 * Contract: contracts/openapi/pulse-api.yaml — request/response types are
 * GENERATED from the spec (openapi-typescript) into ./generated.ts; this file
 * adds only fetch plumbing (auth header, error normalization, WS reconnect).
 * Hand-written shapes that drift from the spec are a contract violation.
 */

// TODO(FE-01): codegen wiring + ApiClient with token injection.
// TODO(FE-01): LiveSocket — auto-reconnecting WebSocket for /live/ws (F1).

export {};
