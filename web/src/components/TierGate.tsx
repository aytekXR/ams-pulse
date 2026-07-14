/**
 * TierGate — tier-entitlement upsell gate.
 *
 * Rendered when the user's licence tier does not permit access to a feature.
 * Extracted from ReportsPage, AnomaliesPage, and ProbesPage; the props
 * interface is the union of all three call sites.
 *
 * Residual bare literals (moved verbatim from the originals, not introduced):
 *   borderRadius: 8 | padding: "3rem 2rem" | gap: 16
 *   h2 fontSize: 18 | h2 fontWeight: 700 | h2 margin: "0 0 8px"
 *   p  fontSize: 14 | p  margin: 0
 *   a  borderRadius: 6 | a padding: "10px 20px" | a fontSize: 13 | a fontWeight: 600
 *
 * CONFLICT NOTED (deferred — Wave 0 is pixel-equivalent extraction):
 *   a borderRadius: 6 should be var(--radius-control) = 8px per tokens.json
 *   radius.control = 8 and WAVE-PLAN §3 conflict C5 ("8 for controls"). Kept
 *   at 6 to preserve the pixel-exact original render; fix deferred to the page
 *   wave that touches this component (Wave 5 for Reports/Probes).
 *
 * WCAG fix (Wave 0 — a DELIBERATE DEVIATION from pixel-equivalent extraction,
 * mandated by the WAVE-PLAN §2.2 accessibility gate, which is BINDING: an
 * extraction may not ship a component that fails contrast. Not operator-ruled;
 * see WAVE-PLAN.md §3 conflict C7):
 *   Default descriptionColor changed from var(--color-muted) to
 *   var(--color-secondary). Reason: --color-muted (#5C6F80 dark / #6B7B88 light)
 *   on --color-surface gives 3.50:1 dark / 4.36:1 light — both below the 4.5:1
 *   AA normal-text threshold for 14px body copy. --color-secondary (#9FB0C0 dark /
 *   #4A5B6B light) gives 8.18:1 dark / 7.00:1 light — both PASS.
 *   AnomaliesPage and ProbesPage relied on the default and get the corrected value
 *   automatically. ReportsPage was already passing var(--color-secondary) explicitly
 *   and is unaffected.
 *
 * WCAG known gap (not fixable in Wave 0 — requires brandkit token change):
 *   Light-theme CTA: var(--color-on-signal) (#FFFFFF) on var(--color-accent)
 *   (#0BA678) = 3.12:1 at 13px — below 4.5:1 AA for normal text. Pre-existing
 *   from baseline 2f53414 (all three pages had it verbatim); Wave 0 neither
 *   introduced nor fixed it. NO waiver has been granted: the fix needs
 *   tokens.json color.light.accent → #087A59 (5.33:1), and brandkit/ is the
 *   operator's to change (D-071). Filed as operator gap G3
 *   (docs/operator-expected.md) and WAVE-PLAN.md §3 conflict C7 — OPEN.
 *
 * Default descriptionMaxWidth: 400 (ReportsPage + AnomaliesPage)
 * Per-call-site override: 420 (ProbesPage)
 *
 * A11y additions (non-pixel):
 *   - Icon slot wrapped in <span aria-hidden="true"> so decorative SVGs are
 *     always suppressed for AT regardless of whether the caller remembered to
 *     set aria-hidden on the SVG itself.
 *   - className="tier-gate-cta" on <a> enables the :focus-visible ring defined
 *     in global.css (2px solid var(--color-link), offset 2px).
 */
import type { ReactNode } from "react";

interface Props {
  /** SVG icon rendered above the heading — wrapped in aria-hidden span */
  icon: ReactNode;
  /** Full h2 text, e.g. "Usage Reports requires Business tier" */
  heading: string;
  /** Current licence tier label, rendered in bold inside the description */
  tier: string;
  /** Upgrade sentence appended after "You are currently on the <tier> plan." */
  upgradeText: string;
  /**
   * CSS variable string for description text colour.
   * Default: "var(--color-secondary)" — passes WCAG AA in both themes
   * (8.18:1 dark, 7.00:1 light). Previously "var(--color-muted)" which
   * failed AA for 14px normal text (3.50:1 dark, 4.36:1 light).
   * ReportsPage already passes "var(--color-secondary)" explicitly.
   */
  descriptionColor?: string;
  /**
   * maxWidth of the description paragraph in px.
   * Default: 400 (ReportsPage + AnomaliesPage).
   * ProbesPage passes 420.
   */
  descriptionMaxWidth?: number;
}

export function TierGate({
  icon,
  heading,
  tier,
  upgradeText,
  descriptionColor = "var(--color-secondary)",
  descriptionMaxWidth = 400,
}: Props) {
  return (
    <div
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: 8,
        padding: "3rem 2rem",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        gap: 16,
        textAlign: "center",
      }}
    >
      {/* Decorative icon: aria-hidden enforced by wrapper so callers need not
          remember to set it on every SVG they pass. */}
      <span aria-hidden="true">{icon}</span>
      <div>
        <h2 style={{ margin: "0 0 8px", fontSize: 18, fontWeight: 700 }}>{heading}</h2>
        <p
          style={{
            margin: 0,
            fontSize: 14,
            color: descriptionColor,
            maxWidth: descriptionMaxWidth,
          }}
        >
          You are currently on the <strong>{tier}</strong> plan. {upgradeText}
        </p>
      </div>
      <a
        href="/settings#license"
        className="tier-gate-cta"
        style={{
          display: "inline-block",
          background: "var(--color-accent)",
          color: "var(--color-on-signal)",
          borderRadius: 6,
          padding: "10px 20px",
          fontSize: 13,
          fontWeight: 600,
          textDecoration: "none",
        }}
      >
        Upgrade License
      </a>
    </div>
  );
}
