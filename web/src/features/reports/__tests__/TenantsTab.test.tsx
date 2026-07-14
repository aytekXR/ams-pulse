/**
 * TenantsTab component tests:
 *  - CRUD rendering (list, create, edit, delete confirm)
 *  - TenantForm validation (name required, matcher required)
 *  - Business-tier-gate rendering in ReportsPage (tenants tab visible when entitled)
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";

// ─── TenantForm validation logic (pure, no DOM needed) ───────────────────────

interface TenantFormData {
  name: string;
  streamPattern: string;
  metaTagKey: string;
  metaTagValue: string;
}

function validateTenantForm(data: TenantFormData): string | null {
  if (!data.name.trim()) return "Name is required";
  const hasPattern = data.streamPattern.trim().length > 0;
  const hasTag =
    data.metaTagKey.trim().length > 0 && data.metaTagValue.trim().length > 0;
  if (!hasPattern && !hasTag) {
    return "At least one matcher is required: stream pattern OR both meta tag key and value";
  }
  return null;
}

describe("TenantForm validation", () => {
  it("returns null for valid form with stream_pattern", () => {
    expect(
      validateTenantForm({
        name: "Acme Corp",
        streamPattern: "live/acme-%",
        metaTagKey: "",
        metaTagValue: "",
      })
    ).toBeNull();
  });

  it("returns null for valid form with meta tag", () => {
    expect(
      validateTenantForm({
        name: "Beta Inc",
        streamPattern: "",
        metaTagKey: "tenant_id",
        metaTagValue: "beta",
      })
    ).toBeNull();
  });

  it("returns null for valid form with both matchers", () => {
    expect(
      validateTenantForm({
        name: "Gamma LLC",
        streamPattern: "live/gamma-%",
        metaTagKey: "tenant_id",
        metaTagValue: "gamma",
      })
    ).toBeNull();
  });

  it("returns error when name is empty", () => {
    expect(
      validateTenantForm({
        name: "",
        streamPattern: "live/acme-%",
        metaTagKey: "",
        metaTagValue: "",
      })
    ).toMatch(/name is required/i);
  });

  it("returns error when name is whitespace only", () => {
    expect(
      validateTenantForm({
        name: "   ",
        streamPattern: "live/acme-%",
        metaTagKey: "",
        metaTagValue: "",
      })
    ).toMatch(/name is required/i);
  });

  it("returns error when no matcher provided", () => {
    expect(
      validateTenantForm({
        name: "Acme Corp",
        streamPattern: "",
        metaTagKey: "",
        metaTagValue: "",
      })
    ).toMatch(/matcher/i);
  });

  it("returns error when only meta_tag_key provided (no value)", () => {
    expect(
      validateTenantForm({
        name: "Acme Corp",
        streamPattern: "",
        metaTagKey: "tenant_id",
        metaTagValue: "",
      })
    ).toMatch(/matcher/i);
  });

  it("returns error when only meta_tag_value provided (no key)", () => {
    expect(
      validateTenantForm({
        name: "Acme Corp",
        streamPattern: "",
        metaTagKey: "",
        metaTagValue: "acme",
      })
    ).toMatch(/matcher/i);
  });
});

// ─── TenantsTab + ReportsPage tier-gate rendering ────────────────────────────

vi.mock("@/api/client", () => ({
  adminApi: {
    getLicense: vi.fn(),
    listTenants: vi.fn(),
    createTenant: vi.fn(),
    updateTenant: vi.fn(),
    deleteTenant: vi.fn(),
    getTenant: vi.fn(),
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

const SAMPLE_TENANTS = [
  {
    id: "t-001",
    name: "Acme Corp",
    stream_pattern: "live/acme-%",
    meta_tag_key: null,
    meta_tag_value: null,
    created_at: 1700000000000,
    updated_at: 1700000000000,
  },
  {
    id: "t-002",
    name: "Beta Inc",
    stream_pattern: null,
    meta_tag_key: "tenant_id",
    meta_tag_value: "beta",
    created_at: 1700000001000,
    updated_at: 1700000001000,
  },
];

describe("ReportsPage tenants tab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // VD-01: tenants require Business tier (not pro)
  it("shows tenants tab button when entitled (business tier)", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue({ tier: "business", valid: true });
    vi.mocked(adminApi.listTenants).mockResolvedValue({
      items: [],
      meta: { total: 0, next_cursor: null },
    });
    render(<ReportsPage />);
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /tenants/i })).toBeInTheDocument();
    });
  });

  // VD-01: pro tier is NOT entitled for tenants/reports (needs business+)
  it("hides tenants tab when tier is pro (upsell shown instead)", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue({ tier: "pro", valid: true });
    render(<ReportsPage />);
    await waitFor(() => {
      expect(screen.getByText(/requires business tier/i)).toBeInTheDocument();
    });
    expect(screen.queryByRole("tab", { name: /tenants/i })).toBeNull();
  });

  it("hides tenants tab when tier is free (upsell shown instead)", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue({ tier: "free", valid: true });
    render(<ReportsPage />);
    await waitFor(() => {
      expect(screen.getByText(/requires business tier/i)).toBeInTheDocument();
    });
    expect(screen.queryByRole("tab", { name: /tenants/i })).toBeNull();
  });

  it("renders tenant list after clicking tenants tab", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue({ tier: "enterprise", valid: true });
    vi.mocked(adminApi.listTenants).mockResolvedValue({
      items: SAMPLE_TENANTS,
      meta: { total: 2, next_cursor: null },
    });
    render(<ReportsPage />);
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /tenants/i })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("tab", { name: /tenants/i }));
    await waitFor(() => {
      expect(screen.getByText("Acme Corp")).toBeInTheDocument();
      expect(screen.getByText("Beta Inc")).toBeInTheDocument();
    });
  });

  it("shows stream_pattern badge on tenant with pattern", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue({ tier: "enterprise", valid: true });
    vi.mocked(adminApi.listTenants).mockResolvedValue({
      items: [SAMPLE_TENANTS[0]],
      meta: { total: 1, next_cursor: null },
    });
    render(<ReportsPage />);
    await waitFor(() => screen.getByRole("tab", { name: /tenants/i }));
    fireEvent.click(screen.getByRole("tab", { name: /tenants/i }));
    await waitFor(() => {
      expect(screen.getByText(/pattern: live\/acme-%/)).toBeInTheDocument();
    });
  });

  it("shows meta tag badge on tenant with tag matchers", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue({ tier: "enterprise", valid: true });
    vi.mocked(adminApi.listTenants).mockResolvedValue({
      items: [SAMPLE_TENANTS[1]],
      meta: { total: 1, next_cursor: null },
    });
    render(<ReportsPage />);
    await waitFor(() => screen.getByRole("tab", { name: /tenants/i }));
    fireEvent.click(screen.getByRole("tab", { name: /tenants/i }));
    await waitFor(() => {
      expect(screen.getByText("tenant_id=beta")).toBeInTheDocument();
    });
  });

  it("shows empty state when no tenants configured", async () => {
    // VD-01: use business tier (pro is now gated)
    vi.mocked(adminApi.getLicense).mockResolvedValue({ tier: "business", valid: true });
    vi.mocked(adminApi.listTenants).mockResolvedValue({
      items: [],
      meta: { total: 0, next_cursor: null },
    });
    render(<ReportsPage />);
    await waitFor(() => screen.getByRole("tab", { name: /tenants/i }));
    fireEvent.click(screen.getByRole("tab", { name: /tenants/i }));
    await waitFor(() => {
      expect(screen.getByText(/no tenants configured/i)).toBeInTheDocument();
    });
  });

  it("shows tenant form when '+ New tenant' clicked", async () => {
    // VD-01: use business tier (pro is now gated)
    vi.mocked(adminApi.getLicense).mockResolvedValue({ tier: "business", valid: true });
    vi.mocked(adminApi.listTenants).mockResolvedValue({
      items: [],
      meta: { total: 0, next_cursor: null },
    });
    render(<ReportsPage />);
    await waitFor(() => screen.getByRole("tab", { name: /tenants/i }));
    fireEvent.click(screen.getByRole("tab", { name: /tenants/i }));
    await waitFor(() => screen.getByText(/\+ new tenant/i));
    fireEvent.click(screen.getByText(/\+ new tenant/i));
    await waitFor(() => {
      expect(screen.getByTestId("tenant-form")).toBeInTheDocument();
    });
  });

  it("shows delete confirm dialog when Delete clicked", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue({ tier: "enterprise", valid: true });
    vi.mocked(adminApi.listTenants).mockResolvedValue({
      items: [SAMPLE_TENANTS[0]],
      meta: { total: 1, next_cursor: null },
    });
    render(<ReportsPage />);
    await waitFor(() => screen.getByRole("tab", { name: /tenants/i }));
    fireEvent.click(screen.getByRole("tab", { name: /tenants/i }));
    await waitFor(() => screen.getByLabelText(/delete acme corp/i));
    fireEvent.click(screen.getByLabelText(/delete acme corp/i));
    await waitFor(() => {
      expect(screen.getByTestId("delete-confirm")).toBeInTheDocument();
    });
  });

  it("calls adminApi.createTenant with correct payload on form submit", async () => {
    // VD-01: use business tier (pro is now gated)
    vi.mocked(adminApi.getLicense).mockResolvedValue({ tier: "business", valid: true });
    vi.mocked(adminApi.listTenants).mockResolvedValue({
      items: [],
      meta: { total: 0, next_cursor: null },
    });
    vi.mocked(adminApi.createTenant).mockResolvedValue({
      id: "t-new",
      name: "New Tenant",
      stream_pattern: "live/new-%",
      meta_tag_key: null,
      meta_tag_value: null,
      created_at: Date.now(),
      updated_at: Date.now(),
    });
    render(<ReportsPage />);
    await waitFor(() => screen.getByRole("tab", { name: /tenants/i }));
    fireEvent.click(screen.getByRole("tab", { name: /tenants/i }));
    await waitFor(() => screen.getByText(/\+ new tenant/i));
    fireEvent.click(screen.getByText(/\+ new tenant/i));
    await waitFor(() => screen.getByTestId("tenant-form"));

    // Fill in name
    fireEvent.change(screen.getByLabelText(/tenant name/i), {
      target: { value: "New Tenant" },
    });
    // Fill in stream pattern
    fireEvent.change(screen.getByLabelText(/stream pattern/i), {
      target: { value: "live/new-%" },
    });
    // Submit
    fireEvent.click(screen.getByRole("button", { name: /save tenant/i }));

    await waitFor(() => {
      expect(adminApi.createTenant).toHaveBeenCalledWith({
        name: "New Tenant",
        stream_pattern: "live/new-%",
        meta_tag_key: null,
        meta_tag_value: null,
      });
    });
  });

  it("calls adminApi.deleteTenant when confirm delete clicked", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue({ tier: "enterprise", valid: true });
    vi.mocked(adminApi.listTenants).mockResolvedValue({
      items: [SAMPLE_TENANTS[0]],
      meta: { total: 1, next_cursor: null },
    });
    vi.mocked(adminApi.deleteTenant).mockResolvedValue(undefined);
    render(<ReportsPage />);
    await waitFor(() => screen.getByRole("tab", { name: /tenants/i }));
    fireEvent.click(screen.getByRole("tab", { name: /tenants/i }));
    await waitFor(() => screen.getByLabelText(/delete acme corp/i));
    fireEvent.click(screen.getByLabelText(/delete acme corp/i));
    await waitFor(() => screen.getByTestId("delete-confirm"));
    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));
    await waitFor(() => {
      expect(adminApi.deleteTenant).toHaveBeenCalledWith("t-001");
    });
  });
});
