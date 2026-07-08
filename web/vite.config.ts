import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Dev server proxies API + WebSocket to a locally running `pulse serve`.
// In production the built assets are served by the pulse binary itself
// (internal/api), so there is no separate web container to operate.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { "@": "/src" },
  },
  server: {
    proxy: {
      "/api": "http://localhost:8090",
      "/live": {
        target: "http://localhost:8090",
        ws: true,
      },
    },
  },
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    css: false,
    // Playwright specs live in e2e/ and must never run under vitest —
    // vitest's default include pattern would sweep *.spec.ts and fail on
    // @playwright/test's test() (D-055 CI red). node_modules is excluded
    // by default; restate it since `exclude` replaces the defaults.
    exclude: ["node_modules/**", "e2e/**"],
    // Set jsdom base URL so that relative fetch('/api/v1/...')
    // resolves to http://localhost/api/v1/... which msw can intercept.
    environmentOptions: {
      jsdom: {
        url: "http://localhost",
      },
    },
    coverage: {
      provider: "v8",
      reporter: ["text", "json-summary"],
      // enabled:true means `vitest run` always collects coverage — no CI change needed.
      enabled: true,
      include: ["src/**"],
      exclude: [
        "src/test/mocks/**",
        "**/*.d.ts",
        "src/main.tsx",
        // types.ts contains ONLY type re-exports (no runtime statements); instrumenting
        // it is noise and keeps functions/lines at 0 despite 100% type coverage.
        "src/lib/api/types.ts",
      ],
      // Thresholds are a RATCHET: set floor(achieved - 3) below the measured values
      // to prevent regressions while keeping CI green. Never lower an existing gate.
      // Measured achieved after WO-4 smoke tests (2026-07-08):
      //   lines 79.48% → gate 76 | branches 75.57% → gate 72
      //   functions 46.57% → gate max(45, floor(46.57-3)) = 45 (new)
      thresholds: {
        lines: 76,
        branches: 72,
        functions: 45,
      },
    },
  },
});
