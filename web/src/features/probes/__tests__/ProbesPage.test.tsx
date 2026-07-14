/**
 * Probes page tests (F10):
 *  - Probe list rendering (name, url, protocol, interval, enabled, last_result)
 *  - Create form validation — render-level (tautological pure-fn copy removed per RULE 9)
 *  - Probe results rendering with SYNTHETIC labeling
 *  - Tier-gated views (Free blocked / Pro+ entitled)
 *  - Synthetic-labeling present on results
 *  - Delete confirm flow
 *  - ProbeForm a11y: aria-invalid, aria-describedby, auto-focus (uipro form pass)
 *  - Chart colour pins via source assertions (RULE 9)
 *  - Chart skeleton loading state
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent, within } from "@testing-library/react";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import type { ReactNode } from "react";
import type { Probe, ProbeResult, LicenseInfo } from "@/lib/api/types";
import { ThemeProvider } from "@/lib/ThemeContext";

// ─── Module mocks ─────────────────────────────────────────────────────────────

vi.mock("@/api/client", () => ({
  probesApi: {
    list: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    delete: vi.fn(),
    getResults: vi.fn(),
  },
  adminApi: {
    getLicense: vi.fn(),
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

vi.mock("recharts", () => ({
  LineChart: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="line-chart">{children}</div>
  ),
  Line: () => null,
  XAxis: () => null,
  YAxis: () => null,
  CartesianGrid: () => null,
  Tooltip: () => null,
  Legend: () => null,
  ResponsiveContainer: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="responsive-container">{children}</div>
  ),
  ReferenceLine: () => null,
}));

import { adminApi, probesApi } from "@/api/client";
import { ProbesPage, ttfbColor, iceVariant, signalingVariant } from "../ProbesPage";

// ─── ThemeProvider wrapper ─────────────────────────────────────────────────────
// ProbeResultsPanel calls useStatusColors() → requires ThemeProvider context.
function wrapper({ children }: { children: ReactNode }) {
  return <ThemeProvider>{children}</ThemeProvider>;
}

// ─── Test fixtures ────────────────────────────────────────────────────────────

const freeLicense: LicenseInfo = { tier: "free", valid: true };
const proLicense: LicenseInfo = { tier: "pro", valid: true };
const enterpriseLicense: LicenseInfo = { tier: "enterprise", valid: true };

const now = Date.now();

const sampleProbes: Probe[] = [
  {
    id: "probe-1",
    name: "Main HLS stream",
    url: "https://example.com/live/main.m3u8",
    protocol: "hls",
    interval_s: 60,
    timeout_s: 10,
    enabled: true,
    created_at: now - 86_400_000,
    last_result: {
      id: "result-1",
      probe_id: "probe-1",
      ts: now - 60_000,
      success: true,
      ttfb_ms: 150,
      bitrate_kbps: 2500,
    },
  },
  {
    id: "probe-2",
    name: "Backup stream",
    url: "rtmp://example.com/live/backup",
    protocol: "rtmp",
    interval_s: 120,
    timeout_s: 15,
    enabled: false,
    created_at: now - 3_600_000,
    last_result: {
      id: "result-2",
      probe_id: "probe-2",
      ts: now - 120_000,
      success: false,
      ttfb_ms: null,
      error_code: "timeout",
      error_message: "Connection timed out",
    },
  },
];

const sampleResults: ProbeResult[] = [
  {
    id: "r-1",
    probe_id: "probe-1",
    ts: now - 60_000,
    success: true,
    ttfb_ms: 150,
    bitrate_kbps: 2500,
  },
  {
    id: "r-2",
    probe_id: "probe-1",
    ts: now - 120_000,
    success: false,
    ttfb_ms: null,
    error_code: "http_5xx",
    error_message: "Service unavailable",
  },
];

// ─── Tier gate ────────────────────────────────────────────────────────────────

describe("ProbesPage tier gate", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows loading spinner while license loads", () => {
    vi.mocked(adminApi.getLicense).mockReturnValue(new Promise(() => {}));
    const { unmount } = render(<ProbesPage />, { wrapper });
    expect(screen.getByRole("status")).toBeInTheDocument();
    unmount();
  });

  it("shows upsell when license is 'free'", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(freeLicense);
    render(<ProbesPage />, { wrapper });
    await waitFor(() => {
      expect(
        screen.getByText(/synthetic probes requires pro tier/i),
      ).toBeInTheDocument();
    });
  });

  it("shows upgrade link when gated", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(freeLicense);
    render(<ProbesPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByRole("link", { name: /upgrade license/i })).toBeInTheDocument();
    });
  });

  it("shows probe list when license is 'pro'", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(proLicense);
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
    render(<ProbesPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByText("Main HLS stream")).toBeInTheDocument();
    });
    expect(screen.queryByText(/requires pro tier/i)).toBeNull();
  });

  it("shows probe list when license is 'enterprise'", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(enterpriseLicense);
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
    render(<ProbesPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByText("Main HLS stream")).toBeInTheDocument();
    });
  });
});

// ─── Probe list rendering ─────────────────────────────────────────────────────

describe("ProbesPage probe list rendering", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(adminApi.getLicense).mockResolvedValue(proLicense);
  });

  it("renders probe names", async () => {
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
    render(<ProbesPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByText("Main HLS stream")).toBeInTheDocument();
      expect(screen.getByText("Backup stream")).toBeInTheDocument();
    });
  });

  it("renders protocol badges", async () => {
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
    render(<ProbesPage />, { wrapper });
    await waitFor(() => {
      // Badge renders text-transform: uppercase via CSS but DOM text is lowercase
      expect(screen.getByText("hls")).toBeInTheDocument();
      expect(screen.getByText("rtmp")).toBeInTheDocument();
    });
  });

  it("renders enabled/off status badges", async () => {
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
    render(<ProbesPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByText("on")).toBeInTheDocument();
      expect(screen.getByText("off")).toBeInTheDocument();
    });
  });

  it("renders last result TTFB", async () => {
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
    render(<ProbesPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByText("150 ms")).toBeInTheDocument();
    });
  });

  it("renders last result status ok/fail", async () => {
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
    render(<ProbesPage />, { wrapper });
    await waitFor(() => {
      // Probe 1 has last_result success=true → "ok" badge
      // Probe 2 has last_result success=false → "fail" badge
      expect(screen.getAllByText("ok").length).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText("fail").length).toBeGreaterThanOrEqual(1);
    });
  });

  it("shows empty state when no probes", async () => {
    vi.mocked(probesApi.list).mockResolvedValue({ items: [], meta: {} });
    render(<ProbesPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByText(/no probes configured/i)).toBeInTheDocument();
    });
  });

  it("shows error banner on fetch failure", async () => {
    vi.mocked(probesApi.list).mockRejectedValue(new Error("network error"));
    render(<ProbesPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
  });

  it("renders synthetic notice banner", async () => {
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
    render(<ProbesPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByRole("note", { name: /synthetic probes notice/i })).toBeInTheDocument();
    });
  });
});

// ─── Create form validation — render-level ────────────────────────────────────

describe("ProbesPage create form validation", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(adminApi.getLicense).mockResolvedValue(proLicense);
    vi.mocked(probesApi.list).mockResolvedValue({ items: [], meta: {} });
  });

  async function openForm() {
    render(<ProbesPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByText(/no probes configured/i)).toBeInTheDocument();
    });
    fireEvent.click(screen.getByText("+ New Probe"));
    await waitFor(() => {
      expect(screen.getByLabelText(/create probe form/i)).toBeInTheDocument();
    });
  }

  it("shows create form when button clicked", async () => {
    await openForm();
    expect(screen.getByLabelText(/create probe form/i)).toBeInTheDocument();
  });

  it("validates interval < 30 and shows error", async () => {
    await openForm();
    const nameInput = screen.getByLabelText(/name/i);
    const urlInput = screen.getByLabelText(/stream url/i);
    const intervalInput = screen.getByLabelText(/interval/i);

    fireEvent.change(nameInput, { target: { value: "Test" } });
    fireEvent.change(urlInput, { target: { value: "https://example.com/stream.m3u8" } });
    fireEvent.change(intervalInput, { target: { value: "10" } });

    fireEvent.submit(screen.getByLabelText(/create probe form/i));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByRole("alert")).toHaveTextContent(/≥ 30/i);
  });

  it("validates empty name", async () => {
    await openForm();
    const urlInput = screen.getByLabelText(/stream url/i);
    fireEvent.change(urlInput, { target: { value: "https://example.com/stream.m3u8" } });

    fireEvent.submit(screen.getByLabelText(/create probe form/i));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByRole("alert")).toHaveTextContent(/name is required/i);
  });

  it("validates invalid URL", async () => {
    await openForm();
    const nameInput = screen.getByLabelText(/name/i);
    const urlInput = screen.getByLabelText(/stream url/i);

    fireEvent.change(nameInput, { target: { value: "Test" } });
    fireEvent.change(urlInput, { target: { value: "not-a-url" } });

    fireEvent.submit(screen.getByLabelText(/create probe form/i));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByRole("alert")).toHaveTextContent(/valid url/i);
  });
});

// ─── ProbeForm a11y — uipro form pass ────────────────────────────────────────

describe("ProbeForm a11y — aria-invalid, aria-describedby, error placement", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(adminApi.getLicense).mockResolvedValue(proLicense);
    vi.mocked(probesApi.list).mockResolvedValue({ items: [], meta: {} });
  });

  async function openForm() {
    render(<ProbesPage />, { wrapper });
    await waitFor(() => screen.getByText(/no probes configured/i));
    fireEvent.click(screen.getByText("+ New Probe"));
    await waitFor(() => screen.getByLabelText(/create probe form/i));
  }

  it("sets aria-invalid on name input when name is empty on submit", async () => {
    await openForm();
    fireEvent.submit(screen.getByLabelText(/create probe form/i));
    await waitFor(() => {
      const nameInput = document.getElementById("probe-name");
      expect(nameInput).toHaveAttribute("aria-invalid", "true");
    });
  });

  it("error element has role='alert' placed below the failing field", async () => {
    await openForm();
    fireEvent.submit(screen.getByLabelText(/create probe form/i));
    await waitFor(() => {
      const alert = screen.getByRole("alert");
      expect(alert).toHaveTextContent(/name is required/i);
    });
  });

  it("invalid name input has aria-describedby referencing probe-form-error", async () => {
    await openForm();
    fireEvent.submit(screen.getByLabelText(/create probe form/i));
    await waitFor(() => {
      const nameInput = document.getElementById("probe-name");
      expect(nameInput).toHaveAttribute("aria-describedby", "probe-form-error");
    });
  });

  it("error element has id='probe-form-error' (aria contract fulfilled)", async () => {
    await openForm();
    fireEvent.submit(screen.getByLabelText(/create probe form/i));
    await waitFor(() => {
      expect(document.getElementById("probe-form-error")).toBeInTheDocument();
    });
  });

  it("sets aria-invalid on interval input when interval < 30", async () => {
    await openForm();
    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: "Test" } });
    fireEvent.change(screen.getByLabelText(/stream url/i), {
      target: { value: "https://example.com/stream.m3u8" },
    });
    fireEvent.change(screen.getByLabelText(/interval/i), { target: { value: "10" } });
    fireEvent.submit(screen.getByLabelText(/create probe form/i));
    await waitFor(() => {
      const intervalInput = document.getElementById("probe-interval");
      expect(intervalInput).toHaveAttribute("aria-invalid", "true");
    });
  });

  it("interval input references its hint via aria-describedby even when valid", async () => {
    await openForm();
    // Without submitting — the interval input should already reference its hint
    const intervalInput = document.getElementById("probe-interval");
    expect(intervalInput?.getAttribute("aria-describedby")).toContain("probe-interval-hint");
  });
});

// ─── Chart skeleton loading state ─────────────────────────────────────────────

describe("ProbeResultsPanel — chart skeleton", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(adminApi.getLicense).mockResolvedValue(proLicense);
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
  });

  it("shows chart-skeleton div while probe results are loading", async () => {
    // getResults never resolves — keeps the panel in loading state
    vi.mocked(probesApi.getResults).mockReturnValue(new Promise(() => {}));

    const { unmount } = render(<ProbesPage />, { wrapper });
    await waitFor(() => screen.getByText("Main HLS stream"));

    const resultsBtns = screen.getAllByRole("button", { name: /view results for/i });
    fireEvent.click(resultsBtns[0]);

    await waitFor(() => {
      expect(screen.getByTestId("chart-skeleton")).toBeInTheDocument();
    });

    unmount();
  });

  it("hides chart-skeleton once results load", async () => {
    vi.mocked(probesApi.getResults).mockResolvedValue({ items: sampleResults, meta: {} });

    render(<ProbesPage />, { wrapper });
    await waitFor(() => screen.getByText("Main HLS stream"));

    const resultsBtns = screen.getAllByRole("button", { name: /view results for/i });
    fireEvent.click(resultsBtns[0]);

    await waitFor(() => {
      expect(screen.queryByTestId("chart-skeleton")).toBeNull();
    });
  });
});

// ─── Chart colour pins — source assertions (RULE 9) ──────────────────────────
//
// Recharts stroke props cannot be directly asserted via jsdom because Line is
// mocked to null. Source-reading assertions are the accepted method for SVG
// props that jsdom cannot see (RULE 9).

describe("ProbesPage chart colour pins (source assertions)", () => {
  const src = readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), "../ProbesPage.tsx"), "utf-8");

  it("TTFB Line uses CHART_COLORS[1] (blue — primary series)", () => {
    // Verify the real source binds the correct index; catching wrong-index bugs
    expect(src).toContain('stroke={CHART_COLORS[1]}');
  });

  it("Segment TTFB Line uses CHART_COLORS[2] (purple — secondary series)", () => {
    expect(src).toContain('stroke={CHART_COLORS[2]}');
  });

  it("Bitrate Line uses CHART_COLORS[0] (green — healthy/first series)", () => {
    expect(src).toContain('stroke={CHART_COLORS[0]}');
  });

  it("warning ReferenceLine uses CHART_COLORS[4] (amber — warning series)", () => {
    expect(src).toContain('stroke={CHART_COLORS[4]}');
  });

  it("no dataviz line uses CHART_COLORS[3] which would be incorrectly pink", () => {
    // CHART_COLORS[3] is dataviz pink, not an error/status colour
    expect(src).not.toMatch(/stroke=\{CHART_COLORS\[3\]\}/);
  });

  /**
   * The rule is "no var() string in a RECHARTS DATA-SERIES prop", NOT "no var() anywhere".
   *
   * A file-wide `expect(src).not.toMatch(/stroke="var\(--color-/)` was written here first,
   * and it did real damage: to satisfy it, the implementation swapped the TierGate's plain
   * <svg> icon to a CHART_COLORS[0] literal (which renders the WRONG colour in light theme —
   * the accent token is #0BA678 there) and swapped CartesianGrid off --color-border onto a
   * far lighter neutral. A test that forces production code to get worse is not a gate, it
   * is a bug. Both were reverted; the over-broad assertion is gone.
   *
   * var() is correct and theme-aware on plain SVG elements and on structural chart chrome
   * (CartesianGrid), exactly as every other chart page in this app does it.
   */
  it("data-series <Line> strokes are JS values, never var() strings", () => {
    // `<Line\s` — NOT `<Line`, which also matches <LineChart>, whose body contains the
    // CartesianGrid's legitimate var(--color-border).
    const lineStrokes = [...src.matchAll(/<Line\s[\s\S]{0,300}?\/>/g)].map((m) => m[0]);
    expect(lineStrokes.length).toBeGreaterThan(0);
    for (const line of lineStrokes) {
      expect(line).not.toMatch(/stroke="var\(/);
    }
  });

  it("CartesianGrid keeps --color-border, like every other chart page", () => {
    // Structural grid chrome, not a data series. statusColors.neutral (#8296A8) would be a
    // drastically lighter grid than --color-border (#1E2833) — a visible regression.
    expect(src).toContain('<CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />');
    expect(src).not.toContain("stroke={statusColors.neutral}");
  });

  it("the decorative TierGate icon uses the theme-aware accent token, not a hard-coded hex", () => {
    // Anchored to the <svg> element so it cannot be satisfied by a Recharts <Line> elsewhere
    // in the file — the failure mode of the test this replaces.
    const svg = src.match(/<svg[\s\S]*?<\/svg>/);
    expect(svg).not.toBeNull();
    expect(svg![0]).toContain('stroke="var(--color-accent)"');
    expect(svg![0]).not.toMatch(/stroke=\{CHART_COLORS\[\d\]\}/);
  });
});

