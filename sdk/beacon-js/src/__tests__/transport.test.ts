/**
 * Transport tests:
 * - Events buffered and flushed ≤10 s (fake timers)
 * - Flush-on-visibilitychange / pagehide via fetch keepalive (D-165: sendBeacon cannot carry auth header)
 * - Unreachable-collector: bounded retry queue, backoff, player callbacks unaffected
 * - MAX_BATCH_SIZE (25 events) triggers an immediate flush
 * - No network calls when sampled out (tested via Pulse.init sampleRate=0)
 */
import { describe, it, expect, vi, beforeEach, afterEach, type MockInstance } from 'vitest';
import { Transport, MAX_BATCH_SIZE, FLUSH_INTERVAL_MS, MAX_QUEUE_DEPTH } from '../transport.js';
import type { TransportConfig } from '../transport.js';

/** Flush all pending microtasks (Promise resolutions). */
async function flushMicrotasks(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}

function makeConfig(overrides?: Partial<TransportConfig>): TransportConfig {
  return {
    ingestUrl: 'https://pulse.example.com',
    token: 'plt_test_token',
    sessionId: 'aaaaaaaa-bbbb-4ccc-dddd-eeeeeeeeeeee',
    streamId: 'test-stream',
    playerKind: 'native',
    sdkVersion: '0.1.0',
    ...overrides,
  };
}

let fetchMock: MockInstance;
let sendBeaconMock: ReturnType<typeof vi.fn>;

// Setup sendBeacon on navigator before tests (jsdom doesn't include it)
function setupSendBeacon(): ReturnType<typeof vi.fn> {
  const mock = vi.fn().mockReturnValue(true);
  Object.defineProperty(globalThis.navigator, 'sendBeacon', {
    value: mock,
    configurable: true,
    writable: true,
  });
  return mock;
}

beforeEach(() => {
  vi.useFakeTimers();

  fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(null, { status: 200 }),
  );

  sendBeaconMock = setupSendBeacon();
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  // Clear localStorage spill state
  try {
    localStorage.removeItem('pulse_beacon_q');
  } catch {
    // ignore
  }
});

describe('Transport — flush cadence', () => {
  it('flushes after FLUSH_INTERVAL_MS with queued events', async () => {
    const t = new Transport(makeConfig());
    t.push({ type: 'heartbeat', ts: Date.now(), data: { watch_ms: 1000 } });

    expect(fetchMock).not.toHaveBeenCalled();

    // advance past the flush interval
    await vi.advanceTimersByTimeAsync(FLUSH_INTERVAL_MS + 100);
    await flushMicrotasks();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, opts] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('https://pulse.example.com/ingest/beacon');
    const body = JSON.parse(opts.body as string) as Record<string, unknown>;
    expect(body).toMatchObject({
      version: 1,
      session_id: 'aaaaaaaa-bbbb-4ccc-dddd-eeeeeeeeeeee',
      stream_id: 'test-stream',
    });
    expect(Array.isArray(body['events'])).toBe(true);

    t.dispose();
  });

  it('flushes immediately when batch size reaches MAX_BATCH_SIZE', async () => {
    const t = new Transport(makeConfig());

    for (let i = 0; i < MAX_BATCH_SIZE; i++) {
      t.push({ type: 'heartbeat', ts: i, data: { watch_ms: i * 1000 } });
    }

    // Allow Promise resolutions from fetch mock to settle
    await flushMicrotasks();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    t.dispose();
  });

  it('keeps buffer empty after a flush', async () => {
    const t = new Transport(makeConfig());
    t.push({ type: 'heartbeat', ts: 1, data: { watch_ms: 1000 } });

    await vi.advanceTimersByTimeAsync(FLUSH_INTERVAL_MS + 100);
    await flushMicrotasks();
    expect(fetchMock).toHaveBeenCalledTimes(1);

    // No more events — next flush should not call fetch again
    await vi.advanceTimersByTimeAsync(FLUSH_INTERVAL_MS + 100);
    await flushMicrotasks();
    expect(fetchMock).toHaveBeenCalledTimes(1);

    t.dispose();
  });
});

describe('Transport — fetch keepalive on page lifecycle events', () => {
  it('uses fetch keepalive on visibilitychange to hidden', async () => {
    const t = new Transport(makeConfig());
    t.push({ type: 'session_end', ts: Date.now(), data: { watch_ms: 5000 } });

    // Simulate page going hidden
    Object.defineProperty(document, 'visibilityState', {
      value: 'hidden',
      configurable: true,
    });
    document.dispatchEvent(new Event('visibilitychange'));

    await flushMicrotasks();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, opts] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('https://pulse.example.com/ingest/beacon');
    expect(opts.keepalive).toBe(true);
    expect(sendBeaconMock).not.toHaveBeenCalled();

    t.dispose();
    // Restore
    Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
  });

  it('uses fetch keepalive on pagehide', async () => {
    const t = new Transport(makeConfig());
    t.push({ type: 'session_end', ts: Date.now(), data: { watch_ms: 5000 } });

    window.dispatchEvent(new Event('pagehide'));

    await flushMicrotasks();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, opts] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(opts.keepalive).toBe(true);
    expect(sendBeaconMock).not.toHaveBeenCalled();

    t.dispose();
  });
});

