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
  });

  afterEach(() => {
    vi.clearAllMocks();
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
    expect(screen.getByPlaceholderText(/pulse_tok/i)).toBeInTheDocument();
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

    const input = screen.getByPlaceholderText(/pulse_tok/i);
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
