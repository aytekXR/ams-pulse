# SESSION-116 — 2026-07-27 — external review round 10 (L-01…L-02) verified & executed

**Decision:** D-184. **Input:** the reviewer's round-10 ledger — their declared *final* round,
verdict *"not blocked by engineering; blocked by one operator rotation and one tag."*

**Result: both filed findings REFUTED as filed — and the session's real work came from auditing
the round's ~90% of *reassurance*, which failed in three of nine areas and produced four
maintainer-found defects, three of them fixed here under TDD.**
Dispositions: `docs/assessment/external-review-2026-07-27-round10.md`.

This is the first round where the reviewer's evidence failed on **every** filed finding, and also
the round that paid for itself most: chasing the failures identified the structural cause of a
three-round dispute, and the discipline of testing the exculpations found real defects that ten
rounds of review had not.

## Gate reads

| Gate | Reading |
|---|---|
| Prod health | 3/3 `/healthz` components `ok`, **1,337,678** server events, newest **1 s** old |
| `CLICKHOUSE_PASSWORD` | **Still un-rotated** — live prefix matches 2 commits (checked silently) |
| Git drift | `origin/main` == local HEAD `a59c2ca`; clean tree; no tag beyond v0.4.4 |
| Release version guard | extracted from `release.yml`, dry-run at `GITHUB_REF_NAME=v0.4.4` → **PASS** after the README edit |

## Filed findings

| ID | Filed | Verdict | Fix |
|---|---|---|---|
| L-01 | MEDIUM — `review-chain.md` asserts something `git status` disproves | **Objection confirmed, evidence refuted (3rd round running)** | The overreaching sentence is rescoped to what this side can observe. The files still do not exist here: five independent probes, incl. a whole-filesystem search and `git worktree list` |
| L-02 | LOW — README footer D-stamp is stale | **Refuted as filed; class confirmed and 4× wider** | README was *correct* (last modified in D-182, stamp said D-182). Seven genuinely stale/malformed stamps in five other files. All eight normalized to date-valued; class closed mechanically |

## The mechanism behind the recurring evidence failures (the round's real L-01 outcome)

J-03, K-01 and now L-01 all reproduce from an untracked `external-review-…-round6.md`. L-01 added
a decisive detail without realizing it: a stale `.git/index.lock` that cannot be unlinked
(`Operation not permitted`). **This tree has no lock, `.git` is writable, the repo is on native
ext4, and D-183 was committed and pushed from it.** A lock its owner cannot remove is
characteristic of an overlay/bind-mounted sandbox.

Conclusion, recorded in `review-chain.md`: **the reviewer's device bridge is attached to a mirror
of the repository, not the repository.** Its writes land there, are reported successful to the
review session, and never arrive here. Rounds 8 and 9 arrived only because they were pasted as
chat text and transcribed by the maintainer. **Chat text reaches this repo; bridge writes do not.**
Neither side was ever wrong about its own vantage. Round 11, if there is one, must not spend a
fourth round on this.

## Exculpation audit — where the session's substance was

Round 10 is mostly an all-clear: a security deep-dive over a core the loop had never read, plus a
PRD F1–F10 table. Per S114 ("a reviewer's *this one is safe* is the one place nobody looks twice"),
nine adversarial lanes attacked the exculpations, each told to default to refuted and to hunt the
scoping gap; every surviving claim was then **re-verified by hand** before being written down —
which mattered, because two lanes' prose and structured output disagreed and two apparently-
confirmed claims did not survive a direct read.

**Upheld (5/9):** `ssrfguard` policy · secrets at rest · auth transport · audit trail · LIM
citations. **Refuted (1):** the claimed webhook cross-source isolation gap is deliberate,
documented and pinned by a test. **Failed (3):**

| ID | Defect | Sev |
|---|---|---|
| **M-01** | `ssrfguard.DialControl` is on 5 outbound clients and missing from 3 — email channel (`smtp_addr`, API-supplied), CertChecker (`cert_expiry` rule scope, API-supplied, wired in prod), S3 uploader (env) | MEDIUM |
| **M-02** | `/ingest/beacon` has two implementations; the **documented default** (main port) lacks the A10 field truncation the optional listener applies. S101 fixed this same divergence once, scoped to schema validation only | MEDIUM |
| **M-03** | Paid alert channels keep delivering after a tier downgrade — while `prober.executeProbe` one package over has exactly the runtime `EntitlementGate` they lack (S37 / D-108) | MEDIUM |
| **M-04** | `session_id`/`stream_id`/`app`/`player_kind` length-unbounded | LOW — **recorded, not fixed** |

**M-04 is deliberately unfixed:** truncating a `session_id` would silently merge distinct sessions
and corrupt the `uniq(session_id)` aggregates K-02 was about — worse than the defect it closes.
Rejecting instead is a behavioural break needing a contract change (frozen, D-004). It needs a
product ruling on limit *and* failure mode; added to the operator's decision-gated list.

**M-01 was fixed in the constructors, not at the call sites** — the class exists precisely because
the guard was wiring-dependent, so a future call site now cannot forget it.

## Doc-stamp class — closed mechanically after four rounds

I-01, I-03, J-03/K-01 and L-02 are all the same class, and each fix was scoped to the file that
round named. `.github/check-doc-stamps.sh` + a `doc-stamps` CI job now enforce (A) every stamp's
value is an ISO date — always fatal, and (B) a change to a stamped doc must also move its stamp —
fatal whenever a base ref is available. Check B is what catches forgetting, and unlike a
stamp-date-vs-commit-date comparison it has no false positive when a PR merges days late.
**What the guard does not cover is written at the top of the script** (per S113's rule).

The checker immediately found **four instances this session's own manual sweep had missed**
(`ARCHITECTURE.md` §§45/68/97, `runbooks/alerting.md:5`) — the mechanical check outperformed the
careful human sweep on its first run, which is the argument for having written it.

## Internal inconsistency found while reading

`operator-expected.md` item 3 said "Submit against `v0.4.4`" while the same file's closing note and
`RESUME-PROMPT.md` both prescribe "rotate, cut v0.4.5, submit against it". Aligned; the choice
remains the operator's.

## Still open, unchanged

**G-02** — rotate `CLICKHOUSE_PASSWORD`. Tenth consecutive round as the sole submission blocker.
Operator-gated; no review round can close it.
