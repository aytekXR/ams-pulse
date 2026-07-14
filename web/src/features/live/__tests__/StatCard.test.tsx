/**
 * StatCard tests.
 *
 * Wave 0 (B2 sweep) pins:
 *  - padding must be "var(--card-padding)" — density-aware.
 *  - metric fontSize must be "var(--metric-size)" — density-aware.
 *
 * Wave 1 (uipro) additions:
 *  - SC-1: outer div has role="group" with a composed aria-label.
 *  - SC-2: value span carries data-metric attribute (activates tabular-nums
 *          via global.css [data-metric] selector).
 *  - SC-4: borderRadius uses var(--radius-control), not a hardcoded 8.
 *  - SC-5: gap uses var(--space-1), not a hardcoded 4.
 *
 * jsdom does NOT resolve CSS custom properties, so we assert on the serialised
 * style string from getAttribute("style").
 */
import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { StatCard } from "../StatCard";

// ── Wave 0 pins (unchanged) ──────────────────────────────────────────────────

describe("StatCard — CSS var pins (B2 sweep)", () => {
  it("container padding uses var(--card-padding) not a hardcoded value", () => {
    const { container } = render(
      <StatCard label="Viewers" value={42} />,
    );
    const card = container.firstElementChild as HTMLElement;
    const styleText = card.getAttribute("style") ?? card.style.cssText;
    expect(styleText).toContain("var(--card-padding)");
  });

  it("metric value fontSize uses var(--metric-size) not a hardcoded pixel value", () => {
    const { container } = render(
      <StatCard label="Bitrate" value="2.5 Mbps" />,
    );
    const card = container.firstElementChild as HTMLElement;
    const metricSpan = card.querySelectorAll("span")[1] as HTMLElement;
    const styleText = metricSpan.getAttribute("style") ?? metricSpan.style.cssText;
    expect(styleText).toContain("var(--metric-size)");
  });

  it("renders label text", () => {
    const { getByText } = render(<StatCard label="CPU" value={55} />);
    expect(getByText("CPU")).toBeInTheDocument();
  });

  it("renders numeric value with locale formatting", () => {
    const { getByText } = render(<StatCard label="Streams" value={1234} />);
    expect(getByText("1,234")).toBeInTheDocument();
  });

  it("renders string value as-is", () => {
    const { getByText } = render(<StatCard label="Version" value="v2.9.1" />);
    expect(getByText("v2.9.1")).toBeInTheDocument();
  });

  it("renders sub label when provided", () => {
    const { getByText } = render(<StatCard label="CPU" value={55} sub="avg" />);
    expect(getByText("avg")).toBeInTheDocument();
  });

  it("does not render sub label when absent", () => {
    const { queryByText } = render(<StatCard label="CPU" value={55} />);
    expect(queryByText("avg")).not.toBeInTheDocument();
  });
});

// ── Wave 1: SC-1 — role="group" + aria-label ─────────────────────────────────

describe("StatCard — accessible group (SC-1)", () => {
  it("outer container has role='group'", () => {
    const { container } = render(<StatCard label="Viewers" value={42} />);
    const card = container.firstElementChild as HTMLElement;
    expect(card.getAttribute("role")).toBe("group");
  });

  it("aria-label includes the label text", () => {
    const { container } = render(<StatCard label="Viewers" value={42} />);
    const card = container.firstElementChild as HTMLElement;
    const ariaLabel = card.getAttribute("aria-label") ?? "";
    expect(ariaLabel).toContain("Viewers");
  });

  it("aria-label includes the formatted numeric value", () => {
    const { container } = render(<StatCard label="Viewers" value={1234} />);
    const card = container.firstElementChild as HTMLElement;
    const ariaLabel = card.getAttribute("aria-label") ?? "";
    // 1234 renders as "1,234" via toLocaleString
    expect(ariaLabel).toContain("1,234");
  });

  it("aria-label includes the string value", () => {
    const { container } = render(<StatCard label="CPU" value="42%" />);
    const card = container.firstElementChild as HTMLElement;
    const ariaLabel = card.getAttribute("aria-label") ?? "";
    expect(ariaLabel).toContain("42%");
  });

  it("aria-label includes sub when provided", () => {
    const { container } = render(<StatCard label="Viewers" value={42} sub="concurrent" />);
    const card = container.firstElementChild as HTMLElement;
    const ariaLabel = card.getAttribute("aria-label") ?? "";
    expect(ariaLabel).toContain("concurrent");
  });

  it("aria-label does not include undefined/null when sub is absent", () => {
    const { container } = render(<StatCard label="Viewers" value={42} />);
    const card = container.firstElementChild as HTMLElement;
    const ariaLabel = card.getAttribute("aria-label") ?? "";
    // Non-vacuous guard: fails if aria-label is removed from the component entirely.
    expect(ariaLabel).not.toBe("");
    expect(ariaLabel).not.toContain("undefined");
    expect(ariaLabel).not.toContain("null");
  });
});

// ── Wave 1: SC-2 — data-metric attribute ─────────────────────────────────────

describe("StatCard — tabular numerics (SC-2)", () => {
  it("value span has data-metric attribute (activates tabular-nums in global.css)", () => {
    const { container } = render(<StatCard label="Viewers" value={42} />);
    const card = container.firstElementChild as HTMLElement;
    // The metric value span is the second child span (first = label, second = value).
    const metricSpan = card.querySelectorAll("span")[1] as HTMLElement;
    expect(metricSpan.hasAttribute("data-metric")).toBe(true);
  });

  it("string value span also has data-metric attribute", () => {
    const { container } = render(<StatCard label="CPU" value="42%" />);
    const card = container.firstElementChild as HTMLElement;
    const metricSpan = card.querySelectorAll("span")[1] as HTMLElement;
    expect(metricSpan.hasAttribute("data-metric")).toBe(true);
  });
});

// ── Wave 1: SC-4 — var(--radius-control) ─────────────────────────────────────

describe("StatCard — border-radius token (SC-4)", () => {
  it("borderRadius uses var(--radius-control), not a hardcoded integer", () => {
    const { container } = render(<StatCard label="Viewers" value={42} />);
    const card = container.firstElementChild as HTMLElement;
    const styleText = card.getAttribute("style") ?? card.style.cssText;
    expect(styleText).toContain("var(--radius-control)");
    // Guard: raw integer 8 must NOT appear (would mean the token wasn't applied).
    // We check for "8px" or ": 8;" patterns that would indicate a hardcoded value.
    // Note: "8" alone is too broad (could match in other values), so check for the
    // specific "border-radius: 8" pattern.
    expect(styleText).not.toMatch(/border-radius:\s*8[^a-z]/);
  });
});

// ── Wave 1: SC-5 — var(--space-1) ────────────────────────────────────────────

describe("StatCard — gap token (SC-5)", () => {
  it("gap uses var(--space-1), not a hardcoded integer", () => {
    const { container } = render(<StatCard label="Viewers" value={42} />);
    const card = container.firstElementChild as HTMLElement;
    const styleText = card.getAttribute("style") ?? card.style.cssText;
    expect(styleText).toContain("var(--space-1)");
    // Guard: raw gap: 4 must NOT appear as a standalone numeric value.
    expect(styleText).not.toMatch(/gap:\s*4[^a-z]/);
  });
});
