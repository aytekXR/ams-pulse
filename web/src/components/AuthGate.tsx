/**
 * AuthGate: shows login screen if no token is stored.
 * Also listens for the global "pulse:auth:401" event dispatched by the API
 * client on HTTP 401 — clears the stored (expired/invalid) token and
 * redirects the user back to the token entry screen without a full page reload.
 *
 * Wave-2 carried fix: 401 → React Router redirect to token screen.
 */
import { useState, useEffect } from "react";
import { getToken, setToken, clearToken } from "@/api/client";

interface Props {
  children: React.ReactNode;
}

export function AuthGate({ children }: Props) {
  const [token, _setToken] = useState(() => getToken());
  const [input, setInput] = useState("");
  const [error, setError] = useState<string | null>(null);

  // Wave-2: listen for 401 events from the API client to auto-redirect
  useEffect(() => {
    const handler = () => {
      clearToken();
      _setToken(null);
      setError("Session expired or token revoked. Please enter your token again.");
    };
    window.addEventListener("pulse:auth:401", handler);
    return () => window.removeEventListener("pulse:auth:401", handler);
  }, []);

  if (token) {
    return <>{children}</>;
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = input.trim();
    if (!trimmed) {
      setError("Token is required");
      return;
    }
    setToken(trimmed);
    _setToken(trimmed);
    setError(null);
  };

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "var(--color-bg)",
      }}
    >
      <div
        style={{
          background: "var(--color-surface)",
          border: "1px solid var(--color-border)",
          borderRadius: 12,
          padding: "2.5rem 2rem",
          width: "100%",
          maxWidth: 400,
          boxShadow: "0 24px 64px rgba(0,0,0,0.5)",
        }}
      >
        <div style={{ textAlign: "center", marginBottom: 28 }}>
          <img src="/logo/pulse-mark.svg" alt="Pulse" width="40" height="40" style={{ marginBottom: 12 }} />
          <h1 style={{ margin: "0 0 4px", fontSize: 22, fontWeight: 700 }}>Pulse</h1>
          <p style={{ margin: 0, fontSize: 14, color: "var(--color-muted)" }}>
            Enter your API token to continue
          </p>
        </div>
        <form onSubmit={handleSubmit} style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <label style={{ fontSize: 13, fontWeight: 500, color: "var(--color-muted)" }}>
            API Token
            <input
              type="password"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="pulse_tok_…"
              autoFocus
              style={{
                display: "block",
                width: "100%",
                marginTop: 6,
                background: "var(--color-surface-2)",
                border: `1px solid ${error ? "var(--color-error)" : "var(--color-border)"}`,
                borderRadius: 6,
                padding: "8px 12px",
                color: "var(--color-text)",
                fontSize: 14,
                outline: "none",
                boxSizing: "border-box",
              }}
            />
          </label>
          {error && (
            <p style={{ margin: 0, fontSize: 12, color: "var(--color-error)" }} role="alert">
              {error}
            </p>
          )}
          <button
            type="submit"
            style={{
              marginTop: 4,
              background: "var(--color-accent)",
              color: "var(--color-on-signal)",
              border: "none",
              borderRadius: 6,
              padding: "10px",
              fontSize: 14,
              fontWeight: 600,
              cursor: "pointer",
            }}
          >
            Sign in
          </button>
        </form>
        <p style={{ marginTop: 20, marginBottom: 0, fontSize: 12, color: "var(--color-muted)", textAlign: "center" }}>
          Generate a token in Settings → API Tokens.
        </p>
      </div>
    </div>
  );
}
