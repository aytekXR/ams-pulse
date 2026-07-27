# SESSION-113 — 2026-07-27 — external review round 7 (I-01…I-06) verified & executed

**Decision:** D-181. **Operator directive:** *"here are the reviews"* (round 7).

**Result: all six findings CONFIRMED, all six fixed, none refuted — two larger than filed.**
Dispositions: `docs/assessment/marketplace-compliance-review-2026-07-27-round7.md`.

Round 7's verdict: *"ready to submit as soon as G-02 closes."* It independently re-verified all
nine round-6 dispositions against the **published v0.4.4 artifacts** rather than the changelog,
confirmed the H-02 recurrence pattern is broken (post-tag drift is internal-only now), and read
the new `system-resources` code as hostile-input code rather than diffing it. What it found was
six items of prose/comment drift plus one metric-semantics nit — which is what a mature pack
looks like.

## Gate reads

| Gate | Reading |
|---|---|
| Prod health | 3/3 `/healthz` components `ok`, collector ingesting |
| `CLICKHOUSE_PASSWORD` | **Still un-rotated** — live prefix matches 2 commits (checked silently) |
| Git drift | `origin/main` == local HEAD `463927e`; clean tree |

## Findings

| ID | Fix |
|---|---|
| **I-01** MED | README inside the *new* submission target still said "current: **v0.4.3**". Three strings corrected; the cosign-history range **de-literalized** ("`0.3.0` and every release since") so it cannot go stale again. **New guard check #19** pins both prose release-pointers to the tag, mutation-proved both directions. |
| **I-02** LOW | Helm README's "0.3.1 carries appVersion 0.4.3" → points at `Chart.yaml` instead of naming a pair. A literal that drifted twice gets removed, not bumped. |
| **I-03** LOW | `compatibility.md` stamp D-176 vs a D-179 rewrite — **plus two the review missed**: `ARCHITECTURE.md` (D-161) and `licensing.md` (**D-066**, eighteen decisions stale over content D-173 changed). |
| **I-04** LOW | Reproduced numerically before fixing: the D-179 timing window spanned the whole fallback chain and measured **603 ms** — the sum of two legs that should not have been in it. Each call now timed separately; `GetVersion` back outside the window. Two regression tests. |
| **I-05** LOW | "standalone AMS 3.x never reports cpu_pct" — false since D-179. Review cited 3 sites; a sweep found **8**. All now state the durable invariant: skip on ABSENCE, never on an assumption about AMS versions. |
| **I-06** LOW | LIM-10 said "AMS 3.x"; D-179 had already proven 2.14 → 3.0.3. Title and body now say **2.14–3.x** with the four verified tags, plus an explicit line for 2.16/2.17-cluster prospects. |

## Three things worth carrying forward

- **A guard's scope is a decision, and scope gaps are where drift lands.** Guard #17 was
  deliberately narrowed to runnable pins to avoid false positives — correct then, and the exact
  reason I-01 slipped through into the new submission target hours later. When narrowing a
  guard, write down what the narrowing leaves uncovered.
- **De-literalize what drifts twice.** The cosign range and the chart-version sentence were both
  "just update the number" fixes the first time. The second time is the signal to remove the
  literal instead.
- **An observation in a review ledger is not a shipped disclosure.** D-179 proved LIM-10's fact
  applies to 2.14–3.x and wrote it in "Found by us" — the customer-facing LIM still said 3.x.
  Round 7 caught it. Check that every "found by us" bullet has a corresponding edit.

## Gates

`gofmt -l` empty · `go build` OK · `go vet` clean · full `go test ./... -race` green with the
repo-root mount (0 FAIL) · alert/anomaly/meta suites green after the comment sweep · guard #19
mutation-proved · ShellCheck clean on 0.9.0 and 0.11.0 · both workflows parse as YAML.

## Left for the operator

1. **Rotate `CLICKHOUSE_PASSWORD`** — the single blocker, and the reviewer agrees. Their stated
   criterion: rotate, then submit against **v0.4.4** per the existing pack.
