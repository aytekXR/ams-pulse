/**
 * Focus-ring CSS contract (S33 / D-095).
 *
 * Three components opt into the shared :focus-visible ring by setting a bare
 * className — `tier-gate-cta`, `tabs-btn`, `filter-input`. Nothing in TypeScript
 * connects that string to the rule in global.css, so the two halves can drift
 * apart silently: S32 committed QoePage.tsx with className="filter-input" (and a
 * comment promising the ring) but never staged the global.css rule that provides
 * it. The unit tests passed anyway — they asserted the class was on the element,
 * which says nothing about whether a rule exists to match it.
 *
 * This test pins BOTH halves. A className with no rule behind it is a false
 * accessibility promise; a rule with no className using it is dead CSS.
 */
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, it, expect } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const css = readFileSync(resolve(here, "../global.css"), "utf-8");

/** class name → the component sources that opt into it (at least one must use it) */
const FOCUS_RING_CONTRACT: Record<string, string[]> = {
  "tier-gate-cta": ["../../components/TierGate.tsx"],
  "tabs-btn": ["../../components/Tabs.tsx"],
  "filter-input": [
    "../../features/qoe/QoePage.tsx",
    "../../features/analytics/DateRangePicker.tsx",
  ],
  // Wave 2
  "seg-btn": ["../../components/SegmentedControl.tsx"],
  "btn-secondary": [
    "../../features/analytics/AnalyticsPage.tsx",
    "../../features/fleet/FleetPage.tsx",
  ],
  "picker-btn": ["../../features/analytics/DateRangePicker.tsx"],
};

/**
 * Selectors that carry a real focus ring: `.foo:focus-visible` appearing in a
 * rule whose body actually sets `outline`. Parsed from the stylesheet rather
 * than hard-coded, so deleting the rule (or its `outline` declaration) is RED.
 */
function selectorsWithOutlineRing(stylesheet: string): Set<string> {
  const found = new Set<string>();
  for (const [, prelude, body] of stylesheet.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    if (!/(^|[\s;])outline\s*:/.test(body)) continue;
    if (/outline\s*:\s*none/.test(body)) continue;
    for (const [, cls] of prelude.matchAll(/\.([A-Za-z0-9_-]+):focus-visible/g)) {
      found.add(cls);
    }
  }
  return found;
}

describe("focus-ring CSS contract", () => {
  const ringed = selectorsWithOutlineRing(css);

  it.each(Object.keys(FOCUS_RING_CONTRACT))(
    "global.css defines a :focus-visible outline for .%s",
    (cls) => {
      expect(ringed).toContain(cls);
    },
  );

  it.each(Object.entries(FOCUS_RING_CONTRACT))(
    ".%s is actually used by a component (no dead focus-ring CSS)",
    (cls, componentPaths) => {
      const users = componentPaths.filter((p) =>
        readFileSync(resolve(here, p), "utf-8").includes(`className="${cls}"`),
      );
      expect(users, `no component sets className="${cls}"`).not.toHaveLength(0);
    },
  );
});
