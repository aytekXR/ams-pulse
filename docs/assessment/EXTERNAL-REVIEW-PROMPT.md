# External review prompt — Pulse (hand this verbatim to the reviewer)

**What this file is.** The standing brief for an independent external reviewer of Pulse
(self-hosted analytics / QoE monitoring / alerting for Ant Media Server). It defines a
three-phase review — **black box → documentation → code** — and mandates a **single
markdown output file** whose schema is consumed directly by the maintenance loop as the
input for the next round. Prompt in, ledger out, same shape every time.

**How to use it**

1. Give the reviewer this whole file, plus the round number and the exact tree to review.
2. The reviewer produces **one** file: `docs/assessment/external-review-<YYYY-MM-DD>-round<N>.md`.
3. The maintenance session verifies every finding against the code, fills the
   **Disposition** column in that same file, and carries it forward as the prior-round
   input for round N+1.

**File ownership (resolves round-8 finding J-03):**
- **Reviewer writes:** `external-review-<YYYY-MM-DD>-round<N>.md` — the review ledger
- **Maintainer writes:** `marketplace-compliance-review-<YYYY-MM-DD>-round<N>.md` — the disposition file (originally used before the prompt was standardized; later rounds use the reviewer's file directly with Disposition filled)
- **Prior-round input for round N:** the reviewer's `external-review-…-round<N-1>.md` with its Disposition column filled by the maintainer (or, for early rounds before the naming was standardized, the maintainer's `marketplace-compliance-review-…-round<N-1>.md`)

---

## 0. Assignment parameters (fill these in before sending)

| Parameter | Value |
|---|---|
| Round number | `<N>` |
| Review target | e.g. `v0.4.4` (a release tag) **or** `main@<sha>` |
| Published image | `ghcr.io/aytekxr/ams-pulse:<version>` (tags have **no** `v` prefix) |
| Repository | `https://github.com/aytekXR/ams-pulse` (public) |
| Prior-round file | `docs/assessment/external-review-<YYYY-MM-DD>-round<N-1>.md` (omit for round 1). For early rounds before naming was standardized, use `marketplace-compliance-review-<date>-round<N-1>.md`. |
| Finding ID prefix | one unused letter, e.g. round 7 used `I-01`…, so round 8 uses `J-01`, `J-02`, … |

**Say which tree you actually reviewed, as a SHA, in the first line of your output.** Past
rounds have gone wrong because the reviewer assumed `main` and the tag were identical when
they were not. If you review the tag, `main` may already be ahead; if you review `main`, the
published release may lag. Both are legitimate — but state which one you looked at, and treat
the gap between them as a finding in its own right if it would mislead a customer.

---

## 1. The product, in one paragraph

Pulse installs **next to** Ant Media Server, not inside it. It polls the AMS REST API
read-only, stores metrics in ClickHouse and configuration in a meta store (SQLite by default,
Postgres optional), serves a React dashboard and a public HTTP API, ingests player-side QoE
via a browser beacon SDK, and fires alerts to email/Slack/Telegram/PagerDuty/webhook. It is
licensed in four tiers (free/pro/business/enterprise) with no phone-home. It is being prepared
for an **Ant Media Marketplace listing**, so "would this survive a vendor qualification
review?" is the standing question behind every phase.

---

## 2. Phase 1 — BLACK BOX (do this first, and finish it before you read any source)

**Rule: until Phase 1 is written up, do not open the source tree.** Not the repo, not the
Dockerfile, not the tests. The purpose is to capture what a customer and an Ant Media
qualification reviewer actually experience, uncontaminated by knowing how it works. Reading
the code first destroys this evidence permanently for the round — you cannot un-know it.

Work only from published artifacts:

- `docker pull ghcr.io/aytekxr/ams-pulse:<version>` — **anonymously**, no login
- the one-command installer documented in the repo README
- the GitHub Release page: binaries, `SHA256SUMS`, SDK tarball, Helm chart
- the OCI Helm chart: `oci://ghcr.io/aytekxr/charts/pulse`
- the running product's UI and HTTP API

Cover at least:

