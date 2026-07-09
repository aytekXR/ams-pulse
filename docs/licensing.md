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
2. an offline file (`PULSE_LICENSE_FILE` path — file contains the key string),
3. at runtime: `POST /api/v1/license/activate {"key":"<key>"}` (admin token;
   takes effect immediately, `GET /api/v1/license` confirms tier/expiry).

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

- `-privkey` reads the 128-hex key from the file, signs with it, and prints
  the matching public key as `PULSE_LICENSE_PUBKEY` (confirm it matches 3c).
- `-expires 365` sets `expires_at` to now+365 days (Unix epoch ms). For
  perpetual licenses, omit `-expires`.
- Stdout is exactly two `KEY=value` lines; append `>> license.env` or copy
  `PULSE_LICENSE_KEY` to the customer's delivery channel.

The private key file is read at mint time on the operator machine and is never
transmitted to or stored on any server.

### 3e. Verify activation

Have the customer (or test yourself with `curl`):

```sh
# Activate
curl -s -X POST https://<pulse-host>/api/v1/license/activate \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"key":"<PULSE_LICENSE_KEY value>"}'

# Confirm
curl -s https://<pulse-host>/api/v1/license \
  -H "Authorization: Bearer <admin-token>"
# → {"tier":"pro","valid":true,"expires_at":"2027-07-09T..."}
```

A `valid: true` response with the correct `tier` and `expires_at` confirms the
key was accepted by the deployed public key.

### 3f. Key rotation

There is no certificate revocation list (CRL). Existing keys remain valid
until their `expires_at`. To rotate the vendor keypair:

1. Generate a new keypair (step 3a).
2. Roll `PULSE_LICENSE_PUBKEY` to the new public key on **all** Pulse nodes
   before re-minting any keys (a node still holding the old public key will
   reject new keys signed by the new private key).
3. Re-mint all active customer licenses with the new private key.
4. Securely delete or archive the old private key.

Because old keys expire naturally, prefer short expiries (1 year) for
subscriptions so rotation windows stay manageable.
