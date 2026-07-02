/**
 * MSW Node server — shared across all test files via src/test/setup.ts.
 *
 * Default handlers provide realistic stub responses for every endpoint
 * exercised by the msw-based component tests. Individual tests that need
 * different data can call server.use(...) to add one-off overrides;
 * afterEach(server.resetHandlers) in setup.ts cleans those overrides up.
 */
import { setupServer } from "msw/node";
import { http, HttpResponse } from "msw";

// Base URL that msw intercepts — must match window.location.href set via
// vite.config.ts test.environmentOptions.jsdom.url = "http://localhost".
const BASE = "http://localhost";

export const handlers = [
  // ── Live ────────────────────────────────────────────────────────────────────

  http.get(`${BASE}/api/v1/live/overview`, () =>
    HttpResponse.json({
      ts: 1_700_000_000_000,
      total_viewers: 42,
      total_publishers: 3,
      nodes: [{ node_id: "node-1", status: "up", cpu_pct: 40, mem_pct: 60 }],
      apps: [{ app: "live", viewers: 42, publishers: 3, streams: 1 }],
      protocol_mix: { webrtc: 20, hls: 15, rtmp: 5, dash: 2, other: 0 },
    })
  ),

  http.get(`${BASE}/api/v1/live/streams`, () =>
    HttpResponse.json({
      items: [
        {
          stream_id: "test-stream-1",
          app: "live",
          node_id: "node-1",
          publisher_state: "publishing",
          viewers: 10,
          bitrate_kbps: 2500,
          health_score: 95,
        },
      ],
      meta: { total: 1 },
    })
  ),

  // ── Alerts ──────────────────────────────────────────────────────────────────

  http.get(`${BASE}/api/v1/alerts/rules`, () =>
    HttpResponse.json({
      items: [
        {
          id: "rule-1",
          name: "High CPU Alert",
          metric: "cpu_pct",
          operator: "gt",
          threshold: 80,
          window_s: 300,
          severity: "warning",
          cooldown_s: 300,
          enabled: true,
          muted: false,
          created_at: 1_000_000,
          updated_at: 1_000_000,
        },
      ],
    })
  ),

  http.get(`${BASE}/api/v1/alerts/channels`, () =>
    HttpResponse.json({ items: [] })
  ),

  http.get(`${BASE}/api/v1/alerts/history`, () =>
    HttpResponse.json({ items: [] })
  ),

  http.post(`${BASE}/api/v1/alerts/rules`, () =>
    HttpResponse.json(
      {
        id: "rule-new",
        name: "New Rule",
        metric: "cpu_pct",
        operator: "gt",
        threshold: 80,
        window_s: 300,
        severity: "warning",
        cooldown_s: 300,
        enabled: true,
        muted: false,
        created_at: 1_700_000_000_000,
        updated_at: 1_700_000_000_000,
      },
      { status: 201 }
    )
  ),
];

export const server = setupServer(...handlers);
