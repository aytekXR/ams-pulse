/**
 * Authenticated file downloads (S35).
 *
 * Both export buttons used to authenticate by appending `?token=` and letting the
 * browser navigate. That is broken on two counts, and the second one is invisible
 * to every test that mocks the api module:
 *
 *   - `bearerAuthMiddleware` deliberately ignores `?token=` (TestTokenInURL_Ignored
 *     guards it), so the analytics export — which is served by a bearer-auth route —
 *     answered 401. The button had never worked.
 *   - a token in the query string leaks into access logs, proxy caches and browser
 *     history, which is the exact thing the server comment forbids.
 *
 * These tests pin the contract that fixes both: the token travels in the
 * Authorization header, and never in the URL.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { analyticsApi, reportsApi, setToken, clearToken, ApiError } from "@/api/client";

const CSV = "app,stream_id\nlive,abc\n";

function mockFetchOk(headers: Record<string, string> = {}) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(CSV, {
      status: 200,
      headers: { "Content-Type": "text/csv", ...headers },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("authenticated downloads", () => {
  beforeEach(() => {
    setToken("plt_secret");
    // jsdom implements neither of these; the click is what triggers the save.
    vi.stubGlobal("URL", {
      ...URL,
      createObjectURL: vi.fn(() => "blob:mock"),
      revokeObjectURL: vi.fn(),
    });
    vi.spyOn(HTMLElement.prototype, "click").mockImplementation(() => {});
  });

  afterEach(() => {
    clearToken();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("sends the token in the Authorization header, never in the URL", async () => {
    const fetchMock = mockFetchOk();

    await reportsApi.downloadExport({ from: 1, to: 2, format: "csv" });

    const [url, init] = fetchMock.mock.calls[0];
    // The regression this pins: `?token=` on a bearer-auth route is rejected by the
    // server, so a download that relies on it 401s.
    expect(url).not.toContain("token=");
    expect(url).not.toContain("plt_secret");
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer plt_secret");
  });

  it("analytics export hits the audience CSV route with auth, not a token URL", async () => {
    const fetchMock = mockFetchOk();

    await analyticsApi.exportCsv({ from: 1, to: 2 });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/analytics/audience");
    expect(url).toContain("format=csv");
    expect(url).not.toContain("token=");
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer plt_secret");
  });

  it("names the file from Content-Disposition when the server supplies one", async () => {
    mockFetchOk({ "Content-Disposition": 'attachment; filename="usage-2026-07-14.csv"' });
    const anchors: HTMLElement[] = [];
    const realCreate = document.createElement.bind(document);
    vi.spyOn(document, "createElement").mockImplementation((tag: string) => {
      const el = realCreate(tag);
      if (tag === "a") anchors.push(el);
      return el;
    });

    await reportsApi.downloadExport({ from: 1, to: 2, format: "csv" });

    expect(anchors[0].getAttribute("download")).toBe("usage-2026-07-14.csv");
  });

  it("raises ApiError instead of silently saving the error body as a file", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ code: "LICENSE_REQUIRED", message: "business tier required" }), {
          status: 403,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    // Without this, a gated user downloads a .csv containing a JSON error.
    await expect(reportsApi.downloadExport({ from: 1, to: 2, format: "csv" })).rejects.toBeInstanceOf(
      ApiError,
    );
  });
});
