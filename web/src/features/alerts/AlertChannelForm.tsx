import { useState, useRef } from "react";
import type { AlertChannel, AlertChannelWrite } from "@/lib/api/types";

interface Props {
  initial?: AlertChannel;
  onSave: (data: AlertChannelWrite) => Promise<void>;
  onCancel: () => void;
}

type ChannelType = "email" | "slack" | "webhook" | "pagerduty" | "telegram";

const CHANNEL_TYPES: ChannelType[] = ["email", "slack", "webhook", "pagerduty", "telegram"];

export function AlertChannelForm({ initial, onSave, onCancel }: Props) {
  const [name, setName] = useState(initial?.name ?? "");
  const [type, setType] = useState<ChannelType>((initial?.type ?? "email") as ChannelType);
  // Derive initial config values from config_summary (non-secret display fields)
  const summary = initial?.config_summary ?? {};
  const [emailTo, setEmailTo] = useState((summary.email_to as string) ?? "");
  const [webhookUrl, setWebhookUrl] = useState(
    (summary.slack_webhook_url as string) ?? (summary.webhook_url as string) ?? ""
  );
  const [saving, setSaving] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});

  // Refs for auto-focus on first invalid field after submit failure.
  const nameRef = useRef<HTMLInputElement>(null);
  const emailToRef = useRef<HTMLInputElement>(null);
  const webhookUrlRef = useRef<HTMLInputElement>(null);

  // Returns the error map and calls setErrors.
  const validate = (): Record<string, string> => {
    const errs: Record<string, string> = {};
    if (!name.trim()) errs.name = "Name is required";
    if (type === "email" && !emailTo.trim()) errs.emailTo = "Email address required";
    if ((type === "slack" || type === "webhook") && !webhookUrl.trim()) errs.webhookUrl = "URL required";
    setErrors(errs);
    return errs;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const errs = validate();
    if (Object.keys(errs).length > 0) {
      // Auto-focus the first invalid field so keyboard/AT users land on it.
      if (errs.name) nameRef.current?.focus();
      else if (errs.emailTo) emailToRef.current?.focus();
      else if (errs.webhookUrl) webhookUrlRef.current?.focus();
      return;
    }
    setSaving(true);
    try {
      // AlertChannelConfig discriminated union — build per selected type
      let config: AlertChannelWrite["config"] = {};
      if (type === "email") config = { email_to: emailTo.trim() };
      if (type === "slack") config = { slack_webhook_url: webhookUrl.trim() };
      if (type === "webhook") config = { webhook_url: webhookUrl.trim() };
      await onSave({ name: name.trim(), type, config });
    } finally {
      setSaving(false);
    }
  };

  const inputStyle: React.CSSProperties = {
    background: "var(--color-surface-2)",
    border: "1px solid var(--color-border)",
    borderRadius: 6,
    padding: "7px 10px",
    color: "var(--color-text)",
    fontSize: 13,
    width: "100%",
    outline: "none",
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

  return (
    <form onSubmit={(e) => void handleSubmit(e)} style={{ display: "flex", flexDirection: "column", gap: 14 }}>
      {/* Each inline field error IS its own live region (role="alert"). An earlier draft
          also mirrored every message into a separate sr-only aria-live div, duplicating the
          text in the DOM and making screen readers announce each error twice. */}

      <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>{initial ? "Edit channel" : "New notification channel"}</h3>

      <label style={labelStyle}>
        Channel name *
        <input
          ref={nameRef}
          style={{ ...inputStyle, borderColor: errors.name ? "var(--color-error)" : "var(--color-border)" }}
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

      <label style={labelStyle}>
        Type
        <select style={inputStyle} value={type} onChange={(e) => setType(e.target.value as ChannelType)}>
          {CHANNEL_TYPES.map((t) => <option key={t} value={t}>{t}</option>)}
        </select>
      </label>

      {type === "email" && (
        <label style={labelStyle}>
          To address *
          <input
            ref={emailToRef}
            type="email"
            style={{ ...inputStyle, borderColor: errors.emailTo ? "var(--color-error)" : "var(--color-border)" }}
            value={emailTo}
            onChange={(e) => setEmailTo(e.target.value)}
            placeholder="alerts@example.com"
            aria-invalid={errors.emailTo ? true : undefined}
            aria-describedby={errors.emailTo ? "ch-email-error" : undefined}
          />
          {errors.emailTo && (
            <span id="ch-email-error" role="alert" style={{ fontSize: 11, color: "var(--color-error)" }}>
              {errors.emailTo}
            </span>
          )}
        </label>
      )}

      {(type === "slack" || type === "webhook") && (
        <label style={labelStyle}>
          {type === "slack" ? "Slack webhook URL *" : "Webhook URL *"}
          <input
            ref={webhookUrlRef}
            type="url"
            style={{ ...inputStyle, borderColor: errors.webhookUrl ? "var(--color-error)" : "var(--color-border)" }}
            value={webhookUrl}
            onChange={(e) => setWebhookUrl(e.target.value)}
            placeholder="https://hooks.slack.com/services/…"
            aria-invalid={errors.webhookUrl ? true : undefined}
            aria-describedby={errors.webhookUrl ? "ch-webhook-error" : undefined}
          />
          {errors.webhookUrl && (
            <span id="ch-webhook-error" role="alert" style={{ fontSize: 11, color: "var(--color-error)" }}>
              {errors.webhookUrl}
            </span>
          )}
        </label>
      )}

      {(type === "pagerduty" || type === "telegram") && (
        <p style={{ margin: 0, fontSize: 12, color: "var(--color-secondary)" }}>
          Configure via environment variables — see documentation.
        </p>
      )}

      <div style={{ display: "flex", gap: 10, justifyContent: "flex-end", paddingTop: "var(--space-1)" }}>
        <button
          type="button"
          onClick={onCancel}
          style={{
            background: "var(--color-surface-2)",
            border: "1px solid var(--color-border)",
            color: "var(--color-secondary)",
            borderRadius: 6,
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
          style={{
            background: "var(--color-accent)",
            border: "none",
            color: "var(--color-on-signal)",
            borderRadius: 6,
            padding: "8px 20px",
            cursor: "pointer",
            fontSize: 13,
            fontWeight: 600,
            opacity: saving ? 0.7 : 1,
          }}
        >
          {saving ? "Saving…" : "Save channel"}
        </button>
      </div>
    </form>
  );
}
