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

/**
 * Fires once per authenticated session (on mount of the authenticated subtree).
 * If the deployment has no AMS sources configured, sends the user to /onboarding
 * so they can add one before interacting with an empty dashboard.
 * Never redirects if the user is already on /onboarding.
 * A failed or errored sources fetch fails open — a network blip must not
 * displace a working user into the setup wizard.
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
    checked.current = true;

    let cancelled = false;
    adminApi
      .getSources()
      .then((res) => {
        if (!cancelled && res.items.length === 0) {
          navigate("/onboarding", { replace: true });
        }
      })
      .catch(() => {
        // Fail open — network error must not block a working user.
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
            element={<OnboardingWizard onComplete={() => navigate("/")} />}
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
