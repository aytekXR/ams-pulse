/**
 * Guards that the SDK_VERSION injected into every beacon batch matches
 * package.json. Build failures here mean tsup.config.ts or vitest.config.ts
 * no longer read the version from package.json.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';

// Injected at build/test time via tsup.config.ts / vitest.config.ts define.
declare const SDK_VERSION: string;

describe('SDK_VERSION injection', () => {
  it('equals package.json version', () => {
    const pkg = JSON.parse(readFileSync('package.json', 'utf8')) as { version: string };
    expect(SDK_VERSION).toBe(pkg.version);
  });

  it('is a non-empty semver-shaped string', () => {
    expect(typeof SDK_VERSION).toBe('string');
    expect(SDK_VERSION.length).toBeGreaterThan(0);
    expect(SDK_VERSION).toMatch(/^\d+\.\d+\.\d+/);
  });
});
