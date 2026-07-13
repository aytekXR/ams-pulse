import { NavLink, useNavigate } from "react-router-dom";
import { clearToken } from "@/api/client";
import type { LiveOverview } from "@/lib/api/types";
import { useTheme } from "@/lib/ThemeContext";
import { useDensity } from "@/lib/ThemeContext";
import type { Density } from "@/lib/density";
import { TrialBanner } from "@/components/TrialBanner";

interface NavItem {
  to: string;
  label: string;
  icon: React.ReactNode;
  wave2?: boolean;
}

const navItems: NavItem[] = [
  {
    to: "/",
    label: "Live",
    icon: (
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
        <circle cx="12" cy="12" r="3" />
        <path d="M12 2v2M12 20v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M2 12h2M20 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42" />
      </svg>
    ),
  },
  {
    to: "/analytics",
    label: "Analytics",
    icon: (
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
        <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
      </svg>
    ),
  },
  {
    to: "/qoe",
    label: "QoE",
    icon: (
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
        <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
        <circle cx="12" cy="12" r="3" />
      </svg>
    ),
  },
  {
    to: "/ingest",
    label: "Ingest",
    icon: (
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
        <polyline points="23 7 13 17 8 12 1 19" />
        <polyline points="17 7 23 7 23 13" />
      </svg>
    ),
  },
  {
    to: "/alerts",
    label: "Alerts",
    icon: (
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
        <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9M13.73 21a2 2 0 0 1-3.46 0" />
      </svg>
    ),
  },
  {
    to: "/reports",
    label: "Reports",
    icon: (
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
        <polyline points="14 2 14 8 20 8" />
        <line x1="16" y1="13" x2="8" y2="13" />
        <line x1="16" y1="17" x2="8" y2="17" />
        <polyline points="10 9 9 9 8 9" />
      </svg>
    ),
  },
  {
    to: "/fleet",
    label: "Fleet",
    icon: (
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
        <rect x="2" y="3" width="7" height="7" />
        <rect x="15" y="3" width="7" height="7" />
        <rect x="2" y="14" width="7" height="7" />
        <rect x="15" y="14" width="7" height="7" />
      </svg>
    ),
  },
  {
    to: "/anomalies",
    label: "Anomalies",
    icon: (
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
        <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
        <line x1="12" y1="9" x2="12" y2="13" />
        <line x1="12" y1="17" x2="12.01" y2="17" />
      </svg>
    ),
  },
  {
    to: "/probes",
    label: "Probes",
    icon: (
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
        <path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22" />
      </svg>
    ),
  },
  {
    to: "/settings",
    label: "Settings",
    icon: (
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
        <circle cx="12" cy="12" r="3" />
        <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
      </svg>
    ),
  },
];

const DENSITY_SEGMENTS: { value: Density; label: string }[] = [
  { value: "default", label: "Default" },
  { value: "compact", label: "Compact" },
  { value: "wall", label: "Wall" },
];

interface LayoutProps {
  children: React.ReactNode;
  wsConnected?: boolean;
  overview?: LiveOverview | null;
  tier?: string;
}

