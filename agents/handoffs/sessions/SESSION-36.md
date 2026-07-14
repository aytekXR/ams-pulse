# SESSION-36 — planned at S35 close (D-097)

> Written by SESSION-35 close (2026-07-14). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146`. Read `RESUME-PROMPT.md` + `ROADMAP-V2.md` §2 + the final-assessment §5
> roadmap before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-37

Before dispatching: re-read ROADMAP-V2 §2 and the final-assessment §5 roadmap and **revise this
plan if a higher-leverage move exists.** This file is a starting point, not a contract. Record any
revision in the D-098 open block.

**S35 exercised exactly that clause and it was the right call** — it discarded S34's plan (e2e
gaps + §2.16) after the operator's question exposed three ship blockers that outranked everything
on the list. Do the same if the evidence points somewhere else.

## ⛔ At open — verify, do not assume

**★★ STANDING RULE (D-095): a session claiming "DONE" is NOT evidence that it landed.**

- `git log --oneline origin/main -3` — S35 (`425b04b`, D-097, PR #51) should be on `origin/main`.
- Prod should print **`v0.4.0-11-g425b04b`**:
  ```sh
  sg docker -c "docker exec pulse-prod-pulse-1 /usr/local/bin/pulse version"
  ```
- **Prove the S35 fix is still live** (this is the cheapest real check in the repo):
  ```sh
  curl -s -o /dev/null -w "%{http_code}\n" --resolve beyondkaira.com:443:161.97.172.146 \
    "https://beyondkaira.com/api/v1/reports/export?from=1&to=2&format=csv"
  # 401 = route exists (correct). 404 = the S35 rollout did not survive; re-run the runbook.
  ```
- Re-check the operator queue **live** (do not trust the doc): GHCR anonymous pull, licence expiry.

## 🔧 Environment gotchas — read BEFORE running any gate

**Playwright cannot run bare-metal here** (`libatk-1.0.so.0`, `libgbm`, `libasound`; no sudo):
```sh
cd web && pkill -f 'vite preview'   # CI=1 disables reuseExistingServer; free 4173 first
sg docker -c "docker run --rm --network host -v \$PWD:/work -w /work \
  -e CI=1 mcr.microsoft.com/playwright:v1.61.1-noble npx playwright test"
# S35 census: 60/60
```

**Go cannot run bare-metal here either** (root-owned ClickHouse leftovers make `./...`
untraversable):
```sh
sg docker -c "docker run --rm -v \$PWD:/src -w /src/server -e GOFLAGS=-buildvcs=false \
  golang:1.25 go test ./..."      # S35 census: 24/24 packages, exit 0
