# WO-B3 (S105) — replace placeholder identities in shipped metadata

**Review finding (Issue 6, P1, verified):** shipped artifacts identify a vendor that doesn't
exist: Helm chart points at github.com/pulse-analytics/pulse with maintainer
infra@pulse-analytics.io; OpenAPI license says "Proprietary" at https://pulse.dev/license
(contradicting the public licensing docs); event-schema `$id`s live under https://pulse.dev/.
Real identity: repo **github.com/aytekXR/ams-pulse**, vendor contact
**support@beyondkaira.com**, license **PolyForm Noncommercial 1.0.0 (server) + MIT (SDKs)**,
owned domain **beyondkaira.com**. (The go.mod module path is handled OUTSIDE this WO — do not
touch any `.go` file or go.mod.)

## Scope — you may ONLY edit these files
- `deploy/helm/pulse/Chart.yaml`
- `contracts/openapi/pulse-api.yaml`
- `contracts/events/beacon-event.schema.json`
- `contracts/events/alert-notification.schema.json`
- `contracts/events/ams-server-event.schema.json`
- `docs/api/index.html`
- `web/**` ONLY IF a grep shows web source references the old `$id` URLs (report if none)

No git commands. Other agents edit other files concurrently — expected.

## Work items

1. **`deploy/helm/pulse/Chart.yaml`:**
   - `home`/`sources` → https://github.com/aytekXR/ams-pulse
   - maintainer → name "Pulse (beyondkaira.com)", email support@beyondkaira.com
   - `appVersion` → "0.4.1" (its own comment says it matches the binary release tag — make that
     true); bump chart `version` 0.1.0 → 0.2.0. Keep every EXPERIMENTAL caveat.

2. **`contracts/openapi/pulse-api.yaml`:**
   - `info.license`: name "PolyForm Noncommercial 1.0.0 (server) / MIT (SDKs); commercial keys
     available", url https://github.com/aytekXR/ams-pulse/blob/main/docs/licensing-public.md
   - add `info.contact` (support@beyondkaira.com, url of the repo).
   - `info.version` stays "1.0.0" BUT add one sentence to `info.description` stating the API
     contract is versioned independently of the product release (deliberate policy — this closes
     the review's "state so in the file" option).
   - If a validator is available (npx @redocly/cli lint or similar already in the repo
     toolchain), sanity-check the YAML still parses; otherwise at minimum python-yaml-parse it.

3. **Schema `$id`s:** in the three `contracts/events/*.schema.json`, change
   `https://pulse.dev/schemas/...` → `https://pulse.beyondkaira.com/schemas/...` (keep the same
   trailing path/filename). They are opaque URIs; grep `server/`, `web/`, `sdk/` for `pulse.dev`
   to find any code coupling — report matches in `.go` files (do NOT edit those; they are handled
   by the orchestrator), fix matches in web/sdk source if any, and confirm the contracts
   drift-guard test suite you can find under server/ still passes conceptually (report which test
   files reference these schemas, if any).

4. **`docs/api/index.html`:** this is the rendered API reference. Determine how it was generated
   (header comment usually says). Update the license/contact strings to match item 2 — regenerate
   if the generator command is self-evident and runnable locally, otherwise make the minimal
   consistent string edits.

## Definition of done
No `pulse.dev` or `pulse-analytics` string remains in the files in scope; Chart.yaml appVersion
0.4.1/version 0.2.0; OpenAPI license matches docs/licensing-public.md. Return: files changed,
every remaining `pulse.dev`/`pulse-analytics` occurrence elsewhere in the repo (report-only),
validation results.
