# Pulse — Licensing Explained

**Last updated:** 2026-10-07

This page is the human-readable guide to Pulse licensing. It covers the
open-source licenses that govern the code, what is free, how license keys
work (they are optional), and the support model.

The *license texts themselves* always govern; summaries on this page are for
clarity and do not modify or replace any license. Where this page and a
license text conflict, the license text prevails.

---

## 1. Software licenses

Pulse is made up of two independently licensed components.

### 1.1 Server, web UI, and deploy tooling — PolyForm Shield License 1.0.0

The main Pulse codebase (everything in `server/`, `web/`, and `deploy/`) is
released under the **PolyForm Shield License 1.0.0**,
**Copyright (c) 2026 Aytekin Erdogan (beyondkaira.com)**. The full text is in
the root `LICENSE` file and at
<https://polyformproject.org/licenses/shield/1.0.0>.

**What the PolyForm Shield license allows:**

- Use, run, and operate Pulse for any purpose, including commercial use, free
  of charge.
- Self-host Pulse on your own infrastructure.
- Modify the source code and make derivative works.
- Share copies (modified or unmodified) with others, provided you pass along
  these license terms and any Required Notices.

**The one restriction:**

You may not use Pulse to provide a product that competes with it. Goods and
services compete even when they provide functionality through different
interfaces or for different platforms — applications compete with services,
libraries with plugins, and so on. If you market a product as a practical
substitute for Pulse, it competes.

> The license text itself governs all rights and restrictions. This summary is
> provided for readability only.

### 1.2 Beacon SDKs — MIT

The player QoE beacon SDKs (`sdk/beacon-js/` and `sdk/beacon-swift/`) are
released under the **MIT License** (see their respective `LICENSE` files).
You may embed the beacon in any player — including commercial products and
services — freely, with no restriction on commercial use.

---

## 2. Pricing — Pulse is free

Pulse is free: every feature, no license key, no node or retention limits.
This is the launch policy for at least the first year (from October 2026).

There are no paid plans, no subscriptions, and no purchase required. Every
feature that Pulse offers — live ops dashboard, historical analytics, QoE
beacon ingest, alerting with all channels, usage reports, Prometheus /metrics,
anomaly detection, synthetic probes, SSO/OIDC — is available to every user at
no cost.

---

## 3. License keys (optional, for compatibility)

### 3.1 Do I need a license key?

No. From v0.5.0, Pulse runs with an "all features free" policy. Every feature
is unlocked by default without any key.

### 3.2 Are license keys still accepted?

Yes, for compatibility with deployments that already have one. The server
accepts keys via:

1. **Environment variable** — `PULSE_LICENSE_KEY=<key>`
2. **Offline file** — `PULSE_LICENSE_FILE=<path>` to a file containing the key
3. **Runtime API** — `PUT /api/v1/admin/license {"key":"<key>"}`

A valid key is accepted and logged, but it does not change what features are
available — all features are already free.

### 3.3 Technical details

License keys are self-contained, offline-verifiable signed tokens (ed25519).
They do not require a connection to any external server. The API endpoint
`GET /api/v1/admin/license` reports the current state, including the field
`all_features_free: true` which confirms the server is running in all-features-
free mode.

The old tier model (Free/Pro/Business/Enterprise) remains in the codebase as
a dormant mechanism. It is not active and does not restrict any functionality.

---

## 4. Support

Support is best effort through GitHub issues and support@beyondkaira.com, with
no guaranteed response times.

| Channel | Where |
|---|---|
| GitHub Issues | [github.com/aytekXR/ams-pulse/issues](https://github.com/aytekXR/ams-pulse/issues) |
| Email | support@beyondkaira.com |
| Security vulnerabilities | aytek@beyondkaira.com (do not open a public issue) |

See `docs/support.md` for the bug-report guide and what to include.

---

## 5. Version history

Versions before v0.5.0 were released under the PolyForm Noncommercial 1.0.0
license, which restricted commercial use without a paid license. From v0.5.0
onwards, Pulse is licensed under PolyForm Shield 1.0.0 and all features are
free.

---

## 6. Frequently asked questions

### Can I use Pulse commercially?

Yes. Pulse is free for commercial use under the PolyForm Shield License 1.0.0.
The one restriction is that you may not use it to provide a product that
competes with Pulse.

### Does the beacon SDK have the same restriction?

No. The beacon SDKs (`sdk/beacon-js/` and `sdk/beacon-swift/`) are MIT-licensed
and may be embedded in commercial players and products freely, with no
restriction.

### Can I run Pulse in an air-gapped environment?

Yes. No license key is required, and no connection to any external server is
needed. Pulse is fully self-contained.

### Can I modify the source code and run my modified version commercially?

Yes, as long as you do not use the modified version to provide a product that
competes with Pulse.

### What if I have an old license key from before v0.5.0?

It will still be accepted, but it is no longer needed. All features are free
regardless of what tier the key encodes.

---

*Sources: `LICENSE` (PolyForm Shield 1.0.0), `sdk/beacon-js/LICENSE` (MIT),
`sdk/beacon-swift/LICENSE` (MIT), `server/internal/license/license.go`.*
