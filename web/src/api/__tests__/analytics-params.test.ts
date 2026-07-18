/**
 * Analytics client query-param contract (S89 / D-151).
 *
 * The four analytics client calls (audience, geo, devices, CSV export) used to send
 * the stream filter as `?stream_id=`, but the server handlers and the OpenAPI spec
 * name the parameter `stream` (the sibling qoeApi calls already do this correctly).
 * The mismatch meant a stream filter was silently dropped and the caller received
 * data for all streams. These tests pin the contract key.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { analyticsApi, setToken, clearToken } from "@/api/client";

function mockFetchJson() {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response("{}", { status: 200, headers: { "Content-Type": "application/json" } }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("analytics client stream filter param", () => {
  beforeEach(() => {
    setToken("plt_secret");
    // exportCsv goes through downloadFile, which touches these jsdom-absent globals.
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

  it("getAudience sends ?stream= (the server/contract key), not ?stream_id=", async () => {
    const fetchMock = mockFetchJson();
    await analyticsApi.getAudience({ from: 1, to: 2, stream_id: "abc" });
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain("stream=abc");
    expect(url).not.toContain("stream_id=");
  });

  it("getGeo sends ?stream=, not ?stream_id=", async () => {
    const fetchMock = mockFetchJson();
    await analyticsApi.getGeo({ from: 1, to: 2, stream_id: "abc" });
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain("stream=abc");
    expect(url).not.toContain("stream_id=");
  });

  it("getDevices sends ?stream=, not ?stream_id=", async () => {
    const fetchMock = mockFetchJson();
    await analyticsApi.getDevices({ from: 1, to: 2, stream_id: "abc" });
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain("stream=abc");
    expect(url).not.toContain("stream_id=");
  });

  it("exportCsv sends ?stream=, not ?stream_id=", async () => {
    const fetchMock = mockFetchJson();
    await analyticsApi.exportCsv({ from: 1, to: 2, stream_id: "abc" });
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain("stream=abc");
    expect(url).not.toContain("stream_id=");
  });
});