1. **First ten minutes.** Follow the documented install path exactly as written, on a clean
   host, with no prior knowledge. Time it. Every place you had to guess, backtrack, or read
   ahead is a finding. If you need an AMS to point at, say so and note whether the docs told
   you that up front.
2. **Honest failure.** Give it a wrong AMS URL, wrong credentials, an occupied port, and no
   AMS at all. Does it fail loudly and specifically, or claim success and degrade silently?
   Silent success on a broken configuration is a **HIGH** finding, always.
3. **Supply chain, as a security reviewer.** Verify the image signature, checksums, SBOM and
   provenance using only the commands the project publishes. **If a published command fails,
   that is a finding even when the underlying artifact is fine** — the reviewer's experience
   is the product. Record your client tool versions; version skew is a real failure mode here.
4. **The product's own claims.** Take the marketplace listing copy, the README and the
   feature list, and try to falsify each claim through the UI and API. Tier gates: does a
   free-tier install actually refuse paid features, or merely hide the buttons?
5. **API surface.** Exercise the public API against its published OpenAPI spec. Undocumented
   endpoints, documented-but-absent endpoints, and response shapes that disagree with the
   spec are all findings.
6. **Screenshots and demo assets.** Compare every listing screenshot against what the running
   product renders. Note staleness, and note any provenance claim that overstates how the
   asset was produced.

Write Phase 1 up **before** proceeding. Include what you could not test and why — a missing
prerequisite, a sandbox network restriction, an unavailable AMS licence. Say it plainly; an
untested area silently presented as tested is worse than an admitted gap.

---

## 3. Phase 2 — DOCUMENTATION (still no source)

Read the docs as the two audiences who matter: an operator installing this in production, and
an Ant Media reviewer deciding whether it is listable.

- **Does the documented path work?** Every command you can run, run. Every link, resolve.
- **Version coherence.** Image pins, chart versions, SDK tarball names, release-note versions
  and doc header stamps must agree with each other *and* with the artifact actually published.
  Check the docs at the **tag** as well as on `main` — they can differ, and a customer
  browsing a release tag sees the tag.
- **Are the limitations honest?** `docs/known-limitations.md` is the project's disclosure
  ledger. Look for anything the product cannot do that is *not* in it — especially anything
  the marketing copy implies it can. An overclaim that survives to a marketplace listing is
  the single most damaging class of defect in this review.
- **Are the disclosures load-bearing or decorative?** A limitation disclosed in a file nobody
  reads, while the listing claims the opposite, is not disclosed.
- **Security and licensing docs.** `SECURITY.md`, licensing/trial terms, support SLA: complete,
  consistent, and actually deliverable by a small vendor?

---

## 4. Phase 3 — CODE

Now read the source. Prioritise by blast radius, not by curiosity:

