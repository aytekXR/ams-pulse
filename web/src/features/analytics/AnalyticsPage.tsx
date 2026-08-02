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
import { analyticsApi, ApiError } from "@/api/client";
import { DateRangePicker, defaultDateRange } from "./DateRangePicker";
import { Tabs } from "@/components/Tabs";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";
import { LicenseRequiredGate, isLicenseError } from "@/components/LicenseRequiredGate";
import { EmptyState } from "@/components/EmptyState";
import { StatCard } from "@/features/live/StatCard";
import { CHART_COLORS, CHART_TICK, CHART_LEGEND_STYLE, CHART_TOOLTIP_STYLE } from "@/lib/chartColors";
import type {
  AudienceResponse,
  GeoResponse,
  DeviceResponse,
} from "@/lib/api/types";

type Tab = "audience" | "geo" | "device";

export function AnalyticsPage() {
  const [range, setRange] = useState(defaultDateRange);
  const [tab, setTab] = useState<Tab>("audience");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // A licence 403 is a tier boundary, not a malfunction — it renders as the
  // designed upgrade prompt rather than a red error banner.
  const [licenseError, setLicenseError] = useState<ApiError | null>(null);
  const [audience, setAudience] = useState<AudienceResponse | null>(null);
  const [geo, setGeo] = useState<GeoResponse | null>(null);
  const [device, setDevice] = useState<DeviceResponse | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    setLicenseError(null);
    try {
      const params = { from: range.from, to: range.to };
      const [audienceData, geoData, deviceData] = await Promise.all([
        analyticsApi.getAudience(params),
        analyticsApi.getGeo(params).catch(() => null as GeoResponse | null),
        analyticsApi.getDevices(params).catch(() => null as DeviceResponse | null),
      ]);
      setAudience(audienceData);
      setGeo(geoData);
      setDevice(deviceData);
    } catch (err) {
      if (isLicenseError(err)) {
        setLicenseError(err);
      } else {
        const msg = err instanceof ApiError ? err.message : "Failed to load analytics";
        setError(msg);
      }
    } finally {
      setLoading(false);
    }
  }, [range]);

  useEffect(() => {
    void load();
  }, [load]);

  const exportCsv = () => analyticsApi.exportCsv({ from: range.from, to: range.to });

  // AudienceResponse: { totals: AudienceTotals, timeseries: AudienceBucket[] }
  // AudienceBucket: { ts, views, uniques, watch_time_s, peak_concurrency }
  const chartData = (audience?.timeseries ?? []).map((b) => ({
    ts: new Date(b.ts).toLocaleDateString(),
    views: b.views ?? 0,
    uniques: b.uniques ?? 0,
    watch_time_h: b.watch_time_s ? Math.round(b.watch_time_s / 3600) : 0,
    peak: b.peak_concurrency ?? 0,
  }));

  const totals = audience?.totals;

  // A licence refusal replaces the page body: showing filters and an empty chart
  // above an upsell reads as a broken screen. The heading stays for orientation.
  if (licenseError) {
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-5)" }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-3)" }}>
          <h1 className="page-title">Analytics</h1>
        </div>
        <LicenseRequiredGate
          error={licenseError}
          feature="Historical analytics"
          unlocks="historical audience analytics with geo and device breakdowns"
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
        <h1 className="page-title">Analytics</h1>
        {/* s111 D7: .btn-secondary owns color/border (hover would be defeated
            by inline copies — do not re-add them here). */}
        <button
          onClick={exportCsv}
          className="btn-secondary"
          style={{
            background: "var(--color-surface-2)",
            borderRadius: "var(--radius-control)",
            padding: "6px 12px",
            cursor: "pointer",
            fontSize: 12,
          }}
        >
          Export CSV
        </button>
      </div>

      <DateRangePicker value={range} onChange={setRange} />

      {error && <ErrorBanner message={error} onRetry={load} />}

      {/* Tabs */}
      <Tabs
        tabs={[
          { id: "audience", label: "Audience" },
          { id: "geo", label: "Geo" },
          { id: "device", label: "Device" },
        ]}
        activeTab={tab}
        onTabChange={(id) => setTab(id as Tab)}
      />

      {loading ? (
        <LoadingSpinner label="Loading analytics…" />
      ) : (
        <>
          {tab === "audience" && (
            <div
              role="tabpanel"
              id="panel-audience"
              aria-labelledby="tab-audience"
              tabIndex={0}
              style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)" }}
            >
              {/* Totals row — AudienceTotals: { views, uniques, watch_time_s, peak_concurrency }
                  Uses the shared <StatCard size="compact">: same geometry as the inline markup
                  it replaces, and it brings the role="group" accessible name the inline cards
                  never had. */}
              {totals && (
                <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(150px, 1fr))", gap: "var(--space-3)" }}>
                  {[
                    { label: "Total Views", value: (totals.views ?? 0).toLocaleString() },
                    { label: "Unique Viewers", value: (totals.uniques ?? 0).toLocaleString() },
                    { label: "Watch Time", value: `${Math.round((totals.watch_time_s ?? 0) / 3600)}h` },
                    { label: "Peak Concurrency", value: (totals.peak_concurrency ?? 0).toLocaleString() },
                  ].map((s) => (
                    <StatCard key={s.label} size="compact" label={s.label} value={s.value} />
                  ))}
                </div>
              )}

              {/* Timeseries chart */}
              {chartData.length === 0 ? (
                <EmptyState
                  title="No data for this range"
                  description="Try a wider date range or wait for data to accumulate."
                />
              ) : (
                <div
                  style={{
                    background: "var(--color-surface)",
                    border: "1px solid var(--color-border)",
                    borderRadius: "var(--radius-card)",
                    padding: "var(--space-4)",
                  }}
                >
                  <h2 className="label" style={{ margin: "0 0 var(--space-4)" }}>
                    Audience over time
                  </h2>
                  <ResponsiveContainer width="100%" height={280}>
                    {/* accessibilityLayer: Recharts keyboard navigation + per-point
                        announcements. The three series are also distinguished by their
                        Legend names, so the chart is not colour-only. */}
                    <LineChart
                      data={chartData}
                      margin={{ top: 4, right: 16, left: 0, bottom: 0 }}
                      accessibilityLayer
                    >
                      <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
                      <XAxis dataKey="ts" tick={CHART_TICK} />
                      <YAxis tick={CHART_TICK} />
                      {/* s111 M4: instant tooltip — no 400ms position-lag while
                          scrubbing the timeline (functional data, QO-1 rule). */}
                      <Tooltip isAnimationActive={false} contentStyle={CHART_TOOLTIP_STYLE} />
                      <Legend wrapperStyle={CHART_LEGEND_STYLE} />
                      {/* stroke= is an SVG presentation attribute: it needs a literal hex,
                          not var(--chart-N). Same hex as before, named by dataviz index. */}
                      {/* QO-1 (s111 D5/M0): isAnimationActive={false} — tokens.json
                          motion.note bans slide animations on charts unconditionally;
                          CSS motion tokens cannot reach Recharts' JS engine. */}
                      <Line type="monotone" dataKey="views" stroke={CHART_COLORS[1]} dot={false} strokeWidth={2} name="Views" isAnimationActive={false} />
                      <Line type="monotone" dataKey="uniques" stroke={CHART_COLORS[0]} dot={false} strokeWidth={2} name="Uniques" isAnimationActive={false} />
                      <Line type="monotone" dataKey="peak" stroke={CHART_COLORS[4]} dot={false} strokeWidth={2} name="Peak concurrent" isAnimationActive={false} />
                    </LineChart>
                  </ResponsiveContainer>
                </div>
              )}
            </div>
          )}

          {tab === "geo" && (
            <div role="tabpanel" id="panel-geo" aria-labelledby="tab-geo" tabIndex={0}>
              {geo && (geo.rows ?? []).length > 0 ? (
                <div
                  style={{
                    background: "var(--color-surface)",
                    border: "1px solid var(--color-border)",
                    borderRadius: "var(--radius-card)",
                    overflow: "hidden",
                  }}
                >
                  <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
                    <thead style={{ background: "var(--color-surface-2)" }}>
                      <tr>
                        {["Country", "Views", "Unique Viewers", "Watch Time"].map((h) => (
                          <th key={h} scope="col" className="label" style={{ padding: "var(--space-3) var(--space-4)", textAlign: h === "Country" ? "left" : "right" }}>{h}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {/* GeoRow: { country, region?, views, uniques, watch_time_s } */}
                      {(geo.rows ?? []).map((row) => (
                        <tr key={row.country} style={{ borderTop: "1px solid var(--color-border)" }}>
                          <td style={{ padding: "var(--cell-pad)" }}>{row.country ?? "Unknown"}</td>
                          {/* s111 D12: data-numeric → tabular-nums (global.css) */}
                          <td data-numeric style={{ padding: "var(--cell-pad)", textAlign: "right" }}>{(row.views ?? 0).toLocaleString()}</td>
                          <td data-numeric style={{ padding: "var(--cell-pad)", textAlign: "right" }}>{(row.uniques ?? 0).toLocaleString()}</td>
                          <td data-numeric style={{ padding: "var(--cell-pad)", textAlign: "right" }}>{Math.round((row.watch_time_s ?? 0) / 60)}m</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <EmptyState
                  title="No geo data"
                  description="Geographic breakdown will appear here once data is collected."
                />
              )}
            </div>
          )}

          {tab === "device" && (
            <div role="tabpanel" id="panel-device" aria-labelledby="tab-device" tabIndex={0}>
              {device && (device.rows ?? []).length > 0 ? (
                <div
                  style={{
                    background: "var(--color-surface)",
                    border: "1px solid var(--color-border)",
                    borderRadius: "var(--radius-card)",
                    overflow: "hidden",
                  }}
                >
                  <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
                    <thead style={{ background: "var(--color-surface-2)" }}>
                      <tr>
                        {["Device", "Browser", "OS", "Views"].map((h) => (
                          <th key={h} scope="col" className="label" style={{ padding: "var(--space-3) var(--space-4)", textAlign: h === "Views" ? "right" : "left" }}>{h}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {/* DeviceRow: { device, os, browser, protocol, views, uniques, watch_time_s } */}
                      {(device.rows ?? []).map((row, i) => (
                        <tr key={i} style={{ borderTop: "1px solid var(--color-border)" }}>
                          <td style={{ padding: "var(--cell-pad)" }}>{row.device ?? "Unknown"}</td>
                          <td style={{ padding: "var(--cell-pad)" }}>{row.browser ?? "—"}</td>
                          <td style={{ padding: "var(--cell-pad)" }}>{row.os ?? "—"}</td>
                          {/* s111 D12: data-numeric → tabular-nums (global.css) */}
                          <td data-numeric style={{ padding: "var(--cell-pad)", textAlign: "right" }}>{(row.views ?? 0).toLocaleString()}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <EmptyState
                  title="No device data"
                  description="Device breakdown will appear here once player beacon data is collected."
                />
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}