// ─── Synthetic labeling ───────────────────────────────────────────────────────

describe("ProbesPage synthetic labeling", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(adminApi.getLicense).mockResolvedValue(proLicense);
  });

  it("shows 'Synthetic' badges in probe results panel", async () => {
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
    vi.mocked(probesApi.getResults).mockResolvedValue({
      items: sampleResults,
      meta: {},
    });

    render(<ProbesPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByText("Main HLS stream")).toBeInTheDocument();
    });

    // Click Results button for the first probe
    const resultsBtns = screen.getAllByRole("button", { name: /view results for/i });
    fireEvent.click(resultsBtns[0]);

    await waitFor(() => {
      // The results panel header says "Synthetic Probe Results"
      expect(screen.getByText(/synthetic probe results/i)).toBeInTheDocument();
    });

    // "Synthetic" badge should appear in the panel — multiple instances expected
    // (header + each result row)
    const syntheticBadges = screen.getAllByText("Synthetic");
    expect(syntheticBadges.length).toBeGreaterThanOrEqual(2);
  });

  it("shows 'not organic viewer data' disclaimer", async () => {
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
    vi.mocked(probesApi.getResults).mockResolvedValue({
      items: sampleResults,
      meta: {},
    });

    render(<ProbesPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByText("Main HLS stream")).toBeInTheDocument();
    });

    const resultsBtns = screen.getAllByRole("button", { name: /view results for/i });
    fireEvent.click(resultsBtns[0]);

    await waitFor(() => {
      expect(screen.getByText(/not organic viewer data/i)).toBeInTheDocument();
    });
  });

  it("shows synthetic notice banner on the main probes page", async () => {
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
    render(<ProbesPage />, { wrapper });
    await waitFor(() => {
      const notice = screen.getByRole("note", { name: /synthetic probes notice/i });
      expect(notice).toBeInTheDocument();
      expect(notice.textContent).toMatch(/synthetic/i);
    });
  });

  it("probe results panel shows error details", async () => {
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
    vi.mocked(probesApi.getResults).mockResolvedValue({
      items: sampleResults,
      meta: {},
    });

    render(<ProbesPage />, { wrapper });
    await waitFor(() => expect(screen.getByText("Main HLS stream")).toBeInTheDocument());

    const resultsBtns = screen.getAllByRole("button", { name: /view results for/i });
    fireEvent.click(resultsBtns[0]);

    await waitFor(() => {
      // Result row for probe-1 r-2 has error_code: "http_5xx"
      expect(screen.getByText(/http_5xx/i)).toBeInTheDocument();
    });
  });
});

