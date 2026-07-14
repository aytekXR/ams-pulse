import { PieChart, Pie, Cell, Tooltip, Legend, ResponsiveContainer } from "recharts";
import type { ProtocolMix } from "@/lib/api/types";
import { PROTOCOL_COLORS, CHART_COLORS } from "@/lib/chartColors";

interface Props {
  data: ProtocolMix;
}

/** Minimal props needed to position and render a Pie label (subset of Recharts PieLabelRenderProps). */
type PieLabelRenderProps = {
  cx?: number;
  cy?: number;
  midAngle?: number;
  outerRadius?: number;
  name?: string;
  percent?: number;
  /** innerRadius is provided by Recharts but not used — radius is outerRadius+15. */
  innerRadius?: number;
};

/**
 * Custom label for each Pie slice — renders name + percentage outside the donut.
 * Slices under 5% are skipped to avoid clutter.
 * Uses SVG <text> so fill/fontFamily work in all browsers without CSS-var issues.
 * Satisfies WCAG 1.4.1: state is encoded by label text, not colour alone (P-1).
 *
 * Exported for direct unit testing (pure function, no component state).
 */
export function renderPieLabel({
  cx = 0,
  cy = 0,
  midAngle = 0,
  outerRadius = 0,
  name = "",
  percent = 0,
}: PieLabelRenderProps) {
  if (percent < 0.05) return null;
  const RADIAN = Math.PI / 180;
  const radius = outerRadius + 15;
  const x = cx + radius * Math.cos(-midAngle * RADIAN);
  const y = cy + radius * Math.sin(-midAngle * RADIAN);
  return (
    <text
      x={x}
      y={y}
      fill="var(--color-text)"
      fontSize={11}
      fontFamily="var(--font-sans)"
      textAnchor={x > cx ? "start" : "end"}
      dominantBaseline="central"
    >
      {name} {Math.round(percent * 100)}%
    </text>
  );
}

export function ProtocolDonut({ data }: Props) {
  const entries = Object.entries(data ?? {})
    .filter(([, v]) => typeof v === "number" && v > 0)
    .map(([name, value]) => ({ name: name.toUpperCase(), value: value as number }));

  if (entries.length === 0) {
    return (
      <div style={{ display: "flex", alignItems: "center", justifyContent: "center", height: 160, color: "var(--color-muted)", fontSize: 13 }}>
        No viewers
      </div>
    );
  }

  return (
    <ResponsiveContainer width="100%" height={180}>
      {/* accessibilityLayer is already true by default in Recharts v3 PolarChart;
          stated explicitly here so the intent is visible (P-2). */}
      <PieChart accessibilityLayer>
        <Pie
          data={entries}
          cx="50%"
          cy="50%"
          innerRadius={50}
          outerRadius={72}
          paddingAngle={2}
          dataKey="value"
          isAnimationActive={false}
          label={renderPieLabel}
          labelLine={false}
        >
          {entries.map((entry) => (
            <Cell
              key={entry.name}
              fill={PROTOCOL_COLORS[entry.name.toLowerCase()] ?? CHART_COLORS[7]}
            />
          ))}
        </Pie>
        <Tooltip
          contentStyle={{
            background: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            borderRadius: 6,
            color: "var(--color-text)",
          }}
        />
        {/* iconType="circle" pairs a consistent shape with colour so adjacent
            series remain distinguishable without relying on hue alone (P-5). */}
        <Legend
          iconSize={10}
          iconType="circle"
          wrapperStyle={{ fontSize: 12, color: "var(--color-muted)" }}
        />
      </PieChart>
    </ResponsiveContainer>
  );
}
