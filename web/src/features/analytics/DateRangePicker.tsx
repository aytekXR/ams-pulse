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
    <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
      {PRESETS.map((preset) => (
        <button
          key={preset.label}
          onClick={() => {
            const now = Date.now();
            const diff = preset.to - preset.from;
            onChange({ label: preset.label, from: now - diff, to: now });
            setShowCustom(false);
          }}
          style={{
            background: value.label === preset.label ? "var(--color-accent)" : "var(--color-surface-2)",
            border: "1px solid var(--color-border)",
            color: value.label === preset.label ? "#fff" : "var(--color-muted)",
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
        style={{
          background: value.label === "Custom" ? "var(--color-accent)" : "var(--color-surface-2)",
          border: "1px solid var(--color-border)",
          color: value.label === "Custom" ? "#fff" : "var(--color-muted)",
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
            gap: 8,
            background: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            borderRadius: 8,
            padding: "8px 12px",
          }}
        >
          <input
            type="datetime-local"
            value={customFrom}
            onChange={(e) => setCustomFrom(e.target.value)}
            style={{
              background: "var(--color-surface-2)",
              border: "1px solid var(--color-border)",
              borderRadius: 4,
              padding: "4px 8px",
              color: "var(--color-text)",
              fontSize: 12,
            }}
          />
          <span style={{ color: "var(--color-muted)", fontSize: 12 }}>to</span>
          <input
            type="datetime-local"
            value={customTo}
            onChange={(e) => setCustomTo(e.target.value)}
            style={{
              background: "var(--color-surface-2)",
              border: "1px solid var(--color-border)",
              borderRadius: 4,
              padding: "4px 8px",
              color: "var(--color-text)",
              fontSize: 12,
            }}
          />
          <button
            onClick={applyCustom}
            style={{
              background: "var(--color-accent)",
              border: "none",
              color: "#fff",
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