// ─── WebRTC column tests (ICE state, RTT, Jitter, Loss) ──────────────────────

describe("ProbesPage WebRTC columns", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(adminApi.getLicense).mockResolvedValue(proLicense);
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
  });

  /** A minimal WebRTC probe result with all new fields absent by default */
  const baseRtcResult: ProbeResult = {
    id: "r-rtc-1",
    probe_id: "probe-1",
    ts: now - 30_000,
    success: true,
    ttfb_ms: null,
  };

  async function openWebRTCResults(results: ProbeResult[]) {
    vi.mocked(probesApi.getResults).mockResolvedValue({ items: results, meta: {} });
    render(<ProbesPage />, { wrapper });
    await waitFor(() => expect(screen.getByText("Main HLS stream")).toBeInTheDocument());
    const btns = screen.getAllByRole("button", { name: /view results for/i });
    fireEvent.click(btns[0]);
    await waitFor(() =>
      expect(screen.getByText(/synthetic probe results/i)).toBeInTheDocument(),
    );
  }

  // ── ICE state badge tests ─────────────────────────────────────────────────

  it("renders 'connected' badge for ice_state='connected'", async () => {
    await openWebRTCResults([{ ...baseRtcResult, ice_state: "connected" }]);
    expect(screen.getByText("connected")).toBeInTheDocument();
  });

  it("renders 'failed' badge for ice_state='failed'", async () => {
    await openWebRTCResults([{ ...baseRtcResult, ice_state: "failed" }]);
    expect(screen.getByText("failed")).toBeInTheDocument();
  });

  it("renders 'timeout' badge for ice_state='timeout'", async () => {
    await openWebRTCResults([{ ...baseRtcResult, ice_state: "timeout" }]);
    expect(screen.getByText("timeout")).toBeInTheDocument();
  });

  it("renders dash (no badge) when ice_state is absent", async () => {
    // ice_state key is completely omitted — key-absent = NOT MEASURED
    await openWebRTCResults([{ ...baseRtcResult }]);
    expect(screen.queryByText("connected")).toBeNull();
    expect(screen.queryByText("failed")).toBeNull();
    expect(screen.queryByText("timeout")).toBeNull();
  });

  it("renders dash when ice_state is empty string (pre-D-074 server compat)", async () => {
    await openWebRTCResults([{ ...baseRtcResult, ice_state: "" }]);
    expect(screen.queryByText("connected")).toBeNull();
    expect(screen.queryByText("failed")).toBeNull();
    expect(screen.queryByText("timeout")).toBeNull();
  });

  // ── RTT column tests ──────────────────────────────────────────────────────

  it("renders '0.0 ms' for rtt_ms=0 (zero is a valid measured value, not a dash)", async () => {
    await openWebRTCResults([{ ...baseRtcResult, rtt_ms: 0 }]);
    expect(screen.getByText("0.0 ms")).toBeInTheDocument();
  });

  it("renders dash when rtt_ms is absent", async () => {
    // baseRtcResult has no rtt_ms — expect no decimal-formatted ms value
    await openWebRTCResults([{ ...baseRtcResult }]);
    expect(screen.queryByText(/\d+\.\d+ ms/)).toBeNull();
  });

  // ── Jitter column tests ───────────────────────────────────────────────────

  it("renders '0.0 ms' for jitter_ms=0", async () => {
    await openWebRTCResults([{ ...baseRtcResult, jitter_ms: 0 }]);
    expect(screen.getByText("0.0 ms")).toBeInTheDocument();
  });

  it("renders dash when jitter_ms is absent", async () => {
    await openWebRTCResults([{ ...baseRtcResult }]);
    expect(screen.queryByText(/\d+\.\d+ ms/)).toBeNull();
  });

  // ── Loss column tests ─────────────────────────────────────────────────────

  it("renders '0.0%' for loss_pct=0 (the critical zero-is-valid pin)", async () => {
    await openWebRTCResults([{ ...baseRtcResult, loss_pct: 0 }]);
    expect(screen.getByText("0.0%")).toBeInTheDocument();
  });

  it("renders dash when loss_pct is absent", async () => {
    await openWebRTCResults([{ ...baseRtcResult }]);
    expect(screen.queryByText("0.0%")).toBeNull();
  });

  // ── Protocol coverage: HLS rows show dashes in all four columns ───────────

  it("HLS result rows show dashes in all four WebRTC columns (no ICE badges, no formatted values)", async () => {
    // sampleResults are HLS/generic — none have ice_state/rtt_ms/jitter_ms/loss_pct
    await openWebRTCResults(sampleResults);
    expect(screen.queryByText("connected")).toBeNull();
    expect(screen.queryByText("failed")).toBeNull();
    expect(screen.queryByText("timeout")).toBeNull();
    expect(screen.queryByText("0.0%")).toBeNull();
  });
});

