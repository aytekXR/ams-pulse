# WO-B2b (S105) — marketplace docs: de-stale banners/rows, strip internal comment, media reality

Context: D-081 was cleared (D-169), pricing/support/SLA decided (D-169), support@beyondkaira.com
provisioned (D-171), GHCR public (D-168), MaxNodes reconciled (D-166). This session (S105/D-172):
screenshots are being COMMITTED to the repo (docs/marketplace/screenshots/ un-gitignored), the
demo rough-cut webm is attached to the GitHub release **v0.4.1**
(https://github.com/aytekXR/ams-pulse/releases/tag/v0.4.1), and both capture scripts are now
portable (no hardcoded machine paths).

## Canonical trial paragraph (identical wording is being applied to faq/licensing docs)
> **14-day Pro trial — no credit card.** Request a trial key from the Ant Media Marketplace
> listing or by emailing **support@beyondkaira.com**; the key arrives by email (typically within
> 1 business day) and activates in Settings → License. On expiry the deployment gracefully
> reverts to Free — no data loss.

Never describe the trial delivery as "self-serve" (no automated minting exists yet).

## Standardized AMS-compatibility claim (Issue 12)
> Validated live on AMS 3.0.3 Enterprise (current release); best-effort compatibility with
> AMS 2.10+ via version-tolerance tests (mock profiles).

## Scope — you may ONLY edit these files
- `docs/marketplace/listing-draft.md`
- `docs/marketplace/submission-package.md`
- `docs/marketplace/submission-process.md`
- `docs/marketplace/release-notes.md`
- `docs/marketplace/screenshot-list.md`
- `docs/marketplace/demo-video-script.md`

No git commands. Other agents edit other files concurrently — expected.

## Work items

1. **All six files:** remove the "DRAFT — INTERNAL … gated on … D-081" banners. Where a status
   banner is still useful (listing-draft), replace with one line: internal working copy; final
   text is pasted into the marketplace form by the operator at submission.

2. **`listing-draft.md`:**
   - Line ~21 "Contact for submission: NEEDS-OPERATOR" and line ~246 "[support channel —
     NEEDS-OPERATOR]" → **support@beyondkaira.com**.
   - §9 table (~lines 250-258): mark Support channel/SLA row RESOLVED (D-169 policy, D-171
     mailbox live); pricing row RESOLVED (D-169); any GHCR row RESOLVED (D-168). Rows that are
     genuinely still operator-outbound (submit listing, billing setup, load-lane capacity number,
     final demo recording) stay open — do not fake them.
   - DELETE the INTERNAL POSITIONING HTML comment (~lines 47-58, about Management-panel-reborn).
     Its content is preserved in docs/compatibility.md §G-27 and git history.
   - Trial mentions (~lines 151, 245): apply the canonical trial paragraph phrasing.
   - Add/align an explicit compatibility line using the standardized claim above.
   - If Kafka ingest is mentioned anywhere as a data source, label it "experimental/preview —
     pending live validation". Do not add new Kafka claims.

3. **`submission-package.md`:**
   - Screenshots row: now COMMITTED in-repo at docs/marketplace/screenshots/ (regenerable via
     the portable script); demo row: rough-cut RENDERED (D-170) and attached to release v0.4.1
     (link above); final voiceover recording remains operator (TBD-EXT is only that part).
   - Fix the internal inconsistency where the table says TBD-EXT but the blocking-items section
     says the rough cut exists — make both tell the same story.
   - Sweep remaining stale rows vs D-168..D-172.

4. **`submission-process.md`** §3 (~lines 84-94): GHCR row → DONE (D-168); support channel → DONE
   (D-169/D-171); pricing + MaxNodes → DONE (D-169/D-166); D-081 → CLEARED (D-169). Keep genuinely
   open items open (load lane, billing, the actual submission).

5. **`release-notes.md`:** de-banner; confirm content is v0.4.1-current; add a line that the
   GitHub release page for v0.4.1 now exists (it previously only existed as a tag).

6. **`screenshot-list.md`:** update status notes: the set is committed to the repo; the capture
   script is portable; reflect the current state of `ss1-light.png` exactly as the file's own
   caveat describes it OR as updated by this session (check whether
   docs/marketplace/screenshots/ss1-light.png is still byte-identical to ss1-dashboard.png at
   the time you run — use cmp — and write the true state).

7. **`demo-video-script.md`:** de-banner; note the rough cut is rendered (D-170) and attached to
   the v0.4.1 GitHub release; operator records the final.

## Definition of done
No D-081/DRAFT banners remain in these six files; no NEEDS-OPERATOR placeholder remains where a
decision exists; the HTML comment is gone; media rows match reality. Return: files changed, and
any row you deliberately left open (with why).
