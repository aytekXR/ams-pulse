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
import { SegmentedControl } from "@/components/SegmentedControl";
import type { FleetNode } from "@/lib/api/types";
import { useStatusColors, CHART_COLORS } from "@/lib/chartColors";

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

/**
 * Pure threshold function — maps cpu % to a status tier string.
 * Exported so tests can verify threshold logic AND palette mapping independently.
 * Updated atomically with the LoadBar ternaries below (D-071 palette sweep).
 */
export function cpuStatus(pct: number): "critical" | "warning" | "healthy" {
  if (pct > 80) return "critical";
  if (pct > 60) return "warning";
  return "healthy";
}

/**
 * Pure threshold function — maps mem % to a status tier string.
 * Note: mem "healthy" renders CHART_COLORS[1] (dataviz blue), not status green,
 * because memory at low levels is a normal secondary metric.
 * Exported so tests can verify threshold logic independently of rendering.
 */
export function memStatus(pct: number): "critical" | "warning" | "healthy" {
  if (pct > 85) return "critical";
  if (pct > 70) return "warning";
  return "healthy";
}

/**
 * LoadBar — a filled bar whose HUE encodes the threshold tier.
 *
 * `status` is the tier the colour represents. It is rendered as screen-reader-only
 * text so the tier reaches the accessibility tree instead of living only in a hex
 * value that assistive tech cannot report. (The visible "85%" already carries the
 * underlying datum for sighted users, colour-blind included — the tier is a pure
 * function of that number — so the hue is a redundant encoding, not the sole one.)
 */
