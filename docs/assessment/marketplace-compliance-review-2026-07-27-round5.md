# External marketplace review — Round 5 (2026-07-27) · review + disposition

**Reviewed tree:** `669952e` = **the `v0.4.3` tag itself** — for the first time the review
target and the published release are the same tree (`v0.4.3-0-g669952e`, main 0 commits ahead).

**Executed in:** SESSION-110 / D-177.

**Reviewer's verdict:** *"Ready to submit — conditional on two operator gates, neither of them
code."* Round 5 re-verified all 14 round-4 dispositions against the code rather than trusting
the changelog, and found **13 fixed, 1 part-fixed-plus-disclosed, and the deep cluster items
honestly disclosed**. No new code defect and no falsifiable claim was found.

---

## The two gates

| Gate | State |
|---|---|
| **G-01** — confirm the v0.4.3 release pipeline completed | **Resolved.** The reviewer saw only 2 assets because they were looking at the *tag* page while the pipeline was still running (GitHub renders 2 auto source archives on a tag with no Release object yet). The first release attempt did legitimately fail — see below — and the re-run completed. |
| **G-02** — rotate `CLICKHOUSE_PASSWORD` | **OPEN, operator-gated.** The operator explicitly chose to defer rotation when authorising the v0.4.3 cut. Unchanged in severity: still the #1 pre-submission item. |

### G-01 in full — what actually happened

The first release run for `v0.4.3` **failed in 21 seconds at the pipeline's own CI gate**,
which requires a successful `ci.yml` run *for the commit being tagged*. PR #222's 19 green
checks ran against the pre-squash branch SHA, so they did not satisfy the gate for the squashed
merge commit `669952e`.

This was a sequencing error on our side — the tag was pushed immediately after the squash merge
instead of waiting for main's post-merge CI. **The gate worked exactly as designed: nothing was
built, scanned, promoted or published.** Once `ci` and `e2e` went green on `669952e`, the failed
job was re-run against the same tag; no re-tag and no force-push were needed.

The reviewer was right to record this as unverified rather than assume either way.

---

## Round-4 dispositions — re-audited by the reviewer

All 14 confirmed as reported in `…-round4.md`. Two worth restating:

- **F-03** was verified as fixed across all six documents *and* credited with the `query.go`
  role-resolution code fix — checked nil-guarded, race-free, and within the OpenAPI enum.
- **F-14**'s `CPUPctOK`/`MemPctOK` absent-field semantics were traced end-to-end through
  client → normalize → discovery → aggregator → anomaly baselines.

**G-08 — reviewer erratum (recorded).** Round 4 claimed the broadcast `originAdress` field
"appears nowhere in the tree". Round 5 retracts it: the field is in the real-AMS 3.0.3 capture
fixtures (`broadcasts_real_liveapp.json`, `broadcasts_real_test123_v303.json`); their round-4
search covered Go sources only. This confirms the S109 refutation — LIM-28's roadmap is grounded
in captured wire data. **Both directions of the verify-first loop are now on record.**

---

## New findings (G-series) — disposition

| ID | Verdict | Disposition |
|---|---|---|
| **G-01** Release artifacts incomplete at review close | **CONFIRMED (self-inflicted, resolved)** | Root cause was a tag-before-post-merge-CI sequencing error, not a pipeline defect. Fixed by re-running the release job after `ci`/`e2e` went green on the merged SHA. Recorded in the session notes as a standing rule: **after a squash merge, wait for main's CI before tagging** — the PR's checks belong to a different SHA. |
| **G-02** Credential rotation pending | **OPEN — operator-gated** | Deferred by explicit operator decision when authorising the cut. Remains #1 in `operator-expected.md` and is now the single authoritative rotation entry in the submission pack (see G-03). |
| **G-03** Rotation listed twice with contradictory status | **CONFIRMED — FIXED** | Self-inflicted in S109: a rotation bullet was added to a block of struck-through DONE items while the numbered list already carried it. The bullet block is now explicitly closed ("nothing above is still open") and rotation is **numbered item 1** of a single open-gates list. |
| **G-04** Port exemption keyed on stack presence, not listener ownership | **CONFIRMED — FIXED** | The S109 exemption skipped the preflight whenever the quickstart stack existed, regardless of which port it published. Now compares the requested port against the port the stack **actually publishes** (`docker compose port pulse 8090`), and exempts only on a match. Error-tolerant: any failure leaves the value empty and the conflict is reported, which is the safe direction. |
| **G-05** Three numbered citations survive an "all de-numbered" claim | **CONFIRMED — FIXED (a genuine miss)** | Round 4 flagged these too (F-11) and S109 fixed only the accompanying env-var count, leaving the citations. Worse, they had **already drifted** — `pulse-api.yaml:2672`/`:2631` now resolve to 3152/3109 — which is precisely the failure the de-numbering exists to prevent. All three now cite schema and column names instead. `AMS-INTEGRATION.md` is at **zero** numbered source citations. |
| **G-06** Version stragglers outside the guard's windows | **CONFIRMED — FIXED + guarded** | Corrected in `troubleshooting.md`, `beacon-sdk.md`, `install.md`, `CLAUDE.md`, `ankush-reply-draft.md` and the `faq.md` date stamp. To stop silent re-drift, **guard check #17** now scans the three customer-facing install/troubleshooting/SDK docs for stale image pins — deliberately reusing check #11's precise `ams-pulse:<semver>` pattern rather than a bare version grep, so prose that legitimately names an older release ("re-verified against 0.4.2") does not trip it. The reviewer's own warning about over-widening (round 4's #11 note) is respected. |
| **G-07** Two forward-looking hardening notes | **CONFIRMED — FIXED** | Both are now comments: the GHCR cleanup endpoint records that an org move requires `/orgs/{org}/packages/...` (and that the call would fail loudly, not silently), and guard #16's diff scope records that excluding `Chart.yaml` is deliberate because metadata-only edits do not change rendered output. |
| **G-08** Reviewer erratum on `originAdress` | **ACKNOWLEDGED** | No action — the S109 refutation stands and is now independently confirmed. |

---

## What this round did not change

- **Cluster engineering (round 4 F-04/F-05/F-06)** stays disclosed in LIM-10, unchanged. The
  reviewer explicitly endorsed the rewritten LIM-10 as *"a disclosure I could not improve on"*.
  It remains gated on a live multi-node cluster.
- **No version bump.** These are copy and hardening items; they ride the next release. v0.4.3 is
  the submission target.
