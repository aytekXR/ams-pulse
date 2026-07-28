# External Review Chain Index

This file indexes the eleven rounds of external review conducted for the Pulse
marketplace submission. Each round follows the protocol in
[`EXTERNAL-REVIEW-PROMPT.md`](EXTERNAL-REVIEW-PROMPT.md): the reviewer writes a
ledger file with findings, the maintainer verifies and dispositions each finding
in-place, and the completed file becomes the prior-round input for the next round.

---

## Round index

| Round | Date | Finding prefix | Files in-tree | Status |
|---|---|---|---|---|
| 1 | 2026-07-25 | A–L (lettered issues) | [`marketplace-compliance-review-2026-07-25.md`](marketplace-compliance-review-2026-07-25.md) | Dispositioned in D-173 |
| 2 | 2026-07-26 | N- (N1–N9) | [`marketplace-compliance-review-2026-07-26.md`](marketplace-compliance-review-2026-07-26.md) | Dispositioned in D-174 |
| 3 | 2026-07-26 | R- (R1–R15) | [`marketplace-compliance-review-2026-07-26-round3.md`](marketplace-compliance-review-2026-07-26-round3.md) | Dispositioned in D-175 |
| 4 | 2026-07-27 | F- (F-01–F-14) | [`marketplace-compliance-review-2026-07-27-round4.md`](marketplace-compliance-review-2026-07-27-round4.md) | Dispositioned in D-176 |
| 5 | 2026-07-27 | G- (G-01–G-08) | [`marketplace-compliance-review-2026-07-27-round5.md`](marketplace-compliance-review-2026-07-27-round5.md) | Dispositioned in D-177 |
| 6 | 2026-07-27 | H- (H-01–H-09) | [`marketplace-compliance-review-2026-07-27-round6.md`](marketplace-compliance-review-2026-07-27-round6.md) | Dispositioned in D-179 |
| 7 | 2026-07-27 | I- (I-01–I-06) | [`marketplace-compliance-review-2026-07-27-round7.md`](marketplace-compliance-review-2026-07-27-round7.md) | Dispositioned in D-181 |
| 8 | 2026-07-27 | J- (J-01–J-03) | [`external-review-2026-07-27-round8.md`](external-review-2026-07-27-round8.md) | Dispositioned in D-182 |
| 9 | 2026-07-27 | K- (K-01–K-02) | [`external-review-2026-07-27-round9.md`](external-review-2026-07-27-round9.md) | Dispositioned in D-183 |
| 10 | 2026-07-27 | L- (L-01–L-02) | [`external-review-2026-07-27-round10.md`](external-review-2026-07-27-round10.md) | Dispositioned in D-184 |
| 11 | 2026-07-28 | N- (N-01; M- skipped to avoid colliding with D-184's own M-01…M-04) | [`external-review-2026-07-28-round11.md`](external-review-2026-07-28-round11.md) | Dispositioned in D-185 |

---

## Known gaps in the chain

Round 8's finding J-03 filed this against rounds 6 and 7. Verifying it tree-wide
showed the gap is **wider than filed — it runs rounds 4 through 7.** Measured by
grepping each file for the reviewer schema markers (`**Claim.`, `**Reproduction.`,
`Proposed fix`) and counting per-finding detail sections:

| Rounds | What is in-tree | Reviewer verbatim text |
|---|---|---|
| 1–3 | Full per-issue narrative (14 / 10 / 1 detail sections) written before the output schema was standardized | **Substantially preserved**, in a pre-standardization format |
| 4–7 | Maintainer disposition tables only — round 4/5 as `\| ID \| Verdict \| Disposition \|`, round 6/7 as `\| ID \| Sev \| Subject \| Verified? \| Disposition \|` | **Absent** — 0 schema markers in all four files |
| 8 | The reviewer ledger in the standardized schema (9 schema markers, 3 detail sections) | **Complete** — the first round to land in-tree as written |
| 9 | The reviewer ledger in the standardized schema, Disposition column filled | **Complete** |
| 10 | The reviewer ledger in the standardized schema, Disposition column filled | **Complete** |

What rounds 4–7 preserve is the finding **subject** and the maintainer's
**disposition**. What they do not preserve is how the reviewer originally described
the defect: the Claim paragraph, the Reproduction steps, the Why-it-matters
rationale, and the Confidence assessment.

**Why the source text is not in this repository.** Two different causes, and round 9
(K-01) was right that collapsing them into one sentence overstated the case:

- **Rounds 4–5:** what these rounds left in-tree is a maintainer disposition table in
  the pre-standardization format. No reviewer ledger for either round has ever been
  delivered to this repository, and none has been offered since — round 9's K-01
  scopes its own recovery claim to rounds 6–7. *Whether a separate reviewer document
  was ever written for these two rounds is not something this repository can
  establish, and this file does not claim to know.*
- **Rounds 6–7:** per round 8's own Environment section, the reviewer's ledgers were
  written in the review session and delivered as chat text rather than as files, and
  round 7's commit attempt failed when the reviewer's device bridge dropped. That
  account is the reviewer's, recorded here as attribution rather than as something we
  verified. **What we can confirm is strictly this: no file named
  `external-review-2026-07-27-round6.md` or `…-round7.md` has ever reached this
  repository** — not tracked, not untracked, not ignored, and nowhere on the operator's
  filesystem. That is a statement about what arrived here, and it is the only kind of
  statement this side is entitled to make. Earlier wording in this file went further and
  described what the reviewer's *deliveries contained*; we cannot observe the reviewer's
  outbound channel, only its effect here, and round 10's L-01 was right to object to the
  overreach even though its own evidence did not survive (see below).

**Checked rather than assumed (rounds 8, 9 and 10).** Three consecutive rounds have
reproduced from an untracked `external-review-2026-07-27-round6.md`: J-03 asserted it,
K-01 asserted it more strongly ("visible in `git status` in every session"), and L-01
asserted it a third time with byte sizes (42,463 and 24,066) and a `head -1` transcript.
Re-verified from scratch at `a59c2ca` in D-184:

| Probe | Result |
|---|---|
| `git status --porcelain --untracked-files=all --ignored docs/assessment/` | empty |
| `git log --all -- …round6.md …round7.md` | no history on any ref |
| `find / -xdev -name 'external-review-2026-07-27-round*.md'` | only round 8 and round 9 (probe run at `a59c2ca`, i.e. before round 10's file landed; re-run at round 11 it returns rounds 8, 9 and 10, still no round 6 or 7) |
| `git worktree list` / other checkouts on the box | one worktree, one checkout |
| `.git/index.lock` | **absent**; `.git` is writable; repo is on native ext4 |

The two files L-01 reports do not exist here, and the byte sizes do not match the
similarly-named maintainer files that do (`marketplace-compliance-review-2026-07-27-round6.md`
is 12,422 bytes, not 42,463).

**The mechanism, finally identified — and it is nobody's dishonesty.** L-01 also reports
a stale `.git/index.lock` and git write operations failing with `Operation not permitted`.
No such lock exists here and git writes work fine; D-183 was committed and pushed from
this tree. A lock that cannot be unlinked by its owner on a native ext4 home directory is
characteristic of an overlay or bind-mounted sandbox, not of this repository. Together
with the file evidence, the conclusion is that **the reviewer's device bridge is attached
to a mirror of the repository rather than to the repository**: writes it makes land in
that mirror, are reported back to the review session as successful, and never arrive here.
The ledgers that *did* arrive — rounds 8, 9, 10 and 11 — entered the tree in maintainer
commits, transcribed from chat text. **Chat-delivered prose reaches this
repository; bridge-written files do not.** Both sides have been reporting their own vantage
accurately for three rounds and disagreeing because the vantages differ.

**Consequences adopted:** the reviewer's file-write receipts are not evidence about this
tree, and neither are our inferences about their delivery's contents. Rounds 6–7 prose can
still be recovered — but only by pasting it as chat text, the channel that demonstrably
works. Until then the gap stands, disclosed rather than backfilled.

**The gaps these findings point at are real and their fixes are adopted above; the
evidence offered for where the missing text lives is false in all three rounds.** The
distinction matters here more than usual — this file exists to be audited.

Reconstructing the missing rounds from the disposition summaries would mean inventing
reviewer prose. An honest gap beats invented evidence, so these rows stay documented
rather than backfilled.

**Round 11 closed this dispute from the other side.** The reviewer's own errata E-1
retracts L-01's evidence and states the mechanism in their own words: their bridge mounts
a mirror that receives the maintainer's commits but does not propagate their writes back.
Both vantages now agree, and nothing in this section rests any longer on our inference
about a channel we cannot observe. What we assert remains only what we can: no round-6 or
round-7 ledger has reached this repository.

**Consequence:** an auditor reconstructing the chain from this repository alone can
see *what was found and what was fixed* for every round, and *how the reviewer
described it* for rounds 1–3 and 8–11.

---

## File naming convention

Starting from round 8, the standardized naming is:
- **Reviewer produces:** `external-review-<YYYY-MM-DD>-round<N>.md`
- **Maintainer fills:** the Disposition column in that same file

Earlier rounds used `marketplace-compliance-review-<date>[-round<N>].md` for both
reviewer and maintainer content, which created the ambiguity this index documents.

The prompt itself contradicted this: §0 named the maintainer file as the prior-round
input while §5 told the reviewer to write the `external-review-…` file. That
contradiction is what produced J-03, and it is resolved in
[`EXTERNAL-REVIEW-PROMPT.md`](EXTERNAL-REVIEW-PROMPT.md) as of round 8.