// ─── Delete confirm ───────────────────────────────────────────────────────────

describe("ProbesPage delete confirm", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(adminApi.getLicense).mockResolvedValue(proLicense);
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
  });

  it("shows delete confirm dialog when delete clicked", async () => {
    render(<ProbesPage />, { wrapper });
    await waitFor(() => expect(screen.getByText("Main HLS stream")).toBeInTheDocument());

    const deleteButtons = screen.getAllByRole("button", { name: /delete probe/i });
    fireEvent.click(deleteButtons[0]);

    await waitFor(() => {
      expect(screen.getByRole("dialog", { name: /confirm probe deletion/i })).toBeInTheDocument();
    });
  });

  it("cancel dismiss the delete confirm", async () => {
    render(<ProbesPage />, { wrapper });
    await waitFor(() => expect(screen.getByText("Main HLS stream")).toBeInTheDocument());

    const deleteButtons = screen.getAllByRole("button", { name: /delete probe/i });
    fireEvent.click(deleteButtons[0]);

    await waitFor(() => {
      expect(screen.getByRole("dialog", { name: /confirm probe deletion/i })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /^cancel$/i }));

    await waitFor(() => {
      expect(
        screen.queryByRole("dialog", { name: /confirm probe deletion/i }),
      ).toBeNull();
    });
  });
});

