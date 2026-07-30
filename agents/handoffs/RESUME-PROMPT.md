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

**Where the product is:** **v0.4.5 is RELEASED and verified, and is the marketplace submission
target.** Published artifacts checked the way a reviewer would, not assumed: GitHub release with
binaries + `SHA256SUMS` + both SDK tarballs, `ghcr.io/aytekxr/ams-pulse:0.4.5` signed and
**cosign v3-verifying**, `latest` moved, helm chart `0.3.3` **anonymously pullable**, and the
released image **scans 0 HIGH/CRITICAL** (Alpine 0, Go binary 0).

**The release pipeline earned its keep twice, and both failures were real.** First the version guard
rejected the tag over a `deploy/helm/pulse/README.md` chart pin still on 0.3.2 — *my local
replication of that check scanned two files when the real one scans four.* **Run the actual guard;
do not paraphrase it.** Then Trivy blocked the release on **CVE-2026-56852** (HIGH: `norm.Iter`
infinite-loop DoS in `golang.org/x/text` 0.38.0, reaching the binary as an *indirect* dependency).
Bumped to 0.39.0 and re-verified by scanning the **rebuilt binary**, not the version number. Nothing
vulnerable was ever promoted: the quarantine image is pushed *before* the scan and promotion happens
*after*, so no public `0.4.5` tag and no moved `latest` existed while the CVE was in the tree —
verified by manifest inspection.

**⚠ A tag push went to the wrong commit, and the reason is worth not repeating.** `git tag -d`
was run with its output suppressed, it did not do what I assumed, the old annotated tag survived,
and `git push origin v0.4.5` shipped the **pre-CVE-fix commit** — while `git tag -a` had already
errored "already exists" in the same breath. The release started building vulnerable code before it
was cancelled. **`git rev-parse v0.4.5` returns the TAG OBJECT, not the commit — always deref with
`v0.4.5^{commit}` and compare against HEAD before pushing a tag, and never suppress the output of a
destructive git command.**

S122 also ran a
brutal-review rehearsal — 11 hostile lenses across documentation, security and functionality, every
finding refuted before it counted, then a second pass attacking the *all-clears*. Result: **six
confirmed defects, every one of them documentation**, and zero in security or functionality. That
zero is credible rather than lazy: the lenses ran the full race suite (26 packages, 79.2% coverage),
683 web tests, 296 PulseKit tests in CI's own container, booted a real clean-room stack, and probed
live prod including `/debug/pprof`. **The single most valuable thing they produced was their honest
blind-spot statements**, which named three things a marketplace reviewer certainly does and nobody
here had ever executed: `cosign verify`, the anonymous Helm OCI pull, and the `curl | bash`
quickstart URL. All three were then run for real and all three pass (cosign v3 digest `81673359…`;
chart `0.3.3` pulls anonymously; quickstart URL 200). **When a lens reports its own blind spots,
that list is the next work item, not a footnote.**

**`[ANOM-TIER]` is resolved and it was bigger than the ticket said.** The operator ruled "advertise
correctly", so anomaly detection is Business+ now (`CheckAnomalies` admits Business — grant-only, so
nothing regresses and e2e A5 still passes). Two discoveries in the doing: the **web UI also gated on
Enterprise**, so an entitled Business tenant would have met an upgrade wall over data the API was
already serving — a half that would have shipped invisibly because no test covered a Business tenant
reaching that page. And the operator queue's own reassurance that *"the website deliberately does
not state anomaly detection's tier"* **was false** — it said `F9 - ENTERPRISE` in the card and put
anomaly detection under Enterprise in the pricing table. **The exculpation was wrong again; that is
now four rounds running.**

**A fix that does not change the artifact has not been verified — re-earned on the SettingsPage
flake.** It had been open debt for sessions. The cause was a class, not a test: sixteen sites did
`await waitFor(() => expect(mock).toHaveBeenCalled())` and then read the DOM synchronously, and
`waitFor` resolves *before* React commits the render. Converting them to `findBy*` fixed the flake
**and introduced a new deterministic failure**, because one of those tests installs
`vi.useFakeTimers()` and `findBy*` polls on a clock that test has frozen. Isolation runs had been
passing 23/23 the whole time while CI's full run failed — **an isolation run is not evidence about a
flake.** Proof is now 6 consecutive full-suite runs, 683/683 each.

**Guard scope escaped for the third time, so check #17 is no longer a file list.** Two stale image
pins were sitting in the band between guards: `deploy/quickstart/.env.example` pinned `0.4.2` (three
releases behind, in the file a quickstart user copies) and the Ankush reply draft pinned `0.4.4`
under prose saying "v0.4.3 is released". Each previous fix had rewritten the check as a slightly
longer explicit list; it now scans every tracked md/yml/yaml/sh/example minus a documented exclusion
set, and **what it does not cover is written down inside the guard, with the reason.** Verified by
planting a stale pin in `docs/support.md` — a file the old version sailed past.

