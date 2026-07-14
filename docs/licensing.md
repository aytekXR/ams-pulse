# Pulse licensing — repo license, and how product license keys are minted & distributed

Last updated: 2026-07-09 (D-066).

## 1. Repository license (what users may do with the code)

- **Server, web UI, deploy tooling: PolyForm Noncommercial 1.0.0** (root `LICENSE`,
  chosen D-066). Anyone may use, modify, and share Pulse **for noncommercial
  purposes**; commercial use requires a separate license from the copyright
  holder. This matches the tier model: the vendor sells commercial rights
  (dual licensing) while the code stays source-visible.
- **Beacon SDK: MIT** (`sdk/beacon-js/LICENSE`, unchanged). The SDK is embedded
  in customers' players, so it stays permissive on purpose.

## 2. Product license keys (what unlocks paid tiers at runtime)

A Pulse license key is a signed claims blob, not a lookup against a server:

```
PULSE_LICENSE_KEY = base64(claimsJSON) + "." + base64(ed25519_signature)
```

Claims fields (`server/internal/license/license.go`, `claims` struct):
`tier` (free|pro|business|enterprise), `max_nodes`, `max_streams`,
`retention_days` (0 or omitted ⇒ unlimited), `data_api`, `white_label`,
`expires_at` (Unix epoch **ms**, optional — omitted ⇒ perpetual).

Verification: `ed25519.Verify` against, in order:
1. `PULSE_LICENSE_PUBKEY` (hex, 32 bytes) if set in the environment, else
2. the **embedded dev public key** (`license.go` `devPublicKeyHex`) — which the
   code itself marks as *not authorizing production use*.

A key that fails to parse/verify/expire **fails open to Free tier** (the server
still runs; paid gates return 403 `LICENSE_REQUIRED`).

### 2.1 One-time vendor key ceremony (operator, offline)

Generate a persistent ed25519 keypair and guard the private key (password
manager / offline medium — it IS the business):

```sh
# Any machine with Go; nothing is persisted by the tool itself.
cd qa/licensegen
go run . -tier enterprise 2>/dev/null   # prints KEY + PUBKEY lines (ephemeral demo)
```

`qa/licensegen` currently generates a **fresh keypair per run** (built for CI).
For production minting, run it once, keep BOTH lines: the `PULSE_LICENSE_PUBKEY`
becomes your vendor public key; but because the tool discards the private key,
the durable ceremony is:

```sh
# Recommended: generate and keep the private key yourself (Go one-liner):
go run - <<'EOF'
package main
import ("crypto/ed25519";"encoding/hex";"fmt")
func main(){pub,priv,_ := ed25519.GenerateKey(nil)
fmt.Println("PRIVATE (keep offline):", hex.EncodeToString(priv))
fmt.Println("PUBLIC  (deploy everywhere):", hex.EncodeToString(pub))}
EOF
```

### 2.2 Minting a customer key

Sign the claims JSON with the vendor private key; emit
`base64(claims).base64(sig)`. Set `expires_at` for subscriptions (epoch ms);
omit it for perpetual licenses. Tier semantics are enforced server-side from
the `tier` string + explicit limits — see `buildEntitlements` (license.go).

### 2.3 Distributing & activating

Every production deployment must carry the vendor public key:
`PULSE_LICENSE_PUBKEY=<hex>` in `deploy/.env` (compose) or the Helm secret.
(Alternative for official builds: replace `devPublicKeyHex` at build time.)

The customer then activates by any of:
1. `PULSE_LICENSE_KEY=<key>` in the environment (picked up at boot),
2. an offline file (`PULSE_LICENSE_FILE` — path to a file containing the key
   string, read at startup via `license.New`),
3. at runtime: `PUT /api/v1/admin/license {"key":"<key>"}` (admin bearer token;
   takes effect immediately, `GET /api/v1/admin/license` confirms tier/expiry).

### 2.4 Security properties & caveats

- Keys are offline-verifiable; no phone-home. Revocation = expiry (`expires_at`)
  — there is no revocation list, so prefer 1-year expiries for subscriptions.
- The embedded dev pubkey means a **default build without
  `PULSE_LICENSE_PUBKEY` only accepts keys signed by the dev keypair** — do not
  distribute the dev private key; treat any key minted against it as CI-only
  (`e2e.yml` mints ephemeral ones per run).
- Rotating the vendor keypair invalidates all outstanding keys — plan for
  overlap (accept old+new) only via a code change (post-GA backlog if needed).

## 3. Vendor key ceremony (production minting)

> **The keypair ceremony is already complete for this deployment.**
> The vendor private key has been generated and vaulted offline; the matching
> `PULSE_LICENSE_PUBKEY` is deployed in production; a minted enterprise license
> is running in production. **Do not repeat §3a–3c** — regenerating the keypair
> would immediately invalidate the live production license and every key already
> delivered to customers (see §3f). These steps exist for a future keypair
> rotation or a net-new vendor deployment, not for day-to-day operation.

### 3a. Generate the production keypair — OFFLINE

