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
    success: "#2a6e3f",
    error: "#7a2020",
    info: "#1c3a5e",
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
              color: "#fff",
              borderRadius: 6,
              padding: "10px 16px",
              fontSize: 14,
              boxShadow: "0 2px 12px rgba(0,0,0,0.4)",
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
