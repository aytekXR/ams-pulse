# @pulse/beacon

Player-side QoE telemetry for [Pulse](../../README.md) — self-hosted analytics and
alerting for Ant Media Server. Reports startup time, rebuffering, errors, bitrate/
resolution switches, and cumulative watch time to your own Pulse collector. **Your
viewer data never leaves your infrastructure.**

**MIT-licensed open source** (PRD §7.12): seeds adoption in the AMS developer
community; the viewer-side moat no server-side DIY stack can replicate.

## Hard gates (PRD F3 — enforced in CI)

| Constraint | Limit | Measured |
|---|---|---|
| Bundle size (gzip) | < 15 KB | **3.52 KB** |
| Runtime dependencies | 0 | 0 |
| Player CPU overhead | < 1% | < 1% (5 s poll cadence) |
| Flush cadence | ≤ 10 s | 10 s |
| Throws on any failure | Never | Zero-throw guarantee |

## Installation

```bash
npm install @pulse/beacon
# or
yarn add @pulse/beacon
```

## One-line init

```ts
import { Pulse } from '@pulse/beacon';

const session = Pulse.init(
  {
    ingestUrl: 'https://pulse.example.com', // your Pulse collector URL
    token: 'plt_xxxxxxxxxxxxxxxx',           // ingest token from Pulse admin
    streamId: 'auction-main',               // AMS stream name
    app: 'live',                            // optional: AMS application name
    metadata: { tenant: 'client-42' },      // optional: billing/routing tags
    sampleRate: 0.1,                        // optional: 0–1, default 1 (all sessions)
  },
  'ams-webrtc',                             // optional: player kind tag
);
```

The `session` object is always non-null. If the config is invalid or the collector
is unreachable, it silently becomes a no-op — **telemetry will never break playback**.

## Player integrations

### AMS WebRTC (getStats-based)

```ts
import { PulseBeaconConfig } from '@pulse/beacon';

// After webRTCAdaptor is created (AMS JS SDK):
session.attachWebRTC(webRTCAdaptor);

// The beacon polls RTCPeerConnection.getStats() every 5 s and emits:
//   startup_complete  — when first frames are decoded
//   heartbeat         — watch_ms, bitrate_kbps, dropped_frames
//   rebuffer_start/end— stall detected via frame-counter delta
//   bitrate_change    — >10% bandwidth change
//   resolution_change — frameWidth x frameHeight switch
```

### hls.js

```ts
import Hls from 'hls.js';

const hls = new Hls();
hls.loadSource('https://stream.example.com/live/auction-main/index.m3u8');
hls.attachMedia(videoElement);

// Layer 1: hls.js events (MANIFEST_LOADED → FRAG_BUFFERED, BUFFER_STALLED, ERROR, LEVEL_SWITCHED)
session.attachHls(hls);

// Layer 2: HTMLVideoElement events (waiting/playing/stalled/error — always instrumented)
session.attachVideoElement(videoElement);
```

Both layers can be attached simultaneously. hls.js events give more precise startup
timing; video element events provide universal rebuffer and error coverage.

### video.js

video.js exposes the underlying `<video>` element. Pass it directly:

```ts
import videojs from 'video.js';

const player = videojs('my-video');

// Use the underlying HTMLVideoElement — no video.js dependency needed
const videoEl = player.el().querySelector('video');
if (videoEl instanceof HTMLVideoElement) {
  session.attachVideoElement(videoEl);
}
```

### Native HLS (`<video>` + Safari)

```ts
const videoEl = document.getElementById('player') as HTMLVideoElement;
videoEl.src = 'https://stream.example.com/live/auction-main/index.m3u8';

session.attachVideoElement(videoEl);
```

### Custom events

```ts
// Custom event with optional data
session.event('error', { code: 'CUSTOM_ERR', message: 'DRM failure', fatal: true });
session.event('heartbeat', { watch_ms: 30000 });
```

Allowed event types match `beacon-event.schema.json`:
`session_start`, `startup_complete`, `heartbeat`, `rebuffer_start`, `rebuffer_end`,
`error`, `bitrate_change`, `resolution_change`, `session_end`.

### Cleanup

```ts
// On player destroy or page unload
session.dispose();
```

`dispose()` flushes the pending event queue (via `sendBeacon` for reliability) and
removes all event listeners. Safe to call multiple times.

## Configuration reference

| Field | Type | Required | Description |
|---|---|---|---|
| `ingestUrl` | string | Yes | Pulse collector base URL (HTTPS) |
| `token` | string | Yes | Ingest token from Pulse admin → Settings → Tokens |
| `streamId` | string | Yes | AMS stream name (matches `beacon_events.stream_id`) |
| `app` | string | No | AMS application name |
| `metadata` | `Record<string,string>` | No | Billing/routing tags (surfaced in F6 statements) |
| `sampleRate` | number 0–1 | No | Fraction of sessions to report; default 1 |

