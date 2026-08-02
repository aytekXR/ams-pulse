import { useLiveDashboard } from "./useLiveDashboard";
import { StatCard } from "./StatCard";
import { ProtocolDonut } from "./ProtocolDonut";
import { StreamsTable } from "./StreamsTable";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";

export function LiveDashboard({ onConnectionChange }: { onConnectionChange?: (v: boolean) => void }) {
  const { overview, streams, connected, loading, error, refresh } = useLiveDashboard();

  // Notify parent layout of connection status
  if (onConnectionChange) {
    // intentional: call during render, values are stable references
    onConnectionChange(connected);
  }

  const totalViewers = overview?.total_viewers ?? 0;
  // LiveOverview uses total_publishers (per generated schema)
  const activePublishers = overview?.total_publishers ?? 0;
  const nodes = overview?.nodes ?? [];
  const avgCpu = nodes.length
    ? Math.round(nodes.reduce((s, n) => s + (n.cpu_pct ?? 0), 0) / nodes.length)
    : null;
  const avgRam = nodes.length
    ? Math.round(nodes.reduce((s, n) => s + (n.mem_pct ?? 0), 0) / nodes.length)
    : null;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-5)" }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <h1 className="page-title">Live Dashboard</h1>
        {/* s111 D1/D7: muted text purged; .btn-secondary owns color/border so
            hover can win (do not re-add them inline). */}
        <button
          onClick={refresh}
          className="btn-secondary"
          style={{
            background: "var(--color-surface-2)",
            borderRadius: "var(--radius-control)",
            padding: "6px 12px",
            cursor: "pointer",
            fontSize: 12,
          }}
        >
          Refresh
        </button>
      </div>

      {error && <ErrorBanner message={error} onRetry={refresh} />}

      {loading && !overview ? (
        <LoadingSpinner label="Loading live data…" />
      ) : (
        <>
          {/* Stat cards */}
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(auto-fill, minmax(160px, 1fr))",
              gap: "var(--space-3)",
            }}
          >
            <StatCard
              label="Viewers"
              value={totalViewers}
              sub="concurrent"
              accent
            />
            <StatCard
              label="Publishers"
              value={activePublishers}
              sub="active streams"
            />
            {avgCpu !== null && (
              <StatCard
                label="Avg CPU"
                value={`${avgCpu}%`}
                sub={`${nodes.length} node${nodes.length !== 1 ? "s" : ""}`}
              />
            )}
            {avgRam !== null && (
              <StatCard
                label="Avg RAM"
                value={`${avgRam}%`}
                sub="memory used"
              />
            )}
            <StatCard
              label="Streams"
              value={streams.length}
              sub="active"
            />
          </div>

          {/* Overview row: protocol donut + per-app breakdown */}
          <div
            style={{ display: "grid", gridTemplateColumns: "260px 1fr", gap: "var(--space-4)", alignItems: "start" }}
          >
            <div
              style={{
                background: "var(--color-surface)",
                border: "1px solid var(--color-border)",
                borderRadius: "var(--radius-card)",
                padding: "var(--space-4)",
              }}
            >
              <h2 className="label" style={{ margin: "0 0 var(--space-3)" }}>
                Protocol mix
              </h2>
              <ProtocolDonut data={overview?.protocol_mix ?? { webrtc: 0, hls: 0, rtmp: 0, dash: 0, other: 0 }} />
            </div>

            <div
              style={{
                background: "var(--color-surface)",
                border: "1px solid var(--color-border)",
                borderRadius: "var(--radius-card)",
                padding: "var(--space-4)",
              }}
            >
              <h2 className="label" style={{ margin: "0 0 var(--space-3)" }}>
                By application
              </h2>
              {(overview?.apps ?? []).length === 0 ? (
                <p style={{ color: "var(--color-secondary)", fontSize: 12, margin: 0 }}>No data</p>
              ) : (
                <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
                  <thead>
                    <tr>
                      <th className="label" style={{ textAlign: "left", padding: "var(--space-1) var(--space-2)" }}>App</th>
                      <th className="label" style={{ textAlign: "right", padding: "var(--space-1) var(--space-2)" }}>Viewers</th>
                      <th className="label" style={{ textAlign: "right", padding: "var(--space-1) var(--space-2)" }}>Publishers</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(overview?.apps ?? []).map((app) => (
                      <tr
                        key={app.app}
                        style={{ borderTop: "1px solid var(--color-border)" }}
                      >
                        <td style={{ padding: "var(--space-2)", fontFamily: "var(--font-mono)", fontSize: 12 }}>{app.app}</td>
                        {/* s111 D12: data-numeric → tabular-nums (global.css);
                            these cells update live and must not jitter. */}
                        <td data-numeric style={{ padding: "var(--space-2)", textAlign: "right" }}>{(app.viewers ?? 0).toLocaleString()}</td>
                        <td data-numeric style={{ padding: "var(--space-2)", textAlign: "right" }}>{app.publishers ?? 0}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          </div>

          {/* Streams table (virtualized) */}
          <div>
            <h2 className="label" style={{ margin: "0 0 var(--space-3)" }}>
              Active streams ({streams.length.toLocaleString()})
            </h2>
            <StreamsTable streams={streams} />
          </div>
        </>
      )}
    </div>
  );
}
