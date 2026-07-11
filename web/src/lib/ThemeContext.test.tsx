/**
 * ThemeContext + DensityContext unit tests — TDD red-first (B1 work order)
 *
 * Tests:
 * (a) useTheme: setTheme triggers re-render with new theme
 * (b) useTheme: storage event from another tab syncs theme
 * (c) useDensity: setDensity triggers re-render with new density
 * (d) useDensity: rowHeight reflects the density via ROW_HEIGHT_MAP
 * (e) useTheme: matchMedia change syncs theme when no explicit localStorage choice
 */
import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import React from "react";

// Modules not yet created — RED
import { ThemeProvider, useTheme } from "./ThemeContext";
import { DensityProvider, useDensity } from "./ThemeContext";
import { THEME_KEY } from "./theme";
import { DENSITY_KEY, ROW_HEIGHT_MAP } from "./density";

// Helper wrappers
const ThemeWrapper = ({ children }: { children: React.ReactNode }) => (
  <ThemeProvider>{children}</ThemeProvider>
);
const DensityWrapper = ({ children }: { children: React.ReactNode }) => (
  <DensityProvider>{children}</DensityProvider>
);
// matchMedia mock factory
function makeMatchMedia(lightMatches: boolean) {
  const listeners: Array<(e: { matches: boolean }) => void> = [];
  const mq = {
    matches: lightMatches,
    media: "(prefers-color-scheme: light)",
    onchange: null as null | ((e: { matches: boolean }) => void),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: (_: string, cb: (e: { matches: boolean }) => void) => {
      listeners.push(cb);
    },
    removeEventListener: (_: string, cb: (e: { matches: boolean }) => void) => {
      const idx = listeners.indexOf(cb);
      if (idx !== -1) listeners.splice(idx, 1);
    },
    dispatchEvent: vi.fn(),
    _fire: (matches: boolean) => {
      mq.matches = matches;
      listeners.forEach((cb) => cb({ matches }));
    },
  };
  return mq;
}

describe("ThemeContext", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
    vi.unstubAllGlobals();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("(a) setTheme re-renders with new theme", () => {
    const { result } = renderHook(() => useTheme(), { wrapper: ThemeWrapper });
    act(() => {
      result.current.setTheme("light");
    });
    expect(result.current.theme).toBe("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("(a) setTheme dark persists", () => {
    const { result } = renderHook(() => useTheme(), { wrapper: ThemeWrapper });
    act(() => {
      result.current.setTheme("light");
    });
    act(() => {
      result.current.setTheme("dark");
    });
    expect(result.current.theme).toBe("dark");
    expect(localStorage.getItem(THEME_KEY)).toBe("dark");
  });

  it("(b) storage event from another tab syncs theme", () => {
    const { result } = renderHook(() => useTheme(), { wrapper: ThemeWrapper });

    // Simulate another tab writing to localStorage and firing storage event
    act(() => {
      const event = new StorageEvent("storage", {
        key: THEME_KEY,
        newValue: "light",
        storageArea: localStorage,
      });
      window.dispatchEvent(event);
    });

    expect(result.current.theme).toBe("light");
  });

  it("(e) matchMedia change syncs theme when no localStorage choice", () => {
    const mq = makeMatchMedia(false);
    vi.stubGlobal("matchMedia", () => mq);

    const { result } = renderHook(() => useTheme(), { wrapper: ThemeWrapper });
    // Initially dark (matchMedia.matches = false)
    expect(result.current.theme).toBe("dark");

    // Simulate prefers-color-scheme: light
    act(() => {
      mq._fire(true);
    });
    expect(result.current.theme).toBe("light");
  });

  it("(e) matchMedia change does NOT override explicit localStorage choice", () => {
    localStorage.setItem(THEME_KEY, "dark");
    const mq = makeMatchMedia(false);
    vi.stubGlobal("matchMedia", () => mq);

    const { result } = renderHook(() => useTheme(), { wrapper: ThemeWrapper });
    expect(result.current.theme).toBe("dark");

    // Even if OS switches to light, localStorage wins
    act(() => {
      mq._fire(true);
    });
    expect(result.current.theme).toBe("dark");
  });
});

describe("DensityContext", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-density");
  });

  it("(c) setDensity re-renders with new density", () => {
    const { result } = renderHook(() => useDensity(), { wrapper: DensityWrapper });
    act(() => {
      result.current.setDensity("compact");
    });
    expect(result.current.density).toBe("compact");
    expect(document.documentElement.getAttribute("data-density")).toBe("compact");
  });

  it("(d) rowHeight reflects density via ROW_HEIGHT_MAP", () => {
    const { result } = renderHook(() => useDensity(), { wrapper: DensityWrapper });
    expect(result.current.rowHeight).toBe(ROW_HEIGHT_MAP.default);

    act(() => {
      result.current.setDensity("compact");
    });
    expect(result.current.rowHeight).toBe(ROW_HEIGHT_MAP.compact);

    act(() => {
      result.current.setDensity("wall");
    });
    expect(result.current.rowHeight).toBe(ROW_HEIGHT_MAP.wall);
  });

  it("(c) setDensity wall persists", () => {
    const { result } = renderHook(() => useDensity(), { wrapper: DensityWrapper });
    act(() => {
      result.current.setDensity("wall");
    });
    expect(localStorage.getItem(DENSITY_KEY)).toBe("wall");
  });
});
