/**
 * ComingSoon component smoke tests.
 *
 * Covers:
 * (a) Renders the feature name as a heading.
 * (b) Renders the "Coming in <wave>" message.
 * (c) Defaults to "Wave 2" when wave prop is omitted.
 */
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ComingSoon } from "../ComingSoon";

describe("ComingSoon", () => {
  it("(a) renders the feature name as a heading", () => {
    render(<ComingSoon feature="Fleet Management" />);
    expect(screen.getByRole("heading", { name: /fleet management/i })).toBeInTheDocument();
  });

  it("(b) renders the wave label in the body text", () => {
    render(<ComingSoon feature="Probes" wave="Wave 3" />);
    expect(screen.getByText(/coming in wave 3/i)).toBeInTheDocument();
  });

  it("(c) defaults to Wave 2 when wave prop is omitted", () => {
    render(<ComingSoon feature="Analytics" />);
    expect(screen.getByText(/coming in wave 2/i)).toBeInTheDocument();
  });

  it("(d) renders without crashing with any feature string", () => {
    expect(() => render(<ComingSoon feature="Some Future Feature" wave="Wave 4" />)).not.toThrow();
  });
});
