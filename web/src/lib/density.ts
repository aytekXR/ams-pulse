/**
 * Density utilities — default / compact / wall display modes.
 *
 * Reads localStorage; defaults to "default".
 * Stamps data-density on document.documentElement.
 * Called synchronously in main.tsx BEFORE ReactDOM.render.
 *
 * ROW_HEIGHT_MAP is consumed by the virtualizer (sweep agent — B2).
 * Values are derived on-grid from the space token array because brandkit
 * names the density modes but gives no explicit row-height numbers for them.
 * ORCH ruling: default=40 (authoritative tokens.json tableRowHeight),
 * compact=32, wall=48.
 */

export type Density = "default" | "compact" | "wall";

export const DENSITY_KEY = "pulse_density";

/** Row heights per density level (px), exported for the virtualizer. */
export const ROW_HEIGHT_MAP: Record<Density, number> = {
  default: 40,
  compact: 32,
  wall: 48,
};

/** Resolve density from localStorage, defaulting to "default". */
function resolveDensity(): Density {
  const stored = localStorage.getItem(DENSITY_KEY);
  if (stored === "compact" || stored === "wall" || stored === "default") return stored;
  return "default";
}

/**
 * Read localStorage and stamp data-density on <html>.
 * For "default" we stamp the attribute rather than removing it so CSS
 * selectors are explicit and predictable.
 */
export function initDensity(): void {
  const density = resolveDensity();
  document.documentElement.setAttribute("data-density", density);
}

/** Return the current density from the DOM attribute (falls back to "default"). */
export function getDensity(): Density {
  const attr = document.documentElement.getAttribute("data-density");
  if (attr === "compact" || attr === "wall") return attr;
  return "default";
}

/** Set the density: stamp DOM attribute + persist to localStorage. */
export function setDensity(density: Density): void {
  document.documentElement.setAttribute("data-density", density);
  localStorage.setItem(DENSITY_KEY, density);
}
