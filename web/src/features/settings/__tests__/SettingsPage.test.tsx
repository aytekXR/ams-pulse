/**
 * SettingsPage rendering tests.
 *
 * Covers:
 * (a) Smoke — mounts without crash; "Settings" heading present.
 * (b) Tab navigation: Sources, API Tokens, Ingest Tokens, Integrations, License, Users.
 * (c) Sources tab — empty state message when no sources.
 * (d) API Tokens tab — empty state when no tokens.
 * (e) License tab — activate form renders.
 * (f) Users tab — placeholder message renders.
 * (g) Integrations tab — Prometheus and S3 sections render.
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

  it("(b) all six tab buttons are present", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetSources).toHaveBeenCalled(); });
    expect(screen.getByRole("button", { name: /^sources$/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /api tokens/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /ingest tokens/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /integrations/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /license/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /users/i })).toBeInTheDocument();
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
    fireEvent.click(screen.getByRole("button", { name: /api tokens/i }));
    await waitFor(() => {
      expect(screen.getByText(/no api tokens/i)).toBeInTheDocument();
    });
  });

  it("(e) License tab — activate form with key input is shown", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetLicense).toHaveBeenCalled(); });
    fireEvent.click(screen.getByRole("button", { name: /license/i }));
    await waitFor(() => {
      expect(screen.getByPlaceholderText(/PULSE-XXXX/i)).toBeInTheDocument();
    });
  });

  it("(f) Users tab — shows placeholder message about user management", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetSources).toHaveBeenCalled(); });
    fireEvent.click(screen.getByRole("button", { name: /users/i }));
    await waitFor(() => {
      expect(screen.getByText(/user management/i)).toBeInTheDocument();
    });
  });

  it("(g) Integrations tab — Prometheus section is shown", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetSources).toHaveBeenCalled(); });
    fireEvent.click(screen.getByRole("button", { name: /integrations/i }));
    await waitFor(() => {
      // Multiple "Prometheus Metrics" headings may appear; check at least one is present
      const prometheusEls = screen.getAllByText(/prometheus metrics/i);
      expect(prometheusEls.length).toBeGreaterThanOrEqual(1);
    });
  });

  it("(g) Integrations tab — S3 Export section is shown", async () => {
    render(<SettingsPage />);
    await waitFor(() => { expect(mockGetSources).toHaveBeenCalled(); });
    fireEvent.click(screen.getByRole("button", { name: /integrations/i }));
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
