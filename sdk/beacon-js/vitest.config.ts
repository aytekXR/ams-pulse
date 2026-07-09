import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom',
    globals: true,
    // fake timers are opted in per-test via vi.useFakeTimers()
    coverage: {
      provider: 'v8',
      enabled: true,
      reporter: ['text', 'json-summary'],
      include: ['src/**'],
      // types.ts is a TypeScript-types-only file: all interfaces/type aliases compile
      // to nothing at runtime, so v8 correctly reports 0% and it would skew the gate.
      exclude: ['src/**/*.test.ts', 'src/types.ts'],
      // Measured baseline (types.ts excluded — type-only file, compiles to nothing):
      // vitest 4 / v8 re-baseline (vitest 4 counts implicit-else and optional-chain
      // branches, so branch % drops materially vs vitest 3):
      // lines 66.06%, branches 45.79%, functions 70.42%
      // Floor at achieved − 3 so the gate is tight but not brittle.
      thresholds: {
        lines: 63,
        branches: 43,
        functions: 67,
      },
    },
  },
});
