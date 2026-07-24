/**
 * OnboardingWizard rendering tests.
 *
 * Covers:
 * (a) Smoke — mounts without crash; welcome step is shown.
 * (b) Step indicator labels are visible.
 * (c) "Get started" button advances to the source step.
 * (d) Source step — form fields and Back/Add source buttons present.
 * (e) Validation — empty name + URL shows error on submit.
 * (f) Wave 4: SVG checkmark replaces the &#10003; entity on the done step.
 * (g) Wave 4: Escape route — "Skip setup" button is always visible.
 * (h) Wave 4: Back navigation preserves source form state.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { OnboardingWizard } from "../OnboardingWizard";

const mockCreateSource = vi.fn();
const mockTestSource = vi.fn();
const mockUpdateSource = vi.fn();

vi.mock("@/api/client", () => ({
  adminApi: {
    createSource: (...args: unknown[]) => mockCreateSource(...args),
    testSource: (...args: unknown[]) => mockTestSource(...args),
    updateSource: (...args: unknown[]) => mockUpdateSource(...args),
  },
  ApiError: class ApiError extends Error {
    status: number;
    body: unknown;
    constructor(status: number, body: { message?: string }) {
      super(body.message ?? `HTTP ${status}`);
      this.status = status;
      this.body = body;
      this.name = "ApiError";
    }
  },
}));

describe("OnboardingWizard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("(a) smoke — renders without crash and shows welcome heading", () => {
    render(<OnboardingWizard onComplete={vi.fn()} />);
    expect(screen.getByRole("heading", { name: /welcome to pulse/i })).toBeInTheDocument();
  });

  it("(b) step indicator labels are visible", () => {
    render(<OnboardingWizard onComplete={vi.fn()} />);
    expect(screen.getByText("Welcome")).toBeInTheDocument();
    expect(screen.getByText("Add source")).toBeInTheDocument();
    expect(screen.getByText("Verify")).toBeInTheDocument();
    expect(screen.getByText("Done")).toBeInTheDocument();
  });

  it("(b) welcome step — shows introductory text about AMS", () => {
    render(<OnboardingWizard onComplete={vi.fn()} />);
    expect(screen.getByText(/ant media server/i)).toBeInTheDocument();
  });

  it("(c) Get started button advances to the source step", async () => {
    render(<OnboardingWizard onComplete={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /get started/i }));
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /add ams source/i })).toBeInTheDocument();
    });
  });

  it("(d) source step — Name and AMS REST URL fields are present after navigating", async () => {
    render(<OnboardingWizard onComplete={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /get started/i }));
    await waitFor(() => {
      expect(screen.getByPlaceholderText(/production cluster/i)).toBeInTheDocument();
      expect(screen.getByPlaceholderText(/your-ams-server/i)).toBeInTheDocument();
    });
  });

  it("(d) source step — Back and Add source buttons are present", async () => {
    render(<OnboardingWizard onComplete={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /get started/i }));
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /back/i })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /add source/i })).toBeInTheDocument();
    });
  });

  it("(e) source step — shows validation error when submitted without required fields", async () => {
    render(<OnboardingWizard onComplete={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /get started/i }));
    await waitFor(() => { expect(screen.getByPlaceholderText(/production cluster/i)).toBeInTheDocument(); });
    fireEvent.click(screen.getByRole("button", { name: /add source/i }));
    await waitFor(() => {
      expect(screen.getByText(/name and rest url are required/i)).toBeInTheDocument();
    });
  });
});

// ── (f) Wave 4: SVG checkmark on done step ──────────────────────────────────

describe("OnboardingWizard — done step SVG checkmark (Wave 4)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // createSource succeeds → wizard can reach the done step via verify
    mockCreateSource.mockResolvedValue({ id: "src-1" });
  });

  it("done step renders an SVG element instead of a text checkmark", async () => {
    render(<OnboardingWizard onComplete={vi.fn()} />);
    // Navigate to done step: welcome -> source -> verify -> done
    fireEvent.click(screen.getByRole("button", { name: /get started/i }));
    await waitFor(() => expect(screen.getByPlaceholderText(/production cluster/i)).toBeInTheDocument());
    // Fill required fields
    fireEvent.change(screen.getByPlaceholderText(/production cluster/i), { target: { value: "Prod" } });
    fireEvent.change(screen.getByPlaceholderText(/your-ams-server/i), { target: { value: "http://ams:5080" } });
    fireEvent.click(screen.getByRole("button", { name: /add source/i }));
    // At verify step
    await waitFor(() => expect(screen.getByRole("heading", { name: /verify connection/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /continue/i }));
    // At done step
    await waitFor(() => expect(screen.getByRole("heading", { name: /you are connected/i })).toBeInTheDocument());
    // SVG must be present; the old ✓ entity should NOT appear as a plain text node
    const svgEl = document.querySelector("svg[aria-hidden='true']");
    expect(svgEl).toBeInTheDocument();
    // The raw checkmark entity text "✓" should not be present as visible text
    expect(screen.queryByText("✓")).not.toBeInTheDocument();
  });
});

// ── (g) Wave 4: Escape route ─────────────────────────────────────────────────

describe("OnboardingWizard — escape route (Wave 4)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("'Skip setup' button is visible on the welcome step", () => {
    render(<OnboardingWizard onComplete={vi.fn()} />);
    expect(screen.getByRole("button", { name: /skip setup/i })).toBeInTheDocument();
  });

  it("'Skip setup' button calls onComplete", () => {
    const onComplete = vi.fn();
    render(<OnboardingWizard onComplete={onComplete} />);
    fireEvent.click(screen.getByRole("button", { name: /skip setup/i }));
    expect(onComplete).toHaveBeenCalledOnce();
  });

  it("'Skip setup' button remains visible on the source step", async () => {
    render(<OnboardingWizard onComplete={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /get started/i }));
    await waitFor(() => expect(screen.getByPlaceholderText(/production cluster/i)).toBeInTheDocument());
    expect(screen.getByRole("button", { name: /skip setup/i })).toBeInTheDocument();
  });
});

// ── (h) Wave 4: Back navigation preserves form state ──────────────────────────

describe("OnboardingWizard — state-preserving back navigation (Wave 4)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("Back from source step to welcome preserves typed values", async () => {
    render(<OnboardingWizard onComplete={vi.fn()} />);
    // Go to source step
    fireEvent.click(screen.getByRole("button", { name: /get started/i }));
    await waitFor(() => expect(screen.getByPlaceholderText(/production cluster/i)).toBeInTheDocument());
    // Type a name
    fireEvent.change(screen.getByPlaceholderText(/production cluster/i), { target: { value: "My AMS" } });
    // Go back to welcome
    fireEvent.click(screen.getByRole("button", { name: /back/i }));
    await waitFor(() => expect(screen.getByRole("heading", { name: /welcome to pulse/i })).toBeInTheDocument());
    // Return to source step — field should still have the typed value
    fireEvent.click(screen.getByRole("button", { name: /get started/i }));
    await waitFor(() => {
      expect(screen.getByDisplayValue("My AMS")).toBeInTheDocument();
    });
  });
});

// ── (i) D-165: Back-then-resubmit is idempotent ──────────────────────────────
//
// Regression: before the fix, navigating Back from the verify step and
// re-submitting the source form called createSource a second time, producing
// a duplicate source in the database.  The fix switches to updateSource when
// createdSourceId is already set.

describe("OnboardingWizard — Back-then-resubmit idempotency (D-165)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateSource.mockResolvedValue({ id: "src-abc" });
    mockUpdateSource.mockResolvedValue({ id: "src-abc" });
  });

  /** Navigate welcome → source (filled) → verify via the first submit. */
  async function submitAndReachVerify() {
    render(<OnboardingWizard onComplete={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /get started/i }));
    await waitFor(() => expect(screen.getByPlaceholderText(/production cluster/i)).toBeInTheDocument());
    fireEvent.change(screen.getByPlaceholderText(/production cluster/i), { target: { value: "Prod" } });
    fireEvent.change(screen.getByPlaceholderText(/your-ams-server/i), { target: { value: "http://ams:5080" } });
    fireEvent.click(screen.getByRole("button", { name: /add source/i }));
    await waitFor(() => expect(screen.getByRole("heading", { name: /verify connection/i })).toBeInTheDocument());
  }

  it("going Back from verify and re-submitting calls createSource exactly once", async () => {
    await submitAndReachVerify();

    // Verify the first submit used createSource.
    expect(mockCreateSource).toHaveBeenCalledOnce();

    // Go Back to the source step.
    fireEvent.click(screen.getByRole("button", { name: /back/i }));
    await waitFor(() => expect(screen.getByRole("heading", { name: /add ams source/i })).toBeInTheDocument());

    // Re-submit — must NOT call createSource a second time.
    fireEvent.click(screen.getByRole("button", { name: /add source/i }));
    await waitFor(() => expect(screen.getByRole("heading", { name: /verify connection/i })).toBeInTheDocument());

    // Still exactly one create; the second submit used updateSource.
    expect(mockCreateSource).toHaveBeenCalledOnce();
    expect(mockUpdateSource).toHaveBeenCalledOnce();
    expect(mockUpdateSource).toHaveBeenCalledWith("src-abc", expect.objectContaining({ name: "Prod" }));
  });

  it("update failure on re-submit stays on source step and shows error", async () => {
    await submitAndReachVerify();

    fireEvent.click(screen.getByRole("button", { name: /back/i }));
    await waitFor(() => expect(screen.getByRole("heading", { name: /add ams source/i })).toBeInTheDocument());

    mockUpdateSource.mockRejectedValueOnce(new Error("update failed"));
    fireEvent.click(screen.getByRole("button", { name: /add source/i }));

    await waitFor(() => expect(screen.getByText(/failed to save source/i)).toBeInTheDocument());
    expect(screen.getByRole("heading", { name: /add ams source/i })).toBeInTheDocument();
    // createSource still called only once; no second create attempt.
    expect(mockCreateSource).toHaveBeenCalledOnce();
  });
});
