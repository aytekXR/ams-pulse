import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom',
    globals: true,
    // fake timers are opted in per-test via vi.useFakeTimers()
    coverage: {
      provider: 'v8',
    },
  },
});
