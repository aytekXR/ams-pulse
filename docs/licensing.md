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

> **Post-GA work order (S9+):** extend `qa/licensegen` with `-privkey` /
> `-expires` flags so customer keys are minted from the persistent vendor key
> instead of an ephemeral one. Until then, mint with a small Go snippet that
> signs the claims JSON with the stored private key (same 10 lines as
> licensegen's `run()`, minus the key generation).

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
