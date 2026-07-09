/**
 * AlertChannelForm rendering and interaction tests.
 *
 * Covers:
 * (a) Smoke — renders without crash; "New notification channel" heading present.
 * (b) Default type — email fields shown initially; webhook URL field absent.
 * (c) Validation — empty name triggers "Name is required" error.
 * (d) Validation — empty email address triggers "Email address required" error.
 * (e) Type switch — selecting "slack" shows Slack webhook URL field.
 * (f) Type switch — selecting "pagerduty" shows env-var instruction message.
 * (g) Cancel button calls onCancel prop.
 * (h) Edit mode — heading shows "Edit channel" when initial prop is provided.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { AlertChannelForm } from "../AlertChannelForm";
import type { AlertChannel } from "@/lib/api/types";

describe("AlertChannelForm", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("(a) smoke — renders without crash and shows New notification channel heading", () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.getByRole("heading", { name: /new notification channel/i })).toBeInTheDocument();
  });

  it("(a) Channel name input is present", () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.getByPlaceholderText(/ops team slack/i)).toBeInTheDocument();
  });

  it("(b) email type shown by default — email To address field is present", () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.getByPlaceholderText(/alerts@example\.com/i)).toBeInTheDocument();
  });

  it("(b) webhook URL field absent when type is email", () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.queryByPlaceholderText(/hooks\.slack\.com/i)).not.toBeInTheDocument();
  });

  it("(c) validation — empty name shows Name is required error", async () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /save channel/i }));
    await waitFor(() => {
      expect(screen.getByText(/name is required/i)).toBeInTheDocument();
    });
  });

  it("(d) validation — missing email shows Email address required error", async () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    fireEvent.change(screen.getByPlaceholderText(/ops team slack/i), {
      target: { value: "Ops Alerts" },
    });
    // Leave email empty and submit
    fireEvent.click(screen.getByRole("button", { name: /save channel/i }));
    await waitFor(() => {
      expect(screen.getByText(/email address required/i)).toBeInTheDocument();
    });
  });

  it("(e) type switch — selecting slack shows Slack webhook URL field", async () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    const select = screen.getByRole("combobox");
    fireEvent.change(select, { target: { value: "slack" } });
    await waitFor(() => {
      expect(screen.getByPlaceholderText(/hooks\.slack\.com/i)).toBeInTheDocument();
    });
  });

  it("(e) type switch — selecting webhook shows Webhook URL field", async () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    const select = screen.getByRole("combobox");
    fireEvent.change(select, { target: { value: "webhook" } });
    await waitFor(() => {
      expect(screen.getByText(/webhook url \*/i)).toBeInTheDocument();
    });
  });

  it("(f) type switch — pagerduty shows env-var instruction", async () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    const select = screen.getByRole("combobox");
    fireEvent.change(select, { target: { value: "pagerduty" } });
    await waitFor(() => {
      expect(screen.getByText(/configure via environment variables/i)).toBeInTheDocument();
    });
  });

  it("(g) Cancel button calls onCancel", () => {
    const onCancel = vi.fn();
    render(<AlertChannelForm onSave={vi.fn()} onCancel={onCancel} />);
    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onCancel).toHaveBeenCalledOnce();
  });

  it("(h) edit mode — shows Edit channel heading when initial prop provided", () => {
    const initial: AlertChannel = {
      id: "ch-1",
      name: "Existing Channel",
      type: "email",
      config_summary: { email_to: "ops@example.com" },
      created_at: 1_000_000,
    };
    render(<AlertChannelForm initial={initial} onSave={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.getByRole("heading", { name: /edit channel/i })).toBeInTheDocument();
  });

  it("(h) edit mode — pre-fills name from initial prop", () => {
    const initial: AlertChannel = {
      id: "ch-1",
      name: "My Channel",
      type: "email",
      config_summary: {},
      created_at: 1_000_000,
    };
    render(<AlertChannelForm initial={initial} onSave={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.getByDisplayValue("My Channel")).toBeInTheDocument();
  });

  it("calls onSave with correct data when form is valid (email type)", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<AlertChannelForm onSave={onSave} onCancel={vi.fn()} />);
    fireEvent.change(screen.getByPlaceholderText(/ops team slack/i), {
      target: { value: "Ops Alerts" },
    });
    fireEvent.change(screen.getByPlaceholderText(/alerts@example\.com/i), {
      target: { value: "ops@example.com" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save channel/i }));
    await waitFor(() => {
      expect(onSave).toHaveBeenCalledOnce();
      const [data] = onSave.mock.calls[0] as [{ name: string; type: string; config: Record<string, string> }];
      expect(data.name).toBe("Ops Alerts");
      expect(data.type).toBe("email");
      expect(data.config).toMatchObject({ email_to: "ops@example.com" });
    });
  });
});
