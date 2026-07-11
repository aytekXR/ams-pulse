/**
 * theme.ts unit tests — TDD red-first (B1 work order)
 *
 * Tests:
 * (a) initTheme reads localStorage and stamps data-theme attribute
 * (b) initTheme falls back to matchMedia when no localStorage entry
 * (c) initTheme defaults to "dark" when matchMedia does not match light
 * (d) setTheme updates the attribute and persists to localStorage
 * (e) getTheme returns the current data-theme attribute value
 */
import { describe, it, expect, beforeEach, vi } from "vitest";

// Module is not yet created — these imports will cause test failures (RED)
import { initTheme, getTheme, setTheme, THEME_KEY } from "./theme";

describe("theme utilities", () => {
  beforeEach(() => {
    // Reset DOM attribute
    document.documentElement.removeAttribute("data-theme");
    // Reset localStorage
    localStorage.clear();
    // Reset matchMedia mock
    vi.unstubAllGlobals();
  });

  it("(a) initTheme stamps data-theme=dark from localStorage 'dark'", () => {
    localStorage.setItem(THEME_KEY, "dark");
    initTheme();
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("(a) initTheme stamps data-theme=light from localStorage 'light'", () => {
    localStorage.setItem(THEME_KEY, "light");
    initTheme();
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("(b) initTheme falls back to matchMedia: stamps light when prefers-color-scheme: light", () => {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query === "(prefers-color-scheme: light)",
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }));
    initTheme();
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("(c) initTheme defaults to dark when matchMedia does not match light and no localStorage", () => {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }));
    initTheme();
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("(d) setTheme('light') stamps attribute and persists to localStorage", () => {
    setTheme("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
    expect(localStorage.getItem(THEME_KEY)).toBe("light");
  });

  it("(d) setTheme('dark') stamps attribute and persists to localStorage", () => {
    setTheme("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    expect(localStorage.getItem(THEME_KEY)).toBe("dark");
  });

  it("(e) getTheme returns current data-theme attribute", () => {
    document.documentElement.setAttribute("data-theme", "light");
    expect(getTheme()).toBe("light");
  });

  it("(e) getTheme returns dark as default when attribute is absent", () => {
    document.documentElement.removeAttribute("data-theme");
    expect(getTheme()).toBe("dark");
  });

  it("localStorage takes precedence over matchMedia", () => {
    localStorage.setItem(THEME_KEY, "light");
    vi.stubGlobal("matchMedia", (query: string) => ({
      // matchMedia says dark (not light)
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }));
    initTheme();
    // localStorage wins → light
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });
});
