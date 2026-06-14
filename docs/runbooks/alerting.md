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
| `rebuffer_ratio` | Live health proxy | QoE rebuffer ratio (live snapshot proxy; Wave 3: full ClickHouse query) |
| `error_rate` | Live health proxy | QoE error rate (live snapshot proxy; Wave 3: full ClickHouse query) |
| `ingest_bitrate_floor` | Ingest health tracker | Fires when ingest health score indicates bitrate < 50% of target (`S_bitrate < 0.5`) |
| `node_down` | Fleet discovery | Fires when a cluster node transitions to `status=down` |
| `node_degraded` | Fleet discovery | Fires when a cluster node transitions to `status=degraded` |
| `cert_expiry` | TLS dial to host | Fires when TLS certificate days_left < threshold (e.g. `threshold: 30`) |

> **Phase-3 roadmap:** Full ClickHouse-backed QoE metrics (rebuffer rate, startup
> latency p50/p95) with time-window queries over `rollup_qoe_1h` are planned for
> Wave 3. The Wave-2 `rebuffer_ratio` and `error_rate` rule types use a live
> snapshot proxy.

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

> **Phase-3 roadmap:** The `group_by` field will allow collapsing multiple matching
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

> Note: Slack channel type is implemented since Wave 1 (code: `channels.SlackChannel`).
> Pro tier enforcement is implemented in Wave 2.

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

### Telegram

Supported on Pro tier and above.

**Via UI:** Settings → Alerts → Channels → New channel → type: telegram.

**Via API:**
```json
{
  "type": "telegram",
  "name": "Ops Telegram",
  "config": {
    "telegram_bot_token": "<SENSITIVE — stored encrypted>",
    "chat_id": "-100123456789"
  }
}
```

`telegram_bot_token` is the Bot API token from @BotFather. `chat_id` is the
group or channel ID (negative number for groups/channels, positive for DMs).
The bot must be added to the group/channel before it can send messages.

Pulse sends HTML-formatted messages via the Bot API `sendMessage` method.

### PagerDuty

Supported on Enterprise tier only.

**Via API:**
```json
{
  "type": "pagerduty",
  "name": "On-call PagerDuty",
  "config": {
    "pagerduty_routing_key": "<SENSITIVE — stored encrypted>",
    "severity": "critical"
  }
}
```

`pagerduty_routing_key` is the Events API v2 integration key from your PagerDuty
service. Pulse sends `event_action=trigger` when an alert fires and
`event_action=resolve` when it clears. The `dedup_key` is set to the Pulse
alert ID for reliable trigger/resolve pairing.

### Webhook (generic HTTP + HMAC)

Supported on Enterprise tier only.

**Via API:**
```json
{
  "type": "webhook",
  "name": "SIEM integration",
  "config": {
    "url": "https://siem.example.com/pulse/alerts",
    "webhook_secret": "<SENSITIVE — stored encrypted>",
    "headers": {"X-Source": "pulse"}
  }
}
```

Pulse POSTs the `alert-notification` JSON payload to the configured URL.
When `webhook_secret` is set, a signature header is added:

```
X-Pulse-Signature: sha256=<hex(HMAC-SHA256(secret, body))>
```

**Verifying the signature (consumer side):**
```python
import hmac, hashlib

def verify_pulse_webhook(body: bytes, secret: str, signature: str) -> bool:
    expected = 'sha256=' + hmac.new(
        secret.encode(), body, hashlib.sha256
    ).hexdigest()
    # Use constant-time comparison to prevent timing attacks:
    return hmac.compare_digest(expected, signature)
```

```go
// Go example (constant-time)
import "crypto/hmac"
import "crypto/sha256"
import "encoding/hex"

func verify(body []byte, secret, sig string) bool {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(body)
    expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(expected), []byte(sig))
}
```

Always use constant-time comparison (`hmac.compare_digest` / `hmac.Equal`) to
prevent timing attacks.

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

Wave 2 implements cron-expression maintenance windows via the rule `maintenance_window`
field.

**Cron format:** 3-field `MIN HOUR WEEKDAY` plus an optional duration:

```json
{
  "name": "Sunday maintenance window",
  "metric": "node_cpu",
  "operator": "gt",
  "threshold": 80,
  "maintenance_window": {
    "cron_expr": "0 2 0",
    "duration_s": 3600
  }
}
```

This rule is suppressed between 02:00–03:00 on Sundays (weekday=0).

During a maintenance window, rules are evaluated and history is written but
notifications are not dispatched (same semantics as `muted: true`).

You can also use the two manual controls on rules:

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

Pulse automatically seeds 4 default rules on first run (Wave 2). All default rules
are seeded with `enabled: true` and `muted: true` — they evaluate and record history
from day one, but send no notifications until you configure channels and unmute them.

| Rule | Metric | Condition | Severity |
|---|---|---|---|
| Stream offline | `stream_offline` | eq 0, window 30s | critical |
| Viewer drop | `viewer_drop_pct` | lt 20, window 60s | warning |
| Node CPU high | `node_cpu` | gt 80, window 60s | warning |
| Ingest bitrate floor | `ingest_bitrate_floor` | bitrate < 50% target, window 30s | critical |

To activate notifications: assign a channel to the rule and set `muted: false`.

Manually create additional rules as needed:

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

History is retained indefinitely in the meta store. History TTL is a Phase-3 roadmap item.

---

## Known issues and limitations

| Issue | Severity | Status |
|---|---|---|
| QoE metrics (`rebuffer_ratio`, `error_rate`) use live snapshot proxy, not historical ClickHouse query | Minor | Phase-3 roadmap |
| `group_by` field for alert grouping not implemented — each stream/node fires independently | Minor | Phase-3 roadmap |
| Alert history has no TTL — grows unbounded in the meta store | Minor | Phase-3 roadmap |
