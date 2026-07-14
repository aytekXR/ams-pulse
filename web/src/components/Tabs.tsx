/**
 * Tabs — shared underline-tab navigation component.
 *
 * Extracted from the verbatim-identical tab pattern in AnalyticsPage,
 * AlertsPage, and ReportsPage. SettingsPage uses a divergent pattern
 * (flexWrap: "wrap", whiteSpace: "nowrap", label-dictionary) and is deferred
 * to Wave 4. FleetPage's cards/table toggle is a segmented control — a
 * different widget; it must never be converted to <Tabs>.
 *
 * Residual bare literals (moved verbatim from the originals, not introduced):
 *   container gap: 0
 *   button padding: "8px 16px" | fontSize: 13
 *   active-indicator: "2px solid"
 *
 * Touch-target (44pt, tokens.json layout.minTouchTarget) and inter-button gap
 * (8px, --space-2) improvements are deferred to the page waves (Wave 2–4) to
 * maintain pixel-exact extraction in Wave 0.
 *
 * ARIA pattern implemented (non-pixel — adds HTML semantics only):
 *   role="tablist" on container
 *   role="tab" + aria-selected on each button
 *   Roving tabIndex: active tab = 0, all others = -1
 *   id="tab-{id}" on each button (for future aria-labelledby panel wiring)
 *   ArrowLeft / ArrowRight / Home / End keyboard navigation with automatic
 *   activation (focus moves and tab activates together — appropriate because
 *   all three source pages already load data on tab change)
 *   aria-controls deferred — panel elements in call-site pages do not yet
 *   carry matching role="tabpanel" + id attributes; adding aria-controls now
 *   would produce invalid ARIA references. Panel-side wiring deferred to each
 *   page wave.
 *
 * Style values: transferred verbatim from the source pages with one WCAG fix.
 * This is a DELIBERATE DEVIATION from pixel-equivalent extraction, mandated by
 * the WAVE-PLAN §2.2 accessibility gate (BINDING: an extraction may not ship a
 * component that fails contrast). Not operator-ruled — see WAVE-PLAN.md §3 C7:
 *   Inactive tab color: var(--color-muted) → var(--color-secondary).
 *   Reason: --color-muted (#5C6F80 dark / #6B7B88 light) on --color-surface
 *   gives 3.50:1 dark / 4.36:1 light — both fail AA for 13px normal text.
 *   --color-secondary (#9FB0C0 dark / #4A5B6B light) gives 8.18:1 dark /
 *   7.00:1 light — both PASS.
 *
 * Labels are pre-capitalised by callers (e.g., "Audience", not "audience"), so
 * the component does not apply textTransform: "capitalize" — this produces the
 * same visible text as the source pattern while avoiding CSS-vs-content drift.
 *
 * Focus ring: className="tabs-btn" enables the :focus-visible ring defined in
 * global.css (2px solid var(--color-link), offset 2px).
 */
import { useRef } from "react";
import type { KeyboardEvent } from "react";

export interface TabItem {
  id: string;
  label: string;
}

export interface TabsProps {
  tabs: TabItem[];
  activeTab: string;
  onTabChange: (id: string) => void;
}

export function Tabs({ tabs, activeTab, onTabChange }: TabsProps) {
  const buttonRefs = useRef<(HTMLButtonElement | null)[]>([]);

  function handleKeyDown(e: KeyboardEvent<HTMLButtonElement>, index: number) {
    let nextIndex: number;
    switch (e.key) {
      case "ArrowRight":
        nextIndex = (index + 1) % tabs.length;
        break;
      case "ArrowLeft":
        nextIndex = (index - 1 + tabs.length) % tabs.length;
        break;
      case "Home":
        nextIndex = 0;
        break;
      case "End":
        nextIndex = tabs.length - 1;
        break;
      default:
        return;
    }
    e.preventDefault();
    const nextTab = tabs[nextIndex];
    onTabChange(nextTab.id);
    buttonRefs.current[nextIndex]?.focus();
  }

  return (
    <div
      role="tablist"
      style={{
        display: "flex",
        gap: 0,
        borderBottom: "1px solid var(--color-border)",
      }}
    >
      {tabs.map((item, index) => {
        const isActive = activeTab === item.id;
        return (
          <button
            key={item.id}
            id={`tab-${item.id}`}
            ref={(el) => {
              buttonRefs.current[index] = el;
            }}
            role="tab"
            aria-selected={isActive}
            tabIndex={isActive ? 0 : -1}
            onClick={() => onTabChange(item.id)}
            onKeyDown={(e) => handleKeyDown(e, index)}
            className="tabs-btn"
            style={{
              background: "none",
              border: "none",
              borderBottom: `2px solid ${isActive ? "var(--color-accent)" : "transparent"}`,
              color: isActive ? "var(--color-text)" : "var(--color-secondary)",
              padding: "8px 16px",
              cursor: "pointer",
              fontSize: 13,
              fontWeight: isActive ? 600 : 400,
            }}
          >
            {item.label}
          </button>
        );
      })}
    </div>
  );
}
