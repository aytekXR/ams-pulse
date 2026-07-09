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
      // lines 65.55%, branches 76.87%, functions 73.77%
      // Floor at achieved − 3 so the gate is tight but not brittle.
      thresholds: {
        lines: 62,
        branches: 73,
        functions: 70,
      },
    },
  },
});
