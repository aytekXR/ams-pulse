# ADR 0004: AES-256-GCM encryption for secrets in the meta store

**Status:** Accepted · **Date:** 2026-06-12 (Wave 1)

## Context

Pulse stores AMS REST credentials (bearer tokens), SMTP passwords, and Slack
webhook URLs in the meta store. These are necessary to deliver alert notifications
and poll AMS without re-asking for credentials on each request. They must not be
stored in plaintext — a compromised database file must not expose live credentials.

The meta store backend in Wave 1 is SQLite (a file on disk). The encryption must
be CGO-free (D-001 build constraint) and must not require a key management service
(no external dependencies, self-hosted by definition).

## Decision

Encrypt all credential columns using **AES-256-GCM** with a random 12-byte nonce
per ciphertext, stored as `nonce || ciphertext` (base64-encoded) in the column.

Key derivation:
1. The operator sets `PULSE_SECRET_KEY` as a 32-byte hex string.
2. If `PULSE_SECRET_KEY` is absent, Pulse generates a random key on first start
   and stores it in `<db_dir>/pulse_secret.key`. This file must be protected with
   filesystem permissions (owned by the pulse process user, mode 0600).

The encryption surface in Wave 1:
- `ams_sources.credential_enc` — AMS bearer token
- `alert_channels.secret_enc` — SMTP password, Slack webhook URL, etc.

Plaintext secrets are never written to the meta store. The `meta.Store` API exposes
`Encrypt(plaintext) (ciphertext, error)` and `Decrypt(ciphertext) (plaintext, error)`.

## Rationale

- **AES-256-GCM** is the industry standard for authenticated encryption at rest —
  it provides both confidentiality and integrity (tampering is detectable).
- **Per-nonce randomness** prevents the same plaintext from producing the same
  ciphertext across rows, which would leak equality.
- **No external KMS dependency** keeps Pulse deployable on air-gapped networks with
  no internet access. An operator managing their own key is appropriate for
  self-hosted software.
- **Envelope encryption** (wrapping data keys with a root key) was considered but
  rejected: the added complexity is not warranted for a single-tenant, single-node
  deployment. Operators who require key rotation can re-encrypt with a new
  `PULSE_SECRET_KEY` using a future `pulse rekey` subcommand (Wave 3 roadmap).
- **CGO-free** requirement rules out libraries that depend on OpenSSL or libsodium.
  Go's `crypto/aes` + `crypto/cipher` (standard library) needs no CGO.

## Consequences

- Operators must back up the `pulse_secret.key` file alongside the database file.
  Losing the key means losing access to stored credentials (new credentials must
  be re-entered). This is documented in the install runbook.
- Token hashing for API bearer tokens uses SHA-256 in Wave 1 (not bcrypt, which
  needs a pure-Go dependency). bcrypt migration is planned for Wave 2 (BE-02 G3).
- The `PULSE_SECRET_KEY` env var must be set consistently across restarts and
  replicas. In Docker Compose, pass it as a secret or env var on the service.
