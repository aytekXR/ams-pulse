interface Props {
  label: string;
  variant?: "default" | "success" | "warning" | "error" | "info" | "muted";
}

const variantStyles: Record<NonNullable<Props["variant"]>, { bg: string; color: string }> = {
  default: { bg: "var(--color-accent)", color: "#fff" },
  success: { bg: "#1a4d2e", color: "#4ade80" },
  warning: { bg: "#3d2c00", color: "#fbbf24" },
  error: { bg: "#3d0000", color: "#f87171" },
  info: { bg: "#0f2744", color: "#60a5fa" },
  muted: { bg: "var(--color-surface-2)", color: "var(--color-muted)" },
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
