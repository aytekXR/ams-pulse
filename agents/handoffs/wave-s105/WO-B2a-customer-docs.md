# WO-B2a (S105) — customer docs: version stamps, trial story, licensing banner, support

## Canonical trial paragraph (use this everywhere, adapt phrasing to context)
> **14-day Pro trial — no credit card.** Request a trial key from the Ant Media Marketplace
> listing or by emailing **support@beyondkaira.com**; the key arrives by email (typically within
> 1 business day) and activates in Settings → License. On expiry the deployment gracefully
> reverts to Free — no data loss.

Rationale (D-169 + review Issue 9): the decided policy is a 14-day Pro trial without a card, but
no automated self-serve minting exists yet (marketplace billing setup is a pending operator
step), so the honest delivery story is request-by-email. Do NOT use the word "self-serve" for
the delivery mechanism anywhere.

## Scope — you may ONLY edit these files
- `docs/product.md`
- `docs/faq.md`
- `docs/licensing-public.md`
- `docs/support.md`
- `deploy/quickstart/.env.example`

No git commands. Other agents edit other files concurrently — expected.

## Work items

1. **`docs/product.md`** line ~33: "current release v0.4.0" → v0.4.1.

2. **`docs/faq.md`:**
   - Line ~3 header badge: "Pulse v0.4.0" → v0.4.1 (keep the last-updated/D-ref convention,
     update to D-172).
   - Q21 (trial): apply the canonical trial paragraph; fix the broken cross-reference
     "docs/licensing.md §2 (trial activation)" → `docs/licensing-public.md` §3.
   - Q2 (AMS versions): align to the standardized claim "Validated live on AMS 3.0.3 Enterprise;
     best-effort compatibility with AMS 2.10+ via version-tolerance tests (mock profiles)" if it
     deviates.

3. **`docs/licensing-public.md`:**
   - Remove the "DRAFT — INTERNAL … gated on D-081" banner (D-081 was cleared, D-169; this doc is
     in the public linkable set).
   - §3 (~lines 165-167): currently self-contradictory ("self-serve" then "contact support").
     Replace with the canonical trial paragraph.
   - Name the licensor where the doc introduces the license: the Pulse server is licensed under
     PolyForm Noncommercial 1.0.0, **Copyright (c) 2026 Aytek Erdoğan (beyondkaira.com)**; SDKs
     MIT. Keep the rest of the pricing/tier content (D-169 values) untouched.

4. **`docs/support.md`:**
   - Add one trial-request line (email support@beyondkaira.com) in the appropriate section so
     support and licensing tell the same story.
   - §4 (~lines 86-88): the OPERATOR-DECISION box about a bug-report issue template — replace
     with a factual line: the template now exists at `.github/ISSUE_TEMPLATE/bug_report.yml`
     (being added in this same session by another agent).

5. **`deploy/quickstart/.env.example`:**
   - Lines ~22-24: replace "A trial key is included with the marketplace listing; you can also
     activate a key later via Settings → License…" with the canonical trial story (short form:
     request via marketplace listing or support@beyondkaira.com; activate later via
     Settings → License).
   - Line ~33: commented example pin `ghcr.io/aytekxr/ams-pulse:0.4.0` → `0.4.1`.

## Definition of done
All five files agree on the trial story verbatim-consistently; no v0.4.0 stamps remain in the
three docs; licensing-public has no DRAFT banner. Return: files changed, lines touched, deviations.
