# Pulse — Beacon SDK Integration Guide

> **Audience:** operators and front-end developers embedding player-side QoE
> telemetry in AMS-served player pages.
>
> **Accuracy note:** every file reference, endpoint path, API name, and code
> fact below was read directly from the source files cited. Nothing is inferred
> from documentation or memory.

---

## 1. Introduction

The `@pulse/beacon` SDK is a client-side JavaScript library that collects
viewer-perceived quality metrics — startup time, rebuffering, errors,
bitrate/resolution switches, and cumulative watch time — from browser player
pages and reports them to your Pulse collector.

Server-side REST polling (`restpoller`) can measure stream health from AMS's
perspective (encoder bitrate, packet-loss ratio, watcher counts). It cannot
measure what the viewer actually experienced: whether playback started in 200 ms
or 8 s, whether the player stalled twice during the session, or which error code
the browser surfaced. The beacon SDK fills that gap.

Beacon data feeds the `viewer_sessions` and `rollup_qoe_1h` tables and surfaces
in `GET /api/v1/qoe/summary`. It is the only source of viewer-perceived QoE in
Pulse; the REST polling path does not contribute to that endpoint.

The SDK is **MIT-licensed** (`sdk/beacon-js/LICENSE`), has **zero runtime
dependencies**, and is marked `"sideEffects": false` for full tree-shaking.
Bundle size is **3.52 KB gzipped** — well inside the < 15 KB hard gate enforced
in CI by `npm run size` (uses `@size-limit/preset-small-lib`).

---

## 2. Prerequisites

### 2.1 License tier

> **⚠️ Pro+ license required (F3 gate):**
> The beacon ingest endpoint (`POST /ingest/beacon`) requires a Pro or higher
> Pulse license. `CheckBeaconIngest()` in `server/internal/license/license.go`
> (lines 405–413) rejects requests from Free-tier deployments with HTTP 403
> and body `{"code":"LICENSE_REQUIRED","message":"..."}`. Configure
> `PULSE_LICENSE_KEY` or `PULSE_LICENSE_FILE` before testing beacon delivery.
> A deployment with neither env var set runs in Free tier.

### 2.2 Provision an ingest token

Beacon batches authenticate with an `X-Pulse-Ingest-Token` header — an API key
distinct from the admin Bearer JWTs used for the `/api/v1/*` management surface.
Mint a token with the admin API:

```bash
curl -s -X POST https://pulse.example.com/api/v1/admin/tokens \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"kind":"ingest","name":"player-page-prod"}' | jq .token
```

The raw token value (`TokenCreated.token`) is returned **only on creation** — store
it immediately. It has no expiry unless you delete it via the admin API. Tokens are
identifiable by their `plt_` prefix. Pass it as the `token` field in `PulseConfig`.

Alternatively, create and copy an ingest token from **Settings › Ingest Tokens** in the
Pulse web UI — the tab provides a one-click copy of a pre-filled SDK snippet.

### 2.3 Ingest URL

Set `ingestUrl` in `PulseConfig` to your Pulse collector base URL (scheme +
host, no path):

| Deployment | `ingestUrl` value |
|---|---|
| Default stack | `http://localhost:8090` |
| Production (TLS, default port) | `https://pulse.example.com` |
| Dedicated ingest listener (`PULSE_INGEST_LISTEN_ADDR`) | `https://pulse.example.com:<port>` |

The SDK appends `/ingest/beacon` to this base URL automatically. Do not include
the path in `ingestUrl`.

The `/ingest/beacon` route is always permissive for CORS — cross-origin browser
requests from any player page origin are accepted without allowlisting (see
`AMS-INTEGRATION.md` §5.4).

---

## 3. Installation

### 3.1 npm / bundler

```bash
npm install @pulse/beacon
# or
yarn add @pulse/beacon
```

The package ships ESM (`dist/index.js`), CJS (`dist/index.cjs`), and full `.d.ts`
declarations. Import whichever your bundler selects automatically. For
tree-shaking-friendly imports, `init` is also exported as a named re-export of
`Pulse.init`:

```ts
import { init } from '@pulse/beacon';
// equivalent to Pulse.init(...)

import type { PulseConfig, SessionHandle, BeaconEventType, PlayerKind } from '@pulse/beacon';
```

