# Pulse — Resume / handoff prompt (SINGLE source of truth)

> **This is the one handoff doc.** Update THIS file + `decisions.md` every session; never
> create a second handoff file, and never stack past sessions here (operator directive).
>
> Pulse = self-hosted analytics/QoE/alerting for Ant Media Server. Repo:
> `/home/aytek/repo/ams-pulse` on the operator's VPS.
>
> - Decision log (binding): `agents/handoffs/decisions.md`
> - Per-session detail: `agents/handoffs/sessions/SESSION-NNN.md`
> - Plan of record: `agents/handoffs/ROADMAP-V2.md`
> - Operator queue: `docs/operator-expected.md`
> - AMS integration facts: `docs/AMS-INTEGRATION.md`
> - Go-live + rollback: `deploy/runbooks/real-ams-go-live.md`
> - Operator creds/keys (gitignored, NEVER commit): `oguz-testing.md`

---

## ▶ START HERE — current state

> **This file carries only current, forward-looking state.** No session history, no superseded
> blocks, nothing struck through. How we got here lives in
> `agents/handoffs/sessions/SESSION-NNN.md` and `decisions.md` (operator directive).
> **Replace this block each session — never append to it.**

**Where the product is:** **v0.4.4 is the release; `v0.4.5` is the submission target once cut.**
S117 (D-185) executed external review **round 11** — the reviewer's *second* declared final round,
and a genuinely adversarial one: it opened by retracting round 10's convergence call, verified
D-184's three fixes against the code, filed one finding (N-01), and owned three errata precisely,
including the mirror-bridge retraction that ends the three-round `review-chain` dispute.

**N-01 was confirmed in mechanism, refuted in reachability, and RAISED in severity.** `amsclient`
really was the last outbound client with no `Control` hook — but the filed chain (an API-created
`ams_sources.rest_url` being polled) does not exist: the poll client is built once from
`PULSE_AMS_URL`, and that column reaches only the already-guarded connectivity test. What *is*
real is the response side, which the ledger's own LOW rating had ruled out: `CheckRedirect` follows
ten hops to a `Location` **the responder chooses**, and a non-2xx body (4 KB) was copied into the
poll error that unauthenticated `/healthz` republishes. So an ordinary AMS 401/500 page was being
echoed to any caller who could reach the port, SSRF or not. Both closed in D-185.

**Auditing the reassurances outproduced the finding for the third round running** — four more
items: the beacon identity fields were reversed off M-04's deferral (rejection, unlike truncation,
does not corrupt `uniq()`, and the deferral had been priced without a ~50× write amplification);
the advertised tier prose and the code disagree about whether anomaly detection is Enterprise-only
(the loop enforced the prose, e2e A5 refuted it — see the lesson below, and the operator item); and
D-184's `doc-stamps` guard was never added to `branch-protection.sh`, so the class L-02 belonged to
was reported-on but not actually blocked.
`main` now carries the round-7 through round-11 fixes, five of them behavioural.

**The review loop is closed by decision, not by proof.** Rounds 6→11 ran H(9) → I(6) → J(3) →
K(2) → L(2, both refuted) → N(1, reachability refuted). But the loop's own audits found 3 (D-184)
and 5 (D-185) items inside the rounds that declared themselves clean. **If a round 12 happens,
spend all of it on the exculpations and on enumerating sets the reviewer describes as lists.**
Remaining product risk needs a live lab — a pentest for the dials, a 2-node cluster for LIM-10, a
Kafka broker for LIM-19 — not another read-through.

**One thing blocks submission, and it is the operator's:** rotate `CLICKHOUSE_PASSWORD`. A
32-hex prefix of the live value is in public git history since `98b011c`. Deliberately deferred
when the v0.4.3 cut was authorised — a recorded decision, not an oversight. **Re-checked S117:
still un-rotated** (live prefix still matches 2 commits). Eleven rounds in, both the reviewer and
the loop agree it is the only remaining gate. The submission-day motion is one movement:
**rotate (G-02), cut v0.4.5, submit against it** — that single cut also clears the tag-frozen
prose items and ships the behavioural breakdown cap and the D-185 security fixes.

