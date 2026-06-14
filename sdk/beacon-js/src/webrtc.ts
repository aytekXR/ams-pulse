/**
 * WebRTC instrumentation: polls RTCPeerConnection.getStats() every 5 s,
 * derives startup_complete (first frame rendered), heartbeat (watch_ms,
 * bitrate_kbps, dropped_frames), stall detection via frame-counter deltas,
 * bitrate_change, and resolution_change events.
 *
 * Works with the AMS WebRTC adaptor pattern (adaptor.peerconnection or
 * adaptor.remotePeerConnections for multi-party calls).
 * Never throws — all errors are caught silently.
 */

import type { BeaconEventItem, RTCAdaptor } from './types.js';

/** Poll cadence in ms — 5 s per WO-201 spec. */
const STATS_INTERVAL_MS = 5_000;

interface InboundStats {
  framesDecoded: number;
  framesDropped: number;
  frameWidth: number;
  frameHeight: number;
  bytesReceived: number;
  timestamp: number;
}

type EventEmitter = (event: BeaconEventItem) => void;

export class WebRTCAdapter {
  private readonly emit: EventEmitter;
  private timer: ReturnType<typeof setInterval> | null = null;
  private adaptor: RTCAdaptor;
  private startedAt: number = Date.now();
  private startupEmitted = false;
  private lastStats: InboundStats | null = null;
  private lastBitrateKbps = 0;
  private lastResolution = '';
  private watchMs = 0;
  private lastTickAt = 0;
  private isPlaying = false;
  private stallStartAt: number | null = null;

  constructor(adaptor: RTCAdaptor, emit: EventEmitter) {
    this.adaptor = adaptor;
    this.emit = emit;
    this.startedAt = Date.now();
    this.lastTickAt = Date.now();
    this._start();
  }

  private _start(): void {
    this.timer = setInterval(() => {
      void this._tick();
    }, STATS_INTERVAL_MS);
  }

  private _getPeerConnection(): RTCPeerConnection | null {
    try {
      if (this.adaptor.peerconnection) return this.adaptor.peerconnection;
      if (this.adaptor.remotePeerConnections) {
        const conns = Object.values(this.adaptor.remotePeerConnections);
        if (conns.length > 0) return conns[0];
      }
    } catch {
      // ignore
    }
    return null;
  }

  private async _tick(): Promise<void> {
    try {
      const pc = this._getPeerConnection();
      if (!pc) return;

      const stats = await pc.getStats();
      const now = Date.now();
      const deltaSec = (now - this.lastTickAt) / 1000;
      this.lastTickAt = now;

      let current: InboundStats | null = null;

      stats.forEach((report) => {
        if (report.type === 'inbound-rtp' && (report as RTCInboundRtpStreamStats).kind === 'video') {
          const r = report as RTCInboundRtpStreamStats;
          current = {
            framesDecoded: (r.framesDecoded ?? 0),
            framesDropped: (r.framesDropped ?? 0),
            frameWidth: (r.frameWidth ?? 0),
            frameHeight: (r.frameHeight ?? 0),
            bytesReceived: (r.bytesReceived ?? 0),
            timestamp: now,
          };
        }
      });

      if (!current) return;

      const cur = current as InboundStats;
      const prev = this.lastStats;

      // Startup detection: first frame decoded
      if (!this.startupEmitted && cur.framesDecoded > 0) {
        this.startupEmitted = true;
        const startupMs = now - this.startedAt;

        // Compute initial bitrate from bytesReceived over delta
        let bitrateKbps = 0;
        if (prev && deltaSec > 0) {
          bitrateKbps = ((cur.bytesReceived - prev.bytesReceived) * 8) / (deltaSec * 1000);
        }

        this.emit({
          type: 'startup_complete',
          ts: now,
          data: {
            startup_ms: Math.round(startupMs),
            ...(bitrateKbps > 0 ? { bitrate_kbps: Math.round(bitrateKbps) } : {}),
          },
        });

        this.isPlaying = true;
      }

      if (!prev) {
        this.lastStats = cur;
        return;
      }

      const deltaDecoded = cur.framesDecoded - prev.framesDecoded;
      const deltaDropped = cur.framesDropped - prev.framesDropped;

      // Stall detection via frame-counter delta
      if (this.startupEmitted) {
        const framesStalledThisTick = deltaDecoded === 0 && this.isPlaying;
        if (framesStalledThisTick && this.stallStartAt === null) {
          this.stallStartAt = now;
          this.emit({
            type: 'rebuffer_start',
            ts: now,
            data: { buffer_ms: 0 },
          });
          this.isPlaying = false;
        } else if (!framesStalledThisTick && this.stallStartAt !== null) {
          const duration_ms = now - this.stallStartAt;
          this.stallStartAt = null;
          this.isPlaying = true;
          this.emit({
            type: 'rebuffer_end',
            ts: now,
            data: { duration_ms },
          });
        }
      }

      // Bitrate
      let bitrateKbps = 0;
      if (deltaSec > 0 && cur.bytesReceived > prev.bytesReceived) {
        bitrateKbps = ((cur.bytesReceived - prev.bytesReceived) * 8) / (deltaSec * 1000);
      }

      // Bitrate change detection (>10% change threshold)
      if (this.lastBitrateKbps > 0 && bitrateKbps > 0) {
        const ratio = Math.abs(bitrateKbps - this.lastBitrateKbps) / this.lastBitrateKbps;
        if (ratio > 0.1) {
          this.emit({
            type: 'bitrate_change',
            ts: now,
            data: {
              from_kbps: Math.round(this.lastBitrateKbps),
              to_kbps: Math.round(bitrateKbps),
            },
          });
        }
      }
      if (bitrateKbps > 0) this.lastBitrateKbps = bitrateKbps;

      // Resolution change detection
      if (cur.frameWidth > 0 && cur.frameHeight > 0) {
        const resolution = `${cur.frameWidth}x${cur.frameHeight}`;
        if (this.lastResolution && this.lastResolution !== resolution) {
          this.emit({
            type: 'resolution_change',
            ts: now,
            data: {
              from: this.lastResolution,
              to: resolution,
            },
          });
        }
        this.lastResolution = resolution;
      }

      // Heartbeat: accumulate watch time when frames are being decoded
      if (this.isPlaying) {
        this.watchMs += Math.round(deltaSec * 1000);
        this.emit({
          type: 'heartbeat',
          ts: now,
          data: {
            watch_ms: this.watchMs,
            ...(bitrateKbps > 0 ? { bitrate_kbps: Math.round(bitrateKbps) } : {}),
            ...(deltaDropped > 0 ? { dropped_frames: deltaDropped } : {}),
          },
        });
      }

      this.lastStats = cur;
    } catch {
      // never throw — silent no-op on stats failure
    }
  }

  /** Stop polling and release resources. */
  dispose(): void {
    if (this.timer !== null) {
      clearInterval(this.timer);
      this.timer = null;
    }
  }
}
