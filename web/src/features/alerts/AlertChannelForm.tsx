import { useState, useRef } from "react";
import type { AlertChannel, AlertChannelConfig, AlertChannelWrite } from "@/lib/api/types";

interface Props {
  initial?: AlertChannel;
  onSave: (data: AlertChannelWrite) => Promise<void>;
  onCancel: () => void;
}

type ChannelType = "email" | "slack" | "webhook" | "pagerduty" | "telegram";

const CHANNEL_TYPES: ChannelType[] = ["email", "slack", "webhook", "pagerduty", "telegram"];
const PD_SEVERITIES = ["critical", "error", "warning", "info"] as const;

// Config fields in display order — the first invalid one takes focus. An error renders
// as `ch-<key>-error`, a hint as `ch-<key>-hint`.
const FIELD_ORDER = [
  "email", "smtp", "from", "smtpUser", "smtpPassword",
  "slack", "slackChannel",
  "webhook", "webhookSecret",
  "telegramToken", "telegramChat",
  "pagerduty",
] as const;
type FieldKey = (typeof FIELD_ORDER)[number];
type Errors = Partial<Record<FieldKey | "name", string>>;

/** An http(s) URL with a host — what the server accepts for webhook and Slack URLs. */
function isHttpUrl(v: string): boolean {
  try {
    const u = new URL(v);
    return (u.protocol === "http:" || u.protocol === "https:") && u.host !== "";
  } catch {
    return false;
  }
}

/** host:port; an IPv6 host goes in brackets ([::1]:25). */
const HOST_PORT = /^(\[[^\]\s]+\]|[^\s:[\]]+):\d{1,5}$/;
/** A plain address, as the server requires: it goes to the SMTP envelope (MAIL FROM / RCPT TO),
 *  where a display name ("Ops <ops@example.com>") is a syntax error. */
const LOOKS_LIKE_ADDRESS = /^[^\s@<>]+@[^\s@<>]+$/;

