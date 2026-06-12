/**
 * Alert rule form validation tests.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { AlertRuleForm } from "../AlertRuleForm";

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
    // Fill name/label
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
      const [data] = onSave.mock.calls[0] as [{ group_by?: string; threshold: number }];
      // label is stored in group_by (pending contract change to add name field)
      expect(data.group_by).toBe("CPU alert");
      expect(data.threshold).toBe(80);
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
      metric: "cpu_pct",
      operator: "gt" as const,
      threshold: 90,
      window_s: 300,
      severity: "critical" as const,
      cooldown_s: 300,
      muted: false,
      created_at: 0,
      updated_at: 0,
    };
    render(<AlertRuleForm initial={initial} onSave={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.getByRole("heading", { name: /edit rule/i })).toBeInTheDocument();
  });
});
