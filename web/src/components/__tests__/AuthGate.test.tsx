/**
 * AuthGate 401 redirect tests.
 *
 * Tests:
 * - Shows children when token is present
 * - Shows login form when no token
 * - 401 event clears token and shows login form
 * - Login form submits and shows children
 * - Token required validation
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { AuthGate } from "../AuthGate";

// Mock the API client token functions
const mockGetToken = vi.fn<() => string | null>();
const mockSetToken = vi.fn<(t: string) => void>();
const mockClearToken = vi.fn<() => void>();

vi.mock("@/api/client", () => ({
  getToken: () => mockGetToken(),
  setToken: (t: string) => mockSetToken(t),
  clearToken: () => mockClearToken(),
}));

describe("AuthGate — 401 redirect fix", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Existing tests don't mock fetch; stub it to resolve quickly so the
    // async OIDC-status/auth-me check doesn't stall. Errors are swallowed.
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("not mocked")));
  });

  afterEach(() => {
    vi.clearAllMocks();
    vi.unstubAllGlobals();
  });

  it("renders children when token is present", () => {
    mockGetToken.mockReturnValue("plt_abc123");
    render(
      <AuthGate>
        <div data-testid="protected">Protected content</div>
      </AuthGate>
    );
    expect(screen.getByTestId("protected")).toBeInTheDocument();
  });

  it("renders login form when no token", () => {
    mockGetToken.mockReturnValue(null);
    render(
      <AuthGate>
        <div>Protected</div>
      </AuthGate>
    );
    expect(screen.getByRole("button", { name: /sign in/i })).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/plt_/i)).toBeInTheDocument();
  });

  it("shows validation error when submitting empty token", async () => {
    mockGetToken.mockReturnValue(null);
    render(<AuthGate><div>Protected</div></AuthGate>);
    fireEvent.click(screen.getByRole("button", { name: /sign in/i }));
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
      expect(screen.getByText(/token is required/i)).toBeInTheDocument();
    });
  });

  it("calls setToken and shows children after successful login", async () => {
    mockGetToken.mockReturnValue(null);
    mockSetToken.mockImplementation(() => {
      // After setToken, simulate getToken returning the new value
      mockGetToken.mockReturnValue("plt_newtoken");
    });

    render(
      <AuthGate>
        <div data-testid="protected">Protected</div>
      </AuthGate>
    );

    const input = screen.getByPlaceholderText(/plt_/i);
    fireEvent.change(input, { target: { value: "plt_newtoken" } });
    fireEvent.click(screen.getByRole("button", { name: /sign in/i }));

    await waitFor(() => {
      expect(mockSetToken).toHaveBeenCalledWith("plt_newtoken");
    });
  });

  it("clears token and shows login form on pulse:auth:401 event", async () => {
    // Start authenticated
    mockGetToken.mockReturnValue("plt_existing");
    render(
      <AuthGate>
        <div data-testid="protected">Protected</div>
      </AuthGate>
    );

    // Verify protected content visible
    expect(screen.getByTestId("protected")).toBeInTheDocument();

    // Fire the 401 event (wave-2 fix)
    act(() => {
      window.dispatchEvent(new Event("pulse:auth:401"));
    });

    // AuthGate should clear token and show login form
    await waitFor(() => {
      expect(mockClearToken).toHaveBeenCalledOnce();
      // Should show the "Session expired" message
      expect(screen.getByText(/session expired/i)).toBeInTheDocument();
    });
  });
});

// ─── OIDC phase-2 tests (S14 WO-C) ──────────────────────────────────────────
//
// Uses the MODULE-LEVEL mockGetToken/mockSetToken/mockClearToken that were
// already captured in the module-level vi.mock("@/api/client") factory above.
// Do NOT re-declare or re-mock here — doing so inside a describe body causes
// vi.mock hoisting issues (vitest moves the call to file scope where the inner
// variable is in TDZ, resulting in the factory closing over the wrong binding).
//
// Global fetch is stubbed per-test via vi.stubGlobal so /auth/oidc/status and
// /auth/me never hit a real server and the pulse:auth:401 event is never fired
// by plain-fetch calls inside AuthGate (only apiFetch triggers that event).
describe("AuthGate — OIDC phase-2", () => {
  // Re-use module-level mock fns — they ARE the fns captured in the factory.
  // (mockGetToken, mockSetToken, mockClearToken are in file scope above.)

  beforeEach(() => {
    vi.clearAllMocks();
    // Default: no localStorage token; plain-fetch rejects (overridden per test)
    mockGetToken.mockReturnValue(null);
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("fetch not stubbed for this test")));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  /** Build a per-test fetch stub that handles both OIDC discovery endpoints. */
  function makeFetch(statusEnabled: boolean, meStatus: number) {
    return vi.fn((url: string) => {
      if (url === "/auth/oidc/status") {
        return Promise.resolve(
          new Response(JSON.stringify({ enabled: statusEnabled }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (url === "/auth/me") {
        return Promise.resolve(
          new Response(
            meStatus === 200
              ? JSON.stringify({ name: "oidc-session", role: "viewer", auth_method: "cookie" })
              : JSON.stringify({ code: "UNAUTHORIZED", message: "not authenticated" }),
            {
              status: meStatus,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      }
      return Promise.reject(new Error(`unmocked fetch: ${url}`));
    });
  }

  it("shows SSO button when OIDC is enabled and no token", async () => {
    vi.stubGlobal("fetch", makeFetch(true, 401));
    render(
      <AuthGate>
        <div>Protected</div>
      </AuthGate>,
    );
    // AuthGate fires /auth/oidc/status on mount; once enabled=true resolves,
    // the SSO button should appear in the login panel.
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /sign in with sso/i })).toBeInTheDocument();
    });
  });

  it("hides SSO button when OIDC is disabled", async () => {
    vi.stubGlobal("fetch", makeFetch(false, 401));
    render(
      <AuthGate>
        <div>Protected</div>
      </AuthGate>,
    );
    // Wait for the login form to appear (confirms fetch settled), then verify
    // the SSO button is absent because OIDC is disabled.
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /^sign in$/i })).toBeInTheDocument();
    });
    expect(screen.queryByRole("button", { name: /sign in with sso/i })).not.toBeInTheDocument();
  });

  it("renders children when /auth/me returns 200 (cookie session)", async () => {
    // No localStorage token; /auth/me 200 → cookieAuthed=true → children
    vi.stubGlobal("fetch", makeFetch(true, 200));
    render(
      <AuthGate>
        <div data-testid="protected">Protected content</div>
      </AuthGate>,
    );
    await waitFor(() => {
      expect(screen.getByTestId("protected")).toBeInTheDocument();
    });
    expect(screen.queryByRole("button", { name: /^sign in$/i })).not.toBeInTheDocument();
  });

  it("shows token panel when /auth/me returns 401 and no localStorage token", async () => {
    vi.stubGlobal("fetch", makeFetch(false, 401));
    render(
      <AuthGate>
        <div>Protected</div>
      </AuthGate>,
    );
    // After both fetches settle: login form visible; clearToken NOT called
    // (the 401 is from a plain fetch, not apiFetch — no pulse:auth:401 event).
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /^sign in$/i })).toBeInTheDocument();
    });
    expect(mockClearToken).not.toHaveBeenCalled();
  });

  it("401 event flow unchanged: pulse:auth:401 clears token and shows login form", async () => {
    mockGetToken.mockReturnValue("plt_existing");
    vi.stubGlobal("fetch", makeFetch(false, 401));
    render(
      <AuthGate>
        <div data-testid="protected">Protected</div>
      </AuthGate>,
    );
    expect(screen.getByTestId("protected")).toBeInTheDocument();

    act(() => {
      window.dispatchEvent(new Event("pulse:auth:401"));
    });

    await waitFor(() => {
      expect(mockClearToken).toHaveBeenCalledOnce();
      expect(screen.getByText(/session expired/i)).toBeInTheDocument();
    });
  });
});

