// @vitest-environment node
/**
 * Coverage threshold guard (WO-4).
 *
 * Asserts that vite.config.ts:
 *  1. Contains a `thresholds` object under test.coverage.
 *  2. Each gate meets the required minimum (set after the 2026-07-08 smoke-test
 *     wave: lines 79.48%, branches 75.57%, functions 46.57%).
 *  3. The coverage.exclude array equals the exact expected set — prevents gaming
 *     the numbers by silently widening the exclusion list.
 *
 * NEVER lower these values.  To raise them, update the literals here and in
 * vite.config.ts together, under review.
 */
import { describe, it, expect } from "vitest";

// Vite config files export the result of defineConfig() as their default export.
// We import it as a plain object (vitest resolves vite.config.ts via the same
// tsconfig paths so the @/ alias is NOT available here — use a relative path).
import config from "../../vite.config";

type CoverageThresholds = {
  lines?: number;
  branches?: number;
  functions?: number;
  statements?: number;
};

// Type-narrow the coverage config from the opaque Vite UserConfig type.
const coverage = (config as { test?: { coverage?: { thresholds?: CoverageThresholds; exclude?: string[] } } })
  .test?.coverage;

describe("Coverage gate guard", () => {
  it("coverage config block exists", () => {
    expect(coverage).toBeDefined();
  });

  it("thresholds object exists inside coverage config", () => {
    expect(coverage?.thresholds).toBeDefined();
  });

  it("lines threshold >= 76", () => {
    expect(coverage?.thresholds?.lines).toBeGreaterThanOrEqual(76);
  });

  it("branches threshold >= 72", () => {
    expect(coverage?.thresholds?.branches).toBeGreaterThanOrEqual(72);
  });

  it("functions threshold >= 45", () => {
    expect(coverage?.thresholds?.functions).toBeGreaterThanOrEqual(45);
  });

  it("coverage exclude list equals the exact expected set (no silent widening)", () => {
    const excludes = coverage?.exclude ?? [];
    const expected = [
      "src/test/mocks/**",
      "**/*.d.ts",
      "src/main.tsx",
      "src/lib/api/types.ts",
    ];
    // Sort both arrays so order differences don't matter, but membership must match exactly.
    expect([...excludes].sort()).toEqual([...expected].sort());
  });
});
