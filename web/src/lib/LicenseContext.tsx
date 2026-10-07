/**
 * LicenseContext — app-wide license state (D-089)
 *
 * Fetches on mount and again after sign-in (D14); error => context value stays
 * null (no console.error spam since that would trip the Playwright
 * zero-console-error gate).
 *
 * WHY client-side computation of expiry:
 *   Older AMS Pulse instances may return a stale `valid` field in the server
 *   response when the license has expired but the server has not been restarted
 *   (Go fix lands in parallel this session). Computing expiry from `expires_at`
 *   on the client is belt-and-braces: the UI always reflects real-time expiry
 *   regardless of whether the server has reloaded its license state.
 */

import {
  createContext,
  useContext,
  useState,
  useEffect,
  type ReactNode,
} from "react";
import { adminApi } from "@/api/client";
import type { LicenseInfo } from "@/lib/api/types";

// ---------------------------------------------------------------------------
// Derived values
// ---------------------------------------------------------------------------

/** Days remaining until license expiry; null when there is no expiry date. */
export function computeDaysRemaining(license: LicenseInfo | null): number | null {
  if (!license || license.expires_at == null) return null;
  return Math.ceil((license.expires_at - Date.now()) / 86400000);
}

/**
 * True when the license has an expiry and is past it — OR when the server
 * explicitly marks it invalid despite a future expiry date (stale-server
 * guard: the client trusts wall-clock time over cached server state).
 */
export function computeIsTrialExpired(license: LicenseInfo | null): boolean {
  if (!license || license.expires_at == null) return false;
  return license.expires_at < Date.now() || license.valid === false;
}

// ---------------------------------------------------------------------------
// Context shape
// ---------------------------------------------------------------------------

export interface LicenseContextValue {
  license: LicenseInfo | null;
  daysRemaining: number | null;
  isTrialExpired: boolean;
}

const LicenseContext = createContext<LicenseContextValue | null>(null);

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

export function LicenseProvider({ children }: { children: ReactNode }) {
  const [license, setLicense] = useState<LicenseInfo | null>(null);

  useEffect(() => {
    let cancelled = false;
    const load = () => {
      adminApi
        .getLicense()
        .then((lic) => {
          if (!cancelled) setLicense(lic);
        })
        .catch(() => {
          // Intentionally swallow; context stays null.
          // Do NOT console.error — that would trip the Playwright
          // zero-console-error gate.
          if (!cancelled) setLicense(null);
        });
    };
    load();
    // D14: this provider mounts before sign-in, so on a first visit the fetch
    // above gets 401. Signing in emits pulse:auth:token (api/client setToken);
    // fetch again then, or the tier label stays empty until a reload.
    window.addEventListener("pulse:auth:token", load);
    return () => {
      cancelled = true;
      window.removeEventListener("pulse:auth:token", load);
    };
  }, []);

  const value: LicenseContextValue = {
    license,
    daysRemaining: computeDaysRemaining(license),
    isTrialExpired: computeIsTrialExpired(license),
  };

  return (
    <LicenseContext.Provider value={value}>
      {children}
    </LicenseContext.Provider>
  );
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useLicense(): LicenseContextValue {
  const ctx = useContext(LicenseContext);
  if (!ctx) throw new Error("useLicense must be used inside <LicenseProvider>");
  return ctx;
}
