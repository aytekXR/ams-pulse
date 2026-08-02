/**
 * F4 — Publisher & Ingest Health (/ingest)
 *
 * Per-publisher list with health score badge + state.
 * Detail view: bitrate/fps/keyframe/packet-loss/jitter timelines.
 * Drop events markers; threshold indicators.
 * Live updates via LiveSocket WS envelope (poll fallback).
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
import { qoeApi, ApiError } from "@/api/client";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";
import { LicenseRequiredGate, isLicenseError } from "@/components/LicenseRequiredGate";
import { EmptyState } from "@/components/EmptyState";
import { Badge } from "@/components/Badge";
import { CHART_COLORS, useStatusColors, CHART_TICK, CHART_LEGEND_STYLE, CHART_TOOLTIP_STYLE, CHART_REF_LABEL_SIZE } from "@/lib/chartColors";
import type { IngestHealthResponse, IngestStream } from "@/lib/api/types";

function healthVariant(score: number): "success" | "warning" | "error" | "muted" {
  if (score >= 80) return "success";
  if (score >= 50) return "warning";
  if (score >= 0) return "error";
  return "muted";
}

function healthLabel(score: number): string {
  if (score >= 80) return "Healthy";
  if (score >= 50) return "Degraded";
  return "Poor";
}

interface StreamDetailProps {
  stream: IngestStream;
  onClose: () => void;
}

function StreamDetail({ stream, onClose }: StreamDetailProps) {
  const statusColors = useStatusColors();
  const chartData = stream.timeseries.map((b) => ({
    ts: new Date(b.ts).toLocaleTimeString(),
    bitrate: b.bitrate_kbps ? Math.round(b.bitrate_kbps) : 0,
    fps: b.fps ?? 0,
    keyframe: b.keyframe_interval_s ?? 0,
    pkt_loss: b.packet_loss_pct ?? 0,
    jitter: b.jitter_ms ?? 0,
  }));

  const dropTimes = (stream.drop_events ?? []).map((d) => ({
    ts: new Date(d.ts).toLocaleTimeString(),
    reason: d.reason,
  }));

  return (
    <div className="panel-enter" style={{
      background: "var(--color-surface)",
      border: "1px solid var(--color-border)",
      borderRadius: "var(--radius-card)",
      padding: "var(--space-5)",
      display: "flex",
      flexDirection: "column",
      gap: "var(--space-4)",
    }}>
      <div style={{ display: "flex", alignItems: "center", gap: "var(--space-3)" }}>
        <div style={{ flex: 1 }}>
          <div style={{ fontWeight: 700, fontSize: 15 }}>{stream.stream_id}</div>
          <div style={{ fontSize: 12, color: "var(--color-secondary)" }}>{stream.app}{stream.node_id ? ` · ${stream.node_id}` : ""}</div>
        </div>
        <Badge label={`${stream.health_score.toFixed(0)}/100`} variant={healthVariant(stream.health_score)} />
        <button
          onClick={onClose}
          aria-label="Close stream detail"
          className="btn-secondary"
          style={{
            background: "none",
            borderRadius: "var(--radius-control)",
            padding: "6px 10px",
            minHeight: 28,
            cursor: "pointer",
            fontSize: 12,
          }}
        >
          Close
        </button>
      </div>

      {/* Drop events */}
      {dropTimes.length > 0 && (
        <div
          aria-live="polite"
          aria-label="Drop events"
          style={{
            background: "var(--color-error-bg)",
            border: "1px solid var(--color-error)",
            borderRadius: "var(--radius-control)",
            padding: "var(--space-3) var(--space-4)",
          }}
        >
          <div style={{ fontSize: 12, fontWeight: 600, color: "var(--color-error)", marginBottom: 6 }}>
            Drop Events ({dropTimes.length})
          </div>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
            {dropTimes.map((d, i) => (
              <span key={i} style={{
                background: "rgba(224,82,82,0.15)",
                borderRadius: "var(--radius-pill)",
                padding: "2px 8px",
                fontSize: 11,
                color: "var(--color-error)",
              }}>
                {d.ts} — {d.reason}
              </span>
            ))}
          </div>
        </div>
      )}

      {/* Bitrate + FPS */}
      {chartData.length > 0 && (
        <>
          <div style={{ background: "var(--color-surface-2)", borderRadius: "var(--radius-control)", padding: "var(--space-4)" }}>
            <h3 className="label" style={{ margin: "0 0 var(--space-3)" }}>
              Bitrate & FPS
            </h3>
            <ResponsiveContainer width="100%" height={180}>
              <LineChart data={chartData} margin={{ top: 4, right: 16, left: 0, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
                <XAxis dataKey="ts" tick={CHART_TICK} />
                <YAxis yAxisId="bitrate" orientation="left" tick={CHART_TICK} unit=" kbps" />
                <YAxis yAxisId="fps" orientation="right" tick={CHART_TICK} unit=" fps" />
                <Tooltip isAnimationActive={false} contentStyle={CHART_TOOLTIP_STYLE} />
                <Legend wrapperStyle={CHART_LEGEND_STYLE} />
                {dropTimes.map((d, i) => (
                  <ReferenceLine key={i} x={d.ts} yAxisId="bitrate" stroke={statusColors.critical} strokeDasharray="3 3" label={{ value: "drop", fontSize: CHART_REF_LABEL_SIZE, fill: statusColors.critical }} />
                ))}
                {/* QO-1 (s111 D5/M0): isAnimationActive={false} — tokens.json
                    motion.note bans slide animations on charts unconditionally;
                    CSS motion tokens cannot reach Recharts' JS engine. */}
                <Line yAxisId="bitrate" type="monotone" dataKey="bitrate" stroke={CHART_COLORS[1]} dot={false} strokeWidth={2} name="Bitrate" isAnimationActive={false} />
                <Line yAxisId="fps" type="monotone" dataKey="fps" stroke={CHART_COLORS[0]} dot={false} strokeWidth={2} name="FPS" isAnimationActive={false} />
              </LineChart>
            </ResponsiveContainer>
          </div>

          <div style={{ background: "var(--color-surface-2)", borderRadius: "var(--radius-control)", padding: "var(--space-4)" }}>
            <h3 className="label" style={{ margin: "0 0 var(--space-3)" }}>
              Packet Loss & Jitter
            </h3>
            <ResponsiveContainer width="100%" height={160}>
              <LineChart data={chartData} margin={{ top: 4, right: 16, left: 0, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
                <XAxis dataKey="ts" tick={CHART_TICK} />
                <YAxis yAxisId="loss" orientation="left" tick={CHART_TICK} unit="%" />
                <YAxis yAxisId="jitter" orientation="right" tick={CHART_TICK} unit=" ms" />
                <Tooltip isAnimationActive={false} contentStyle={CHART_TOOLTIP_STYLE} />
                <Legend wrapperStyle={CHART_LEGEND_STYLE} />
                {/* Threshold indicators */}
                <ReferenceLine yAxisId="loss" y={1} stroke={statusColors.warning} strokeDasharray="4 2" label={{ value: "1% threshold", fontSize: CHART_REF_LABEL_SIZE, fill: statusColors.warning }} />
                {/* QO-1 (s111 D5/M0): see comment on the bitrate chart above. */}
                <Line yAxisId="loss" type="monotone" dataKey="pkt_loss" stroke={statusColors.critical} dot={false} strokeWidth={2} name="Packet Loss %" isAnimationActive={false} />
                <Line yAxisId="jitter" type="monotone" dataKey="jitter" stroke={CHART_COLORS[4]} dot={false} strokeWidth={2} name="Jitter ms" isAnimationActive={false} />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </>
      )}
    </div>
  );
}

export function IngestPage() {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // A licence 403 is a tier boundary, not a malfunction — it renders as the
  // designed upgrade prompt rather than a red error banner.
  const [licenseError, setLicenseError] = useState<ApiError | null>(null);
  const [data, setData] = useState<IngestHealthResponse | null>(null);
  const [selected, setSelected] = useState<IngestStream | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    setLicenseError(null);
    try {
      const result = await qoeApi.getIngestHealth({
        from: Date.now() - 15 * 60 * 1000, // last 15 min
        to: Date.now(),
      });
      setData(result);
    } catch (err) {
      if (isLicenseError(err)) {
        setLicenseError(err);
      } else {
        const msg = err instanceof ApiError ? err.message : "Failed to load ingest health";
        setError(msg);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
    // Auto-refresh every 15 seconds (degradation visible ≤15s)
    const timer = setInterval(() => { void load(); }, 15_000);
    return () => clearInterval(timer);
  }, [load]);

  const streams = data?.streams ?? [];

  // A licence refusal replaces the page body: showing filters and an empty chart
  // above an upsell reads as a broken screen. The heading stays for orientation.
  if (licenseError) {
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-5)" }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-3)" }}>
          <h1 className="page-title">Ingest Health</h1>
        </div>
        <LicenseRequiredGate
          error={licenseError}
          feature="Ingest health"
          unlocks="publisher and ingest health, bitrate timelines and drop tracking"
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
        <h1 className="page-title">Ingest Health</h1>
        <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
          <span style={{
            width: 7,
            height: 7,
            borderRadius: "50%",
            background: loading ? "var(--color-warning)" : "var(--color-success)",
            display: "inline-block",
          }} aria-hidden="true" />
          <span style={{ fontSize: 12, color: "var(--color-secondary)" }}>
            {loading ? "Refreshing…" : "Live (15s)"}
          </span>
          <button
            onClick={load}
            disabled={loading}
            className="btn-secondary"
            style={{
              background: "var(--color-surface-2)",
              borderRadius: "var(--radius-control)",
              padding: "6px 10px",
              minHeight: 28,
              fontSize: 11,
            }}
          >
            Refresh
          </button>
        </div>
      </div>

      {error && <ErrorBanner message={error} onRetry={load} />}

      {loading && streams.length === 0 ? (
        <LoadingSpinner label="Loading ingest health…" />
      ) : streams.length === 0 ? (
        <EmptyState
          title="No active publishers"
          description="Ingest health data will appear here when publishers are active. Data updates every 15 seconds."
        />
      ) : (
        <>
          {/* Publisher list */}
          <div style={{
            background: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            borderRadius: "var(--radius-card)",
            overflow: "hidden",
          }}>
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
              <thead style={{ background: "var(--color-surface-2)" }}>
                <tr>
                  {["Stream", "App", "Node", "Health", "Drops", ""].map((h) => (
                    <th key={h} className="label" style={{
                      padding: "var(--space-3) var(--space-4)",
                      textAlign: h === "" ? "right" : "left",
                    }}>
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {streams.map((s, i) => (
                  <tr
                    key={s.stream_id}
                    style={{
                      borderTop: i === 0 ? "none" : "1px solid var(--color-border)",
                      cursor: "pointer",
                      background: selected?.stream_id === s.stream_id ? "var(--color-surface-2)" : "transparent",
                    }}
                    onClick={() => setSelected(selected?.stream_id === s.stream_id ? null : s)}
                  >
                    <td style={{ padding: "var(--cell-pad)", fontFamily: "var(--font-mono)", fontWeight: 600, fontSize: 12 }}>{s.stream_id}</td>
                    <td style={{ padding: "var(--cell-pad)", color: "var(--color-secondary)" }}>{s.app}</td>
                    <td style={{ padding: "var(--cell-pad)", color: "var(--color-secondary)", fontSize: 12 }}>{s.node_id ?? "—"}</td>
                    <td style={{ padding: "var(--cell-pad)" }}>
                      <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
                        <div data-testid="health-bar-bg" aria-hidden="true" style={{
                          width: 60,
                          height: 6,
                          background: "var(--color-surface-2)",
                          borderRadius: "var(--radius-pill)",
                          overflow: "hidden",
                        }}>
                          <div style={{
                            width: `${s.health_score}%`,
                            height: "100%",
                            background: s.health_score >= 80 ? "var(--color-success)" : s.health_score >= 50 ? "var(--color-warning)" : "var(--color-error)",
                          }} />
                        </div>
                        <Badge label={healthLabel(s.health_score)} variant={healthVariant(s.health_score)} />
                      </div>
                    </td>
                    <td style={{ padding: "var(--cell-pad)", color: "var(--color-secondary)", fontSize: 12 }}>
                      {(s.drop_events ?? []).length > 0 ? (
                        <span style={{ color: "var(--color-error)", fontWeight: 600 }}>
                          {s.drop_events!.length} drop{s.drop_events!.length > 1 ? "s" : ""}
                        </span>
                      ) : "—"}
                    </td>
                    <td style={{ padding: "var(--cell-pad)", textAlign: "right" }}>
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          setSelected(selected?.stream_id === s.stream_id ? null : s);
                        }}
                        className="btn-secondary"
                        style={{
                          background: "none",
                          borderRadius: "var(--radius-control)",
                          padding: "6px 10px",
                          minHeight: 28,
                          cursor: "pointer",
                          fontSize: 11,
                        }}
                      >
                        {selected?.stream_id === s.stream_id ? "Collapse" : "Details"}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Detail panel */}
          {selected && (
            <StreamDetail stream={selected} onClose={() => setSelected(null)} />
          )}
        </>
      )}
    </div>
  );
}
