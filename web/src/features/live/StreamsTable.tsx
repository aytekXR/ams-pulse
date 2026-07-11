import { useRef, useMemo } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Badge } from "@/components/Badge";
import type { LiveStream } from "@/lib/api/types";
import { useDensity } from "@/lib/ThemeContext";

interface Props {
  streams: LiveStream[];
}

// ROW_HEIGHT was 44 (phase-1 divergence vs tokens.json tableRowHeight=40).
// Replaced by useDensity().rowHeight so all three density modes are correct:
//   default=40, compact=32, wall=48 (see density.ts ROW_HEIGHT_MAP).

type BadgeVariant = "success" | "warning" | "error" | "muted" | "default" | "info";

function healthVariant(score: number | undefined): BadgeVariant {
  if (score === undefined || score === null) return "muted";
  if (score >= 80) return "success";
  if (score >= 50) return "warning";
  return "error";
}

function healthLabel(score: number | undefined): string {
  if (score === undefined || score === null) return "unknown";
  if (score >= 80) return "good";
  if (score >= 50) return "degraded";
  return "critical";
}

function stateVariant(state: string | undefined): BadgeVariant {
  switch (state) {
    case "publishing": return "success";
    case "idle": return "warning";
    case "offline": return "muted";
    default: return "muted";
  }
}

function fmtBitrate(kbps: number | undefined): string {
  if (kbps === undefined || kbps === null) return "—";
  if (kbps >= 1000) return `${(kbps / 1000).toFixed(1)} Mbps`;
  return `${kbps} Kbps`;
}

export function StreamsTable({ streams }: Props) {
  const parentRef = useRef<HTMLDivElement>(null);
  // rowHeight is density-aware: default=40, compact=32, wall=48.
  const { rowHeight } = useDensity();

  // eslint-disable-next-line react-hooks/incompatible-library -- TanStack Virtual returns non-memoizable functions; this is expected and documented behavior
  const rowVirtualizer = useVirtualizer({
    count: streams.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => rowHeight,
    overscan: 10,
  });

  const virtualItems = rowVirtualizer.getVirtualItems();
  const totalHeight = rowVirtualizer.getTotalSize();

  const colWidths = useMemo(
    () => ({ stream: "25%", app: "12%", node: "13%", state: "10%", viewers: "10%", bitrate: "13%", health: "10%", id: "7%" }),
    [],
  );

  const headerStyle: React.CSSProperties = {
    fontSize: 11,
    fontWeight: 600,
    color: "var(--color-muted)",
    textTransform: "uppercase",
    letterSpacing: "0.07em",
    padding: "0 12px",
    textAlign: "left" as const,
  };

  const cellStyle: React.CSSProperties = {
    padding: "0 12px",
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
  };

  return (
    <div
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: 8,
        overflow: "hidden",
        display: "flex",
        flexDirection: "column",
      }}
    >
      {/* Header */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          height: 36,
          borderBottom: "1px solid var(--color-border)",
          background: "var(--color-surface-2)",
          flexShrink: 0,
        }}
        role="row"
        aria-rowindex={1}
      >
        <div style={{ ...headerStyle, width: colWidths.stream }}>Stream</div>
        <div style={{ ...headerStyle, width: colWidths.app }}>App</div>
        <div style={{ ...headerStyle, width: colWidths.node }}>Node</div>
        <div style={{ ...headerStyle, width: colWidths.state }}>State</div>
        <div style={{ ...headerStyle, width: colWidths.viewers, textAlign: "right" as const }}>Viewers</div>
        <div style={{ ...headerStyle, width: colWidths.bitrate, textAlign: "right" as const }}>Bitrate</div>
        <div style={{ ...headerStyle, width: colWidths.health }}>Health</div>
      </div>

      {/* Virtualized body */}
      <div
        ref={parentRef}
        style={{ overflowY: "auto", maxHeight: 520, flex: 1 }}
        role="grid"
        aria-label="Active streams"
        aria-rowcount={streams.length + 1}
      >
        {streams.length === 0 ? (
          <div
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              height: 120,
              color: "var(--color-muted)",
              fontSize: 13,
            }}
          >
            No active streams
          </div>
        ) : (
          <div style={{ height: totalHeight, position: "relative" }} role="rowgroup">
            {virtualItems.map((virtualRow) => {
              const stream = streams[virtualRow.index];
              const rowStyle: React.CSSProperties = {
                position: "absolute",
                top: 0,
                left: 0,
                width: "100%",
                height: rowHeight,
                transform: `translateY(${virtualRow.start}px)`,
                display: "flex",
                alignItems: "center",
                borderBottom: "1px solid var(--color-border)",
                fontSize: 13,
              };

              return (
                <div
                  key={stream.stream_id ?? virtualRow.index}
                  style={rowStyle}
                  role="row"
                  aria-rowindex={virtualRow.index + 2}
                >
                  <div
                    style={{
                      ...cellStyle,
                      width: colWidths.stream,
                      fontFamily: "var(--font-mono)",
                      fontSize: 12,
                      color: "var(--color-accent-hover)",
                    }}
                    title={stream.stream_id}
                  >
                    {stream.stream_id ?? "—"}
                  </div>
                  <div style={{ ...cellStyle, width: colWidths.app, color: "var(--color-muted)" }}>
                    {stream.app ?? "—"}
                  </div>
                  <div style={{ ...cellStyle, width: colWidths.node, color: "var(--color-muted)", fontSize: 12 }}>
                    {stream.node_id ?? "—"}
                  </div>
                  <div style={{ ...cellStyle, width: colWidths.state }}>
                    <Badge label={stream.publisher_state ?? "unknown"} variant={stateVariant(stream.publisher_state)} />
                  </div>
                  <div style={{ ...cellStyle, width: colWidths.viewers, textAlign: "right" }}>
                    {(stream.viewers ?? 0).toLocaleString()}
                  </div>
                  <div style={{ ...cellStyle, width: colWidths.bitrate, textAlign: "right", fontFamily: "var(--font-mono)", fontSize: 12 }}>
                    {fmtBitrate(stream.bitrate_kbps)}
                  </div>
                  <div style={{ ...cellStyle, width: colWidths.health }}>
                    <Badge label={healthLabel(stream.health_score)} variant={healthVariant(stream.health_score)} />
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Footer count */}
      <div
        style={{
          padding: "6px 12px",
          borderTop: "1px solid var(--color-border)",
          fontSize: 12,
          color: "var(--color-muted)",
          background: "var(--color-surface-2)",
          flexShrink: 0,
        }}
      >
        {streams.length.toLocaleString()} stream{streams.length !== 1 ? "s" : ""}
      </div>
    </div>
  );
}
