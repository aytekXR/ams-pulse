/**
 * TrialBanner — global license-expiry notification strip (D-089)
 *
 * Placement: between </header> and <main> in Layout.tsx.
 * Height: 36px fixed, flexShrink 0 so it does not compress the page body.
 *
 * Visibility rules:
 *   - daysRemaining null (no expiry)    => null
 *   - daysRemaining > 14                => null
 *   - 0 < daysRemaining <= 14           => warning strip (dismissable)
 *   - isTrialExpired                    => error strip (NON-dismissable)
 *
 * Dismiss: sessionStorage key 'trial-banner-dismissed' suppresses the
 * warning-only variant; the expired variant is always shown.
 *
 * All colors come from CSS custom properties in global.css (brandkit tokens).
 */

import { useState } from "react";
import { useLicense } from "@/lib/LicenseContext";

const DISMISSED_KEY = "trial-banner-dismissed";

export function TrialBanner() {
  const { daysRemaining, isTrialExpired } = useLicense();
  const [dismissed, setDismissed] = useState(
    () => sessionStorage.getItem(DISMISSED_KEY) === "1",
  );

  const handleDismiss = () => {
    sessionStorage.setItem(DISMISSED_KEY, "1");
    setDismissed(true);
  };

  // Expired banner — always show, non-dismissable
  if (isTrialExpired) {
    return (
      <div
        role="alert"
        style={{
          height: 36,
          flexShrink: 0,
          display: "flex",
          alignItems: "center",
          paddingInline: 20,
          gap: 8,
          background: "var(--color-error-bg)",
          borderLeft: "3px solid var(--color-error)",
          color: "var(--color-error)",
          fontSize: 13,
          fontWeight: 500,
        }}
      >
        License expired — Pulse is running on Free tier limits. Activate a
        license in Settings › License.
      </div>
    );
  }

  // Warning banner — only when 0 < daysRemaining <= 14 and not dismissed
  if (daysRemaining !== null && daysRemaining > 0 && daysRemaining <= 14) {
    if (dismissed) return null;

    const dayWord = daysRemaining === 1 ? "day" : "days";

    return (
      <div
        role="alert"
        style={{
          height: 36,
          flexShrink: 0,
          display: "flex",
          alignItems: "center",
          paddingInline: 20,
          gap: 8,
          background: "var(--color-warning-bg)",
          borderLeft: "3px solid var(--color-warning)",
          color: "var(--color-warning)",
          fontSize: 13,
          fontWeight: 500,
        }}
      >
        <span style={{ flex: 1 }}>
          License expires in {daysRemaining} {dayWord} — activate a key in
          Settings › License.
        </span>
        <button
          onClick={handleDismiss}
          aria-label="Dismiss license expiry notice"
          style={{
            background: "none",
            border: "none",
            cursor: "pointer",
            color: "var(--color-warning)",
            fontSize: 16,
            lineHeight: 1,
            padding: "0 4px",
            display: "flex",
            alignItems: "center",
          }}
        >
          ×
        </button>
      </div>
    );
  }

  return null;
}
