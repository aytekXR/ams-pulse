# External Review Chain Index

This file indexes the eight rounds of external review conducted for the Pulse
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

What rounds 4–7 preserve is the finding **subject** and the maintainer's
**disposition**. What they do not preserve is how the reviewer originally described
the defect: the Claim paragraph, the Reproduction steps, the Why-it-matters
rationale, and the Confidence assessment.

**Why the source text cannot be recovered:** the reviewer's round-6 and round-7
ledgers were delivered in a chat session, not committed (round 7's write failed
mid-commit when the reviewer's device bridge dropped — recorded in round 8's own
Environment section). That text is not available to this repository, so these rows
are documented rather than backfilled. Reconstructing them from the disposition
files would mean inventing reviewer prose, which is worse than an honest gap.

**Consequence:** an auditor reconstructing the chain from this repository alone can
see *what was found and what was fixed* for every round, and *how the reviewer
described it* for rounds 1–3 and 8.

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
