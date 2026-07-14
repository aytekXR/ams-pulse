/**
 * OnboardingGuard — post-auth onboarding redirect tests.
 *
 * (a) authed + zero sources + not env-configured → redirected to /onboarding
 * (b) authed + ≥1 source                         → NOT redirected
 * (c) sources fetch throws                        → NOT redirected (fail open)
 * (d) already on /onboarding                      → NOT redirected (no double-trigger)
 * (e) deliberate navigation elsewhere             → NOT hijacked
 * (f) AMS env-configured (empty ams_sources)      → NOT redirected (protects operator)
 * (g) dismissal flag already set                  → NOT redirected
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";
import { OnboardingGuard, ONBOARDING_DISMISSED_KEY } from "../../App";

// ─── Mock adminApi ────────────────────────────────────────────────────────────
//
// vi.mock is hoisted, so the factory must reference variables declared BEFORE
// the mock call. We capture a stable ref then reassign per-test in beforeEach.

const mockGetSources = vi.fn();

vi.mock("@/api/client", () => ({
  adminApi: {
    getSources: (...args: unknown[]) => mockGetSources(...args),
  },
  // AuthGate is NOT rendered here; these stubs satisfy any transitive imports.
  getToken: () => "plt_test",
  setToken: vi.fn(),
  clearToken: vi.fn(),
}));

// The guard also GETs /healthz (unauthenticated) to read ams_env_configured.
// Default stub: NOT env-configured, so the sources check decides. Individual
// tests override this to exercise the env-configured branch.
function stubHealthz(amsEnvConfigured = false) {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ status: "ok", ams_env_configured: amsEnvConfigured }),
    }),
  );
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

/** Displays the current router pathname so tests can assert navigation. */
function LocationDisplay() {
  const { pathname } = useLocation();
  return <div data-testid="path">{pathname}</div>;
}

/**
 * Renders OnboardingGuard inside a MemoryRouter.
 * LocationDisplay is outside Routes so it always reflects the current path.
 * The /onboarding route renders a sentinel element so tests can assert the
 * redirect landed on the correct page.
 */
function renderGuard(initialPath = "/") {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <LocationDisplay />
      {/*
       * OnboardingGuard sits OUTSIDE <Routes>, exactly as it does in App.tsx, so
       * it stays mounted on every path — including /onboarding itself. Mounting it
       * on a wildcard <Route> instead would mean React Router matched the specific
       * /onboarding route and never rendered the guard at all, and case (d) below
       * would pass without executing a single line of the code it claims to test.
       */}
      <OnboardingGuard />
      <Routes>
        <Route path="/onboarding" element={<div data-testid="onboarding">Wizard</div>} />
        <Route path="*" element={<div data-testid="app">App</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

// ─── Tests ───────────────────────────────────────────────────────────────────

describe("OnboardingGuard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    stubHealthz(false); // default: not env-configured
  });

  afterEach(() => {
    vi.clearAllMocks();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("(a) zero sources → redirected to /onboarding", async () => {
    mockGetSources.mockResolvedValue({ items: [], meta: {} });

    renderGuard("/");

    await waitFor(() => {
      expect(screen.getByTestId("path")).toHaveTextContent("/onboarding");
    });
    expect(screen.getByTestId("onboarding")).toBeInTheDocument();
  });

  it("(b) one or more sources → NOT redirected", async () => {
    mockGetSources.mockResolvedValue({
      items: [{ id: "s1", name: "Live AMS", type: "rest_poll" }],
      meta: { total: 1 },
    });

    renderGuard("/");

    // Wait for the fetch to complete before asserting no redirect happened.
    await waitFor(() => expect(mockGetSources).toHaveBeenCalledOnce());
    expect(screen.getByTestId("path")).toHaveTextContent("/");
    expect(screen.queryByTestId("onboarding")).not.toBeInTheDocument();
  });

  it("(c) sources fetch throws → NOT redirected (fail open)", async () => {
    mockGetSources.mockRejectedValue(new Error("Network error"));

    renderGuard("/");

    await waitFor(() => expect(mockGetSources).toHaveBeenCalledOnce());
    expect(screen.getByTestId("path")).toHaveTextContent("/");
    expect(screen.queryByTestId("onboarding")).not.toBeInTheDocument();
  });

  it("(d) already on /onboarding → fetch never called, no redirect loop", async () => {
    mockGetSources.mockResolvedValue({ items: [], meta: {} });

    renderGuard("/onboarding");

    // Let the event loop settle — getSources must not be called.
    await new Promise((r) => setTimeout(r, 50));
    expect(mockGetSources).not.toHaveBeenCalled();
    expect(screen.getByTestId("path")).toHaveTextContent("/onboarding");
  });

  // The guard nudges a user who lands on the dashboard; it must not hijack a
  // deliberate navigation. Without this, a user with no sources is trapped: every
  // route — including the Settings page they need in order to add a source — snaps
  // back to the wizard.
  it("(e) deliberate navigation to another page is never hijacked, even with zero sources", async () => {
    mockGetSources.mockResolvedValue({ items: [], meta: {} });

    renderGuard("/settings");

    await new Promise((r) => setTimeout(r, 50));
    expect(mockGetSources).not.toHaveBeenCalled();
    expect(screen.getByTestId("path")).toHaveTextContent("/settings");
    expect(screen.queryByTestId("onboarding")).not.toBeInTheDocument();
  });

  // The operator-protection case. Prod and the documented quickstart configure AMS
  // via PULSE_AMS_URL and leave ams_sources empty. Such a deployment reports
  // ams_env_configured=true and must NEVER be redirected — not even once — or the
  // operator lands in a "connect your first AMS" wizard for a running system.
  it("(f) AMS env-configured → NOT redirected, and getSources is never called", async () => {
    stubHealthz(true);
    mockGetSources.mockResolvedValue({ items: [], meta: {} });

    renderGuard("/");

    // Give the /healthz promise chain time to settle.
    await new Promise((r) => setTimeout(r, 50));
    // env-configured short-circuits before the source check — the operator is
    // never even queried for sources, let alone redirected.
    expect(mockGetSources).not.toHaveBeenCalled();
    expect(screen.getByTestId("path")).toHaveTextContent("/");
    expect(screen.queryByTestId("onboarding")).not.toBeInTheDocument();
  });

  it("(g) dismissal flag already set → NOT redirected, no fetch", async () => {
    localStorage.setItem(ONBOARDING_DISMISSED_KEY, "1");
    mockGetSources.mockResolvedValue({ items: [], meta: {} });

    renderGuard("/");

    await new Promise((r) => setTimeout(r, 50));
    expect(mockGetSources).not.toHaveBeenCalled();
    expect(screen.getByTestId("path")).toHaveTextContent("/");
    expect(screen.queryByTestId("onboarding")).not.toBeInTheDocument();
  });
});
