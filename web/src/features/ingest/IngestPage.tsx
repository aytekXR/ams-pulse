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
import { EmptyState } from "@/components/EmptyState";
import { Badge } from "@/components/Badge";
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
    <div style={{
      background: "var(--color-surface)",
      border: "1px solid var(--color-border)",
      borderRadius: 8,
      padding: 20,
      display: "flex",
      flexDirection: "column",
      gap: 16,
    }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <div style={{ flex: 1 }}>
          <div style={{ fontWeight: 700, fontSize: 15 }}>{stream.stream_id}</div>
          <div style={{ fontSize: 12, color: "var(--color-muted)" }}>{stream.app}{stream.node_id ? ` · ${stream.node_id}` : ""}</div>
        </div>
        <Badge label={`${stream.health_score.toFixed(0)}/100`} variant={healthVariant(stream.health_score)} />
        <button
          onClick={onClose}
          title="Close"
          style={{
            background: "none",
            border: "1px solid var(--color-border)",
            color: "var(--color-muted)",
            borderRadius: 4,
            padding: "4px 10px",
            cursor: "pointer",
            fontSize: 12,
          }}
        >
          Close
        </button>
      </div>

      {/* Drop events */}
      {dropTimes.length > 0 && (
        <div style={{
          background: "var(--color-error-bg, #2d1a1a)",
          border: "1px solid var(--color-error, #e05252)",
          borderRadius: 6,
          padding: "10px 14px",
        }}>
          <div style={{ fontSize: 12, fontWeight: 600, color: "var(--color-error, #e05252)", marginBottom: 6 }}>
            Drop Events ({dropTimes.length})
          </div>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
            {dropTimes.map((d, i) => (
              <span key={i} style={{
                background: "rgba(224,82,82,0.15)",
                borderRadius: 4,
                padding: "2px 8px",
                fontSize: 11,
                color: "var(--color-error, #e05252)",
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
          <div style={{ background: "var(--color-surface-2)", borderRadius: 8, padding: 14 }}>
            <h3 style={{ margin: "0 0 12px", fontSize: 12, fontWeight: 600, color: "var(--color-muted)", textTransform: "uppercase", letterSpacing: "0.06em" }}>
              Bitrate & FPS
            </h3>
            <ResponsiveContainer width="100%" height={180}>
              <LineChart data={chartData} margin={{ top: 4, right: 16, left: 0, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
                <XAxis dataKey="ts" tick={{ fill: "var(--color-muted)", fontSize: 10 }} />
                <YAxis yAxisId="bitrate" orientation="left" tick={{ fill: "var(--color-muted)", fontSize: 10 }} unit=" kbps" />
                <YAxis yAxisId="fps" orientation="right" tick={{ fill: "var(--color-muted)", fontSize: 10 }} unit=" fps" />
                <Tooltip contentStyle={{ background: "var(--color-surface)", border: "1px solid var(--color-border)", borderRadius: 6, color: "var(--color-text)" }} />
                <Legend wrapperStyle={{ fontSize: 11, color: "var(--color-muted)" }} />
                {dropTimes.map((d, i) => (
                  <ReferenceLine key={i} x={d.ts} yAxisId="bitrate" stroke="var(--color-error, #e05252)" strokeDasharray="3 3" label={{ value: "drop", fontSize: 9, fill: "var(--color-error, #e05252)" }} />
                ))}
                <Line yAxisId="bitrate" type="monotone" dataKey="bitrate" stroke="#60a5fa" dot={false} strokeWidth={2} name="Bitrate" />
                <Line yAxisId="fps" type="monotone" dataKey="fps" stroke="#34d399" dot={false} strokeWidth={2} name="FPS" />
              </LineChart>
            </ResponsiveContainer>
          </div>

          <div style={{ background: "var(--color-surface-2)", borderRadius: 8, padding: 14 }}>
            <h3 style={{ margin: "0 0 12px", fontSize: 12, fontWeight: 600, color: "var(--color-muted)", textTransform: "uppercase", letterSpacing: "0.06em" }}>
              Packet Loss & Jitter
            </h3>
            <ResponsiveContainer width="100%" height={160}>
              <LineChart data={chartData} margin={{ top: 4, right: 16, left: 0, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
                <XAxis dataKey="ts" tick={{ fill: "var(--color-muted)", fontSize: 10 }} />
                <YAxis yAxisId="loss" orientation="left" tick={{ fill: "var(--color-muted)", fontSize: 10 }} unit="%" />
                <YAxis yAxisId="jitter" orientation="right" tick={{ fill: "var(--color-muted)", fontSize: 10 }} unit=" ms" />
                <Tooltip contentStyle={{ background: "var(--color-surface)", border: "1px solid var(--color-border)", borderRadius: 6, color: "var(--color-text)" }} />
                <Legend wrapperStyle={{ fontSize: 11, color: "var(--color-muted)" }} />
                {/* Threshold indicators */}
                <ReferenceLine yAxisId="loss" y={1} stroke="var(--color-warning, #fbbf24)" strokeDasharray="4 2" label={{ value: "1% threshold", fontSize: 9, fill: "var(--color-warning, #fbbf24)" }} />
                <Line yAxisId="loss" type="monotone" dataKey="pkt_loss" stroke="#f87171" dot={false} strokeWidth={2} name="Packet Loss %" />
                <Line yAxisId="jitter" type="monotone" dataKey="jitter" stroke="#fbbf24" dot={false} strokeWidth={2} name="Jitter ms" />
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
  const [data, setData] = useState<IngestHealthResponse | null>(null);
  const [selected, setSelected] = useState<IngestStream | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await qoeApi.getIngestHealth({
        from: Date.now() - 15 * 60 * 1000, // last 15 min
        to: Date.now(),
      });
      setData(result);
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to load ingest health";
      setError(msg);
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

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 12 }}>
        <h1 style={{ margin: 0, fontSize: 18, fontWeight: 700 }}>Ingest Health</h1>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <span style={{
            width: 7,
            height: 7,
            borderRadius: "50%",
            background: loading ? "var(--color-warning, #fbbf24)" : "var(--color-success, #4ade80)",
            display: "inline-block",
          }} />
          <span style={{ fontSize: 12, color: "var(--color-muted)" }}>
            {loading ? "Refreshing…" : "Live (15s)"}
          </span>
          <button
            onClick={load}
            disabled={loading}
            style={{
              background: "var(--color-surface-2)",
              border: "1px solid var(--color-border)",
              color: "var(--color-muted)",
              borderRadius: 4,
              padding: "4px 10px",
              cursor: "pointer",
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
            borderRadius: 8,
            overflow: "hidden",
          }}>
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
              <thead style={{ background: "var(--color-surface-2)" }}>
                <tr>
                  {["Stream", "App", "Node", "Health", "Drops", ""].map((h) => (
                    <th key={h} style={{
                      padding: "10px 14px",
                      textAlign: h === "" ? "right" : "left",
                      fontSize: 11,
                      color: "var(--color-muted)",
                      textTransform: "uppercase",
                      letterSpacing: "0.06em",
                      fontWeight: 600,
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
                    <td style={{ padding: "10px 14px", fontFamily: "var(--font-mono)", fontWeight: 600, fontSize: 12 }}>{s.stream_id}</td>
                    <td style={{ padding: "10px 14px", color: "var(--color-muted)" }}>{s.app}</td>
                    <td style={{ padding: "10px 14px", color: "var(--color-muted)", fontSize: 12 }}>{s.node_id ?? "—"}</td>
                    <td style={{ padding: "10px 14px" }}>
                      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                        <div style={{
                          width: 60,
                          height: 6,
                          background: "var(--color-surface-2)",
                          borderRadius: 3,
                          overflow: "hidden",
                        }}>
                          <div style={{
                            width: `${s.health_score}%`,
                            height: "100%",
                            background: s.health_score >= 80 ? "#4ade80" : s.health_score >= 50 ? "#fbbf24" : "#f87171",
                            borderRadius: 3,
                          }} />
                        </div>
                        <Badge label={healthLabel(s.health_score)} variant={healthVariant(s.health_score)} />
                      </div>
                    </td>
                    <td style={{ padding: "10px 14px", color: "var(--color-muted)", fontSize: 12 }}>
                      {(s.drop_events ?? []).length > 0 ? (
                        <span style={{ color: "var(--color-error, #e05252)", fontWeight: 600 }}>
                          {s.drop_events!.length} drop{s.drop_events!.length > 1 ? "s" : ""}
                        </span>
                      ) : "—"}
                    </td>
                    <td style={{ padding: "10px 14px", textAlign: "right" }}>
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          setSelected(selected?.stream_id === s.stream_id ? null : s);
                        }}
                        style={{
                          background: "none",
                          border: "1px solid var(--color-border)",
                          color: "var(--color-muted)",
                          borderRadius: 4,
                          padding: "3px 8px",
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
