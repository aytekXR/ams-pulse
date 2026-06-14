/**
 * F7 — Fleet View (/fleet)
 *
 * Node cards/table: role badge origin/edge, status, last_seen,
 * version, cpu/mem/net load bars.
 * Aggregate header with dedup note.
 * Auto-refresh (nodes appear within 2 min of first contact).
 */
import { useState, useEffect, useCallback } from "react";
import { fleetApi, ApiError } from "@/api/client";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";
import { EmptyState } from "@/components/EmptyState";
import { Badge } from "@/components/Badge";
import type { FleetNode } from "@/lib/api/types";

function statusVariant(status: string): "success" | "warning" | "error" {
  if (status === "up") return "success";
  if (status === "degraded") return "warning";
  return "error";
}

function roleVariant(role: string): "info" | "muted" | "default" {
  if (role === "origin") return "info";
  if (role === "edge") return "muted";
  return "default";
}

function LoadBar({ value, color }: { value: number; color: string }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
      <div style={{
        flex: 1,
        height: 5,
        background: "var(--color-surface-2)",
        borderRadius: 3,
        overflow: "hidden",
        minWidth: 60,
      }}>
        <div style={{
          width: `${Math.min(value, 100)}%`,
          height: "100%",
          background: color,
          borderRadius: 3,
          transition: "width 0.3s ease",
        }} />
      </div>
      <span style={{ fontSize: 11, color: "var(--color-muted)", width: 32, textAlign: "right" }}>
        {value.toFixed(0)}%
      </span>
    </div>
  );
}

function lastSeenLabel(ts: number): string {
  const diff = Date.now() - ts;
  if (diff < 60_000) return `${Math.round(diff / 1000)}s ago`;
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  return new Date(ts).toLocaleTimeString();
}

interface NodeCardProps {
  node: FleetNode;
}

function NodeCard({ node }: NodeCardProps) {
  return (
    <div style={{
      background: "var(--color-surface)",
      border: "1px solid var(--color-border)",
      borderRadius: 8,
      padding: "16px 20px",
      display: "flex",
      flexDirection: "column",
      gap: 10,
    }}>
      <div style={{ display: "flex", alignItems: "flex-start", gap: 8 }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontWeight: 700, fontSize: 13, fontFamily: "var(--font-mono)" }}>{node.node_id}</div>
          {node.version && (
            <div style={{ fontSize: 11, color: "var(--color-muted)", marginTop: 2 }}>v{node.version}</div>
          )}
        </div>
        <Badge label={node.role} variant={roleVariant(node.role)} />
        <Badge label={node.status} variant={statusVariant(node.status)} />
      </div>

      <div style={{ fontSize: 12, color: "var(--color-muted)" }}>
        Last seen: {lastSeenLabel(node.last_seen)}
      </div>

      {node.cpu_pct != null && (
        <div>
          <div style={{ fontSize: 11, color: "var(--color-muted)", marginBottom: 3 }}>CPU</div>
          <LoadBar value={node.cpu_pct} color={node.cpu_pct > 80 ? "#f87171" : node.cpu_pct > 60 ? "#fbbf24" : "#4ade80"} />
        </div>
      )}

      {node.mem_pct != null && (
        <div>
          <div style={{ fontSize: 11, color: "var(--color-muted)", marginBottom: 3 }}>Memory</div>
          <LoadBar value={node.mem_pct} color={node.mem_pct > 85 ? "#f87171" : node.mem_pct > 70 ? "#fbbf24" : "#60a5fa"} />
        </div>
      )}

      {(node.net_in_mbps != null || node.net_out_mbps != null) && (
        <div style={{ display: "flex", gap: 12, fontSize: 12, color: "var(--color-muted)" }}>
          {node.net_in_mbps != null && <span>↓ {node.net_in_mbps.toFixed(1)} Mbps</span>}
          {node.net_out_mbps != null && <span>↑ {node.net_out_mbps.toFixed(1)} Mbps</span>}
        </div>
      )}
    </div>
  );
}

