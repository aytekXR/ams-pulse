/**
 * SettingsPage — S73/D-139 [8] regression guard.
 *
 * deleteSource / deleteToken / createApiToken / createIngestToken awaited their API
 * call with no try/catch and were invoked as `() => void handler()`, so a failed
 * request was silently swallowed — no error toast, no feedback. Each is now wrapped
 * in try/catch + toast(..., "error") (mirroring saveLicense). This asserts a failed
 * deleteSource surfaces the error to the user; the other three share the pattern.
 *
 * Mutation proof: remove the catch from deleteSource → the toast never fires → the
 * final assertion times out and this test goes RED.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ApiError } from "@/api/client";
import { ToastProvider } from "@/components/Toast";
import { SettingsPage } from "./SettingsPage";

// Replace adminApi with mocks; keep the real ApiError so `err instanceof ApiError` narrows.
// vi.hoisted so the object is initialized before the hoisted vi.mock factory runs.
const { mockAdminApi } = vi.hoisted(() => ({
  mockAdminApi: {
    getSources: vi.fn(),
    getTokens: vi.fn(),
    getLicense: vi.fn(),
    deleteSource: vi.fn(),
    deleteToken: vi.fn(),
    createToken: vi.fn(),
    setLicense: vi.fn(),
  },
}));
vi.mock("@/api/client", async (orig) => {
  const actual = await orig<typeof import("@/api/client")>();
  return { ...actual, adminApi: mockAdminApi };
});

describe("SettingsPage — API errors surface a toast (S73/D-139 [8])", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAdminApi.getSources.mockResolvedValue({ items: [{ id: "s1", name: "Test Source", type: "ams" }] });
    mockAdminApi.getTokens.mockResolvedValue({ items: [] });
    mockAdminApi.getLicense.mockResolvedValue({ tier: "free", valid: true });
    vi.stubGlobal("confirm", vi.fn(() => true));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("deleteSource failure shows an error toast instead of silently swallowing it", async () => {
    mockAdminApi.deleteSource.mockRejectedValue(
      new ApiError(500, { code: "INTERNAL", message: "ClickHouse unavailable" }),
    );

    render(
      <ToastProvider>
        <SettingsPage />
      </ToastProvider>,
    );

    // Wait for the loaded source row, then click its Remove button.
    const removeBtn = await screen.findByText("Remove");
    fireEvent.click(removeBtn);

    // The API error message must surface as a toast (was silently dropped).
    await waitFor(() => {
      expect(screen.getByText("ClickHouse unavailable")).toBeTruthy();
    });
  });
});