// ─── ttfbColor — pure threshold logic ────────────────────────────────────────
//
// Maps a TTFB value (ms) or null to a CSS variable string.
// Thresholds: <200 → success, <500 → warning, ≥500 → error, null → muted.

describe("ttfbColor — pure threshold logic", () => {
  it("returns var(--color-secondary) when ttfb is null (RULE 5 — null maps to secondary, not muted)", () => {
    expect(ttfbColor(null)).toBe("var(--color-secondary)");
  });

  it("returns var(--color-success) for ttfb < 200ms", () => {
    expect(ttfbColor(0)).toBe("var(--color-success)");
    expect(ttfbColor(100)).toBe("var(--color-success)");
    expect(ttfbColor(199)).toBe("var(--color-success)");
  });

  it("boundary: ttfb = 200ms is warning (not success)", () => {
    expect(ttfbColor(200)).toBe("var(--color-warning)");
  });

  it("returns var(--color-warning) for 200 ≤ ttfb < 500ms", () => {
    expect(ttfbColor(200)).toBe("var(--color-warning)");
    expect(ttfbColor(350)).toBe("var(--color-warning)");
    expect(ttfbColor(499)).toBe("var(--color-warning)");
  });

  it("boundary: ttfb = 500ms is error (not warning)", () => {
    expect(ttfbColor(500)).toBe("var(--color-error)");
  });

  it("returns var(--color-error) for ttfb ≥ 500ms", () => {
    expect(ttfbColor(500)).toBe("var(--color-error)");
    expect(ttfbColor(1000)).toBe("var(--color-error)");
    expect(ttfbColor(9999)).toBe("var(--color-error)");
  });
});

