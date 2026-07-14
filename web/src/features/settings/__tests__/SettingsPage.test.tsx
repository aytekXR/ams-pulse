/**
 * SettingsPage rendering tests.
 *
 * Covers:
 * (a) Smoke — mounts without crash; "Settings" heading present.
 * (b) Tab navigation: Sources, API Tokens, Ingest Tokens, Integrations, License, Users.
 *     SettingsPage uses a custom tab bar with role="tab" on each button (not the
 *     shared <Tabs> component, which lacks flexWrap support for 6 tabs).
 * (c) Sources tab — empty state message when no sources.
 * (d) API Tokens tab — empty state when no tokens.
 * (e) License tab — activate form renders.
 * (f) Users tab — placeholder message renders.
 * (g) Integrations tab — Prometheus and S3 sections render.
 * (h) Wave 4: tabpanel ARIA wiring — role, id, aria-labelledby.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { SettingsPage } from "../SettingsPage";
import type { LicenseInfo } from "@/lib/api/types";

const mockGetSources = vi.fn();
const mockGetTokens = vi.fn();
const mockGetLicense = vi.fn();

vi.mock("@/api/client", () => ({
  adminApi: {
    getSources: (...args: unknown[]) => mockGetSources(...args),
    getTokens: (...args: unknown[]) => mockGetTokens(...args),
    getLicense: (...args: unknown[]) => mockGetLicense(...args),
  },
  ApiError: class ApiError extends Error {
    status: number;
    body: unknown;
    constructor(status: number, body: { message?: string }) {
      super(body.message ?? `HTTP ${status}`);
      this.status = status;
      this.body = body;
      this.name = "ApiError";
    }
  },
}));

// Toast is used by SettingsPage; provide a minimal stub so it doesn't crash
vi.mock("@/components/Toast", () => ({
  useToast: () => ({ toast: vi.fn() }),
  ToastProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

const freeLicense: LicenseInfo = { tier: "free", valid: true };

function setupDefaultMocks() {
  mockGetSources.mockResolvedValue({ items: [] });
  mockGetTokens.mockResolvedValue({ items: [] });
  mockGetLicense.mockResolvedValue(freeLicense);
}

describe("SettingsPage rendering", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupDefaultMocks();
  });

  it("(a) smoke — mounts without crash and shows Settings heading", async () => {
    render(<SettingsPage />);
    expect(screen.getByRole("heading", { name: /settings/i })).toBeInTheDocument();
  });

  it("(b) all six tab buttons are present with role='tab'", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetSources).toHaveBeenCalled(); });
    // SettingsPage's custom tab bar emits role="tab" on each button
    expect(screen.getByRole("tab", { name: /^sources$/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /api tokens/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /ingest tokens/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /integrations/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /license/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /users/i })).toBeInTheDocument();
  });

  it("(b) tab container has role='tablist'", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetSources).toHaveBeenCalled(); });
    const tablist = screen.getByRole("tablist");
    expect(tablist).toBeInTheDocument();
  });

  it("(b) sources tab is aria-selected='true' by default", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetSources).toHaveBeenCalled(); });
    const sourcesTab = screen.getByRole("tab", { name: /^sources$/i });
    expect(sourcesTab).toHaveAttribute("aria-selected", "true");
  });

  it("(c) Sources tab — empty state message is shown when no sources configured", async () => {
    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByText(/no ams sources configured/i)).toBeInTheDocument();
    });
  });

  it("(c) Sources tab — Add source button is present", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(screen.queryByText(/loading/i)).not.toBeInTheDocument(); });
    expect(screen.getByRole("button", { name: /\+ add source/i })).toBeInTheDocument();
  });

  it("(d) Tokens tab — empty state shown when navigating to tokens tab", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetTokens).toHaveBeenCalled(); });
    fireEvent.click(screen.getByRole("tab", { name: /api tokens/i }));
    await waitFor(() => {
      expect(screen.getByText(/no api tokens/i)).toBeInTheDocument();
    });
  });

  it("(e) License tab — activate form with key input is shown", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetLicense).toHaveBeenCalled(); });
    fireEvent.click(screen.getByRole("tab", { name: /license/i }));
    await waitFor(() => {
      expect(screen.getByPlaceholderText(/PULSE-XXXX/i)).toBeInTheDocument();
    });
  });

  it("(f) Users tab — shows placeholder message about user management", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetSources).toHaveBeenCalled(); });
    fireEvent.click(screen.getByRole("tab", { name: /users/i }));
    await waitFor(() => {
      expect(screen.getByText(/user management/i)).toBeInTheDocument();
    });
  });

  it("(g) Integrations tab — Prometheus section is shown", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetSources).toHaveBeenCalled(); });
    fireEvent.click(screen.getByRole("tab", { name: /integrations/i }));
    await waitFor(() => {
      // Multiple "Prometheus Metrics" headings may appear; check at least one is present
      const prometheusEls = screen.getAllByText(/prometheus metrics/i);
      expect(prometheusEls.length).toBeGreaterThanOrEqual(1);
    });
  });

  it("(g) Integrations tab — S3 Export section is shown", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetSources).toHaveBeenCalled(); });
    fireEvent.click(screen.getByRole("tab", { name: /integrations/i }));
    await waitFor(() => {
      expect(screen.getByText(/s3 export destination/i)).toBeInTheDocument();
    });
  });

  it("shows error banner when data fetch fails", async () => {
    mockGetSources.mockRejectedValue(new Error("network error"));
    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
  });
});

// ── (h) Wave 4: tabpanel ARIA wiring ────────────────────────────────────────

describe("SettingsPage — tabpanel ARIA wiring (Wave 4)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupDefaultMocks();
  });

  it("sources tab panel has role='tabpanel' after load", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetSources).toHaveBeenCalled(); });
    const panel = screen.getByRole("tabpanel");
    expect(panel).toBeInTheDocument();
  });

  it("sources tabpanel id and aria-labelledby are correctly wired", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetSources).toHaveBeenCalled(); });
    const panel = screen.getByRole("tabpanel");
    expect(panel.id).toBe("settings-panel-sources");
    expect(panel.getAttribute("aria-labelledby")).toBe("tab-sources");
    // The element with that id must actually exist in the DOM
    const tabButton = document.getElementById("tab-sources");
    expect(tabButton).toBeInTheDocument();
  });

  it("tokens tabpanel has correct id and aria-labelledby after tab switch", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetTokens).toHaveBeenCalled(); });
    fireEvent.click(screen.getByRole("tab", { name: /api tokens/i }));
    await waitFor(() => {
      const panel = screen.getByRole("tabpanel");
      expect(panel.id).toBe("settings-panel-tokens");
      expect(panel.getAttribute("aria-labelledby")).toBe("tab-tokens");
      expect(document.getElementById("tab-tokens")).toBeInTheDocument();
    });
  });

  it("integrations tabpanel has correct wiring", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetSources).toHaveBeenCalled(); });
    fireEvent.click(screen.getByRole("tab", { name: /integrations/i }));
    await waitFor(() => {
      const panel = screen.getByRole("tabpanel");
      expect(panel.id).toBe("settings-panel-integrations");
      expect(panel.getAttribute("aria-labelledby")).toBe("tab-integrations");
      expect(document.getElementById("tab-integrations")).toBeInTheDocument();
    });
  });
});
