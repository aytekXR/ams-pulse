/**
 * AlertChannelForm rendering, interaction, and a11y tests.
 *
 * Covers:
 * (a) Smoke — renders without crash; "New notification channel" heading present.
 * (b) Default type — email fields shown initially; webhook URL field absent.
 * (c) Validation — empty name triggers "Name is required" error.
 * (d) Validation — empty email address triggers "Email address required" error.
 * (e) Type switch — selecting "slack" shows Slack webhook URL field.
 * (f) Type switch — "pagerduty" shows its own fields (there are no env vars for it).
 * (g) Cancel button calls onCancel prop.
 * (h) Edit mode — heading shows "Edit channel" when initial prop is provided.
 * (i) a11y — aria-invalid + aria-describedby wired correctly on error.
 * (j) a11y — aria-live region present and populated on error.
 * (k) S126 — every type has its real config fields; edits keep stored secrets.
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

  // The form used to say "Configure via environment variables" for PagerDuty and Telegram:
  // no such variables exist, and the channel was saved with an empty config.
  it("(f) type switch — pagerduty shows its routing key and severity, no env-var claim", async () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    const select = screen.getByRole("combobox");
    fireEvent.change(select, { target: { value: "pagerduty" } });
    await waitFor(() => {
      expect(screen.getByLabelText(/routing key \*/i)).toBeInTheDocument();
    });
    expect(screen.getByLabelText(/severity sent to pagerduty/i)).toBeInTheDocument();
    expect(screen.queryByText(/environment variables/i)).not.toBeInTheDocument();
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

// ── (k) S126: real config per type; an edit keeps what it does not show ─────

