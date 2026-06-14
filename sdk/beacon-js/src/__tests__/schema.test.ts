/**
 * Schema round-trip tests.
 * Every event type must validate against beacon-event.schema.json.
 * Malformed data and bad config must not throw.
 */
import { describe, it, expect, beforeAll } from 'vitest';
import Ajv2020 from 'ajv/dist/2020';
import type { BeaconBatch } from '../types.js';
// Load schema and fixtures from contracts/ (frozen D-004 — never modify)
import schema from '../../../../contracts/events/beacon-event.schema.json';
import validFixture1 from '../../../../contracts/events/fixtures/beacon-event-valid-1.json';
import validFixture2 from '../../../../contracts/events/fixtures/beacon-event-valid-2.json';
import invalidFixture1 from '../../../../contracts/events/fixtures/beacon-event-invalid-1.json';

let validate: ReturnType<Ajv2020['compile']>;

beforeAll(() => {
  const ajv = new Ajv2020({ strict: false });
  validate = ajv.compile(schema);
});

// Helper to build a minimal valid batch
function batch(events: BeaconBatch['events']): BeaconBatch {
  return {
    version: 1,
    session_id: 'aaaaaaaa-bbbb-4ccc-dddd-eeeeeeeeeeee',
    stream_id: 'test-stream',
    events,
  };
}

describe('beacon-event schema — contract fixtures', () => {
  it('valid-1 fixture validates', () => {
    expect(validate(validFixture1)).toBe(true);
  });

  it('valid-2 fixture validates', () => {
    expect(validate(validFixture2)).toBe(true);
  });

  it('invalid-1 fixture fails validation (empty events array)', () => {
    expect(validate(invalidFixture1)).toBe(false);
    const errors = validate.errors ?? [];
    expect(errors.length).toBeGreaterThan(0);
  });
});

describe('beacon-event schema — per event type round-trip', () => {
  it('session_start validates', () => {
    const b = batch([
      { type: 'session_start', ts: Date.now(), data: { page_url: 'https://example.com', autoplay: true } },
    ]);
    expect(validate(b)).toBe(true);
  });

  it('startup_complete validates (with required startup_ms)', () => {
    const b = batch([
      { type: 'startup_complete', ts: Date.now(), data: { startup_ms: 1200, bitrate_kbps: 2000 } },
    ]);
    expect(validate(b)).toBe(true);
  });

  it('heartbeat validates (with required watch_ms)', () => {
    const b = batch([
      { type: 'heartbeat', ts: Date.now(), data: { watch_ms: 30000, bitrate_kbps: 2000, buffer_ms: 8000, dropped_frames: 0 } },
    ]);
    expect(validate(b)).toBe(true);
  });

  it('rebuffer_start validates', () => {
    const b = batch([
      { type: 'rebuffer_start', ts: Date.now(), data: { buffer_ms: 0 } },
    ]);
    expect(validate(b)).toBe(true);
  });

  it('rebuffer_end validates (with required duration_ms)', () => {
    const b = batch([
      { type: 'rebuffer_end', ts: Date.now(), data: { duration_ms: 3500 } },
    ]);
    expect(validate(b)).toBe(true);
  });

  it('error validates (with required code)', () => {
    const b = batch([
      { type: 'error', ts: Date.now(), data: { code: 'MEDIA_ERR_NETWORK', message: 'Network error', fatal: false } },
    ]);
    expect(validate(b)).toBe(true);
  });

  it('bitrate_change validates (with required from_kbps/to_kbps)', () => {
    const b = batch([
      { type: 'bitrate_change', ts: Date.now(), data: { from_kbps: 2000, to_kbps: 1000 } },
    ]);
    expect(validate(b)).toBe(true);
  });

  it('resolution_change validates (with required from/to)', () => {
    const b = batch([
      { type: 'resolution_change', ts: Date.now(), data: { from: '1280x720', to: '1920x1080' } },
    ]);
    expect(validate(b)).toBe(true);
  });

  it('session_end validates', () => {
    const b = batch([
      { type: 'session_end', ts: Date.now(), data: { watch_ms: 120000, reason: 'user_exit' } },
    ]);
    expect(validate(b)).toBe(true);
  });

  it('multiple event types in one batch validates', () => {
    const b = batch([
      { type: 'session_start', ts: 1000 },
      { type: 'startup_complete', ts: 1500, data: { startup_ms: 500 } },
      { type: 'heartbeat', ts: 31000, data: { watch_ms: 30000 } },
      { type: 'session_end', ts: 120000, data: { watch_ms: 119000, reason: 'page_unload' } },
    ]);
    expect(validate(b)).toBe(true);
  });
});

describe('beacon-event schema — rejection cases', () => {
  it('rejects missing session_id', () => {
    const b = { version: 1, stream_id: 'x', events: [{ type: 'session_start', ts: 1 }] };
    expect(validate(b)).toBe(false);
  });

  it('rejects missing stream_id', () => {
    const b = { version: 1, session_id: 'aaaaaaaa-bbbb-4ccc-dddd-eeeeeeeeeeee', events: [{ type: 'session_start', ts: 1 }] };
    expect(validate(b)).toBe(false);
  });

  it('rejects unknown event type', () => {
    const b = batch([{ type: 'unknown_type' as never, ts: 1 }]);
    expect(validate(b)).toBe(false);
  });

  it('rejects empty events array', () => {
    const b = { version: 1, session_id: 'aaaaaaaa-bbbb-4ccc-dddd-eeeeeeeeeeee', stream_id: 'x', events: [] };
    expect(validate(b)).toBe(false);
  });

  it('rejects startup_complete missing required startup_ms', () => {
    const b = batch([{ type: 'startup_complete', ts: 1, data: { bitrate_kbps: 1000 } }]);
    // startup_ms is required for startup_complete
    expect(validate(b)).toBe(false);
  });

  it('rejects heartbeat missing required watch_ms', () => {
    const b = batch([{ type: 'heartbeat', ts: 1, data: { bitrate_kbps: 1000 } }]);
    expect(validate(b)).toBe(false);
  });

  it('rejects rebuffer_end missing required duration_ms', () => {
    const b = batch([{ type: 'rebuffer_end', ts: 1, data: {} }]);
    expect(validate(b)).toBe(false);
  });

  it('rejects error missing required code', () => {
    const b = batch([{ type: 'error', ts: 1, data: { message: 'oops' } }]);
    expect(validate(b)).toBe(false);
  });

  it('rejects bitrate_change missing required from_kbps', () => {
    const b = batch([{ type: 'bitrate_change', ts: 1, data: { to_kbps: 1000 } }]);
    expect(validate(b)).toBe(false);
  });

  it('rejects resolution_change missing required from', () => {
    const b = batch([{ type: 'resolution_change', ts: 1, data: { to: '1920x1080' } }]);
    expect(validate(b)).toBe(false);
  });
});
