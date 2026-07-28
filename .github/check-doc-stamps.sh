#!/usr/bin/env bash
#
# check-doc-stamps.sh — guard the "Last updated:" stamp class in tracked docs.
#
# WHY THIS EXISTS (external review rounds 7, 8, 9, 10 — four consecutive rounds).
# Stamp drift is the single most-repeated defect class in this repository:
#   • I-01 — README prose release pointer shipped one release stale
#   • I-03 — compatibility.md stamp said D-176 against a D-179 rewrite
#   • J-03/K-01 — evidence-chain bookkeeping went stale the same way
#   • L-02 — filed against the README footer (which was, in fact, correct);
#     verifying it tree-wide found three OTHER docs genuinely stale.
# Every instance was fixed file-by-file, so the class kept landing in whatever
# file the last fix did not happen to touch. This closes it mechanically.
#
# TWO CHECKS
#
#   A. STAMP VALUE MUST BE A DATE (always fatal).
#      "Last updated: D-179 (2026-07-27)" is banned; "Last updated: 2026-07-27"
#      is required. A session D-number is internal bookkeeping that no reader
#      outside this repo can interpret, and it is ambiguous by construction —
#      it cannot distinguish "the content changed" from "someone bumped the
#      stamp". Five of the eight stamped docs carried a wrong D-number when
#      this was written. D-numbers remain welcome as PROSE PROVENANCE after
#      the date ("...corrected in D-179"); they just may not BE the stamp.
#
#   B. TOUCH THE DOC, TOUCH THE STAMP (fatal when a base ref is available).
#      If a commit range modifies a stamped doc, the stamp line must be one of
#      the lines that range adds. This is what actually catches forgetting —
#      and unlike a stamp-date-vs-commit-date comparison it has no false
#      positive when a PR merges days after it was written.
#
# WHAT THIS DOES *NOT* COVER — write it down, or the next reviewer will (S113):
#   • Docs with NO stamp at all are not required to grow one.
#   • Check A validates the stamp's SHAPE, never that the date is truthful.
#   • Prose D-numbers appearing after the date are deliberately allowed.
#   • Excluded trees (see EXCLUDE below) are immutable-by-design records:
#     dated review ledgers, per-session handoffs, and frozen brandkit uploads
#     whose stamps describe the snapshot's content, not the file's history.
#   • Release/version pins are a different class, guarded in release.yml (#19).
#
# USAGE
#   .github/check-doc-stamps.sh                # check A only
#   .github/check-doc-stamps.sh origin/main    # checks A and B
#
set -euo pipefail

BASE_REF="${1:-}"

# Immutable-by-design records; see "WHAT THIS DOES NOT COVER" above.
EXCLUDE='^(docs/assessment/|agents/handoffs/sessions/|brandkit/uploads/)'

# The stamp marker, allowing markdown emphasis around the label and the value:
#   Last updated: 2026-07-27          **Last updated:** 2026-07-27
#   _Last updated: 2026-07-27._       Last updated: **2026-07-27** — ...
MARKER='^[_*[:space:]]*[Ll]ast updated:?[_*[:space:]]*'

fail=0

note() { printf '%s\n' "$*" >&2; }

# ── Collect every tracked markdown file carrying a stamp ─────────────────────
stamped=()
while IFS= read -r f; do
  [[ "$f" =~ $EXCLUDE ]] && continue
  grep -qE "$MARKER" -- "$f" 2>/dev/null && stamped+=("$f")
done < <(git ls-files '*.md')

if [ ${#stamped[@]} -eq 0 ]; then
  note "check-doc-stamps: no stamped docs found — did the marker pattern drift?"
  exit 1
fi

# ── Check A: the stamp value is an ISO date ──────────────────────────────────
for f in "${stamped[@]}"; do
  while IFS=: read -r lineno line; do
    # Strip the label and any emphasis, then require YYYY-MM-DD up front.
    value="$(printf '%s' "$line" | sed -E "s/$MARKER//")"
    if ! printf '%s' "$value" | grep -qE '^[0-9]{4}-[0-9]{2}-[0-9]{2}'; then
      note "FAIL [A] $f:$lineno — stamp value is not an ISO date."
      note "        got:  $line"
      note "        want: 'Last updated: YYYY-MM-DD' (a D-number may follow as prose, but may not BE the stamp)"
      fail=1
    fi
  done < <(grep -nE "$MARKER" -- "$f")
done

# ── Check B: a change to a stamped doc must include its stamp line ───────────
if [ -n "$BASE_REF" ]; then
  if ! git rev-parse --verify --quiet "$BASE_REF" >/dev/null; then
    note "check-doc-stamps: base ref '$BASE_REF' not found (shallow clone?) — skipping check B."
  else
    # Diff the MERGE BASE against the WORKING TREE, not BASE...HEAD.
    #
    # This was a real bug caught by testing the guard instead of trusting it: with
    # BASE...HEAD, an uncommitted edit is invisible, so running this locally before
    # committing reported PASS no matter what you had changed — the one moment the
    # check is most useful. Diffing from the merge base to the working tree covers
    # committed branch work AND uncommitted edits, and using the merge base (rather
    # than the base tip) keeps unrelated commits that landed on the base out of it.
    base_commit="$(git merge-base "$BASE_REF" HEAD 2>/dev/null || echo "$BASE_REF")"
    changed="$(git diff --name-only "$base_commit" -- '*.md' || true)"
    for f in $changed; do
      [[ "$f" =~ $EXCLUDE ]] && continue
      [ -f "$f" ] || continue
      grep -qE "$MARKER" -- "$f" 2>/dev/null || continue
      # Added lines for this file across the same range. The leading '+' must be
      # stripped before matching: MARKER is anchored at ^, and the anchor would
      # otherwise land on the diff marker instead of the start of the line.
      #
      # ⚠ Collect into a variable, then match with a HERESTRING. Do NOT pipe
      # straight into `grep -q`. This script runs under `set -o pipefail`, and
      # `grep -q` exits the instant it matches — which sends SIGPIPE to the `sed`
      # and `grep` upstream of it, which makes the PIPELINE status non-zero, which
      # this reads as "no stamp found". So a SUCCESSFUL match was reported as a
      # FAILURE, and only sometimes: whether the upstream commands had already
      # finished writing is a buffering race. It presented as two of three
      # correctly-stamped files failing, with `sed: couldn't flush stdout: Broken
      # pipe` in the CI log as the only clue.
      #
      # Third time this guard has shipped a bug of its own (see the header). A
      # check that cannot be trusted to fail correctly is worse than no check: it
      # trains people to work around it.
      added_lines="$(git diff -U0 "$base_commit" -- "$f" | grep -E '^\+' | sed -E 's/^\+//' || true)"
      if ! grep -qE "$MARKER" <<<"$added_lines"; then
        note "FAIL [B] $f — modified since $BASE_REF but its 'Last updated:' stamp was not."
        note "        Bump the stamp in the same change, or the doc starts lying the moment it merges."
        fail=1
      fi
    done
  fi
fi

if [ "$fail" -ne 0 ]; then
  note ""
  note "Doc stamp guard FAILED. Rationale and scope: top of .github/check-doc-stamps.sh"
  exit 1
fi

printf 'Doc stamp guard: PASS (%d stamped docs checked%s)\n' \
  "${#stamped[@]}" "$([ -n "$BASE_REF" ] && printf ', incl. touch-the-stamp vs %s' "$BASE_REF")"
