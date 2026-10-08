/**
 * WCAG contrast guard over the BRANDKIT TOKENS themselves (G3/G5/G6, S33/D-095).
 *
 * Why this exists: the brandkit's own WCAG table in design-rationale.md §2 carried a WRONG
 * ratio for years — it claimed `textMuted` was ~4.6:1 (AA) when the true value is 3.72:1,
 * which FAILS AA for normal text. The table is binding on every design decision, so a wrong
 * number in it produced wrong decisions downstream (muted was used for labels and captions
 * everywhere). A hand-maintained table of ratios drifts from the hexes it describes.
 *
 * So: recompute the ratios FROM tokens.json, on every test run. If someone changes a token,
 * the arithmetic re-runs and an AA failure becomes a red test rather than a wrong table.
 *
 * The pairs below are the ones the product actually renders. Bars (WCAG 2.x):
 *   normal text    >= 4.5:1
 *   large text     >= 3:1   (>=18.66px bold or >=24px)
 *   non-text / UI  >= 3:1   (borders, graphics, status fills)
 */
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, it, expect } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const tokens = JSON.parse(
  readFileSync(resolve(here, "../../../../brandkit/design-system/tokens.json"), "utf-8"),
) as { color: { dark: Record<string, string>; light: Record<string, string> } };

/** WCAG 2.x relative luminance (sRGB, gamma-corrected). */
function luminance(hex: string): number {
  const h = hex.replace("#", "");
  const ch = [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16) / 255);
  const lin = ch.map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  return 0.2126 * lin[0] + 0.7152 * lin[1] + 0.0722 * lin[2];
}

