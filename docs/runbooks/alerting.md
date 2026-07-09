# Pulse — Alerting Runbook

**PRD ref:** F5 (core alerting)  
**Budget:** alert detection-to-notification < 30 s (QA-verified: 15 s)  
**Last updated:** V3b fix-loop (2026-06-15) — muted suppression, group_by grouping, node_down absence detection, cron range syntax all verified and shipped.

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
| `group_by` | string | Optional. When set (e.g. `"app"`, `"stream_id"`), collapses multiple matching streams/nodes into a single notification per unique group key value. Without `group_by`, each stream fires independently. See [Storm protection](#storm-protection). |
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
| `rebuffer_ratio` | ClickHouse `rollup_qoe_1h` | QoE rebuffer ratio from beacon-fed hourly rollup (D-062). Requires beacon ingest data (Pro+ license, U3). Rule is skipped with a WARN log when the QoE reader is not configured or ClickHouse returns an error. A value of 0.0 means no buffering events in the window (evaluated normally against the threshold). |
| `error_rate` | ClickHouse `rollup_qoe_1h` | QoE error rate from beacon-fed hourly rollup (D-062). Same beacon/license/skip semantics as `rebuffer_ratio`. |
| `ingest_bitrate_floor` | Ingest health tracker | Fires when ingest health score indicates bitrate < 50% of target (`S_bitrate < 0.5`) |
| `node_down` | Fleet discovery | Fires when a cluster node is absent from the live snapshot (not seen within `3 × PollInterval`). Use a scope `node_id` to target a specific node, or leave scope empty to monitor all nodes. |
| `node_degraded` | Fleet discovery | Fires when a cluster node transitions to `status=degraded` |
| `cert_expiry` | TLS dial to host | Fires when TLS certificate days_left < threshold (e.g. `threshold: 30`) |

> **QoE metrics (`rebuffer_ratio`, `error_rate`) — what you need:**
> These rules read the `rollup_qoe_1h` ClickHouse aggregate table via the QoE reader
> (D-062). For data to appear there, player-side beacons must be active — which requires
> a Pro+ license (U3). Without a configured QoE reader or when ClickHouse is unreachable,
> the evaluator skips every stream for those rules and emits one WARN log per tick
> (`alert: qoe_reader not configured — rebuffer_ratio/error_rate rules skipped this tick (D-062: G6)`).
> The legacy HealthScore proxy (`(1−HealthScore)×0.1` / `×0.05`) was removed in D-062.

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
offline), Pulse uses `group_by` to collapse notifications:

- **Without `group_by`:** Each distinct stream/node fires its own notification. Use
  when you need per-stream visibility (e.g. PagerDuty incident per stream).
- **With `group_by: "app"`:** All firing streams in the same AMS app produce
  exactly 1 notification per app. N streams in app `live` → 1 notification, not N.
- **With `group_by: "stream_id"`:** One notification per unique stream (same as the
  default behavior; useful when you want explicit grouping semantics documented in the rule).

Set `group_by` when a single node failure could trigger hundreds of `stream_offline`
alerts and you want one actionable alert, not a storm.

```json
{
  "name": "App stream offline (grouped)",
  "metric": "stream_offline",
  "operator": "eq",
  "threshold": 0,
  "window_s": 30,
  "group_by": "app",
  "channel_ids": ["ch-slack-01"]
}
```

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
  "config": {
    "email_to": "ops@example.com",
    "smtp_addr": "smtp.example.com:587",
    "from": "alerts@example.com",
    "username": "alerts@example.com",
    "password": "your-smtp-password",
    "starttls": true
  }
}
```

**Implementation details:**
- STARTTLS is **disabled by default** (`starttls: false`); add `"starttls": true` to enable it. TLS errors are non-fatal against local SMTP servers.
- `password` is stored in the channel config (public portion — not encrypted). Avoid using
  shared SMTP accounts; prefer a dedicated alerts credential with send-only permissions.
- Free tier: email is the only supported channel type.

**Email config keys** (source of truth: `server/internal/alert/factory.go`):

| Key | Required | Default | Notes |
|---|---|---|---|
| `email_to` | Yes | — | Recipient address |
| `smtp_addr` | No | `localhost:587` | `host:port` |
| `from` | No | `pulse-alerts@localhost` | Sender address |
| `username` | No | — | SMTP AUTH username |
| `password` | No | — | SMTP AUTH password |
| `starttls` | No | `false` | Enable STARTTLS (bool) |

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
  "config": {
    "slack_webhook_url": "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX",
    "slack_channel": "#alerts"
  }
}
```

**Slack config keys** (source of truth: `server/internal/alert/factory.go`):

