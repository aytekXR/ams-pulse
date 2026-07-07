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
      exclude: ["src/test/mocks/**", "**/*.d.ts", "src/main.tsx"],
      // Thresholds are a RATCHET: set ~4-5 pts below the measured achieved values
      // to prevent regressions while keeping CI green.
      // Aspirational targets from PRD: 60% lines / 55% branches.
      // Measured achieved after msw tests: lines 61.72% / branches 75.35%.
      thresholds: {
        lines: 57,
        branches: 71,
      },
    },
  },
});