**Third-party licence attribution now exists** (`THIRD-PARTY-LICENSES.md`), and it is **generated,
not written** — `scripts/gen-third-party-licenses.sh` reads licence text from the Go module cache
and `node_modules`, so it cannot claim a licence a dependency does not carry, and `--check` fails
when stale. That paid off on the first run: it reported `nhooyr.io/websocket` as UNKNOWN because the
detector only matched the ISC wording "and/or distribute" while that module says "and distribute".
A hand-written file would have recorded a guess. Findings: 56 Go modules + 66 npm packages
redistributed, **zero GPL/AGPL/LGPL/SSPL, zero undetermined**, both SDKs have zero runtime
dependencies (so the listing's "embed in commercial products without restriction" holds), and
**ClickHouse is Apache-2.0, not SSPL** — stated explicitly because it is the dependency most likely
to be challenged on a wrong assumption.

**Stale claim corrected:** the previous handoff warned that `.github/branch-protection.sh` "must be
re-run to apply the `ios-kit` context". It was already live — 16 contexts, exactly matching the
script. Verified, not assumed.

**Two local-hygiene traps for the next session:** container runs as root leave root-owned
`.build/`, `node_modules/.vite/` and `coverage/` in the working tree, which makes `npm ci` fail
EACCES and makes `swift test` fail with a PCH module-cache path error. Nothing tracked is affected
(checked). Test Swift from a clean `git archive` copy, which is also what CI does.

**Two independent tracks remain, each blocked by exactly one operator action; neither blocks the
other:**

- **Marketplace** — blocked by **G-02: rotate `CLICKHOUSE_PASSWORD`**, and now by nothing else. A
  32-hex prefix of the live value has been in public git history since `98b011c`; re-checked
  2026-07-30, still 2 commits. `v0.4.5` is already cut, so the motion is down to **rotate, then
  submit**. Not remotely exploitable today (ClickHouse is Docker-internal, never published to the
  host), but it is 128 bits of a live secret in a public repo. Nothing in v0.4.5 claims it was
  rotated.
- **iOS TestFlight** — blocked by **Apple Developer Program enrolment**. Everything a machine can do
  is done: builds for iOS 26 under Swift 6 strict concurrency on a real macOS runner, 296 PulseKit
  tests on Linux + 50 simulator tests, live-validated against a real Pulse server, archive/sign/
  upload job written but **never executed** — treat its first run as discovery, not regression.
  `docs/operator-expected.md` §A is the eight-step path.

**The website is live with no operator action** — GitHub Pages publishes to
https://aytekxr.github.io/ams-pulse/ on merge to `main`. `/privacy/` and `/support/` are the two
URLs App Store Connect demands; `/beta/` shows a disabled button plus a working manual-invitation
route until a TestFlight link exists. The swap marker is `TESTFLIGHT_PUBLIC_LINK_PLACEHOLDER`,
appearing exactly once.

**Standing lessons that keep paying (condensed — session narration lives in `sessions/`):**

- **Test the artifact, not the documentation, and run it the way CI will.** PulseKit once passed
  206/206 on this host and failed 28 in CI's container over a hardcoded `/home/aytek` fixture path.
  A host-green run is not evidence for a CI claim. Corollary re-earned in S122: an *isolation* run
  is not evidence about a *flake*.
- **Before filing a defect against code, confirm the artifact you tested was built from it.**
  `pulse version` + `git merge-base --is-ancestor` is the whole procedure; it once saved a session.
- **Audit the exculpations, not just the findings.** Four rounds running, a review's "this area is
  clean" has hidden more real defects than its findings contained. What fails is the generalisation
  step. In S122 the highest-yield artefact was the lenses' own *blind-spot* statements.
- **Verify the exact string you depend on** — the version, the tag, the field name — not the thing
  it belongs to. A `v3` action tag once shipped under a comment saying it had been verified; the tag
  did not exist.
- **Measure the third party before writing code against it.** A ten-minute probe on a real
  `macos-15` runner (`docs/mobile/ci-runner-facts.md`) caught that the default Xcode is 16.4 while
  App Store Connect requires 26. Re-measure after a runner-image bump.
- **Test the guard you write to enforce testing, in BOTH directions** — make it fail on purpose, and
  check the negative test tests what you think. And **write down what a guard does NOT cover, inside
  the guard**; scope gaps are where drift lands.
- **A subagent lane's self-report is not a gate.** Read every diff yourself; ORCH commits centrally.
- **⚠ Concurrent-session hazard is real and has happened.** If HEAD moves or the tree dirties with
  work you did not do, STOP and inspect. Quarantine, never delete.
- **A client is not finished when it works against the server it was compiled against.** PulseKit
  tolerates null/absent arrays and unknown enum values, but the floor is pinned by tests: an HTML
  error page, a bare `{}`, a top-level array, truncated JSON and a number-where-an-enum-belongs must
  all still throw. Do not lower that floor for convenience.

**Do first, every session:**
1. **Gate reads** — prod health (component-scoped `/healthz`, not a whole-body `"status":"ok"` grep,
   which passes while the collector is degraded — plus a ClickHouse count), git/PR drift, and
   whether the operator rotated `CLICKHOUSE_PASSWORD` (compare the live value's first 32 chars
   against `git log -S`; never print the secret).
2. **Check whether the Apple account exists yet** (`gh secret list` for the three
   `APP_STORE_CONNECT_*` secrets). If present, the TestFlight path is unblocked.
3. **If a TestFlight public link arrives**, do the `/beta/` swap and redeploy.

**Open engineering debt (loop-owned, non-blocking):** the app's `KeychainService` duplicates
PulseKit's `TokenStore`/`KeychainTokenStore` — two ways to store a credential, the divergence shape
this repo keeps getting bitten by · cluster node-alerting rework (LIM-10, waits on a real cluster) ·
verify the `ClusterNodeDTO.lastUpdateTime` unit against AMS source · surface `stream_ingest_error`
(LIM-27) · thread the owning node through per-app polling (LIM-28) · poller/discovery cadence
consolidation · helm NetworkPolicy golden · **the ClickHouse integration harness's 45 s startup
budget is too tight for a slow runner** — `TestIntegration_BatchInsert` failed with "timeout
waiting for ClickHouse to start" on a DOCS-ONLY PR in S122 and passed on re-run, with the same
test green on the previous three `main` runs; it is a `server`-job flake in a required context, so
it can red a merge for no reason (`server/internal/store/clickhouse/integration_test.go:83-101`) · **release: switch the candidate push to buildx
`push-by-digest=true`** so promoted images stop carrying a public `candidate-<sha>` alias (round 6
H-09) · **probe whether AMS's `ams-webrtc-stats` shape can restore per-stream FPS** now that the
console-endpoint assumption behind LIM-01 turned out to be wrong (LIM-04 rests on a similar
inference) · **self-host the IBM Plex OFL woff2 files for `website/` and the iOS app** — note the
previous wording ("the website *and the app both* fall back to system fonts") overstated it: the
**web UI already self-hosts them** (`web/dist/assets/ibm-plex-*.woff2` are present and were
confirmed during the S122 licence inventory), so this is scoped to `website/` and the app, not the
dashboard.

**Closed in S122:** the `SettingsPage` ARIA flake (fixed at the class level — sixteen racy sites,
proven over 6 consecutive full-suite runs) · `[ANOM-TIER]` (operator-ruled Business+, enforced in
server, web UI and every doc) · missing third-party licence attribution.

**Operator queue:** `docs/operator-expected.md`. **How we got here** (read only if you need it):
`decisions.md` · `agents/handoffs/sessions/` · `docs/assessment/`.

---
## 1. CURRENT STATE (verified facts — refresh each session, never let this go stale)

- **Shipped product, pre-marketplace.** All 10 PRD features implemented and live-validated
  against a real AMS 3.0.3 Enterprise (46/50 scenarios). **Latest release: v0.4.5** (2026-07-30,
  the marketplace submission target; it carries review rounds 7–11 plus the S122 corrections,
  which v0.4.4 did not). AMS 3.0.3 is still the latest AMS release.
- **Production** runs behind host nginx on this VPS at `https://pulse.beyondkaira.com`, against
  the operator's own `antmedia` container (AMS Enterprise 3.0.3, `--network host`). It is on the
  stamped **v0.4.0-139** build — rolling prod forward is deliberate and operator-gated, never
  automatic. Health at last check (S122, 2026-07-30): all three `/healthz` components `ok`,
  **1,373,306** server events, newest 6 s old, collector actively ingesting.
- **The iOS app exists and is CI-verified** (D-186). `ios/PulseKit` — Foundation-only, **296 tests
  green on Linux** (re-verified S122 in CI's `swift:6.1` container from a clean `git archive` copy), which is the point of the split: no Apple toolchain exists on this VPS, so
  anything living in a SwiftUI view is logic no gate here can check. `ios/PulseApp` — SwiftUI plus
  an AVPlayer view instrumented with our own `PulseBeacon` SDK, **50 tests green on a real iOS 26.2
  simulator** via `macos-15`. The archive/sign/upload job is written and has **never executed** —
  it cannot until an Apple Developer account exists. Measured runner facts (re-measure after an
  image bump): `docs/mobile/ci-runner-facts.md`.
- **The public website exists** (D-186) at `website/` — landing, `/beta/`, `/privacy/`, `/support/`,
  `/terms/`, built from the brandkit against `tokens.json`, zero external requests (enforced by a
  check, not by intent). GitHub Pages is **enabled** (`build_type: workflow`) and publishes to
  `https://aytekxr.github.io/ams-pulse/` on merge to `main`.
- **`main` is protected** (strict, 1 review, `enforce_admins=false` so owner pushes work; **16**
  required contexts — D-185 added `shellcheck` and `doc-stamps`, D-186 added `ios-kit`. A guard job
  that is not in that list cannot block a merge. `ios-app` is deliberately excluded: it depends on
  a third-party runner image and Homebrew, the same argument that excludes `compose-boot`. What
  that leaves uncovered is that a SwiftUI regression can merge on a red `ios-app` if ignored).
  The live setting and the script now MATCH — verified S122 via the API: 16 contexts, exactly the
  script's 14 plus the two CodeQL `Analyze (…)` jobs. The earlier warning that the script still
  needed re-running was stale.
  Work on a branch → PR → merge on green.
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
