# Pulse Beacon SDK Integration Guide

**PRD ref:** F3 (QoE beacon SDK) · **Status: Shipped (Wave 2)**  
**Budget:** SDK < 15 KB gzip (measured: **3.44 KB**) · < 1% player CPU · flush ≤ 10 s

---

## Overview

`@pulse/beacon` is a zero-dependency TypeScript SDK that instruments your players
and sends QoE telemetry (startup time, rebuffering, errors, bitrate/resolution
changes, watch time) to your own Pulse collector. **Viewer data never leaves your
infrastructure.**

The SDK is MIT-licensed and published from `sdk/beacon-js/`. It has been measured
at 3.44 KB gzipped with 56 tests green.

---

## Step 1 — Create an ingest token

Before integrating the SDK you need a Pulse ingest token.

**Via the UI:**  
Settings → Ingest Tokens → Create token → copy the displayed value.

The raw token is shown once. Pulse stores only its SHA-256 hash.

**Via the API:**
```sh
curl -X POST http://localhost:8090/api/v1/admin/tokens \
  -H "Authorization: Bearer plt_<admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"my-player","kind":"ingest"}'
```

Ingest tokens begin with `plt_`. They authenticate only the beacon `POST /ingest/beacon`
endpoint; they cannot be used for the admin API.

---

## Step 2 — Install the SDK

```bash
npm install @pulse/beacon
# or
yarn add @pulse/beacon
```

The package is marked `"sideEffects": false` — bundlers tree-shake unused exports.

### CDN / IIFE (no bundler)

```html
<script src="https://cdn.example.com/pulse-beacon.min.js"></script>
<script>
  const session = PulseBeacon.Pulse.init(config);
</script>
```

---

## Step 3 — Initialize the beacon

```ts
import { Pulse } from '@pulse/beacon';

const session = Pulse.init({
  ingestUrl: 'https://pulse.example.com', // Pulse collector base URL
  token: 'plt_xxxxxxxxxxxxxxxx',           // ingest token from step 1
  streamId: 'live/my-stream',             // AMS stream name
  app: 'live',                            // optional: AMS application name
  metadata: { tenant: 'client-42' },      // optional: billing/routing tags (F6)
  sampleRate: 0.1,                        // optional: 0–1, default 1 (all sessions)
});
```

`Pulse.init()` returns a `SessionHandle`. The handle is **always non-null** — if the
config is invalid or the collector is unreachable, it silently becomes a no-op.
**Telemetry never breaks playback.**

---

## Player integrations

### AMS WebRTC (AMS JS SDK — recommended)

```ts
// After the webRTCAdaptor object is created by the AMS JS SDK:
session.attachWebRTC(webRTCAdaptor);
```

The adapter polls `RTCPeerConnection.getStats()` every 5 seconds and emits:

| Trigger | Beacon event | Key data |
|---------|-------------|----------|
| First decoded frames (getStats) | `startup_complete` | `startup_ms`, `bitrate_kbps` |
| 5 s getStats poll | `heartbeat` | `watch_ms`, `bitrate_kbps`, `dropped_frames` |
| Frame-counter delta = 0 | `rebuffer_start` | `buffer_ms: 0` |
| Frame-counter resumes | `rebuffer_end` | `duration_ms` |
| >10% bandwidth change | `bitrate_change` | `from_kbps`, `to_kbps` |
| frameWidth/Height change | `resolution_change` | `from`, `to` |
| `session.dispose()` | `session_end` | `watch_ms`, `reason: 'user_exit'` |

### hls.js

Layer 1: hls.js events (precise startup and level switching):
```ts
import Hls from 'hls.js';

const hls = new Hls();
hls.loadSource('https://your-cdn.com/live/my-stream/index.m3u8');
hls.attachMedia(videoElement);

session.attachHls(hls);
```

Layer 2: HTML video element (universal stall and error coverage):
```ts
session.attachVideoElement(videoElement);
```

Both layers can be attached simultaneously. The hls.js adapter fires on
`MANIFEST_LOADED`, `FRAG_BUFFERED`, `BUFFER_STALLED`, `ERROR`, and
`LEVEL_SWITCHED`. The media element adapter fires on `waiting`, `stalled`,
`playing`, and `error`.

