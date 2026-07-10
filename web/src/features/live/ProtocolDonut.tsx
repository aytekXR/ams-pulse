import { PieChart, Pie, Cell, Tooltip, Legend, ResponsiveContainer } from "recharts";
import type { ProtocolMix } from "@/lib/api/types";
import { PROTOCOL_COLORS } from "@/lib/chartColors";

interface Props {
  data: ProtocolMix;
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
      <PieChart>
        <Pie
          data={entries}
          cx="50%"
          cy="50%"
          innerRadius={50}
          outerRadius={72}
          paddingAngle={2}
          dataKey="value"
          isAnimationActive={false}
        >
          {entries.map((entry) => (
            <Cell
              key={entry.name}
              fill={PROTOCOL_COLORS[entry.name.toLowerCase()] ?? "#7C93AD"}
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
        <Legend
          iconSize={10}
          wrapperStyle={{ fontSize: 12, color: "var(--color-muted)" }}
        />
      </PieChart>
    </ResponsiveContainer>
  );
}
