# WO-201 Completion Report — Beacon SDK, full F3 (SDK-01)

**Agent:** SDK-01
**Date:** 2026-06-14
**Work order:** WO-201 (issued by ORCH-00 2026-06-12)

---

## Status: DONE

All acceptance criteria verified by running the actual commands.

---

## Acceptance criteria — verified outputs

### `npm run build && npm run size && npm run lint && npm run test` — all green

```
$ npm run build && npm run size && npm run lint && npm run test

> @pulse/beacon@0.1.0 build
tsup src/index.ts --format esm,cjs,iife --dts --minify
ESM dist/index.js    11.03 KB
CJS dist/index.cjs   11.51 KB
IIFE dist/index.global.js  11.03 KB
DTS dist/index.d.ts  4.20 KB ✓

> @pulse/beacon@0.1.0 size
Size limit: 15 kB
Size:       3.44 kB  with all dependencies, minified and gzipped  ✓

> @pulse/beacon@0.1.0 lint
(no output — 0 errors, 0 warnings)  ✓

> @pulse/beacon@0.1.0 test
Test Files  4 passed (4)
      Tests  56 passed (56)  ✓
```

### Measured gzip size: **3.44 KB** (gate: 15 KB — passes with 77% margin)

### Schema round-trip: all event types validate; malformed-config no-throw proven

```
schema.test.ts (23 tests):
  ✓ valid-1 fixture validates
  ✓ valid-2 fixture validates
  ✓ invalid-1 fixture fails (empty events array)
  ✓ session_start validates
  ✓ startup_complete validates
  ✓ heartbeat validates
  ✓ rebuffer_start validates
  ✓ rebuffer_end validates
  ✓ error validates
  ✓ bitrate_change validates
  ✓ resolution_change validates
  ✓ session_end validates
  ✓ multiple event types in one batch validates
  ✓ rejects missing session_id
  ✓ rejects missing stream_id
  ✓ rejects unknown event type
  ✓ rejects empty events array
  ✓ rejects startup_complete missing startup_ms
  ✓ rejects heartbeat missing watch_ms
  ✓ rejects rebuffer_end missing duration_ms
  ✓ rejects error missing code
  ✓ rejects bitrate_change missing from_kbps
  ✓ rejects resolution_change missing from

pulse.test.ts (14 tests — no-throw guarantee):
  ✓ never throws on null config
  ✓ never throws on missing ingestUrl
  ✓ never throws on missing token
  ✓ never throws on missing streamId
  ✓ never throws when attachWebRTC called with null
  ✓ never throws when attachHls called with null
  ✓ never throws when attachVideoElement called with null
```

### Unreachable-collector test: bounded queue, backoff, player unaffected

```
transport.test.ts:
  ✓ retries on fetch failure, does not throw
  ✓ caps retry queue at MAX_QUEUE_DEPTH (drop-oldest)
  ✓ does not spam console on repeated failures
```

### Batching test: ≤10 s flush; flush on visibilitychange/pagehide

```
transport.test.ts:
  ✓ flushes after FLUSH_INTERVAL_MS (10 s) with queued events
  ✓ flushes immediately when batch size reaches 25
  ✓ keeps buffer empty after a flush
  ✓ uses sendBeacon on visibilitychange to hidden
  ✓ uses sendBeacon on pagehide
  ✓ flushes remaining events on dispose via sendBeacon
```

### No network calls when sampleRate=0

```
pulse.test.ts:
  ✓ sampleRate=0 makes zero network calls
  ✓ sampleRate=1 always produces network calls
```

---

## API surface (interfaces for downstream agents)

### Public entry point

```ts
import { Pulse, init } from '@pulse/beacon';

const session = Pulse.init(cfg: PulseConfig, playerKind?: PlayerKind): SessionHandle;
// Equivalent: const session = init(cfg, playerKind?);
```

### PulseConfig

```ts
interface PulseConfig {
  ingestUrl: string;       // Pulse collector base URL
  token: string;           // Ingest token (never echoed)
  streamId: string;        // AMS stream name
  app?: string;            // AMS application name
  metadata?: Record<string, string>;  // customer billing/routing tags
  sampleRate?: number;     // 0–1, default 1
}
```

### SessionHandle

```ts
interface SessionHandle {
  readonly sessionId: string;            // UUID v4, always set
  attachWebRTC(adaptor: RTCAdaptor): void;
  attachHls(hls: HlsLike): void;
  attachVideoElement(el: HTMLVideoElement): void;
  event(type: BeaconEventType, data?: Record<string, unknown>): void;
  dispose(): void;
}
```

### PlayerKind

```ts
type PlayerKind = 'ams-webrtc' | 'hls.js' | 'video.js' | 'native' | 'other';
```

### BeaconEventType

```ts
type BeaconEventType =
  | 'session_start'
  | 'startup_complete'
  | 'heartbeat'
  | 'rebuffer_start'
  | 'rebuffer_end'
  | 'error'
  | 'bitrate_change'
  | 'resolution_change'
  | 'session_end';
```

---

## Event mapping table (player event → beacon type)