export function Layout({ children, wsConnected, overview: _overview, tier }: LayoutProps) {
  const navigate = useNavigate();
  const { theme, setTheme } = useTheme();
  const { density, setDensity } = useDensity();

  const handleSignOut = () => {
    clearToken();
    // S14 WO-C: fire-and-forget OIDC logout so cookie sessions are server-revoked.
    // Ignore failures — the cookie will expire naturally if the request fails.
    void fetch("/auth/oidc/logout", { method: "POST" }).catch(() => undefined);
    navigate("/");
    window.location.reload();
  };

  const toggleTheme = () => {
    setTheme(theme === "dark" ? "light" : "dark");
  };

  return (
    <div
      style={{
        display: "flex",
        height: "100vh",
        overflow: "hidden",
      }}
    >
      {/* Left nav */}
      <nav
        aria-label="Main navigation"
        style={{
          width: "var(--nav-width)",
          background: "var(--color-surface)",
          borderRight: "1px solid var(--color-border)",
          display: "flex",
          flexDirection: "column",
          flexShrink: 0,
          overflow: "hidden",
        }}
      >
        {/* Logo */}
        <div
          style={{
            height: "var(--header-height)",
            display: "flex",
            alignItems: "center",
            paddingLeft: 16,
            gap: 10,
            borderBottom: "1px solid var(--color-border)",
          }}
        >
          <img src="/logo/pulse-mark.svg" alt="" width="22" height="22" aria-hidden />
          <span style={{ fontWeight: 700, fontSize: 15, letterSpacing: "-0.01em" }}>Pulse</span>
        </div>

        {/* Nav items */}
        <ul
          role="list"
          style={{ flex: 1, margin: 0, padding: "8px 0", listStyle: "none", overflowY: "auto" }}
        >
          {navItems.map((item) => (
            <li key={item.to}>
              <NavLink
                to={item.to}
                end={item.to === "/"}
                style={({ isActive }) => ({
                  display: "flex",
                  alignItems: "center",
                  gap: 10,
                  padding: "8px 16px",
                  paddingLeft: isActive ? 14 : 16,
                  color: isActive ? "var(--color-text)" : "var(--color-muted)",
                  background: isActive
                    ? `rgba(var(--color-accent-rgb),0.1)`
                    : "transparent",
                  borderLeft: isActive
                    ? `2px solid var(--color-accent)`
                    : "2px solid transparent",
                  textDecoration: "none",
                  fontSize: 13,
                  fontWeight: isActive ? 600 : 400,
                  borderRadius: 4,
                  margin: "1px 8px",
                  transition: `background var(--motion-fast), color var(--motion-fast)`,
                })}
              >
                {item.icon}
                {item.label}
                {item.wave2 && (
                  <span
                    style={{
                      marginLeft: "auto",
                      fontSize: 9,
                      background: "var(--color-surface-2)",
                      color: "var(--color-muted)",
                      borderRadius: 3,
                      padding: "1px 4px",
                      fontWeight: 600,
                    }}
                  >
                    W2
                  </span>
                )}
              </NavLink>
            </li>
          ))}
        </ul>

        {/* Preferences cluster — ABOVE sign-out */}
        <div
          style={{
            padding: "10px 12px 8px",
            borderTop: "1px solid var(--color-border)",
            display: "flex",
            flexDirection: "column",
            gap: 8,
          }}
        >
          {/* Theme toggle */}
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <button
              onClick={toggleTheme}
              aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
              title={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
              style={{
                background: "none",
                border: "1px solid var(--color-border)",
                color: "var(--color-secondary)",
                cursor: "pointer",
                padding: "3px 8px",
                borderRadius: 6,
                display: "flex",
                alignItems: "center",
                gap: 5,
                fontSize: 11,
                transition: `border-color var(--motion-fast), color var(--motion-fast)`,
              }}
            >
              {theme === "dark" ? (
                /* Sun icon */
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
                  <circle cx="12" cy="12" r="5" />
                  <line x1="12" y1="1" x2="12" y2="3" />
                  <line x1="12" y1="21" x2="12" y2="23" />
                  <line x1="4.22" y1="4.22" x2="5.64" y2="5.64" />
                  <line x1="18.36" y1="18.36" x2="19.78" y2="19.78" />
                  <line x1="1" y1="12" x2="3" y2="12" />
                  <line x1="21" y1="12" x2="23" y2="12" />
                  <line x1="4.22" y1="19.78" x2="5.64" y2="18.36" />
                  <line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
                </svg>
              ) : (
                /* Moon icon */
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
                  <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
                </svg>
              )}
              {theme === "dark" ? "Light" : "Dark"}
            </button>
          </div>

          {/* Density 3-segment control */}
          <div
            role="group"
            aria-label="Display density"
            style={{
              display: "flex",
              borderRadius: 6,
              overflow: "hidden",
              border: "1px solid var(--color-border)",
            }}
          >
            {DENSITY_SEGMENTS.map((seg) => (
              <button
                key={seg.value}
                onClick={() => setDensity(seg.value)}
                aria-pressed={density === seg.value}
                style={{
                  flex: 1,
                  padding: "3px 0",
                  background:
                    density === seg.value
                      ? `rgba(var(--color-accent-rgb),0.15)`
                      : "transparent",
                  border: "none",
                  borderRight:
                    seg.value !== "wall" ? "1px solid var(--color-border)" : "none",
                  color:
                    density === seg.value
                      ? "var(--color-accent)"
                      : "var(--color-muted)",
                  cursor: "pointer",
                  fontSize: 10,
                  fontWeight: density === seg.value ? 600 : 400,
                  transition: `background var(--motion-fast), color var(--motion-fast)`,
                }}
              >
                {seg.label}
              </button>
            ))}
          </div>
        </div>

        {/* Footer — sign-out */}
        <div
          style={{
            padding: "10px 16px",
            borderTop: "1px solid var(--color-border)",
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
          }}
        >
          {tier && (
            <span
              style={{
                fontSize: 11,
                fontWeight: 600,
                color: "var(--color-accent-hover)",
                textTransform: "uppercase",
                letterSpacing: "0.06em",
              }}
            >
              {tier}
            </span>
          )}
          <button
            onClick={handleSignOut}
            title="Sign out"
            style={{
              background: "none",
              border: "none",
              color: "var(--color-muted)",
              cursor: "pointer",
              padding: 4,
              borderRadius: 4,
              display: "flex",
              alignItems: "center",
            }}
          >
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
              <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9" />
            </svg>
          </button>
        </div>
      </nav>

      {/* Main area */}
      <div style={{ flex: 1, display: "flex", flexDirection: "column", overflow: "hidden" }}>
        {/* Header */}
        <header
          style={{
            height: "var(--header-height)",
            background: "var(--color-surface)",
            borderBottom: "1px solid var(--color-border)",
            display: "flex",
            alignItems: "center",
            paddingInline: 20,
            gap: 12,
            flexShrink: 0,
          }}
        >
          <div style={{ flex: 1 }} />
          {/* Connection status */}
          <div
            title={wsConnected ? "Live push connected" : "Polling mode (WebSocket unavailable)"}
            style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12, color: "var(--color-muted)" }}
          >
            <span
              style={{
                width: 7,
                height: 7,
                borderRadius: "50%",
                background: wsConnected ? "var(--color-success)" : "var(--color-warning)",
                display: "inline-block",
              }}
            />
            {wsConnected ? "Live" : "Polling"}
          </div>
        </header>

        {/* Trial / expiry banner — between header and main; flexShrink 0 preserves density modes */}
        <TrialBanner />

        {/* Page content */}
        <main
          style={{ flex: 1, overflowY: "auto", padding: 24 }}
          id="main-content"
        >
          {children}
        </main>
      </div>
    </div>
  );
}
