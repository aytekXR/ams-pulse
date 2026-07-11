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
  success: { bg: "var(--color-success-bg)", color: "var(--color-success)" },
  warning: { bg: "var(--color-warning-bg)", color: "var(--color-warning)" },
  error:   { bg: "var(--color-error-bg)",   color: "var(--color-error)" },
  info:    { bg: "var(--color-info-bg)",     color: "var(--color-info)" },
  muted:   { bg: "var(--color-surface-2)",  color: "var(--color-muted)" },
};

export function Badge({ label, variant = "default" }: Props) {
  const { bg, color } = variantStyles[variant];
  return (
    <span
      style={{
        display: "inline-block",
        background: bg,
        color,
        borderRadius: 4,
        padding: "2px 8px",
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
