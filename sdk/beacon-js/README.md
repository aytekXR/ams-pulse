# Pulse Beacon (JS)

Player-side QoE telemetry for [Pulse](../../README.md). Reports startup time,
rebuffer count/duration, playback errors, bitrate/resolution switches and watch
time to your own Pulse collector. **MIT-licensed and open-sourced deliberately**
(PRD §7.12): it seeds adoption in the AMS developer community and is the
viewer-side moat no server-side DIY stack can replicate.

## Hard requirements (PRD F3 acceptance criteria — enforced in CI)

- **< 15 KB gzipped** (`npm run size` gate)
- One-line init with stream + customer metadata
- Events batched, sent at most every 10 s (`sendBeacon` + retry queue)
- < 1% player CPU overhead
- **Graceful no-op** if the collector is unreachable — a beacon must never break playback
- CMCD-aligned field naming
- Configurable sampling rate for very large audiences

## Target integration (Phase 2)

```js
import { PulseBeacon } from "@pulse/beacon";

const beacon = PulseBeacon.init({
  collector: "https://pulse.example.com",
  token: "ingest-token",
  streamId: "auction-main",
  meta: { tenant: "client-42" },
});

beacon.attachWebRTC(webRTCAdaptor);  // AMS WebRTC adapter (getStats-based)
beacon.attachMedia(videoElement);    // hls.js / video.js / native HLS
```

Payload contract: [`contracts/events/beacon-event.schema.json`](../../contracts/events/beacon-event.schema.json).
Mobile SDKs (Android/iOS/Flutter) are Phase 3, as sibling directories under `sdk/`.
