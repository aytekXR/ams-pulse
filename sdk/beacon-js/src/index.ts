/**
 * @pulse/beacon — Player-side QoE telemetry for Pulse (Ant Media Server analytics).
 *
 * One-line init: Pulse.init({ ingestUrl, token, streamId }) returns a session handle
 * with attachWebRTC(), attachHls(), attachVideoElement(), event(), dispose().
 *
 * Contract: contracts/events/beacon-event.schema.json (frozen D-004).
 * Hard gate: < 15 KB gzipped; zero runtime deps; zero throws.
 * MIT-licensed open source (PRD §7.12).
 */

export type { PulseConfig, PlayerKind, BeaconEventType, BeaconEventItem, BeaconBatch, SessionHandle, RTCAdaptor, HlsLike } from './types.js';

import type { PulseConfig, BeaconEventItem, BeaconEventType, PlayerKind, SessionHandle, RTCAdaptor, HlsLike } from './types.js';
import { generateSessionId, isSampled } from './session.js';
import { Transport } from './transport.js';
import { WebRTCAdapter } from './webrtc.js';
import { MediaElementAdapter, HlsAdapter } from './hls.js';

/** SDK version embedded at build time (updated on release). */
const SDK_VERSION = '0.1.0';

/** Internal no-op session returned when sampling excludes the session or config is invalid. */
class NoOpSession implements SessionHandle {
  readonly sessionId: string;
  constructor() {
    this.sessionId = generateSessionId();
  }
  attachWebRTC(_adaptor: RTCAdaptor): void { /* no-op */ }
  attachHls(_hls: HlsLike): void { /* no-op */ }
  attachVideoElement(_el: HTMLVideoElement): void { /* no-op */ }
  event(_type: BeaconEventType, _data?: Record<string, unknown>): void { /* no-op */ }
  dispose(): void { /* no-op */ }
}

/** Live session that actually emits events. */
class LiveSession implements SessionHandle {
  readonly sessionId: string;
  private readonly transport: Transport;
  private webrtcAdapter: WebRTCAdapter | null = null;
  private mediaAdapter: MediaElementAdapter | null = null;
  private hlsAdapter: HlsAdapter | null = null;
  private disposed = false;

  constructor(cfg: PulseConfig, playerKind: PlayerKind) {
    this.sessionId = generateSessionId();
    this.transport = new Transport({
      ingestUrl: cfg.ingestUrl,
      token: cfg.token,
      sessionId: this.sessionId,
      streamId: cfg.streamId,
      app: cfg.app,
      meta: cfg.metadata,
      playerKind,
      sdkVersion: SDK_VERSION,
    });
    // Emit session_start
    this._emit({
      type: 'session_start',
      ts: Date.now(),
      data: {
        ...(typeof window !== 'undefined' && window.location
          ? { page_url: window.location.href }
          : {}),
      },
    });
  }

  attachWebRTC(adaptor: RTCAdaptor): void {
    if (this.disposed) return;
    try {
      this.webrtcAdapter?.dispose();
      this.webrtcAdapter = new WebRTCAdapter(adaptor, (e) => this._emit(e));
    } catch {
      // never throw
    }
  }

  attachHls(hls: HlsLike): void {
    if (this.disposed) return;
    try {
      this.hlsAdapter?.dispose();
      this.hlsAdapter = new HlsAdapter(hls, (e) => this._emit(e));
    } catch {
      // never throw
    }
  }

  attachVideoElement(el: HTMLVideoElement): void {
    if (this.disposed) return;
    try {
      this.mediaAdapter?.dispose();
      this.mediaAdapter = new MediaElementAdapter(el, (e) => this._emit(e));
    } catch {
      // never throw
    }
  }

  event(type: BeaconEventType, data?: Record<string, unknown>): void {
    if (this.disposed) return;
    this._emit({ type, ts: Date.now(), data });
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    try {
      this.webrtcAdapter?.dispose();
      this.hlsAdapter?.dispose();
      this.mediaAdapter?.dispose();
      this._emit({
        type: 'session_end',
        ts: Date.now(),
        data: { reason: 'user_exit' },
      });
      this.transport.dispose();
    } catch {
      // never throw
    }
  }

  private _emit(event: BeaconEventItem): void {
    try {
      this.transport.push(event);
    } catch {
      // never throw
    }
  }
}

/**
 * Main Pulse beacon facade.
 *
 * @example
 * ```ts
 * const session = Pulse.init({
 *   ingestUrl: 'https://pulse.example.com',
 *   token: 'plt_...',
 *   streamId: 'auction-main',
 *   app: 'live',
 *   metadata: { tenant: 'client-42' },
 *   sampleRate: 0.1,
 * });
 *
 * session.attachWebRTC(webRTCAdaptor);    // AMS WebRTC adapter
 * session.attachHls(hlsInstance);         // hls.js Hls instance
 * session.attachVideoElement(videoEl);    // plain <video> or video.js .el()
 *
 * // on page unload / player destroy:
 * session.dispose();
 * ```
 */
export const Pulse = {
  /**
   * Create and start a beacon session.
   *
   * Never throws: on any configuration or environment problem the returned
   * instance is a silent no-op (PRD F3 — telemetry must never break playback).
   *
   * @param cfg - SDK configuration
   * @param playerKind - optional player kind tag for analytics
   * @returns a session handle
   */
  init(cfg: PulseConfig, playerKind: PlayerKind = 'other'): SessionHandle {
    try {
      // Validate required fields — return no-op on bad config
      if (!cfg || typeof cfg !== 'object') {
        _debugNote('Pulse.init: config must be an object');
        return new NoOpSession();
      }
      if (!cfg.ingestUrl || typeof cfg.ingestUrl !== 'string') {
        _debugNote('Pulse.init: ingestUrl is required');
        return new NoOpSession();
      }
      if (!cfg.token || typeof cfg.token !== 'string') {
        _debugNote('Pulse.init: token is required');
        return new NoOpSession();
      }
      if (!cfg.streamId || typeof cfg.streamId !== 'string') {
        _debugNote('Pulse.init: streamId is required');
        return new NoOpSession();
      }

      // Sampling decision — made once per session
      const sampleRate = typeof cfg.sampleRate === 'number' ? cfg.sampleRate : 1;
      if (!isSampled(sampleRate)) {
        return new NoOpSession();
      }

      return new LiveSession(cfg, playerKind);
    } catch {
      // Last-resort: return no-op; never let init() throw
      return new NoOpSession();
    }
  },
};

/** Single debug-level note when collector is unreachable or config is invalid.
 * Emits a single console.debug message; never spams. */
function _debugNote(msg: string): void {
  try {
    // Use console.debug (lowest priority — not shown in production devtools by default)
    // eslint-disable-next-line no-console
    if (typeof console !== 'undefined' && typeof console.debug === 'function') {
      // eslint-disable-next-line no-console
      console.debug(`[pulse/beacon] ${msg}`);
    }
  } catch {
    // ignore
  }
}

// Convenience re-export of init as a named export for tree-shaking friendliness
export const init = Pulse.init.bind(Pulse);
