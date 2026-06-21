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
import { DateRangePicker, defaultDateRange } from "@/features/analytics/DateRangePicker";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";
import { EmptyState } from "@/components/EmptyState";
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
    borderRadius: 8,
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
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 12 }}>
        <h1 style={{ margin: 0, fontSize: 18, fontWeight: 700 }}>Viewer QoE</h1>
      </div>

      {/* Slice controls */}
      <div style={{ display: "flex", flexWrap: "wrap", gap: 12, alignItems: "flex-end" }}>
        <DateRangePicker value={range} onChange={setRange} />
        <input
          type="text"
          placeholder="Stream ID filter"
          value={streamFilter}
          onChange={(e) => setStreamFilter(e.target.value)}
          style={{
            background: "var(--color-surface-2)",
            border: "1px solid var(--color-border)",
            borderRadius: 6,
            padding: "6px 10px",
            color: "var(--color-text)",
            fontSize: 13,
            outline: "none",
            width: 180,
          }}
        />
        <input
          type="text"
          placeholder="App filter"
          value={appFilter}
          onChange={(e) => setAppFilter(e.target.value)}
          style={{
            background: "var(--color-surface-2)",
            border: "1px solid var(--color-border)",
            borderRadius: 6,
            padding: "6px 10px",
            color: "var(--color-text)",
            fontSize: 13,
            outline: "none",
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
                color: "#fff",
                borderRadius: 6,
                padding: "8px 16px",
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
            <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))", gap: 12 }}>
              <div style={cardStyle}>
                <div style={labelStyle}>Startup p50</div>
                <div style={valueStyle}>{totals.startup_p50_ms.toFixed(0)}<span style={{ fontSize: 14, color: "var(--color-muted)", marginLeft: 4 }}>ms</span></div>
              </div>
              <div style={cardStyle}>
                <div style={labelStyle}>Startup p95</div>
                <div style={valueStyle}>{totals.startup_p95_ms.toFixed(0)}<span style={{ fontSize: 14, color: "var(--color-muted)", marginLeft: 4 }}>ms</span></div>
              </div>
              <div style={cardStyle}>
                <div style={labelStyle}>Rebuffer Ratio</div>
                <div style={{ ...valueStyle, color: totals.rebuffer_ratio > 0.05 ? "var(--color-warning, #fbbf24)" : "inherit" }}>
                  {(totals.rebuffer_ratio * 100).toFixed(1)}<span style={{ fontSize: 14, color: "var(--color-muted)", marginLeft: 4 }}>%</span>
                </div>
              </div>
              <div style={cardStyle}>
                <div style={labelStyle}>Error Rate</div>
                <div style={{ ...valueStyle, color: totals.error_rate > 0.01 ? "var(--color-error, #e05252)" : "inherit" }}>
                  {(totals.error_rate * 100).toFixed(2)}<span style={{ fontSize: 14, color: "var(--color-muted)", marginLeft: 4 }}>%</span>
                </div>
              </div>
            </div>
          )}

          {/* Bitrate timeline */}
          {chartData.length > 0 ? (
            <div style={{ background: "var(--color-surface)", border: "1px solid var(--color-border)", borderRadius: 8, padding: 16 }}>
              <h2 style={{ margin: "0 0 16px", fontSize: 13, fontWeight: 600, color: "var(--color-muted)", textTransform: "uppercase", letterSpacing: "0.06em" }}>
                Bitrate Timeline (Kbps)
              </h2>
              <ResponsiveContainer width="100%" height={240}>
                <LineChart data={chartData} margin={{ top: 4, right: 16, left: 0, bottom: 0 }}>
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
                  <Line type="monotone" dataKey="p50" stroke="#60a5fa" dot={false} strokeWidth={2} name="Bitrate p50" />
                  <Line type="monotone" dataKey="p95" stroke="#fbbf24" dot={false} strokeWidth={2} name="Bitrate p95" strokeDasharray="4 2" />
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