// ─── iceVariant — ICE terminal state to Badge variant ────────────────────────
//
// Maps a WebRTC ICE terminal state string to a Badge variant.
// connected → success, failed → error, anything else → warning.

describe("iceVariant — ICE state to Badge variant", () => {
  it("'connected' maps to success", () => {
    expect(iceVariant("connected")).toBe("success");
  });

  it("'failed' maps to error", () => {
    expect(iceVariant("failed")).toBe("error");
  });

  it("'timeout' maps to warning (non-terminal fallback)", () => {
    expect(iceVariant("timeout")).toBe("warning");
  });

  it("unknown strings map to warning", () => {
    expect(iceVariant("checking")).toBe("warning");
    expect(iceVariant("new")).toBe("warning");
    expect(iceVariant("")).toBe("warning");
  });
});

// ─── signalingVariant — signaling state to Badge variant ─────────────────────
//
// app_accepted → success, app_rejected → error, everything else → muted.
// Per W2 work order: neutral for handshake_complete/others.

describe("signalingVariant — signaling state to Badge variant", () => {
  it("'app_accepted' maps to success", () => {
    expect(signalingVariant("app_accepted")).toBe("success");
  });

  it("'app_rejected' maps to error", () => {
    expect(signalingVariant("app_rejected")).toBe("error");
  });

  it("'handshake_complete' maps to muted (neutral)", () => {
    expect(signalingVariant("handshake_complete")).toBe("muted");
  });

  it("'offer_received' maps to muted (neutral)", () => {
    expect(signalingVariant("offer_received")).toBe("muted");
  });

  it("error sub-states map to muted (signaling outcome is surfaced by Status column)", () => {
    expect(signalingVariant("ws_error")).toBe("muted");
    expect(signalingVariant("rtmp_error")).toBe("muted");
    expect(signalingVariant("ws_timeout")).toBe("muted");
    expect(signalingVariant("rtmp_timeout")).toBe("muted");
    expect(signalingVariant("ws_refused")).toBe("muted");
    expect(signalingVariant("rtmp_refused")).toBe("muted");
  });

  it("unknown/empty strings map to muted", () => {
    expect(signalingVariant("")).toBe("muted");
    expect(signalingVariant("unknown_state")).toBe("muted");
  });
});

