interface Props {
  message: string;
  onRetry?: () => void;
}

export function ErrorBanner({ message, onRetry }: Props) {
  return (
    <div
      role="alert"
      style={{
        background: "var(--color-error-bg, rgba(255,92,104,0.1))",
        border: "1px solid var(--color-error, #FF5C68)",
        borderRadius: 6,
        padding: "12px 16px",
        display: "flex",
        alignItems: "center",
        gap: 12,
        color: "var(--color-error, #FF5C68)",
      }}
    >
      <span style={{ flex: 1, fontSize: 14 }}>{message}</span>
      {onRetry && (
        <button
          onClick={onRetry}
          style={{
            background: "none",
            border: "1px solid currentColor",
            color: "inherit",
            borderRadius: 4,
            padding: "4px 10px",
            cursor: "pointer",
            fontSize: 12,
          }}
        >
          Retry
        </button>
      )}
    </div>
  );
}
