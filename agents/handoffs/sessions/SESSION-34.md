# SESSION-34 — e2e for the six uncovered pages (D-096) — **CLOSED 2026-07-14**

**Branch:** `s34-e2e-six-pages` · **Decision record:** D-096 in `agents/handoffs/decisions.md`

> ## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — CARRY THIS INTO SESSION-35
> Before dispatching: re-read ROADMAP-V2 §2 and the final-assessment §5 roadmap and REVISE the
> session plan if a higher-leverage move exists. The plan is a starting point, not a contract.

**S33 merge question (the plan's ⛔ blocker): RESOLVED.** PR #47 went green on all 15 checks and
was merged; `main` reached `31f55b0` carrying §2.19 Waves 0–5, the S32 commit-escape fix, and the
G3/G5/G6 token fixes. Nothing was stacked on an unmerged branch.

**Chosen from the plan's candidates: #2** — "e2e coverage for the six pages that still have NONE
… **Strongest technical candidate**." The plan was right.

---

## Goal

Waves 3/4/5 rewrote six pages — Ingest, Anomalies, Alerts, Settings, Reports, Probes — and
**none had ever been driven in a real browser.** The unit suite was green throughout. That is not
reassurance; it is the problem. Every defect found this session is one jsdom structurally cannot
see: focus movement, key dispatch, native dialogs, the real ARIA tree.

## What shipped

| | |
|---|---|
| `web/e2e/support/stubs.ts` | NEW. One canonical boot layer (`stubApp`/`json`/`collectErrors`). |
| 6 new specs | 38 tests. Playwright suite **22 → 60 green**. |
| `AlertsPage.tsx` | Channel deletion no longer calls native `window.confirm()`. |
| `ProbesPage.tsx` | The delete `role="dialog"` now behaves like a dialog. |
| `ROADMAP-V2.md` §2.3 | Ledger correction — it was never open. |

The tier matrix is **not uniform**, which is why `stubApp` takes a tier: Reports gates unless
business|enterprise, Anomalies gates unless enterprise, and **Probes gates only free** — it opens
at pro, the opposite direction from the others.

## The two real defects — both fixed, both RED-proven

**1. Alerts channel deletion used native `window.confirm()`** (`AlertsPage.tsx:132`). Wave 4 gave
*rules* an inline confirmation step and missed *channels*, leaving two confirmation models for
the same destructive verb. It survived for an instructive reason: **jsdom stubs
`window.confirm`**, so no unit test ever saw a dialog at all. A whole class of bug hides there.

**2. The Probes delete dialog was a `role="dialog"` that behaved like a `<div>`.** Opening it left
focus on the row button — a screen-reader user was never told it appeared — and Escape did
nothing. Focus now lands on the dialog container (`tabIndex={-1}`), so AT announces the label
*and* the body copy that says the deletion is permanent, and the destructive button is not one
Enter keypress away. Escape cancels; focus returns to the trigger. It deliberately does **not**
set `aria-modal` and does **not** trap Tab: it renders inline, not as an overlay, and the page
behind it stays live. Claiming modality we don't implement would be a lie to assistive tech.

## The false green I nearly shipped

The Probes free-tier gate test stubbed no probes route. **Delete the gate entirely and there is
no data, so no table renders, and `expect(table).toHaveCount(0)` still passes** — it was measuring
the absent stub, not the gate. The adversarial audit caught it. Four other assertions were weak in
the same family and were tightened: an `emulateMedia` call that pinned nothing (dark is the
fallback, so the test passed for the wrong reason); an `aria-live` check scoped to `form` that a
mirror one tag outside would have escaped; a sigma re-fetch proven to *fire* but never to
*render*; an `aria-labelledby` pointing at an id nobody asserted exists.

## Two things I got wrong (recorded because the next session will be tempted the same way)

**1. I accused the agents of fabricating their test runs. They had not.** Chromium cannot launch
bare-metal on this host, so when every agent hit that error and still reported `green: true`, I
concluded they had faked it — and said so before checking. The transcripts showed one had
extracted the missing libraries from `.deb` packages and set `LD_LIBRARY_PATH`; Chromium really
ran, and the greens were real. **Check the evidence before asserting the conclusion — including
when the conclusion is "the agent lied."**

**2. I re-solved a solved problem.** The correct way to run e2e on this host was **already in this
file's own gates section**, and is how S33 got 22/22. Neither I nor the agents read it before
improvising. **Read the gates section before running gates.**

## ⚠️ Running e2e on this host — the ONE right way

Bare-metal Chromium **cannot launch** (`libatk-1.0.so.0`, `libgbm`, `libasound` are not installed;
no sudo). Do not install anything and do not patch `LD_LIBRARY_PATH`. Use the image:

```sh
cd web && pkill -f 'vite preview'   # CI=1 disables reuseExistingServer; free 4173 first
sg docker -c "docker run --rm --network host -v \$PWD:/work -w /work \
  -e CI=1 mcr.microsoft.com/playwright:v1.61.1-noble npx playwright test"
```

**The Go toolchain is also not on PATH.** The only copy is pre-commit's:
`export PATH="/home/aytek/.cache/pre-commit/repoiavouv2x/golangenv-default/.go/bin:$PATH"` (go1.26.5).

## Gates

- **Playwright 60/60** (was 22) in `mcr.microsoft.com/playwright:v1.61.1-noble` — CI-faithful
- vitest **619/619** (36 files); coverage 67.47 / 65.30 / 59.65 / 69.98 vs floors 59 / 54 / 45
- lint, `tsc --noEmit`, build — all clean
- **RED proofs:** neutering the dialog focus + Escape reddens exactly the 2 new a11y tests and
  nothing else; reverting the channel confirm to a direct API call reddens exactly the 2 new
  channel tests and nothing else.

## Operator action required? **No.**

Nothing new blocks the product, and nothing was needed from the operator this session. The
pre-existing blockers are unchanged — see `docs/operator-expected.md`. The two that matter:

1. **GHCR is still private** (anonymous pull → 401). Until it is flipped, **no customer can
   `docker pull` Pulse**, and every install doc and marketplace claim is fiction.
2. **The AMS license expires 2026-07-27T13:45Z — 13 days.** The only item with a hard clock. A
   lapse **plus** the next `antmedia` restart = total ingest death; both arms are proven
   (D-092/D-093).

## Still open

- **Prod is 7 commits behind `main`** — still on the S27 build (`v0.3.0-34-g58a9c84`), so none of
  D-089..D-096 is live. **Unblocked; it is S35's first task.**
  Runbook: `deploy/runbooks/upgrade-rollback.md` (5-overlay `DC_ARGS`, tag a rollback point, take
  a backup, **stamped** build, assert the stamp, `up -d`, smoke).
- Design gaps **G1** (mobile), **G2** (icons), **G4** (touch targets), **G7** (three light Badge
  variants fail AA: success 2.73:1, warning 4.25:1, error 4.13:1) — all need an operator ruling
  (`brandkit/` is the operator's, D-071).
- Roadmap §2.1, §2.6 (decision first), §2.7 (date-gated ≥2026-07-23), §2.12, §2.16, §2.17.
- Coverage the audit flagged but that was out of scope here: the Reports **Schedules** tab is
  never activated by any test, and no test drives the Probes **create** happy path.
