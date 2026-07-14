# SESSION-35 — planned at S34 close (D-096)

> Written by SESSION-34 close (2026-07-14). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146`. Read `RESUME-PROMPT.md` + `ROADMAP-V2.md` §2 + the final-assessment §5
> roadmap before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-36

Before dispatching: re-read ROADMAP-V2 §2 and the final-assessment §5 roadmap and **revise this
plan if a higher-leverage move exists.** This file is a starting point, not a contract. Record any
revision in the D-097 open block.

## ✅ OUTCOME — S35 is CLOSED (D-097, PR #51 `425b04b`, prod `v0.4.0-11-g425b04b`)

**The plan in this file was DISCARDED, deliberately, under the standing directive's own
"revise this plan if a higher-leverage move exists" clause.**

This file had planned: *close two e2e gaps, then build §2.16*. Instead the operator asked
*"have you finished all development? is installation and generating license keys ready?"* — and
the only honest way to answer was to **execute** the docs rather than read them.

**The answer was no, on both counts.** 42 agents ran every documented command against a live
system; 36 raw findings went through an adversarial refutation pass → **33 confirmed, 3 refuted**.
**3 blockers, 10 majors.** Everything a session was permitted to fix, S35 fixed and shipped.

### What was actually broken (full evidence: D-097)

1. **`GET /api/v1/reports/export` did not exist** — yet the Reports page shipped **Export CSV**
   and **Export PDF** buttons wired to it. A paying Business customer clicked Export and got a
   **404**. A *missing feature*, not a doc bug. **CSV implemented; PDF removed rather than left
   broken (LIM-24).**
2. **The analytics Export CSV button had never worked either.** It put `?token=` in the URL and
   navigated — but `bearerAuthMiddleware` **deliberately ignores** `?token=`
   (`TestTokenInURL_Ignored`), so it answered **401**. Downloads now authenticate via the
   `Authorization` header and a blob, which also keeps the token out of logs and browser history.
3. **`docs/licensing.md` documented an activation API that does not exist** — `POST
   /api/v1/license/activate` vs the real **`PUT /api/v1/admin/license`**. Wrong path AND wrong
   method, under a heading titled *"Verify activation."* Verifying a key you had just **sold**
   returned 404.
4. **An expired key returns `200`, not `422`** — check the body, not the status code.
5. **`make up` / `docker compose up -d` — install.md's own primary command — always failed**
   (`pulse-migrate` had no `PULSE_SECRET_KEY`).
6. **The README Quick Start silently monitored a MOCK AMS** — a customer could set their real AMS
   address and see no data and no error.
7. `prometheus.md` had a fabricated metric table and the wrong tier; `probes.md` told **Business**
   customers they had no probes.

### Two prior-session claims corrected

- **"No customer can install Pulse" was overstated.** Clone-and-build never touches GHCR and
  **works**. Only the **quickstart** is dead. Two paths, two fates.
- **The vendor key ceremony is DONE** (S16/D-077) — it had been carried as an open operator
  blocker. Redoing it would invalidate every outstanding key.

### A gate that did not exist

This session's own gate prompt asserted a "§2.2 hex-literal grep"; an agent reported it **RED**
with 35 matches. **No such gate exists anywhere in the repo.** The matches are legitimate test
assertions. Trusting the report would have mangled nine files to satisfy an imaginary rule.
**Verify a rule exists before obeying a red.**

### Gates + rollout

vitest **626/626** · Playwright **60/60** · Go **24/24** · lint/tsc/build clean · CI **15/15** ·
prod rolled to `v0.4.0-11-g425b04b` and smoked with **evidence**: `/api/v1/reports/export`
returns **401** where it returned **404** an hour earlier, and authenticated returns **200
`text/csv`** with a real header row. `Caddyfile.prod` intact (7 matbu/evrak lines), in no commit.

### Operator queue after S35 — unchanged, but sharper

**GHCR private** (one click; kills the quickstart) and the **AMS licence expiring 2026-07-27**
(13 days) still outrank everything a session can do. See `docs/operator-expected.md`.

**➡ Next session: `SESSION-36.md`.**

---

## 📌 The original S35 plan (superseded — kept for provenance)

## ⛔ At open — verify, do not assume

**★★ STANDING RULE (D-095): a session claiming "DONE" is NOT evidence that it landed.** S33 opened
to find S32's PR still open AND its branch missing a file its own gates had run against. So:

- `git log --oneline origin/main -3` — S34 (`a01aaea`, D-096) should be on `origin/main`.
- `sg docker -c "docker compose -p pulse-prod … exec -T pulse /usr/local/bin/pulse version"` —
  should print **`v0.4.0-8-ga01aaea`**. If it prints `v0.3.0-34-g58a9c84`, the S34 rollout did not
  survive; re-run `deploy/runbooks/upgrade-rollback.md`.
- Re-check the operator queue live (do not trust the doc): GHCR anonymous pull, license expiry.

## 🔧 Environment gotchas — read BEFORE running any gate

S34 wasted real effort re-solving both of these. Do not repeat it.

**Playwright cannot run bare-metal here** (`libatk-1.0.so.0`, `libgbm`, `libasound` missing; no
sudo). Use the image — this is the sanctioned path and how S33/S34 got their greens:
```sh
cd web && pkill -f 'vite preview'   # CI=1 disables reuseExistingServer; free 4173 first
sg docker -c "docker run --rm --network host -v \$PWD:/work -w /work \
  -e CI=1 mcr.microsoft.com/playwright:v1.61.1-noble npx playwright test"
