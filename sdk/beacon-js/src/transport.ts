/**
 * Event transport: batches events (flush at most every 10 s or on visibility
 * change / pagehide), prefers navigator.sendBeacon with fetch keepalive
 * fallback, keeps a bounded in-memory retry queue, and silently drops on
 * persistent failure. Payload shape: contracts/events/beacon-event.schema.json.
 */

// TODO(SDK-01, Phase 2)
export {};
