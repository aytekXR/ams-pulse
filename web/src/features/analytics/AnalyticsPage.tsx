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
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";
import { EmptyState } from "@/components/EmptyState";
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
  const [audience, setAudience] = useState<AudienceResponse | null>(null);
  const [geo, setGeo] = useState<GeoResponse | null>(null);
  const [device, setDevice] = useState<DeviceResponse | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
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
      const msg = err instanceof ApiError ? err.message : "Failed to load analytics";
      setError(msg);
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

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 12 }}>
        <h1 style={{ margin: 0, fontSize: 18, fontWeight: 700 }}>Analytics</h1>
        <button
          onClick={exportCsv}
          style={{
            background: "var(--color-surface-2)",
            border: "1px solid var(--color-border)",
            color: "var(--color-muted)",
            borderRadius: 6,
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
      <div style={{ display: "flex", gap: 0, borderBottom: "1px solid var(--color-border)" }}>
        {(["audience", "geo", "device"] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            style={{
              background: "none",
              border: "none",
              borderBottom: `2px solid ${tab === t ? "var(--color-accent)" : "transparent"}`,
              color: tab === t ? "var(--color-text)" : "var(--color-muted)",
              padding: "8px 16px",
              cursor: "pointer",
              fontSize: 13,
              fontWeight: tab === t ? 600 : 400,
              textTransform: "capitalize",
            }}
          >
            {t}
          </button>
        ))}
      </div>

      {loading ? (
        <LoadingSpinner label="Loading analytics…" />
      ) : (
        <>
          {tab === "audience" && (
            <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
              {/* Totals row — AudienceTotals: { views, uniques, watch_time_s, peak_concurrency } */}
              {totals && (
                <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(150px, 1fr))", gap: 12 }}>
                  {[
                    { label: "Total Views", value: (totals.views ?? 0).toLocaleString() },
                    { label: "Unique Viewers", value: (totals.uniques ?? 0).toLocaleString() },
                    { label: "Watch Time", value: `${Math.round((totals.watch_time_s ?? 0) / 3600)}h` },
                    { label: "Peak Concurrency", value: (totals.peak_concurrency ?? 0).toLocaleString() },
                  ].map((s) => (
                    <div
                      key={s.label}
                      style={{
                        background: "var(--color-surface)",
                        border: "1px solid var(--color-border)",
                        borderRadius: 8,
                        padding: "14px 16px",
                      }}
                    >
                      <div style={{ fontSize: 11, color: "var(--color-muted)", textTransform: "uppercase", letterSpacing: "0.06em", fontWeight: 500, marginBottom: 4 }}>
                        {s.label}
                      </div>
                      <div style={{ fontSize: 24, fontWeight: 700 }}>{s.value}</div>
                    </div>
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
                    borderRadius: 8,
                    padding: 16,
                  }}
                >
                  <h2 style={{ margin: "0 0 16px", fontSize: 13, fontWeight: 600, color: "var(--color-muted)", textTransform: "uppercase", letterSpacing: "0.06em" }}>
                    Audience over time
                  </h2>
                  <ResponsiveContainer width="100%" height={280}>
                    <LineChart data={chartData} margin={{ top: 4, right: 16, left: 0, bottom: 0 }}>
                      <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
                      <XAxis dataKey="ts" tick={{ fill: "var(--color-muted)", fontSize: 11 }} />
                      <YAxis tick={{ fill: "var(--color-muted)", fontSize: 11 }} />
                      <Tooltip
                        contentStyle={{
                          background: "var(--color-surface)",
                          border: "1px solid var(--color-border)",
                          borderRadius: 6,
                          color: "var(--color-text)",
                        }}
                      />
                      <Legend wrapperStyle={{ fontSize: 12, color: "var(--color-muted)" }} />
                      <Line type="monotone" dataKey="views" stroke="#60a5fa" dot={false} strokeWidth={2} name="Views" />
                      <Line type="monotone" dataKey="uniques" stroke="#34d399" dot={false} strokeWidth={2} name="Uniques" />
                      <Line type="monotone" dataKey="peak" stroke="#fbbf24" dot={false} strokeWidth={2} name="Peak concurrent" />
                    </LineChart>
                  </ResponsiveContainer>
                </div>
              )}
            </div>
          )}

          {tab === "geo" && (
            geo && (geo.rows ?? []).length > 0 ? (
              <div
                style={{
                  background: "var(--color-surface)",
                  border: "1px solid var(--color-border)",
                  borderRadius: 8,
                  overflow: "hidden",
                }}
              >
                <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
                  <thead style={{ background: "var(--color-surface-2)" }}>
                    <tr>
                      {["Country", "Views", "Unique Viewers", "Watch Time"].map((h) => (
                        <th key={h} style={{ padding: "10px 14px", textAlign: h === "Country" ? "left" : "right", fontSize: 11, color: "var(--color-muted)", textTransform: "uppercase", letterSpacing: "0.06em", fontWeight: 600 }}>{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {/* GeoRow: { country, region?, views, uniques, watch_time_s } */}
                    {(geo.rows ?? []).map((row) => (
                      <tr key={row.country} style={{ borderTop: "1px solid var(--color-border)" }}>
                        <td style={{ padding: "8px 14px" }}>{row.country ?? "Unknown"}</td>
                        <td style={{ padding: "8px 14px", textAlign: "right" }}>{(row.views ?? 0).toLocaleString()}</td>
                        <td style={{ padding: "8px 14px", textAlign: "right" }}>{(row.uniques ?? 0).toLocaleString()}</td>
                        <td style={{ padding: "8px 14px", textAlign: "right" }}>{Math.round((row.watch_time_s ?? 0) / 60)}m</td>
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
            )
          )}

          {tab === "device" && (
            device && (device.rows ?? []).length > 0 ? (
              <div
                style={{
                  background: "var(--color-surface)",
                  border: "1px solid var(--color-border)",
                  borderRadius: 8,
                  overflow: "hidden",
                }}
              >
                <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
                  <thead style={{ background: "var(--color-surface-2)" }}>
                    <tr>
                      {["Device", "Browser", "OS", "Views"].map((h) => (
                        <th key={h} style={{ padding: "10px 14px", textAlign: h === "Views" ? "right" : "left", fontSize: 11, color: "var(--color-muted)", textTransform: "uppercase", letterSpacing: "0.06em", fontWeight: 600 }}>{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {/* DeviceRow: { device, os, browser, protocol, views, uniques, watch_time_s } */}
                    {(device.rows ?? []).map((row, i) => (
                      <tr key={i} style={{ borderTop: "1px solid var(--color-border)" }}>
                        <td style={{ padding: "8px 14px" }}>{row.device ?? "Unknown"}</td>
                        <td style={{ padding: "8px 14px" }}>{row.browser ?? "—"}</td>
                        <td style={{ padding: "8px 14px" }}>{row.os ?? "—"}</td>
                        <td style={{ padding: "8px 14px", textAlign: "right" }}>{(row.views ?? 0).toLocaleString()}</td>
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
            )
          )}
        </>
      )}
    </div>
  );
}