### 3.2 No-bundler IIFE build

For pages that load scripts without a bundler, use the pre-built IIFE at
`sdk/beacon-js/dist/index.global.js`. Self-host this file — **do not load it from
a CDN**. The Pulse project policy requires all scripts and fonts to be
self-hosted; no CDN dependencies are permitted.

```html
<script src="/static/pulse-beacon.global.js"></script>
<script>
  const session = Pulse.init({ ingestUrl: '...', token: '...', streamId: '...' });
</script>
```

---

## 4. Player adapter selection

Choose the adapter(s) that match your player stack:

| Player | `playerKind` | Required attach calls |
|---|---|---|
| AMS WebRTC (JS SDK `webRTCAdaptor`) | `'ams-webrtc'` | `session.attachWebRTC(webRTCAdaptor)` |
| hls.js | `'hls.js'` | `session.attachHls(hls)` **and** `session.attachVideoElement(videoEl)` |
| video.js | `'video.js'` | `session.attachVideoElement(player.el().querySelector('video'))` |
| Safari native HLS / plain `<video>` | `'native'` | `session.attachVideoElement(videoEl)` |
| Other / unknown | `'other'` | `session.attachVideoElement(videoEl)` (best effort) |

For hls.js, attach both layers: `attachHls` gives precise startup timing from
hls.js lifecycle events (`MANIFEST_LOADED` → `FRAG_BUFFERED`); `attachVideoElement`
provides universal rebuffer and error coverage via `waiting`/`stalled`/`error`
on the `HTMLVideoElement`.

---

## 5. Quick-start snippets

### 5.1 AMS WebRTC

```ts
import { Pulse } from '@pulse/beacon';

// Call after webRTCAdaptor is created (AMS JS SDK).
const session = Pulse.init(
  {
    ingestUrl: 'https://pulse.example.com',
    token: 'plt_xxxxxxxxxxxxxxxx',
    streamId: 'auction-main',   // AMS stream name — must be explicit, not derived from URL
    app: 'live',                // optional: AMS application name
    metadata: { tenant: 'client-42' }, // optional: billing/routing tags
  },
  'ams-webrtc',
);

session.attachWebRTC(webRTCAdaptor);

// On player teardown:
session.dispose();
```

`attachWebRTC` polls `RTCPeerConnection.getStats()` every 5 s and fires
`startup_complete` when the first frames are decoded.

### 5.2 hls.js

```ts
import Hls from 'hls.js';
import { Pulse } from '@pulse/beacon';

const hls = new Hls();
hls.loadSource('https://stream.example.com/live/auction-main/index.m3u8');
hls.attachMedia(videoElement);

const session = Pulse.init(
  {
    ingestUrl: 'https://pulse.example.com',
    token: 'plt_xxxxxxxxxxxxxxxx',
    streamId: 'auction-main',
    app: 'live',
  },
  'hls.js',
);

// Attach both layers for full coverage.
session.attachHls(hls);
session.attachVideoElement(videoElement);

// On player destroy:
session.dispose();
```

### 5.3 video.js

```ts
import videojs from 'video.js';
import { Pulse } from '@pulse/beacon';

const player = videojs('my-video');

const session = Pulse.init(
  {
    ingestUrl: 'https://pulse.example.com',
    token: 'plt_xxxxxxxxxxxxxxxx',
    streamId: 'auction-main',
    app: 'live',
  },
  'video.js',
);

// Wait for the player to be ready before attaching.
player.ready(() => {
  // video.js wraps a native <video> element — pass it directly.
  const videoEl = player.el().querySelector('video');
  if (videoEl instanceof HTMLVideoElement) {
    session.attachVideoElement(videoEl);
  }
});

player.on('dispose', () => session.dispose());
```

### 5.4 Safari native HLS / plain `<video>`

```ts
import { Pulse } from '@pulse/beacon';

const videoEl = document.getElementById('player') as HTMLVideoElement;
videoEl.src = 'https://stream.example.com/live/auction-main/index.m3u8';

const session = Pulse.init(
  {
    ingestUrl: 'https://pulse.example.com',
    token: 'plt_xxxxxxxxxxxxxxxx',
    streamId: 'auction-main',
    app: 'live',
  },
  'native',
);

session.attachVideoElement(videoEl);

window.addEventListener('pagehide', () => session.dispose());
```