```

**`sleep` in the foreground is BLOCKED** by the harness. Do not put it in a gate command — it
kills the whole invocation (exit 144). S35 lost two runs to this.

## Mission

**The product's remaining distance to a first sale is almost entirely NOT engineering.**

> ### The two operator items still outrank everything a session can do.
> **GHCR is private** (anonymous pull → 401; kills the one-command quickstart for every user —
> one click to fix). **The AMS licence expires 2026-07-27T13:45Z.** From ~07-25 the licence
> outranks GHCR: a lapse **plus** the next `antmedia` restart = **total ingest death** (D-092/D-093).
>
> Surface both, every time. **No amount of session work substitutes for either.**

## S36 candidates — pick by leverage

1. **§2.16 AMS operational early-warning** [S–M, **OPERATOR-APPROVED already**, D-086 addendum] —
   the largest *approved, unblocked* feature left, and now the strongest candidate: S35 cleared
   the ship blockers that were ahead of it. **Start here unless the evidence says otherwise.**
2. **The e2e gaps S34's audit named and S35 did not close** [S] — the Reports **Schedules** tab is
   never activated by any test, and no test drives the Probes **create** happy path. Both are real
   holes in freshly-changed code.
3. **The residual honesty items S35 surfaced but did not fix** [XS–S each]:
   - Scheduled report artifacts have **no download endpoint** — a user without filesystem or S3
     access cannot retrieve a report Pulse generated for them. The Schedules UI still offers PDF
     as a format, but `/reports/export` returns **501** for PDF (LIM-24). That is a coherent
     seam today, but it will confuse someone.
   - `PULSE_LICENSE_OFFLINE_FILE` is read by `internal/config/config.go` but `config.Load()` is
     **never called at runtime** (`main.go` uses a `loadEnvConfig()` shim — `HOOK(BE-02)`). The
     working variable is `PULSE_LICENSE_FILE`. The docs now say so, but the dead code path should
     be reconciled.
4. **§2.7 CI job promotions** — **date-gated ≥ 2026-07-23.** Not yet. (`web-e2e` and `csp-e2e`
   still carry `continue-on-error: true`, so a red e2e does not block a merge today.)
5. **§2.6 unsigned-webhook ingest mode** — **OPERATOR DECISION FIRST (D-V2-1).** Do not design or
   build. Re-surface only.
6. **§2.12 Mobile SDKs** [L per platform] — do not start without an explicit operator call.

**Not candidates:** §2.3 (licensegen — done, S34 ledger correction), §2.19 (uipro — COMPLETE and
live), §2.1 `enforce_admins` (RESOLVED-deferred, S10/D-068).

## ⚠️ Binding lessons — carry into every wave

1. **★ RUN the doc; do not read it.** Every S35 blocker had passed prior review. Reading a doc
   tells you it is *plausible*. Executing it tells you it is *true*. This one lesson produced
   every real finding of the session.
2. **★ A gate you cannot point at in the repo is not a gate.** S35's own prompt asserted a
   "§2.2 hex-literal grep"; an agent reported it RED with 35 matches; **no such gate exists**.
   Trusting it would have mangled nine files to satisfy an imaginary rule. Verify the rule exists
   before obeying a red. (Likewise: `git diff --exit-code` reads RED against this repo's
   permanently-dirty tree — scope drift checks to `schema.d.ts`.)
3. **Adversarially verify every finding.** 36 raw → 33 confirmed, **3 refuted**. One auditor cited
   a file that does not exist and produced line-numbered "evidence" from it. Without the
   refutation pass, that ships.
4. **Sub-agent synthesis hallucinates confidently.** S35's synthesizer invented
   `licensegen --duration 365d --out customer.sig`. The real flags are `-tier -privkey -expires
   -expires-minutes`. **Check the source, not the summary — especially your own agents' summaries.**
5. **A missing button beats a button that 404s.** PDF export was removed, not faked.
6. **An absence-assertion is meaningless without its positive counterpart**, and **a gate test must
   stub the data the gate is hiding** (D-096).
7. **jsdom stubs `window.confirm`**; **never add ARIA the code cannot honour** (D-096).
8. **Verify a mutation LANDED before trusting a RED or a GREEN.** S35 asserted the mutation string
   was present before running.
9. **Two install paths, two fates.** Clone-and-build works; the GHCR quickstart is dead. Never
   flatten them into one verdict again.
10. **Check the body, not the status code** — an expired licence key activates with **200**.

## Gates (before any commit)

- Web: lint + `tsc --noEmit` + build + `npx vitest run --coverage` (floors 59/54/45;
  **S35 census: 626 tests / 37 files**) + **Playwright in docker** (**60/60**).
- Contract drift: `npm run gen:api` then check **`schema.d.ts` only** (not `git diff --exit-code`).
- Any Go change: full suite in `golang:1.25` docker (**24/24**), `vet`, `gofmt`.
- WCAG re-check on changed components. **Trust `web/src/styles/__tests__/wcag-tokens.test.ts`,
  which recomputes every ratio from `tokens.json` — not the table in `design-rationale.md`** (G5
  proved one of its rows was simply wrong).
- **`brandkit/` is byte-untouched unless the operator has ruled (D-071).** G3/G5/G6 applied;
  **G7 is NOT approved** — report, do not self-approve.
- `docs/marketplace/` stays DRAFT-INTERNAL (D-081).
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** — bcrypt hash, public repo, operator ruling
  pending. **It is the only expected dirty file. A CLEAN `git status` is a FAILURE signal.**
- **NEVER** `git reset --hard` / `git checkout -- .` / `git stash` / `git clean` (D-096).
- **Never restart or "fix" AMS** — observe-only.

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — and VERIFY the merge landed.**
2. `decisions.md` **D-098** evidence — append EARLY, not at the end.
3. RESUME-PROMPT ▶ START HERE → SESSION-37; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-37.md` (carry the standing-directive header).
6. **Roll prod forward** if the session changed server or web code, per
   `deploy/runbooks/upgrade-rollback.md` — and smoke with **evidence**, not the compose
   "Healthy" label.
