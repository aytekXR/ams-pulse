/**
 * HlsAdapter tests:
 * - VD-12: rebuffer_end emitted on FRAG_BUFFERED after a stall
 * - VD-13: bitrate_change populates from_kbps/to_kbps from hls.levels[].bitrate
 */
import { describe, it, expect } from 'vitest';
import { HlsAdapter } from '../hls.js';
import type { BeaconEventItem, HlsLike } from '../types.js';

// ---------------------------------------------------------------------------
// Minimal HlsLike stub
// ---------------------------------------------------------------------------

type EventCallback = (...args: unknown[]) => void;

function makeHls(levels?: Array<{ bitrate: number }>): {
  hls: HlsLike;
  emit: (event: string, data?: unknown) => void;
} {
  const listeners: Record<string, EventCallback[]> = {};
  const hls: HlsLike = {
    on(event: string, cb: EventCallback) {
      (listeners[event] ??= []).push(cb);
    },
    off(event: string, cb: EventCallback) {
      listeners[event] = (listeners[event] ?? []).filter((fn) => fn !== cb);
    },
    levels,
  };
  return {
    hls,
    emit(event: string, data?: unknown) {
      for (const cb of listeners[event] ?? []) {
        cb(event, data);
      }
    },
  };
}

// ---------------------------------------------------------------------------
// VD-12 — rebuffer_end on FRAG_BUFFERED after a stall
// ---------------------------------------------------------------------------

describe('HlsAdapter — VD-12: rebuffer_end after stall', () => {
  it('emits rebuffer_end on FRAG_BUFFERED when a stall is active', () => {
    const collected: BeaconEventItem[] = [];
    const { hls, emit } = makeHls();
    const adapter = new HlsAdapter(hls, (ev) => collected.push(ev));

    // Simulate manifest loaded (so startup_complete fires on first FRAG_BUFFERED)
    emit('hlsManifestLoaded', {});
    // First FRAG_BUFFERED → startup_complete (clears startupEmitted)
    emit('hlsFragBuffered', {});

    // Verify startup_complete was emitted
    const startupEvents = collected.filter((e) => e.type === 'startup_complete');
    expect(startupEvents).toHaveLength(1);

    // Now stall
    emit('hlsBufferStalled', {});
    const rebufferStarts = collected.filter((e) => e.type === 'rebuffer_start');
    expect(rebufferStarts).toHaveLength(1);

    // FRAG_BUFFERED while stalling → must emit rebuffer_end
    emit('hlsFragBuffered', {});
    const rebufferEnds = collected.filter((e) => e.type === 'rebuffer_end');
    expect(rebufferEnds).toHaveLength(1);
    expect(typeof rebufferEnds[0].data?.['duration_ms']).toBe('number');
    expect((rebufferEnds[0].data?.['duration_ms'] as number)).toBeGreaterThanOrEqual(0);

    adapter.dispose();
  });

  it('does NOT emit rebuffer_end on FRAG_BUFFERED when no stall is active', () => {
    const collected: BeaconEventItem[] = [];
    const { hls, emit } = makeHls();
    const adapter = new HlsAdapter(hls, (ev) => collected.push(ev));

    // No stall before FRAG_BUFFERED
    emit('hlsManifestLoaded', {});
    emit('hlsFragBuffered', {});

    const rebufferEnds = collected.filter((e) => e.type === 'rebuffer_end');
    expect(rebufferEnds).toHaveLength(0);

    adapter.dispose();
  });

  it('closes the stall only once — second FRAG_BUFFERED without re-stall does not emit rebuffer_end', () => {
    const collected: BeaconEventItem[] = [];
    const { hls, emit } = makeHls();
    const adapter = new HlsAdapter(hls, (ev) => collected.push(ev));

    emit('hlsManifestLoaded', {});
    emit('hlsFragBuffered', {}); // startup

    emit('hlsBufferStalled', {}); // stall open
    emit('hlsFragBuffered', {}); // stall closed → rebuffer_end
    emit('hlsFragBuffered', {}); // no stall → no rebuffer_end

    const rebufferEnds = collected.filter((e) => e.type === 'rebuffer_end');
    expect(rebufferEnds).toHaveLength(1);

    adapter.dispose();
  });

  it('handles multiple stall/resume cycles correctly', () => {
    const collected: BeaconEventItem[] = [];
    const { hls, emit } = makeHls();
    const adapter = new HlsAdapter(hls, (ev) => collected.push(ev));

    emit('hlsManifestLoaded', {});
    emit('hlsFragBuffered', {}); // startup

    // Cycle 1
    emit('hlsBufferStalled', {});
    emit('hlsFragBuffered', {});
    // Cycle 2
    emit('hlsBufferStalled', {});
    emit('hlsFragBuffered', {});

    const rebufferStarts = collected.filter((e) => e.type === 'rebuffer_start');
    const rebufferEnds = collected.filter((e) => e.type === 'rebuffer_end');
    expect(rebufferStarts).toHaveLength(2);
    expect(rebufferEnds).toHaveLength(2);

    adapter.dispose();
  });
});

