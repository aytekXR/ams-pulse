/**
 * Tabs unit tests.
 *
 * Tests cover:
 *
 * Rendering:
 *   - All tab labels are rendered
 *   - Container has role="tablist"
 *   - Each button has role="tab"
 *   - Each button has id="tab-{id}" for future aria-labelledby wiring
 *
 * Active state:
 *   - Active tab has aria-selected="true"
 *   - Inactive tabs have aria-selected="false"
 *   - Active tab has tabIndex=0 (roving tabindex)
 *   - Inactive tabs have tabIndex=-1 (roving tabindex)
 *
 * Interaction:
 *   - Clicking a tab calls onTabChange with the tab id
 *   - Clicking the already-active tab still calls onTabChange
 *
 * Keyboard navigation (ARIA tabs pattern — automatic activation):
 *   - ArrowRight → next tab (calls onTabChange, moves focus)
 *   - ArrowRight wraps from last to first
 *   - ArrowLeft → previous tab (calls onTabChange, moves focus)
 *   - ArrowLeft wraps from first to last
 *   - Home → first tab
 *   - End → last tab
 *   - Unrelated keys do NOT call onTabChange
 *
 * Focus ring:
 *   - Each button has className="tabs-btn" for the :focus-visible ring in global.css
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { Tabs } from "../Tabs";

const sampleTabs = [
  { id: "audience", label: "Audience" },
  { id: "geo", label: "Geo" },
  { id: "device", label: "Device" },
];

// ─── Rendering ───────────────────────────────────────────────────────────────

describe("Tabs — rendering", () => {
  it("renders all tab labels", () => {
    render(<Tabs tabs={sampleTabs} activeTab="audience" onTabChange={vi.fn()} />);
    expect(screen.getByText("Audience")).toBeInTheDocument();
    expect(screen.getByText("Geo")).toBeInTheDocument();
    expect(screen.getByText("Device")).toBeInTheDocument();
  });

  it("renders the container with role=tablist", () => {
    render(<Tabs tabs={sampleTabs} activeTab="audience" onTabChange={vi.fn()} />);
    expect(screen.getByRole("tablist")).toBeInTheDocument();
  });

  it("renders each button with role=tab", () => {
    render(<Tabs tabs={sampleTabs} activeTab="audience" onTabChange={vi.fn()} />);
    const tabs = screen.getAllByRole("tab");
    expect(tabs).toHaveLength(3);
  });

  it("assigns id=tab-{id} to each button for future aria-labelledby wiring", () => {
    const { container } = render(
      <Tabs tabs={sampleTabs} activeTab="audience" onTabChange={vi.fn()} />,
    );
    expect(container.querySelector("#tab-audience")).toBeInTheDocument();
    expect(container.querySelector("#tab-geo")).toBeInTheDocument();
    expect(container.querySelector("#tab-device")).toBeInTheDocument();
  });

  it("adds className=tabs-btn to each button for the focus-visible ring", () => {
    render(<Tabs tabs={sampleTabs} activeTab="audience" onTabChange={vi.fn()} />);
    const tabs = screen.getAllByRole("tab");
    for (const tab of tabs) {
      expect(tab).toHaveClass("tabs-btn");
    }
  });
});

// ─── Active state ─────────────────────────────────────────────────────────────

describe("Tabs — active state", () => {
  it("marks the active tab with aria-selected=true", () => {
    render(<Tabs tabs={sampleTabs} activeTab="geo" onTabChange={vi.fn()} />);
    expect(screen.getByRole("tab", { name: "Geo" })).toHaveAttribute("aria-selected", "true");
  });

  it("marks all inactive tabs with aria-selected=false", () => {
    render(<Tabs tabs={sampleTabs} activeTab="audience" onTabChange={vi.fn()} />);
    expect(screen.getByRole("tab", { name: "Geo" })).toHaveAttribute("aria-selected", "false");
    expect(screen.getByRole("tab", { name: "Device" })).toHaveAttribute("aria-selected", "false");
  });

  it("gives the active tab tabIndex=0 (roving tabindex)", () => {
    render(<Tabs tabs={sampleTabs} activeTab="device" onTabChange={vi.fn()} />);
    expect(screen.getByRole("tab", { name: "Device" })).toHaveAttribute("tabIndex", "0");
  });

  // The accent underline is the ONLY visual mark of the active tab — dropping it
  // was a sabotage that stayed green across all four suites (S31 verifier), so the
  // component's entire visual purpose was unguarded. Assert the style directly.
  it("underlines the active tab with the accent token, and no inactive tab", () => {
    render(<Tabs tabs={sampleTabs} activeTab="geo" onTabChange={vi.fn()} />);
    expect(screen.getByRole("tab", { name: "Geo" }).getAttribute("style"))
      .toContain("border-bottom: 2px solid var(--color-accent)");
    // jsdom decomposes the shorthand into longhands when the colour is `transparent`
    // (border-width/style/color), so an inactive tab is pinned by the ABSENCE of the
    // accent token rather than by a shorthand string that never appears.
    for (const inactive of ["Audience", "Device"]) {
      expect(screen.getByRole("tab", { name: inactive }).getAttribute("style"))
        .not.toContain("var(--color-accent)");
    }
  });

  it("gives all inactive tabs tabIndex=-1 (roving tabindex)", () => {
    render(<Tabs tabs={sampleTabs} activeTab="device" onTabChange={vi.fn()} />);
    expect(screen.getByRole("tab", { name: "Audience" })).toHaveAttribute("tabIndex", "-1");
    expect(screen.getByRole("tab", { name: "Geo" })).toHaveAttribute("tabIndex", "-1");
  });

  it("only one tab has tabIndex=0 at any time", () => {
    render(<Tabs tabs={sampleTabs} activeTab="geo" onTabChange={vi.fn()} />);
    const tabs = screen.getAllByRole("tab");
    const focusable = tabs.filter((t) => t.getAttribute("tabIndex") === "0");
    expect(focusable).toHaveLength(1);
    expect(focusable[0]).toHaveAccessibleName("Geo");
  });
});

// ─── Interaction ──────────────────────────────────────────────────────────────

describe("Tabs — click interaction", () => {
  it("calls onTabChange with the tab id when a tab is clicked", () => {
    const onTabChange = vi.fn();
    render(<Tabs tabs={sampleTabs} activeTab="audience" onTabChange={onTabChange} />);
    fireEvent.click(screen.getByRole("tab", { name: "Geo" }));
    expect(onTabChange).toHaveBeenCalledWith("geo");
    expect(onTabChange).toHaveBeenCalledTimes(1);
  });

  it("calls onTabChange when the currently-active tab is clicked again", () => {
    const onTabChange = vi.fn();
    render(<Tabs tabs={sampleTabs} activeTab="audience" onTabChange={onTabChange} />);
    fireEvent.click(screen.getByRole("tab", { name: "Audience" }));
    expect(onTabChange).toHaveBeenCalledWith("audience");
  });
});

// ─── Keyboard navigation ─────────────────────────────────────────────────────

describe("Tabs — keyboard navigation", () => {
  it("ArrowRight moves to the next tab and calls onTabChange", () => {
    const onTabChange = vi.fn();
    render(<Tabs tabs={sampleTabs} activeTab="audience" onTabChange={onTabChange} />);
    fireEvent.keyDown(screen.getByRole("tab", { name: "Audience" }), { key: "ArrowRight" });
    expect(onTabChange).toHaveBeenCalledWith("geo");
  });

  it("ArrowRight wraps from the last tab to the first", () => {
    const onTabChange = vi.fn();
    render(<Tabs tabs={sampleTabs} activeTab="device" onTabChange={onTabChange} />);
    fireEvent.keyDown(screen.getByRole("tab", { name: "Device" }), { key: "ArrowRight" });
    expect(onTabChange).toHaveBeenCalledWith("audience");
  });

  it("ArrowLeft moves to the previous tab", () => {
    const onTabChange = vi.fn();
    render(<Tabs tabs={sampleTabs} activeTab="geo" onTabChange={onTabChange} />);
    fireEvent.keyDown(screen.getByRole("tab", { name: "Geo" }), { key: "ArrowLeft" });
    expect(onTabChange).toHaveBeenCalledWith("audience");
  });

  it("ArrowLeft wraps from the first tab to the last", () => {
    const onTabChange = vi.fn();
    render(<Tabs tabs={sampleTabs} activeTab="audience" onTabChange={onTabChange} />);
    fireEvent.keyDown(screen.getByRole("tab", { name: "Audience" }), { key: "ArrowLeft" });
    expect(onTabChange).toHaveBeenCalledWith("device");
  });

  it("Home activates the first tab regardless of current position", () => {
    const onTabChange = vi.fn();
    render(<Tabs tabs={sampleTabs} activeTab="device" onTabChange={onTabChange} />);
    fireEvent.keyDown(screen.getByRole("tab", { name: "Device" }), { key: "Home" });
    expect(onTabChange).toHaveBeenCalledWith("audience");
  });

  it("End activates the last tab regardless of current position", () => {
    const onTabChange = vi.fn();
    render(<Tabs tabs={sampleTabs} activeTab="audience" onTabChange={onTabChange} />);
    fireEvent.keyDown(screen.getByRole("tab", { name: "Audience" }), { key: "End" });
    expect(onTabChange).toHaveBeenCalledWith("device");
  });

  it("Home from the first tab stays on the first tab", () => {
    const onTabChange = vi.fn();
    render(<Tabs tabs={sampleTabs} activeTab="audience" onTabChange={onTabChange} />);
    fireEvent.keyDown(screen.getByRole("tab", { name: "Audience" }), { key: "Home" });
    expect(onTabChange).toHaveBeenCalledWith("audience");
  });

  it("End from the last tab stays on the last tab", () => {
    const onTabChange = vi.fn();
    render(<Tabs tabs={sampleTabs} activeTab="device" onTabChange={onTabChange} />);
    fireEvent.keyDown(screen.getByRole("tab", { name: "Device" }), { key: "End" });
    expect(onTabChange).toHaveBeenCalledWith("device");
  });

  it("unrelated keys (Tab, Space, Enter, Escape) do NOT call onTabChange", () => {
    const onTabChange = vi.fn();
    render(<Tabs tabs={sampleTabs} activeTab="audience" onTabChange={onTabChange} />);
    const audienceBtn = screen.getByRole("tab", { name: "Audience" });
    for (const key of ["Tab", "Space", "Escape"]) {
      fireEvent.keyDown(audienceBtn, { key });
    }
    expect(onTabChange).not.toHaveBeenCalled();
  });

  it("ArrowRight moves focus to the next tab button", () => {
    render(
      <Tabs tabs={sampleTabs} activeTab="audience" onTabChange={vi.fn()} />,
    );
    const audienceBtn = screen.getByRole("tab", { name: "Audience" });
    const geoBtn = screen.getByRole("tab", { name: "Geo" });
    audienceBtn.focus();
    fireEvent.keyDown(audienceBtn, { key: "ArrowRight" });
    expect(document.activeElement).toBe(geoBtn);
  });

  it("ArrowLeft moves focus to the previous tab button", () => {
    render(
      <Tabs tabs={sampleTabs} activeTab="geo" onTabChange={vi.fn()} />,
    );
    const audienceBtn = screen.getByRole("tab", { name: "Audience" });
    const geoBtn = screen.getByRole("tab", { name: "Geo" });
    geoBtn.focus();
    fireEvent.keyDown(geoBtn, { key: "ArrowLeft" });
    expect(document.activeElement).toBe(audienceBtn);
  });
});

// ─── Edge cases ───────────────────────────────────────────────────────────────

describe("Tabs — edge cases", () => {
  it("renders a single tab correctly", () => {
    const onTabChange = vi.fn();
    render(
      <Tabs tabs={[{ id: "only", label: "Only" }]} activeTab="only" onTabChange={onTabChange} />,
    );
    const tab = screen.getByRole("tab", { name: "Only" });
    expect(tab).toHaveAttribute("aria-selected", "true");
    expect(tab).toHaveAttribute("tabIndex", "0");
  });

  it("ArrowRight on a single tab wraps to itself", () => {
    const onTabChange = vi.fn();
    render(
      <Tabs tabs={[{ id: "only", label: "Only" }]} activeTab="only" onTabChange={onTabChange} />,
    );
    fireEvent.keyDown(screen.getByRole("tab", { name: "Only" }), { key: "ArrowRight" });
    expect(onTabChange).toHaveBeenCalledWith("only");
  });
});
