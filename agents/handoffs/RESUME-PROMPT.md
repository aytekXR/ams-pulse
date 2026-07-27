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

**Where the product is:** **v0.4.4 is the release and the marketplace submission target.**
S113 (D-181) executed external review **round 7**: all six findings confirmed, all six fixed,
none refuted. Round 7's verdict is *"ready to submit as soon as G-02 closes"* — it re-verified
every round-6 disposition against the **published artifacts**, confirmed the tag↔main drift
pattern is broken, and found only prose/comment drift plus one metric nit. **The external review
loop has converged**; further rounds are optional polish, not gating.

The round-6 headline still stands as the product change: LIM-01 is closed — standalone AMS
reports CPU/memory/disk via `GET /rest/v2/system-resources`, no Kafka.

**One thing blocks submission, and it is the operator's:** rotate `CLICKHOUSE_PASSWORD`. A
32-hex prefix of the live value is in public git history since `98b011c`. Deliberately deferred
when the v0.4.3 cut was authorised — a recorded decision, not an oversight. **Re-checked S113:
still un-rotated** (live prefix still matches 2 commits). Both the reviewer and the loop agree
it is the only remaining gate: rotate, then submit against `v0.4.4`.

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

**Guard scope is a decision; scope gaps are where drift lands (S113's lesson).** Guard #17 was
deliberately narrowed to runnable `ams-pulse:<semver>` pins to avoid false positives — correct,
and the exact reason the README's prose pointer shipped one release stale *inside* the new
submission target hours later. New check **#19** closes that. When you narrow a guard, write down
what the narrowing leaves uncovered. And **de-literalize whatever drifts twice**: the cosign
version range and the chart-version sentence were both "just bump the number" the first time.

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
`SettingsPage` ARIA test flake under parallel execution · **release: switch the candidate push
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
  automatic. Health at last check (S112, 2026-07-27): all three `/healthz` components `ok`,
  **1,328,195** server events, collector actively ingesting.
- **`main` is protected** (required contexts, strict, 1 review, `enforce_admins=false` so owner
  pushes work). Work on a branch → PR → merge on green.
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
