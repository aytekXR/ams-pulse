interface Props {
  label: string;
  value: string | number;
  sub?: string;
  accent?: boolean;
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

export function StatCard({ label, value, sub, accent, size = "default" }: Props) {
  const formattedValue = typeof value === "number" ? value.toLocaleString() : value;
  // Compose a screen-reader accessible name for the card group (SC-1).
  const accessibleName = `${label}: ${formattedValue}${sub ? ` ${sub}` : ""}`;
  const compact = size === "compact";

  return (
    <div
      role="group"
      aria-label={accessibleName}
      style={{
        background: "var(--color-surface)",
        border: `1px solid ${accent ? "var(--color-accent)" : "var(--color-border)"}`,
        borderRadius: "var(--radius-control)",
        padding: compact ? "14px 16px" : "var(--card-padding)",
        display: "flex",
        flexDirection: "column",
        gap: "var(--space-1)",
        ...(compact ? {} : { minWidth: 140 }),
      }}
    >
      {/* Label colour is --color-secondary, not --color-muted: muted is
          3.50:1 (dark) / 4.36:1 (light) on --color-surface, failing AA for
          normal text at both 11px and 12px. Wave 2 WCAG pass. */}
      <span
        style={{
          fontSize: compact ? 11 : 12,
          color: "var(--color-secondary)",
          fontWeight: 500,
          textTransform: "uppercase",
          letterSpacing: "0.06em",
        }}
      >
        {label}
      </span>
      {/* data-metric activates `font-variant-numeric: tabular-nums` via global.css
          preventing layout jitter as live values update every 5 s (SC-2). */}
      <span
        data-metric
        style={{
          fontSize: compact ? 24 : "var(--metric-size)",
          fontWeight: 700,
          // The compact card inherits the normal line-height: the analytics
          // markup it replaces set none, and forcing 1.2 there would shrink the
          // line box and move the card's height.
          ...(compact ? {} : { lineHeight: 1.2 }),
        }}
      >
        {formattedValue}
      </span>
      {sub && (
        <span style={{ fontSize: 12, color: "var(--color-secondary)" }}>{sub}</span>
      )}
    </div>
  );
}