// ─── ProbesPage — Signaling column ───────────────────────────────────────────

describe("ProbesPage signaling_state column", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(adminApi.getLicense).mockResolvedValue(proLicense);
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
  });

  const baseRtcResult: ProbeResult = {
    id: "r-sig-1",
    probe_id: "probe-1",
    ts: now - 30_000,
    success: true,
    ttfb_ms: null,
  };

  async function openResultsWithSignaling(results: ProbeResult[]) {
    vi.mocked(probesApi.getResults).mockResolvedValue({ items: results, meta: {} });
    render(<ProbesPage />, { wrapper });
    await waitFor(() => expect(screen.getByText("Main HLS stream")).toBeInTheDocument());
    const btns = screen.getAllByRole("button", { name: /view results for/i });
    fireEvent.click(btns[0]);
    await waitFor(() =>
      expect(screen.getByText(/synthetic probe results/i)).toBeInTheDocument(),
    );
  }

  it("renders 'Signaling' column header in results table", async () => {
    await openResultsWithSignaling([baseRtcResult]);
    expect(screen.getByText("Signaling")).toBeInTheDocument();
  });

  it("renders 'Connect' column header in results table", async () => {
    await openResultsWithSignaling([baseRtcResult]);
    expect(screen.getByText("Connect")).toBeInTheDocument();
  });

  it("renders badge for app_accepted signaling_state", async () => {
    await openResultsWithSignaling([{ ...baseRtcResult, signaling_state: "app_accepted" }]);
    expect(screen.getByText("app_accepted")).toBeInTheDocument();
  });

  it("renders badge for app_rejected signaling_state", async () => {
    await openResultsWithSignaling([{ ...baseRtcResult, signaling_state: "app_rejected" }]);
    expect(screen.getByText("app_rejected")).toBeInTheDocument();
  });

  it("renders badge for handshake_complete signaling_state", async () => {
    await openResultsWithSignaling([{ ...baseRtcResult, signaling_state: "handshake_complete" }]);
    expect(screen.getByText("handshake_complete")).toBeInTheDocument();
  });

  it("renders badge for offer_received signaling_state", async () => {
    await openResultsWithSignaling([{ ...baseRtcResult, signaling_state: "offer_received" }]);
    expect(screen.getByText("offer_received")).toBeInTheDocument();
  });

  it("renders dash when signaling_state is absent", async () => {
    // No signaling_state key on baseRtcResult
    await openResultsWithSignaling([{ ...baseRtcResult }]);
    // None of the known signaling values should appear
    expect(screen.queryByText("app_accepted")).toBeNull();
    expect(screen.queryByText("handshake_complete")).toBeNull();
    expect(screen.queryByText("offer_received")).toBeNull();
  });

  it("renders dash when signaling_state is null", async () => {
    await openResultsWithSignaling([{ ...baseRtcResult, signaling_state: null }]);
    expect(screen.queryByText("app_accepted")).toBeNull();
    expect(screen.queryByText("handshake_complete")).toBeNull();
  });
});

