/**
 * Session UUID generation for @pulse/beacon.
 * Uses crypto.randomUUID() (available in all modern browsers and Node 14.17+).
 * Falls back to a Math.random-based v4 UUID if unavailable.
 */

/** Generate a v4 UUID string. Never throws. */
export function generateSessionId(): string {
  try {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      return crypto.randomUUID();
    }
  } catch {
    // fall through to fallback
  }
  // Fallback: RFC 4122 §4.4 compliant UUID v4 via Math.random
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    const v = c === 'x' ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

/**
 * Decide once per session whether this session is sampled.
 * @param sampleRate - 0..1 (1 = always report, 0 = never report)
 * @returns true if the session should be reported
 */
export function isSampled(sampleRate: number): boolean {
  if (sampleRate >= 1) return true;
  if (sampleRate <= 0) return false;
  return Math.random() < sampleRate;
}