export function AlertChannelForm({ initial, onSave, onCancel }: Props) {
  const [name, setName] = useState(initial?.name ?? "");
  const [type, setType] = useState<ChannelType>((initial?.type ?? "email") as ChannelType);
  // Non-secret config comes back in config_summary and pre-fills its field.
  const summary = initial?.config_summary ?? {};
  const stored = (key: string) => (typeof summary[key] === "string" ? (summary[key] as string) : "");
  const [emailTo, setEmailTo] = useState(stored("email_to"));
  const [smtpAddr, setSmtpAddr] = useState(stored("smtp_addr"));
  const [from, setFrom] = useState(stored("from"));
  const [starttls, setStarttls] = useState(summary.starttls === true);
  const [slackChannel, setSlackChannel] = useState(stored("slack_channel"));
  const [webhookUrl, setWebhookUrl] = useState(stored("webhook_url"));
  const [telegramChatId, setTelegramChatId] = useState(stored("telegram_chat_id"));
  const [pdSeverity, setPdSeverity] = useState(stored("pagerduty_severity"));
  // Secrets are never returned, so their fields start empty.
  const [smtpUser, setSmtpUser] = useState("");
  const [smtpPassword, setSmtpPassword] = useState("");
  const [slackUrl, setSlackUrl] = useState("");
  const [webhookSecret, setWebhookSecret] = useState("");
  const [telegramToken, setTelegramToken] = useState("");
  const [pdKey, setPdKey] = useState("");
  const [saving, setSaving] = useState(false);
  const [errors, setErrors] = useState<Errors>({});

  // Refs for auto-focus on first invalid field after submit failure.
  const nameRef = useRef<HTMLInputElement>(null);
  const inputRefs = useRef<Partial<Record<FieldKey, HTMLInputElement | null>>>({});

  // On an edit that keeps the type, the server keeps every config key the request leaves
  // out — so a blank secret field keeps the stored secret. On create, or after a type
  // change (the server then starts from an empty config), required secrets must be typed.
  // Before S126 this form sent only the fields it showed and the server replaced the whole
  // config: saving an edit erased the signing secret and the SMTP settings.
  const keepsSecrets = !!initial && initial.type === type && initial.credential_set === true;
  const typeChanged = !!initial && initial.type !== type;
  const secretPlaceholder = (createPlaceholder: string) =>
    keepsSecrets ? "Stored — leave blank to keep" : createPlaceholder;

  // Returns the error map and calls setErrors; mirrors the server's checks so a refusal
  // shows next to its field instead of as a toast.
  const validate = (): Errors => {
    const errs: Errors = {};
    if (!name.trim()) errs.name = "Name is required";
    const missingSecret = (v: string) => !keepsSecrets && !v.trim();
    switch (type) {
      case "email":
        if (!emailTo.trim()) errs.email = "Email address required";
        else if (!LOOKS_LIKE_ADDRESS.test(emailTo.trim())) errs.email = "Enter a plain address, e.g. ops@example.com";
        if (smtpAddr.trim() && !HOST_PORT.test(smtpAddr.trim()))
          errs.smtp = "Use host:port, e.g. smtp.example.com:587";
        if (from.trim() && !LOOKS_LIKE_ADDRESS.test(from.trim())) errs.from = "Enter a plain address, e.g. ops@example.com";
        break;
      case "slack":
        if (missingSecret(slackUrl)) errs.slack = "URL required";
        else if (slackUrl.trim() && !isHttpUrl(slackUrl.trim())) errs.slack = "Enter an http(s) URL";
        break;
      case "webhook":
        if (!webhookUrl.trim()) errs.webhook = "URL required";
        else if (!isHttpUrl(webhookUrl.trim())) errs.webhook = "Enter an http(s) URL";
        break;
      case "telegram":
        if (missingSecret(telegramToken)) errs.telegramToken = "Bot token required";
        if (!telegramChatId.trim()) errs.telegramChat = "Chat ID required";
        break;
      case "pagerduty":
        if (missingSecret(pdKey)) errs.pagerduty = "Routing key required";
        break;
    }
    setErrors(errs);
    return errs;
  };

  // Non-secret fields are sent as shown — an emptied field removes the stored value. A
  // secret is sent only when typed; left out, the stored one is kept.
  const buildConfig = (): AlertChannelConfig => {
    const cfg: AlertChannelConfig = {};
    switch (type) {
      case "email":
        cfg.email_to = emailTo.trim();
        cfg.smtp_addr = smtpAddr.trim();
        cfg.from = from.trim();
        cfg.starttls = starttls;
        if (smtpUser.trim()) cfg.username = smtpUser.trim();
        if (smtpPassword) cfg.password = smtpPassword;
        break;
      case "slack":
        cfg.slack_channel = slackChannel.trim();
        if (slackUrl.trim()) cfg.slack_webhook_url = slackUrl.trim();
        break;
      case "webhook":
        cfg.webhook_url = webhookUrl.trim();
        if (webhookSecret) cfg.webhook_secret = webhookSecret;
        break;
      case "telegram":
        cfg.telegram_chat_id = telegramChatId.trim();
        if (telegramToken.trim()) cfg.telegram_bot_token = telegramToken.trim();
        break;
      case "pagerduty":
        cfg.pagerduty_severity = pdSeverity;
        if (pdKey.trim()) cfg.pagerduty_routing_key = pdKey.trim();
        break;
    }
    return cfg;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const errs = validate();
    if (Object.keys(errs).length > 0) {
      // Auto-focus the first invalid field so keyboard/AT users land on it.
      if (errs.name) nameRef.current?.focus();
      else {
        const first = FIELD_ORDER.find((k) => errs[k]);
        if (first) inputRefs.current[first]?.focus();
      }
      return;
    }
    setSaving(true);
    try {
      await onSave({ name: name.trim(), type, config: buildConfig() });
    } finally {
      setSaving(false);
    }
  };

  // s111 D4-pattern: outline:"none" removed — inputs carry
  // className="filter-input" so the shared :focus-visible ring applies.
  const inputStyle: React.CSSProperties = {
    background: "var(--color-surface-2)",
    border: "1px solid var(--color-border)",
    borderRadius: "var(--radius-control)",
    padding: "7px 10px",
    color: "var(--color-text)",
    fontSize: 13,
    width: "100%",
    boxSizing: "border-box",
  };

  const labelStyle: React.CSSProperties = {
    fontSize: 12,
    fontWeight: 500,
    color: "var(--color-secondary)",
    display: "flex",
    flexDirection: "column",
    gap: "var(--space-1)",
  };

  const hintStyle: React.CSSProperties = { fontSize: 11, fontWeight: 400, color: "var(--color-secondary)" };

  const twoColumns: React.CSSProperties = { display: "grid", gridTemplateColumns: "1fr 1fr", gap: "var(--space-3)" };

  // One config input: label, optional hint and inline error (the error span is the live
  // region). The hint sits outside the <label> so it is read once, as the description.
  const field = (
    key: FieldKey,
    label: string,
    value: string,
    onChange: (v: string) => void,
    opts: { placeholder?: string; type?: "text" | "url" | "email" | "password"; autoComplete?: string; hint?: string } = {},
  ) => {
    const described = [opts.hint ? `ch-${key}-hint` : "", errors[key] ? `ch-${key}-error` : ""].filter(Boolean).join(" ");
    return (
      <div style={labelStyle}>
        <label htmlFor={`ch-${key}`}>{label}</label>
        <input
          id={`ch-${key}`}
          ref={(el) => { inputRefs.current[key] = el; }}
          type={opts.type ?? "text"}
          autoComplete={opts.autoComplete}
          className="filter-input" style={{ ...inputStyle, borderColor: errors[key] ? "var(--color-error)" : "var(--color-border)" }}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={opts.placeholder}
          aria-invalid={errors[key] ? true : undefined}
          aria-describedby={described || undefined}
        />
        {opts.hint && <span id={`ch-${key}-hint`} style={hintStyle}>{opts.hint}</span>}
        {errors[key] && (
          <span id={`ch-${key}-error`} role="alert" style={{ fontSize: 11, color: "var(--color-error)" }}>
            {errors[key]}
          </span>
        )}
      </div>
    );
  };

  return (
    // noValidate: the inline messages below are the validation UI; the browser's own
    // bubbles for type="url"/"email" would pre-empt them with a different wording.
    <form noValidate onSubmit={(e) => void handleSubmit(e)} style={{ display: "flex", flexDirection: "column", gap: 14 }}>
      {/* Each inline field error IS its own live region (role="alert"). An earlier draft
          also mirrored every message into a separate sr-only aria-live div, duplicating the
          text in the DOM and making screen readers announce each error twice. */}

      <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>{initial ? "Edit channel" : "New notification channel"}</h3>

      <label style={labelStyle}>
        Channel name *
        <input
          ref={nameRef}
          className="filter-input" style={{ ...inputStyle, borderColor: errors.name ? "var(--color-error)" : "var(--color-border)" }}
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g. Ops team Slack"
          aria-invalid={errors.name ? true : undefined}
          aria-describedby={errors.name ? "ch-name-error" : undefined}
        />
        {errors.name && (
          <span id="ch-name-error" role="alert" style={{ fontSize: 11, color: "var(--color-error)" }}>
            {errors.name}
          </span>
        )}
      </label>

      <div style={labelStyle}>
        <label htmlFor="ch-type">Type</label>
        <select
          id="ch-type"
          className="filter-input" style={inputStyle}
          value={type}
          onChange={(e) => setType(e.target.value as ChannelType)}
          aria-describedby={typeChanged ? "ch-type-hint" : undefined}
        >
          {CHANNEL_TYPES.map((t) => <option key={t} value={t}>{t}</option>)}
        </select>
        {typeChanged && (
          <span id="ch-type-hint" style={hintStyle}>
            A new type starts from empty settings: enter all of them, secrets included.
          </span>
        )}
      </div>

      {type === "email" && (
        <>
          {field("email", "To address *", emailTo, setEmailTo, { type: "email", placeholder: "alerts@example.com" })}
          <div style={twoColumns}>
            {field("smtp", "SMTP server", smtpAddr, setSmtpAddr, {
              placeholder: "smtp.example.com:587",
              hint: "host:port. Blank means localhost:587 — inside a container, the container itself.",
            })}
            {field("from", "From address", from, setFrom, { type: "email", placeholder: "pulse-alerts@localhost" })}
          </div>
          <div style={twoColumns}>
            {field("smtpUser", "SMTP user", smtpUser, setSmtpUser, {
              autoComplete: "off",
              placeholder: secretPlaceholder("Only if the server needs AUTH"),
            })}
            {field("smtpPassword", "SMTP password", smtpPassword, setSmtpPassword, {
              type: "password",
              autoComplete: "new-password",
              placeholder: secretPlaceholder(""),
            })}
          </div>
          <label style={{ display: "flex", alignItems: "center", gap: "var(--space-2)", fontSize: 13, cursor: "pointer" }}>
            <input
              type="checkbox"
              checked={starttls}
              onChange={(e) => setStarttls(e.target.checked)}
              style={{ width: 14, height: 14, accentColor: "var(--color-accent)" }}
            />
            STARTTLS (if the upgrade fails, the message is not sent in plain text)
          </label>
        </>
      )}

      {type === "slack" && (
        <>
          {field("slack", keepsSecrets ? "Slack webhook URL" : "Slack webhook URL *", slackUrl, setSlackUrl, {
            type: "url",
            placeholder: secretPlaceholder("https://hooks.slack.com/services/…"),
          })}
          {field("slackChannel", "Slack channel (optional)", slackChannel, setSlackChannel, {
            placeholder: "#ops",
            hint: "Shown here for reference — the webhook URL decides where messages go.",
          })}
        </>
      )}

      {type === "webhook" && (
        <>
          {field("webhook", "Webhook URL *", webhookUrl, setWebhookUrl, {
            type: "url",
            placeholder: "https://example.com/pulse-alerts",
          })}
          {field("webhookSecret", "Signing secret (optional)", webhookSecret, setWebhookSecret, {
            type: "password",
            autoComplete: "new-password",
            placeholder: secretPlaceholder(""),
            hint: "Signs each request: X-Pulse-Signature: sha256=<HMAC of the body>.",
          })}
        </>
      )}

      {type === "telegram" && (
        <div style={twoColumns}>
          {field("telegramToken", keepsSecrets ? "Bot token" : "Bot token *", telegramToken, setTelegramToken, {
            type: "password",
            autoComplete: "new-password",
            placeholder: secretPlaceholder("From @BotFather"),
          })}
          {field("telegramChat", "Chat ID *", telegramChatId, setTelegramChatId, {
            placeholder: "-100123456789",
            hint: "Negative for groups and channels.",
          })}
        </div>
      )}

      {type === "pagerduty" && (
        <div style={twoColumns}>
          {field("pagerduty", keepsSecrets ? "Routing key" : "Routing key *", pdKey, setPdKey, {
            type: "password",
            autoComplete: "new-password",
            placeholder: secretPlaceholder("Events API v2 integration key"),
          })}
          <div style={labelStyle}>
            <label htmlFor="ch-pdSeverity">Severity sent to PagerDuty</label>
            <select
              id="ch-pdSeverity"
              className="filter-input" style={inputStyle}
              value={pdSeverity}
              onChange={(e) => setPdSeverity(e.target.value)}
            >
              <option value="">From the rule</option>
              {PD_SEVERITIES.map((s) => <option key={s} value={s}>{s}</option>)}
            </select>
          </div>
        </div>
      )}

      <div style={{ display: "flex", gap: 10, justifyContent: "flex-end", paddingTop: "var(--space-1)" }}>
        <button
          type="button"
          onClick={onCancel}
          className="btn-secondary"
          style={{
            background: "var(--color-surface-2)",
            borderRadius: "var(--radius-control)",
            padding: "var(--space-2) var(--space-4)",
            cursor: "pointer",
            fontSize: 13,
          }}
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={saving}
          className="btn-primary"
          style={{
            border: "none",
            color: "var(--color-on-signal)",
            borderRadius: "var(--radius-control)",
            padding: "8px 20px",
            fontSize: 13,
            fontWeight: 600,
          }}
        >
          {saving ? "Saving…" : "Save channel"}
        </button>
      </div>
    </form>
  );
}