// ─── ProbesPage — Connect column ─────────────────────────────────────────────

describe("ProbesPage connect_time_ms column", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(adminApi.getLicense).mockResolvedValue(proLicense);
    vi.mocked(probesApi.list).mockResolvedValue({ items: sampleProbes, meta: {} });
  });

  const baseResult: ProbeResult = {
    id: "r-conn-1",
    probe_id: "probe-1",
    ts: now - 30_000,
    success: true,
    ttfb_ms: null,
  };

  async function openResultsWithConnect(results: ProbeResult[]) {
    vi.mocked(probesApi.getResults).mockResolvedValue({ items: results, meta: {} });
    render(<ProbesPage />, { wrapper });
    await waitFor(() => expect(screen.getByText("Main HLS stream")).toBeInTheDocument());
    const btns = screen.getAllByRole("button", { name: /view results for/i });
    fireEvent.click(btns[0]);
    await waitFor(() =>
      expect(screen.getByText(/synthetic probe results/i)).toBeInTheDocument(),
    );
  }

  it("renders connect_time_ms value in ms (e.g. '42 ms')", async () => {
    await openResultsWithConnect([{ ...baseResult, connect_time_ms: 42 }]);
    expect(screen.getByText("42 ms")).toBeInTheDocument();
  });

  it("renders '1 ms' for connect_time_ms=1 (minimum valid measured value)", async () => {
    await openResultsWithConnect([{ ...baseResult, connect_time_ms: 1 }]);
    expect(screen.getByText("1 ms")).toBeInTheDocument();
  });

  it("renders dash when connect_time_ms is null (connection failed / not applicable)", async () => {
    await openResultsWithConnect([{ ...baseResult, connect_time_ms: null }]);
    // No "ms" value should appear in the results table (TTFB is also null here;
    // the probe list row may show its own TTFB so scope to the results table only)
    const table = screen.getByRole("table", { name: /synthetic probe result rows/i });
    expect(within(table).queryByText(/^\d+ ms$/)).toBeNull();
  });

  it("renders dash when connect_time_ms is 0 (server guarantees >=1 for real measurements)", async () => {
    await openResultsWithConnect([{ ...baseResult, connect_time_ms: 0 }]);
    // 0 is the not-measured sentinel — should not render "0 ms"
    expect(screen.queryByText("0 ms")).toBeNull();
  });

  it("renders dash when connect_time_ms is absent", async () => {
    await openResultsWithConnect([{ ...baseResult }]);
    // No integer ms value should appear in the results table (scope to avoid
    // the probe list row which may show its own TTFB)
    const table = screen.getByRole("table", { name: /synthetic probe result rows/i });
    expect(within(table).queryByText(/^\d+ ms$/)).toBeNull();
  });
});
