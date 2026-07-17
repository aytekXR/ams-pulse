/**
 * OnboardingWizard — verify-step & error-path coverage (S83/D-145).
 *
 * The existing __tests__/OnboardingWizard.test.tsx covers welcome/source
 * navigation and reaches the done step, but never runs handleTest (the whole
 * verify flow, lines ~302–330) nor handleSourceSave's failure path, and never
 * touches the optional source fields (rest_user / credential_env_ref / log_path).
 * This file drives every reachable/unreachable/error branch of the connection
 * test plus both save-error branches. Pure test additions — no behavior change.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ApiError } from "@/api/client";
import { OnboardingWizard } from "../OnboardingWizard";

const h = vi.hoisted(() => ({ createSource: vi.fn(), testSource: vi.fn() }));

vi.mock("@/api/client", async (orig) => {
  const actual = await orig<typeof import("@/api/client")>();
  return {
    ...actual,
    adminApi: { createSource: h.createSource, testSource: h.testSource },
  };
});

/** Drive the wizard welcome → source → verify, filling every source field. */
async function gotoVerify() {
  h.createSource.mockResolvedValue({ id: "src-1" });
  render(<OnboardingWizard onComplete={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: /get started/i }));
  await screen.findByPlaceholderText(/production cluster/i);

  fireEvent.change(screen.getByPlaceholderText(/production cluster/i), { target: { value: "Prod" } });
  fireEvent.change(screen.getByPlaceholderText(/your-ams-server/i), {
    target: { value: "http://ams:5080" },
  });
  // Optional fields — exercise the remaining onChange handlers.
  fireEvent.change(screen.getByPlaceholderText("admin"), { target: { value: "amsadmin" } });
  fireEvent.change(screen.getByPlaceholderText("AMS_ADMIN_PASSWORD"), {
    target: { value: "AMS_PW_ENV" },
  });
  fireEvent.change(screen.getByPlaceholderText(/ant-media-server\.log/), {
    target: { value: "/var/log/ams.log" },
  });

  fireEvent.click(screen.getByRole("button", { name: /add source/i }));
  await screen.findByRole("heading", { name: /verify connection/i });
}

describe("OnboardingWizard — connection test (verify step)", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => vi.restoreAllMocks());

  it("reports a reachable source with latency and version, then continues to done", async () => {
    await gotoVerify();
    h.testSource.mockResolvedValue({ reachable: true, latency_ms: 42, version: "2.9.0" });
    fireEvent.click(screen.getByRole("button", { name: /test connection/i }));

    expect(await screen.findByText(/connection verified \(42 ms, AMS 2\.9\.0\)/i)).toBeInTheDocument();
    // A verified test should let Continue advance to the done step.
    fireEvent.click(screen.getByRole("button", { name: /continue/i }));
    expect(await screen.findByRole("heading", { name: /you are connected/i })).toBeInTheDocument();
  });

  it("reports a reachable source without latency detail", async () => {
    await gotoVerify();
    h.testSource.mockResolvedValue({ reachable: true });
    fireEvent.click(screen.getByRole("button", { name: /test connection/i }));
    expect(await screen.findByText(/connection verified successfully/i)).toBeInTheDocument();
  });

  it("shows the server-supplied error when the source is unreachable", async () => {
    await gotoVerify();
    h.testSource.mockResolvedValue({ reachable: false, error: "connection refused" });
    fireEvent.click(screen.getByRole("button", { name: /test connection/i }));
    expect(await screen.findByText(/connection refused/i)).toBeInTheDocument();
  });

  it("falls back to 'Source unreachable' when no error detail is given", async () => {
    await gotoVerify();
    h.testSource.mockResolvedValue({ reachable: false });
    fireEvent.click(screen.getByRole("button", { name: /test connection/i }));
    expect(await screen.findByText(/source unreachable/i)).toBeInTheDocument();
  });

  it("surfaces an ApiError message when the test call throws", async () => {
    await gotoVerify();
    h.testSource.mockRejectedValue(new ApiError(502, { code: "BAD_GATEWAY", message: "gateway timeout" }));
    fireEvent.click(screen.getByRole("button", { name: /test connection/i }));
    expect(await screen.findByText(/gateway timeout/i)).toBeInTheDocument();
  });

  it("shows 'Test failed' for a non-ApiError rejection", async () => {
    await gotoVerify();
    h.testSource.mockRejectedValue(new Error("boom"));
    fireEvent.click(screen.getByRole("button", { name: /test connection/i }));
    expect(await screen.findByText(/test failed/i)).toBeInTheDocument();
  });

  it("shows a testing spinner while the check is in flight", async () => {
    await gotoVerify();
    let resolve!: (v: unknown) => void;
    h.testSource.mockReturnValue(new Promise((r) => { resolve = r; }));
    fireEvent.click(screen.getByRole("button", { name: /test connection/i }));
    expect(await screen.findByText(/testing connection/i)).toBeInTheDocument();
    resolve({ reachable: true });
    await screen.findByText(/connection verified/i);
  });
});

describe("OnboardingWizard — source save failures", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => vi.restoreAllMocks());

  async function fillAndSubmit() {
    render(<OnboardingWizard onComplete={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /get started/i }));
    await screen.findByPlaceholderText(/production cluster/i);
    fireEvent.change(screen.getByPlaceholderText(/production cluster/i), { target: { value: "Prod" } });
    fireEvent.change(screen.getByPlaceholderText(/your-ams-server/i), {
      target: { value: "http://ams:5080" },
    });
    fireEvent.click(screen.getByRole("button", { name: /add source/i }));
  }

  it("shows the API error and stays on the source step (ApiError)", async () => {
    h.createSource.mockRejectedValue(new ApiError(409, { code: "CONFLICT", message: "source exists" }));
    await fillAndSubmit();
    expect(await screen.findByText(/source exists/i)).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /add ams source/i })).toBeInTheDocument();
  });

  it("shows a generic message for a non-ApiError failure", async () => {
    h.createSource.mockRejectedValue(new Error("network down"));
    await fillAndSubmit();
    expect(await screen.findByText(/failed to save source/i)).toBeInTheDocument();
  });
});
