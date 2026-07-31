# CodeQL alert triage record

**Last updated:** 2026-07-31

This is the standing record of how every CodeQL alert on this repository was
dispositioned, and why. It exists because "4 open alerts, no explanation" is a
worse signal to a reviewer than "4 alerts, each triaged with reasoning" — and
because a dismissal that is not written down is indistinguishable from a
dismissal that was never thought about.

**Method.** Each alert was triaged adversarially (D-191, 2026-07-31): one analyst
was tasked with building a concrete exploit and told the vendor would claim a
false positive; a second was tasked with arguing the alert should be dismissed,
from the code rather than from convenience; an independent judge then verified
the load-bearing claim of *both* before recommending a disposition. Where the
judge overruled a unanimous verdict, that is recorded below — it happened once,
and it changed the outcome.

The GitHub `dismissed_comment` field caps at 280 characters, so each dismissal
there points at this file.

---

## Open / mitigated

### #6 — `go/weak-sensitive-data-hashing` · `server/internal/store/meta/meta.go:1649`

**Disposition: WON'T FIX (now), mitigated. Severity: low, conditional.**

`meta.deriveKey` has two branches for a non-empty `PULSE_SECRET_KEY`:

| Input | Behaviour |
|---|---|
| 64 hex chars decoding to 32 bytes | used **directly** as the AES-256-GCM key — no hashing |
| anything else | `SHA-256(key)` becomes the AES-256-GCM key |

Every documented install path produces the first form:
`deploy/quickstart/install.sh:274` generates `openssl rand -hex 32`, and
`.env.example`'s placeholder is rejected at boot. On that path the alert does not
apply at all — there is no hash in the chain.

**The alert is nonetheless correct about the second branch, and both analysts
initially got this wrong.** They argued "the input is a high-entropy machine
secret, so the low-entropy-password threat model does not apply". That is true
only of the hex path. Nothing forces an operator onto it: a hand-set
`PULSE_SECRET_KEY=mysecretpassword1` clears the 16-byte floor, passes the
placeholder check, and lands on `SHA-256`. SHA-256 is fast, so if the meta
database is exfiltrated that key is recoverable by dictionary attack. **A floor
on length is not a floor on entropy.**

**Why the derivation was NOT changed.** Switching the non-hex branch to a slow
KDF changes the derived key, which makes every existing `credential_enc`,
`webhook_secret_enc` and `config_enc` blob undecryptable. There is no
`pulse rekey` command to re-encrypt under a new key — ADR-0004 defers it. A fix
that silently destroys an operator's stored AMS credentials is worse than the
weakness it closes. Rejecting non-hex keys outright has the same problem from the
other direction: it refuses to boot deployments whose data is encrypted under
exactly that derivation.

**What was done instead (D-191).** `config.ValidateSecretKey` now warns at
startup when the key is not canonical 64-char hex, naming the consequence and the
remedy. It is a warning, so every existing deployment keeps working. Verified at
the artifact level, not just in unit tests: the built binary emits the warning
with a passphrase key and stays silent with a hex key.

The same change collapsed four copy-pasted implementations of this validation
into one. Two of the four had never gained the `changeme` placeholder check, so a
placeholder key that `pulse serve` rejected was accepted by `pulse reconcile`.

**To close this properly:** ship `pulse rekey`, then move the non-hex branch to
Argon2id/HKDF behind a version-tagged blob format, with a documented migration.

---

## Dismissed — false positive

### #10 — `go/weak-sensitive-data-hashing` · `server/internal/api/oidc.go:124`

`hmacKey = SHA-256("oidc-state-v1:" + PULSE_SECRET_KEY)` derives an HMAC key for
the OIDC state cookie (carrying state, nonce and the PKCE code verifier).

This is key derivation from a **high-entropy machine secret**, not password
hashing. The rule targets low-entropy human-chosen passwords, where a
deliberately slow KDF raises the cost of offline brute force. That reasoning does
not transfer to a 256-bit random value: there is no dictionary to search. SHA-256
with a domain separator is standard, correct derivation here.

Unlike #6, the weak-input concern does not carry over in any practical way — the
state cookie is short-lived, and forging one yields a CSRF/PKCE-binding attack
that still requires defeating the provider's own `state`/`nonce` checks. Both
analysts and the judge agreed: severity **none**.

Switching to HKDF would silence the scanner without changing the security
properties. It would only invalidate in-flight state cookies, so it remains
available as a cosmetic option if a reviewer objects to the pattern on sight.

### #3 and #4 — `js/insecure-randomness` · `sdk/beacon-js/src/index.ts:35` and `:54`

Both report the same source: the `Math.random`-based UUID fallback in
`sdk/beacon-js/src/session.ts:18`, surfaced at its two call sites.

`session_id` is an **analytics correlation identifier**. It authenticates
nothing and authorises nothing. The beacon ingest endpoint requires a separate
token, is rate-limited, caps bodies at 64 KB, and treats input as hostile.

The SDK already prefers `crypto.randomUUID()` and reaches `Math.random` only when
that is unavailable. Because `crypto.randomUUID` is restricted to **secure
contexts**, the fallback effectively means a plain-HTTP deployment — where an
on-path attacker can simply read session IDs off the wire. Guessing them confers
nothing that observing them does not.

*Optional hardening, not required and not done:* `crypto.getRandomValues` is
**not** secure-context restricted, so inserting it as a middle tier would improve
the fallback in HTTP deployments. It would not close these alerts (the
`Math.random` branch would remain for environments with no `crypto` at all), so
it was left out rather than made to look like a fix.

---

## Excluded from analysis

### #21 `js/prototype-pollution-utility` and #22 `js/double-escaping` · `docs/api/index.html`

**Not Pulse code.** These are inside the ReDoc v2.5.3 minified vendor bundle,
which is inlined into the rendered API reference (longest line ~729,000
characters).

**Self-inflicted, and worth recording as such.** The bundle was inlined on
2026-07-31 because redocly's default output loads it from `cdn.redocly.com` and
pulls fonts from `fonts.googleapis.com` — contradicting this repo's no-CDN rule
and `api-guide.md`'s claim that the file is self-contained, and rendering blank
offline. Inlining fixed that and immediately created these two alerts against
third-party code.

Reverting to the CDN would trade two unfixable alerts in a documentation file for
a real privacy and supply-chain regression affecting every reader. So the path is
excluded from analysis instead, in `.github/codeql/codeql-config.yml` — which
documents the scope discipline that keeps that list from growing into a place to
hide our own findings.

---

## Standing gap

`Analyze (go)` and `Analyze (javascript-typescript)` are required status checks,
but they report whether the **scan ran**, not what it **found**. The aggregate
`CodeQL` check-run — the one that says "N new alerts" — is not required. That is
why four HIGH alerts sat open from 2026-07-09 to 2026-07-31 behind fully green
CI.

Adding `CodeQL` to the required contexts in `.github/branch-protection.sh` is the
fix, and it is safe to do **once this triage is reflected in the alert states** —
attempting it earlier would have blocked the PR that performed the triage on the
alerts it was triaging.