describe("AlertChannelForm — config per type (S126)", () => {
  const fill = (label: RegExp, value: string) =>
    fireEvent.change(screen.getByLabelText(label), { target: { value } });
  const setType = (t: string) => fireEvent.change(screen.getByLabelText(/^type$/i), { target: { value: t } });
  const save = () => fireEvent.click(screen.getByRole("button", { name: /save channel/i }));
  const savedConfig = async (onSave: ReturnType<typeof vi.fn>) => {
    await waitFor(() => expect(onSave).toHaveBeenCalledOnce());
    return (onSave.mock.calls[0][0] as { config: Record<string, unknown> }).config;
  };

  it("email: sends the SMTP server, sender, credentials and STARTTLS", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<AlertChannelForm onSave={onSave} onCancel={vi.fn()} />);
    fill(/channel name/i, "NOC mail");
    fill(/to address/i, "noc@example.com");
    fill(/smtp server/i, "smtp.example.com:587");
    fill(/from address/i, "pulse@example.com");
    fill(/smtp user/i, "pulse");
    fill(/smtp password/i, "pw with spaces ");
    fireEvent.click(screen.getByLabelText(/starttls/i));
    save();
    expect(await savedConfig(onSave)).toEqual({
      email_to: "noc@example.com",
      smtp_addr: "smtp.example.com:587",
      from: "pulse@example.com",
      starttls: true,
      username: "pulse",
      password: "pw with spaces ",
    });
  });

  it("telegram: needs the bot token and the chat ID, and sends both", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<AlertChannelForm onSave={onSave} onCancel={vi.fn()} />);
    fill(/channel name/i, "Ops Telegram");
    setType("telegram");
    save();
    expect(await screen.findByText("Bot token required")).toBeInTheDocument();
    expect(screen.getByText("Chat ID required")).toBeInTheDocument();
    expect(onSave).not.toHaveBeenCalled();

    fill(/bot token/i, "123:abc");
    fill(/chat id/i, "-100123");
    save();
    expect(await savedConfig(onSave)).toEqual({ telegram_bot_token: "123:abc", telegram_chat_id: "-100123" });
  });

  it("pagerduty: sends the routing key and the chosen severity", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<AlertChannelForm onSave={onSave} onCancel={vi.fn()} />);
    fill(/channel name/i, "On-call");
    setType("pagerduty");
    save();
    expect(await screen.findByText("Routing key required")).toBeInTheDocument();
    fill(/routing key/i, "R0UT1NG");
    fill(/severity sent to pagerduty/i, "critical");
    save();
    expect(await savedConfig(onSave)).toEqual({ pagerduty_routing_key: "R0UT1NG", pagerduty_severity: "critical" });
  });

  it("webhook: sends the URL and the signing secret", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<AlertChannelForm onSave={onSave} onCancel={vi.fn()} />);
    fill(/channel name/i, "Incidents");
    setType("webhook");
    fill(/webhook url/i, "https://example.com/hook");
    fill(/signing secret/i, "s3cret");
    save();
    expect(await savedConfig(onSave)).toEqual({ webhook_url: "https://example.com/hook", webhook_secret: "s3cret" });
  });

  it("edit: a blank secret is left out, so the server keeps the stored one", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const initial: AlertChannel = {
      id: "ch-1", name: "Incidents", type: "webhook", credential_set: true,
      config_summary: { webhook_url: "https://example.com/a" }, created_at: 1,
    };
    render(<AlertChannelForm initial={initial} onSave={onSave} onCancel={vi.fn()} />);
    expect(screen.getByLabelText(/webhook url/i)).toHaveValue("https://example.com/a");
    expect(screen.getByLabelText(/signing secret/i)).toHaveAttribute("placeholder", expect.stringMatching(/leave blank to keep/i));
    fill(/webhook url/i, "https://example.com/b");
    save();
    const config = await savedConfig(onSave);
    expect(config).toEqual({ webhook_url: "https://example.com/b" });
    expect(config).not.toHaveProperty("webhook_secret");
  });

  it("edit: a stored Slack URL is not required again, and is not overwritten", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const initial: AlertChannel = {
      id: "ch-2", name: "Ops Slack", type: "slack", credential_set: true,
      config_summary: { slack_channel: "#ops" }, created_at: 1,
    };
    render(<AlertChannelForm initial={initial} onSave={onSave} onCancel={vi.fn()} />);
    save();
    expect(await savedConfig(onSave)).toEqual({ slack_channel: "#ops" });
  });

  it("edit: SMTP settings are shown and sent back, so saving does not drop them", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const initial: AlertChannel = {
      id: "ch-3", name: "NOC mail", type: "email", credential_set: true,
      config_summary: { email_to: "noc@example.com", smtp_addr: "mail:1025", from: "p@example.com", starttls: true },
      created_at: 1,
    };
    render(<AlertChannelForm initial={initial} onSave={onSave} onCancel={vi.fn()} />);
    expect(screen.getByLabelText(/smtp server/i)).toHaveValue("mail:1025");
    expect(screen.getByLabelText(/starttls/i)).toBeChecked();
    save();
    expect(await savedConfig(onSave)).toEqual({
      email_to: "noc@example.com", smtp_addr: "mail:1025", from: "p@example.com", starttls: true,
    });
  });

  it("a secret is required on create", async () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    fill(/channel name/i, "Ops Slack");
    setType("slack");
    save();
    expect(await screen.findByText("URL required")).toBeInTheDocument();
    expect(screen.getByLabelText(/slack webhook url/i)).toHaveAttribute("aria-invalid", "true");
  });

  it("a secret is required again after a type change, and the form says why", async () => {
    const onSave = vi.fn();
    const initial: AlertChannel = {
      id: "ch-1", name: "Incidents", type: "webhook", credential_set: true,
      config_summary: { webhook_url: "https://example.com/a" }, created_at: 1,
    };
    render(<AlertChannelForm initial={initial} onSave={onSave} onCancel={vi.fn()} />);
    setType("pagerduty");
    expect(screen.getByText(/a new type starts from empty settings/i)).toBeInTheDocument();
    save();
    expect(await screen.findByText("Routing key required")).toBeInTheDocument();
    expect(onSave).not.toHaveBeenCalled();
  });

  it("refuses what the server would refuse, next to the field", async () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    fill(/channel name/i, "x");
    fill(/to address/i, "ops");
    fill(/smtp server/i, "smtp.example.com");
    save();
    expect(await screen.findByText(/enter a plain address/i)).toBeInTheDocument();
    expect(screen.getByText(/use host:port/i)).toBeInTheDocument();

    // A display name goes to the SMTP envelope as is and fails every delivery.
    fill(/to address/i, "Ops <ops@example.com>");
    fill(/smtp server/i, "smtp.example.com:587");
    save();
    expect(await screen.findByText(/enter a plain address/i)).toBeInTheDocument();

    setType("webhook");
    fill(/webhook url/i, "example.com/hook");
    save();
    expect(await screen.findByText("Enter an http(s) URL")).toBeInTheDocument();
  });

  it("focuses the first invalid config field", async () => {
    render(<AlertChannelForm onSave={vi.fn()} onCancel={vi.fn()} />);
    fill(/channel name/i, "x");
    fill(/to address/i, "ops@example.com");
    fill(/smtp server/i, "no-port");
    save();
    await waitFor(() => expect(screen.getByLabelText(/smtp server/i)).toHaveFocus());
  });
});
