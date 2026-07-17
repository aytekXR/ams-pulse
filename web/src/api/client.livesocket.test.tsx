/**
 * LiveSocket — S73/D-140 [7] regression guard.
 *
 * The Live WebSocket used to embed the bearer token in the URL as ?token=<token>,
 * which reverse proxies (Caddy) record in their access logs. The token now travels
 * in the Sec-WebSocket-Protocol handshake header instead: the browser offers
 * ["pulse.v1", token]. This asserts connect() keeps the token OUT of the URL and
 * passes it as the subprotocol.
 *
 * Mutation proof: revert connect() to put ?token= in the URL → the "not in URL"
 * assertion goes RED.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { LiveSocket } from "./client";

let captured: { url: string; protocols?: string | string[] } | null = null;

class MockWS {
  onopen: (() => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  constructor(url: string, protocols?: string | string[]) {
    captured = { url, protocols };
  }
  close() {}
}

describe("LiveSocket — bearer token via subprotocol, not URL (S73/D-140 [7])", () => {
  beforeEach(() => {
    captured = null;
    vi.stubGlobal("WebSocket", MockWS as unknown as typeof WebSocket);
  });
  afterEach(() => {
    localStorage.clear();
    vi.unstubAllGlobals();
  });

  it("passes the token as a subprotocol and keeps it OUT of the URL", () => {
    localStorage.setItem("pulse_token", "plt_test_token");
    const sock = new LiveSocket();
    sock.connect();

    expect(captured).not.toBeNull();
    expect(captured!.url).not.toContain("plt_test_token");
    expect(captured!.url).not.toContain("token=");
    expect(captured!.protocols).toEqual(["pulse.v1", "plt_test_token"]);

    sock.destroy();
  });

  it("uses no subprotocol when there is no bearer token (OIDC cookie path)", () => {
    localStorage.clear();
    const sock = new LiveSocket();
    sock.connect();

    expect(captured).not.toBeNull();
    expect(captured!.protocols).toBeUndefined();

    sock.destroy();
  });
});
