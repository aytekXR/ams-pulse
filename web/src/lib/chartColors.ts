/**
 * Brandkit dataviz palette — tokens.json color.dataviz
 *
 * Use literal hex strings (not CSS vars) in Recharts stroke/fill props.
 * CSS custom properties resolve in SVG presentation attributes in browsers,
 * but Recharts may stringify them before the browser resolves them.
 */
import { useTheme } from "./ThemeContext";

/** 8-color sequential dataviz palette from tokens.json */
export const CHART_COLORS = [
  '#2CE5A7', // [0] signal/healthy — first series
  '#58A6FF', // [1] primary series
  '#A78BFA', // [2] secondary series
  '#F06BB2', // [3] tertiary series
  '#FFB224', // [4] warning/fifth series
  '#38D6E0', // [5] sixth series
  '#C4B26A', // [6] seventh series
  '#7C93AD', // [7] eighth/muted series
] as const;

/** Status semantic colors from tokens.json color.dark (static dark constant). */
export const STATUS_COLORS = {
  healthy:  '#2CE5A7',
  warning:  '#FFB224',
  critical: '#FF5C68',
  neutral:  '#8296A8',
} as const;

/** Status semantic colors from tokens.json color.light */
export const LIGHT_STATUS_COLORS = {
  healthy:  '#0BA678',
  warning:  '#B45309',
  critical: '#DC2626',
  neutral:  '#64748B',
} as const;

/** Structural shape shared by STATUS_COLORS (dark) and LIGHT_STATUS_COLORS (light). */
export type StatusColorMap = {
  readonly healthy: string;
  readonly warning: string;
  readonly critical: string;
  readonly neutral: string;
};

/**
 * Hook: returns STATUS_COLORS (dark) or LIGHT_STATUS_COLORS (light) based on
 * the current theme from ThemeContext. Use this wherever JS color values are
 * needed in SVG/Recharts props (CSS vars cannot be used in those attributes).
 *
 * Must be called inside a <ThemeProvider>.
 */
export function useStatusColors(): StatusColorMap {
  const { theme } = useTheme();
  return theme === "light" ? LIGHT_STATUS_COLORS : STATUS_COLORS;
}

/** Protocol-to-color map for ProtocolDonut — hue per protocol, not semantic status */
export const PROTOCOL_COLORS: Record<string, string> = {
  hls:    '#2CE5A7', // dataviz[0]
  webrtc: '#58A6FF', // dataviz[1]
  rtmp:   '#A78BFA', // dataviz[2] — purple, not red (avoids false alarm look)
  dash:   '#F06BB2', // dataviz[3]
  other:  '#7C93AD', // dataviz[7] muted
};
