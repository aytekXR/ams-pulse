interface Props {
  feature: string;
  wave?: string;
}

export function ComingSoon({ feature, wave = "Wave 2" }: Props) {
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        gap: 16,
        height: "100%",
        padding: "4rem 2rem",
        textAlign: "center",
        // s111 D3: secondary, not muted — the 14px <p> inherits this colour
        // and muted fails the binding WCAG table for normal text. The icon
        // keeps muted separately (non-text UI, 3:1 bar).
        color: "var(--color-secondary)",
      }}
    >
      <svg width="56" height="56" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" style={{ color: "var(--color-muted)" }} aria-hidden>
        <rect x="2" y="3" width="20" height="14" rx="2" />
        <path d="M8 21h8M12 17v4" />
      </svg>
      <h2 className="page-title" style={{ color: "var(--color-text)" }}>
        {feature}
      </h2>
      <p style={{ margin: 0, fontSize: 14, maxWidth: 360 }}>
        Coming in {wave}. This surface is under construction and will be available in the next build.
      </p>
    </div>
  );
}
