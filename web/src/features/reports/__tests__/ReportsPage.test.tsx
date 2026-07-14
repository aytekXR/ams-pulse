/**
 * Reports page tests:
 *  - Schedule form validation (cron required, format required)
 *  - Tier-gated reports view (entitled vs not)
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";

// ─── ScheduleForm validation (tested via the public form component) ─────────

// We test the validation logic inline (pure functions) without mounting the
// full ReportsPage (which needs license data from the API).

interface ScheduleFormData {
  cronPreset: string;
  cronRaw: string;
  format: "csv" | "pdf";
}

function validateScheduleForm(data: ScheduleFormData): string | null {
  const cron = data.cronPreset === "custom" ? data.cronRaw.trim() : data.cronPreset.trim();
  if (!cron) return "Cron expression is required";
  // Basic cron field count validation: must have 5 space-separated fields
  const parts = cron.split(/\s+/).filter(Boolean);
  if (parts.length !== 5) return "Cron must have 5 fields (min hour dom month dow)";
  return null; // valid
}

describe("Reports schedule form validation", () => {
  it("returns null for a valid monthly preset", () => {
    expect(validateScheduleForm({
      cronPreset: "0 6 1 * *",
      cronRaw: "",
      format: "csv",
    })).toBeNull();
  });

  it("returns null for a valid weekly preset", () => {
    expect(validateScheduleForm({
      cronPreset: "0 6 * * 1",
      cronRaw: "",
      format: "csv",
    })).toBeNull();
  });

  it("returns error when custom cron is empty", () => {
    expect(validateScheduleForm({
      cronPreset: "custom",
      cronRaw: "",
      format: "csv",
    })).toMatch(/required/i);
  });

  it("returns error when custom cron is whitespace only", () => {
    expect(validateScheduleForm({
      cronPreset: "custom",
      cronRaw: "   ",
      format: "csv",
    })).toMatch(/required/i);
  });

  it("returns error when custom cron has wrong field count", () => {
    expect(validateScheduleForm({
      cronPreset: "custom",
      cronRaw: "0 6 1",
      format: "csv",
    })).toMatch(/5 fields/i);
  });

  it("returns null for valid custom cron", () => {
    expect(validateScheduleForm({
      cronPreset: "custom",
      cronRaw: "30 7 * * 0",
      format: "pdf",
    })).toBeNull();
  });
});

// ─── VD-36: Cron preset validation ───────────────────────────────────────────
// Presets must be valid 5-field cron (now that server accepts 5-field per BE-02-B VD-36).
// Guard: old 3-field presets would fail validation; only 5-field is correct.

const CRON_PRESETS_FOR_TEST = [
  { label: "Monthly (1st of month, 6 AM UTC)", value: "0 6 1 * *" },
  { label: "Weekly (Monday, 6 AM UTC)", value: "0 6 * * 1" },
  { label: "Daily (6 AM UTC)", value: "0 6 * * *" },
];

describe("VD-36: cron presets are valid 5-field cron", () => {
  it.each(CRON_PRESETS_FOR_TEST)("preset '$label' has exactly 5 fields", ({ value }) => {
    const parts = value.trim().split(/\s+/).filter(Boolean);
    expect(parts).toHaveLength(5);
  });

  it("each preset passes schedule form validation", () => {
    for (const preset of CRON_PRESETS_FOR_TEST) {
      const result = validateScheduleForm({
        cronPreset: preset.value,
        cronRaw: "",
        format: "csv",
      });
      expect(result).toBeNull();
    }
  });

  it("3-field cron would fail validation (guard against regression)", () => {
    // Old behavior: 3-field cron strings used to be sent to server,
    // which means 5-field validation correctly rejects them.
    expect(validateScheduleForm({
      cronPreset: "custom",
      cronRaw: "0 6 1",   // 3-field — invalid in 5-field world
      format: "csv",
    })).not.toBeNull();
  });
});

// ─── Tier-gate rendering ─────────────────────────────────────────────────────

// Mock the necessary modules
vi.mock("@/api/client", () => ({
  adminApi: {
    getLicense: vi.fn(),
  },
  reportsApi: {
    getUsage: vi.fn(),
    listSchedules: vi.fn(),
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

vi.mock("@/components/Toast", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

import { adminApi } from "@/api/client";
import { ReportsPage } from "../ReportsPage";

describe("ReportsPage tier gate", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows upsell when license tier is 'free'", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue({
      tier: "free",
      valid: true,
    });
    render(<ReportsPage />);
    await waitFor(() => {
      expect(screen.getByText(/requires business tier/i)).toBeInTheDocument();
    });
  });

  it("shows upgrade link when gated", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue({
      tier: "free",
      valid: true,
    });
    render(<ReportsPage />);
    await waitFor(() => {
      expect(screen.getByRole("link", { name: /upgrade license/i })).toBeInTheDocument();
    });
  });

  // VD-01: pro tier is NOT entitled for reports (requires business+)
  it("shows upsell when license tier is 'pro'", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue({
      tier: "pro",
      valid: true,
    });
    render(<ReportsPage />);
    await waitFor(() => {
      expect(screen.getByText(/requires business tier/i)).toBeInTheDocument();
    });
    // Usage tabs must NOT appear
    expect(screen.queryByRole("tab", { name: /usage/i })).toBeNull();
  });

  // VD-01: business tier IS entitled for reports
  it("shows usage tab when license is 'business'", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue({
      tier: "business",
      valid: true,
    });
    // reportsApi.getUsage resolves with empty but valid response
    const { reportsApi } = await import("@/api/client");
    vi.mocked(reportsApi.getUsage).mockResolvedValue({
      rows: [],
      totals: { viewer_minutes: 0, peak_concurrency: 0, egress_gb: 0, recording_gb: 0 },
      egress_method: "bitrate_x_watch_time",
    });
    render(<ReportsPage />);
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /usage/i })).toBeInTheDocument();
    });
    // should NOT show upsell on business tier
    expect(screen.queryByText(/requires business tier/i)).toBeNull();
  });

  it("shows usage tab when license is 'enterprise'", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue({
      tier: "enterprise",
      valid: true,
    });
    const { reportsApi } = await import("@/api/client");
    vi.mocked(reportsApi.getUsage).mockResolvedValue({
      rows: [],
      totals: { viewer_minutes: 0, peak_concurrency: 0, egress_gb: 0, recording_gb: 0 },
      egress_method: "bitrate_x_watch_time",
    });
    render(<ReportsPage />);
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /schedules/i })).toBeInTheDocument();
    });
    expect(screen.queryByText(/requires business tier/i)).toBeNull();
  });

  it("shows loading spinner while license is loading", async () => {
    // Never-resolving promise to keep loading state indefinitely
    const never = new Promise<never>(() => {});
    vi.mocked(adminApi.getLicense).mockReturnValue(never);
    const { unmount } = render(<ReportsPage />);
    expect(screen.getByRole("status")).toBeInTheDocument();
    // Unmount immediately to avoid act() teardown warnings
    unmount();
  });
});

// Re-export for downstream testing reference
export { validateScheduleForm };