# S34 census: 60/60
```

**Go cannot run bare-metal here either.** Root-owned ClickHouse leftovers (`internal/*/access`,
`internal/*/preprocessed_configs`, Jun 30, gitignored, no sudo) make `./...` untraversable. Use:
```sh
sg docker -c "docker run --rm -v \$PWD:/src -w /src/server -e GOFLAGS=-buildvcs=false \
  golang:1.25 go test ./..."      # S34 census: 24/24 packages ok
```
(A bare `go` binary does exist at
`/home/aytek/.cache/pre-commit/repoiavouv2x/golangenv-default/.go/bin` — fine for a single
package, useless for `./...`.)

## Mission

**§2.19 is COMPLETE and now LIVE in prod. Prod is current. Do not plan another UI wave** — the
only remaining UI work is operator-gated (G1, G2, G4, G7).

The product is **not shippable**, and the reason is almost entirely **not engineering**:

> **The single blocker that matters is that GHCR is private.** Anonymous
> `docker pull ghcr.io/aytekxr/ams-pulse` → **401**. Until the operator flips it, no customer can
> install Pulse and every install doc is fiction. **No amount of session work substitutes for
> this.** Surface it first, every time.
>
> **And the clock: the AMS license expires 2026-07-27T13:45Z.** From ~07-25 this outranks
> everything, GHCR included. Lapse + next `antmedia` restart = total ingest death (D-092/D-093).

## S35 candidates — pick by leverage

1. **The e2e gaps S34's audit named but did not close** [S] — the Reports **Schedules** tab is
   never activated by any test, and no test drives the Probes **create** happy path. Both are
   real holes in freshly-changed code. Cheapest genuine risk reduction available.
2. **§2.16 AMS operational early-warning** [S–M, OPERATOR-APPROVED already, D-086 addendum] — the
   largest *approved, unblocked* feature left. Strongest candidate if you want to build.
3. **§2.17 anomaly/fleet honesty tail** [XS–S each] — items 2 and 3 are already done (S28); check
   what actually remains before scoping.
4. **§2.7 CI job promotions** — **date-gated ≥ 2026-07-23.** Not yet. Skip carry ×24.
5. **§2.6 unsigned-webhook ingest mode** — **OPERATOR DECISION FIRST (D-V2-1).** Do not design or
   build. Re-surface only.
6. **§2.12 Mobile SDKs** [L per platform] — large; do not start without an explicit operator call.
7. **§2.1 enforce_admins** — RESOLVED-deferred (S10/D-068); would deadlock all session pushes
   while the repo has one human collaborator. Leave it unless the operator adds a reviewer.

**Not a candidate: §2.3.** S34 corrected the ledger — `qa/licensegen` already has
`-privkey`/`-expires`/`-expires-minutes` and the tests pass. Only the **vendor key ceremony**
remains, and that is the operator's.

## ⚠️ Binding lessons — carry into every wave

1. **Check the evidence before asserting the conclusion — including when the conclusion is "the
   agent lied."** S34 accused its sub-agents of faking test runs; the transcripts proved they had
   not (one had legitimately worked around the missing Chromium libraries). Read the transcript.
2. **Read the gates section BEFORE running gates.** Both S34 environment gotchas above were
   already documented; improvising cost real time.
3. **An absence-assertion is meaningless without its positive counterpart.** "Cancel fires no
   DELETE" passes just as well when delete is *entirely broken*. S34 had to add the "confirm
   really does fire the DELETE" half to three specs.
4. **A gate test must stub the data the gate is hiding.** S34's Probes tier-gate test stubbed no
   probes — so deleting the gate would render no table and the test would still pass. It was
   measuring the absent stub, not the gate.
5. **jsdom stubs `window.confirm`.** A whole class of destructive-action bug hides there. Only a
   real browser sees it.
6. **Never add ARIA the code cannot honour.** `role="dialog"` promises focus-in and Escape;
   `role="tab"` + roving tabIndex with no key handler makes tabs *keyboard-unreachable*.
7. **A className is a CONTRACT with the stylesheet** — `web/src/styles/__tests__/focus-rings.test.ts`
   pins both halves. Any new bare styling className must be added to its map.
8. **`#FF5C68` is NOT in `CHART_COLORS`** — it is `--color-error`. `CHART_COLORS[3]` is pink.
9. **Recharts `var()` rule is SCOPED** to data-series props (`<Line>`/`<Area>`/`<Bar>`
   stroke/fill). `var()` IS correct on plain `<svg>` and chart chrome. A broader gate makes the
   product worse — S33 caught an agent breaking working code to satisfy exactly such a test.
