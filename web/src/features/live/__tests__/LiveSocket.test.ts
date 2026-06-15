/**
 * LiveSocket unit tests — reconnect logic and fallback behavior.
 * Tests use vitest with fake timers and a manual WebSocket mock.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { LiveSocket } from "@/api/client";

// ─── WebSocket mock ───────────────────────────────────────────────────────────
class MockWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  readyState = MockWebSocket.CONNECTING;
  url: string;
  onopen: (() => void) | null = null;
  onclose: ((ev: { code: number; reason: string }) => void) | null = null;
  onerror: ((ev: unknown) => void) | null = null;
  onmessage: ((ev: { data: string }) => void) | null = null;

  static instances: MockWebSocket[] = [];

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }

  open() {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.();
  }

  triggerClose(code = 1006, reason = "") {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.({ code, reason });
  }

  triggerMessage(data: unknown) {
    this.onmessage?.({ data: JSON.stringify(data) });
  }

  close() {
    this.triggerClose(1000, "normal");
  }

  send(_data: string) {
    // noop for tests
  }
}

beforeEach(() => {
  MockWebSocket.instances = [];
  vi.stubGlobal("WebSocket", MockWebSocket);
  // Ensure localStorage.getItem returns null (no token by default)
  vi.stubGlobal("localStorage", {
    getItem: vi.fn().mockReturnValue(null),
    setItem: vi.fn(),
    removeItem: vi.fn(),
  });
  vi.stubGlobal("window", {
    location: { protocol: "http:", host: "localhost:5173" },
  });
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("LiveSocket", () => {
  it("calls onStatusChange(true) when socket opens", () => {
    const onStatusChange = vi.fn();
    const sock = new LiveSocket({ onStatusChange });
    sock.connect();

    expect(MockWebSocket.instances).toHaveLength(1);
    MockWebSocket.instances[0].open();

    expect(onStatusChange).toHaveBeenCalledWith(true);
    sock.destroy();
  });

  it("calls onStatusChange(false) when socket closes", () => {
    const onStatusChange = vi.fn();
    const sock = new LiveSocket({ onStatusChange });
    sock.connect();

    MockWebSocket.instances[0].open();
    MockWebSocket.instances[0].triggerClose();

    expect(onStatusChange).toHaveBeenCalledWith(false);
    sock.destroy();
  });

  it("schedules reconnect after socket close (backoff)", () => {
    const sock = new LiveSocket({ baseDelay: 1000, maxDelay: 30_000 });
    sock.connect();

    const ws1 = MockWebSocket.instances[0];
    ws1.open();
    ws1.triggerClose();

    // After close, a timer should be scheduled; before it fires, no new socket
    expect(MockWebSocket.instances).toHaveLength(1);

    // Advance time past baseDelay
    vi.advanceTimersByTime(1100);
    expect(MockWebSocket.instances).toHaveLength(2);

    sock.destroy();
  });

  it("doubles backoff delay on repeated failures", () => {
    const sock = new LiveSocket({ baseDelay: 500, maxDelay: 30_000 });
    sock.connect();

    // First failure
    MockWebSocket.instances[0].triggerClose();
    vi.advanceTimersByTime(600);
    expect(MockWebSocket.instances).toHaveLength(2);

    // Second failure — delay should have doubled to 1000ms
    MockWebSocket.instances[1].triggerClose();
    vi.advanceTimersByTime(600); // not enough
    expect(MockWebSocket.instances).toHaveLength(2); // still 2

    vi.advanceTimersByTime(500); // total 1100ms — enough
    expect(MockWebSocket.instances).toHaveLength(3);

    sock.destroy();
  });

  it("resets backoff to baseDelay on successful connect", () => {
    const sock = new LiveSocket({ baseDelay: 500, maxDelay: 30_000 });
    sock.connect();

    // Fail once (delay doubles to 1000)
    MockWebSocket.instances[0].triggerClose();
    vi.advanceTimersByTime(600);
    // Connect again and succeed (resets delay back to 500)
    MockWebSocket.instances[1].open();
    MockWebSocket.instances[1].triggerClose();

    // Should reconnect at baseDelay again (500ms)
    vi.advanceTimersByTime(600);
    expect(MockWebSocket.instances).toHaveLength(3);

    sock.destroy();
  });

  it("delivers snapshot messages to subscribers", () => {
    const messages: unknown[] = [];
    const sock = new LiveSocket();
    sock.connect();
    sock.subscribe((msg) => messages.push(msg));

    MockWebSocket.instances[0].open();
    MockWebSocket.instances[0].triggerMessage({
      type: "snapshot",
      ts: 1700000000000,
      payload: { total_viewers: 42 },
    });

    expect(messages).toHaveLength(1);
    expect((messages[0] as { type: string }).type).toBe("snapshot");

    sock.destroy();
  });

  // VD-02: verify WS payload is read as LiveOverview and carries all required fields
  it("delivers LiveOverview fields — total_publishers, protocol_mix, apps — from WS snapshot", () => {
    type Msg = { type: string; ts: number; payload: Record<string, unknown> };
    const messages: Msg[] = [];
    const sock = new LiveSocket();
    sock.connect();
    sock.subscribe((msg) => messages.push(msg as Msg));

    MockWebSocket.instances[0].open();
    const liveOverviewPayload = {
      ts: 1700000001000,
      total_viewers: 120,
      total_publishers: 8,             // field that was missing from old LiveSnapshot
      protocol_mix: { webrtc: 80, hls: 40, rtmp: 0, dash: 0, other: 0 },
      apps: [{ app: "live", viewers: 120, publishers: 8, streams: 8 }],
      nodes: [],
    };

    MockWebSocket.instances[0].triggerMessage({
      type: "snapshot",
      ts: 1700000001000,
      payload: liveOverviewPayload,
    });

    expect(messages).toHaveLength(1);
    const msg = messages[0];
    expect(msg.type).toBe("snapshot");
    // Payload must carry the full LiveOverview shape including total_publishers,
    // protocol_mix, and apps (VD-02: these fields went stale on old LiveSnapshot).
    expect(msg.payload.total_publishers).toBe(8);
    expect(msg.payload.protocol_mix).toEqual({ webrtc: 80, hls: 40, rtmp: 0, dash: 0, other: 0 });
    expect(msg.payload.apps).toHaveLength(1);
    expect((msg.payload.apps as { app: string }[])[0].app).toBe("live");

    sock.destroy();
  });

  // VD-02: delta message also carries LiveOverview fields
  it("delivers LiveOverview fields from WS delta message", () => {
    type Msg = { type: string; payload: Record<string, unknown> };
    const messages: Msg[] = [];
    const sock = new LiveSocket();
    sock.connect();
    sock.subscribe((msg) => messages.push(msg as Msg));

    MockWebSocket.instances[0].open();
    // Delta carries partial LiveOverview — total_publishers and protocol_mix must be present
    MockWebSocket.instances[0].triggerMessage({
      type: "delta",
      ts: 1700000002000,
      payload: {
        total_viewers: 130,
        total_publishers: 9,
        protocol_mix: { webrtc: 90, hls: 40, rtmp: 0, dash: 0, other: 0 },
      },
    });

    expect(messages).toHaveLength(1);
    const msg = messages[0];
    expect(msg.type).toBe("delta");
    expect(msg.payload.total_publishers).toBe(9);
    expect(msg.payload.protocol_mix).toBeDefined();

    sock.destroy();
  });

  it("does not reconnect after destroy()", () => {
    const sock = new LiveSocket({ baseDelay: 100 });
    sock.connect();
    MockWebSocket.instances[0].open();
    sock.destroy();
    MockWebSocket.instances[0].triggerClose();

    vi.advanceTimersByTime(500);
    // Destroy cancels retry; no new socket
    expect(MockWebSocket.instances).toHaveLength(1);
  });

  it("unsubscribe removes listener", () => {
    const messages: unknown[] = [];
    const sock = new LiveSocket();
    sock.connect();
    const unsub = sock.subscribe((msg) => messages.push(msg));
    MockWebSocket.instances[0].open();
    unsub();

    MockWebSocket.instances[0].triggerMessage({ type: "heartbeat", ts: 1 });
    expect(messages).toHaveLength(0);

    sock.destroy();
  });
});
