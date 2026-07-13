/**
 * F10 — Synthetic Probes (/probes)
 *
 * CRUD UI: list (name, url, protocol, interval, enabled, last result success/TTFB),
 * create/edit form (validate interval ≥ 30, protocol enum, URL), delete with confirm.
 * Per-probe results view: recent probe_results with success/TTFB/bitrate/error timeline.
 *
 * SYNTHETIC LABELING (the PRD F10 acceptance criterion):
 * All probe results are CLEARLY marked as "Synthetic" — labeled section header, badge
 * on every result row, and a notice when results appear near QoE data. Probe results
 * are NEVER silently mixed into organic beacon charts.
 *
 * Gate-aware: Pro+ upsell when not entitled (Free tier blocked).
 */
import { useState, useEffect, useCallback } from "react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
  ReferenceLine,
} from "recharts";
import { probesApi, adminApi, ApiError } from "@/api/client";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";
import { EmptyState } from "@/components/EmptyState";
import { Badge } from "@/components/Badge";
import { useToast } from "@/components/Toast";
import type { Probe, ProbeWrite, ProbeResult, LicenseInfo } from "@/lib/api/types";

// ─── Tier gate ────────────────────────────────────────────────────────────────

interface TierUpsellProps {
  tier: string;
}

function TierUpsell({ tier }: TierUpsellProps) {
  return (
    <div
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: 8,
        padding: "3rem 2rem",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        gap: 16,
        textAlign: "center",
      }}
    >
      <svg
        width="48"
        height="48"
        viewBox="0 0 24 24"
        fill="none"
        stroke="var(--color-accent)"
        strokeWidth="1.5"
        aria-hidden
      >
        <path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22" />
      </svg>
      <div>
        <h2 style={{ margin: "0 0 8px", fontSize: 18, fontWeight: 700 }}>
          Synthetic Probes requires Pro tier
        </h2>
        <p
          style={{
            margin: 0,
            fontSize: 14,
            color: "var(--color-muted)",
            maxWidth: 420,
          }}
        >
          You are currently on the <strong>{tier}</strong> plan. Upgrade to Pro or
          Enterprise to create synthetic stream probes and monitor playback health
          from outside your infrastructure.
        </p>
      </div>
      <a
        href="/settings#license"
        style={{
          display: "inline-block",
          background: "var(--color-accent)",
          color: "var(--color-on-signal)",
          borderRadius: 6,
          padding: "10px 20px",
          fontSize: 13,
          fontWeight: 600,
          textDecoration: "none",
        }}
      >
        Upgrade License
      </a>
    </div>
  );
}

// ─── Synthetic label badge ────────────────────────────────────────────────────

function SyntheticBadge() {
  return (
    <span
      style={{
        display: "inline-block",
        background: "rgba(88,166,255,0.1)",
        color: "var(--color-info)",
        border: "1px solid rgba(88,166,255,0.25)",
        borderRadius: 4,
        padding: "2px 8px",
        fontSize: 10,
        fontWeight: 700,
        letterSpacing: "0.08em",
        textTransform: "uppercase",
      }}
      title="Data from synthetic probes — not organic viewer beacons"
    >
      Synthetic
    </span>
  );
}

// ─── Probe form ───────────────────────────────────────────────────────────────

const PROTOCOL_OPTIONS: Array<ProbeWrite["protocol"]> = ["hls", "webrtc", "rtmp", "dash"];

interface ProbeFormData {
  name: string;
  url: string;
  protocol: NonNullable<ProbeWrite["protocol"]>;
  interval_s: string; // string for input binding
  timeout_s: string;
  enabled: boolean;
}

const defaultFormData: ProbeFormData = {
  name: "",
  url: "",
  protocol: "hls",
  interval_s: "60",
  timeout_s: "10",
  enabled: true,
};

function probeToForm(probe: Probe): ProbeFormData {
  return {
    name: probe.name,
    url: probe.url,
    protocol: probe.protocol ?? "hls",
    interval_s: String(probe.interval_s),
    timeout_s: String(probe.timeout_s),
    enabled: probe.enabled,
  };
}

function validateProbeForm(data: ProbeFormData): string | null {
  if (!data.name.trim()) return "Name is required";
  if (!data.url.trim()) return "URL is required";
  try {
    new URL(data.url.trim());
  } catch {
    return "URL must be a valid URL (include http:// or rtmp://)";
  }
  const interval = Number(data.interval_s);
  if (!Number.isInteger(interval) || interval < 30) {
    return "Interval must be an integer ≥ 30 seconds";
  }
  const timeout = Number(data.timeout_s);
  if (!Number.isInteger(timeout) || timeout < 1) {
    return "Timeout must be a positive integer";
  }
  return null;
}

interface ProbeFormProps {
  initial?: Probe;
  onSave: (data: ProbeWrite) => Promise<void>;
  onCancel: () => void;
  saving: boolean;
}