| Key | Required | Notes |
|---|---|---|
| `slack_webhook_url` | Yes | Incoming webhook URL (stored encrypted) |
| `slack_channel` | No | Channel name for display only (not used to route the message) |

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
    "telegram_chat_id": "-100123456789"
  }
}
```

`telegram_bot_token` is the Bot API token from @BotFather. `telegram_chat_id` is the
group or channel ID (negative number for groups/channels, positive for DMs).
The bot must be added to the group/channel before it can send messages.

**Telegram config keys** (source of truth: `server/internal/alert/factory.go`):

| Key | Required | Notes |
|---|---|---|
| `telegram_bot_token` | Yes | Bot API token (stored encrypted) |
| `telegram_chat_id` | Yes | Group/channel ID; negative for groups/channels, positive for DMs |

Pulse sends HTML-formatted messages via the Bot API `sendMessage` method.

### PagerDuty

Supported on Business tier and above.

**Via API:**
```json
{
  "type": "pagerduty",
  "name": "On-call PagerDuty",
  "config": {
    "pagerduty_routing_key": "<SENSITIVE — stored encrypted>",
    "pagerduty_severity": "critical"
  }
}
```

`pagerduty_routing_key` is the Events API v2 integration key from your PagerDuty
service. Pulse sends `event_action=trigger` when an alert fires and
`event_action=resolve` when it clears. The `dedup_key` is set to the Pulse
alert ID for reliable trigger/resolve pairing.

**PagerDuty config keys** (source of truth: `server/internal/alert/factory.go`):

| Key | Required | Notes |
|---|---|---|
| `pagerduty_routing_key` | Yes | Events API v2 integration key (stored encrypted) |
| `pagerduty_severity` | No | Override severity string sent to PagerDuty (e.g. `critical`, `error`, `warning`, `info`) |

### Webhook (generic HTTP + HMAC)

Supported on Business tier and above.

**Via API:**
```json
{
  "type": "webhook",
  "name": "SIEM integration",
  "config": {
    "webhook_url": "https://siem.example.com/pulse/alerts",
    "webhook_secret": "<SENSITIVE — stored encrypted>"
  }
}
```

**Webhook config keys** (source of truth: `server/internal/alert/factory.go`):

| Key | Required | Notes |
|---|---|---|
| `webhook_url` | Yes | Target URL for POST requests |
| `webhook_secret` | No | HMAC-SHA256 signing secret; when set, adds `X-Pulse-Signature` header (stored encrypted) |

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

## Delivery reliability

### Retry policy

Each channel send is retried up to **3 times** after the initial attempt (4 total) with
exponential backoff and ±20% jitter:

| Setting | Value |
|---|---|
| Initial attempt | 1 |
| Retries after failure | 3 (`RetryMaxAttempts`) |
| Total attempts | 4 |
| Base delay | 500 ms |
| Backoff cap | 5 s |
| Backoff formula | `min(500ms × 2^(n−1), 5s) × jitter` where `jitter ∈ [0.8, 1.2)` |

A `delivery_failure` row is written to alert history **only when all 4 attempts are
exhausted**. A single send failure followed by a successful retry produces no history
row. Retries are aborted cleanly on evaluator shutdown.

Source of truth: `server/internal/alert/evaluator.go` (`retryDeliver`, `RetryMaxAttempts=3`,
`RetryBaseDelay=500ms`, `RetryCap=5s`).

### Rule and channel changes take effect within one tick

The evaluator rebuilds the channel registry from the meta store at the **start of every
tick** (sync-on-tick, D-061). Creating, updating, or deleting a rule or channel via the
API or UI takes effect within the next tick interval (default **5 s**). No process restart
is needed.

Source of truth: `server/internal/alert/evaluator.go:evaluate()` → `syncRegistryFromStore()`.

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

History is retained up to **1000 rows per rule** in the meta store. After every insert,
`CreateAlertHistory` automatically prunes older rows for the same `rule_id`, keeping the
newest 1000 (D-054, `AlertHistoryDefaultKeep=1000`, `meta.go:45`). A calendar-based TTL
(e.g. delete rows older than N days) remains a Phase-3 roadmap item, but unbounded growth
is not possible.

---

## Known issues and limitations

| Issue | Severity | Status |
|---|---|---|
| Alert history has no calendar TTL — oldest rows beyond the per-rule cap of 1000 are deleted immediately on insert, but no time-based expiry is implemented | Minor | Phase-3 roadmap |
| `node_down` alert requires `scope.node_id` to target a specific node; without scope it checks node absence globally from the snapshot | Minor | By design — use scope to target individual nodes |
| `node_degraded` alert metric is a placeholder — `status=degraded` transition is not yet implemented in the aggregator | Minor | Phase-3 roadmap |
| `rebuffer_ratio` and `error_rate` rules require a configured ClickHouse QoE reader wired at startup (D-062). When the reader is **not configured** (`qoeReader == nil` — ClickHouse absent or `PULSE_CLICKHOUSE_DSN` unset), all such rules are **skipped for that tick** and at most one WARN is emitted: `alert: qoe_reader not configured — rebuffer_ratio/error_rate rules skipped this tick (D-062: G6)`. When the reader **errors** on a per-stream call, only the affected stream is skipped (at most one WARN per tick: `alert: qoe_reader error — stream skipped for this tick`). When `rollup_qoe_1h` simply has **no data** (e.g. no beacon ingest yet), `QoEForStream` returns `(0, 0, nil)` and the rule **evaluates normally against 0.0** — a `gt` threshold greater than 0 will never fire, but evaluation is not skipped and no WARN is logged. | Info | By design — see [Supported metrics](#supported-metrics) |
