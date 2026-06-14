/**
 * Media-element + hls.js instrumentation.
 *
 * Two layers:
 *   1. HTMLVideoElement events (universal, covers native HLS, video.js underlying element)
 *   2. hls.js Hls events for more precise timing (optional, layered on top of #1)
 *
 * No hls.js or video.js production dependency — we only listen to their events via
 * the documented public event interfaces. video.js integration: use the underlying
 * <video> element (videojs_instance.tech_?.el_() or videojs_instance.el()).
 * Never throws.
 */

import type { BeaconEventItem, HlsLike } from './types.js';

type EventEmitter = (event: BeaconEventItem) => void;

/** hls.js event name constants we care about. */
const HLS_EVENTS = {
  MANIFEST_LOADED: 'hlsManifestLoaded',
  FRAG_BUFFERED: 'hlsFragBuffered',
  BUFFER_STALLED: 'hlsBufferStalled',
  BUFFER_APPENDING: 'hlsBufferAppending',
  ERROR: 'hlsError',
  LEVEL_SWITCHED: 'hlsLevelSwitched',
} as const;

export class MediaElementAdapter {
  private readonly el: HTMLVideoElement;
  private readonly emit: EventEmitter;
  private playStartAt: number | null = null;
  private startupEmitted = false;
  private stallStartAt: number | null = null;
  private watchMs = 0;
  private lastHeartbeatAt = 0;

  /** Bound event listeners (needed for cleanup). */
  private readonly handlers: Array<[string, EventListenerOrEventListenerObject]> = [];

  constructor(el: HTMLVideoElement, emit: EventEmitter) {
    this.el = el;
    this.emit = emit;
    this._attach();
  }

  private _attach(): void {
    this._on('loadstart', this._onLoadstart);
    this._on('playing', this._onPlaying);
    this._on('waiting', this._onWaiting);
    this._on('stalled', this._onStalled);
    this._on('error', this._onError);
    this._on('ratechange', this._onRatechange);
    this._on('ended', this._onEnded);
    this._on('timeupdate', this._onTimeupdate);
  }

  private _on(event: string, handler: () => void): void {
    const bound = handler.bind(this) as EventListener;
    this.handlers.push([event, bound]);
    try {
      this.el.addEventListener(event, bound);
    } catch {
      // ignore — non-browser environment
    }
  }

  private _onLoadstart(): void {
    this.playStartAt = Date.now();
    this.startupEmitted = false;
  }

  private _onPlaying(): void {
    const now = Date.now();

    // startup_complete
    if (!this.startupEmitted && this.playStartAt !== null) {
      this.startupEmitted = true;
      const startup_ms = now - this.playStartAt;
      this.emit({
        type: 'startup_complete',
        ts: now,
        data: { startup_ms },
      });
      this.lastHeartbeatAt = now;
    }

    // rebuffer_end
    if (this.stallStartAt !== null) {
      const duration_ms = now - this.stallStartAt;
      this.stallStartAt = null;
      this.emit({
        type: 'rebuffer_end',
        ts: now,
        data: { duration_ms },
      });
    }
  }

  private _onWaiting(): void {
    if (this.stallStartAt === null) {
      this.stallStartAt = Date.now();
      this.emit({
        type: 'rebuffer_start',
        ts: Date.now(),
        data: {
          buffer_ms: this._bufferMs(),
        },
      });
    }
  }

  private _onStalled(): void {
    // Same as waiting — treat as rebuffer start if not already stalled
    this._onWaiting();
  }

  private _onError(): void {
    const now = Date.now();
    const error = this.el.error;
    if (!error) return;
    const codes: Record<number, string> = {
      1: 'MEDIA_ERR_ABORTED',
      2: 'MEDIA_ERR_NETWORK',
      3: 'MEDIA_ERR_DECODE',
      4: 'MEDIA_ERR_SRC_NOT_SUPPORTED',
    };
    this.emit({
      type: 'error',
      ts: now,
      data: {
        code: codes[error.code] ?? `MEDIA_ERR_${error.code}`,
        message: error.message || undefined,
        fatal: true,
      },
    });
  }

  private _onRatechange(): void {
    // Emit a heartbeat on rate change so the server sees it quickly
    this._emitHeartbeat();
  }

  private _onEnded(): void {
    this._emitHeartbeat();
  }