function ProbeForm({ initial, onSave, onCancel, saving }: ProbeFormProps) {
  const [form, setForm] = useState<ProbeFormData>(
    initial ? probeToForm(initial) : defaultFormData,
  );
  const [formError, setFormError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const err = validateProbeForm(form);
    if (err) {
      setFormError(err);
      return;
    }
    setFormError(null);
    const body: ProbeWrite = {
      name: form.name.trim(),
      url: form.url.trim(),
      protocol: form.protocol,
      interval_s: Number(form.interval_s),
      timeout_s: Number(form.timeout_s),
      enabled: form.enabled,
    };
    await onSave(body);
  };

  const field = (
    id: string,
    label: string,
    input: React.ReactNode,
    hint?: string,
  ) => (
    <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
      <label
        htmlFor={id}
        style={{ fontSize: 12, fontWeight: 600, color: "var(--color-muted)" }}
      >
        {label}
      </label>
      {input}
      {hint && (
        <span style={{ fontSize: 11, color: "var(--color-muted)" }}>{hint}</span>
      )}
    </div>
  );

  const inputStyle: React.CSSProperties = {
    background: "var(--color-surface-2)",
    border: "1px solid var(--color-border)",
    borderRadius: 4,
    color: "var(--color-text)",
    padding: "6px 10px",
    fontSize: 13,
    width: "100%",
    boxSizing: "border-box",
  };

  return (
    <form
      onSubmit={handleSubmit}
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: 8,
        padding: 20,
        display: "flex",
        flexDirection: "column",
        gap: 14,
        maxWidth: 520,
      }}
      aria-label={initial ? "Edit probe form" : "Create probe form"}
    >
      <h3 style={{ margin: 0, fontSize: 15, fontWeight: 700 }}>
        {initial ? "Edit Probe" : "New Synthetic Probe"}
      </h3>

      {field(
        "probe-name",
        "Name",
        <input
          id="probe-name"
          type="text"
          value={form.name}
          onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
          placeholder="e.g. Main live stream"
          style={inputStyle}
          required
          aria-required="true"
        />,
      )}

      {field(
        "probe-url",
        "Stream URL",
        <input
          id="probe-url"
          type="text"
          value={form.url}
          onChange={(e) => setForm((f) => ({ ...f, url: e.target.value }))}
          placeholder="https://example.com/live/stream.m3u8"
          style={inputStyle}
          required
          aria-required="true"
        />,
        "HLS, RTMP, DASH, or WebRTC URL",
      )}

      {field(
        "probe-protocol",
        "Protocol",
        <select
          id="probe-protocol"
          value={form.protocol}
          onChange={(e) =>
            setForm((f) => ({
              ...f,
              protocol: e.target.value as NonNullable<ProbeWrite["protocol"]>,
            }))
          }
          style={inputStyle}
        >
          {PROTOCOL_OPTIONS.map((p) => (
            <option key={p} value={p}>
              {p?.toUpperCase() ?? "HLS"}
            </option>
          ))}
        </select>,
      )}

      {field(
        "probe-interval",
        "Interval (seconds)",
        <input
          id="probe-interval"
          type="number"
          min={30}
          value={form.interval_s}
          onChange={(e) => setForm((f) => ({ ...f, interval_s: e.target.value }))}
          style={inputStyle}
          aria-describedby="probe-interval-hint"
        />,
        "Minimum 30 seconds",
      )}

      {field(
        "probe-timeout",
        "Timeout (seconds)",
        <input
          id="probe-timeout"
          type="number"
          min={1}
          value={form.timeout_s}
          onChange={(e) => setForm((f) => ({ ...f, timeout_s: e.target.value }))}
          style={inputStyle}
        />,
      )}

      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
        <input
          id="probe-enabled"
          type="checkbox"
          checked={form.enabled}
          onChange={(e) => setForm((f) => ({ ...f, enabled: e.target.checked }))}
          style={{ width: 15, height: 15 }}
        />
        <label htmlFor="probe-enabled" style={{ fontSize: 13 }}>
          Enabled (start probing immediately)
        </label>
      </div>

      {formError && (
        <div
          role="alert"
          style={{
            background: "var(--color-error-bg, rgba(255,92,104,0.1))",
            border: "1px solid var(--color-error, #FF5C68)",
            borderRadius: 4,
            padding: "8px 12px",
            fontSize: 13,
            color: "var(--color-error, #FF5C68)",
          }}
        >
          {formError}
        </div>
      )}

      <div style={{ display: "flex", gap: 10, marginTop: 4 }}>
        <button
          type="submit"
          disabled={saving}
          style={{
            background: "var(--color-accent)",
            color: "var(--color-on-signal)",
            border: "none",
            borderRadius: 6,
            padding: "8px 18px",
            fontSize: 13,
            fontWeight: 600,
            cursor: saving ? "not-allowed" : "pointer",
            opacity: saving ? 0.7 : 1,
          }}
        >
          {saving ? "Saving…" : initial ? "Save Changes" : "Create Probe"}
        </button>
        <button
          type="button"
          onClick={onCancel}
          disabled={saving}
          style={{
            background: "none",
            color: "var(--color-muted)",
            border: "1px solid var(--color-border)",
            borderRadius: 6,
            padding: "8px 18px",
            fontSize: 13,
            cursor: "pointer",
          }}
        >
          Cancel
        </button>
      </div>
    </form>
  );
}

