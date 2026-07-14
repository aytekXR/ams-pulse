interface Props {
  label: string;
  value: string | number;
  sub?: string;
  accent?: boolean;
}

export function StatCard({ label, value, sub, accent }: Props) {
  const formattedValue = typeof value === "number" ? value.toLocaleString() : value;
  // Compose a screen-reader accessible name for the card group (SC-1).
  const accessibleName = `${label}: ${formattedValue}${sub ? ` ${sub}` : ""}`;

  return (
    <div
      role="group"
      aria-label={accessibleName}
      style={{
        background: "var(--color-surface)",
        border: `1px solid ${accent ? "var(--color-accent)" : "var(--color-border)"}`,
        borderRadius: "var(--radius-control)",
        padding: "var(--card-padding)",
        display: "flex",
        flexDirection: "column",
        gap: "var(--space-1)",
        minWidth: 140,
      }}
    >
      <span style={{ fontSize: 12, color: "var(--color-muted)", fontWeight: 500, textTransform: "uppercase", letterSpacing: "0.06em" }}>
        {label}
      </span>
      {/* data-metric activates `font-variant-numeric: tabular-nums` via global.css
          preventing layout jitter as live values update every 5 s (SC-2). */}
      <span data-metric style={{ fontSize: "var(--metric-size)", fontWeight: 700, lineHeight: 1.2 }}>
        {formattedValue}
      </span>
      {sub && (
        <span style={{ fontSize: 12, color: "var(--color-muted)" }}>{sub}</span>
      )}
    </div>
  );
}
