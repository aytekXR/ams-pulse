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
import { CHART_COLORS, CHART_TICK, CHART_LEGEND_STYLE, CHART_TOOLTIP_STYLE } from "@/lib/chartColors";
import { DateRangePicker, defaultDateRange } from "@/features/analytics/DateRangePicker";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";
import { LicenseRequiredGate, isLicenseError } from "@/components/LicenseRequiredGate";
import { EmptyState } from "@/components/EmptyState";
import { Badge } from "@/components/Badge";
import { StatCard } from "@/features/live/StatCard";
import type { QoeSummaryResponse } from "@/lib/api/types";

export function QoePage() {
  const [range, setRange] = useState(defaultDateRange);
  const [streamFilter, setStreamFilter] = useState("");
  const [appFilter, setAppFilter] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // A licence 403 is a tier boundary, not a malfunction — it renders as the
  // designed upgrade prompt rather than a red error banner.
  const [licenseError, setLicenseError] = useState<ApiError | null>(null);
  const [data, setData] = useState<QoeSummaryResponse | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    setLicenseError(null);
    try {
      const result = await qoeApi.getSummary({
        from: range.from,
        to: range.to,
        stream_id: streamFilter || undefined,
        app: appFilter || undefined,
      });
      setData(result);
    } catch (err) {
      if (isLicenseError(err)) {
        setLicenseError(err);
      } else {
        const msg = err instanceof ApiError ? err.message : "Failed to load QoE data";
        setError(msg);
      }
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

  // A licence refusal replaces the page body: showing filters and an empty chart
  // above an upsell reads as a broken screen. The heading stays for orientation.
  if (licenseError) {
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-5)" }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-3)" }}>
          <h1 className="page-title">Viewer QoE</h1>
        </div>
        <LicenseRequiredGate
          error={licenseError}
          feature="Player QoE"
          unlocks="player QoE analytics: startup time, rebuffer ratio and error rates"
          icon={
            <svg
              width="48"
              height="48"
              viewBox="0 0 24 24"
              fill="none"
              stroke="var(--color-accent)"
              strokeWidth="1.5"
              aria-hidden="true"
            >
              <path d="M12 2 2 7l10 5 10-5-10-5Z" />
              <path d="m2 17 10 5 10-5" />
              <path d="m2 12 10 5 10-5" />
            </svg>
          }
        />
      </div>
    );
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-5)" }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-3)" }}>
        <h1 className="page-title">Viewer QoE</h1>
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
            borderRadius: "var(--radius-control)",
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
            borderRadius: "var(--radius-control)",
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
              className="btn-primary"
              style={{
                display: "inline-block",
                color: "var(--color-on-signal)",
                borderRadius: "var(--radius-control)",
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
          {/* s111 D9: the shared <StatCard> replaces the hand-rolled third card
              variant — value size rides var(--metric-size) and padding rides
              var(--card-padding), so Wall/Compact density finally applies here.
              QO-5 threshold colour + Badge survive via valueColor/trailing. */}
          {totals && (
            <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))", gap: "var(--space-3)" }}>
              <StatCard label="Startup p50" value={totals.startup_p50_ms.toFixed(0)} unit="ms" />
              <StatCard label="Startup p95" value={totals.startup_p95_ms.toFixed(0)} unit="ms" />
              <StatCard
                label="Rebuffer Ratio"
                value={(totals.rebuffer_ratio * 100).toFixed(1)}
                unit="%"
                valueColor={totals.rebuffer_ratio > 0.05 ? "var(--color-warning)" : undefined}
                trailing={totals.rebuffer_ratio > 0.05 ? <Badge label="HIGH" variant="warning" /> : undefined}
              />
              <StatCard
                label="Error Rate"
                value={(totals.error_rate * 100).toFixed(2)}
                unit="%"
                valueColor={totals.error_rate > 0.01 ? "var(--color-error)" : undefined}
                trailing={totals.error_rate > 0.01 ? <Badge label="HIGH" variant="error" /> : undefined}
              />
            </div>
          )}

          {/* Bitrate timeline */}
          {chartData.length > 0 ? (
            <div style={{ background: "var(--color-surface)", border: "1px solid var(--color-border)", borderRadius: "var(--radius-card)", padding: "var(--space-4)" }}>
              <h2 className="label" style={{ margin: "0 0 var(--space-4)" }}>
                Bitrate Timeline (Kbps)
              </h2>
              <ResponsiveContainer width="100%" height={240}>
                {/* QO-2: accessibilityLayer injects <title>/<desc> into the SVG and makes
                    data points keyboard-navigable (Recharts v2.1+). */}
                <LineChart accessibilityLayer data={chartData} margin={{ top: 4, right: 16, left: 0, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
                  <XAxis dataKey="ts" tick={CHART_TICK} />
                  <YAxis tick={CHART_TICK} unit=" kbps" />
                  {/* s111 M4: instant tooltip — the default 400ms position-lag
                      trails the crosshair while reading precise p50/p95 values. */}
                  <Tooltip isAnimationActive={false} contentStyle={CHART_TOOLTIP_STYLE} />
                  <Legend wrapperStyle={CHART_LEGEND_STYLE} />
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