Run this one-liner on an air-gapped or dedicated operator machine. It prints
the 128-hex private key (64 bytes: seed||pub) and the 64-hex public key (32
bytes). Nothing is written to disk by the tool; you pipe or copy the output
yourself.

```sh
go run - <<'EOF'
package main
import ("crypto/ed25519";"encoding/hex";"fmt")
func main() {
    pub, priv, _ := ed25519.GenerateKey(nil)
    fmt.Println("PRIVATE (128 hex, keep offline):", hex.EncodeToString(priv))
    fmt.Println("PUBLIC  (64 hex, deploy everywhere):", hex.EncodeToString(pub))
}
EOF
```

### 3b. Store the private key

Store the 128-hex private key string in an **encrypted vault or offline
medium** (hardware token, encrypted USB, password manager secret). It must
never appear in:

- any git repository (source or config),
- any CI secret or environment variable,
- any server deploy config or container image.

A leaked private key lets anyone forge perpetual enterprise licenses with no
way to revoke them short of rolling the public key.

### 3c. Deploy the public key

Copy the 64-hex PUBLIC key to every Pulse deployment:

```sh
# deploy/.env  (docker-compose) or the Helm secret
PULSE_LICENSE_PUBKEY=<64-hex-public-key>
```

The server reads this at startup and uses it as the sole verification key
(overriding the embedded dev key).

### 3d. Mint a customer key

On the operator machine (where the private key file is accessible), run:

```sh
cd qa/licensegen
go run . -tier pro -privkey /secure/path/vendor.priv -expires 365
```

Flags (all optional except `‑privkey` for production):

| Flag | Argument | Effect |
|---|---|---|
| `-tier` | `free\|pro\|business\|enterprise` | Tier to encode in the claims (default: `pro`). |
| `-privkey` | path to file | Reads the 128-hex ed25519 private key from that file and derives the matching public key. Omit in CI to generate an ephemeral keypair per run. |
| `-expires` | positive integer (days) | Sets `expires_at` to `now + N days` (Unix epoch ms). Omit for a perpetual license. Mutually exclusive with `-expires-minutes`. |
| `-expires-minutes` | positive integer (minutes) | Sets `expires_at` to `now + N minutes`. For live trial-flow demos. Mutually exclusive with `-expires`. |

Stdout is exactly two `KEY=value` lines:

```
PULSE_LICENSE_KEY=<base64(claims)>.<base64(sig)>
PULSE_LICENSE_PUBKEY=<64-hex-public-key>
```

Append `>> license.env` or copy `PULSE_LICENSE_KEY` to the customer's delivery
channel. The private key is read only at mint time and never transmitted.

### 3e. Verify activation

Have the customer (or test yourself with `curl`):

```sh
# Activate (PUT, not POST; path is /api/v1/admin/license)
curl -s -X PUT https://<pulse-host>/api/v1/admin/license \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"key":"<PULSE_LICENSE_KEY value>"}'

# Confirm
curl -s https://<pulse-host>/api/v1/admin/license \
  -H "Authorization: Bearer <admin-token>"
```

A successful response (HTTP 200) looks like:

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

`expires_at` is a **Unix epoch millisecond integer** (not an ISO-8601 string),
or `null` for perpetual licenses. `null` limits mean unlimited. A `valid: true`
response with the correct `tier` confirms the key was accepted by the deployed
public key.

Error responses:
- **400** `INVALID_JSON` — `key` field missing or empty body.
- **422** `INVALID_LICENSE` — the key is malformed, or its signature did not
  verify against the deployed `PULSE_LICENSE_PUBKEY`.

> **An EXPIRED key does not return 422 — it returns `200`.** `activate()` verifies
> the signature and deliberately does *not* reject a signed-but-expired key; expiry
> is applied lazily on the first read, so the response comes back **`200` with
> `"valid": false` and `"tier": "free"`**. The status line alone will tell you the
> activation "succeeded." **Check the body, not the status code**: a key has only
> truly taken effect when `valid` is `true` *and* `tier` is what you sold. Anything
> that pattern-matches on 422 to detect an expired key will miss it silently.

### 3f. Key rotation

There is no certificate revocation list (CRL). Existing keys remain valid
until their `expires_at`. To rotate the vendor keypair:

> **Ordering hazard — read before touching `PULSE_LICENSE_PUBKEY`.**
> `PULSE_LICENSE_PUBKEY` is read exactly once, at `Manager.New()` (i.e. server
> startup). Rolling the env var and restarting a node **immediately** downgrades
> every outstanding customer key to Free tier on that node — `ed25519.Verify`
> will fail for all keys signed by the old private key. Do not restart any node
> with the new public key until you have re-minted **all** active customer
> licenses with the new private key and delivered the new keys to customers.
> Plan for a coordinated cutover window.

1. Generate a new keypair (step 3a).
2. Re-mint all active customer licenses with the new private key and deliver the
   new `PULSE_LICENSE_KEY` values to each customer.
3. Only after customers have confirmed receipt, roll `PULSE_LICENSE_PUBKEY` to
   the new public key on all Pulse nodes and restart them.
4. Securely delete or archive the old private key.

Because old keys expire naturally, prefer short expiries (1 year) for
subscriptions so rotation windows stay manageable.
