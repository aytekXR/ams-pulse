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

import { useState, useEffect } from "react";
import { useLicense } from "@/lib/LicenseContext";

const DISMISSED_KEY = "trial-banner-dismissed";

export function TrialBanner() {
  const { daysRemaining, isTrialExpired } = useLicense();
  const [dismissed, setDismissed] = useState(
    () => sessionStorage.getItem(DISMISSED_KEY) === "1",
  );
  // s111 M7: two-phase dismiss. Phase 1 sets `leaving` — the strip loses its
  // alert role immediately (out of the a11y tree at once) and its height
  // transitions 36→0 on var(--motion-base) so the page below settles instead
  // of jumping 36px in one frame. Phase 2 unmounts on transitionend, with a
  // timeout fallback (below) for environments where it never fires.
  // Reduced motion: --motion-base collapses to 0ms → instant, today's behavior.
  const [leaving, setLeaving] = useState(false);

  const handleDismiss = () => {
    sessionStorage.setItem(DISMISSED_KEY, "1");
    setLeaving(true);
  };

  useEffect(() => {
    if (!leaving) return;
    // Fallback slightly over motion.base (200ms) in case transitionend is lost.
    const t = window.setTimeout(() => setDismissed(true), 250);
    return () => window.clearTimeout(t);
  }, [leaving]);

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
        role={leaving ? undefined : "alert"}
        aria-hidden={leaving || undefined}
        onTransitionEnd={(e) => {
          if (e.propertyName === "height") setDismissed(true);
        }}
        style={{
          height: leaving ? 0 : 36,
          opacity: leaving ? 0 : 1,
          overflow: "hidden",
          transition: "height var(--motion-base), opacity var(--motion-fast)",
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
        {/* s111 D20/D14: drawn 2px-stroke SVG replaces the '×' font glyph
            (one icon system, consistent weight); minWidth rides the
            minTouchTarget token — height is capped by the 36px strip, so the
            hit area grows sideways, which is where the room is. .icon-btn
            supplies the focus ring + press scale; the warning colour stays
            inline (inline wins over the class base, intentionally). */}
        <button
          onClick={handleDismiss}
          aria-label="Dismiss license expiry notice"
          className="icon-btn"
          style={{
            background: "none",
            border: "none",
            cursor: "pointer",
            color: "var(--color-warning)",
            padding: "0 4px",
            minWidth: "var(--min-touch)",
            alignSelf: "stretch",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
          }}
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
            <path d="M18 6 6 18M6 6l12 12" />
          </svg>
        </button>
      </div>
    );
  }

  return null;
}