---

## 6. Configuration reference

`Pulse.init(cfg: PulseConfig, playerKind: PlayerKind = 'other'): SessionHandle`

`Pulse.init` never throws. If `cfg` is invalid or the collector is unreachable,
it returns a no-op `SessionHandle` that silently discards all calls — telemetry
will never break playback.

### PulseConfig

| Field | Type | Required | Description |
|---|---|---|---|
| `ingestUrl` | `string` | Yes | Pulse collector base URL (no trailing path). HTTPS recommended for production. |
| `token` | `string` | Yes | Ingest token (`kind=ingest`) minted via `POST /api/v1/admin/tokens`. Sent as `X-Pulse-Ingest-Token`. |
| `streamId` | `string` | Yes | AMS stream name. Must be provided explicitly — the SDK does not derive it from the player URL. |
| `app` | `string` | No | AMS application name (e.g. `live`, `LiveApp`). Stored in `beacon_events.app`. |
| `metadata` | `Record<string,string>` | No | Arbitrary string key-value pairs for billing or routing (e.g. `{ tenant: 'client-42' }`). Surfaced in F6 statements. |
| `sampleRate` | `number` 0–1 | No | Fraction of sessions to report. Default `1` (all sessions). The decision is made once at `Pulse.init()` time; sampled-out sessions return a no-op handle and make zero network calls. |

### PlayerKind

Allowed values: `'ams-webrtc'`, `'hls.js'`, `'video.js'`, `'native'`, `'other'`.

Passed as the second argument to `Pulse.init()`. Stored in `player.kind` on
every batch. Defaults to `'other'` if omitted.

### SessionHandle

| Method | Description |
|---|---|
| `attachWebRTC(adaptor: RTCAdaptor)` | Instrument an AMS JS SDK `webRTCAdaptor`. Polls `RTCPeerConnection.getStats()` every 5 s. |
| `attachHls(hls: HlsLike)` | Instrument a hls.js `Hls` instance. Listens for `MANIFEST_LOADED`, `FRAG_BUFFERED`, `BUFFER_STALLED`, `ERROR`, `LEVEL_SWITCHED`. |
| `attachVideoElement(el: HTMLVideoElement)` | Instrument a plain `<video>` element. Works for video.js (pass `player.el().querySelector('video')`) and Safari native HLS. |
| `event(type: BeaconEventType, data?)` | Emit a custom event with optional data payload. |
| `dispose()` | Flush the pending event queue via `navigator.sendBeacon`, emit `session_end`, remove all listeners. Safe to call multiple times. |

Custom event examples:

```ts
// DRM failure not surfaced by the player adapter:
session.event('error', { code: 'DRM_FAIL', message: 'License server unreachable', fatal: true });
// Force a heartbeat checkpoint with a known watch duration:
session.event('heartbeat', { watch_ms: 30000 });
```

Allowed `BeaconEventType` values: `session_start`, `startup_complete`, `heartbeat`,
`rebuffer_start`, `rebuffer_end`, `error`, `bitrate_change`, `resolution_change`,
`session_end`. Full data shape per type: `contracts/events/beacon-event.schema.json`.

---

## 7. What is collected

### 7.1 Collected fields

| Field | Description |
|---|---|
| `session_id` | UUID v4 generated client-side per `Pulse.init()` call. One session = one playback attempt. |
| `stream_id` | As provided to `Pulse.init()`. |
| `app` | As provided to `Pulse.init()`. |
| `meta` | `metadata` key-value pairs as provided. |
| `player.kind` | `PlayerKind` value passed to `Pulse.init()`. |
| `player.sdk_version` | SDK version string (`0.1.0`). |

Each `Pulse.init()` call creates a distinct `session_id`, mapping to one playback
attempt. Call `Pulse.init()` again for each new playback attempt — page refresh,
new stream selection, or player re-creation.

### 7.2 Events and metrics

