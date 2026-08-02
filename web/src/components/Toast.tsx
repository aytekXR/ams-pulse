import { createContext, useContext, useState, useCallback } from "react";

interface ToastItem {
  id: string;
  message: string;
  kind: "success" | "error" | "info";
  /** Exit phase (s111 M1): set 200ms before removal so the .toast[data-leaving]
   * fade in global.css can play; removal is the second timeout below. */
  leaving?: boolean;
}

interface ToastContextValue {
  toast: (message: string, kind?: ToastItem["kind"]) => void;
}

const ToastContext = createContext<ToastContextValue>({ toast: () => undefined });

export function useToast() {
  return useContext(ToastContext);
}

/* s111 D6: kind is encoded by ICON SHAPE + colour, never hue alone (brandkit
 * §2 "state is never encoded by hue alone"). Icons come from the app's own
 * 2px-stroke inline-SVG system (same paths as the nav/anomalies set). */
const icons: Record<ToastItem["kind"], React.ReactNode> = {
  success: (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="var(--color-success)" strokeWidth="2" aria-hidden>
      <path d="M20 6 9 17l-5-5" />
    </svg>
  ),
  error: (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="var(--color-error)" strokeWidth="2" aria-hidden>
      <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
      <line x1="12" y1="9" x2="12" y2="13" />
      <line x1="12" y1="17" x2="12.01" y2="17" />
    </svg>
  ),
  info: (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="var(--color-info)" strokeWidth="2" aria-hidden>
      <circle cx="12" cy="12" r="10" />
      <path d="M12 16v-4M12 8h.01" />
    </svg>
  ),
};

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);

  const toast = useCallback((message: string, kind: ToastItem["kind"] = "info") => {
    const id = Math.random().toString(36).slice(2);
    setItems((prev) => [...prev, { id, message, kind }]);
    // Two-phase removal: mark leaving at 4s (starts the CSS exit fade), drop
    // from the list 200ms later (tokens.json motion.base — the fade duration).
    setTimeout(() => {
      setItems((prev) => prev.map((t) => (t.id === id ? { ...t, leaving: true } : t)));
    }, 4000);
    setTimeout(() => {
      setItems((prev) => prev.filter((t) => t.id !== id));
    }, 4200);
  }, []);

  return (
    <ToastContext.Provider value={{ toast }}>
      {children}
      <div
        aria-live="polite"
        style={{
          position: "fixed",
          bottom: 24,
          right: 24,
          display: "flex",
          flexDirection: "column",
          gap: 8,
          zIndex: 9999,
        }}
      >
        {/* s111 D6: opaque token surface (raised + border) instead of the old
            12%-alpha tint floating over page content; theme-aware via vars.
            The overlay shadow is legal here — toasts ARE overlays
            (tokens.json elevation.note). Enter/exit fades live on the .toast
            class in global.css (inline styles cannot express @starting-style). */}
        {items.map((item) => (
          <div
            key={item.id}
            role="status"
            className="toast"
            data-leaving={item.leaving || undefined}
            style={{
              display: "flex",
              alignItems: "flex-start",
              gap: 8,
              background: "var(--color-raised)",
              border: "1px solid var(--color-border)",
              color: "var(--color-text)",
              borderRadius: "var(--radius-control)",
              padding: "10px 16px",
              fontSize: 14,
              boxShadow: "0 24px 64px rgba(0,0,0,0.5)",
              maxWidth: 360,
            }}
          >
            <span aria-hidden style={{ flexShrink: 0, display: "flex", marginTop: 3 }}>
              {icons[item.kind]}
            </span>
            {item.message}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}
