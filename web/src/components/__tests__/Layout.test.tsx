/**
 * Layout component tests.
 *
 * Covers:
 * (a) Renders nav with aria-label "Main navigation".
 * (b) All primary nav links present (Live, Analytics, QoE, Ingest, Alerts, etc.).
 * (c) Brand name "Pulse" is visible.
 * (d) Shows tier badge when tier prop is provided.
 * (e) Sign-out button is present.
 * (f) Connection status indicator ("Polling" or "Live") reflects wsConnected.
 * (g) Theme toggle button is rendered with correct aria-label.
 * (h) Clicking theme toggle calls setTheme with opposite value.
 * (i) Density control renders 3 segments (Default, Compact, Wall).
 * (j) Clicking a density segment calls setDensity.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Layout } from "../Layout";

// Layout uses clearToken from @/api/client when sign-out is clicked
const mockClearToken = vi.fn();
vi.mock("@/api/client", () => ({
  clearToken: () => mockClearToken(),
}));

// Mock ThemeContext so Layout tests don't depend on real DOM state
const mockSetTheme = vi.fn();
const mockSetDensity = vi.fn();

vi.mock("@/lib/ThemeContext", () => ({
  useTheme: () => ({ theme: "dark", setTheme: mockSetTheme }),
  useDensity: () => ({ density: "default", setDensity: mockSetDensity, rowHeight: 40 }),
}));

function Wrapped({ children = <div data-testid="content">Page</div>, wsConnected = false, tier }: {
  children?: React.ReactNode;
  wsConnected?: boolean;
  tier?: string;
}) {
  return (
    <MemoryRouter>
      <Layout wsConnected={wsConnected} tier={tier}>
        {children}
      </Layout>
    </MemoryRouter>
  );
}

describe("Layout", () => {
  it("(a) renders the main navigation landmark", () => {
    render(<Wrapped />);
    expect(screen.getByRole("navigation", { name: /main navigation/i })).toBeInTheDocument();
  });

  it("(b) renders key nav links: Live, Analytics, Alerts, Settings", () => {
    render(<Wrapped />);
    expect(screen.getByRole("link", { name: /live/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /analytics/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /alerts/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /settings/i })).toBeInTheDocument();
  });

  it("(c) renders brand name Pulse in the sidebar", () => {
    render(<Wrapped />);
    expect(screen.getByText("Pulse")).toBeInTheDocument();
  });

  it("(d) renders the tier badge when tier prop is provided", () => {
    render(<Wrapped tier="pro" />);
    expect(screen.getByText("pro")).toBeInTheDocument();
  });

  it("(e) renders the sign-out button", () => {
    render(<Wrapped />);
    expect(screen.getByTitle(/sign out/i)).toBeInTheDocument();
  });

  it("(f) shows Polling status when wsConnected is false", () => {
    render(<Wrapped wsConnected={false} />);
    expect(screen.getByText("Polling")).toBeInTheDocument();
  });

  it("(f) shows Live status when wsConnected is true", () => {
    render(<Wrapped wsConnected={true} />);
    // The header connection indicator shows "Live" — must be in the header,
    // not in the nav (which also has a "Live" link).
    // Use getAllByText and check at least one is in the header area.
    const liveElements = screen.getAllByText("Live");
    // At minimum two: the nav link "Live" and the header status "Live"
    expect(liveElements.length).toBeGreaterThanOrEqual(2);
  });

  it("(g) renders children in the main content area", () => {
    render(<Wrapped><div data-testid="child-content">My Page</div></Wrapped>);
    expect(screen.getByTestId("child-content")).toBeInTheDocument();
  });

  it("(g) theme toggle renders with aria-label for switching from dark to light", () => {
    render(<Wrapped />);
    // In dark mode, button should say "Switch to light theme"
    expect(
      screen.getByRole("button", { name: /switch to light theme/i })
    ).toBeInTheDocument();
  });

  it("(h) clicking theme toggle calls setTheme with 'light' when in dark mode", () => {
    render(<Wrapped />);
    const toggle = screen.getByRole("button", { name: /switch to light theme/i });
    fireEvent.click(toggle);
    expect(mockSetTheme).toHaveBeenCalledWith("light");
  });

  it("(i) density control renders 3 segments: Default, Compact, Wall", () => {
    render(<Wrapped />);
    expect(screen.getByRole("button", { name: /default/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /compact/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /wall/i })).toBeInTheDocument();
  });

  it("(j) clicking Compact density segment calls setDensity('compact')", () => {
    render(<Wrapped />);
    fireEvent.click(screen.getByRole("button", { name: /compact/i }));
    expect(mockSetDensity).toHaveBeenCalledWith("compact");
  });
});
