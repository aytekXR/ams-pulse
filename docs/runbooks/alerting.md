# Pulse — Alerting Runbook

**PRD ref:** F5 (core alerting)  
**Budget:** alert detection-to-notification < 30 s (QA-verified: 15 s)

---

## Overview

Pulse evaluates alert rules on a tick loop (default 5 s) against the live
in-memory aggregates from the collector. When a rule condition is met for its
configured window, Pulse fires a notification to every configured channel.
When the condition clears, it sends a resolved notification.

---

## Rule semantics

### State machine

```
pending ──(condition met for window_s)──► firing ──(condition clears)──► resolved
   ▲                                         │
   └──────────── cooldown expires ◄──────────┘  (allows re-fire if condition returns)
```

- **pending**: condition is currently true but the window has not elapsed yet.
- **firing**: condition was true for the full `window_s`; notification sent.
- **resolved**: condition is no longer true; resolved notification sent.

A rule in **firing** state that is still true after `cooldown_s` will re-fire.
A rule suppressed by a maintenance window or `muted: true` produces no notifications. A rule with `enabled: false` is not evaluated at all.

### Rule fields

| Field | Type | Description |
|---|---|---|
| `name` | string | **Required.** Human-readable label for this rule (e.g. "CPU high on node-1"). Displayed in the alerts list and notification payloads. |
| `metric` | string | What to evaluate. See [supported metrics](#supported-metrics). |
| `operator` | string | Comparison: `gt`, `lt`, `gte`, `lte`, `eq` |
| `threshold` | number | The value to compare against |
| `window_s` | integer | Seconds the condition must be true before firing (default 60) |
| `cooldown_s` | integer | Seconds between repeat firings for the same alert (default 300) |
| `severity` | string | `info`, `warning`, `critical` |
| `enabled` | boolean | **Default true.** When `false`, the rule is completely skipped — not evaluated, no history written. Use to pause a rule without deleting it. |
| `muted` | boolean | When `true`, the rule is evaluated and history is written, but no notifications are dispatched. Use for maintenance periods where you want to keep the evaluation record. |
| `scope` | object | Optional filter: `node_id`, `app`, `stream_id` |
| `channel_ids` | array | IDs of alert channels to notify |

#### `enabled` vs `muted` — distinct semantics

| State | Evaluated? | History written? | Notifications sent? |
|---|---|---|---|
| `enabled: true, muted: false` | Yes | Yes | Yes |
| `enabled: true, muted: true` | Yes | Yes | No |
| `enabled: false` | **No** | **No** | No |

- **`enabled: false`** — the rule is completely paused. The evaluator skips it before any metric fetch. No evaluation cost, no history.
- **`muted: true`** — the rule keeps running and firing internally (history is preserved), but notifications are suppressed. Useful for planned maintenance where you want a record of what would have fired.

A disabled rule's `muted` state is not surfaced — it has no effect until the rule is re-enabled.

### Supported metrics

| Metric name | Evaluated against | Notes |
|---|---|---|
| `stream_offline` | Live aggregator | Fires when a stream_id is no longer active. No threshold needed (`operator: eq`, `threshold: 0`). |
| `viewer_drop_pct` | Live aggregator | Current viewer count per stream vs threshold |
| `viewer_count` | Live aggregator | Absolute viewer count per stream |
| `ingest_bitrate_kbps` | Live aggregator | Ingested bitrate for active streams |
| `fps` | Live aggregator | Current frames-per-second |
| `node_cpu` | Live aggregator | CPU % per node (0–100). AMS returns 0–100 directly; Pulse passes it through unchanged. |
| `node_mem` | Live aggregator | Memory % per node |
| `node_disk` | Live aggregator | Disk % per node |

> **Roadmap (Wave 2):** ClickHouse-backed historical metrics (e.g. rebuffer rate,
> startup latency) will be added as rule metrics in Wave 2.

### Scope filtering

A rule without a scope evaluates against all streams/nodes and produces one
notification per distinct stream/node that meets the condition (subject to
`group_by` grouping). Use scope to narrow a rule to a specific stream, app, or node:

```json
{
  "metric": "stream_offline",
  "operator": "eq",
  "threshold": 0,
  "window_s": 30,
  "scope": { "stream_id": "live/main-stage" }
}
```

### Storm protection

When many streams match a rule simultaneously (e.g. a node failure takes 50 streams
offline), Pulse fires one notification per group key. The group key defaults to
`stream_id` for stream-scoped rules and `node_id` for node-scoped rules.
There is no cap on concurrent alerts — each distinct group key fires independently.

> **Roadmap (Wave 2):** The `group_by` field will allow collapsing multiple matching
> streams into a single grouped notification.

### Latency

The detection-to-notification path:

```
AMS REST poll (≤5 s) → aggregator update → evaluator tick (≤5 s) → channel.Send (~0 ms)
```

Worst case: 10.1 s. QA-verified at **15 s** (conservative: `window_s=10` must elapse
before firing, plus one tick confirmation). Budget: 30 s (PRD F5).

---

## Channel setup

Channels are created via the UI (Settings → Alerts → Channels) or the API
(`POST /api/v1/alerts/channels`). Each channel type has a different config shape.

### Email (SMTP)

Supported on all tiers.

**Via UI:** Settings → Alerts → Channels → New channel → type: email.

**Via API:**
```json
{
  "type": "email",
  "name": "Ops team",
  "email_to": "ops@example.com",
  "smtp_addr": "smtp.example.com:587",
  "smtp_user": "alerts@example.com",
  "smtp_password_env_ref": "SMTP_PASSWORD"
}
```

**Implementation details:**
- Uses STARTTLS on port 587 by default. TLS errors are non-fatal against local SMTP servers.
- `smtp_password_env_ref` stores the env var name — Pulse resolves it from the process
  environment at send time, never storing the password in the meta store in plaintext.
- Free tier: email is the only supported channel type.

**SMTP config reference:**

| Setting | Default | Notes |
|---|---|---|
| `smtp_addr` | `localhost:587` | `host:port` |
| `smtp_user` | — | Optional; required for services that require SMTP AUTH |
| STARTTLS | enabled | Disabled automatically if the server does not support it |

### Slack (incoming webhook)

Supported on Pro tier and above.

> Note: Slack channel type is implemented in Wave 1 (code: `channels.SlackChannel`).
> Tier enforcement (Pro+) is a Wave 2 feature; in Wave 1 the channel type is available
> but the license check is not yet wired.

**Via UI:** Settings → Alerts → Channels → New channel → type: slack.

**Via API:**
```json
{
  "type": "slack",
  "name": "Slack #alerts",
  "slack_webhook_url": "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX",
  "slack_channel": "#alerts"
}
```

Pulse formats notifications as Slack messages with state emoji, metric name, value,
threshold, scope, and a dashboard deep-link (if `PULSE_BASE_URL` is set).

**Slack message format:**

```
:red_circle: *FIRING: stream live/main-stage / stream_offline eq 0* [CRITICAL]
Metric: `stream_offline` | Value: `0` | Threshold: `0`
Scope: `stream_id=live/main-stage`
<https://pulse.example.com/alerts|Open Dashboard>
```

### Other channels (roadmap)

The following channels are architected (interface defined) but not implemented in Wave 1:

| Channel | Wave | Notes |
|---|---|---|
| Webhook (generic HTTP) | Wave 2 | POST the `alert-notification` JSON payload to any URL |
| PagerDuty | Wave 2 | Events API v2 |
| Telegram | Wave 2 | Bot API |

---

## Test-fire

Every configured channel has a test-fire button in the UI (Settings → Alerts →
Channels → channel row → Test). This sends a synthetic `test: true` notification
to verify delivery before live alerts fire.

**Via API:**
```sh
curl -X POST http://localhost:8090/api/v1/alerts/channels/{channel_id}/test \
  -H "Authorization: Bearer plt_<your_admin_token>"
```

Response:
```json
{ "accepted": true, "message": "test notification queued" }
```

A `test: true` flag is set in the notification payload so recipients can identify
test messages. Email subject prefixed `[Pulse TEST]`; Slack message prefixed `:test_tube:`.

---

## Cooldown and grouping

### Cooldown

`cooldown_s` prevents notification spam when a transient condition re-triggers.
After a rule fires, the same rule+group will not fire again until `cooldown_s` has elapsed.

Example: `cooldown_s: 300` (5 minutes) means a CPU alert for node-1 will fire once,
then be suppressed for 5 minutes even if the condition remains true.

The resolved notification is **not** affected by cooldown — it fires immediately when
the condition clears regardless of where the cooldown timer stands.

### Maintenance windows

> **Roadmap (Wave 2):** Full cron-expression maintenance windows.

Wave 1 supports two manual controls on rules:

- `muted: true` — rule evaluates normally and history is recorded, but no notifications are dispatched. Useful for planned maintenance where you want to keep the evaluation record.
- `enabled: false` — rule is completely paused (not evaluated at all). Use to stop a rule temporarily without deleting it.

See [enabled vs muted semantics](#enabled-vs-muted--distinct-semantics) in the Rule fields section for the full comparison.

**Mute a rule via API:**
```sh
curl -X PUT http://localhost:8090/api/v1/alerts/rules/{rule_id} \
  -H "Authorization: Bearer plt_<token>" \
  -H "Content-Type: application/json" \
  -d '{"muted": true}'
```

**Disable a rule via API:**
```sh
curl -X PUT http://localhost:8090/api/v1/alerts/rules/{rule_id} \
  -H "Authorization: Bearer plt_<token>" \
  -H "Content-Type: application/json" \
  -d '{"enabled": false}'
```

---

## Default rule pack

> **Roadmap (Wave 2):** A default rule pack will be seeded automatically on first run.
> See BE-02 gap G8.

In Wave 1, rules must be created manually. Recommended starting rules:

**Stream offline (critical):**
```json
{
  "name": "Stream offline",
  "metric": "stream_offline",
  "operator": "eq",
  "threshold": 0,
  "window_s": 30,
  "cooldown_s": 300,
  "severity": "critical",
  "enabled": true,
  "scope": {}
}
```

**Node CPU high (warning):**
```json
{
  "name": "Node CPU high",
  "metric": "node_cpu",
  "operator": "gt",
  "threshold": 80,
  "window_s": 60,
  "cooldown_s": 600,
  "severity": "warning",
  "enabled": true,
  "scope": {}
}
```

**Viewer count low (info):**
```json
{
  "name": "Viewer count low",
  "metric": "viewer_count",
  "operator": "lt",
  "threshold": 1,
  "window_s": 60,
  "cooldown_s": 300,
  "severity": "info",
  "enabled": true,
  "scope": {}
}
```

---

## Alert history

All fired and resolved notifications are persisted in the meta store
(`alert_history` table) and surfaced via:

- **UI:** Alerts → History tab (paginated, filterable by rule/severity/state)
- **API:** `GET /api/v1/alerts/history`

History entries include: `alert_id`, `rule_id`, `state`, `severity`, `ts`, `metric`,
`value`, `threshold`, `scope`, `group_key`, `cooldown_until`.

History is retained indefinitely in the meta store. No TTL in Wave 1.

---

## Known issues and limitations

| Issue | Severity | Status |
|---|---|---|
| Maintenance windows not implemented — only `muted` and `enabled` toggles available (BE-02 G2) | Minor | Wave 2 |
| Telegram, PagerDuty, generic webhook channels not implemented (BE-02 G1) | Minor | Wave 2 |
| Default rule pack not seeded on bootstrap (BE-02 G8) | Minor | Wave 2 |
| Prometheus `/metrics` endpoint is stub — only 2 metrics exported (BE-02 G4) | Minor | Wave 2 |
