/**
 * Guards the SDK version injection chain (REVIEW-MP3 N7).
 *
 * The __SDK_VERSION__ define is injected by tsup.config.ts (build) and
 * vitest.config.ts (tests) from package.json. This test validates the test-time
 * injection matches package.json; the BUILT artifact is separately guarded by a
 * CI grep of dist/index.js for the version literal, because this test cannot
 * see the tsup output (it runs under vitest's own define).
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';

// Injected at build/test time via tsup.config.ts / vitest.config.ts define.
declare const __SDK_VERSION__: string | undefined;

describe('__SDK_VERSION__ injection', () => {
  it('equals package.json version', () => {
    const pkg = JSON.parse(readFileSync('package.json', 'utf8')) as { version: string };
    expect(__SDK_VERSION__).toBe(pkg.version);
  });

  it('is a non-empty semver-shaped string', () => {
    expect(typeof __SDK_VERSION__).toBe('string');
    expect(__SDK_VERSION__!.length).toBeGreaterThan(0);
    expect(__SDK_VERSION__).toMatch(/^\d+\.\d+\.\d+/);
  });
});
