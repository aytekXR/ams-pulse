/**
 * SegmentedControl — shared fill-background view/mode switch.
 *
 * Extracted from FleetPage's cards/table toggle (Wave 2). This is deliberately
 * NOT a <Tabs>: the S31 scout ruled the widget a segmented control on visual
 * grounds (fill-background active state, 11px text, no underline), and Wave 0
 * recorded that converting it to <Tabs> would change design intent.
 *
 * ARIA pattern: role="radiogroup" + role="radio" + aria-checked.
 *
 *   Why NOT role="tablist": a tablist promises tabpanels. The cards/table
 *   toggle does not reveal separate content regions — it re-renders ONE region
 *   in a different shape, and no element carries role="tabpanel". Announcing
 *   tabs to a screen reader with no tabpanel to move to is a false semantic
 *   contract, the same class of defect as the aria-sort="none" on unsortable
 *   columns that S32 caught pre-merge. A radiogroup is what this actually is:
 *   a choice of one display mode from a mutually exclusive set.
 *
 * Keyboard (WAI-ARIA APG radio group): Arrow Right/Down → next, Arrow Left/Up →
 * previous (both wrap), Home → first, End → last. Selection follows focus
 * (auto-activation), which is correct here — switching view is instant and
 * lossless. Roving tabIndex: the checked radio is the group's single tab stop.
 *
 * Style values transferred VERBATIM from FleetPage's inline toggle — this
 * extraction may not move a pixel. Residual bare literals kept as-is:
 *   container borderRadius: 4 | button padding: "4px 10px" | fontSize: 11
 * (4px has a --space-1 token but 10px has none; splitting the shorthand would
 * gain nothing, so it stays verbatim — same call the Tabs extraction made.)
 *
 * ONE DELIBERATE DEVIATION, mandated by the BINDING WAVE-PLAN §2.2 contrast
 * gate (an extraction may not ship a component that fails AA):
 *   inactive label colour var(--color-muted) → var(--color-secondary).
 *   --color-muted (#5C6F80 dark / #6B7B88 light) fails AA for 11px normal text;
 *   --color-secondary (#9FB0C0 dark / #4A5B6B light) passes in both themes.
 * Identical to the fix Tabs.tsx carries (WAVE-PLAN §3 C7b/c).
 *
 * Labels are pre-capitalised by callers ("Cards", not "cards"), so the component
 * applies no textTransform: "capitalize" — the source toggle capitalised a
 * lowercase value in CSS, which renders the same text while letting the DOM and
 * the accessible name drift from each other. Passing the display label directly
 * keeps them identical. Same call Tabs.tsx made.
 *
 * Focus ring: className="seg-btn" enables the :focus-visible ring in global.css.
 */
import { useRef } from "react";
import type { KeyboardEvent } from "react";

export interface SegmentItem {
  /** Machine value reported to onChange. */
  value: string;
  /** Pre-capitalised display label (also the accessible name). */
  label: string;
}

export interface SegmentedControlProps {
  items: SegmentItem[];
  value: string;
  onChange: (value: string) => void;
  /** Required: a radiogroup with no accessible name is unusable to a screen reader. */
  "aria-label": string;
}

export function SegmentedControl({
  items,
  value,
  onChange,
  "aria-label": ariaLabel,
}: SegmentedControlProps) {
  const buttonRefs = useRef<(HTMLButtonElement | null)[]>([]);

  function handleKeyDown(e: KeyboardEvent<HTMLButtonElement>, index: number) {
    let nextIndex: number;
    switch (e.key) {
      case "ArrowRight":
      case "ArrowDown":
        nextIndex = (index + 1) % items.length;
        break;
      case "ArrowLeft":
      case "ArrowUp":
        nextIndex = (index - 1 + items.length) % items.length;
        break;
      case "Home":
        nextIndex = 0;
        break;
      case "End":
        nextIndex = items.length - 1;
        break;
      default:
        return;
    }
    e.preventDefault();
    onChange(items[nextIndex].value);
    buttonRefs.current[nextIndex]?.focus();
  }

  return (
    <div
      role="radiogroup"
      aria-label={ariaLabel}
      style={{
        display: "flex",
        border: "1px solid var(--color-border)",
        borderRadius: 4,
        overflow: "hidden",
      }}
    >
      {items.map((item, index) => {
        const isChecked = value === item.value;
        return (
          <button
            key={item.value}
            ref={(el) => {
              buttonRefs.current[index] = el;
            }}
            role="radio"
            aria-checked={isChecked}
            tabIndex={isChecked ? 0 : -1}
            onClick={() => onChange(item.value)}
            onKeyDown={(e) => handleKeyDown(e, index)}
            className="seg-btn"
            style={{
              background: isChecked ? "var(--color-surface-2)" : "transparent",
              border: "none",
              color: isChecked ? "var(--color-text)" : "var(--color-secondary)",
              padding: "4px 10px",
              cursor: "pointer",
              fontSize: 11,
              fontWeight: isChecked ? 600 : 400,
            }}
          >
            {item.label}
          </button>
        );
      })}
    </div>
  );
}
