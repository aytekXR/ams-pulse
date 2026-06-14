/**
 * Session tests:
 * - UUID generation always produces valid v4 UUID
 * - Sampling: sampleRate=0 always excluded, sampleRate=1 always included
 */
import { describe, it, expect, vi } from 'vitest';
import { generateSessionId, isSampled } from '../session.js';

const UUID_V4_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

describe('generateSessionId', () => {
  it('generates a valid v4 UUID', () => {
    const id = generateSessionId();
    expect(id).toMatch(UUID_V4_RE);
  });

  it('generates unique IDs each call', () => {
    const ids = new Set(Array.from({ length: 100 }, () => generateSessionId()));
    expect(ids.size).toBe(100);
  });

  it('falls back gracefully when crypto.randomUUID is unavailable', () => {
    const original = crypto.randomUUID;
    // @ts-expect-error — intentionally clobber
    crypto.randomUUID = undefined;
    const id = generateSessionId();
    expect(id).toMatch(UUID_V4_RE);
    crypto.randomUUID = original;
  });
});

describe('isSampled', () => {
  it('returns true when sampleRate is 1', () => {
    expect(isSampled(1)).toBe(true);
  });

  it('returns false when sampleRate is 0', () => {
    expect(isSampled(0)).toBe(false);
  });

  it('returns false when sampleRate > 1 (treated as 1)', () => {
    // sampleRate >= 1 always returns true
    expect(isSampled(2)).toBe(true);
  });

  it('returns false when sampleRate < 0 (treated as 0)', () => {
    expect(isSampled(-1)).toBe(false);
  });

  it('respects sampleRate=0.5 probabilistically', () => {
    // With Math.random mocked to 0.3, 0.3 < 0.5 => sampled
    vi.spyOn(Math, 'random').mockReturnValue(0.3);
    expect(isSampled(0.5)).toBe(true);
    vi.restoreAllMocks();
  });

  it('excludes session when Math.random >= sampleRate', () => {
    vi.spyOn(Math, 'random').mockReturnValue(0.9);
    expect(isSampled(0.5)).toBe(false);
    vi.restoreAllMocks();
  });
});