**Do first, every session:**
1. **Gate reads** — prod health (component-scoped `/healthz` + a ClickHouse count), git/PR
   drift, and whether the operator rotated `CLICKHOUSE_PASSWORD` (check silently: compare the
   live value's first 32 chars against `git log -S`, never print the secret).
2. **If a new review arrives** — verify every claim against the code BEFORE fixing anything,
   and check the reviewer's assumptions about the tree state as carefully as their findings.
   This has caught errors in both directions — ours and the reviewer's. To commission a review,
   hand over `docs/assessment/EXTERNAL-REVIEW-PROMPT.md` verbatim (blackbox → docs → code; one
   output file whose Disposition column you fill and feed back as the next round's input).
3. **If a 2-node cluster appears** (the operator's PAYG load-lane item) — the deferred cluster
   work (LIM-10: node alerting during an AMS API outage) becomes fixable with verification
   instead of guesswork. Highest-value technical unblock.

**Test the artifact, not the documentation (S111's lesson, re-earned twice in S112).** D-178's
worst finding was invisible to every doc review: the published `cosign verify` command **fails
on a cosign v2 client** because releases from `v0.3.0` on use the OCI 1.1 referrer layout, not
the legacy `.sig` tag. When a claim names a command a third party will run, **run it as that
third party would**, on the published artifact, with a client version you did not choose. Do not
change how releases are signed to accommodate old clients — that is deliberate (`release.yml`).
S112 applied the rule again: the anchored cosign regexp was run against the published `0.4.3`
(and its negative control), and the installer's new exit code was clean-room tested in both
directions rather than reasoned about.

**Re-test the generalization, not just the premise (S112's lesson).** LIM-01 rested on a
verified fact — `/rest/v2/system-status` omits CPU/mem/disk — and an *inferred* conclusion,
"therefore AMS cannot report it on standalone", which nobody ever probed. It was wrong for the
product's entire life: a sibling console endpoint had the data all along. When a limitation says
"the platform cannot do X", check whether what was actually tested was "this one endpoint does
not do X". Sibling endpoints cost one curl.

**Tag/main parity:** `v0.4.4` closed the gap that H-02 flagged, and round 7 confirmed the
pattern is broken — post-tag drift is now internal-only. Keep it that way: **the moment a fix an
evaluator would meet lands on `main`, it belongs in a release**, because "it rides the next one"
is exactly the ruling round 6 had to overturn. `main` currently carries the round-7 prose/comment
fixes, which genuinely do ride the next cut (the reviewer's own recommendation).

**Verify the exculpations, not just the accusations (S114's lesson).** Round 8's most valuable
sentence was not a finding — it was a reassurance: *"the sibling `GeoBreakdown` is shape-identical
but domain-bounded (~250 countries), so it is fine."* It was wrong (`?region=true` switches the
grouping to `geo_country, geo_region`), and it was hiding a defect strictly larger than the one
actually filed. **A reviewer's "this one is safe" is the one place nobody looks twice.** The same
round, the filed MEDIUM turned out to be bounded at 448 rows once the enum mapping was read.

**Apply "verify, don't trust" to our own code comments (S114).** Our J-02 fix shipped with an
in-code proof that `tail = total − Σ(capped)` was exact because the groups are disjoint.
Disjointness is necessary but not sufficient: `uniq()` is an approximate aggregate and the totals
query is a second round trip. It emitted `uniques=-20`. **Both the authoring lane and its
adversarial verifier certified that code SOUND** — an in-code proof is a claim to test, exactly
like a changelog entry. Reproduce numerically before believing any arithmetic argument.

**A reviewer's claim about OUR tree state is the cheapest thing to verify — and round 11 closed
this one from the other side.** J-03, K-01 and L-01 all reproduced from an untracked
`external-review-…-round6.md` that has never existed here. S116 diagnosed the cause (their device
bridge is attached to a *mirror*: its writes land there, report success, and never arrive), and
round 11's errata E-1 **confirms it in the reviewer's own words and retracts the finding**.
**Chat text reaches this repo, bridge writes do not** — if a future round offers files, ask for the
text inline. Every one of those findings was still right about its defect: disposition the
conclusion and the evidence separately, always.

**Verify the exculpations — this is now the highest-yield move in a review round (S116's lesson,
and the strongest form of S114's).** Round 10 filed two findings, both refuted. Auditing the
*reassurances* in the same round produced three real defects. What failed was never the reviewer's
analysis, it was the **generalization step**: "the webhook channel wires the guard" became
"operator-controlled outbound URLs are guarded"; "the beacon package truncates fields" became
"beacon ingest truncates fields". Both true of the file read, false of the system. When a review
says an area is clean, ask which file they read and then go and find its siblings.

**A subagent lane's self-report is not a gate (S116).** All three TDD lanes produced sound logic and
honest red→green evidence — and shipped a literal `(D-xxx finding)` placeholder, a **fabricated
finding id** (`K-03`, when round 10's prefix is `L-` and the item was maintainer-found `M-03`), a
wrong session number, and two docstrings claiming a guard was installed in a constructor when the
dialer is built per call. Read every diff yourself and correct provenance centrally before
committing; lanes invent plausible-looking ids when you do not hand them one.

**Test the guard you write to enforce testing (S116).** `check-doc-stamps.sh` shipped two bugs that
only a negative test exposed: it diffed `BASE...HEAD`, so uncommitted edits were invisible and a
local pre-commit run reported PASS no matter what had changed; and its `^`-anchored pattern was
matched against diff lines still carrying `+`, so it fired on files whose stamps *had* moved. Run a
new check in **both** directions — make it fail on purpose — before trusting a PASS.

**Guard scope is a decision; scope gaps are where drift lands (S113's lesson, re-earned in S114
and again in S115 — four rounds running).** S115's instance was the worst yet: D-181 corrected
the stale "standalone AMS never reports `cpu_pct`" claim at eight *code comment* sites and swept
no *documentation*, so `kafka-integration.md` spent two more rounds contradicting itself inside
twenty lines and `compatibility.md`'s customer-facing matrix kept saying "Via Kafka only". **Write
what a sweep did NOT cover into the decision entry** — otherwise the next reviewer writes it for
you.
Round 8's sweep found round 7's *own* fixed classes still live in two unswept files —
`submission-package.md` carried I-01's stale range **inside the marketplace submission document**,
and `install.md` carried I-02's chart-version contradiction. Both round-7 fixes were file-scoped.
Third consecutive round where drift landed in a known scoping gap. Guard #17 was
deliberately narrowed to runnable `ams-pulse:<semver>` pins to avoid false positives — correct,
and the exact reason the README's prose pointer shipped one release stale *inside* the new
submission target hours later. New check **#19** closes that. When you narrow a guard, write down
what the narrowing leaves uncovered. And **de-literalize whatever drifts twice**: the cosign
version range and the chart-version sentence were both "just bump the number" the first time.

**S116 closed the doc-stamp half of this class mechanically, after four rounds of fixing it by
hand.** Stamps are now date-valued (a session D-number cannot be interpreted by any reader outside
this repo and cannot distinguish "content changed" from "someone bumped the stamp"), and
`.github/check-doc-stamps.sh` + the `doc-stamps` CI job enforce it. On its first run the checker
found **four instances the same session's careful manual sweep had missed**. That is the argument
for mechanizing a class instead of sweeping it again: the fifth hand-sweep would have missed them
too. The S101 beacon divergence (M-02) is the same shape and is now shared-code rather than
duplicated — prefer *making divergence impossible* over *checking for divergence*.

**The e2e corpus is the behavioural contract — check it BEFORE writing a gate, not after CI says no
(S117, and the second time this exact lesson has been paid for).** D-185 enforced the advertised
"anomaly detection is Enterprise-only" tier table at three points, with unit tests, a negative
control, a green `-race` suite, and an adversarial workflow lane that certified it SOUND after
asking exactly the right question ("which tiers now stop computing baselines?") and checking the
answer against the tier table. **e2e A5 then failed:** it mints a *Business* licence, POSTs an
anomaly rule expecting 201, and asserts the alert fires — green for many sessions. The gate was
reverted and the contradiction filed for the operator. **Documentation is not the contract; the
live scenarios are.** When a change would newly REFUSE something, grep `.github/workflows/e2e.yml`
and `qa/` for that request shape first — it costs one grep and it is the only artifact that knows
what the product actually accepts. Over-rejection is as much a regression as under-rejection.

**Enumerate the set; a reviewer's list is not the set (S117's lesson, and the sharpest form of
"verify the exculpations").** Round 11 wrote *"prober ✓ / reports ✓ / alerts ✓ are now uniform"* —
three true checks and a false universal. The anomaly detector is a fourth tier-gated background
loop and was ungated. The same shape produced M-01 (one guarded client → "the class is guarded")
and M-02 (one guarded ingest path → "beacon ingest is guarded"). **When a claim names N things and
implies all things, go and count.** The corrective is cheap: `grep` for the pattern, not for the
names you were given.

**A test that cannot fail is worse than no test (S117).** `TestClient_IgnoresProxyEnv` asserted the
transport ignores `HTTP_PROXY` by watching an httptest proxy for hits — and passed identically with
the fix reverted, because Go's `ProxyFromEnvironment` never proxies loopback, so the proxy could
never have been hit either way. It looked like coverage and asserted nothing. **Re-run every new
test against a reverted tree** (copy to `/tmp` inside the container; never edit the repo to do it)
and delete or rewrite whichever ones still pass. This is the only way to find a vacuous assertion.

**A guard job without a branch-protection context is advisory (S117).** D-184 closed a four-round
drift class "mechanically" with the `doc-stamps` CI job, but never added it to
`.github/branch-protection.sh` — so it reported and could not block a merge. **Add the context in
the same change that adds the job**, and remember the `gh` token here has repo-admin, so the PUT is
executable from this box (D-162 precedent) rather than something to queue for the operator.

**Standing rules learned the hard way:**
- **After a squash merge, wait for main's post-merge CI before tagging.** The PR's green checks
  belong to the pre-squash SHA; the release pipeline's CI gate requires a successful run for
  the commit being tagged. Tagging early fails the gate (cleanly — nothing gets published).
- **Dry-run the version guard BEFORE tagging** (learned D-180). Extract the
  `Version consistency guard` step out of `release.yml`, create a throwaway local tag, and run
  it: `GITHUB_REF_NAME=vX.Y.Z bash guard.sh`. It caught a real miss on the first pass. The 18
  checks cover: `VERSION` · `Chart.yaml` appVersion · doc header stamps
  (`product.md`, `faq.md`, `known-limitations.md`, `submission-package.md`) · SDK
  `package.json` + lock · Swift SDK constant · deploy-surface image pins · helm `values.yaml`
  + README table + `tests/values-*.yaml` · `install.sh` `PULSE_IMAGE` **and** `PULSE_REF` ·
  root README pins · `listing.md` version mention · beacon tarball names · customer-doc image
  pins · runnable OCI-chart pins. **Bump the chart semver whenever `templates/` or
  `values.yaml` changed** — `helm push` overwrites a published chart version — and regenerate
  goldens with helm **3.17.0**.
- **Mount the repo root for every container gate**, Go and Node alike — tests reach into
  `contracts/`. A subtree mount produces fake failures that look like regressions.
- **Regenerate helm goldens with CI's pinned helm 3.17.0**; newer helm injects blank lines and
  fakes golden drift.
- **Don't guess at cluster fixes.** They retune alert timing and there is no live cluster to
  verify against; trading a missed alert for a false one is not an improvement. Disclose in
  LIM-10 instead.
- **Pin ShellCheck to CI's version when touching `deploy/` scripts.** CI installs Ubuntu's
  **0.9.0**; `koalaman/shellcheck:stable` is 0.11.0 and renumbered the reachability check
  (SC2317 → SC2329). A change can be clean on `:stable` and red in CI. Run both.
- **When a review proposes a fix, verify it is implementable before executing it.** Round 6's
  H-09 asked for a GHCR tag delete that would have deleted the release digest. The finding was
  right and the remedy was wrong; both go in the disposition.

**Open engineering debt (loop-owned, non-blocking):** cluster node-alerting rework (LIM-10,
waits on a real cluster) · verify the `ClusterNodeDTO.lastUpdateTime` unit against AMS source ·
surface `stream_ingest_error` (LIM-27) · thread the owning node through per-app polling
(LIM-28) · poller/discovery cadence consolidation · helm NetworkPolicy golden · the
`SettingsPage` ARIA test flake under parallel execution · **alert retry loop does not re-check the
entitlement gate between attempts** (~5 s window; the sync gate closes it within one interval —
D-185 recorded it rather than adding a licence read inside a retry loop) · **`check-doc-stamps.sh`
validates stamps inside fenced code blocks** (a false *positive*: it can fail CI on a doc showing an
example stamp, never pass a real stale one) · **release: switch the candidate push
to buildx `push-by-digest=true`** so promoted images stop carrying a public `candidate-<sha>`
alias (round 6 H-09 — the correct fix, deferred because it changes the publish mechanism and
only a real tag exercises it; reasoning is recorded in `release.yml`) · **probe whether AMS's
`ams-webrtc-stats` shape can restore per-stream FPS** now that the console-endpoint assumption
behind LIM-01 turned out to be wrong (LIM-04 rests on a similar inference).

**Operator queue:** `docs/operator-expected.md`. **How we got here** (read only if you need it):
`decisions.md` · `agents/handoffs/sessions/` · `docs/assessment/`.

---
## 1. CURRENT STATE (verified facts — refresh each session, never let this go stale)

- **Shipped product, pre-marketplace.** All 10 PRD features implemented and live-validated
  against a real AMS 3.0.3 Enterprise (46/50 scenarios). **Latest release: v0.4.4** (2026-07-27,
  the marketplace submission target). AMS 3.0.3 is still the latest AMS release.
- **Production** runs behind host nginx on this VPS at `https://pulse.beyondkaira.com`, against
  the operator's own `antmedia` container (AMS Enterprise 3.0.3, `--network host`). It is on the
  stamped **v0.4.0-139** build — rolling prod forward is deliberate and operator-gated, never
  automatic. Health at last check (S117, 2026-07-28): all three `/healthz` components `ok`,
  **1,340,393** server events, ingest steady at 720/h, collector actively ingesting.
- **`main` is protected** (strict, 1 review, `enforce_admins=false` so owner pushes work; **15**
  required contexts since D-185 added `shellcheck` and `doc-stamps` — a guard job that is not in
  that list cannot block a merge). Work on a branch → PR → merge on green.
- **Known limitations are disclosed, not hidden:** `docs/known-limitations.md` carries 28
  entries. **LIM-01 was closed in D-179** (standalone CPU/mem/disk now work without Kafka) and
  rewritten down to a memory-threshold calibration note rather than deleted. LIM-10 (cluster) is
  the significant remaining one — AMS 3.x exposes no node role or version, so
  all nodes display as `origin`, edge/origin viewer dedup is inert, and node alerting during an
  AMS API outage is not fully reliable. All deliberate and written down.
- **Operator-gated items** (never do these autonomously): secret rotation · marketplace listing
  submission · billing · the Ankush reply · the PAYG load lane · prod rolls. Queue:
  `docs/operator-expected.md`.

---

## 2. TDD ENFORCEMENT (BINDING — bias toward test coverage over implementation speed)

**Every change follows red→green→refactor: write the failing test FIRST, watch it fail, implement, watch it pass.**
For each unit of work produce tests at ALL applicable levels (do not stop at "unit"):

| Level | What it asserts | Where |
|---|---|---|
| **Unit** | pure logic, table-driven, both branches | `*_test.go`, `*.test.ts(x)` |
| **Integration** | real ClickHouse/sqlite via the Go harness (`-tags integration`, `/tmp/clickhouse`) | `*_integration_test.go` |
| **Contract** | HTTP response bodies validated against `contracts/openapi/pulse-api.yaml` (kin-openapi) | `internal/api/*_contract_test.go` |
| **Functional** | a feature's user-visible behavior end-to-end through the API (publish→visible, alert→history) | `e2e.yml` steps + api tests |
| **E2E (browser)** | dashboard render, auth redirect, CSP header, large-table virtualization | `web/e2e/*.spec.ts` (Playwright — NEW) |
| **Regression** | a fixed bug stays fixed (every D-0NN fix gets a pinning test) | co-located with the fix |
| **Edge-case** | empty/zero/max/null/unicode/pagination boundaries | per package |
| **Failure-path** | timeouts, 4xx/5xx, drop-on-full, retry exhaustion, decode errors | per package |

**Coverage gate (must not regress; the three 0.0% packages must reach ≥60%):**
```
sg docker -c 'docker run --rm -v /home/aytek/repo/ams-pulse:/repo -w /repo/server -e GOFLAGS=-buildvcs=false -e CGO_ENABLED=1 golang:1.25 sh -c "go test -race -coverprofile=cover.out -covermode=atomic ./... && go tool cover -func=cover.out | grep -E \"^total|0.0%\""'
```
**Prioritize critical business logic first:** (1) license/tier enforcement, (2) alert firing + delivery, (3) ingest
health scoring, (4) AMS wire decode/normalize, (5) the query layer. Report coverage in every handoff.

---

## 3. VERIFICATION WORKFLOW (BINDING — every implementation runs ALL of these before "done")

1. **Build:** `go build ./...` (CGO_ENABLED=0) + `cd web && npm run build`.
2. **Lint:** `cd web && npm run lint`; Go `gofmt -l` (must be empty) + `go vet ./...`.
3. **Type-check:** `cd web && npm run typecheck` (or `tsc --noEmit`).
4. **Test (race):** `go test ./... -race -count=1` **repo-root mount** (D-028: server-only mount silently skips ~90 api
   tests → false green). Confirm **0 FAIL, 0 unexpected SKIP**.
5. **Coverage:** the gate command in §2; attach numbers to the handoff.
6. **Contract drift:** `cd web && npm run gen:api` then `git diff --exit-code` (generated types match spec);
   `redocly lint` + `ajv` on event schemas.
7. **Staging verify:** bring the change up on an **isolated compose project** (NOT pulse-prod) and curl the affected
   endpoints. Never verify on prod first.
8. **Deploy smoke (after a prod change):** `/healthz` ok via `--resolve`; affected endpoint returns expected real
   data; `pulse logs` shows no 401/403/decode/login errors; for migrate, DSN masked (`:xxxxx@`).
9. **Independent/adversarial re-check:** default to "refuted" until reproduced on a fresh build (D-013/017/019). A
   verify harness that silently skips == no verify (D-028).

---

## 4. BINDING FLOWS — every workflow MUST end with these (user directive)

- **Verify** — independent/adversarial re-check of *every* claim against a running stack or fresh build; default to
  "refuted" until reproduced; **repo-root mount** or api tests silently skip (D-028). QA alone is not authoritative
  (D-013/017/019).
- **Commit** — by **EXPLICIT path**, per scope; never `git add -A/-u/.` (parallel agents share the tree — D-008/D-011).
  In a workflow, agents AUTHOR only; ORCH commits centrally (avoids `.git/index.lock` races). Message
  `<scope> D-0NN: <summary>` + evidence. Push when the user directs.
- **Handoff** — update **THIS `RESUME-PROMPT.md`** + `decisions.md` (new D-0NN) every session, then commit + push.

---

## 5. OPERATING PROTOCOL (binding — learned the hard way)

- **Orchestrate with the Workflow tool.** One phase = one Workflow: ORCH writes the plan + pre-approved CRs to
  `decisions.md`, fans out to disjoint-scope agents, then **independently gates**. Background work is harness-tracked —
  you're re-invoked on completion; don't poll-spin.
- **CodeGraph (operator-installed 2026-07-09, D-061).** Local index `.codegraph/` + CLI `~/.local/bin/codegraph`.
  Scouts/authors query the graph BEFORE grep/file sweeps: `codegraph explore "<question>"`,
  `codegraph node <sym>`, `codegraph callers <sym>` (blast radius). Put this in every agent work order
  (subagents use the CLI via Bash). **Closing protocol: `codegraph sync` after the last commit** (+
  `codegraph status` to confirm; stale lock → `codegraph unlock`).
- **Local compose stacks NEVER run from the real repo** — compose auto-loads `deploy/.env` (prod secrets) from
  the `-f` dir. Use a pristine working-tree copy:
  `git ls-files -co --exclude-standard -z | tar --null -T - -cf - | tar -C <scratch> -xf -` + unique `-p` name (D-061).
- **Anti-stall (D-016):** NEVER run `pulse serve`/`clickhouse server` in the foreground inside an agent. Use
  `docker compose up -d` (detached) + health polling; CH unit work via the integration harness. `timeout` on builds,
  `-timeout` on `go test`, vitest `run` not watch, `curl -m`. Long local repros: Bash `run_in_background: true`.
- **Single-writer scope map** in `agents/manifest.yaml`. **Contracts frozen (D-004)** — changes only via an
  ORCH-approved CR applied by INT-01 (OpenAPI + event schemas + migrations).
- **⚠️ Workflow/fork agents have Write+commit access** — a reviewer fork once auto-committed during a concurrent ORCH
  edit (D-030 process note). Scope reviewer agents read-only when ORCH is editing the same files.
- **⚠️ Subagents NEVER revert shared-tree files (D-063):** no `git restore` / `git checkout --` /
  `git stash` inside workflow agents — concurrent agents' UNCOMMITTED work shares the tree, and a
  verifier reading `git status` cannot tell foreign work from scope violations. Violations are
  REPORTED; ORCH decides and reverts. ORCH also commits early per scope to shrink the window.
  (A wo6 fixer once destroyed two files of verified work; recovered only via transcript-replay.)

---

## 6. HARD RULES (CLAUDE.md / ARCHITECTURE §3)

- AMS wire formats ONLY in `server/pkg/amsclient` + `server/internal/collector`; metrics in ClickHouse, config in the
  meta store, never crossed; web UI consumes ONLY generated public-API types; beacon ingest is hostile input.
- `CGO_ENABLED=0` for the shipping build (pure-Go sqlite); single binary `pulse serve|migrate|diag`; React 19 + RR7 +
  Vite + TS strict; recharts; no external fonts/CDNs. `go test -race` needs `CGO_ENABLED=1` + gcc.
- **4 tiers** (free/pro/**business**/enterprise) in the contract enum + `internal/license/license.go` (D-014).
- Deploy fixes live in `deploy/`. Base `docker-compose.yml` stays clean (`expose:`, no host ports); exposure in
  overrides. Prod stack = `base + hardened + prod-tls + real-ams + backup` (5 overlays since D-054 — see §14).

---

## 7. ENVIRONMENT (VPS)

- **Ubuntu 24.04 VPS `161.97.172.146`**, Docker 29 + Compose v5. **`go` is NOT on PATH** — run Go only in Docker
  (`golang:1.25`). node 20 + npm 10 on PATH. **`gh` IS installed + authed as owner `aytekXR`**.
- **⚠️ For `go test` mount the REPO ROOT** (`-v /home/aytek/repo/ams-pulse:/repo -w /repo/server -e
  GOFLAGS=-buildvcs=false`): a `server/`-only mount makes `metaDDLPath` escape the mount → `t.Skip` →
  skip-counts-as-pass false green (~90 api tests). Confirm **0 SKIP** for api.
- **Docker:** user `aytek` is in `docker` group but stale in non-login shells → prefix `sg docker -c "…"`. `sudo` needs
  a password → ask the user via the `! <cmd>` prompt for privileged ops. For host-root debugging without sudo, run a
  privileged container in the host netns (e.g. `docker run --rm --net=host --cap-add=NET_RAW corfr/tcpdump …`, D-036).
- **Real-AMS prod ops** (run from repo root): `DC="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml
  -f deploy/docker-compose.backup.yml --env-file deploy/.env"` (backup overlay is part of the standing combo
  since D-054 — omitting it on `up -d` would REMOVE the backup sidecar). Status: `sg docker -c "docker compose $DC ps"`. Admin token: in `oguz-testing.md`
  (gitignored) — persisted in the `pulse-prod_pulse-data` volume; **never `down -v` that volume.** TLS check: always
  `--resolve beyondkaira.com:443:161.97.172.146` (VPS DNS is stale). Rollback: runbook §5.
- `deploy/.env`, `*.db*`, `oguz-testing.md`, `web/pulse_secret.key` are gitignored — never commit.
- `deploy/config/Caddyfile.prod` is clean and tracked, and uses `{$AMS_UPSTREAM}`.
- ⚠️ **Concurrent-session hazard (learned D-062):** the operator may run a second Claude session in
  this repo. If HEAD moves or the tree dirties mid-session with work you didn't do, STOP and inspect
  before committing/pushing — a foreign unpushed commit once carried a hardcoded live secret (O11).