// ─── Probe results panel (synthetic labeling — the F10 acceptance) ────────────

export function ttfbColor(ttfb: number | null): string {
  if (ttfb == null) return "var(--color-muted)";
  if (ttfb < 200) return "var(--color-success)";
  if (ttfb < 500) return "var(--color-warning)";
  return "var(--color-error)";
}

/** Maps an ICE terminal state string to the Badge variant — modeled on FleetPage statusVariant. */
export function iceVariant(state: string): "success" | "error" | "warning" {
  if (state === "connected") return "success";
  if (state === "failed") return "error";
  return "warning"; // "timeout"
}

/**
 * Maps a signaling_state string to the Badge variant.
 *
 * Colour key (W2):
 *   app_accepted  → success  (stream accepted by the AMS application)
 *   app_rejected  → error    (stream rejected by the AMS application)
 *   everything else → muted  (handshake_complete, offer_received, error substates,
 *                             unknown) — outcome is already surfaced by the Status column.
 */
export function signalingVariant(state: string): "success" | "error" | "muted" {
  if (state === "app_accepted") return "success";
  if (state === "app_rejected") return "error";
  return "muted";
}

interface ProbeResultsChartData {
  ts: number;
  ttfb_ms: number | null;
  segment_ttfb_ms: number | null;
  bitrate_kbps: number | undefined;
  success: boolean;
}

interface ProbeResultsPanelProps {
  probe: Probe;
  onClose: () => void;
}

