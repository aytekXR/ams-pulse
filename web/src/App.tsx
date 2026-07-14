/**
 * App shell: router + layout. Feature pages live under src/features/, one
 * folder per PRD feature; FE-01 fills them in per phase. Routes mirror the
 * left-nav information architecture:
 *
 *   /            → live ops dashboard (F1)        — Wave 1
 *   /analytics   → historical audience (F2)       — Wave 1 core, Wave 2 full
 *   /qoe         → viewer QoE (F3)                — Wave 2
 *   /ingest      → publisher/ingest health (F4)   — Wave 2
 *   /alerts      → rules, channels, history (F5)  — Wave 1
 *   /reports     → usage & billing (F6)           — Wave 2
 *   /fleet       → cluster nodes (F7)             — Wave 2
 *   /anomalies   → anomaly flags (F9)             — Wave 3
 *   /probes      → synthetic probes (F10)         — Wave 3
 *   /settings    → sources, tokens, license, users
 */

import { BrowserRouter, Routes, Route, useNavigate, useLocation } from "react-router-dom";
import { useState, useEffect, useRef } from "react";
import { AuthGate } from "@/components/AuthGate";
import { Layout } from "@/components/Layout";
import { ToastProvider } from "@/components/Toast";
import { ThemeProvider, DensityProvider } from "@/lib/ThemeContext";
import { LicenseProvider, useLicense } from "@/lib/LicenseContext";
import { adminApi } from "@/api/client";
import { LiveDashboard } from "@/features/live/LiveDashboard";
import { AnalyticsPage } from "@/features/analytics/AnalyticsPage";
import { QoePage } from "@/features/qoe/QoePage";
import { IngestPage } from "@/features/ingest/IngestPage";
import { AlertsPage } from "@/features/alerts/AlertsPage";
import { ReportsPage } from "@/features/reports/ReportsPage";
import { FleetPage } from "@/features/fleet/FleetPage";
import { AnomaliesPage } from "@/features/anomalies/AnomaliesPage";
import { ProbesPage } from "@/features/probes/ProbesPage";
import { SettingsPage } from "@/features/settings/SettingsPage";
import { OnboardingWizard } from "@/features/settings/OnboardingWizard";
import "@/styles/global.css";

/** localStorage key: set once the user has seen (skipped or completed) the wizard. */
export const ONBOARDING_DISMISSED_KEY = "pulse_onboarding_dismissed";

/**
 * On landing on the dashboard, nudge a genuinely-new user toward the setup wizard
 * so they are not stranded on an empty dashboard with no visible next step.
 *
 * Deliberately conservative, because "no AMS *source*" does NOT mean "not set up":
 * production and the documented quickstart configure AMS via the PULSE_AMS_URL env
 * var and never write the `ams_sources` table, so `getSources()` is legitimately
 * empty on a fully-running instance. The guard therefore redirects **at most once
 * ever** (persisted via ONBOARDING_DISMISSED_KEY, set when the wizard is skipped or
 * completed) and only from `/`, so an env-configured operator sees the wizard once,
 * dismisses it, and is never sent back — and can always reach Settings in between.
 * A failed sources fetch fails open.
 */
export function OnboardingGuard() {
  const navigate = useNavigate();
  const location = useLocation();
  const checked = useRef(false);

  useEffect(() => {
    // Only redirect someone who has just landed on the dashboard, never someone
    // who navigated somewhere on purpose. Firing on every path would mean a user
    // with no sources could not open Settings to add one by hand — the guard would
    // yank them back to the wizard from every route, which is a trap, not a hint.
    if (checked.current || location.pathname !== "/") return;
    // Already skipped/completed the wizard once — never nag again. This is what
    // stops a UI-mode user who declined setup from being redirected on every login.
    if (localStorage.getItem(ONBOARDING_DISMISSED_KEY)) return;
    checked.current = true;

    let cancelled = false;
    // Check /healthz first (unauthenticated): if AMS was configured via the
    // environment, the deployment is set up regardless of the ams_sources table,
    // so stop here and never touch getSources — that both protects a running
    // operator from the wizard and spares the extra call. Only a deployment with
    // no env AMS falls through to the source-list check.
    fetch("/healthz")
      .then((r) => (r.ok ? r.json() : null))
      .catch(() => null)
      .then((health) => {
        if (cancelled || health?.ams_env_configured) return;
        return adminApi
          .getSources()
          .then((res) => {
            // Fail open: redirect only when we positively know there are zero sources.
            if (!cancelled && res.items.length === 0) {
              navigate("/onboarding", { replace: true });
            }
          })
          .catch(() => {
            // Network error must not block a working user.
          });
      });

    return () => {
      cancelled = true;
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps -- intentional: check once on mount

  return null;
}

function AppRoutesInner() {
  const [wsConnected, setWsConnected] = useState(false);
  const navigate = useNavigate();
  const { license } = useLicense();

  return (
    <AuthGate>
      <OnboardingGuard />
      <Layout wsConnected={wsConnected} tier={license?.tier}>
        <Routes>
          <Route path="/" element={<LiveDashboard onConnectionChange={setWsConnected} />} />
          <Route path="/analytics" element={<AnalyticsPage />} />
          <Route path="/qoe" element={<QoePage />} />
          <Route path="/ingest" element={<IngestPage />} />
          <Route path="/alerts" element={<AlertsPage />} />
          <Route path="/reports" element={<ReportsPage />} />
          <Route path="/fleet" element={<FleetPage />} />
          <Route path="/anomalies" element={<AnomaliesPage />} />
          <Route path="/probes" element={<ProbesPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route
            path="/onboarding"
            element={
              <OnboardingWizard
                onComplete={() => {
                  // Remember the dismissal so a user who skips without adding a
                  // source is not redirected back here on the next login.
                  localStorage.setItem(ONBOARDING_DISMISSED_KEY, "1");
                  navigate("/");
                }}
              />
            }
          />
          <Route
            path="*"
            element={
              <div style={{ padding: "3rem", color: "var(--color-muted)", fontSize: 14 }}>
                Page not found — <a href="/">go home</a>
              </div>
            }
          />
        </Routes>
      </Layout>
    </AuthGate>
  );
}

function AppRoutes() {
  return (
    <LicenseProvider>
      <AppRoutesInner />
    </LicenseProvider>
  );
}

export function App() {
  return (
    <BrowserRouter>
      <ThemeProvider>
        <DensityProvider>
          <ToastProvider>
            <AppRoutes />
          </ToastProvider>
        </DensityProvider>
      </ThemeProvider>
    </BrowserRouter>
  );
}
