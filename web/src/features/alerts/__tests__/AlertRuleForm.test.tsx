/**
 * Alert rule form validation tests.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { AlertRuleForm } from "../AlertRuleForm";
import type { AlertRuleWrite } from "@/lib/api/types";

describe("AlertRuleForm", () => {
  it("renders the form with default fields", () => {
    render(<AlertRuleForm onSave={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.getByRole("heading", { name: /new alert rule/i })).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/e\.g\. High CPU/i)).toBeInTheDocument();
  });

  it("shows validation error when name is empty on submit", async () => {
    render(<AlertRuleForm onSave={vi.fn()} onCancel={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /save rule/i }));
    await waitFor(() => {
      expect(screen.getByText(/name is required/i)).toBeInTheDocument();
    });
  });

  it("shows validation error when threshold is not a number", async () => {
    render(<AlertRuleForm onSave={vi.fn()} onCancel={vi.fn()} />);
    // Fill name
    fireEvent.change(screen.getByPlaceholderText(/e\.g\. High CPU/i), {
      target: { value: "Test rule" },
    });
    // Set invalid threshold
    const thresholdInput = screen.getByPlaceholderText("0");
    fireEvent.change(thresholdInput, { target: { value: "abc" } });
    fireEvent.click(screen.getByRole("button", { name: /save rule/i }));
    await waitFor(() => {
      expect(screen.getByText(/valid number required/i)).toBeInTheDocument();
    });
  });

  it("calls onSave with correct data when form is valid", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<AlertRuleForm onSave={onSave} onCancel={vi.fn()} />);

    fireEvent.change(screen.getByPlaceholderText(/e\.g\. High CPU/i), {
      target: { value: "CPU alert" },
    });
    fireEvent.change(screen.getByPlaceholderText("0"), {
      target: { value: "80" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save rule/i }));

    await waitFor(() => {
      expect(onSave).toHaveBeenCalledOnce();
      const [data] = onSave.mock.calls[0] as [{ name: string; threshold: number; enabled: boolean }];
      // CR-1: name is now the real contract field (no longer stored in group_by)
      expect(data.name).toBe("CPU alert");
      expect(data.threshold).toBe(80);
      // CR-2: enabled defaults to true
      expect(data.enabled).toBe(true);
    });
  });

  it("calls onCancel when cancel is clicked", () => {
    const onCancel = vi.fn();
    render(<AlertRuleForm onSave={vi.fn()} onCancel={onCancel} />);
    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onCancel).toHaveBeenCalledOnce();
  });

  it("shows edit heading when initial data provided", () => {
    const initial = {
      id: "rule-1",
      name: "High CPU",
      metric: "cpu_pct",
      operator: "gt" as const,
      threshold: 90,
      window_s: 300,
      severity: "critical" as const,
      cooldown_s: 300,
      enabled: true,
      muted: false,
      created_at: 0,
      updated_at: 0,
      rule_type: "threshold" as const,
      sigma: 4,
      min_samples: 30,
    };
    render(<AlertRuleForm initial={initial} onSave={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.getByRole("heading", { name: /edit rule/i })).toBeInTheDocument();
  });
});

// ── D-087: metric list content pins ────────────────────────────────────────
describe("AlertRuleForm — metric list content (D-087)", () => {
  it("ANOMALY_METRICS includes ams_api_latency_ms", () => {
    render(<AlertRuleForm onSave={vi.fn()} onCancel={vi.fn()} />);
    // Switch to anomaly mode so the anomaly metric dropdown is rendered.
    fireEvent.change(screen.getByLabelText(/rule type/i), { target: { value: "anomaly" } });
    // ams_api_latency_ms must be present as an option in the metric dropdown.
    const options = screen.getAllByRole("option");
    const labels = options.map((o) => o.textContent ?? "");
    expect(labels).toContain("ams_api_latency_ms");
  });

  it("threshold METRICS includes node_degraded", () => {
    render(<AlertRuleForm onSave={vi.fn()} onCancel={vi.fn()} />);
    // Default mode is threshold.
    const options = screen.getAllByRole("option");
    const labels = options.map((o) => o.textContent ?? "");
    expect(labels).toContain("node_degraded");
  });

  it("threshold METRICS includes node_down", () => {
    render(<AlertRuleForm onSave={vi.fn()} onCancel={vi.fn()} />);
    // Default mode is threshold.
    const options = screen.getAllByRole("option");
    const labels = options.map((o) => o.textContent ?? "");
    expect(labels).toContain("node_down");
  });
});

// ── S11 WO-B: anomaly rule type tests ──────────────────────────────────────
describe("AlertRuleForm — anomaly rule type (S11 WO-B)", () => {
  it("renders threshold fields by default", () => {
    render(<AlertRuleForm onSave={vi.fn()} onCancel={vi.fn()} />);
    // Threshold mode: threshold input visible, sigma/min-samples absent
    expect(screen.getByPlaceholderText("0")).toBeInTheDocument();
    expect(screen.queryByLabelText(/sigma/i)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/min samples/i)).not.toBeInTheDocument();
  });

  it("renders anomaly fields when rule_type is anomaly", () => {
    render(<AlertRuleForm onSave={vi.fn()} onCancel={vi.fn()} />);
    // Switch to anomaly
    const ruleTypeSelect = screen.getByLabelText(/rule type/i);
    fireEvent.change(ruleTypeSelect, { target: { value: "anomaly" } });
    // Sigma and Min Samples appear
    expect(screen.getByLabelText(/sigma/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/min samples/i)).toBeInTheDocument();
    // Threshold input hidden
    expect(screen.queryByPlaceholderText("0")).not.toBeInTheDocument();
  });

  it("validates sigma must be a positive number", async () => {
    render(<AlertRuleForm onSave={vi.fn()} onCancel={vi.fn()} />);
    // Switch to anomaly
    fireEvent.change(screen.getByLabelText(/rule type/i), { target: { value: "anomaly" } });
    // Fill name
    fireEvent.change(screen.getByPlaceholderText(/e\.g\. High CPU/i), {
      target: { value: "Anomaly rule" },
    });
    // Clear sigma (set to empty)
    fireEvent.change(screen.getByLabelText(/sigma/i), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: /save rule/i }));
    await waitFor(() => {
      expect(screen.getByText(/sigma must be a positive number/i)).toBeInTheDocument();
    });
  });

  it("does not validate threshold for anomaly rule", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<AlertRuleForm onSave={onSave} onCancel={vi.fn()} />);
    // Switch to anomaly
    fireEvent.change(screen.getByLabelText(/rule type/i), { target: { value: "anomaly" } });
    // Fill name only — no threshold interaction needed
    fireEvent.change(screen.getByPlaceholderText(/e\.g\. High CPU/i), {
      target: { value: "Anomaly test" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save rule/i }));
    await waitFor(() => {
      expect(onSave).toHaveBeenCalledOnce();
    });
    // No threshold validation error
    expect(screen.queryByText(/valid number required/i)).not.toBeInTheDocument();
  });

  it("submits correct payload for anomaly rule", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<AlertRuleForm onSave={onSave} onCancel={vi.fn()} />);
    // Switch to anomaly
    fireEvent.change(screen.getByLabelText(/rule type/i), { target: { value: "anomaly" } });
    // Fill name
    fireEvent.change(screen.getByPlaceholderText(/e\.g\. High CPU/i), {
      target: { value: "Viewer anomaly" },
    });
    // Set sigma and min_samples
    fireEvent.change(screen.getByLabelText(/sigma/i), { target: { value: "2.5" } });
    fireEvent.change(screen.getByLabelText(/min samples/i), { target: { value: "5" } });
    fireEvent.click(screen.getByRole("button", { name: /save rule/i }));
    await waitFor(() => {
      expect(onSave).toHaveBeenCalledOnce();
    });
    const [data] = onSave.mock.calls[0] as [AlertRuleWrite];
    expect(data.rule_type).toBe("anomaly");
    expect(data.sigma).toBe(2.5);
    expect(data.min_samples).toBe(5);
  });

  it("submits correct payload for threshold rule", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<AlertRuleForm onSave={onSave} onCancel={vi.fn()} />);
    // Fill name and threshold (default rule type is threshold)
    fireEvent.change(screen.getByPlaceholderText(/e\.g\. High CPU/i), {
      target: { value: "CPU threshold" },
    });
    fireEvent.change(screen.getByPlaceholderText("0"), {
      target: { value: "99.5" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save rule/i }));
    await waitFor(() => {
      expect(onSave).toHaveBeenCalledOnce();
    });
    const [data] = onSave.mock.calls[0] as [AlertRuleWrite];
    expect(data.rule_type).toBe("threshold");
    expect(data.operator).toBe("gt");
    expect(data.threshold).toBe(99.5);
  });
});
