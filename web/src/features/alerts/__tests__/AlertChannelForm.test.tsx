/**
 * AlertChannelForm rendering, interaction, and a11y tests.
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
 * (i) a11y — aria-invalid + aria-describedby wired correctly on error.
 * (j) a11y — aria-live region present and populated on error.
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

// ── (i) a11y: aria-invalid + aria-describedby ──────────────────────────────

describe("AlertChannelForm — a11y: aria-invalid + aria-describedby", () => {
  it("name input gains aria-invalid='true' when name validation fails", async () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /save channel/i }));
    await waitFor(() => {
      const nameInput = screen.getByPlaceholderText(/ops team slack/i);
      expect(nameInput).toHaveAttribute("aria-invalid", "true");
    });
  });

  it("name input aria-describedby references an element containing the error text", async () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /save channel/i }));
    await waitFor(() => {
      const nameInput = screen.getByPlaceholderText(/ops team slack/i);
      const describedById = nameInput.getAttribute("aria-describedby");
      expect(describedById).toBeTruthy();
      const errorEl = document.getElementById(describedById!);
      expect(errorEl).toBeInTheDocument();
      expect(errorEl?.textContent).toMatch(/name is required/i);
    });
  });

  it("email input gains aria-invalid when email is missing", async () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    // Fill name so only emailTo fails
    fireEvent.change(screen.getByPlaceholderText(/ops team slack/i), { target: { value: "Ops" } });
    fireEvent.click(screen.getByRole("button", { name: /save channel/i }));
    await waitFor(() => {
      const emailInput = screen.getByPlaceholderText(/alerts@example\.com/i);
      expect(emailInput).toHaveAttribute("aria-invalid", "true");
    });
  });

  it("inputs have no aria-invalid when form is not yet submitted", () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    const nameInput = screen.getByPlaceholderText(/ops team slack/i);
    // Before submit: aria-invalid must not be present (not even "false")
    expect(nameInput).not.toHaveAttribute("aria-invalid");
  });
});

// ── (j) a11y: aria-live error region ────────────────────────────────────────

/**
 * As in AlertRuleForm: the inline message IS the live region. These replace two tests that
 * pinned a separate sr-only aria-live div duplicating every message — which made screen
 * readers announce each error twice.
 */
describe("AlertChannelForm — a11y: the inline error is the live region", () => {
  it("the error is announced (role=alert) and appears exactly ONCE in the DOM", async () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /save channel/i }));

    // One alert per invalid field is correct; the same message twice is not.
    const alerts = await screen.findAllByRole("alert");
    expect(alerts.some((a) => /name is required/i.test(a.textContent ?? ""))).toBe(true);
    expect(screen.getAllByText(/name is required/i)).toHaveLength(1);
    expect(document.querySelector("[aria-live='polite']")).toBeNull();
  });

  it("the invalid field points at a message node that actually exists", async () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /save channel/i }));

    const nameInput = await screen.findByLabelText(/name/i);
    await waitFor(() => expect(nameInput).toHaveAttribute("aria-invalid", "true"));
    const describedBy = nameInput.getAttribute("aria-describedby");
    expect(describedBy).toBe("ch-name-error");
    expect(document.getElementById(describedBy!)).toHaveTextContent(/name is required/i);
  });
});