function ProbeResultsPanel({ probe, onClose }: ProbeResultsPanelProps) {
  const [results, setResults] = useState<ProbeResult[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchResults = useCallback(() => {
    setLoading(true);
    setError(null);
    probesApi
      .getResults(probe.id, { limit: 50 })
      .then((res) => {
        setResults(res.items);
        setLoading(false);
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : "Failed to load results");
        setLoading(false);
      });
  }, [probe.id]);

  useEffect(() => {
    fetchResults();
  }, [fetchResults]);

  const chartData: ProbeResultsChartData[] = results.map((r) => ({
    ts: r.ts,
    ttfb_ms: r.ttfb_ms,
    segment_ttfb_ms: r.segment_ttfb_ms ?? null,
    bitrate_kbps: r.bitrate_kbps,
    success: r.success,
  }));

  const formatTsShort = (ts: number) =>
    new Date(ts).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

  return (
    <div
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: 8,
        overflow: "hidden",
        marginTop: 8,
      }}
      aria-label={`Probe results for ${probe.name}`}
    >
      {/* ── SYNTHETIC LABEL — PRD F10 acceptance criterion ─────────────────── */}
      <div
        style={{
          background: "var(--color-surface-2)",
          borderBottom: "1px solid var(--color-border-strong)",
          padding: "10px 16px",
          display: "flex",
          alignItems: "center",
          gap: 10,
        }}
      >
        <SyntheticBadge />
        <span style={{ fontSize: 13, fontWeight: 600, color: "var(--color-info)" }}>
          Synthetic Probe Results
        </span>
        <span style={{ fontSize: 12, color: "var(--color-muted)", marginLeft: 4 }}>
          — not organic viewer data
        </span>
        <div style={{ flex: 1 }} />
        <span
          style={{ fontSize: 12, color: "var(--color-muted)", fontFamily: "var(--font-mono)" }}
        >
          {probe.name} · {probe.url}
        </span>
        <button
          onClick={onClose}
          aria-label="Close results panel"
          style={{
            background: "none",
            border: "none",
            color: "var(--color-muted)",
            cursor: "pointer",
            padding: "2px 6px",
            fontSize: 16,
          }}
        >
          ×
        </button>
      </div>

      <div style={{ padding: 16 }}>
        {loading && <LoadingSpinner label="Loading synthetic probe results…" />}
        {error && <ErrorBanner message={error} onRetry={fetchResults} />}

        {!loading && !error && results.length === 0 && (
          <EmptyState
            title="No results yet"
            description="The probe runner has not executed this probe yet. Results appear after the first probe interval."
          />
        )}

        {!loading && results.length > 0 && (
          <>
            {/* TTFB timeline */}
            <div style={{ marginBottom: 20 }}>
              <div
                style={{
                  fontSize: 12,
                  fontWeight: 600,
                  color: "var(--color-muted)",
                  marginBottom: 8,
                  textTransform: "uppercase",
                  letterSpacing: "0.06em",
                }}
              >
                Time to First Byte (ms)
              </div>
              <ResponsiveContainer width="100%" height={120}>
                <LineChart data={chartData} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
                  <XAxis
                    dataKey="ts"
                    tickFormatter={formatTsShort}
                    tick={{ fontSize: 10, fill: "var(--color-muted)" }}
                    axisLine={false}
                    tickLine={false}
                  />
                  <YAxis
                    tick={{ fontSize: 10, fill: "var(--color-muted)" }}
                    axisLine={false}
                    tickLine={false}
                    width={40}
                  />
                  <Tooltip
                    formatter={(v: unknown) => [
                      typeof v === "number" ? `${v} ms` : "—",
                    ]}
                    labelFormatter={(ts: unknown) =>
                      typeof ts === "number" ? new Date(ts).toLocaleString() : String(ts)
                    }
                    contentStyle={{
                      background: "var(--color-surface)",
                      border: "1px solid var(--color-border)",
                      fontSize: 12,
                    }}
                  />
                  <Legend wrapperStyle={{ fontSize: 11, color: "var(--color-muted)" }} />
                  {/* 500ms warning threshold */}
                  <ReferenceLine y={500} stroke="#FFB224" strokeDasharray="4 2" />
                  <Line
                    type="monotone"
                    dataKey="ttfb_ms"
                    stroke="#58A6FF"
                    strokeWidth={2}
                    dot={false}
                    connectNulls={false}
                    name="TTFB (ms)"
                  />
                  <Line
                    type="monotone"
                    dataKey="segment_ttfb_ms"
                    stroke="#A78BFA"
                    strokeWidth={2}
                    dot={false}
                    connectNulls={false}
                    name="Segment TTFB (ms)"
                  />
                </LineChart>
              </ResponsiveContainer>
            </div>

            {/* Bitrate timeline */}
            {chartData.some((d) => d.bitrate_kbps != null) && (
              <div style={{ marginBottom: 20 }}>
                <div
                  style={{
                    fontSize: 12,
                    fontWeight: 600,
                    color: "var(--color-muted)",
                    marginBottom: 8,
                    textTransform: "uppercase",
                    letterSpacing: "0.06em",
                  }}
                >
                  Measured Bitrate (kbps)
                </div>
                <ResponsiveContainer width="100%" height={100}>
                  <LineChart
                    data={chartData}
                    margin={{ top: 4, right: 8, left: 0, bottom: 0 }}
                  >
                    <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
                    <XAxis
                      dataKey="ts"
                      tickFormatter={formatTsShort}
                      tick={{ fontSize: 10, fill: "var(--color-muted)" }}
                      axisLine={false}
                      tickLine={false}
                    />
                    <YAxis
                      tick={{ fontSize: 10, fill: "var(--color-muted)" }}
                      axisLine={false}
                      tickLine={false}
                      width={40}
                    />
                    <Tooltip
                      formatter={(v: unknown) => [
                        typeof v === "number" ? `${v.toFixed(0)} kbps` : "—",
                        "Bitrate",
                      ]}
                      labelFormatter={(ts: unknown) =>
                        typeof ts === "number" ? new Date(ts).toLocaleString() : String(ts)
                      }
                      contentStyle={{
                        background: "var(--color-surface)",
                        border: "1px solid var(--color-border)",
                        fontSize: 12,
                      }}
                    />
                    <Line
                      type="monotone"
                      dataKey="bitrate_kbps"
                      stroke="#2CE5A7"
                      strokeWidth={2}
                      dot={false}
                      name="Bitrate (kbps)"
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            )}

            {/* Recent results table */}
            <div
              style={{
                fontSize: 12,
                fontWeight: 600,
                color: "var(--color-muted)",
                marginBottom: 8,
                textTransform: "uppercase",
                letterSpacing: "0.06em",
              }}
            >
              Recent Results
            </div>
            <div style={{ overflowX: "auto" }}>
              <table
                style={{ width: "100%", borderCollapse: "collapse", fontSize: 12 }}
                aria-label="Synthetic probe result rows"
              >
                <thead>
                  <tr style={{ background: "var(--color-surface-2)" }}>
                    <th
                      style={{
                        padding: "6px 10px",
                        textAlign: "left",
                        fontWeight: 600,
                        color: "var(--color-muted)",
                      }}
                    >
                      Time
                    </th>
                    <th style={{ padding: "6px 10px", textAlign: "center", fontWeight: 600, color: "var(--color-muted)" }}>
                      Type
                    </th>
                    <th style={{ padding: "6px 10px", textAlign: "center", fontWeight: 600, color: "var(--color-muted)" }}>
                      Status
                    </th>
                    <th style={{ padding: "6px 10px", textAlign: "right", fontWeight: 600, color: "var(--color-muted)" }}>
                      TTFB
                    </th>
                    <th style={{ padding: "6px 10px", textAlign: "right", fontWeight: 600, color: "var(--color-muted)" }}>
                      Segment TTFB
                    </th>
                    <th style={{ padding: "6px 10px", textAlign: "right", fontWeight: 600, color: "var(--color-muted)" }}>
                      Bitrate
                    </th>
                    <th style={{ padding: "6px 10px", textAlign: "center", fontWeight: 600, color: "var(--color-muted)", whiteSpace: "nowrap" }}>
                      Signaling
                    </th>
                    <th style={{ padding: "6px 10px", textAlign: "right", fontWeight: 600, color: "var(--color-muted)", whiteSpace: "nowrap" }}>
                      Connect
                    </th>
                    <th style={{ padding: "6px 10px", textAlign: "center", fontWeight: 600, color: "var(--color-muted)", whiteSpace: "nowrap" }}>
                      ICE State
                    </th>
                    <th style={{ padding: "6px 10px", textAlign: "right", fontWeight: 600, color: "var(--color-muted)", whiteSpace: "nowrap" }}>
                      RTT
                    </th>
                    <th style={{ padding: "6px 10px", textAlign: "right", fontWeight: 600, color: "var(--color-muted)", whiteSpace: "nowrap" }}>
                      Jitter
                    </th>
                    <th style={{ padding: "6px 10px", textAlign: "right", fontWeight: 600, color: "var(--color-muted)", whiteSpace: "nowrap" }}>
                      Loss
                    </th>
                    <th style={{ padding: "6px 10px", textAlign: "left", fontWeight: 600, color: "var(--color-muted)" }}>
                      Error
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {results.slice(0, 20).map((r) => (
                    <tr
                      key={r.id}
                      style={{
                        borderBottom: "1px solid var(--color-border)",
                        background: r.success ? "transparent" : "rgba(255,92,104,0.04)",
                      }}
                    >
                      <td style={{ padding: "6px 10px", color: "var(--color-muted)", whiteSpace: "nowrap" }}>
                        {new Date(r.ts).toLocaleString()}
                      </td>
                      {/* Synthetic label on every result row */}
                      <td style={{ padding: "6px 10px", textAlign: "center" }}>
                        <SyntheticBadge />
                      </td>
                      <td style={{ padding: "6px 10px", textAlign: "center" }}>
                        <Badge
                          label={r.success ? "ok" : "fail"}
                          variant={r.success ? "success" : "error"}
                        />
                      </td>
                      <td
                        style={{
                          padding: "6px 10px",
                          textAlign: "right",
                          color: ttfbColor(r.ttfb_ms),
                          fontFamily: "var(--font-mono)",
                          fontSize: 12,
                        }}
                      >
                        {r.ttfb_ms != null ? `${r.ttfb_ms} ms` : "—"}
                      </td>
                      <td
                        style={{
                          padding: "6px 10px",
                          textAlign: "right",
                          color: ttfbColor(r.segment_ttfb_ms ?? null),
                          fontFamily: "var(--font-mono)",
                          fontSize: 12,
                        }}
                      >
                        {r.segment_ttfb_ms != null ? `${r.segment_ttfb_ms} ms` : "—"}
                      </td>
                      <td
                        style={{
                          padding: "6px 10px",
                          textAlign: "right",
                          color: "var(--color-muted)",
                          fontFamily: "var(--font-mono)",
                          fontSize: 12,
                        }}
                      >
                        {r.bitrate_kbps != null ? `${r.bitrate_kbps.toFixed(0)} kbps` : "—"}
                      </td>
                      {/* Signaling — badge for signaling_state; "—" when absent/null */}
                      <td style={{ padding: "6px 10px", textAlign: "center", whiteSpace: "nowrap" }}>
                        {r.signaling_state != null ? (
                          <Badge label={r.signaling_state} variant={signalingVariant(r.signaling_state)} />
                        ) : (
                          "—"
                        )}
                      </td>
                      {/* Connect — connect_time_ms in ms; 0 and null/absent → "—"
                          (0 is Go zero-value / not-measured sentinel; server guarantees >=1
                          for real connection-time measurements) */}
                      <td
                        style={{
                          padding: "6px 10px",
                          textAlign: "right",
                          color: "var(--color-muted)",
                          fontFamily: "var(--font-mono)",
                          fontSize: 12,
                          whiteSpace: "nowrap",
                        }}
                      >
                        {r.connect_time_ms != null && r.connect_time_ms > 0
                          ? `${r.connect_time_ms} ms`
                          : "—"}
                      </td>
                      <td style={{ padding: "6px 10px", textAlign: "center", whiteSpace: "nowrap" }}>
                        {r.ice_state ? (
                          <Badge label={r.ice_state} variant={iceVariant(r.ice_state)} />
                        ) : (
                          "—"
                        )}
                      </td>
                      <td
                        style={{
                          padding: "6px 10px",
                          textAlign: "right",
                          color: "var(--color-muted)",
                          fontFamily: "var(--font-mono)",
                          fontSize: 12,
                          whiteSpace: "nowrap",
                        }}
                      >
                        {r.rtt_ms != null ? `${r.rtt_ms.toFixed(1)} ms` : "—"}
                      </td>
                      <td
                        style={{
                          padding: "6px 10px",
                          textAlign: "right",
                          color: "var(--color-muted)",
                          fontFamily: "var(--font-mono)",
                          fontSize: 12,
                          whiteSpace: "nowrap",
                        }}
                      >
                        {r.jitter_ms != null ? `${r.jitter_ms.toFixed(1)} ms` : "—"}
                      </td>
                      <td
                        style={{
                          padding: "6px 10px",
                          textAlign: "right",
                          color: "var(--color-muted)",
                          fontFamily: "var(--font-mono)",
                          fontSize: 12,
                          whiteSpace: "nowrap",
                        }}
                      >
                        {r.loss_pct != null ? `${r.loss_pct.toFixed(1)}%` : "—"}
                      </td>
                      <td
                        style={{
                          padding: "6px 10px",
                          color: "var(--color-error)",
                          fontSize: 11,
                          maxWidth: 200,
                          overflow: "hidden",
                          textOverflow: "ellipsis",
                          whiteSpace: "nowrap",
                        }}
                        title={r.error_message ?? ""}
                      >
                        {r.error_code ? `${r.error_code}: ${r.error_message ?? ""}` : "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

// ─── Delete confirm ───────────────────────────────────────────────────────────

interface DeleteConfirmProps {
  probeName: string;
  onConfirm: () => void;
  onCancel: () => void;
  deleting: boolean;
}

function DeleteConfirm({ probeName, onConfirm, onCancel, deleting }: DeleteConfirmProps) {
  return (
    <div
      role="dialog"
      aria-label="Confirm probe deletion"
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-error)",
        borderRadius: 8,
        padding: 16,
        maxWidth: 400,
        display: "flex",
        flexDirection: "column",
        gap: 12,
      }}
    >
      <p style={{ margin: 0, fontSize: 14, color: "var(--color-text)" }}>
        Delete probe <strong>{probeName}</strong>? This will permanently remove the probe
        configuration and stop all future runs. Historical results in ClickHouse are
        retained per the 90-day TTL.
      </p>
      <div style={{ display: "flex", gap: 8 }}>
        <button
          onClick={onConfirm}
          disabled={deleting}
          style={{
            background: "var(--color-error-bg)",
            color: "var(--color-error)",
            border: "1px solid var(--color-error)",
            borderRadius: 6,
            padding: "6px 14px",
            fontSize: 13,
            fontWeight: 600,
            cursor: deleting ? "not-allowed" : "pointer",
            opacity: deleting ? 0.7 : 1,
          }}
        >
          {deleting ? "Deleting…" : "Delete"}
        </button>
        <button
          onClick={onCancel}
          disabled={deleting}
          style={{
            background: "none",
            color: "var(--color-muted)",
            border: "1px solid var(--color-border)",
            borderRadius: 6,
            padding: "6px 14px",
            fontSize: 13,
            cursor: "pointer",
          }}
        >
          Cancel
        </button>
      </div>
    </div>
  );
}

// ─── Probe list row ───────────────────────────────────────────────────────────

interface ProbeRowProps {
  probe: Probe;
  onEdit: (probe: Probe) => void;
  onDelete: (probe: Probe) => void;
  onViewResults: (probe: Probe) => void;
  showingResults: boolean;
}

function ProbeRow({
  probe,
  onEdit,
  onDelete,
  onViewResults,
  showingResults,
}: ProbeRowProps) {
  const lr = probe.last_result;
  return (
    <tr
      style={{
        borderBottom: "1px solid var(--color-border)",
        background: showingResults ? "rgba(88,166,255,0.08)" : "transparent",
      }}
    >
      {/* Name */}
      <td style={{ padding: "10px 12px", fontWeight: 600, fontSize: 13 }}>{probe.name}</td>
      {/* URL */}
      <td
        style={{
          padding: "10px 12px",
          fontSize: 12,
          color: "var(--color-muted)",
          fontFamily: "var(--font-mono)",
          maxWidth: 200,
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap",
        }}
        title={probe.url}
      >
        {probe.url}
      </td>
      {/* Protocol */}
      <td style={{ padding: "10px 12px", fontSize: 12 }}>
        <Badge label={probe.protocol ?? "hls"} variant="info" />
      </td>
      {/* Interval */}
      <td
        style={{
          padding: "10px 12px",
          fontSize: 12,
          color: "var(--color-muted)",
          textAlign: "right",
          fontFamily: "var(--font-mono)",
        }}
      >
        {probe.interval_s}s
      </td>
      {/* Enabled */}
      <td style={{ padding: "10px 12px", textAlign: "center" }}>
        <Badge label={probe.enabled ? "on" : "off"} variant={probe.enabled ? "success" : "muted"} />
      </td>
      {/* Last result */}
      <td style={{ padding: "10px 12px" }}>
        {lr ? (
          <div style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12 }}>
            <Badge label={lr.success ? "ok" : "fail"} variant={lr.success ? "success" : "error"} />
            {lr.ttfb_ms != null && (
              <span
                style={{ color: ttfbColor(lr.ttfb_ms), fontFamily: "var(--font-mono)" }}
              >
                {lr.ttfb_ms} ms
              </span>
            )}
          </div>
        ) : (
          <span style={{ fontSize: 12, color: "var(--color-muted)" }}>No results yet</span>
        )}
      </td>
      {/* Actions */}
      <td style={{ padding: "10px 12px" }}>
        <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
          <button
            onClick={() => onViewResults(probe)}
            style={{
              background: showingResults ? "rgba(88,166,255,0.15)" : "none",
              color: "var(--color-info)",
              border: "1px solid rgba(88,166,255,0.25)",
              borderRadius: 4,
              padding: "3px 9px",
              fontSize: 11,
              cursor: "pointer",
              fontWeight: 600,
            }}
            aria-label={`View results for ${probe.name}`}
            title="View synthetic probe results"
          >
            Results
          </button>
          <button
            onClick={() => onEdit(probe)}
            style={{
              background: "none",
              color: "var(--color-muted)",
              border: "1px solid var(--color-border)",
              borderRadius: 4,
              padding: "3px 9px",
              fontSize: 11,
              cursor: "pointer",
            }}
            aria-label={`Edit probe ${probe.name}`}
          >
            Edit
          </button>
          <button
            onClick={() => onDelete(probe)}
            style={{
              background: "none",
              color: "var(--color-error)",
              border: "1px solid var(--color-error)",
              borderRadius: 4,
              padding: "3px 9px",
              fontSize: 11,
              cursor: "pointer",
            }}
            aria-label={`Delete probe ${probe.name}`}
          >
            Delete
          </button>
        </div>
      </td>
    </tr>
  );
}

// ─── Main page ────────────────────────────────────────────────────────────────

export function ProbesPage() {
  const { toast } = useToast();

  const [license, setLicense] = useState<LicenseInfo | null>(null);
  const [licenseLoading, setLicenseLoading] = useState(true);
  const [licenseError, setLicenseError] = useState<string | null>(null);

  const [probes, setProbes] = useState<Probe[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [showForm, setShowForm] = useState(false);
  const [editingProbe, setEditingProbe] = useState<Probe | null>(null);
  const [saving, setSaving] = useState(false);

  const [deletingProbe, setDeletingProbe] = useState<Probe | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<Probe | null>(null);

  const [selectedProbe, setSelectedProbe] = useState<Probe | null>(null);

  // Load license
  useEffect(() => {
    let cancelled = false;
    setLicenseLoading(true);
    adminApi
      .getLicense()
      .then((lic) => {
        if (!cancelled) {
          setLicense(lic);
          setLicenseLoading(false);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setLicenseError(err instanceof Error ? err.message : "Failed to load license");
          setLicenseLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const fetchProbes = useCallback(() => {
    setLoading(true);
    setError(null);
    probesApi
      .list({ limit: 100 })
      .then((res) => {
        setProbes(res.items);
        setLoading(false);
      })
      .catch((err) => {
        const msg =
          err instanceof ApiError
            ? `API error ${err.status}: ${err.message}`
            : err instanceof Error
              ? err.message
              : "Unknown error";
        setError(msg);
        setLoading(false);
      });
  }, []);

  // Load probes after license confirmed as Pro+
  useEffect(() => {
    if (license && license.tier !== "free") {
      fetchProbes();
    }
  }, [license, fetchProbes]);

  const handleSave = async (body: ProbeWrite) => {
    setSaving(true);
    try {
      if (editingProbe) {
        const updated = await probesApi.update(editingProbe.id, body);
        setProbes((prev) => prev.map((p) => (p.id === updated.id ? updated : p)));
        toast("Probe updated", "success");
      } else {
        const created = await probesApi.create(body);
        setProbes((prev) => [...prev, created]);
        toast("Probe created", "success");
      }
      setShowForm(false);
      setEditingProbe(null);
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? `Save failed: ${err.message}`
          : err instanceof Error
            ? err.message
            : "Save failed";
      toast(msg, "error");
    } finally {
      setSaving(false);
    }
  };

  const handleDeleteConfirm = async () => {
    if (!confirmDelete) return;
    setDeletingProbe(confirmDelete);
    try {
      await probesApi.delete(confirmDelete.id);
      setProbes((prev) => prev.filter((p) => p.id !== confirmDelete.id));
      if (selectedProbe?.id === confirmDelete.id) setSelectedProbe(null);
      toast("Probe deleted", "success");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Delete failed";
      toast(msg, "error");
    } finally {
      setDeletingProbe(null);
      setConfirmDelete(null);
    }
  };

  const handleViewResults = (probe: Probe) => {
    setSelectedProbe((prev) => (prev?.id === probe.id ? null : probe));
  };

  if (licenseLoading) {
    return <LoadingSpinner label="Loading license…" />;
  }

  if (licenseError) {
    return <ErrorBanner message={licenseError} />;
  }

  // Gate: only Pro+
  if (license && license.tier === "free") {
    return (
      <div style={{ maxWidth: 700, margin: "0 auto", paddingTop: 40 }}>
        <h1 style={{ fontSize: 20, fontWeight: 700, margin: "0 0 24px" }}>
          Synthetic Probes
        </h1>
        <TierUpsell tier={license.tier} />
      </div>
    );
  }

  return (
    <div style={{ maxWidth: 1100, margin: "0 auto" }}>
      {/* Page header */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 16,
          marginBottom: 20,
        }}
      >
        <h1 style={{ flex: 1, fontSize: 20, fontWeight: 700, margin: 0 }}>
          Synthetic Probes
        </h1>
        <SyntheticBadge />
        <button
          onClick={fetchProbes}
          disabled={loading}
          style={{
            background: "none",
            color: "var(--color-muted)",
            border: "1px solid var(--color-border)",
            borderRadius: 6,
            padding: "6px 12px",
            fontSize: 12,
            cursor: loading ? "not-allowed" : "pointer",
            opacity: loading ? 0.7 : 1,
          }}
        >
          Refresh
        </button>
        <button
          onClick={() => {
            setEditingProbe(null);
            setShowForm(true);
          }}
          style={{
            background: "var(--color-accent)",
            color: "var(--color-on-signal)",
            border: "none",
            borderRadius: 6,
            padding: "6px 14px",
            fontSize: 13,
            fontWeight: 600,
            cursor: "pointer",
          }}
        >
          + New Probe
        </button>
      </div>

      {/* Probes-are-synthetic notice */}
      <div
        style={{
          background: "rgba(88,166,255,0.08)",
          border: "1px solid rgba(88,166,255,0.25)",
          borderRadius: 6,
          padding: "8px 14px",
          fontSize: 12,
          color: "var(--color-info)",
          marginBottom: 16,
          display: "flex",
          alignItems: "center",
          gap: 8,
        }}
        role="note"
        aria-label="Synthetic probes notice"
      >
        <SyntheticBadge />
        <span>
          Probe results are <strong>synthetic</strong> (generated by the Pulse probe
          runner, not organic viewer beacons). They are always displayed with a
          &ldquo;Synthetic&rdquo; label and kept separate from organic QoE charts.
        </span>
      </div>

      {error && (
        <div style={{ marginBottom: 16 }}>
          <ErrorBanner message={error} onRetry={fetchProbes} />
        </div>
      )}

      {/* Create/edit form */}
      {(showForm || editingProbe) && (
        <div style={{ marginBottom: 20 }}>
          <ProbeForm
            initial={editingProbe ?? undefined}
            onSave={handleSave}
            onCancel={() => {
              setShowForm(false);
              setEditingProbe(null);
            }}
            saving={saving}
          />
        </div>
      )}

      {/* Delete confirmation */}
      {confirmDelete && (
        <div style={{ marginBottom: 16 }}>
          <DeleteConfirm
            probeName={confirmDelete.name}
            onConfirm={handleDeleteConfirm}
            onCancel={() => setConfirmDelete(null)}
            deleting={deletingProbe?.id === confirmDelete.id}
          />
        </div>
      )}

      {loading && !error && (
        <div style={{ marginBottom: 16 }}>
          <LoadingSpinner label="Loading probes…" />
        </div>
      )}

      {!loading && !error && probes.length === 0 && !showForm && (
        <EmptyState
          title="No probes configured"
          description='Create a synthetic probe to monitor your streams from outside your infrastructure. Click "+ New Probe" to get started.'
          action={
            <button
              onClick={() => setShowForm(true)}
              style={{
                background: "var(--color-accent)",
                color: "var(--color-on-signal)",
                border: "none",
                borderRadius: 6,
                padding: "8px 18px",
                fontSize: 13,
                fontWeight: 600,
                cursor: "pointer",
              }}
            >
              Create First Probe
            </button>
          }
        />
      )}

      {probes.length > 0 && (
        <div
          style={{
            background: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            borderRadius: 8,
            overflow: "hidden",
          }}
        >
          <div style={{ overflowX: "auto" }}>
            <table
              style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}
              aria-label="Synthetic probes list"
            >
              <thead>
                <tr style={{ background: "var(--color-surface-2)" }}>
                  {[
                    "Name",
                    "URL",
                    "Protocol",
                    "Interval",
                    "Enabled",
                    "Last Result",
                    "Actions",
                  ].map((h) => (
                    <th
                      key={h}
                      style={{
                        padding: "8px 12px",
                        textAlign: h === "Interval" ? "right" : "left",
                        fontSize: 11,
                        fontWeight: 600,
                        color: "var(--color-muted)",
                        textTransform: "uppercase",
                        letterSpacing: "0.06em",
                        whiteSpace: "nowrap",
                      }}
                    >
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {probes.map((probe) => (
                  <ProbeRow
                    key={probe.id}
                    probe={probe}
                    onEdit={(p) => {
                      setEditingProbe(p);
                      setShowForm(false);
                    }}
                    onDelete={(p) => setConfirmDelete(p)}
                    onViewResults={handleViewResults}
                    showingResults={selectedProbe?.id === probe.id}
                  />
                ))}
              </tbody>
            </table>
          </div>

          {/* Probe results panel — synthetic labeled */}
          {selectedProbe && (
            <div style={{ padding: "0 16px 16px" }}>
              <ProbeResultsPanel
                probe={selectedProbe}
                onClose={() => setSelectedProbe(null)}
              />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
