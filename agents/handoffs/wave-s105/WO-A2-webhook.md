# WO-A2 (S105) — Webhook parser: accept the official AMS payload

**Review finding (Issue 2, P1, verified locally + against AMS source `AntMediaApplicationAdapter.java`):**
official AMS webhook payloads carry the stream id in **`id`** (fields: `id`, `action`,
`streamName`, `category`, `metadata`, `timestamp`, `app`; `vodReady` adds `vodName`, `vodId`,
`duration`). Default content type is JSON; **form-urlencoded is an opt-in AMS setting** and Pulse
currently 200-and-drops such bodies. Pulse's `translateWebhook` reads only `streamId` (never
`id`), and the `vodReady` branch reads `vodSize` but not `vodId`/`duration` (the publish-end
branch DOES already read `duration` — leave it). AMS retries on any non-200
(`webhookRetryCount`/`webhookRetryDelay`) — the existing "return 200 on parse failure" behavior
is deliberate and must be preserved.

## Scope — you may ONLY edit these files
- `server/internal/collector/webhook/webhook.go`
- `server/internal/collector/webhook/webhook_test.go`
- `server/internal/collector/webhook/webhook_more_test.go`
- `server/internal/collector/webhook/webhook_persource_test.go` (only if needed)

Do NOT touch docs (a parallel agent owns them), do NOT run git commands.
Other agents are editing other files in this tree concurrently — expected.

## Work items

1. **`id` as stream-id fallback.** In `translateWebhook` (~line 269, `streamID :=
   jsonString(raw["streamId"])`): when empty, fall back to `raw["id"]`. Comment: official AMS
   webhook field is `id`; `streamId` kept first for existing proxy deployments.

2. **vodReady completeness.** In the vodReady branch (~line 305): also read `vodId` (string) and
   `duration` (numeric, AMS sends ms) into the event data (e.g. `vod_id`, `duration_ms` — match
   the existing key naming convention in that map). Keep `vodSize` as-is.

3. **Accept `application/x-www-form-urlencoded`.** When the request Content-Type is
   form-urlencoded, parse the body with `url.ParseQuery` into the same `map[string]any`
   (values arrive as strings). Make the numeric helpers (`jsonInt`, `jsonInt64`, and any float
   helper used by translateWebhook) tolerant of numeric *strings* so form values normalize
   identically. IMPORTANT: HMAC signature verification runs over the RAW body — verify the raw
   bytes exactly as today, before any parsing; do not change the signing contract.
   Keep the 200-on-parse-failure behavior (anti-retry-storm, documented in code).

4. **Tests with verbatim official payloads:**
   - JSON `liveStreamStarted` using `id` (no `streamId`) + `action`, `streamName`, `category`,
     `timestamp` → event has the correct stream id.
   - JSON `vodReady` with `vodName`, `vodId`, `app`, `duration` → data carries vod_id + duration.
   - form-urlencoded `liveStreamStarted` with a correct HMAC over the raw form body → accepted,
     correct stream id and timestamp.
   - Legacy `streamId` JSON payload still works (regression).
   - Wrong/missing HMAC on a form body still 401s (fail-closed unchanged).

## Definition of done
- `gofmt -l` clean on touched files (CI hard-fails otherwise).
- `cd server && go build ./... && go test ./internal/collector/webhook/...` green.
- Return: files changed, test results, exact data-map keys you emitted for vodReady, any deviation.