The sampling decision is made **once per session** at `Pulse.init()` time. Sampled-out
sessions return a no-op handle with a valid `sessionId` but make zero network calls.

## What is collected

| Field | Description |
|---|---|
| `session_id` | UUID v4 generated client-side per playback attempt |
| `stream_id` | As provided to `Pulse.init()` — not derived from the URL |
| `app` | As provided |
| `meta` | As provided |
| `player.kind` | Player kind tag passed to `Pulse.init()` |
| `player.sdk_version` | Beacon SDK version |
| Event data | Timing, bitrate, resolution, error codes — see schema below |

**Not collected:** IP addresses (collector handles anonymization server-side via
`PULSE_ANONYMIZE_IP`), user IDs, cookies, page content, form data.

Schema: [`contracts/events/beacon-event.schema.json`](../../contracts/events/beacon-event.schema.json)

## Event mapping table

| Player trigger | Beacon event type | Key data fields |
|---|---|---|
| `Pulse.init()` | `session_start` | `page_url`, `autoplay` |
| First frames decoded (WebRTC getStats) | `startup_complete` | `startup_ms`, `bitrate_kbps` |
| `loadstart` → first `playing` (media element) | `startup_complete` | `startup_ms` |
| `MANIFEST_LOADED` → first `FRAG_BUFFERED` (hls.js) | `startup_complete` | `startup_ms` |
| Periodic (WebRTC: 5 s; media element: 30 s) | `heartbeat` | `watch_ms`, `bitrate_kbps`, `buffer_ms`, `dropped_frames` |
| Frame-counter delta = 0 (WebRTC) | `rebuffer_start` | `buffer_ms` |
| `waiting`/`stalled` (media element) | `rebuffer_start` | `buffer_ms` |
| `BUFFER_STALLED` (hls.js) | `rebuffer_start` | `buffer_ms` |
| Frame-counter resumes (WebRTC) | `rebuffer_end` | `duration_ms` |
| `playing` after stall (media element) | `rebuffer_end` | `duration_ms` |
| `error` (media element) | `error` | `code` (MEDIA_ERR_*), `message`, `fatal` |
| `hlsError` (hls.js) | `error` | `code` (details), `fatal` |
| >10% bandwidth change (WebRTC) | `bitrate_change` | `from_kbps`, `to_kbps` |
| `LEVEL_SWITCHED` (hls.js) | `bitrate_change` | `hls_level` |
| frameWidth/frameHeight change (WebRTC) | `resolution_change` | `from`, `to` |
| `session.dispose()` | `session_end` | `watch_ms`, `reason` |

## Transport behavior

- Batch flush: every ≤10 s OR 25 events (whichever first)
- Page unload flush: `visibilitychange hidden` and `pagehide` via `navigator.sendBeacon`
- Unreachable collector: exponential backoff (1 s → 60 s cap), bounded retry queue (100 batches max, drop-oldest)
- localStorage spill: failed batches written to `pulse_beacon_q` for cross-page recovery
- No retry storms: backoff is per-transport, not per-event
- Zero console output on network failure (single `console.debug` on bad config only)

## Session stitching

Each `Pulse.init()` call generates a fresh UUID v4 `session_id`. This maps to a single
playback attempt in Pulse analytics. If the viewer refreshes or starts a new stream,
call `Pulse.init()` again.

## CMCD alignment

Field naming is CMCD-aligned where applicable:
- `bitrate_kbps` → CMCD `br`
- `buffer_ms` → CMCD `bl` (buffer length)
- `startup_ms` → delivery-latency at first frame

## Tree-shaking

The package is marked `"sideEffects": false`. Bundlers (webpack, Rollup, esbuild,
Vite) can tree-shake unused exports. `Pulse.init()` and `init` are the only exports
most applications need.

## TypeScript

Full `.d.ts` declarations are included. All public types are exported:

```ts
import type {
  PulseConfig,
  SessionHandle,
  BeaconEventType,
  PlayerKind,
} from '@pulse/beacon';
```

## Changelog

### 0.1.0 (Wave 2 — 2026-06-14)
- Initial production implementation
- WebRTC getStats adapter (5 s poll: startup, heartbeat, stall, bitrate/resolution)
- Media element adapter (waiting/playing/stalled/error/ratechange)
- hls.js adapter (MANIFEST_LOADED, FRAG_BUFFERED, BUFFER_STALLED, ERROR, LEVEL_SWITCHED)
- Batching transport: sendBeacon + fetch keepalive, localStorage spill, exp. backoff
- Sampling (0–1 per-session), session UUID v4 generation
- ESM + CJS + IIFE build, full `.d.ts`
- Schema round-trip tests (AJV 2020), 56 tests green
- Bundle: **3.52 KB** gzipped

## License

MIT — see [LICENSE](LICENSE).
