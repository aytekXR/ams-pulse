import { useState } from "react";

export interface DateRange {
  from: number; // epoch ms
  to: number;   // epoch ms
  label: string;
}

interface Props {
  value: DateRange;
  onChange: (range: DateRange) => void;
}

const PRESETS: DateRange[] = [
  { label: "Last 24h", from: Date.now() - 24 * 3600_000, to: Date.now() },
  { label: "Last 7d", from: Date.now() - 7 * 86400_000, to: Date.now() },
  { label: "Last 30d", from: Date.now() - 30 * 86400_000, to: Date.now() },
];

function toInputDate(ms: number): string {
  return new Date(ms).toISOString().slice(0, 16);
}

export function DateRangePicker({ value, onChange }: Props) {
  const [showCustom, setShowCustom] = useState(false);
  const [customFrom, setCustomFrom] = useState(toInputDate(value.from));
  const [customTo, setCustomTo] = useState(toInputDate(value.to));

  const applyCustom = () => {
    const from = new Date(customFrom).getTime();
    const to = new Date(customTo).getTime();
    if (isNaN(from) || isNaN(to) || from >= to) return;
    onChange({ label: "Custom", from, to });
    setShowCustom(false);
  };

  return (
    <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)", flexWrap: "wrap" }}>
      {PRESETS.map((preset) => (
        <button
          key={preset.label}
          onClick={() => {
            const now = Date.now();
            const diff = preset.to - preset.from;
            onChange({ label: preset.label, from: now - diff, to: now });
            setShowCustom(false);
          }}
          aria-pressed={value.label === preset.label}
          className="picker-btn"
          style={{
            background: value.label === preset.label ? "var(--color-accent)" : "var(--color-surface-2)",
            border: "1px solid var(--color-border)",
            color: value.label === preset.label ? "var(--color-on-signal)" : "var(--color-secondary)",
            borderRadius: 6,
            padding: "6px 12px",
            cursor: "pointer",
            fontSize: 12,
            fontWeight: 500,
          }}
        >
          {preset.label}
        </button>
      ))}
      <button
        onClick={() => setShowCustom((v) => !v)}
        aria-pressed={value.label === "Custom"}
        aria-expanded={showCustom}
        className="picker-btn"
        style={{
          background: value.label === "Custom" ? "var(--color-accent)" : "var(--color-surface-2)",
          border: "1px solid var(--color-border)",
          color: value.label === "Custom" ? "var(--color-on-signal)" : "var(--color-secondary)",
          borderRadius: 6,
          padding: "6px 12px",
          cursor: "pointer",
          fontSize: 12,
          fontWeight: 500,
        }}
      >
        Custom
      </button>

      {showCustom && (
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: "var(--space-2)",
            background: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            borderRadius: 8,
            padding: "var(--space-2) var(--space-3)",
          }}
        >
          {/* aria-label, not a visible <label>: the row is a compact inline
              control strip with no room for field labels, and the placeholder
              of a datetime-local input is the format hint, not a name. */}
          <input
            type="datetime-local"
            aria-label="Custom range start"
            value={customFrom}
            onChange={(e) => setCustomFrom(e.target.value)}
            className="filter-input"
            style={{
              background: "var(--color-surface-2)",
              border: "1px solid var(--color-border)",
              borderRadius: 4,
              padding: "var(--space-1) var(--space-2)",
              color: "var(--color-text)",
              fontSize: 12,
            }}
          />
          <span style={{ color: "var(--color-secondary)", fontSize: 12 }}>to</span>
          <input
            type="datetime-local"
            aria-label="Custom range end"
            value={customTo}
            onChange={(e) => setCustomTo(e.target.value)}
            className="filter-input"
            style={{
              background: "var(--color-surface-2)",
              border: "1px solid var(--color-border)",
              borderRadius: 4,
              padding: "var(--space-1) var(--space-2)",
              color: "var(--color-text)",
              fontSize: 12,
            }}
          />
          <button
            onClick={applyCustom}
            className="picker-btn"
            style={{
              background: "var(--color-accent)",
              border: "none",
              color: "var(--color-on-signal)",
              borderRadius: 4,
              padding: "4px 10px",
              cursor: "pointer",
              fontSize: 12,
            }}
          >
            Apply
          </button>
        </div>
      )}
    </div>
  );
}

export function defaultDateRange(): DateRange {
  return { label: "Last 24h", from: Date.now() - 24 * 3600_000, to: Date.now() };
}