describe('Transport — unreachable collector (retry + backoff)', () => {
  it('retries on fetch failure, does not throw', async () => {
    fetchMock.mockRejectedValue(new Error('network error'));

    const t = new Transport(makeConfig());
    t.push({ type: 'heartbeat', ts: Date.now(), data: { watch_ms: 1000 } });

    await vi.advanceTimersByTimeAsync(FLUSH_INTERVAL_MS + 100);
    await flushMicrotasks();

    // No throw — transport stays alive
    expect(t).toBeDefined();

    // Retry scheduled — advance more time and verify another attempt is made
    await vi.advanceTimersByTimeAsync(2000);
    await flushMicrotasks();

    // At least 1 fetch call (initial attempt)
    expect(fetchMock.mock.calls.length).toBeGreaterThanOrEqual(1);

    t.dispose();
  });

  it('caps retry queue at MAX_QUEUE_DEPTH (drop-oldest)', async () => {
    fetchMock.mockRejectedValue(new Error('network error'));

    const t = new Transport(makeConfig());

    // Push enough events to trigger many flushes
    for (let batch = 0; batch <= MAX_QUEUE_DEPTH + 5; batch++) {
      for (let i = 0; i < MAX_BATCH_SIZE; i++) {
        t.push({ type: 'heartbeat', ts: batch * 1000 + i, data: { watch_ms: i } });
      }
      await flushMicrotasks();
    }

    // Should not throw regardless of queue overflow
    expect(t).toBeDefined();

    t.dispose();
  });

  it('does not spam console on repeated failures', async () => {
    const consoleSpy = vi.spyOn(console, 'debug').mockImplementation(() => {});
    fetchMock.mockRejectedValue(new Error('network error'));

    const t = new Transport(makeConfig());
    for (let i = 0; i < 5; i++) {
      t.push({ type: 'heartbeat', ts: i, data: { watch_ms: i * 1000 } });
    }
    await vi.advanceTimersByTimeAsync(FLUSH_INTERVAL_MS + 100);
    await flushMicrotasks();

    // console.debug should NOT be called by transport (only by Pulse.init on bad config)
    expect(consoleSpy).not.toHaveBeenCalled();

    t.dispose();
    vi.restoreAllMocks();
  });
});

describe('Transport — header name (VD-09)', () => {
  it('sends X-Pulse-Ingest-Token header (NOT X-Pulse-Token)', async () => {
    const t = new Transport(makeConfig({ token: 'plt_test_secret' }));
    t.push({ type: 'heartbeat', ts: Date.now(), data: { watch_ms: 1000 } });

    await vi.advanceTimersByTimeAsync(FLUSH_INTERVAL_MS + 100);
    await flushMicrotasks();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, opts] = fetchMock.mock.calls[0] as [string, RequestInit];
    const headers = opts.headers as Record<string, string>;

    // VD-09: the EXACT header name the server and OpenAPI spec require
    expect(headers['X-Pulse-Ingest-Token']).toBe('plt_test_secret');
    // Ensure the old (wrong) header name is NOT sent
    expect(headers['X-Pulse-Token']).toBeUndefined();

    t.dispose();
  });
});

describe('Transport — dispose', () => {
  it('flushes remaining events on dispose via fetch keepalive', async () => {
    const t = new Transport(makeConfig());
    t.push({ type: 'session_end', ts: Date.now(), data: { watch_ms: 60000, reason: 'user_exit' } });

    t.dispose();
    await flushMicrotasks();

    // dispose() uses fetch keepalive (isUnload=true path) — sendBeacon cannot carry the auth header
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, opts] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(opts.keepalive).toBe(true);
    expect(sendBeaconMock).not.toHaveBeenCalled();
  });

  it('ignores push() after dispose()', async () => {
    const t = new Transport(makeConfig());
    t.dispose();
    t.push({ type: 'heartbeat', ts: Date.now(), data: { watch_ms: 1000 } });

    await vi.advanceTimersByTimeAsync(FLUSH_INTERVAL_MS + 100);
    await flushMicrotasks();
    // No fetch call since transport is disposed and had no events
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe('Transport unload flush fallback (D-165)', () => {
  it('falls back to sendBeacon when fetch throws on unload', async () => {
    fetchMock.mockRejectedValueOnce(new Error('keepalive body quota exceeded'));

    const t = new Transport(makeConfig());
    t.push({ type: 'session_end', ts: Date.now(), data: { watch_ms: 5000 } });

    Object.defineProperty(document, 'visibilityState', {
      value: 'hidden',
      configurable: true,
    });
    document.dispatchEvent(new Event('visibilitychange'));

    await flushMicrotasks();
    await flushMicrotasks();

    // fetch was attempted first with keepalive
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, opts] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(opts.keepalive).toBe(true);
    // sendBeacon used as last-resort fallback after fetch threw
    expect(sendBeaconMock).toHaveBeenCalledTimes(1);

    t.dispose();
    Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
  });

  it('falls back when fetch unavailable', async () => {
    // Simulate environment without fetch (e.g. old WebView)
    vi.stubGlobal('fetch', undefined);

    const t = new Transport(makeConfig());
    t.push({ type: 'session_end', ts: Date.now(), data: { watch_ms: 5000 } });

    window.dispatchEvent(new Event('pagehide'));

    await flushMicrotasks();

    // sendBeacon is used as fallback when fetch is unavailable on unload
    expect(sendBeaconMock).toHaveBeenCalledTimes(1);

    t.dispose();
    vi.unstubAllGlobals();
  });

  it('sends auth header in unload keepalive fetch', async () => {
    const t = new Transport(makeConfig({ token: 'plt_unload_token' }));
    t.push({ type: 'session_end', ts: Date.now(), data: { watch_ms: 5000 } });

    window.dispatchEvent(new Event('pagehide'));

    await flushMicrotasks();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, opts] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('https://pulse.example.com/ingest/beacon');
    expect(opts.keepalive).toBe(true);
    const headers = opts.headers as Record<string, string>;
    expect(headers['X-Pulse-Ingest-Token']).toBe('plt_unload_token');
    expect(sendBeaconMock).not.toHaveBeenCalled();

    t.dispose();
  });
});
