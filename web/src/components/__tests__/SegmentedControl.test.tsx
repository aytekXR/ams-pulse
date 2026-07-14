/**
 * SegmentedControl — shared view/mode switch (Wave 2).
 *
 * Pins:
 * - SG-1: radiogroup/radio semantics with an accessible name — and explicitly NOT
 *         a tablist (announcing tabs with no tabpanel is a false promise).
 * - SG-2: aria-checked tracks the selected value.
 * - SG-3: roving tabIndex — the checked radio is the group's only tab stop.
 * - SG-4: keyboard nav (Arrow Right/Down/Left/Up with wrap, Home, End) selects.
 * - SG-5: click selects.
 * - SG-6: inactive label uses --color-secondary, not the AA-failing --color-muted.
 * - SG-7: labels render verbatim (no textTransform capitalize → no DOM/name drift).
 *
 * Every test renders the component. None asserts an expression computed in this file.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { SegmentedControl } from "../SegmentedControl";

const ITEMS = [
  { value: "cards", label: "Cards" },
  { value: "table", label: "Table" },
  { value: "grid", label: "Grid" },
];

function setup(value = "cards") {
  const onChange = vi.fn();
  render(
    <SegmentedControl aria-label="View" items={ITEMS} value={value} onChange={onChange} />,
  );
  return { onChange };
}

describe("SegmentedControl", () => {
  it("SG-1: is a named radiogroup, not a tablist", () => {
    setup();
    expect(screen.getByRole("radiogroup", { name: "View" })).toBeInTheDocument();
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
    expect(screen.queryByRole("tab")).not.toBeInTheDocument();
    expect(screen.getAllByRole("radio")).toHaveLength(3);
  });

  it("SG-2: aria-checked marks only the selected item", () => {
    setup("table");
    expect(screen.getByRole("radio", { name: "Table" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("radio", { name: "Cards" })).toHaveAttribute("aria-checked", "false");
    expect(screen.getByRole("radio", { name: "Grid" })).toHaveAttribute("aria-checked", "false");
  });

  it("SG-3: roving tabIndex — the checked radio is the only tab stop", () => {
    setup("table");
    expect(screen.getByRole("radio", { name: "Table" })).toHaveAttribute("tabindex", "0");
    expect(screen.getByRole("radio", { name: "Cards" })).toHaveAttribute("tabindex", "-1");
    expect(screen.getByRole("radio", { name: "Grid" })).toHaveAttribute("tabindex", "-1");
  });

  it("SG-4: ArrowRight selects the next item", () => {
    const { onChange } = setup("cards");
    fireEvent.keyDown(screen.getByRole("radio", { name: "Cards" }), { key: "ArrowRight" });
    expect(onChange).toHaveBeenCalledWith("table");
  });

  it("SG-4: ArrowDown behaves as ArrowRight (APG radio group)", () => {
    const { onChange } = setup("cards");
    fireEvent.keyDown(screen.getByRole("radio", { name: "Cards" }), { key: "ArrowDown" });
    expect(onChange).toHaveBeenCalledWith("table");
  });

  it("SG-4: ArrowLeft wraps from the first item to the last", () => {
    const { onChange } = setup("cards");
    fireEvent.keyDown(screen.getByRole("radio", { name: "Cards" }), { key: "ArrowLeft" });
    expect(onChange).toHaveBeenCalledWith("grid");
  });

  it("SG-4: ArrowRight wraps from the last item to the first", () => {
    const { onChange } = setup("grid");
    fireEvent.keyDown(screen.getByRole("radio", { name: "Grid" }), { key: "ArrowRight" });
    expect(onChange).toHaveBeenCalledWith("cards");
  });

  it("SG-4: Home selects the first, End selects the last", () => {
    const { onChange } = setup("table");
    fireEvent.keyDown(screen.getByRole("radio", { name: "Table" }), { key: "Home" });
    expect(onChange).toHaveBeenCalledWith("cards");
    fireEvent.keyDown(screen.getByRole("radio", { name: "Table" }), { key: "End" });
    expect(onChange).toHaveBeenCalledWith("grid");
  });

  it("SG-4: an unhandled key does not select anything", () => {
    const { onChange } = setup("cards");
    fireEvent.keyDown(screen.getByRole("radio", { name: "Cards" }), { key: "a" });
    expect(onChange).not.toHaveBeenCalled();
  });

  it("SG-5: clicking an item selects it", () => {
    const { onChange } = setup("cards");
    fireEvent.click(screen.getByRole("radio", { name: "Table" }));
    expect(onChange).toHaveBeenCalledWith("table");
  });

  it("SG-6: the inactive label uses --color-secondary (--color-muted fails AA at 11px)", () => {
    setup("cards");
    const inactive = screen.getByRole("radio", { name: "Table" });
    expect(inactive.style.color).toBe("var(--color-secondary)");
    expect(inactive.style.color).not.toBe("var(--color-muted)");
    // The active one is full-strength text.
    expect(screen.getByRole("radio", { name: "Cards" }).style.color).toBe("var(--color-text)");
  });

  it("SG-7: labels render verbatim — no CSS capitalize, so DOM text == accessible name", () => {
    setup();
    const table = screen.getByRole("radio", { name: "Table" });
    expect(table).toHaveTextContent("Table");
    expect(table.style.textTransform).toBe("");
  });

  it("SG-6: carries the seg-btn class that global.css hangs the focus ring on", () => {
    setup();
    for (const label of ["Cards", "Table", "Grid"]) {
      expect(screen.getByRole("radio", { name: label })).toHaveClass("seg-btn");
    }
  });
});
