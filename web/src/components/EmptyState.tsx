interface Props {
  title: string;
  description?: string;
  action?: React.ReactNode;
}

export function EmptyState({ title, description, action }: Props) {
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        gap: 12,
        padding: "3rem 2rem",
        textAlign: "center",
        color: "var(--color-muted)",
      }}
    >
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden>
        <circle cx="12" cy="12" r="10" />
        <path d="M12 8v4M12 16h.01" />
      </svg>
      <h3 style={{ margin: 0, fontSize: 16, fontWeight: 600, color: "var(--color-text)" }}>
        {title}
      </h3>
      {description && (
        <p style={{ margin: 0, fontSize: 14, maxWidth: 320 }}>{description}</p>
      )}
      {action}
    </div>
  );
}
