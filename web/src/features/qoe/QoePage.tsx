/**
 * F3 — Viewer QoE Dashboard (/qoe)
 *
 * Summary cards: startup p50/p95, rebuffer ratio, error rate.
 * Bitrate timeline (recharts LineChart).
 * Slice controls: stream/app/geo/device/time.
 * Honest empty state when no beacons yet (links to SDK setup docs).
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
} from "recharts";
import { qoeApi, ApiError } from "@/api/client";
import { CHART_COLORS } from "@/lib/chartColors";
import { DateRangePicker, defaultDateRange } from "@/features/analytics/DateRangePicker";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";
import { EmptyState } from "@/components/EmptyState";
import { Badge } from "@/components/Badge";
import type { QoeSummaryResponse } from "@/lib/api/types";

export function QoePage() {
  const [range, setRange] = useState(defaultDateRange);
  const [streamFilter, setStreamFilter] = useState("");
  const [appFilter, setAppFilter] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [data, setData] = useState<QoeSummaryResponse | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await qoeApi.getSummary({
        from: range.from,
        to: range.to,
        stream_id: streamFilter || undefined,
        app: appFilter || undefined,
      });
      setData(result);
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to load QoE data";
      setError(msg);
    } finally {
      setLoading(false);
    }
  }, [range, streamFilter, appFilter]);

  useEffect(() => {
    void load();
  }, [load]);

  const totals = data?.totals;
  const hasData = (data?.bitrate_timeline ?? []).length > 0 || totals != null;

  const chartData = (data?.bitrate_timeline ?? []).map((b) => ({
    ts: new Date(b.ts).toLocaleTimeString(),
    p50: b.bitrate_kbps_p50 ? Math.round(b.bitrate_kbps_p50) : 0,
    p95: b.bitrate_kbps_p95 ? Math.round(b.bitrate_kbps_p95) : 0,
  }));

  const cardStyle: React.CSSProperties = {
    background: "var(--color-surface)",
    border: "1px solid var(--color-border)",
    borderRadius: "var(--radius-control)",
    padding: "16px 20px",
  };

  const labelStyle: React.CSSProperties = {
    fontSize: 11,
    color: "var(--color-muted)",
    textTransform: "uppercase",
    letterSpacing: "0.06em",
    fontWeight: 500,
    marginBottom: 6,
  };

  const valueStyle: React.CSSProperties = {
    fontSize: 28,
    fontWeight: 700,
    lineHeight: 1,
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-3)" }}>
        <h1 style={{ margin: 0, fontSize: 18, fontWeight: 700 }}>Viewer QoE</h1>
      </div>

      {/* Slice controls */}
      <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--space-3)", alignItems: "flex-end" }}>
        <DateRangePicker value={range} onChange={setRange} />
        {/* QO-3: aria-label provides accessible label (placeholder disappears on type).
            QO-4: outline:none removed; focus ring provided by .filter-input:focus-visible in global.css */}
        <input
          type="text"
          placeholder="Stream ID filter"
          aria-label="Stream ID filter"
          value={streamFilter}
          onChange={(e) => setStreamFilter(e.target.value)}
          className="filter-input"
          style={{
            background: "var(--color-surface-2)",
            border: "1px solid var(--color-border)",
            borderRadius: 6,
            padding: "6px 10px",
            color: "var(--color-text)",
            fontSize: 13,
            width: 180,
          }}
        />
        <input
          type="text"
          placeholder="App filter"
          aria-label="App filter"
          value={appFilter}
          onChange={(e) => setAppFilter(e.target.value)}
          className="filter-input"
          style={{
            background: "var(--color-surface-2)",
            border: "1px solid var(--color-border)",
            borderRadius: 6,
            padding: "6px 10px",
            color: "var(--color-text)",
            fontSize: 13,
            width: 160,
          }}
        />
      </div>

      {error && <ErrorBanner message={error} onRetry={load} />}

      {loading ? (
        <LoadingSpinner label="Loading QoE data…" />
      ) : !hasData ? (
        <EmptyState
          title="No QoE data yet"
          description="Viewer QoE data is collected by the beacon SDK. Install the SDK in your player to start collecting startup times, rebuffer events, and error rates."
          action={
            <a
              href="https://github.com/aytekXR/ams-pulse#sdk-setup"
              target="_blank"
              rel="noopener noreferrer"
              style={{
                display: "inline-block",
                background: "var(--color-accent)",
                color: "var(--color-on-signal)",
                borderRadius: 6,
                padding: "var(--space-2) var(--space-4)",
                fontSize: 13,
                fontWeight: 600,
                textDecoration: "none",
              }}
            >
              SDK Setup Docs
            </a>
          }
        />
      ) : (
        <>
          {/* Summary cards */}
          {totals && (
            <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))", gap: "var(--space-3)" }}>
              <div style={cardStyle}>
                <div style={labelStyle}>Startup p50</div>
                <div style={valueStyle}>{totals.startup_p50_ms.toFixed(0)}<span style={{ fontSize: 14, color: "var(--color-muted)", marginLeft: "var(--space-1)" }}>ms</span></div>
              </div>
              <div style={cardStyle}>
                <div style={labelStyle}>Startup p95</div>
                <div style={valueStyle}>{totals.startup_p95_ms.toFixed(0)}<span style={{ fontSize: 14, color: "var(--color-muted)", marginLeft: "var(--space-1)" }}>ms</span></div>
              </div>
              {/* QO-5: Badge added alongside color for color-not-only compliance (WCAG 1.4.1).
                  QO-5: Dead fallback hex dropped — var(--color-warning) is defined for both themes
                  in global.css (:root #FFB224 dark; [data-theme="light"] #B45309 light). */}
              <div style={cardStyle}>
                <div style={labelStyle}>Rebuffer Ratio</div>
                <div style={{ display: "flex", alignItems: "baseline", gap: "var(--space-2)", flexWrap: "wrap" }}>
                  <div style={{ ...valueStyle, color: totals.rebuffer_ratio > 0.05 ? "var(--color-warning)" : "inherit" }}>
                    {(totals.rebuffer_ratio * 100).toFixed(1)}<span style={{ fontSize: 14, color: "var(--color-muted)", marginLeft: "var(--space-1)" }}>%</span>
                  </div>
                  {totals.rebuffer_ratio > 0.05 && <Badge label="HIGH" variant="warning" />}
                </div>
              </div>
              {/* QO-5: Same pattern for error rate.
                  Dead fallback hex dropped — var(--color-error) is defined for both themes
                  (:root #FF5C68 dark; [data-theme="light"] #DC2626 light). */}
              <div style={cardStyle}>
                <div style={labelStyle}>Error Rate</div>
                <div style={{ display: "flex", alignItems: "baseline", gap: "var(--space-2)", flexWrap: "wrap" }}>
                  <div style={{ ...valueStyle, color: totals.error_rate > 0.01 ? "var(--color-error)" : "inherit" }}>
                    {(totals.error_rate * 100).toFixed(2)}<span style={{ fontSize: 14, color: "var(--color-muted)", marginLeft: "var(--space-1)" }}>%</span>
                  </div>
                  {totals.error_rate > 0.01 && <Badge label="HIGH" variant="error" />}
                </div>
              </div>
            </div>
          )}

          {/* Bitrate timeline */}
          {chartData.length > 0 ? (
            <div style={{ background: "var(--color-surface)", border: "1px solid var(--color-border)", borderRadius: "var(--radius-control)", padding: "var(--space-4)" }}>
              <h2 style={{ margin: "0 0 var(--space-4)", fontSize: 13, fontWeight: 600, color: "var(--color-muted)", textTransform: "uppercase", letterSpacing: "0.06em" }}>
                Bitrate Timeline (Kbps)
              </h2>
              <ResponsiveContainer width="100%" height={240}>
                {/* QO-2: accessibilityLayer injects <title>/<desc> into the SVG and makes
                    data points keyboard-navigable (Recharts v2.1+). */}
                <LineChart accessibilityLayer data={chartData} margin={{ top: 4, right: 16, left: 0, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
                  <XAxis dataKey="ts" tick={{ fill: "var(--color-muted)", fontSize: 11 }} />
                  <YAxis tick={{ fill: "var(--color-muted)", fontSize: 11 }} unit=" kbps" />
                  <Tooltip
                    contentStyle={{
                      background: "var(--color-surface)",
                      border: "1px solid var(--color-border)",
                      borderRadius: 6,
                      color: "var(--color-text)",
                    }}
                  />
                  <Legend wrapperStyle={{ fontSize: 12, color: "var(--color-muted)" }} />
                  {/* QO-1: isAnimationActive={false} — tokens.json motion.note bans slide
                      animations on charts unconditionally ("never slide charts"), not only
                      under prefers-reduced-motion. CSS --motion-base does not reach Recharts'
                      JS animation engine; the prop is the required fix.
                      P-3/hex gate: CHART_COLORS[1]=#58A6FF, CHART_COLORS[4]=#FFB224. */}
                  <Line
                    type="monotone"
                    dataKey="p50"
                    stroke={CHART_COLORS[1]}
                    dot={false}
                    strokeWidth={2}
                    name="Bitrate p50"
                    isAnimationActive={false}
                  />
                  <Line
                    type="monotone"
                    dataKey="p95"
                    stroke={CHART_COLORS[4]}
                    dot={false}
                    strokeWidth={2}
                    name="Bitrate p95"
                    strokeDasharray="4 2"
                    isAnimationActive={false}
                  />
                </LineChart>
              </ResponsiveContainer>
            </div>
          ) : (
            <EmptyState
              title="No bitrate data in range"
              description="Adjust the time range or wait for beacons to accumulate."
            />
          )}
        </>
      )}
    </div>
  );
}
