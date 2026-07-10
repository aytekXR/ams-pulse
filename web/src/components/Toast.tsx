import { createContext, useContext, useState, useCallback } from "react";

interface ToastItem {
  id: string;
  message: string;
  kind: "success" | "error" | "info";
}

interface ToastContextValue {
  toast: (message: string, kind?: ToastItem["kind"]) => void;
}

const ToastContext = createContext<ToastContextValue>({ toast: () => undefined });

export function useToast() {
  return useContext(ToastContext);
}

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);

  const toast = useCallback((message: string, kind: ToastItem["kind"] = "info") => {
    const id = Math.random().toString(36).slice(2);
    setItems((prev) => [...prev, { id, message, kind }]);
    setTimeout(() => {
      setItems((prev) => prev.filter((t) => t.id !== id));
    }, 4000);
  }, []);

  const colors: Record<ToastItem["kind"], string> = {
    success: "rgba(44,229,167,0.12)",
    error:   "rgba(255,92,104,0.12)",
    info:    "rgba(88,166,255,0.12)",
  };

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
        {items.map((item) => (
          <div
            key={item.id}
            role="status"
            style={{
              background: colors[item.kind],
              color: "var(--color-text)",
              borderRadius: 6,
              padding: "10px 16px",
              fontSize: 14,
              boxShadow: "0 24px 64px rgba(0,0,0,0.5)",
              maxWidth: 360,
            }}
          >
            {item.message}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}
