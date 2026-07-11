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

import { BrowserRouter, Routes, Route, useNavigate } from "react-router-dom";
import { useState } from "react";
import { AuthGate } from "@/components/AuthGate";
import { Layout } from "@/components/Layout";
import { ToastProvider } from "@/components/Toast";
import { ThemeProvider, DensityProvider } from "@/lib/ThemeContext";
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

function AppRoutes() {
  const [wsConnected, setWsConnected] = useState(false);
  const navigate = useNavigate();

  return (
    <AuthGate>
      <Layout wsConnected={wsConnected}>
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