**hls.js `bitrate_change` note:** the `LEVEL_SWITCHED` event populates `hls_level`
in the data; `from_kbps`/`to_kbps` are set to 0 because the HlsLike interface
does not import hls.js types (zero production deps). If you need precise kbps
values, emit a custom event:
```ts
hls.on(Hls.Events.LEVEL_SWITCHED, (_, data) => {
  const levels = hls.levels;
  session.event('bitrate_change', {
    from_kbps: levels[data.level - 1]?.bitrate / 1000 ?? 0,
    to_kbps: levels[data.level]?.bitrate / 1000 ?? 0,
  });
});
```

### video.js

video.js wraps an `HTMLVideoElement`. Extract it and call `attachVideoElement`:

```ts
import videojs from 'video.js';

const player = videojs('my-video-element-id');

// Wait for the player to be ready before attaching:
player.ready(() => {
  const videoEl = player.el().querySelector('video');
  if (videoEl instanceof HTMLVideoElement) {
    session.attachVideoElement(videoEl);
  }
});
```

### Plain `<video>` element (Safari HLS, DASH, native)

```ts
const videoEl = document.getElementById('my-player') as HTMLVideoElement;
// Set src before attaching (or attach before src; both work):
videoEl.src = 'https://your-cdn.com/live/my-stream/index.m3u8';

session.attachVideoElement(videoEl);
```

The media element adapter fires on:

| Trigger | Beacon event | Key data |
|---------|-------------|----------|
| `loadstart` → first `playing` | `startup_complete` | `startup_ms` |
| Every 30 s (`timeupdate`) | `heartbeat` | `watch_ms` |
| `waiting` / `stalled` | `rebuffer_start` | `buffer_ms` (buffered ranges) |
| `playing` after stall | `rebuffer_end` | `duration_ms` |
| `error` | `error` | `code` (MEDIA_ERR_*), `message`, `fatal: true` |

---

## Full event mapping table

| Player trigger | Beacon type | Key data fields |
|---|---|---|
| `Pulse.init()` | `session_start` | `page_url`, `autoplay` |
| First frames (WebRTC getStats) | `startup_complete` | `startup_ms`, `bitrate_kbps` |
| `loadstart` → first `playing` (media element) | `startup_complete` | `startup_ms` |
| `MANIFEST_LOADED` → first `FRAG_BUFFERED` (hls.js) | `startup_complete` | `startup_ms` |
| 5 s WebRTC poll | `heartbeat` | `watch_ms`, `bitrate_kbps`, `buffer_ms`, `dropped_frames` |
| 30 s media element `timeupdate` | `heartbeat` | `watch_ms` |
| Frame-counter delta = 0 (WebRTC) | `rebuffer_start` | `buffer_ms: 0` |
| `waiting` / `stalled` (media element) | `rebuffer_start` | `buffer_ms` |
| `BUFFER_STALLED` (hls.js) | `rebuffer_start` | `buffer_ms: 0` |
| Frame-counter resumes (WebRTC) | `rebuffer_end` | `duration_ms` |
| `playing` after stall (media element) | `rebuffer_end` | `duration_ms` |
| `error` (media element) | `error` | `code` (MEDIA_ERR_*), `message`, `fatal: true` |
| `hlsError` (hls.js) | `error` | `code` (hls.js details), `fatal` |
| >10% bandwidth change (WebRTC) | `bitrate_change` | `from_kbps`, `to_kbps` |
| `LEVEL_SWITCHED` (hls.js) | `bitrate_change` | `hls_level` (from_kbps/to_kbps = 0) |
| frameWidth/Height change (WebRTC) | `resolution_change` | `from`, `to` |
| `session.dispose()` | `session_end` | `watch_ms`, `reason: 'user_exit'` |

---

## Custom events

```ts
// Custom event with optional data
session.event('error', { code: 'DRM_FAIL', message: 'License server unreachable', fatal: true });
session.event('heartbeat', { watch_ms: 30000 });
```

Allowed types: `session_start`, `startup_complete`, `heartbeat`, `rebuffer_start`,
`rebuffer_end`, `error`, `bitrate_change`, `resolution_change`, `session_end`.
See `contracts/events/beacon-event.schema.json` for the full data shape per type.

