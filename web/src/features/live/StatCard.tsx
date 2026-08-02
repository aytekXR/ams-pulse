interface Props {
  label: string;
  value: string | number;
  sub?: string;
  accent?: boolean;
  /** s111 D9: unit suffix rendered inside the value line (e.g. "ms", "%"). */
  unit?: string;
  /** s111 D9: threshold colour for the value (e.g. "var(--color-warning)"). */
  valueColor?: string;
  /** s111 D9: node rendered beside the value (e.g. a threshold <Badge>). */
  trailing?: React.ReactNode;
  /**
   * Visual density (Wave 2).
   *
   * "default" — the live-dashboard card: density-responsive, driven by the
   *   --card-padding / --metric-size tokens (24px/40px normal, 16/32 compact,
   *   32/64 wall).
   * "compact" — the analytics totals card: fixed 14px 16px padding, 24px value.
   *
   * The variant exists because a 1:1 swap of Analytics' inline stat-card markup
   * for the default StatCard is NOT pixel-neutral — it would inflate padding
   * 14→24px and the value 24→40px. These waves may not move pixels, so the
   * Analytics geometry is carried verbatim instead of being silently restyled.
   * Whether Analytics SHOULD adopt the density-responsive default look is a
   * design decision for the operator, not a refactor's to make; it is filed in
   * docs/operator-expected.md rather than assumed here.
   */
  size?: "default" | "compact";
}

export function StatCard({ label, value, sub, accent, unit, valueColor, trailing, size = "default" }: Props) {
  const formattedValue = typeof value === "number" ? value.toLocaleString() : value;
  // Compose a screen-reader accessible name for the card group (SC-1).
  const accessibleName = `${label}: ${formattedValue}${unit ? ` ${unit}` : ""}${sub ? ` ${sub}` : ""}`;
  const compact = size === "compact";

  return (
    <div
      role="group"
      aria-label={accessibleName}
      style={{
        background: "var(--color-surface)",
        border: `1px solid ${accent ? "var(--color-accent)" : "var(--color-border)"}`,
        // s111 D10: cards sit on the card radius (12), not the control radius.
        borderRadius: "var(--radius-card)",
        padding: compact ? "14px 16px" : "var(--card-padding)",
        display: "flex",
        flexDirection: "column",
        gap: "var(--space-1)",
        ...(compact ? {} : { minWidth: 140 }),
      }}
    >
      {/* s111 D11: the tokenized label role (global.css .label — 11px mono,
          0.1em, uppercase, --color-secondary per the binding WCAG table). */}
      <span className="label">{label}</span>
      {/* data-metric activates `font-variant-numeric: tabular-nums` via global.css
          preventing layout jitter as live values update every 5 s (SC-2). */}
      <div style={{ display: "flex", alignItems: "baseline", gap: "var(--space-2)", flexWrap: "wrap" }}>
        <span
          data-metric
          style={{
            fontSize: compact ? 24 : "var(--metric-size)",
            fontWeight: 700,
            // The compact card inherits the normal line-height: the analytics
            // markup it replaces set none, and forcing 1.2 there would shrink the
            // line box and move the card's height.
            ...(compact ? {} : { lineHeight: 1.2 }),
            ...(valueColor ? { color: valueColor } : {}),
          }}
        >
          {formattedValue}
          {unit && (
            <span style={{ fontSize: 14, fontWeight: 400, color: "var(--color-secondary)", marginLeft: "var(--space-1)" }}>
              {unit}
            </span>
          )}
        </span>
        {trailing}
      </div>
      {sub && (
        <span style={{ fontSize: 12, color: "var(--color-secondary)" }}>{sub}</span>
      )}
    </div>
  );
}
