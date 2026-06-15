/**
 * Event transport: batches events (flush every ≤10 s or 25 events, or on
 * visibilitychange/pagehide), prefers navigator.sendBeacon with fetch keepalive
 * fallback, keeps a bounded in-memory retry queue (+ localStorage spill),
 * exponential backoff with cap, silently drops on persistent failure.
 *
 * Payload shape: contracts/events/beacon-event.schema.json (frozen D-004).
 * Zero runtime deps; never throws.
 */

import type { BeaconEventItem, BeaconBatch, PlayerKind } from './types.js';

/** Internal transport configuration. */
export interface TransportConfig {
  ingestUrl: string;
  token: string;
  sessionId: string;
  streamId: string;
  app?: string;
  meta?: Record<string, string>;
  playerKind: PlayerKind;
  sdkVersion: string;
}

/** Maximum events buffered before a forced flush. */
const MAX_BATCH_SIZE = 25;

/** Flush interval in ms (≤10 s per contract). */
const FLUSH_INTERVAL_MS = 10_000;

/** Max retry queue depth (in-memory). */
const MAX_QUEUE_DEPTH = 100;

/** Exponential backoff: base delay ms. */
const BACKOFF_BASE_MS = 1_000;

/** Exponential backoff: max delay ms. */
const BACKOFF_CAP_MS = 60_000;

/** localStorage key prefix for spill queue. */
const LS_KEY = 'pulse_beacon_q';

export class Transport {
  private readonly cfg: TransportConfig;
  private buffer: BeaconEventItem[] = [];
  private retryQueue: BeaconBatch[] = [];
  private flushTimer: ReturnType<typeof setTimeout> | null = null;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private backoffMs = BACKOFF_BASE_MS;
  private destroyed = false;

  /** Bound handlers for lifecycle events (needed for removeEventListener). */
  private readonly onVisibility: () => void;
  private readonly onPagehide: () => void;

  constructor(cfg: TransportConfig) {
    this.cfg = cfg;

    this.onVisibility = () => {
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
        this._flush(true);
      }
    };
    this.onPagehide = () => {
      this._flush(true);
    };

