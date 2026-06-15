/**
 * Shared types for @pulse/beacon SDK.
 * Aligned with contracts/events/beacon-event.schema.json (frozen, D-004).
 */

/** SDK public configuration. */
export interface PulseConfig {
  /** Pulse collector base URL (HTTPS). */
  ingestUrl: string;
  /** Ingest token issued by Pulse. */
  token: string;
  /** Stream identifier matching the AMS stream name. */
  streamId: string;
  /** AMS application name. */
  app?: string;
  /** Customer-supplied metadata (string key-value pairs). */
  metadata?: Record<string, string>;
  /** Sampling rate 0–1; default 1. Decided once per session. */
  sampleRate?: number;
}

/** Player kind enum matching the schema. */
export type PlayerKind = 'ams-webrtc' | 'hls.js' | 'video.js' | 'native' | 'other';

/** Beacon event types matching the schema enum. */
export type BeaconEventType =
  | 'session_start'
  | 'startup_complete'
  | 'heartbeat'
  | 'rebuffer_start'
  | 'rebuffer_end'
  | 'error'
  | 'bitrate_change'
  | 'resolution_change'
  | 'session_end';

/** Individual event item in a batch. */
export interface BeaconEventItem {
  type: BeaconEventType;
  /** Unix epoch ms — client-side timestamp. */
  ts: number;
  data?: Record<string, unknown>;
}

/** Full beacon batch payload matching beacon-event.schema.json. */
export interface BeaconBatch {
  version: 1;
  session_id: string;
  stream_id: string;
  app?: string;
  meta?: Record<string, string>;
  player?: {
    kind: PlayerKind;
    sdk_version: string;
  };
  events: BeaconEventItem[];
}

/** Session handle returned by PulseBeacon.init(). */
export interface SessionHandle {
  readonly sessionId: string;
  /** Instrument an AMS WebRTC adaptor. */
  attachWebRTC(adaptor: RTCAdaptor): void;
  /** Instrument an hls.js Hls instance. */
  attachHls(hls: HlsLike): void;
  /** Instrument a plain HTMLVideoElement (also works for video.js underlying element). */
  attachVideoElement(el: HTMLVideoElement): void;
  /** Emit a custom event with optional data. */
  event(type: BeaconEventType, data?: Record<string, unknown>): void;
  /** Flush pending events and end the session gracefully. */
  dispose(): void;
}

/** Minimal shape of the AMS WebRTC adaptor needed for instrumentation. */
export interface RTCAdaptor {
  peerconnection?: RTCPeerConnection | null;
  remotePeerConnections?: Record<string, RTCPeerConnection>;
}

/** Minimal hls.js Hls shape needed for event listening. */
export interface HlsLike {
  on(event: string, callback: (...args: unknown[]) => void): void;
  off(event: string, callback: (...args: unknown[]) => void): void;
  /** hls.js levels array — each entry has at minimum a bitrate field (bps). */
  levels?: Array<{ bitrate: number }>;
}
