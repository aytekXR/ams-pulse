interface Props {
  label: string;
  variant?: "default" | "success" | "warning" | "error" | "info" | "muted";
}

const variantStyles: Record<NonNullable<Props["variant"]>, { bg: string; color: string }> = {
  default: { bg: "var(--color-accent)", color: "var(--color-on-signal)" },
  success: { bg: "rgba(44,229,167,0.12)", color: "#2CE5A7" },
  warning: { bg: "rgba(255,178,36,0.12)", color: "#FFB224" },
  error:   { bg: "rgba(255,92,104,0.1)",  color: "#FF5C68" },
  info:    { bg: "rgba(88,166,255,0.1)",  color: "#58A6FF" },
  muted:   { bg: "var(--color-surface-2)", color: "var(--color-muted)" },
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