  private _onTimeupdate(): void {
    // Emit heartbeat every 30 s of watch time
    const now = Date.now();
    if (this.lastHeartbeatAt > 0 && now - this.lastHeartbeatAt >= 30_000) {
      this._emitHeartbeat();
    }
  }

  private _emitHeartbeat(): void {
    const now = Date.now();
    if (this.lastHeartbeatAt > 0) {
      this.watchMs += now - this.lastHeartbeatAt;
    }
    this.lastHeartbeatAt = now;
    this.emit({
      type: 'heartbeat',
      ts: now,
      data: {
        watch_ms: this.watchMs,
        buffer_ms: this._bufferMs(),
      },
    });
  }

  private _bufferMs(): number {
    try {
      const el = this.el;
      if (el.buffered.length === 0) return 0;
      const ct = el.currentTime;
      const end = el.buffered.end(el.buffered.length - 1);
      return Math.max(0, Math.round((end - ct) * 1000));
    } catch {
      return 0;
    }
  }

  dispose(): void {
    for (const [event, handler] of this.handlers) {
      try {
        this.el.removeEventListener(event, handler);
      } catch {
        // ignore
      }
    }
    this.handlers.length = 0;
  }
}

export class HlsAdapter {
  private readonly hls: HlsLike;
  private readonly emit: EventEmitter;
  private manifestLoadedAt: number | null = null;
  private startupEmitted = false;

  /** Bound handlers for cleanup. */
  private readonly hlsHandlers: Array<[string, (...args: unknown[]) => void]> = [];

  constructor(hls: HlsLike, emit: EventEmitter) {
    this.hls = hls;
    this.emit = emit;
    this._attach();
  }

  private _attach(): void {
    this._on(HLS_EVENTS.MANIFEST_LOADED, this._onManifestLoaded);
    this._on(HLS_EVENTS.FRAG_BUFFERED, this._onFragBuffered);
    this._on(HLS_EVENTS.BUFFER_STALLED, this._onBufferStalled);
    this._on(HLS_EVENTS.ERROR, this._onHlsError);
    this._on(HLS_EVENTS.LEVEL_SWITCHED, this._onLevelSwitched);
  }

  private _on(event: string, handler: (...args: unknown[]) => void): void {
    const bound = handler.bind(this);
    this.hlsHandlers.push([event, bound]);
    try {
      this.hls.on(event, bound);
    } catch {
      // ignore
    }
  }

  private _onManifestLoaded(): void {
    this.manifestLoadedAt = Date.now();
    this.startupEmitted = false;
  }

  private _onFragBuffered(): void {
    // First FRAG_BUFFERED after MANIFEST_LOADED = startup complete
    if (!this.startupEmitted && this.manifestLoadedAt !== null) {
      this.startupEmitted = true;
      const now = Date.now();
      const startup_ms = now - this.manifestLoadedAt;
      this.emit({
        type: 'startup_complete',
        ts: now,
        data: { startup_ms },
      });
    }
  }

  private _onBufferStalled(): void {
    this.emit({
      type: 'rebuffer_start',
      ts: Date.now(),
      data: { buffer_ms: 0 },
    });
  }

  private _onHlsError(_event: unknown, data: unknown): void {
    try {
      const d = data as { type?: string; details?: string; fatal?: boolean };
      this.emit({
        type: 'error',
        ts: Date.now(),
        data: {
          code: d.details ?? d.type ?? 'HLS_ERROR',
          fatal: d.fatal ?? false,
        },
      });
    } catch {
      // ignore
    }
  }

  private _onLevelSwitched(_event: unknown, data: unknown): void {
    try {
      const d = data as { level?: number };
      const level = d.level ?? -1;
      // Emit bitrate_change — we report level index as a proxy;
      // hls.js levels[level].bitrate is available if the caller provides
      // the Hls instance with levels, but we don't want to type it here.
      // Instead, emit a minimal resolution_change marker (level number).
      if (level >= 0) {
        this.emit({
          type: 'bitrate_change',
          ts: Date.now(),
          data: {
            from_kbps: 0,
            to_kbps: 0,
            hls_level: level,
          },
        });
      }
    } catch {
      // ignore
    }
  }

  dispose(): void {
    for (const [event, handler] of this.hlsHandlers) {
      try {
        this.hls.off(event, handler);
      } catch {
        // ignore
      }
    }
    this.hlsHandlers.length = 0;
  }
}