| Event type | When fired | Key data fields |
|---|---|---|
| `session_start` | Immediately on `Pulse.init()` | `page_url` — `autoplay` is schema-defined but not auto-emitted by the SDK |
| `startup_complete` | First frames decoded (WebRTC) or first `playing` after `loadstart` (media element) or first `FRAG_BUFFERED` after `MANIFEST_LOADED` (hls.js) | `startup_ms`, `bitrate_kbps` |
| `heartbeat` | Periodic — WebRTC: every 5 s; media element: every 30 s | `watch_ms`, `bitrate_kbps`, `buffer_ms`, `dropped_frames` |
| `rebuffer_start` | Frame-counter delta = 0 (WebRTC); `waiting`/`stalled` (media element); `BUFFER_STALLED` (hls.js) | `buffer_ms` |
| `rebuffer_end` | Frame-counter resumes (WebRTC); `playing` after stall (media element) | `duration_ms` |
| `error` | `error` event (media element); `hlsError` (hls.js) | `code`, `message`, `fatal` |
| `bitrate_change` | >10% bandwidth change (WebRTC); `LEVEL_SWITCHED` (hls.js) | `from_kbps`, `to_kbps`, `hls_level` |
| `resolution_change` | `frameWidth`/`frameHeight` change (WebRTC) | `from`, `to` |
| `session_end` | `session.dispose()` | `reason` — `watch_ms` is schema-defined but not auto-emitted on `session_end`; cumulative watch time arrives via `heartbeat` events |

> **CMCD field alignment:** Key metric names follow Common Media Client Data (CMCD)
> conventions — `bitrate_kbps` maps to CMCD `br`, `buffer_ms` maps to CMCD `bl`
> (buffer length ahead), and `startup_ms` maps to delivery-latency at first frame.

### 7.3 Not collected

The SDK does **not** collect:

- IP addresses — the collector handles anonymization server-side via
  `PULSE_ANONYMIZE_IP=true`. Set this env var to zero-out the last octet
  (IPv4) or last 80 bits (IPv6) before geo lookup and storage.
- User identifiers, login state, cookies, or any authentication material.
- Page content, DOM snapshots, form data, or any content outside the player
  element instrumented via `attachVideoElement` / `attachWebRTC`.

Geo enrichment (country, city) requires a MaxMind GeoLite2 database at
`PULSE_GEO_MMDB_PATH`. If the file is absent, no geo fields are stored —
`city` and `country_code` remain `null` in `viewer_sessions`. Geo enrichment
was confirmed absent on the production AMS deployment (AV-10 CONFIRMED ABSENT).

**Absent fields are absent, not zero.** A `startup_ms` field that does not
appear in `qoe/summary` means no successful `startup_complete` event was
received for that stream in the query window — not that startup took 0 ms.
Interpret missing aggregate fields as insufficient data, not as a healthy zero.

---

## 8. Verifying events

### 8.1 Check server logs

Start Pulse with `PULSE_LOG_LEVEL=debug`. Successful beacon batches produce an
ingest handler log line at the path `/ingest/beacon`. Look for `202` responses
from the beacon handler in the server output.

### 8.2 Check qoe/summary

After embedding the SDK and playing a stream, query the QoE summary endpoint:

```bash
curl -s "https://pulse.example.com/api/v1/qoe/summary?stream_id=auction-main" \
  -H "Authorization: Bearer <ADMIN_TOKEN>" | jq .
```

Beacon events aggregate through the ClickHouse `rollup_qoe_1h` materialized
view. Allow up to approximately **120 s** after the first beacon batch before
results appear in `qoe/summary` (`capability-map.md` §10, AV-11 evidence).

> **⚠️ Do not use `GET /api/v1/qoe/ingest` to verify beacon data:**
> That endpoint queries server-side RTMP/WebRTC publisher ingest health
> (bitrate, fps, packet-loss from REST polling). It is a separate pipeline
> from the beacon SDK. Beacon data does not appear there. Additionally, the
> declared `from`/`to` query parameters are silently ignored (BUG-004 — the
> handler invokes `IngestTimeseries` with no time window, returning all-time
> era-mixed buckets regardless of the window supplied). Use
> `GET /api/v1/qoe/summary` for viewer-side QoE.

### 8.3 Live session count

`GET /api/v1/live/overview` reports active streams from the REST poller — not
beacon viewer sessions. Beacon sessions feed `viewer_sessions` directly, not the
live aggregator. Use `qoe/summary` to verify beacon data, not `live/overview`.

---

## 9. Ingest endpoint reference