export function FleetPage() {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [nodes, setNodes] = useState<FleetNode[]>([]);
  const [view, setView] = useState<"cards" | "table">("cards");

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await fleetApi.listNodes();
      setNodes(result.items ?? []);
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to load fleet nodes";
      setError(msg);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
    // Auto-refresh every 30s (node discovery ≤2 min)
    const timer = setInterval(() => { void load(); }, 30_000);
    return () => clearInterval(timer);
  }, [load]);

  const upCount = nodes.filter((n) => n.status === "up").length;
  const downCount = nodes.filter((n) => n.status === "down").length;
  const degradedCount = nodes.filter((n) => n.status === "degraded").length;
  const originCount = nodes.filter((n) => n.role === "origin").length;
  const edgeCount = nodes.filter((n) => n.role === "edge").length;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      {/* Header */}
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 12 }}>
        <h1 style={{ margin: 0, fontSize: 18, fontWeight: 700 }}>Fleet</h1>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <span style={{
            width: 7,
            height: 7,
            borderRadius: "50%",
            background: loading ? "var(--color-warning, #fbbf24)" : "var(--color-success, #4ade80)",
            display: "inline-block",
          }} />
          <span style={{ fontSize: 12, color: "var(--color-muted)" }}>
            {loading ? "Refreshing…" : "Auto-refresh (30s)"}
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
          {/* View toggle */}
          <div style={{ display: "flex", border: "1px solid var(--color-border)", borderRadius: 4, overflow: "hidden" }}>
            {(["cards", "table"] as const).map((v) => (
              <button
                key={v}
                onClick={() => setView(v)}
                style={{
                  background: view === v ? "var(--color-surface-2)" : "transparent",
                  border: "none",
                  color: view === v ? "var(--color-text)" : "var(--color-muted)",
                  padding: "4px 10px",
                  cursor: "pointer",
                  fontSize: 11,
                  fontWeight: view === v ? 600 : 400,
                  textTransform: "capitalize",
                }}
              >
                {v}
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* Aggregate header */}
      {nodes.length > 0 && (
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(130px, 1fr))", gap: 12 }}>
          {[
            { label: "Total Nodes", value: nodes.length },
            { label: "Up", value: upCount },
            { label: "Degraded", value: degradedCount },
            { label: "Down", value: downCount },
            { label: "Origins", value: originCount },
            { label: "Edges", value: edgeCount },
          ].map(({ label, value }) => (
            <div key={label} style={{
              background: "var(--color-surface)",
              border: "1px solid var(--color-border)",
              borderRadius: 8,
              padding: "12px 16px",
            }}>
              <div style={{ fontSize: 11, color: "var(--color-muted)", textTransform: "uppercase", letterSpacing: "0.06em", fontWeight: 500, marginBottom: 4 }}>
                {label}
              </div>
              <div style={{ fontSize: 22, fontWeight: 700 }}>{value}</div>
            </div>
          ))}
        </div>
      )}

      <p style={{ margin: 0, fontSize: 12, color: "var(--color-muted)" }}>
        Nodes are auto-discovered within 2 minutes of first contact. Origin and edge nodes are de-duplicated by node_id.
      </p>

      {error && <ErrorBanner message={error} onRetry={load} />}

      {loading && nodes.length === 0 ? (
        <LoadingSpinner label="Discovering fleet nodes…" />
      ) : nodes.length === 0 ? (
        <EmptyState
          title="No fleet nodes discovered"
          description="Cluster nodes will appear here once Pulse detects them. Discovery takes up to 2 minutes from first contact."
        />
      ) : view === "cards" ? (
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))", gap: 16 }}>
          {nodes.map((node) => <NodeCard key={node.node_id} node={node} />)}
        </div>
      ) : (
        <div style={{ background: "var(--color-surface)", border: "1px solid var(--color-border)", borderRadius: 8, overflow: "hidden" }}>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
            <thead style={{ background: "var(--color-surface-2)" }}>
              <tr>
                {["Node ID", "Role", "Status", "Last Seen", "Version", "CPU", "Memory", "Network"].map((h) => (
                  <th key={h} style={{ padding: "10px 14px", textAlign: "left", fontSize: 11, color: "var(--color-muted)", textTransform: "uppercase", letterSpacing: "0.06em", fontWeight: 600 }}>
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {nodes.map((node, i) => (
                <tr key={node.node_id} style={{ borderTop: i === 0 ? "none" : "1px solid var(--color-border)" }}>
                  <td style={{ padding: "10px 14px", fontFamily: "var(--font-mono)", fontSize: 12, fontWeight: 600 }}>{node.node_id}</td>
                  <td style={{ padding: "10px 14px" }}><Badge label={node.role} variant={roleVariant(node.role)} /></td>
                  <td style={{ padding: "10px 14px" }}><Badge label={node.status} variant={statusVariant(node.status)} /></td>
                  <td style={{ padding: "10px 14px", color: "var(--color-muted)", fontSize: 12 }}>{lastSeenLabel(node.last_seen)}</td>
                  <td style={{ padding: "10px 14px", color: "var(--color-muted)", fontSize: 12 }}>{node.version ?? "—"}</td>
                  <td style={{ padding: "10px 14px", minWidth: 100 }}>
                    {node.cpu_pct != null ? (
                      <LoadBar value={node.cpu_pct} color={node.cpu_pct > 80 ? "#f87171" : node.cpu_pct > 60 ? "#fbbf24" : "#4ade80"} />
                    ) : "—"}
                  </td>
                  <td style={{ padding: "10px 14px", minWidth: 100 }}>
                    {node.mem_pct != null ? (
                      <LoadBar value={node.mem_pct} color={node.mem_pct > 85 ? "#f87171" : node.mem_pct > 70 ? "#fbbf24" : "#60a5fa"} />
                    ) : "—"}
                  </td>
                  <td style={{ padding: "10px 14px", fontSize: 12, color: "var(--color-muted)" }}>
                    {node.net_in_mbps != null || node.net_out_mbps != null ? (
                      <span>↓{(node.net_in_mbps ?? 0).toFixed(1)} ↑{(node.net_out_mbps ?? 0).toFixed(1)} Mbps</span>
                    ) : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
