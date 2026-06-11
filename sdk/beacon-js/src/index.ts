/**
 * Pulse beacon public API. Everything exported from this file is the SDK's
 * public surface; keep it tiny — the 15 KB gzipped budget is a release gate.
 *
 * Module layout:
 *   index.ts     — PulseBeacon facade, session lifecycle, sampling decision
 *   webrtc.ts    — AMS WebRTC adapter instrumentation (getStats polling)
 *   hls.ts       — media-element / hls.js / video.js instrumentation
 *   transport.ts — batching, sendBeacon + fetch fallback, retry queue
 */

export interface PulseBeaconConfig {
  /** Pulse collector base URL (HTTPS). */
  collector: string;
  /** Ingest token issued by Pulse. */
  token: string;
  /** Stream identifier as known to AMS. */
  streamId: string;
  /** Optional customer metadata (e.g. tenant tag for billing reports). */
  meta?: Record<string, string>;
  /** Sampling rate 0–1; default 1 (report every session). */
  sampleRate?: number;
}

export class PulseBeacon {
  /**
   * Create and start a beacon session. Never throws: on any configuration or
   * environment problem the returned instance is a silent no-op (PRD F3 —
   * telemetry must never break playback).
   */
  static init(_config: PulseBeaconConfig): PulseBeacon {
    // TODO(SDK-01, Phase 2)
    return new PulseBeacon();
  }

  /** Instrument an AMS WebRTC adaptor (WebRTC playback, getStats-based). */
  attachWebRTC(_adaptor: unknown): void {
    // TODO(SDK-01, Phase 2)
  }

  /** Instrument an HTMLMediaElement (HLS/DASH playback via hls.js, video.js, native). */
  attachMedia(_el: unknown): void {
    // TODO(SDK-01, Phase 2)
  }

  /** Flush pending events and end the session. */
  destroy(): void {
    // TODO(SDK-01, Phase 2)
  }
}