| Property | Value |
|---|---|
| Path | `POST /ingest/beacon` |
| Port | `:8090` (main API, `PULSE_LISTEN_ADDR`) or dedicated `PULSE_INGEST_LISTEN_ADDR` |
| Auth header | `X-Pulse-Ingest-Token: <token>` (API key scheme; NOT a Bearer JWT) |
| Body | `BeaconBatch` JSON (`contracts/events/beacon-event.schema.json`); max 64 KB |
| CORS | Always permissive — no allowlist configuration required |

Response codes:

| Code | Meaning |
|---|---|
| `202 Accepted` | Batch accepted (all or partial events stored) |
| `401 Unauthorized` | Token missing or invalid |
| `403 LICENSE_REQUIRED` | Pulse license tier is Free; Pro+ required |
| `413 Request Entity Too Large` | Batch body exceeds 64 KB |
| `422 Unprocessable Entity` | All events in the batch failed schema validation |
| `429 Too Many Requests` | Rate limit exceeded |

> **Rate limiting:** both ingest paths enforce 100 req/s per token with a burst
> of 200 — the dedicated listener (`serve.go:326`) and the main-port
> `/ingest/beacon` handler (`server.go:2318`, shipped as A2). Use
> `PULSE_INGEST_LISTEN_ADDR` when you want beacon traffic on a separate port
> for DMZ routing, not for rate-limit coverage.

---

## 10. Transport behavior

The SDK manages batching and delivery internally — no application code is
needed beyond `Pulse.init()` and an attach call.

| Behavior | Detail |
|---|---|
| Batch flush | Every ≤ 10 s **or** 25 events, whichever occurs first |
| Unload flush | `visibilitychange hidden` and `pagehide` via `navigator.sendBeacon` |
| Backoff on failure | Exponential: 1 s → 60 s cap; per-transport, not per-event; no retry storms |
| Retry queue | Bounded at 100 batches max; drops oldest when full |
| `localStorage` spill | Failed batches written to `pulse_beacon_q` key for cross-page recovery on the next page load |
| Console output | Zero on network failure; single `console.debug` on bad config only |

`dispose()` flushes the pending queue synchronously via `navigator.sendBeacon`
before removing listeners. It is safe to call multiple times.

---

## 11. Known limitations

### Free tier — 403 LICENSE_REQUIRED

Beacon ingest requires a Pro or higher Pulse license. Free-tier deployments
(no `PULSE_LICENSE_KEY` or `PULSE_LICENSE_FILE`) receive HTTP 403 with body
`{"code":"LICENSE_REQUIRED","message":"..."}` on every beacon POST. The SDK
backs off and continues retrying; no playback impact, but no data is stored.

### Sampled-out sessions are silent by design

When `sampleRate` is set below `1`, a fraction of `Pulse.init()` calls return
a no-op handle. Those sessions make no network calls and produce no server-side
records. This is intentional — the sampled-out share is not an error and is not
logged. If `qoe/summary` shows fewer sessions than expected, verify the
`sampleRate` value in your `PulseConfig`.

---

## 12. Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| HTTP 401 on beacon POST | Token missing, wrong kind, or expired | Mint a new token with `kind=ingest` via `POST /api/v1/admin/tokens`; confirm the `X-Pulse-Ingest-Token` header is set (not `Authorization: Bearer`) |
| HTTP 403 `LICENSE_REQUIRED` | Free tier deployment | Set `PULSE_LICENSE_KEY` or `PULSE_LICENSE_FILE` to a Pro+ license |
| HTTP 429 rate limit | High viewer count exceeding 100 req/s per token (both ports enforce the same limit) | Spread traffic across per-player ingest tokens, or lower the SDK `sampleRate` |
| No data in `qoe/summary` after play | Rollup window not elapsed | Wait 120 s after the first batch; verify `202` responses in server logs |
| CSP blocks beacon POST | Script or fetch blocked by Content-Security-Policy | Self-host `dist/index.global.js` and add the Pulse origin to `connect-src` in your CSP header; do not load the SDK from any CDN |
| `startup_ms` absent from summary | `startup_complete` event never fired | Confirm the correct attach method was called; for WebRTC confirm `webRTCAdaptor` was passed after the adaptor was fully initialised |
| `sampleRate` below 1 — missing sessions | Expected: sampled-out sessions are silent | See §11 Known limitations |
