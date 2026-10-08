/**
 * Badge — semantic status pill.
 *
 * All colour values reference CSS custom properties so badges follow the
 * active theme (dark/light) automatically. The backing vars are declared in
 * global.css under both :root (dark) and [data-theme="light"].
 */

interface Props {
  label: string;
  variant?: "default" | "success" | "warning" | "error" | "info" | "muted";
}

const variantStyles: Record<NonNullable<Props["variant"]>, { bg: string; color: string }> = {
  default: { bg: "var(--color-accent)",    color: "var(--color-on-signal)" },
  // *-text: the hue in dark, textPrimary in light, where the hue fails AA as a label on
  // its own tint (global.css; S126).
  success: { bg: "var(--color-success-bg)", color: "var(--color-success-text)" },
  warning: { bg: "var(--color-warning-bg)", color: "var(--color-warning-text)" },
  error:   { bg: "var(--color-error-bg)",   color: "var(--color-error-text)" },
  info:    { bg: "var(--color-info-bg)",     color: "var(--color-info)" },
  // --color-secondary, not --color-muted: at 11px the badge label is normal
  // text, and muted gives 3.50:1 (dark) / 4.36:1 (light) on --color-surface-2 —
  // below the 4.5:1 the brandkit WCAG table requires. Reached from FleetPage
  // (edge-role badges, Wave 2's surface) and AlertsPage (disabled/muted rules).
  // Wave 2 WCAG pass.
  muted:   { bg: "var(--color-surface-2)",  color: "var(--color-secondary)" },
};

export function Badge({ label, variant = "default" }: Props) {
  const { bg, color } = variantStyles[variant];
  return (
    // s111 D10: radius 4 → var(--radius-pill) — the component calls itself a
    // "semantic status pill" and tokens.json radius is {control 8, card 12,
    // pill 999}, "nothing else" (design-rationale §3). Horizontal padding
    // grows 8→10 so the rounded caps keep the cap-height ratio.
    <span
      style={{
        display: "inline-block",
        background: bg,
        color,
        borderRadius: "var(--radius-pill)",
        padding: "2px 10px",
        fontSize: 11,
        fontWeight: 600,
        letterSpacing: "0.05em",
        textTransform: "uppercase",
      }}
    >
      {label}
    </span>
  );
}