function LoadBar({ value, color, status }: { value: number; color: string; status: string }) {
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
        {/* data-testid: the fill's HUE is the whole point of this component and it
            lives only in an inline style, so the tests need a stable handle on it.
            Without one they can only re-assert the palette constants they imported —
            which is what the old FleetPage colour tests did, and why a swap of the
            memory-healthy colour to status-green would have left them all green. */}
        <div
          data-testid="loadbar-fill"
          data-status={status}
          style={{
            width: `${Math.min(value, 100)}%`,
            height: "100%",
            background: color,
            borderRadius: 3,
            transition: "width var(--motion-base)",
          }}
        />
      </div>
      <span style={{ fontSize: 11, color: "var(--color-secondary)", width: 32, textAlign: "right" }}>
        {value.toFixed(0)}%
      </span>
      <span className="sr-only">{status}</span>
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
  // useStatusColors() returns the theme-correct hex set (dark or light).
  const statusColors = useStatusColors();
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
      <div style={{ display: "flex", alignItems: "flex-start", gap: "var(--space-2)" }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontWeight: 700, fontSize: 13, fontFamily: "var(--font-mono)" }}>{node.node_id}</div>
          {node.version && (
            <div style={{ fontSize: 11, color: "var(--color-secondary)", marginTop: 2 }}>v{node.version}</div>
          )}
        </div>
        <Badge label={node.role} variant={roleVariant(node.role)} />
        <Badge label={node.status} variant={statusVariant(node.status)} />
      </div>

      <div style={{ fontSize: 12, color: "var(--color-secondary)" }}>
        Last seen: {lastSeenLabel(node.last_seen)}
      </div>

      {node.cpu_pct != null && (
        <div>
          <div style={{ fontSize: 11, color: "var(--color-secondary)", marginBottom: 3 }}>CPU</div>
          <LoadBar
            value={node.cpu_pct}
            color={statusColors[cpuStatus(node.cpu_pct)]}
            status={cpuStatus(node.cpu_pct)}
          />
        </div>
      )}

      {node.mem_pct != null && (
        <div>
          <div style={{ fontSize: 11, color: "var(--color-secondary)", marginBottom: 3 }}>Memory</div>
          <LoadBar
            value={node.mem_pct}
            color={
              memStatus(node.mem_pct) === "critical"
                ? statusColors.critical
                : memStatus(node.mem_pct) === "warning"
                  ? statusColors.warning
                  // Dataviz blue, NOT statusColors.healthy: normal memory is a
                  // secondary metric, not a health signal. Same hex as before —
                  // CHART_COLORS[1] just names the intent (WAVE-PLAN §4 W2).
                  : CHART_COLORS[1]
            }
            status={memStatus(node.mem_pct)}
          />
        </div>
      )}

      {(node.net_in_mbps != null || node.net_out_mbps != null) && (
        <div style={{ display: "flex", gap: "var(--space-3)", fontSize: 12, color: "var(--color-secondary)" }}>
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

  // Theme-correct status colors for the table view's LoadBar calls.
  const statusColors = useStatusColors();

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
  const degradedCount = nodes.filter((n) => n.status === "degraded").length;
  const originCount = nodes.filter((n) => n.role === "origin").length;
  const edgeCount = nodes.filter((n) => n.role === "edge").length;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      {/* Header */}
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-3)" }}>
        <h1 style={{ margin: 0, fontSize: 18, fontWeight: 700 }}>Fleet</h1>
        <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
          {/* The hex fallbacks these var() calls used to carry were dead AND stale:
              --color-warning and --color-success are defined in BOTH themes, so the
              fallback was unreachable — and each theme's light value differs from the
              fallback it carried, so had one ever been reached it would have painted
              the wrong colour. Same defect Wave 1 removed from QoePage. */}
          {/* Decorative: the sibling text ("Refreshing…" / "Auto-refresh (30s)")
              already states this. Giving the dot its own role would announce the
              same fact twice — and role="status" here would collide with the
              LoadingSpinner's live region. */}
          <span
            aria-hidden="true"
            style={{
              width: 7,
              height: 7,
              borderRadius: "50%",
              background: loading ? "var(--color-warning)" : "var(--color-success)",
              display: "inline-block",
            }}
          />
          <span style={{ fontSize: 12, color: "var(--color-secondary)" }}>
            {loading ? "Refreshing…" : "Auto-refresh (30s)"}
          </span>
          <button
            onClick={load}
            disabled={loading}
            className="btn-secondary"
            style={{
              background: "var(--color-surface-2)",
              border: "1px solid var(--color-border)",
              color: "var(--color-secondary)",
              borderRadius: 4,
              padding: "4px 10px",
              cursor: "pointer",
              fontSize: 11,
            }}
          >
            Refresh
          </button>
          {/* View toggle — a segmented control (fill-background active state), never
              a <Tabs>: see SegmentedControl.tsx for why it is a radiogroup, not a
              tablist. Labels are passed pre-capitalised; the old markup capitalised
              a lowercase value in CSS, which let the DOM text and the accessible
              name drift apart. */}
          <SegmentedControl
            aria-label="Fleet view"
            value={view}
            onChange={(v) => setView(v as "cards" | "table")}
            items={[
              { value: "cards", label: "Cards" },
              { value: "table", label: "Table" },
            ]}
          />
        </div>
      </div>

      {/* Aggregate header */}
      {nodes.length > 0 && (
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(130px, 1fr))", gap: "var(--space-3)" }}>
          {[
            { label: "Total Nodes", value: nodes.length },
            { label: "Up", value: upCount },
            { label: "Degraded", value: degradedCount },
            { label: "Origins", value: originCount },
            { label: "Edges", value: edgeCount },
          ].map(({ label, value }) => (
            <div key={label} style={{
              background: "var(--color-surface)",
              border: "1px solid var(--color-border)",
              borderRadius: 8,
              padding: "var(--space-3) var(--space-4)",
            }}>
              <div style={{ fontSize: 11, color: "var(--color-secondary)", textTransform: "uppercase", letterSpacing: "0.06em", fontWeight: 500, marginBottom: "var(--space-1)" }}>
                {label}
              </div>
              <div style={{ fontSize: 22, fontWeight: 700 }}>{value}</div>
            </div>
          ))}
        </div>
      )}

      <p style={{ margin: 0, fontSize: 12, color: "var(--color-secondary)" }}>
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
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))", gap: "var(--space-4)" }}>
          {nodes.map((node) => <NodeCard key={node.node_id} node={node} />)}
        </div>
      ) : (
        <div style={{ background: "var(--color-surface)", border: "1px solid var(--color-border)", borderRadius: 8, overflow: "hidden" }}>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
            <thead style={{ background: "var(--color-surface-2)" }}>
              <tr>
                {["Node ID", "Role", "Status", "Last Seen", "Version", "CPU", "Memory", "Network"].map((h) => (
                  <th key={h} scope="col" style={{ padding: "10px 14px", textAlign: "left", fontSize: 11, color: "var(--color-secondary)", textTransform: "uppercase", letterSpacing: "0.06em", fontWeight: 600 }}>
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
                  <td style={{ padding: "10px 14px", color: "var(--color-secondary)", fontSize: 12 }}>{lastSeenLabel(node.last_seen)}</td>
                  <td style={{ padding: "10px 14px", color: "var(--color-secondary)", fontSize: 12 }}>{node.version ?? "—"}</td>
                  <td style={{ padding: "10px 14px", minWidth: 100 }}>
                    {node.cpu_pct != null ? (
                      <LoadBar
                        value={node.cpu_pct}
                        color={statusColors[cpuStatus(node.cpu_pct)]}
                        status={cpuStatus(node.cpu_pct)}
                      />
                    ) : "—"}
                  </td>
                  <td style={{ padding: "10px 14px", minWidth: 100 }}>
                    {node.mem_pct != null ? (
                      <LoadBar
                        value={node.mem_pct}
                        color={
                          memStatus(node.mem_pct) === "critical"
                            ? statusColors.critical
                            : memStatus(node.mem_pct) === "warning"
                              ? statusColors.warning
                              : CHART_COLORS[1] // dataviz blue — normal memory level
                        }
                        status={memStatus(node.mem_pct)}
                      />
                    ) : "—"}
                  </td>
                  <td style={{ padding: "10px 14px", fontSize: 12, color: "var(--color-secondary)" }}>
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
