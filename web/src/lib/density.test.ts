/**
 * density.ts unit tests — TDD red-first (B1 work order)
 *
 * Tests:
 * (a) initDensity reads localStorage and stamps data-density attribute
 * (b) initDensity defaults to "default" when no localStorage entry
 * (c) setDensity updates attribute and persists to localStorage
 * (d) getDensity returns current density
 * (e) ROW_HEIGHT_MAP has the correct values per work order
 */
import { describe, it, expect, beforeEach } from "vitest";

// Module not yet created — these imports cause RED failures
import { initDensity, getDensity, setDensity, DENSITY_KEY, ROW_HEIGHT_MAP } from "./density";

describe("density utilities", () => {
  beforeEach(() => {
    document.documentElement.removeAttribute("data-density");
    localStorage.clear();
  });

  it("(a) initDensity stamps data-density=compact from localStorage", () => {
    localStorage.setItem(DENSITY_KEY, "compact");
    initDensity();
    expect(document.documentElement.getAttribute("data-density")).toBe("compact");
  });

  it("(a) initDensity stamps data-density=wall from localStorage", () => {
    localStorage.setItem(DENSITY_KEY, "wall");
    initDensity();
    expect(document.documentElement.getAttribute("data-density")).toBe("wall");
  });

  it("(b) initDensity defaults to default when no localStorage entry", () => {
    initDensity();
    // "default" should either stamp data-density="default" or remove the attribute
    // We assert that getDensity() returns "default"
    expect(getDensity()).toBe("default");
  });

  it("(c) setDensity('compact') stamps attribute and persists to localStorage", () => {
    setDensity("compact");
    expect(document.documentElement.getAttribute("data-density")).toBe("compact");
    expect(localStorage.getItem(DENSITY_KEY)).toBe("compact");
  });

  it("(c) setDensity('wall') stamps attribute and persists to localStorage", () => {
    setDensity("wall");
    expect(document.documentElement.getAttribute("data-density")).toBe("wall");
    expect(localStorage.getItem(DENSITY_KEY)).toBe("wall");
  });

  it("(c) setDensity('default') persists to localStorage", () => {
    setDensity("default");
    expect(localStorage.getItem(DENSITY_KEY)).toBe("default");
  });

  it("(d) getDensity returns compact when attribute is compact", () => {
    document.documentElement.setAttribute("data-density", "compact");
    expect(getDensity()).toBe("compact");
  });

  it("(d) getDensity returns default when attribute is absent", () => {
    document.documentElement.removeAttribute("data-density");
    expect(getDensity()).toBe("default");
  });

  it("(e) ROW_HEIGHT_MAP has correct values from spec", () => {
    expect(ROW_HEIGHT_MAP.default).toBe(40);
    expect(ROW_HEIGHT_MAP.compact).toBe(32);
    expect(ROW_HEIGHT_MAP.wall).toBe(48);
  });
});
