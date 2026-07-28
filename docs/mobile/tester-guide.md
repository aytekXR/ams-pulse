# Pulse iOS Beta Tester Guide

Last updated: 2026-07-28

Welcome to the Pulse iOS beta. This guide explains how to install the app,
connect to your Pulse server, and send feedback.

---

## What you need

1. **An iPhone or iPad** running iOS 17.0 or later
2. **A TestFlight invitation** (email from Apple)
3. **A Pulse server URL** (e.g., `https://pulse.example.com`)
4. **An API token** (starts with `plt_`)

If you do not have a Pulse server or token, contact the person who invited you
to the beta.

---

## Installing the app

### Step 1: Install TestFlight

TestFlight is Apple's official beta testing app.

1. Open the App Store on your device.
2. Search for **TestFlight**.
3. Tap **Get** to install.

If you already have TestFlight, skip to step 2.

### Step 2: Accept the invitation

You received an email from Apple with the subject "You're invited to test Pulse."

1. Open the email on your iPhone or iPad.
2. Tap **View in TestFlight** or **Start Testing**.
3. TestFlight opens and shows Pulse.
4. Tap **Install** (or **Update** if reinstalling).

The app installs alongside your regular apps. It has an orange dot badge to
indicate it is a beta.

### Step 3: Open Pulse

Tap the Pulse icon to launch the app. You will see the connection screen.

---

## Connecting to your Pulse server

### What is a Pulse server?

Pulse is a self-hosted analytics platform for Ant Media Server. Each
organization runs their own Pulse server. The iOS app connects to your
organization's server to display monitoring data.

### What is an API token?

An API token authenticates your app with the Pulse server. It looks like:

```
plt_a1b2c3d4e5f6...
```

Tokens are created by your Pulse administrator.

### Getting a token

If you are the administrator:

1. Open your Pulse deployment in a web browser.
2. Sign in with your admin credentials.
3. Navigate to **Settings** > **API Tokens**.
4. Click **Create Token**.
5. Select **Kind:** `api` and **Scopes:** `read` (read-only access is sufficient).
6. Click **Create**.
7. Copy the token immediately. It is shown only once.

If you are not the administrator, ask your admin to create a read-only token
for you.

### Entering connection details

1. On the Pulse connection screen, enter the server URL (e.g., `https://pulse.example.com`).
2. Enter your API token in the token field.
3. Tap **Connect**.

The app validates the connection. If successful, you see the Live dashboard.

### Connection errors

| Error | Cause | Fix |
|-------|-------|-----|
| "Could not connect to server" | URL unreachable | Check the URL; ensure your device has internet; confirm the server is running |
| "Invalid API token" | Token is wrong or revoked | Verify the token; request a new one from your admin |
| "Could not establish a secure connection" | Self-signed certificate | Trust the certificate in iOS Settings > General > About > Certificate Trust |

---

## What the app shows

The app has three tabs at the bottom: **Live**, **Alerts**, and **Settings**.

### Live tab

The main dashboard displays:

- **Viewers** — total viewers currently watching streams
- **Publishers** — total publishers sending streams
- **Streams** — count of active streams
- **Protocol mix** — WebRTC, HLS, RTMP viewer counts

Below the overview tiles, **Active Streams** shows each stream with:

- Stream ID
- App name
- Viewer count
- Health score (circular indicator: green/yellow/red)
- Bitrate (if available)

Tap a stream to open the player view.

The dashboard auto-refreshes every 10 seconds while the app is in the foreground.
Pull down to refresh manually.

### Alerts tab

Shows recent alert history with:

- Severity icon (info/warning/critical)
- Metric name
- Breach details (value vs. threshold)
- When it fired (relative time)
- State badge (Firing/Resolved/Delivery Failed)
- Scope (node, app, stream, or tenant)

Pull down to refresh.

### Settings tab

Shows:

- **Server** — connected server URL and overall health status
- **Components** — ClickHouse, Meta Store, Collector, Kafka status and latency
- **App** — version and build number
- **Sign Out** — disconnects and clears stored credentials

---

## Known limitations in this beta

This is an early beta. The following limitations exist:

| Limitation | Status |
|------------|--------|
| No push notifications for alerts | Planned for future release |
| No offline mode | App requires active connection |
| No multi-server support | One server at a time |
| No historical charts | Shows live data only; use web UI for history |
| No write operations | Read-only; cannot create alerts or tokens from app |

Please do not report these as bugs. They are known scope limitations for the
beta.

---

## Sending feedback

### In-app feedback (recommended)

1. In TestFlight, open Pulse.
2. Take a screenshot (press Side + Volume Up).
3. TestFlight shows a feedback prompt.
4. Write your feedback and tap **Submit**.

The screenshot and device logs are automatically attached.

### Shake to report

1. While using Pulse, shake your device.
2. TestFlight shows a feedback dialog.
3. Describe what happened and tap **Submit**.

### Via email

If the in-app feedback does not work, email:

- **Feedback address:** (provided by your beta coordinator)

Include:

- What you were doing
- What you expected
- What actually happened
- Your iOS version (Settings > General > About)
- Your device model

---

## Updating the app

TestFlight automatically notifies you when a new build is available.

1. Open TestFlight.
2. Tap **Update** next to Pulse.

Beta builds expire after 90 days. If a build expires, TestFlight prompts you to
update or the app stops working.

---

## Removing the app

If you want to stop testing:

1. Long-press the Pulse icon on your home screen.
2. Tap **Remove App** > **Delete App**.

This does not delete your server data. It only removes the app from your device.

To rejoin the beta later, reinstall from TestFlight.

---

## Questions?

Contact your Pulse administrator or the person who invited you to this beta.
