# ADR 0008: F10 probe protocol coverage — HLS full, others minimal-honest

**Status:** Partially superseded (see Amendments) · **Date:** 2026-06-14 · **Wave:** 3-MVP

## Context

F10 synthetic probes (PRD §7.10) require periodic stream health checks for
HLS, WebRTC, RTMP, and DASH streams. A full native-protocol check for each
would require:
- **WebRTC:** WHIP/WHEP HTTP signaling + STUN reachability.
- **RTMP:** TCP socket + RTMP handshake (librtmp equivalent in pure Go).
- **DASH:** MPEG-DASH manifest parse + segment fetch (similar to HLS).

These are non-trivial to implement correctly without external libraries,
and the Wave 3-MVP time budget is constrained (D-001: minimal-but-working).

## Decision

For Wave 3-MVP:
- **HLS:** Full implementation — GET manifest + first segment fetch;
  measures TTFB, parses `#EXTM3U`, fetches first media URI, computes
  `bitrate_kbps = segment_bytes × 8 / segment_duration_s / 1000`.
- **WebRTC, RTMP, DASH:** Minimal-honest reachability — perform an HTTP
  GET against the URL, record `success=false`, `error_code=not_probed`
  with an `error_msg` that explicitly states the limitation:

  ```
  protocol=webrtc: full probing not yet implemented (Phase 3); HTTP 200 received
  ```

No faked success is ever emitted for these protocols.

## Rationale

- **HLS first.** HLS is the dominant delivery protocol for VOD and live
  streams in the AMS customer base. It is also the easiest to probe via
  standard HTTP: manifests are plain text, segments are HTTP objects.
  Implementing HLS correctly delivers 80% of the value of synthetic probes.
- **Honest minimal coverage beats silent gaps.** The `not_probed` error code
  and `error_msg` tell the operator exactly what is and is not measured. A
  faked `success=true` based on an HTTP 200 from a WebRTC signaling endpoint
  would be misleading: an HTTP 200 says nothing about ICE connectivity or
  codec negotiation.
- **CGO_ENABLED=0 constraint (ARCHITECTURE §3).** The entire server builds
  with CGO disabled. Pure-Go librtmp-equivalent or WebRTC stacks were not
  available as mature pure-Go libraries at time of Wave 3-MVP.

## Consequences

- Operators who monitor WebRTC, RTMP, or DASH streams will see
  `error_code=not_probed` in probe results. The UI and runbook explicitly
  document this scope limitation.
- `success=false` for non-HLS probes is intentional and documented — it is
  not an error to be silenced.
- The `ProbeResult.ErrorCode` field in the OpenAPI contract already includes
  `"not_probed"` as a valid enum value (contracts frozen per D-004), so no
  contract change is needed when Phase-3 implementations land.

## Phase-3 plan

| Protocol | Phase-3 implementation |
|---|---|
| WebRTC | WHIP/WHEP HTTP signaling check + STUN reachability (pure-Go stun library) |
| RTMP | Native RTMP TCP handshake (pure-Go client; evaluate `aler/rtmpeg` or similar) |
| DASH | MPEG-DASH manifest parse + first segment fetch (mirrors HLS implementation) |

The probe runner's `executeProbe` branch structure (`switch p.Protocol`) is
designed for incremental enhancement: replace `probeReachability` call with
a protocol-specific function, keeping the `ResultStore` and `ProbeConfigSource`
interfaces unchanged.

## Amendments

### Amended — D-072 (2026-07-10): WebRTC phase-1 signaling probe

The WebRTC stub was replaced by a signaling-level probe (`probeWebRTC`,
nhooyr.io/websocket): WS dial → `play` command → success on
`takeConfiguration`/offer; measures `connect_time_ms`; error codes
`ws_timeout`/`ws_refused`/`ws_error`. The WHIP/WHEP+STUN plan above was NOT
used — AMS 3.0.3 exposes its own WS signaling protocol, which the probe
speaks directly (fixture shapes live-captured). Media path (ICE/DTLS/SRTP
via pion) is planned for S14 (D-073 triage).

### Amended — D-073 (2026-07-10): RTMP phase-1 handshake + DASH full probe

- **RTMP** (`probeRTMP`): stdlib-only TCP C0/C1 → S0/S1/S2 → C2 with strict
  S2-echo validation (verified live against real AMS 3.0.3); measures
  `connect_time_ms`; `signaling_state=handshake_complete`; error codes
  `rtmp_timeout`/`rtmp_refused`/`rtmp_error`. No external library was needed
  (the "evaluate aler/rtmpeg" note is obsolete). AMF0 `connect` round-trip
  is a future phase.
- **DASH** (`probeDASH`): MPD parse via stdlib `encoding/xml`
  (SegmentTemplate incl. `$Number%0Nd$`/`$RepresentationID$`, SegmentList,
  BaseURL chain per ISO/IEC 23009-1 §5.6, RFC 3986 resolution) + first
  segment fetch; mirrors the HLS measurement shape (manifest TTFB, segment
  TTFB, timescale-adjusted `bitrate_kbps`) — exactly the Phase-3 plan above.
- `probeReachability`/`not_probed` now applies ONLY to unknown/non-enum
  protocol strings; the `executeProbe` switch has dedicated cases for all
  four contract protocols, confirming the incremental-enhancement design.

### Amended — D-074 (2026-07-10): WebRTC phase-2a ICE media-path check

`probeWebRTC` now continues past the offer into pion ICE negotiation
(`continueWebRTCICE`, github.com/pion/webrtc/v4 — pure Go, CGO=0 verified):
SDP answer + trickle `takeCandidate` exchange over the SAME AMS WS session →
new `ice_state` field (`connected` | `failed` | `timeout`; CH migration 0007;
key omitted when ICE was not attempted). New error codes `ice_failed` /
`ice_timeout`. ICE outcome NEVER flips `Success` — signaling success is the
success criterion, ICE is a bonus measurement (mirrors the HLS
manifest-vs-segment semantics). The WHIP/WHEP+STUN idea above is now fully
retired: both signaling AND media-path validation ride AMS's own WS protocol.
RTCP receiver stats (rtt/jitter/loss) remain phase-2b (CH 0008 reserved,
D-074 pre-declared yield).
