/**
 * StatCard tests (B2 sweep — density-aware CSS vars).
 *
 * Pins:
 *  - padding must be "var(--card-padding)" — density-aware (global.css: default=24px,
 *    compact=16px, wall=32px). The old hardcoded "16px 20px" only fit the compact density.
 *  - metric fontSize must be "var(--metric-size)" — density-aware (default=40px,
 *    compact=32px, wall=64px). The old hardcoded 28 (→ "28px") was off for all densities.
 *
 * jsdom does NOT resolve CSS custom properties for standard CSS properties, so we
 * assert on getAttribute("style") which reflects what React serialised to the DOM.
 */
import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { StatCard } from "../StatCard";

describe("StatCard — CSS var pins (B2 sweep)", () => {
  it("container padding uses var(--card-padding) not a hardcoded value", () => {
    const { container } = render(
      <StatCard label="Viewers" value={42} />,
    );
    // The outermost div carries the padding style.
    const card = container.firstElementChild as HTMLElement;
    // React sets inline styles via element.style.*  In jsdom, CSS custom-property
    // references in shorthand properties may not be preserved. Check cssText instead.
    const styleText = card.getAttribute("style") ?? card.style.cssText;
    expect(styleText).toContain("var(--card-padding)");
  });

  it("metric value fontSize uses var(--metric-size) not a hardcoded pixel value", () => {
    const { container } = render(
      <StatCard label="Bitrate" value="2.5 Mbps" />,
    );
    const card = container.firstElementChild as HTMLElement;
    // The metric <span> is the second child (label is first).
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
