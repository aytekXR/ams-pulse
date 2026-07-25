# WO-B4 (S105) — licensor identification (LICENSE files + README license section)

**Review finding (Issue 8, P1, verified):** root LICENSE is verbatim PolyForm Noncommercial
1.0.0 with no licensor named; README license section names no copyright holder;
sdk/beacon-js/LICENSE has a placeholder copyright holder; sdk/beacon-swift has NO license file.
The commercial model (selling keys for noncommercially-licensed code) depends on unambiguous
ownership.

## Scope — you may ONLY edit these files
- `LICENSE`
- `sdk/beacon-js/LICENSE`
- `sdk/beacon-swift/LICENSE` (create)
- `README.md` — the License section ONLY (other agents edit other README sections concurrently;
  keep your edit surgically scoped to that section's exact lines)

No git commands.

## Work items

1. **Root `LICENSE`:** prepend exactly:

   ```
   Copyright (c) 2026 Aytek Erdoğan (beyondkaira.com)

   Licensed under the PolyForm Noncommercial License 1.0.0 (full text below).
   Commercial licenses (paid tiers) are available — see docs/licensing-public.md.

   ---
   ```
   followed by the untouched PolyForm text.

2. **`sdk/beacon-js/LICENSE`:** replace the placeholder holder line with
   `Copyright (c) 2026 Aytek Erdoğan (beyondkaira.com)` (keep MIT text intact).

3. **`sdk/beacon-swift/LICENSE`:** create — standard MIT text with the same holder.

4. **`README.md` License section:** state: server + web under PolyForm Noncommercial 1.0.0,
   Copyright (c) 2026 Aytek Erdoğan (beyondkaira.com); SDKs (`sdk/beacon-js`, `sdk/beacon-swift`)
   MIT; commercial licensing via docs/licensing-public.md. Keep the section's existing length
   discipline (a few lines, not an essay).

## Definition of done
All four files name the same licensor; PolyForm/MIT texts unmodified apart from the notice
lines. Return: files changed, exact notice text used.
