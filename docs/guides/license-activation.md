# Activating your Pulse license

> **Pulse v0.5.0 and later need no license key.** Every feature is included on every
> install, for any use including commercial (see [licensing-public.md](../licensing-public.md)).
> A key you still load is verified and reported by `GET /api/v1/admin/license`, but it
> changes nothing: no feature is gated and no limit applies, whether the key is valid,
> expired or absent. This guide is kept for installs that carry a legacy key and for a
> possible future paid model.

This guide is for a customer who has received a `PULSE_LICENSE_KEY` value and
wants to load it on a running Pulse deployment.

If you are the vendor minting new keys, see `docs/licensing.md` instead.

## Before you start

You need:

- Your `PULSE_LICENSE_KEY` string (delivered by the vendor).
- The Pulse deployment URL and an admin bearer token
  (created via **Settings → API Tokens** or `POST /api/v1/admin/tokens`).
- The vendor's public key is the embedded default since v0.4.1, so no
  `PULSE_LICENSE_PUBKEY` configuration is needed unless you self-sign
  (set the env var only to override the embedded key).

## Three activation routes

### Route A — Environment variable at boot (recommended for containers)

Set the key before the container or process starts:

```sh
# docker-compose or .env file
PULSE_LICENSE_KEY=<your-key>
```

The server reads this at startup via `license.New` (`server/cmd/pulse/serve.go`).
Restart the container/process after setting the variable. Confirm with a GET
request (see §"Confirming activation" below).

### Route B — Offline key file at boot

Write the key string to a file on the host (no newline at the end is fine —
the server trims whitespace):

```sh
printf '%s' "$PULSE_LICENSE_KEY" > /etc/pulse/license.key
chmod 600 /etc/pulse/license.key
```

Then set:

```sh
PULSE_LICENSE_FILE=/etc/pulse/license.key
```

Restart the container/process after setting the variable.

> **`PULSE_LICENSE_FILE` is the variable the binary reads.**
> `PULSE_LICENSE_OFFLINE_FILE` appears in an unreachable config code path and
> has no effect at runtime. Use `PULSE_LICENSE_FILE` only.

### Route C — Runtime API activation (no restart required)

Send a `PUT` request while the server is running. The new tier takes effect
immediately without a restart:

```sh
curl -s -X PUT https://<pulse-host>/api/v1/admin/license \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"key":"<PULSE_LICENSE_KEY value>"}'
```

A successful response looks like:

```json
{
  "tier": "pro",
  "valid": true,
  "expires_at": 1815569297073,
  "offline_file": false,
  "limits": {
    "max_nodes": 10,
    "max_streams": null,
    "retention_days": 90,
    "data_api": true,
    "white_label": false
  }
}
```

`expires_at` is a Unix epoch millisecond integer (`null` for perpetual
licenses). `null` limits mean unlimited.

**Error codes:**

| HTTP | Code | Meaning |
|---|---|---|
| 400 | `INVALID_JSON` | `key` field missing or empty. |
| 422 | `INVALID_LICENSE` | The key is malformed, or its signature did not verify — usually because it was issued for a deployment with a different `PULSE_LICENSE_PUBKEY`. Contact your vendor. |
| 401 | — | Bad or missing admin bearer token. |

> **An expired key returns `200`, not an error.** Pulse verifies the signature and
> accepts the key, then applies the expiry — so you get **`200` with
> `"valid": false` and `"tier": "free"`**. Do not read the status code as success.
> **The activation worked only if the body says `"valid": true` and shows the tier
> you expect.** If you see `"valid": false`, your key has expired: ask your vendor
> for a replacement.

## Confirming activation

After any of the three routes:

```sh
curl -s https://<pulse-host>/api/v1/admin/license \
  -H "Authorization: Bearer <admin-token>"
```

Check that `tier` matches what you purchased and `valid` is `true`.

A free-tier response (no valid key loaded) looks like:

```json
{
  "tier": "free",
  "valid": true,
  "expires_at": null,
  "offline_file": false,
  "limits": {
    "max_nodes": 1,
    "max_streams": null,
    "retention_days": 7,
    "data_api": false,
    "white_label": false
  }
}
```

If you sent a key and still see `"tier": "free"`, the key did not verify.
Check that `PULSE_LICENSE_PUBKEY` is set to the vendor's public key on the
deployment and retry.

## What happens when the license expires

**Since v0.5.0, nothing changes for your users.** When `expires_at` passes, the
license endpoint reports the key as no longer valid, and every feature stays enabled
with no limits — the all-features-free policy does not depend on the key. (Before
v0.5.0, an expired key dropped the install to the Free tier's limits and paid-tier
endpoints returned `403 LICENSE_REQUIRED`.)

## Reading the tier in the UI

Navigate to **Settings → License** (top-right menu). Since v0.5.0 the tab says
Pulse is free and shows every limit as "Unlimited". The UI
reads `GET /api/v1/admin/license` on page load — the same endpoint you
called above.
