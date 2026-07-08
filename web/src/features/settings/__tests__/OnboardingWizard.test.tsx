/**
 * OnboardingWizard rendering tests.
 *
 * Covers:
 * (a) Smoke — mounts without crash; welcome step is shown.
 * (b) Step indicator labels are visible.
 * (c) "Get started" button advances to the source step.
 * (d) Source step — form fields and Back/Add source buttons present.
 * (e) Validation — empty name + URL shows error on submit.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { OnboardingWizard } from "../OnboardingWizard";

const mockCreateSource = vi.fn();
const mockTestSource = vi.fn();

vi.mock("@/api/client", () => ({
  adminApi: {
    createSource: (...args: unknown[]) => mockCreateSource(...args),
    testSource: (...args: unknown[]) => mockTestSource(...args),
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