10. **Verify a mutation LANDED before trusting a RED or a GREEN.** `perl -0pi` without `/g` once
    hit a doc comment instead of the JSX and reported a false green.

## Gates (before any commit)

- Web: `npm run gen:api && git diff --exit-code` (drift) + lint + `tsc --noEmit` + build +
  `npx vitest run --coverage` (floors 59/54/45; **S34 census: 619 tests / 36 files**) +
  **Playwright in docker** (recipe above; **S34 census: 60/60**).
- **The §2.2 hex grep scans WHOLE FILES — no hex literals in comments** under `src/features/`.
- WCAG re-check on changed components. `design-rationale.md` §2 is binding, **but G5 proved one of
  its rows was simply wrong** — `web/src/styles/__tests__/wcag-tokens.test.ts` now recomputes every
  ratio from `tokens.json` on each run. Trust the test, not the table.
- **`brandkit/` is byte-untouched unless the operator has ruled (D-071).** G3/G5/G6 were approved
  and applied; **G7 is NOT approved** — report, do not self-approve.
- Any Go change: full suite in `golang:1.25` docker (recipe above), gofmt, vet, contract-drift.
- `docs/marketplace/` stays DRAFT-INTERNAL (D-081).
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** — bcrypt hash, public repo, operator ruling
  pending. It is the only expected dirty file at open.
- **Never restart or "fix" AMS** — observe-only.

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — and VERIFY the merge landed.**
2. `decisions.md` **D-097** evidence — append EARLY, not at the end.
3. RESUME-PROMPT ▶ START HERE → SESSION-36; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-36.md` (carry the standing-directive header).