    this._attachLifecycle();
    this._scheduleFlush();
    this._drainLocalStorageSpill();
  }

  /** Enqueue an event. Flushes immediately if batch is full. */
  push(item: BeaconEventItem): void {
    if (this.destroyed) return;
    try {
      this.buffer.push(item);
      if (this.buffer.length >= MAX_BATCH_SIZE) {
        this._flush(false);
      }
    } catch {
      // never throw
    }
  }

  /** Flush pending events and tear down. */
  dispose(): void {
    if (this.destroyed) return;
    this.destroyed = true;
    this._flush(true);
    this._detachLifecycle();
    if (this.flushTimer !== null) clearTimeout(this.flushTimer);
    if (this.retryTimer !== null) clearTimeout(this.retryTimer);
  }

  // ---------------------------------------------------------------------------
  // Internal helpers
  // ---------------------------------------------------------------------------

  private _scheduleFlush(): void {
    if (this.destroyed) return;
    this.flushTimer = setTimeout(() => {
      this._flush(false);
      this._scheduleFlush();
    }, FLUSH_INTERVAL_MS);
  }

  /** Flush the in-flight buffer.
   * @param useSendBeacon - when true, prefer sendBeacon (page unload paths).
   */
  private _flush(useSendBeacon: boolean): void {
    if (this.buffer.length === 0) return;
    const events = this.buffer.splice(0);
    const batch = this._buildBatch(events);
    this._send(batch, useSendBeacon);
  }

  private _buildBatch(events: BeaconEventItem[]): BeaconBatch {
    return {
      version: 1,
      session_id: this.cfg.sessionId,
      stream_id: this.cfg.streamId,
      ...(this.cfg.app !== undefined ? { app: this.cfg.app } : {}),
      ...(this.cfg.meta !== undefined ? { meta: this.cfg.meta } : {}),
      player: {
        kind: this.cfg.playerKind,
        sdk_version: this.cfg.sdkVersion,
      },
      events,
    };
  }

  private _send(batch: BeaconBatch, preferBeacon: boolean): void {
    const url = `${this.cfg.ingestUrl}/ingest/beacon`;
    const body = JSON.stringify(batch);
    const headers = {
      'Content-Type': 'application/json',
      'X-Pulse-Ingest-Token': this.cfg.token,
    };

    try {
      if (preferBeacon && typeof navigator !== 'undefined' && navigator.sendBeacon) {
        const blob = new Blob([body], { type: 'application/json' });
        const sent = navigator.sendBeacon(url, blob);
        if (sent) return;
        // sendBeacon returned false (queue full) — fall through to fetch
      }
      // fetch with keepalive for background delivery
      if (typeof fetch !== 'undefined') {
        fetch(url, {
          method: 'POST',
          headers,
          body,
          keepalive: true,
        }).then((res) => {
          if (!res.ok) {
            this._enqueueRetry(batch);
          } else {
            // success — reset backoff
            this.backoffMs = BACKOFF_BASE_MS;
          }
        }).catch(() => {
          this._enqueueRetry(batch);
        });
      } else {
        // No fetch API — spill to localStorage
        this._spillToLocalStorage(batch);
      }
    } catch {
      this._enqueueRetry(batch);
    }
  }

  private _enqueueRetry(batch: BeaconBatch): void {
    if (this.retryQueue.length >= MAX_QUEUE_DEPTH) {
      // drop-oldest at cap
      this.retryQueue.shift();
    }
    this.retryQueue.push(batch);
    this._spillToLocalStorage(batch);
    this._scheduleRetry();
  }

  private _scheduleRetry(): void {
    if (this.retryTimer !== null || this.destroyed) return;
    this.retryTimer = setTimeout(() => {
      this.retryTimer = null;
      this._retryNext();
    }, this.backoffMs);
    // exponential backoff with cap
    this.backoffMs = Math.min(this.backoffMs * 2, BACKOFF_CAP_MS);
  }

  private _retryNext(): void {
    if (this.retryQueue.length === 0 || this.destroyed) return;
    const batch = this.retryQueue.shift();
    if (!batch) return;
    this._send(batch, false);
    if (this.retryQueue.length > 0) {
      this._scheduleRetry();
    }
  }

  /** Spill a batch to localStorage for cross-page recovery. */
  private _spillToLocalStorage(batch: BeaconBatch): void {
    try {
      if (typeof localStorage === 'undefined') return;
      const raw = localStorage.getItem(LS_KEY);
      const queue: BeaconBatch[] = raw ? (JSON.parse(raw) as BeaconBatch[]) : [];
      if (queue.length < MAX_QUEUE_DEPTH) {
        queue.push(batch);
        localStorage.setItem(LS_KEY, JSON.stringify(queue));
      }
    } catch {
      // localStorage may be unavailable (private mode, quota exceeded)
    }
  }

  /** Drain previously spilled batches from localStorage on init. */
  private _drainLocalStorageSpill(): void {
    try {
      if (typeof localStorage === 'undefined') return;
      const raw = localStorage.getItem(LS_KEY);
      if (!raw) return;
      const queue: BeaconBatch[] = JSON.parse(raw) as BeaconBatch[];
      localStorage.removeItem(LS_KEY);
      // Retry each spilled batch (re-queues on failure)
      for (const batch of queue) {
        this._send(batch, false);
      }
    } catch {
      // ignore
    }
  }

  private _attachLifecycle(): void {
    try {
      if (typeof document !== 'undefined') {
        document.addEventListener('visibilitychange', this.onVisibility);
      }
      if (typeof window !== 'undefined') {
        window.addEventListener('pagehide', this.onPagehide);
      }
    } catch {
      // non-browser environment — no-op
    }
  }

  private _detachLifecycle(): void {
    try {
      if (typeof document !== 'undefined') {
        document.removeEventListener('visibilitychange', this.onVisibility);
      }
      if (typeof window !== 'undefined') {
        window.removeEventListener('pagehide', this.onPagehide);
      }
    } catch {
      // ignore
    }
  }
}

// Re-export for tests
export { MAX_BATCH_SIZE, FLUSH_INTERVAL_MS, MAX_QUEUE_DEPTH, BACKOFF_BASE_MS, BACKOFF_CAP_MS };
