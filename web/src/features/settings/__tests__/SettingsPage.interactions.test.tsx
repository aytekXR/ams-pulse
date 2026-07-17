/**
 * SettingsPage — interaction & data-rendering coverage (S83/D-145).
 *
 * Complements __tests__/SettingsPage.test.tsx (rendering/ARIA) and the sibling
 * SettingsPage.test.tsx (S73 error-toast guard) by exercising the branches those
 * two leave uncovered (lines ~378–721): ingest-token creation + the IngestSnippet
 * copy button, populated token/source list rows, the delete/revoke success and
 * cancel paths, the S3 export form, and the license card + activation form.
 *
 * Pure test additions — no behavior change, no prod deploy. adminApi and useToast
 * are replaced so each side effect (API call, toast) is asserted directly.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ApiError } from "@/api/client";
import { SettingsPage } from "../SettingsPage";

// vi.hoisted so the mock objects exist before the hoisted vi.mock factories run.
const h = vi.hoisted(() => ({
  getSources: vi.fn(),
  getTokens: vi.fn(),
  getLicense: vi.fn(),
  createToken: vi.fn(),
  deleteToken: vi.fn(),
  deleteSource: vi.fn(),
  setLicense: vi.fn(),
  toast: vi.fn(),
}));

// Keep the real ApiError export so `err instanceof ApiError` still narrows.
vi.mock("@/api/client", async (orig) => {
  const actual = await orig<typeof import("@/api/client")>();
  return {
    ...actual,
    adminApi: {
      getSources: h.getSources,
      getTokens: h.getTokens,
      getLicense: h.getLicense,
      createToken: h.createToken,
      deleteToken: h.deleteToken,
      deleteSource: h.deleteSource,
      setLicense: h.setLicense,
    },
  };
});

vi.mock("@/components/Toast", () => ({
  useToast: () => ({ toast: h.toast }),
  ToastProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

// Epoch-ms timestamps (the API returns numbers, not ISO strings).
const JAN_2024 = 1704067200000; // 2024-01-01
const FEB_2024 = 1706745600000; // 2024-02-01

function setDefaults() {
  h.getSources.mockResolvedValue({ items: [] });
  h.getTokens.mockResolvedValue({ items: [] });
  h.getLicense.mockResolvedValue({ tier: "free", valid: true });
}

function stubClipboard() {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
  return writeText;
}

async function gotoTab(name: RegExp) {
  render(<SettingsPage />);
  await waitFor(() => expect(h.getSources).toHaveBeenCalled());
  fireEvent.click(screen.getByRole("tab", { name }));
}

describe("SettingsPage — ingest tokens", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setDefaults();
  });
  afterEach(() => vi.restoreAllMocks());

  it("creates an ingest token and shows the SDK snippet with the raw value", async () => {
    h.createToken.mockResolvedValue({
      token: "ingest_raw_XYZ",
      id: "ing-1",
      kind: "ingest",
      name: "player-prod",
      scopes: ["ingest"],
      created_at: JAN_2024,
    });
    await gotoTab(/ingest tokens/i);
    vi.spyOn(window, "prompt").mockReturnValueOnce("player-prod");
    fireEvent.click(screen.getByRole("button", { name: /\+ new ingest token/i }));

    // Raw token shows in both the panel and the SDK snippet.
    await waitFor(() => {
      expect(screen.getAllByText(/ingest_raw_XYZ/).length).toBeGreaterThanOrEqual(1);
    });
    expect(screen.getByText(/Pulse\.init/)).toBeInTheDocument();
    expect(h.createToken).toHaveBeenCalledWith({
      kind: "ingest",
      name: "player-prod",
      scopes: ["ingest"],
    });
    expect(h.toast).toHaveBeenCalledWith(
      expect.stringMatching(/won't be shown again/i),
      "success",
    );
  });

  it("copies the SDK snippet to the clipboard", async () => {
    const writeText = stubClipboard();
    h.createToken.mockResolvedValue({
      token: "ingest_raw_XYZ",
      id: "ing-1",
      kind: "ingest",
      name: "p",
      scopes: ["ingest"],
      created_at: JAN_2024,
    });
    await gotoTab(/ingest tokens/i);
    vi.spyOn(window, "prompt").mockReturnValueOnce("p");
    fireEvent.click(screen.getByRole("button", { name: /\+ new ingest token/i }));
    await waitFor(() => expect(screen.getByText(/Pulse\.init/)).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: /^copy$/i }));
    await waitFor(() => expect(writeText).toHaveBeenCalled());
    expect(writeText.mock.calls[0][0]).toContain("Pulse.init");
    expect(writeText.mock.calls[0][0]).toContain("ingest_raw_XYZ");
  });

  it("cancelling the name prompt creates nothing", async () => {
    await gotoTab(/ingest tokens/i);
    vi.spyOn(window, "prompt").mockReturnValueOnce(null);
    fireEvent.click(screen.getByRole("button", { name: /\+ new ingest token/i }));
    expect(h.createToken).not.toHaveBeenCalled();
  });

  it("renders ingest token rows and revokes on confirm", async () => {
    h.getTokens.mockResolvedValue({
      items: [
        {
          id: "ing-9",
          name: "player-prod",
          kind: "ingest",
          scopes: ["ingest"],
          created_at: JAN_2024,
          last_used_at: FEB_2024,
        },
      ],
    });
    h.deleteToken.mockResolvedValue(undefined);
    await gotoTab(/ingest tokens/i);

    expect(await screen.findByText("player-prod")).toBeInTheDocument();
    expect(screen.getByText(/last used/i)).toBeInTheDocument();

    vi.spyOn(window, "confirm").mockReturnValueOnce(true);
    fireEvent.click(screen.getByRole("button", { name: /revoke/i }));
    await waitFor(() => expect(h.deleteToken).toHaveBeenCalledWith("ing-9"));
    expect(h.toast).toHaveBeenCalledWith("Token revoked", "info");
  });

  it("does not revoke when confirm is declined", async () => {
    h.getTokens.mockResolvedValue({
      items: [{ id: "ing-9", name: "player-prod", kind: "ingest", scopes: ["ingest"], created_at: JAN_2024 }],
    });
    await gotoTab(/ingest tokens/i);
    await screen.findByText("player-prod");
    vi.spyOn(window, "confirm").mockReturnValueOnce(false);
    fireEvent.click(screen.getByRole("button", { name: /revoke/i }));
    expect(h.deleteToken).not.toHaveBeenCalled();
  });
});

describe("SettingsPage — sources & API tokens", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setDefaults();
  });
  afterEach(() => vi.restoreAllMocks());

  it("renders source rows and removes on confirm", async () => {
    h.getSources.mockResolvedValue({
      items: [{ id: "s1", name: "Prod AMS", type: "rest_poll", rest_url: "http://ams:5080" }],
    });
    h.deleteSource.mockResolvedValue(undefined);
    render(<SettingsPage />);

    expect(await screen.findByText("Prod AMS")).toBeInTheDocument();
    expect(screen.getByText("http://ams:5080")).toBeInTheDocument();

    vi.spyOn(window, "confirm").mockReturnValueOnce(true);
    fireEvent.click(screen.getByRole("button", { name: /remove/i }));
    await waitFor(() => expect(h.deleteSource).toHaveBeenCalledWith("s1"));
    expect(h.toast).toHaveBeenCalledWith("Source removed", "info");
  });

  it("Add source button shows the onboarding hint toast", async () => {
    render(<SettingsPage />);
    await waitFor(() => expect(h.getSources).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("button", { name: /\+ add source/i }));
    expect(h.toast).toHaveBeenCalledWith("Use the onboarding wizard to add sources", "info");
  });

  it("renders API token rows with scopes and last-used date", async () => {
    h.getTokens.mockResolvedValue({
      items: [
        {
          id: "api-1",
          name: "ci-reader",
          kind: "api",
          scopes: ["read"],
          created_at: JAN_2024,
          last_used_at: FEB_2024,
        },
      ],
    });
    await gotoTab(/api tokens/i);
    expect(await screen.findByText("ci-reader")).toBeInTheDocument();
    expect(screen.getByText(/last used/i)).toBeInTheDocument();
  });

  it("copies a newly created API token to the clipboard", async () => {
    const writeText = stubClipboard();
    h.createToken.mockResolvedValue({
      token: "api_raw_ABC",
      id: "api-1",
      kind: "api",
      name: "t",
      scopes: ["read"],
      created_at: JAN_2024,
    });
    await gotoTab(/api tokens/i);
    vi.spyOn(window, "prompt").mockReturnValueOnce("ci");
    vi.spyOn(window, "confirm").mockReturnValueOnce(false); // read-only scope
    fireEvent.click(screen.getByRole("button", { name: /\+ new token/i }));

    await waitFor(() => expect(screen.getByText("api_raw_ABC")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /^copy$/i }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith("api_raw_ABC"));
  });

  it("cancelling the API-token name prompt creates nothing", async () => {
    await gotoTab(/api tokens/i);
    vi.spyOn(window, "prompt").mockReturnValueOnce(null);
    fireEvent.click(screen.getByRole("button", { name: /\+ new token/i }));
    expect(h.createToken).not.toHaveBeenCalled();
  });
});

describe("SettingsPage — integrations S3 form", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setDefaults();
  });
  afterEach(() => vi.restoreAllMocks());

  it("fills and submits the S3 export form", async () => {
    await gotoTab(/integrations/i);
    fireEvent.change(await screen.findByPlaceholderText(/my-pulse-reports/i), {
      target: { value: "my-bucket" },
    });
    fireEvent.change(screen.getByPlaceholderText("us-east-1"), { target: { value: "eu-west-1" } });
    fireEvent.change(screen.getByPlaceholderText("AWS_ACCESS_KEY_ID"), { target: { value: "MY_KEY" } });
    fireEvent.change(screen.getByPlaceholderText("AWS_SECRET_ACCESS_KEY"), {
      target: { value: "MY_SECRET" },
    });
    expect(screen.getByDisplayValue("my-bucket")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /save s3 config/i }));
    expect(h.toast).toHaveBeenCalledWith(
      expect.stringMatching(/s3 export config saved/i),
      "info",
    );
  });
});

describe("SettingsPage — license tab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setDefaults();
  });
  afterEach(() => vi.restoreAllMocks());

  it("renders the license card with tier badge, expiry, and limits (-1 → ∞)", async () => {
    h.getLicense.mockResolvedValue({
      tier: "pro",
      valid: true,
      expires_at: 1798761600000, // 2027-01-01
      limits: { max_streams: 100, max_nodes: -1, retention_days: 90 },
    });
    await gotoTab(/license/i);

    expect(await screen.findByText(/current license/i)).toBeInTheDocument();
    expect(screen.getByText("pro")).toBeInTheDocument();
    expect(screen.getByText(/expires/i)).toBeInTheDocument();
    expect(screen.getByText(/max streams/i)).toBeInTheDocument();
    expect(screen.getByText(/max nodes/i)).toBeInTheDocument();
    expect(screen.getByText("100")).toBeInTheDocument();
    expect(screen.getByText("∞")).toBeInTheDocument();
    // tier != free → the form heading is "Update license key"
    expect(screen.getByRole("heading", { name: /update license key/i })).toBeInTheDocument();
  });

  it("activates a license key, toasts the new tier, and clears the input", async () => {
    h.setLicense.mockResolvedValue({ tier: "pro", valid: true });
    await gotoTab(/license/i);

    const input = (await screen.findByPlaceholderText(/PULSE-XXXX/i)) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "PULSE-AAAA-BBBB-CCCC" } });
    fireEvent.click(screen.getByRole("button", { name: /^activate$/i }));

    await waitFor(() => expect(h.setLicense).toHaveBeenCalledWith("PULSE-AAAA-BBBB-CCCC"));
    expect(h.toast).toHaveBeenCalledWith("License activated — tier: pro", "success");
    await waitFor(() => expect(input.value).toBe(""));
  });

  it("shows an error toast when activation fails", async () => {
    h.setLicense.mockRejectedValue(new ApiError(400, { code: "BAD_KEY", message: "invalid key" }));
    await gotoTab(/license/i);

    const input = await screen.findByPlaceholderText(/PULSE-XXXX/i);
    fireEvent.change(input, { target: { value: "PULSE-BAD" } });
    fireEvent.click(screen.getByRole("button", { name: /^activate$/i }));

    await waitFor(() => expect(h.toast).toHaveBeenCalledWith("invalid key", "error"));
  });
});
