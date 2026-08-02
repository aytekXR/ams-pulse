import { PieChart, Pie, Cell, Tooltip, Legend, ResponsiveContainer } from "recharts";
import type { ProtocolMix } from "@/lib/api/types";
import { PROTOCOL_COLORS, CHART_COLORS, CHART_LEGEND_STYLE, CHART_TOOLTIP_STYLE } from "@/lib/chartColors";

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
  // Small offset: this donut lives in a narrow dashboard column, and a larger
  // gap pushed the left/right labels past the card edge (they were clipped
  // mid-word in the marketplace screenshot).
  const radius = outerRadius + 10;
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
      <div style={{ display: "flex", alignItems: "center", justifyContent: "center", height: 160, color: "var(--color-secondary)", fontSize: 12 }}>
        No viewers
      </div>
    );
  }

  return (
    // height 180 gave the donut (144 px across) plus its outside labels (which sit
    // at outerRadius + 15) essentially the whole box, so the lower labels landed on
    // top of the Legend and rendered as overlapping text — visible in the primary
    // marketplace screenshot. 220 gives the legend its own band. The radii shrink
    // slightly so the labels also stay inside this card's narrow column; the labels
    // must keep the protocol NAME (not just the percentage) because encoding the
    // series by colour alone would break WCAG 1.4.1 (P-1).
    <ResponsiveContainer width="100%" height={220}>
      {/* accessibilityLayer is already true by default in Recharts v3 PolarChart;
          stated explicitly here so the intent is visible (P-2). */}
      <PieChart accessibilityLayer>
        <Pie
          data={entries}
          cx="50%"
          cy="45%"
          innerRadius={38}
          outerRadius={54}
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
        {/* s111 M4: isAnimationActive={false} — the 400ms position-lag trails the
            cursor while an operator reads precise values; instant is correct for
            functional data (same rule as the QO-1 Line fix). */}
        <Tooltip isAnimationActive={false} contentStyle={CHART_TOOLTIP_STYLE} />
        {/* iconType="circle" pairs a consistent shape with colour so adjacent
            series remain distinguishable without relying on hue alone (P-5). */}
        <Legend
          iconSize={10}
          iconType="circle"
          wrapperStyle={CHART_LEGEND_STYLE}
        />
      </PieChart>
    </ResponsiveContainer>
  );
}
