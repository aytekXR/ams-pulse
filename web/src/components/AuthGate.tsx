/**
 * AuthGate: shows login screen if no token is stored.
 * Also listens for the global "pulse:auth:401" event dispatched by the API
 * client on HTTP 401 — clears the stored (expired/invalid) token and
 * redirects the user back to the token entry screen without a full page reload.
 *
 * Wave-2 carried fix: 401 → React Router redirect to token screen.
 *
 * S14 WO-C OIDC phase-2:
 * - On mount fires GET /auth/oidc/status (plain fetch, no auth) to discover
 *   whether SSO is configured and show a "Sign in with SSO" button.
 * - On mount fires GET /auth/me (plain fetch, cookie rides same-origin) to
 *   detect an existing OIDC session without a localStorage token. A 401 from
 *   /auth/me is handled quietly (no pulse:auth:401 event) — it simply means
 *   the user is not cookie-authenticated.
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

  // S14 WO-C: OIDC discovery state
  const [oidcEnabled, setOidcEnabled] = useState(false);
  const [cookieAuthed, setCookieAuthed] = useState(false);

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

  // S14 WO-C: fire BOTH /auth/oidc/status and /auth/me on every mount.
  // Plain fetch (not apiFetch) so a 401 from /auth/me does NOT fire
  // pulse:auth:401. Cookie rides same-origin automatically for /auth/me.
  useEffect(() => {
    let cancelled = false;

    fetch("/auth/oidc/status")
      .then((r) => (r.ok ? r.json() : null))
      .then((data: { enabled?: boolean } | null) => {
        if (!cancelled && data) {
          setOidcEnabled(!!data.enabled);
        }
      })
      .catch(() => {
        /* ignore — server may not have the endpoint yet */
      });

    fetch("/auth/me")
      .then(async (r) => {
        if (cancelled) return;
        if (!r.ok) return; // 401/403 etc — stay on gate, no event fired
        // Guard against the SPA-fallback fail-open: only trust the response when
        // it is actually JSON (a Go binary returning index.html is HTTP 200 but
        // text/html and must NOT be treated as "authenticated").
        const ct = r.headers.get("content-type") ?? "";
        if (!ct.includes("application/json")) return;
        // Validate the shape per contracts/openapi/pulse-api.yaml /auth/me: the
        // object must carry auth_method so we know it is the real endpoint.
        const data = (await r.json()) as { auth_method?: string };
        if (!cancelled && typeof data?.auth_method === "string") {
          setCookieAuthed(true);
        }
      })
      .catch(() => {
        /* ignore — server unreachable, cookie not present, or JSON parse error */
      });

    return () => {
      cancelled = true;
    };
  }, []);

  // Cookie-authenticated OR bearer-token — render the protected content.
  if (token || cookieAuthed) {
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
              placeholder="plt_…"
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
        {oidcEnabled && (
          <>
            <hr style={{ border: "none", borderTop: "1px solid var(--color-border)", margin: "8px 0" }} />
            <button
              type="button"
              onClick={() => { window.location.href = "/auth/oidc/login"; }}
              style={{
                background: "var(--color-surface-2)",
                color: "var(--color-text)",
                border: "1px solid var(--color-border)",
                borderRadius: 6,
                padding: "10px",
                fontSize: 14,
                fontWeight: 600,
                cursor: "pointer",
              }}
            >
              Sign in with SSO
            </button>
          </>
        )}
        <p style={{ marginTop: 20, marginBottom: 0, fontSize: 12, color: "var(--color-muted)", textAlign: "center" }}>
          Generate a token in Settings → API Tokens.
        </p>
      </div>
    </div>
  );
}
