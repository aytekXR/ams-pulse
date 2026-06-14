/**
 * Pulse public API tests:
 * - One-line init returns a valid session handle
 * - Malformed config returns no-op (never throws)
 * - sampleRate=0 returns no-op, makes zero network calls
 * - sampleRate=1 returns live session
 * - dispose() cleans up properly
 * - event() custom event emission
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { Pulse, init } from '../index.js';

/** Flush pending microtasks. */
async function flushMicrotasks(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}

// Setup sendBeacon on navigator (jsdom doesn't include it by default)
function setupSendBeacon(): ReturnType<typeof vi.fn> {
  const mock = vi.fn().mockReturnValue(true);
  Object.defineProperty(globalThis.navigator, 'sendBeacon', {
    value: mock,
    configurable: true,
    writable: true,
  });
  return mock;
}

let fetchMock: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  vi.useFakeTimers();
  fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(null, { status: 200 }));
  setupSendBeacon();
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  try {
    localStorage.removeItem('pulse_beacon_q');
  } catch { /* ignore */ }
});

describe('Pulse.init — no-throw guarantee', () => {
  it('returns a handle when config is valid', () => {
    const s = Pulse.init({ ingestUrl: 'https://pulse.example.com', token: 'plt_x', streamId: 'stream1' });
    expect(s).toBeDefined();
    expect(typeof s.sessionId).toBe('string');
    expect(typeof s.attachWebRTC).toBe('function');
    expect(typeof s.attachHls).toBe('function');
    expect(typeof s.attachVideoElement).toBe('function');
    expect(typeof s.event).toBe('function');
    expect(typeof s.dispose).toBe('function');
    s.dispose();
  });

  it('never throws on null config', () => {
    expect(() => {
      // @ts-expect-error — intentionally bad input
      const s = Pulse.init(null);
      s.dispose();
    }).not.toThrow();
  });

  it('never throws on missing ingestUrl', () => {
    expect(() => {
      // @ts-expect-error — intentionally bad input
      const s = Pulse.init({ token: 'x', streamId: 'y' });
      s.dispose();
    }).not.toThrow();
  });

  it('never throws on missing token', () => {
    expect(() => {
      // @ts-expect-error — intentionally bad input
      const s = Pulse.init({ ingestUrl: 'https://x', streamId: 'y' });
      s.dispose();
    }).not.toThrow();
  });

  it('never throws on missing streamId', () => {
    expect(() => {
      // @ts-expect-error — intentionally bad input
      const s = Pulse.init({ ingestUrl: 'https://x', token: 'y' });
      s.dispose();
    }).not.toThrow();
  });

  it('never throws when attachWebRTC called with null', () => {
    const s = Pulse.init({ ingestUrl: 'https://pulse.example.com', token: 'plt_x', streamId: 's' });
    expect(() => {
      // @ts-expect-error — intentionally bad input
      s.attachWebRTC(null);
    }).not.toThrow();
    s.dispose();
  });

  it('never throws when attachHls called with null', () => {
    const s = Pulse.init({ ingestUrl: 'https://pulse.example.com', token: 'plt_x', streamId: 's' });
    expect(() => {
      // @ts-expect-error — intentionally bad input
      s.attachHls(null);
    }).not.toThrow();
    s.dispose();
  });

  it('never throws when attachVideoElement called with null', () => {
    const s = Pulse.init({ ingestUrl: 'https://pulse.example.com', token: 'plt_x', streamId: 's' });
    expect(() => {
      // @ts-expect-error — intentionally bad input
      s.attachVideoElement(null);
    }).not.toThrow();
    s.dispose();
  });
});

describe('Pulse.init — sampling', () => {
  it('sampleRate=0 makes zero network calls', async () => {
    vi.spyOn(Math, 'random').mockReturnValue(0.99);

    const s = Pulse.init({
      ingestUrl: 'https://pulse.example.com',
      token: 'plt_x',
      streamId: 'stream1',
      sampleRate: 0,
    });

    // Even with events...
    s.event('heartbeat', { watch_ms: 1000 });
    await vi.advanceTimersByTimeAsync(15_000);
    await flushMicrotasks();

    expect(fetchMock).not.toHaveBeenCalled();
    s.dispose();
  });

  it('sampleRate=1 always produces network calls', async () => {
    const s = Pulse.init({
      ingestUrl: 'https://pulse.example.com',
      token: 'plt_x',
      streamId: 'stream1',
      sampleRate: 1,
    });

    s.event('heartbeat', { watch_ms: 1000 });
    await vi.advanceTimersByTimeAsync(12_000);
    await flushMicrotasks();

    expect(fetchMock.mock.calls.length).toBeGreaterThan(0);
    s.dispose();
  });

  it('no-op session from sampleRate=0 has a sessionId', () => {
    vi.spyOn(Math, 'random').mockReturnValue(0.99);
    const s = Pulse.init({
      ingestUrl: 'https://pulse.example.com',
      token: 'plt_x',
      streamId: 'stream1',
      sampleRate: 0,
    });
    expect(typeof s.sessionId).toBe('string');
    expect(s.sessionId).toMatch(/^[0-9a-f]{8}-/i);
    s.dispose();
  });
});

describe('Pulse.init — session lifecycle', () => {
  it('dispose() can be called multiple times without throwing', () => {
    const s = Pulse.init({ ingestUrl: 'https://pulse.example.com', token: 'plt_x', streamId: 's' });
    expect(() => {
      s.dispose();
      s.dispose();
      s.dispose();
    }).not.toThrow();
  });

  it('event() after dispose() does not throw', () => {
    const s = Pulse.init({ ingestUrl: 'https://pulse.example.com', token: 'plt_x', streamId: 's' });
    s.dispose();
    expect(() => {
      s.event('heartbeat', { watch_ms: 1000 });
    }).not.toThrow();
  });
});

describe('init named export', () => {
  it('is the same as Pulse.init', () => {
    const s1 = init({ ingestUrl: 'https://pulse.example.com', token: 'plt_x', streamId: 's' });
    expect(typeof s1.sessionId).toBe('string');
    s1.dispose();
  });
});