function contrast(a: string, b: string): number {
  const [la, lb] = [luminance(a), luminance(b)];
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

/** Composite `fg` at `alpha` over `bg` — badge tints are alpha over the surface. */
function over(fg: string, bg: string, alpha: number): string {
  const px = (hex: string) =>
    [0, 2, 4].map((i) => parseInt(hex.replace("#", "").slice(i, i + 2), 16));
  const [f, b] = [px(fg), px(bg)];
  const mix = f.map((v, i) => Math.round(v * alpha + b[i] * (1 - alpha)));
  return `#${mix.map((v) => v.toString(16).padStart(2, "0")).join("")}`.toUpperCase();
}

const dark = tokens.color.dark;
const light = tokens.color.light;

describe("brandkit tokens — WCAG AA for text (>= 4.5:1)", () => {
  const textPairs: Array<[string, string, string]> = [
    ["dark  textPrimary   on bg", dark.textPrimary, dark.bg],
    ["dark  textSecondary on bg", dark.textSecondary, dark.bg],
    ["dark  textPrimary   on surface", dark.textPrimary, dark.surface],
    ["dark  textSecondary on surface", dark.textSecondary, dark.surface],
    ["light textPrimary   on bg", light.textPrimary, light.bg],
    ["light textSecondary on bg", light.textSecondary, light.bg],
    ["light textPrimary   on surface", light.textPrimary, light.surface],
    ["light textSecondary on surface", light.textSecondary, light.surface],
  ];

  it.each(textPairs)("%s passes AA", (_label, fg, bg) => {
    expect(contrast(fg, bg)).toBeGreaterThanOrEqual(4.5);
  });

  it("G3: the light CTA (onSignal on signal) passes AA — it was 3.12:1 before the token fix", () => {
    expect(contrast(light.onSignal, light.signal)).toBeGreaterThanOrEqual(4.5);
  });

  it("G3: the light CTA still passes AA on HOVER, and hover is DARKER than the resting state", () => {
    // The old hover (#099168) was 3.99:1 AND, against the corrected signal, would have been
    // lighter than the base — inverting the affordance. Both halves are pinned.
    expect(contrast(light.onSignal, light.signalHover)).toBeGreaterThanOrEqual(4.5);
    expect(luminance(light.signalHover)).toBeLessThan(luminance(light.signal));
  });

  it("dark CTA passes AA in both states", () => {
    expect(contrast(dark.onSignal, dark.signal)).toBeGreaterThanOrEqual(4.5);
    expect(contrast(dark.onSignal, dark.signalHover)).toBeGreaterThanOrEqual(4.5);
  });

  // S126: every Badge variant as global.css renders it — label on its tint, over both
  // surfaces a badge sits on. The label colour is read from global.css (--color-*-text),
  // so reverting the stylesheet turns this red: light success/warning/error used their
  // hues as labels, 2.74 / 4.26 / 4.14:1.
  const css = readFileSync(resolve(here, "../global.css"), "utf-8");
  const block = (selector: string) => {
    const start = css.indexOf(`${selector} {`);
    return css.slice(start, css.indexOf("\n}", start));
  };
  const cssVarToToken: Record<string, keyof typeof light> = {
    "--color-text": "textPrimary", "--color-success": "healthy",
    "--color-warning": "warning", "--color-error": "critical",
  };
  const labelToken = (theme: "dark" | "light", variant: string): keyof typeof light => {
    const re = new RegExp(`--color-${variant}-text:\\s*var\\((--color-[a-z-]+)\\)`);
    const m = re.exec(block(theme === "light" ? '[data-theme="light"]' : ":root")) ?? re.exec(block(":root"));
    if (!m || !cssVarToToken[m[1]]) throw new Error(`global.css: no --color-${variant}-text mapping for ${theme}`);
    return cssVarToToken[m[1]];
  };
  const badgeCases: Array<[string, string, string]> = [];
  for (const [theme, t] of [["dark", dark], ["light", light]] as const) {
    for (const [variant, hue, alpha] of [["success", t.healthy, 0.12], ["warning", t.warning, 0.12], ["error", t.critical, 0.1]] as const) {
      const label = t[labelToken(theme, variant)];
      for (const [surfaceName, surface] of [["surface", t.surface], ["raised", t.raised]] as const) {
        badgeCases.push([`${theme} ${variant} badge over ${surfaceName}`, label, over(hue, surface, alpha)]);
      }
    }
  }
  it.each(badgeCases)("%s passes AA", (_label, fg, bg) => {
    expect(contrast(fg, bg)).toBeGreaterThanOrEqual(4.5);
  });

  it("S126: the selected density segment (signalHover on a 15% signal tint) passes AA", () => {
    for (const t of [dark, light]) {
      for (const surface of [t.surface, t.raised]) {
        expect(contrast(t.signalHover, over(t.signal, surface, 0.15))).toBeGreaterThanOrEqual(4.5);
      }
    }
  });

  it("G6: the info Badge passes AA in BOTH themes (light was 2.32:1 before the token fix)", () => {
    // Badge renders `color: info` on `background: info @ 10% over the surface`.
    expect(contrast(light.info, over(light.info, light.surface, 0.1))).toBeGreaterThanOrEqual(4.5);
    expect(contrast(dark.info, over(dark.info, dark.surface, 0.1))).toBeGreaterThanOrEqual(4.5);
  });
});

describe("brandkit tokens — textMuted is NOT safe for text (the G5 finding, pinned)", () => {
  /**
   * This is the assertion the WCAG table should have made. It is written as an EXPECTED
   * FAILURE of the 4.5 bar, so that if someone ever "fixes" textMuted by darkening it, this
   * test goes red and forces the table + the usage guidance to be revisited together —
   * rather than the codebase quietly drifting back to using muted for labels.
   */
  it("textMuted fails the 4.5:1 normal-text bar in both themes", () => {
    expect(contrast(dark.textMuted, dark.surface)).toBeLessThan(4.5);
    expect(contrast(light.textMuted, light.surface)).toBeLessThan(4.5);
  });

  it("textMuted DOES clear the 3:1 non-text bar (borders/dividers remain a valid use)", () => {
    expect(contrast(dark.textMuted, dark.surface)).toBeGreaterThanOrEqual(3);
    expect(contrast(light.textMuted, light.surface)).toBeGreaterThanOrEqual(3);
  });
});

describe("brandkit tokens — status fills meet the 3:1 non-text bar", () => {
  const fills: Array<[string, string, string]> = [
    ["dark  healthy", dark.healthy, dark.surface],
    ["dark  warning", dark.warning, dark.surface],
    ["dark  critical", dark.critical, dark.surface],
    ["light healthy", light.healthy, light.surface],
    ["light warning", light.warning, light.surface],
    ["light critical", light.critical, light.surface],
  ];

  it.each(fills)("%s clears 3:1 as a graphic", (_label, fg, bg) => {
    expect(contrast(fg, bg)).toBeGreaterThanOrEqual(3);
  });
});