1. **License and tier enforcement** — is a paid feature gated server-side, or only in the UI?
2. **Alert firing and delivery** — false negatives (an alert that should fire and doesn't)
   outrank false positives. Check edge-detection, state eviction, and what happens when the
   subject of an alert disappears.
3. **Ingest health scoring and the collector** — staleness handling, partial-outage behaviour.
4. **AMS wire decode/normalize** — hostile and malformed input; unit assumptions on
   timestamps; fields assumed present.
5. **The query layer** — unbounded growth, N+1 patterns, cardinality traps.
6. **Beacon ingest** — treat it as fully hostile input; it is public and unauthenticated.

Architectural rules that a change can violate (worth checking, since violations are invisible
at runtime until they hurt): AMS wire formats belong **only** in `server/pkg/amsclient` and
`server/internal/collector`; metrics live in ClickHouse and configuration in the meta store,
never crossed; the web UI consumes only generated public-API types.

**Verify against the tree, never against the changelog.** A changelog entry saying something
was fixed is a claim to test, not evidence. Rounds 4 and 5 both found items whose changelog
said "all fixed" while instances survived in the tree.

---

## 5. Output contract — ONE file, this schema

Write exactly one file: `docs/assessment/external-review-<YYYY-MM-DD>-round<N>.md`.
No second file, no separate summary, no appendix file. The maintenance loop reads this one.

````markdown
# External review — Round <N> (<YYYY-MM-DD>)

**Reviewed tree:** `<sha>` (`<tag or branch>`) · **main at review time:** `<sha or "same">`
**Reviewer:** <name/tool> · **Phases completed:** blackbox / docs / code
**Verdict:** <one sentence: ready to submit / not ready — and the single reason why>

## Environment and limits
- Tool versions used for verification (docker, cosign, helm, node, curl …)
- What could NOT be tested, and why

## Findings

| ID | Sev | Phase | Subject | Claim (falsifiable) | Evidence | Proposed fix | Disposition |
|---|---|---|---|---|---|---|---|
| <P>-01 | HIGH | blackbox | <one line> | <the specific assertion to verify or refute> | <commands run + exact output> | <smallest correct change> | *(left blank — maintainer fills)* |

## Finding detail

### <P>-01 — <title>  [HIGH · blackbox]
**Claim.** One falsifiable sentence. Not "error handling is weak" but "with `PULSE_AMS_URL`
pointing at a closed port, `install.sh` exits 0 and prints 'Pulse is healthy'."
**Reproduction.** Exact commands, exact output, timestamps.
**Why it matters.** Concrete consequence for a customer or an Ant Media reviewer.
**Proposed fix.** Smallest correct change. Say if you are unsure it is correct.
**Confidence.** high / medium / low — and what would raise it.

## Prior-round re-audit  *(omit in round 1)*

| Prior ID | Reported as | Verified state now | Note |
|---|---|---|---|

## What I checked and found correct
Brief. This is what stops the next round re-treading ground, and it is the section that makes
this file useful as maintenance input rather than just a defect list.

## Errata — my own prior-round errors
If a previous round of yours made a claim this round disproves, say so explicitly here.
````

**Rules for the output**

- **Severity**: `HIGH` = ships wrong behaviour, a false public claim, or a security exposure ·
  `MEDIUM` = real defect with a workaround, or a misleading doc · `LOW` = cosmetic, stale,
  or internal-only. Rank the table most-severe first.
- **No line numbers as citations.** They drift between the review and the fix, and have
  already gone stale mid-round here. Cite `file → symbol/heading` instead.
- **One finding per row.** Do not bundle.
- **State assumptions as assumptions.** If a finding depends on something you did not verify,
  say which part is assumed. A confidently-worded wrong finding costs more to refute than a
  hedged right one costs to confirm.
- **Refutation is a valid finding.** If you cannot reproduce something you expected to find,
  record that. It is evidence.
- **Leave `Disposition` empty.** The maintainer fills it with
  `FIXED <ref>` / `REFUTED <why>` / `DISCLOSED <LIM-NN>` / `DEFERRED <reason>`, and the
  completed file becomes the prior-round input for round N+1.

---

## 6. Standing context the reviewer should not re-litigate

These are known, deliberate, and recorded. Report them only if you find them **materially
worse than described** — in which case say precisely how.

- **Cluster support is limited.** AMS 3.x exposes no node role or version, so all nodes
  display as `origin`, edge/origin viewer dedup is inert, and node alerting during an AMS API
  outage is not fully reliable. Disclosed as **LIM-10**. There is no live multi-node cluster
  to verify a fix against, and guessing trades a missed alert for a false one.
- **Capacity numbers are provisional** until a load lane runs on a dedicated AMS instance.
- **Prod runs a stamped older build** than the current release. Rolling production forward is
  a deliberate, operator-gated act, never automatic.
- **Some items are operator-gated by design** and are not engineering defects: credential
  rotation, marketplace listing submission, billing setup, the AMS PAYG load lane.

---

## 7. What a good round looks like

The best previous round found no new code defect, re-verified every prior disposition against
the code rather than the changelog, and **retracted one of its own earlier claims** when the
evidence went the other way. That is the standard: adversarial, specific, reproducible, and
willing to be wrong in public. A round that produces three reproducible HIGH findings and a
short honest "checked and correct" list is worth more than one that produces thirty
speculative ones.