---

## Cleanup

```ts
// On player destroy or page navigation:
session.dispose();
```

`dispose()` flushes the pending event queue via `navigator.sendBeacon` (reliable
delivery even on page unload) and removes all event listeners. Safe to call multiple
times.

---

## Sampling

```ts
const session = Pulse.init({
  // ... required fields ...
  sampleRate: 0.05, // instrument 5% of sessions
});
```

The sampling decision is made **once per session** at `Pulse.init()` time.
Sampled-out sessions return a no-op `SessionHandle` with a valid `sessionId`
but make zero network calls. This is useful to reduce ingest volume on
high-concurrency deployments while preserving a representative sample for QoE analysis.

---

## Privacy / IP anonymization

IP addresses are resolved on the **server side** and are never included in the
beacon payload. To zero the last octet of IPv4 addresses (and the last 80 bits
of IPv6) before geo lookup and ClickHouse storage, set:

```sh
PULSE_ANONYMIZE_IP=true
```

With this flag, geo enrichment degrades gracefully to country-level only
(city and region fields are zeroed). This setting satisfies GDPR and KVKK
"data minimization" postures for viewer IP handling.

The beacon payload itself contains only:
- `session_id` — UUID v4 generated client-side (no PII)
- `stream_id`, `app`, `meta` — as you pass them (you control these)
- Timing, bitrate, resolution, error codes — structured QoE data

**Not collected:** raw IP (handled server-side), user IDs, cookies, page content,
form data, browser fingerprints.

---

## Transport behavior

| Behavior | Value |
|---|---|
| Batch flush cadence | ≤ 10 s OR 25 events (whichever first) |
| Page unload flush | `visibilitychange hidden` + `pagehide` via `navigator.sendBeacon` |
| Retry backoff | 1 s → exponential → 60 s cap |
| Retry queue cap | 100 batches; drop-oldest on overflow |
| localStorage spill | Failed batches written to `pulse_beacon_q` for cross-page recovery |
| Console noise on failure | None (single `console.debug` on bad config only) |
| On unreachable collector | Silent no-op; playback unaffected |

---

## Configuration reference

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `ingestUrl` | string | Yes | — | Pulse collector base URL (HTTPS recommended) |
| `token` | string | Yes | — | Ingest token from Settings → Ingest Tokens |
| `streamId` | string | Yes | — | AMS stream name (matches `beacon_events.stream_id`) |
| `app` | string | No | `""` | AMS application name |
| `metadata` | `Record<string,string>` | No | `{}` | Billing/routing tags (surfaced in F6 usage statements) |
| `sampleRate` | number 0–1 | No | `1` | Fraction of sessions to instrument |

---

## TypeScript types

```ts
import type { PulseConfig, SessionHandle, BeaconEventType, PlayerKind } from '@pulse/beacon';

const cfg: PulseConfig = {
  ingestUrl: 'https://pulse.example.com',
  token: 'plt_...',
  streamId: 'live/my-stream',
};

const session: SessionHandle = Pulse.init(cfg, 'hls.js');
```

Full declarations in `sdk/beacon-js/dist/index.d.ts`.

---

## Verify events are arriving

```sh
# Tail the pulse log (structured JSON lines):
journalctl -u pulse -f | grep beacon_events

# Or check the QoE dashboard:
# http://localhost:8090/qoe → Viewer QoE
```

The QoE dashboard shows startup p50/p95 and rebuffer ratio once events
have been collected and rolled up (rollup runs every hour via ClickHouse
materialized views).

---

## Known limitations

- **hls.js `from_kbps`/`to_kbps`:** Both fields are 0 on `LEVEL_SWITCHED` events
  because the HlsLike interface avoids importing hls.js types (zero production deps).
  Use a custom `session.event()` call if you need precise kbps values — see the
  hls.js section above.
- **Edge discovery dedup (F7):** In Wave 2, the origin viewer count is not deduplicated
  from edge viewer counts in multi-node deployments. Viewer counts may be slightly
  overstated in edge-heavy topologies. Fix planned for Wave 3.
