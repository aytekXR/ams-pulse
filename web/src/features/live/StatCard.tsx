interface Props {
  label: string;
  value: string | number;
  sub?: string;
  accent?: boolean;
}

export function StatCard({ label, value, sub, accent }: Props) {
  return (
    <div
      style={{
        background: "var(--color-surface)",
        border: `1px solid ${accent ? "var(--color-accent)" : "var(--color-border)"}`,
        borderRadius: 8,
        padding: "16px 20px",
        display: "flex",
        flexDirection: "column",
        gap: 4,
        minWidth: 140,
      }}
    >
      <span style={{ fontSize: 12, color: "var(--color-muted)", fontWeight: 500, textTransform: "uppercase", letterSpacing: "0.06em" }}>
        {label}
      </span>
      <span style={{ fontSize: 28, fontWeight: 700, lineHeight: 1.2 }}>
        {typeof value === "number" ? value.toLocaleString() : value}
      </span>
      {sub && (
        <span style={{ fontSize: 12, color: "var(--color-muted)" }}>{sub}</span>
      )}
    </div>
  );
}
