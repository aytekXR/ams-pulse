# SESSION-110 — 2026-07-27 — external review round 5 executed; v0.4.3 released & verified

**Decision:** D-177. **Disposition table:** `docs/assessment/marketplace-compliance-review-2026-07-27-round5.md`.

## What round 5 was

Reviewed `669952e` = **the `v0.4.3` tag itself** — the first round where the review target and
the published release are the same tree. It re-audited all 14 round-4 dispositions **against the
code, not the changelog**, confirmed them, and found no new code defect and no falsifiable
claim. Verdict: *"Ready to submit — conditional on two operator gates, neither of them code."*

**G-08: the reviewer retracted one of their own round-4 claims**, confirming the refutation made
in S109 (`originAdress` is in the real-AMS capture fixtures). The verify-first loop now
demonstrably works in both directions.

## v0.4.3 — released and verified

| | |
|---|---|
| Release run | `30225886730` — **success** |
| Assets | `pulse-linux-amd64`, `pulse-linux-arm64`, `SHA256SUMS`, `ams-pulse-beacon-0.4.3.tgz`, `pulse-0.3.1.tgz` |
| Image | `0.4.3` and `latest` → **same digest** `sha256:75a76c67…727b4`, verified **anonymously** (HTTP 200, no auth) |
| Architectures | `linux/amd64` + `linux/arm64` |

### The first attempt failed — and the gate was right

The initial release run failed in **21 seconds** at the pipeline's CI gate, which requires a
successful `ci.yml` run **for the commit being tagged**. PR #222's 19 green checks belonged to
the *pre-squash branch SHA*, not the squashed merge commit `669952e`. The tag had been pushed
immediately after merging, before main's post-merge CI finished.

**Nothing was built, scanned, promoted or published.** Re-running the job once `ci` and `e2e`
went green on `669952e` was the entire fix — no re-tag, no force-push.

> **Standing rule: after a squash merge, wait for main's post-merge CI before tagging.**

The reviewer's "only 2 assets" observation (G-01) was the GitHub **tag** page, which renders two
auto source archives when no Release object exists yet. They were right to record it as
unverified rather than assume either way.

## Round-5 findings — all fixed

| ID | Disposition |
|---|---|
| G-01 release artifacts | Resolved (above) |
| G-02 rotate `CLICKHOUSE_PASSWORD` | **OPEN — operator-gated**, deferred by explicit operator decision |
| G-03 rotation listed twice, contradictory | Fixed — self-inflicted in S109; rotation is now item 1 of one authoritative open list |
| G-04 port exemption keyed on stack presence | Fixed — now compares the requested port against the port the stack actually publishes |
| G-05 three numbered citations survived "all de-numbered" | Fixed — **a genuine miss** (round 4 flagged them; S109 fixed only the adjacent count). They had *already drifted*. Zero numbered citations remain |
| G-06 version stragglers outside guard windows | Fixed + new **guard check #17** using check #11's precise `ams-pulse:<semver>` pattern |
| G-07 two hardening notes | Fixed — GHCR org-move caveat; guard #16's `Chart.yaml` exclusion documented as deliberate |
| G-08 reviewer erratum | Acknowledged; no action |

## Handoff hygiene (operator directive)

`RESUME-PROMPT.md` went from **4,171 → ~200 lines**. Every stacked "(previous) ▶ START HERE" and
"prior session context" block (84 of them) was deleted, along with the spent §0–§6 and §9–§10
state sections describing D-029…D-065 era work. What remains: the current session block, a
refreshed CURRENT STATE, and the binding rules (TDD, verification workflow, binding flows,
operating protocol, hard rules, environment).

**Rule going forward: only the current session block lives in `RESUME-PROMPT.md`.** Past
sessions live here in `sessions/` and in `decisions.md`.

## Gates

shellcheck clean · actionlint clean · guard check #17 dry-run PASS at 0.4.3 · release pipeline
green end-to-end. Docs-and-workflow only this session; no server/web/SDK source touched.

Prod untouched: v0.4.0-139, collector `ok`.

## Next session

1. Gate reads: prod health, git/PR drift, **and whether the operator rotated
   `CLICKHOUSE_PASSWORD`** (compare the live value's first 32 chars against `git log -S` —
   never print the secret).
2. If a new review arrives: verify every claim before fixing. Check tree-state assumptions too.
3. If a 2-node cluster appears: the deferred cluster work (LIM-10) becomes fixable with
   verification instead of guesswork.
