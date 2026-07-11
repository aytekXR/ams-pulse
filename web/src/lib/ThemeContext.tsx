/**
 * ThemeContext + DensityContext
 *
 * ThemeProvider:
 *   - Initialises from the already-stamped data-theme attribute (set by initTheme in main.tsx)
 *   - Listens to storage events (cross-tab sync)
 *   - Listens to matchMedia change (only when no explicit localStorage choice)
 *
 * DensityProvider:
 *   - Initialises from data-density attribute (set by initDensity in main.tsx)
 *   - Exposes rowHeight via ROW_HEIGHT_MAP for the virtualizer
 *
 * Both providers can be used independently or nested; the file exports both.
 */

import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  type ReactNode,
} from "react";
import { getTheme, setTheme as applyTheme, THEME_KEY, type Theme } from "./theme";
import { getDensity, setDensity as applyDensity, ROW_HEIGHT_MAP, type Density } from "./density";

// ---------------------------------------------------------------------------
// Theme context
// ---------------------------------------------------------------------------

interface ThemeContextValue {
  theme: Theme;
  setTheme: (theme: Theme) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(() => getTheme());

  const setTheme = useCallback((next: Theme) => {
    applyTheme(next);
    setThemeState(next);
  }, []);

  useEffect(() => {
    // Cross-tab sync via storage event
    const onStorage = (e: StorageEvent) => {
      if (e.storageArea !== localStorage || e.key !== THEME_KEY) return;
      const next = e.newValue === "light" ? "light" : "dark";
      applyTheme(next);
      setThemeState(next);
    };
    window.addEventListener("storage", onStorage);

    // OS-level preference change (only when no explicit localStorage choice)
    let mq: MediaQueryList | null = null;
    const onMediaChange = (e: MediaQueryListEvent | { matches: boolean }) => {
      // Ignore if the user has made an explicit choice
      if (localStorage.getItem(THEME_KEY)) return;
      const next: Theme = e.matches ? "light" : "dark";
      applyTheme(next);
      setThemeState(next);
    };

    if (typeof window !== "undefined" && window.matchMedia) {
      mq = window.matchMedia("(prefers-color-scheme: light)");
      mq.addEventListener("change", onMediaChange);
    }

    return () => {
      window.removeEventListener("storage", onStorage);
      if (mq) mq.removeEventListener("change", onMediaChange);
    };
  }, []);

  return (
    <ThemeContext.Provider value={{ theme, setTheme }}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error("useTheme must be used inside <ThemeProvider>");
  return ctx;
}

// ---------------------------------------------------------------------------
// Density context
// ---------------------------------------------------------------------------

interface DensityContextValue {
  density: Density;
  setDensity: (density: Density) => void;
  rowHeight: number;
}

const DensityContext = createContext<DensityContextValue | null>(null);

export function DensityProvider({ children }: { children: ReactNode }) {
  const [density, setDensityState] = useState<Density>(() => getDensity());

  const setDensity = useCallback((next: Density) => {
    applyDensity(next);
    setDensityState(next);
  }, []);

  return (
    <DensityContext.Provider
      value={{ density, setDensity, rowHeight: ROW_HEIGHT_MAP[density] }}
    >
      {children}
    </DensityContext.Provider>
  );
}

export function useDensity(): DensityContextValue {
  const ctx = useContext(DensityContext);
  if (!ctx) throw new Error("useDensity must be used inside <DensityProvider>");
  return ctx;
}
