/**
 * Theme utilities — light/dark mode.
 *
 * Reads localStorage first; falls back to (prefers-color-scheme: light); defaults dark.
 * Stamps data-theme on document.documentElement.
 * Called synchronously in main.tsx BEFORE ReactDOM.render so the correct theme
 * token set is active before the first paint (no FOUC).
 */

export type Theme = "dark" | "light";

export const THEME_KEY = "pulse_theme";

/** Determine the preferred theme: localStorage > matchMedia > "dark" */
function resolveTheme(): Theme {
  const stored = localStorage.getItem(THEME_KEY);
  if (stored === "light" || stored === "dark") return stored;
  if (typeof window !== "undefined" && window.matchMedia) {
    if (window.matchMedia("(prefers-color-scheme: light)").matches) return "light";
  }
  return "dark";
}

/**
 * Read localStorage / matchMedia and stamp data-theme on <html>.
 * Called once at startup before React hydrates.
 */
export function initTheme(): void {
  const theme = resolveTheme();
  document.documentElement.setAttribute("data-theme", theme);
}

/** Return the current theme from the DOM attribute (falls back to "dark"). */
export function getTheme(): Theme {
  const attr = document.documentElement.getAttribute("data-theme");
  return attr === "light" ? "light" : "dark";
}

/** Set the theme: stamp DOM attribute + persist to localStorage. */
export function setTheme(theme: Theme): void {
  document.documentElement.setAttribute("data-theme", theme);
  localStorage.setItem(THEME_KEY, theme);
}