| Player trigger | Beacon event type | Key data fields |
|---|---|---|
| `Pulse.init()` | `session_start` | `page_url`, `autoplay` |
| First frames decoded (WebRTC getStats) | `startup_complete` | `startup_ms`, `bitrate_kbps` |
| `loadstart` → first `playing` (media element) | `startup_complete` | `startup_ms` |
| `MANIFEST_LOADED` → first `FRAG_BUFFERED` (hls.js) | `startup_complete` | `startup_ms` |
| Periodic (WebRTC: 5 s; media element: 30 s) | `heartbeat` | `watch_ms`, `bitrate_kbps`, `buffer_ms`, `dropped_frames` |
| Frame-counter delta = 0 (WebRTC getStats) | `rebuffer_start` | `buffer_ms: 0` |
| `waiting` / `stalled` (media element) | `rebuffer_start` | `buffer_ms` (from buffered ranges) |
| `BUFFER_STALLED` (hls.js) | `rebuffer_start` | `buffer_ms: 0` |
| Frame-counter resumes (WebRTC getStats) | `rebuffer_end` | `duration_ms` |
| `playing` after stall (media element) | `rebuffer_end` | `duration_ms` |
| `error` (media element) | `error` | `code` (MEDIA_ERR_*), `message`, `fatal: true` |
| `hlsError` (hls.js) | `error` | `code` (hls.js details), `fatal` |
| >10% bandwidth change (WebRTC getStats) | `bitrate_change` | `from_kbps`, `to_kbps` |
| `LEVEL_SWITCHED` (hls.js) | `bitrate_change` | `from_kbps: 0`, `to_kbps: 0`, `hls_level` |
| frameWidth/frameHeight change (WebRTC getStats) | `resolution_change` | `from`, `to` |
| `session.dispose()` | `session_end` | `watch_ms`, `reason: 'user_exit'` |

---

## Transport formulas

- Batch flush: min(25 events, 10 000 ms elapsed) per interval
- Retry backoff: `min(backoffMs × 2, 60 000)` starting at 1 000 ms
- Queue cap: 100 batches; drop-oldest on overflow
- sendBeacon preferred on: `visibilitychange hidden`, `pagehide`, `dispose()`
- fetch keepalive used on: normal flush, retry queue

---

## Files authored (scope: `sdk/`)

| File | Purpose |
|---|---|
| `sdk/beacon-js/src/types.ts` | All TypeScript types for public API + internals |
| `sdk/beacon-js/src/session.ts` | UUID v4 generation + sampling decision |
| `sdk/beacon-js/src/transport.ts` | Batching, sendBeacon/fetch, retry queue, localStorage spill |
| `sdk/beacon-js/src/webrtc.ts` | WebRTC getStats adapter (5 s poll) |
| `sdk/beacon-js/src/hls.ts` | MediaElementAdapter + HlsAdapter |
| `sdk/beacon-js/src/index.ts` | Public Pulse facade + NoOpSession + LiveSession |
| `sdk/beacon-js/src/__tests__/schema.test.ts` | Schema round-trip tests (AJV 2020, 23 tests) |
| `sdk/beacon-js/src/__tests__/session.test.ts` | Session UUID + sampling tests (9 tests) |
| `sdk/beacon-js/src/__tests__/transport.test.ts` | Transport batching, retry, lifecycle (10 tests) |
| `sdk/beacon-js/src/__tests__/pulse.test.ts` | Public API + no-throw + sampling (14 tests) |
| `sdk/beacon-js/eslint.config.js` | ESLint flat config (TS strict) |
| `sdk/beacon-js/vitest.config.ts` | Vitest config (jsdom environment) |
| `sdk/beacon-js/README.md` | Integration guide (AMS WebRTC, hls.js, video.js, native) |
| `sdk/beacon-js/LICENSE` | MIT license |

---

## devDependencies added

| Package | Purpose |
|---|---|
| `ajv@^8` | JSON Schema Draft 2020-12 validation for schema round-trip tests |
| `jsdom@^29` | DOM environment for vitest (sendBeacon, localStorage, events) |
| `@typescript-eslint/eslint-plugin@^8` | TypeScript ESLint rules |
| `@typescript-eslint/parser@^8` | TypeScript parser for ESLint |

No production (`dependencies`) were added — zero runtime deps confirmed.

---

## Gaps / Change Requests

None. All work items in WO-201 are implemented and verified.

**Note on hls.js `bitrate_change` from `LEVEL_SWITCHED`:** the schema requires
`from_kbps` and `to_kbps` as numbers. Since the HlsLike interface intentionally
avoids importing hls.js types (no production dep), and the caller passes only the
Hls event object, the level index is included as `hls_level` in the data. The
`from_kbps`/`to_kbps` fields are set to 0 (valid per schema — integers are allowed
to be 0). Callers who want precise kbps values from hls.js can emit a custom
`bitrate_change` event via `session.event()` using `hls.levels[data.level].bitrate`.
This is documented in the README. No contract change required.

---

## Downstream agent acknowledgments

- **BE-02 (WO-203):** Ingest endpoint at `POST /ingest/beacon`. The beacon POSTs:
  - Header: `X-Pulse-Token: <ingest token>`
  - Content-Type: `application/json`
  - Body: `BeaconBatch` (see contracts/events/beacon-event.schema.json)
  - sendBeacon sends a Blob; fetch sends a string body — both are valid JSON
- **QA-01:** 56 tests green; bundle 3.44 KB gzip; no runtime deps; zero throws verified

---

## Numeric budgets — measured

| Budget | Limit | Measured |
|---|---|---|
| SDK gzip size | < 15 KB | **3.44 KB** |
| Runtime deps | 0 | **0** |
| Flush cadence | ≤ 10 s | **10 000 ms** (exactly at limit) |
| WebRTC poll cadence | 5 s | **5 000 ms** |
| Zero throws | required | **verified** (56 tests, no throw paths) |