// ─── Fail-open bug fix tests (D-074) ─────────────────────────────────────────
//
// Guards against the regression where any HTTP 200 from /auth/me (including an
// HTML SPA-fallback page) silently sets cookieAuthed=true and hides the gate.
// Test (a) MUST fail before the AuthGate fix and pass after.
describe("AuthGate — fail-open fix (D-074)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetToken.mockReturnValue(null);
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("not mocked")));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  /** Build a fetch stub that returns the given /auth/me response; /auth/oidc/status always returns disabled JSON. */
  function makeMeResponse(meRes: Response): ReturnType<typeof vi.fn> {
    return vi.fn((url: string) => {
      if (url === "/auth/oidc/status")
        return Promise.resolve(
          new Response(JSON.stringify({ enabled: false }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      if (url === "/auth/me") return Promise.resolve(meRes);
      return Promise.reject(new Error(`unmocked fetch: ${url}`));
    });
  }

  it("(a) 200 text/html response from /auth/me does NOT authenticate (fail-open guard)", async () => {
    // This is the SPA-fallback case: vite dev server (or a stale Go binary) answers
    // /auth/me with 200 + index.html.  The gate MUST still render.
    vi.stubGlobal(
      "fetch",
      makeMeResponse(
        new Response(
          "<!doctype html><html><head></head><body>SPA fallback</body></html>",
          { status: 200, headers: { "Content-Type": "text/html; charset=UTF-8" } },
        ),
      ),
    );

    render(
      <AuthGate>
        <div data-testid="protected">Protected</div>
      </AuthGate>,
    );

    // After both fetches settle the gate view must render and the protected
    // content must NOT be visible — cookieAuthed must NOT be set for HTML bodies.
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /^sign in$/i })).toBeInTheDocument();
    });
    expect(screen.queryByTestId("protected")).not.toBeInTheDocument();
  });

  it("(b) 200 application/json with valid auth_method field → children render", async () => {
    vi.stubGlobal(
      "fetch",
      makeMeResponse(
        new Response(
          JSON.stringify({ name: "oidc-user", role: "admin", auth_method: "cookie" }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      ),
    );

    render(
      <AuthGate>
        <div data-testid="protected">Protected</div>
      </AuthGate>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("protected")).toBeInTheDocument();
    });
    expect(screen.queryByRole("button", { name: /^sign in$/i })).not.toBeInTheDocument();
  });

  it("(c) /auth/me network error → gate renders (not authenticated)", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string) => {
        if (url === "/auth/oidc/status")
          return Promise.resolve(
            new Response(JSON.stringify({ enabled: false }), {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }),
          );
        if (url === "/auth/me")
          return Promise.reject(new TypeError("NetworkError: Failed to fetch"));
        return Promise.reject(new Error(`unmocked fetch: ${url}`));
      }),
    );

    render(
      <AuthGate>
        <div data-testid="protected">Protected</div>
      </AuthGate>,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /^sign in$/i })).toBeInTheDocument();
    });
    expect(screen.queryByTestId("protected")).not.toBeInTheDocument();
  });

  it("(d) /auth/me 401 → gate renders quietly (clearToken NOT called)", async () => {
    vi.stubGlobal(
      "fetch",
      makeMeResponse(
        new Response(JSON.stringify({ code: "UNAUTHORIZED", message: "not authenticated" }), {
          status: 401,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    render(
      <AuthGate>
        <div data-testid="protected">Protected</div>
      </AuthGate>,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /^sign in$/i })).toBeInTheDocument();
    });
    expect(mockClearToken).not.toHaveBeenCalled();
    expect(screen.queryByTestId("protected")).not.toBeInTheDocument();
  });
});