// ---------------------------------------------------------------------------
// VD-13 — bitrate_change uses hls.levels[].bitrate
// ---------------------------------------------------------------------------

describe('HlsAdapter — VD-13: bitrate_change from_kbps/to_kbps', () => {
  const levels = [
    { bitrate: 300_000 },  // level 0 → 300 kbps
    { bitrate: 1_500_000 }, // level 1 → 1500 kbps
    { bitrate: 4_000_000 }, // level 2 → 4000 kbps
  ];

  it('populates to_kbps from hls.levels[level].bitrate', () => {
    const collected: BeaconEventItem[] = [];
    const { hls, emit } = makeHls(levels);
    const adapter = new HlsAdapter(hls, (ev) => collected.push(ev));

    // Switch to level 1
    emit('hlsLevelSwitched', { level: 1 });

    const bc = collected.find((e) => e.type === 'bitrate_change');
    expect(bc).toBeDefined();
    expect(bc?.data?.['to_kbps']).toBe(1500);

    adapter.dispose();
  });

  it('populates from_kbps from the previous level bitrate', () => {
    const collected: BeaconEventItem[] = [];
    const { hls, emit } = makeHls(levels);
    const adapter = new HlsAdapter(hls, (ev) => collected.push(ev));

    // First switch to level 0 (from_kbps=0, no prior level)
    emit('hlsLevelSwitched', { level: 0 });
    // Then switch to level 2 (from_kbps should be level 0 = 300 kbps)
    emit('hlsLevelSwitched', { level: 2 });

    const bcs = collected.filter((e) => e.type === 'bitrate_change');
    expect(bcs).toHaveLength(2);

    expect(bcs[0].data?.['from_kbps']).toBe(0); // no prior level
    expect(bcs[0].data?.['to_kbps']).toBe(300);

    expect(bcs[1].data?.['from_kbps']).toBe(300); // was at level 0
    expect(bcs[1].data?.['to_kbps']).toBe(4000);

    adapter.dispose();
  });

  it('emits 0/0 when hls.levels is not provided (graceful fallback)', () => {
    const collected: BeaconEventItem[] = [];
    const { hls, emit } = makeHls(undefined); // no levels
    const adapter = new HlsAdapter(hls, (ev) => collected.push(ev));

    emit('hlsLevelSwitched', { level: 1 });

    const bc = collected.find((e) => e.type === 'bitrate_change');
    expect(bc).toBeDefined();
    expect(bc?.data?.['from_kbps']).toBe(0);
    expect(bc?.data?.['to_kbps']).toBe(0);

    adapter.dispose();
  });

  it('hls_level field is always present', () => {
    const collected: BeaconEventItem[] = [];
    const { hls, emit } = makeHls(levels);
    const adapter = new HlsAdapter(hls, (ev) => collected.push(ev));

    emit('hlsLevelSwitched', { level: 2 });

    const bc = collected.find((e) => e.type === 'bitrate_change');
    expect(bc?.data?.['hls_level']).toBe(2);

    adapter.dispose();
  });
});
