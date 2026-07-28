# livecheck

Live-server contract validation for PulseKit.

## What this is

A SwiftPM executable that points PulseKit at a **real Pulse server** and calls every
GET endpoint the API client exposes. It prints PASS or FAIL for each endpoint and
exits non-zero on any failure.

## What this proves

The one thing the unit tests cannot prove: that our decoders match a server that
actually exists.

PulseKit's unit tests decode from static fixtures. Those fixtures are correct
snapshots of the contract, but they cannot drift. A live server can:

- **Older servers** may send `null` where the current contract says `[]` (this is
  what started this investigation — a server built 2026-07-13 returned
  `{"items":null}` on an empty streams list, while the fix for that landed
  2026-07-18).
- **Newer servers** may send an enum value this build has never heard of. A Swift
  enum decoded from an unknown raw value **throws**, so one new `publisher_state`
  or alert severity added server-side breaks the streams list on every
  already-shipped app.

Pulse is self-hosted. Operators run whatever server version they last upgraded to.
An iOS app on the App Store cannot be shipped in lockstep with every server it
talks to. This tool surfaces those mismatches before they reach production.

## What this is NOT

- **Not part of CI.** CI has no Pulse server to point at. This is a manual check
  run against a dev, staging, or production deployment.
- **Not a load test.** It makes one request per endpoint, sequentially.
- **Not a mutation test.** It only calls GET endpoints. It never creates, updates,
  or deletes anything.

## Usage

```bash
cd ios/livecheck

# Set credentials via environment variables.
# NEVER pass the token as a command-line argument — arguments land in shell
# history and process lists.
export PULSE_URL="https://pulse.example.com"
export PULSE_TOKEN="plt_abc123..."

# Run the checks.
swift run livecheck
```

Example output:

```
livecheck: PulseKit live-server contract validation
=========================================================

Server: https://pulse.example.com

PASS  GET /healthz
PASS  GET /auth/me
PASS  GET /api/v1/live/overview
PASS  GET /api/v1/live/streams
PASS  GET /api/v1/alerts/history
PASS  GET /api/v1/fleet/nodes
PASS  GET /api/v1/anomalies
PASS  GET /api/v1/qoe/summary
PASS  GET /api/v1/admin/license

=========================================================
All checks passed.
```

## Endpoints covered

| Endpoint                   | Method | Auth required |
|----------------------------|--------|---------------|
| `/healthz`                 | GET    | No            |
| `/auth/me`                 | GET    | Yes           |
| `/api/v1/live/overview`    | GET    | Yes           |
| `/api/v1/live/streams`     | GET    | Yes           |
| `/api/v1/alerts/history`   | GET    | Yes           |
| `/api/v1/fleet/nodes`      | GET    | Yes           |
| `/api/v1/anomalies`        | GET    | Yes           |
| `/api/v1/qoe/summary`      | GET    | Yes           |
| `/api/v1/admin/license`    | GET    | Yes           |

## When to run this

- Before cutting a new PulseKit or PulseApp release.
- After making changes to any `Decodable` model in PulseKit.
- When debugging a wire-format mismatch (the error message includes the decoding
  context, e.g., "null value at 'items': expected Array<Any>").
- Against the oldest server version you intend to support, to verify backward
  compatibility.
