// @vitest-environment node
/**
 * Coverage threshold guard (WO-4).
 *
 * Asserts that vite.config.ts:
 *  1. Contains a `thresholds` object under test.coverage.
 *  2. Each gate meets the required minimum (re-baselined for vitest 4 / rolldown
 *     on 2026-07-09: rolldown instruments differently from vitest-3's esbuild engine,
 *     producing lower but equally valid numbers; old gates lines 76 / branches 72
 *     replaced by lines 59 / branches 54 per floor(achieved - 3) methodology).
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

  it("lines threshold >= 59", () => {
    expect(coverage?.thresholds?.lines).toBeGreaterThanOrEqual(59);
  });

  it("branches threshold >= 54", () => {
    expect(coverage?.thresholds?.branches).toBeGreaterThanOrEqual(54);
  });

  it("functions threshold >= 45", () => {
    expect(coverage?.thresholds?.functions).toBeGreaterThanOrEqual(45);
  });

  it("coverage exclude list equals the exact expected set (no silent widening)", () => {
    const excludes = coverage?.exclude ?? [];
    const expected = [
      "src/test/mocks/**",
      "**/*.d.ts",
      "**/*.md",
      "src/main.tsx",
      "src/lib/api/types.ts",
    ];
    // Sort both arrays so order differences don't matter, but membership must match exactly.
    expect([...excludes].sort()).toEqual([...expected].sort());
  });
});
