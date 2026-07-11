/**
 * LoadingSpinner tests (B2 sweep — prefers-reduced-motion support).
 *
 * When the user's OS preference is "prefers-reduced-motion: reduce", the spin
 * animation must be disabled (animation: none). This avoids vestibular-disorder
 * discomfort and satisfies WCAG 2.1 SC 2.3.3.
 *
 * Implementation strategy: the component reads window.matchMedia on mount via
 * useEffect and disables the animation if the media query matches. Tests mock
 * window.matchMedia to control the result.
 */
import { describe, it, expect, vi, afterEach } from "vitest";
import { render } from "@testing-library/react";
import { LoadingSpinner } from "../LoadingSpinner";

// Helpers to configure the matchMedia mock
function mockMatchMedia(matches: boolean) {
  const mql = {
    matches,
    media: "(prefers-reduced-motion: reduce)",
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  };
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: vi.fn().mockReturnValue(mql),
  });
  return mql;
}

describe("LoadingSpinner — prefers-reduced-motion (B2 sweep)", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("spins when prefers-reduced-motion is NOT set", () => {
    mockMatchMedia(false);
    const { container } = render(<LoadingSpinner />);
    const svg = container.querySelector("svg") as SVGElement;
    // Animation string should contain "pulse-spin"
    expect(svg.style.animation).toContain("pulse-spin");
  });

  it("stops animation when prefers-reduced-motion: reduce is set", () => {
    mockMatchMedia(true);
    const { container } = render(<LoadingSpinner />);
    const svg = container.querySelector("svg") as SVGElement;
    // Animation must be disabled — "none" or empty
    expect(svg.style.animation).toBe("none");
  });

  it("renders with role=status for screen readers", () => {
    mockMatchMedia(false);
    const { getByRole } = render(<LoadingSpinner />);
    expect(getByRole("status")).toBeInTheDocument();
  });

  it("renders custom label", () => {
    mockMatchMedia(false);
    const { getByText } = render(<LoadingSpinner label="Please wait" />);
    expect(getByText("Please wait")).toBeInTheDocument();
  });
});
