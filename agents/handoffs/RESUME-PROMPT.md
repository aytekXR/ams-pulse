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

**Where the product is:** **v0.4.4 is the release and the marketplace submission target.** S118
(D-186) opened a second track: **the Pulse iOS app and the public website now exist and are
CI-verified.** S119 (D-187) then pointed the real client at a real server for the first time and
made it survive the servers it will actually meet — older ones that send `null` where the contract
says array, newer ones that send enum values this build has never heard of. The external-review loop converged at round 11 (D-185, merged) and is closed by
decision — rounds 6→11 ran H(9) → I(6) → J(3) → K(2) → L(2, both refuted) → N(1), and no reviewer-
filed product defect has survived verification since round 6.

**There are now two independent tracks, each blocked by exactly one operator action, and neither
blocks the other:**

- **Marketplace** — blocked by **G-02: rotate `CLICKHOUSE_PASSWORD`** (a 32-hex prefix of the live
  value has been in public git history since `98b011c`; re-checked S118, still 2 commits). The
  submission-day motion is one movement: **rotate, cut `v0.4.5`, submit against it** — that cut
  also clears the tag-frozen prose and ships rounds 7–11.
- **iOS TestFlight** — blocked by **Apple Developer Program enrolment**. Everything a machine can
  do is done: the app builds for iOS 26 under Swift 6 strict concurrency on a real macOS runner,
  **291** Linux tests + 50 simulator tests pass, the client is live-validated against a real Pulse
  server (`ios/livecheck`, 9/9 endpoints), the archive/sign/upload job is written, and CI uploads a
  **runnable simulator build** so a Mac-owning tester can see the app before TestFlight exists.
  `docs/operator-expected.md` §A is the eight-step critical path.

**The website is live-able with no operator action.** GitHub Pages was enabled from here via the
API (`build_type: workflow`); it publishes to **https://aytekxr.github.io/ams-pulse/** on merge to
`main`. `/privacy/` and `/support/` are the two URLs App Store Connect demands; `/beta/` is the
tester page. Until the operator has a TestFlight link, `/beta/` shows a **disabled** button plus a
working manual-invitation route — honest and usable today. The grep marker for the swap is
`TESTFLIGHT_PUBLIC_LINK_PLACEHOLDER`, and it appears exactly once.

**Do first, every session:**
1. **Gate reads** — prod health (component-scoped `/healthz` + a ClickHouse count), git/PR drift,
   and whether the operator rotated `CLICKHOUSE_PASSWORD` (check silently: compare the live
   value's first 32 chars against `git log -S`, never print the secret).
2. **Check whether the Apple account exists yet.** If the three `APP_STORE_CONNECT_*` secrets are
   present (`gh secret list`), the TestFlight path is unblocked: dispatch the `ios` workflow, watch
   the archive/upload, and drive the rest. That job has **never executed** — treat its first run as
   a discovery exercise, not a regression check.
3. **If a TestFlight public link arrives**, do the `/beta/` swap and redeploy the site.

**The app RUNS, and that is a separate claim from "the app builds" (S120).** CI now boots a
simulator, installs, launches, waits 12 s (a SwiftUI crash in `body` happens *after* `simctl
launch` returns a pid), screenshots both appearances and fails on a crash report or a dead process.
Opening those PNGs found two defects no test could: the capture step itself was mislabelling a
light screenshot as dark (a simulator boots in light), and the Server URL placeholder rendered as a
blue Markdown autolink — `TextField("https://…", text:)` takes a `LocalizedStringKey`, which SwiftUI
parses as Markdown. **The first fix for that was wrong and the screenshot said so**: reasoning from
the sibling field produced a confident, plausible, useless change, and the next run came back just
as blue. A fix that does not change the artifact has not been verified.

**Before filing a defect against code, confirm the artifact you tested was built from it (S119).**
The live harness found `/live/streams` returning `{"items":null}` against its own contract. The
obvious write-up — "the server is wrong" — was wrong: the server code is correct, commented, and
has a regression test. The container under test was built 2026-07-13; the fix landed 2026-07-18.
Five minutes of checking saved a session. `pulse version` and `git merge-base --is-ancestor` are
the whole procedure.

**A client is not finished when it works against the server it was compiled against (S119).**
Pulse is self-hosted; the app meets whatever version the operator last upgraded to, and an App
Store app cannot ship in lockstep. PulseKit now tolerates null/absent arrays and unknown enum
values (preserving the raw string). **The floor is pinned by tests**: an HTML error page, a bare
`{}`, a top-level array, truncated JSON and a number-where-an-enum-belongs all still throw — a
decoder that turns malformed responses into empty lists makes the app say "no streams" when the
truth is "the server is broken", and that is worse than the brittleness it replaced. Do not lower
that floor for convenience.

**Test the artifact, not the documentation — and run it the way CI will (S111, re-earned hard in
S118).** PulseKit passed 206/206 on this host and failed **28** in the container CI actually uses,
because a fixture loader hardcoded a path under `/home/aytek`. The host run was never evidence for
the claim being made. The same session: the published `cosign verify` command fails on a cosign v2
client (D-178), and the iOS `Info.plist` would have silently discarded CI's build number at upload
time. **When a claim names an environment you are not in, reproduce it in that environment.**

**Measure the third party before you write code that depends on it (S118).** A ten-minute
throwaway probe on a real `macos-15` runner (`docs/mobile/ci-runner-facts.md`) paid for itself
instantly: the runner's *default* Xcode is 16.4, and App Store Connect has refused anything below
Xcode 26 / iOS 26 SDK since 2026-04-28. A CI job taking the default goes green and produces an
artifact Apple rejects. Re-measure after a runner-image bump; the default moves.

**"Verified" is a claim about a specific thing, not about a neighbourhood (S118).** The TestFlight
upload step was pinned to `Apple-Actions/upload-testflight-build@v3` under a comment saying the
action had been verified that day. The action is real and maintained. **The `v3` tag does not
exist.** Verify the exact string you are about to depend on — the version, the tag, the field name
— not the thing it belongs to.

**Verify the exculpations, not just the accusations (S114/S116, still the highest-yield move).**
Round 10 filed two findings, both refuted; auditing the same round's *reassurances* produced three
real defects. What fails is the generalisation step: "this file truncates its fields" becomes "the
system truncates its fields". When a review says an area is clean, ask which file they read, then
go and find its siblings.

**Test the guard you write to enforce testing (S116, third instance in S118).**
`check-doc-stamps.sh` reported **correctly-stamped files as failing**: it runs under `pipefail` and
piped into `grep -q`, which exits on first match, SIGPIPEs the upstream `sed`, and turns a
successful match into a non-zero pipeline. Non-deterministic, buffering-dependent, and it presented
as two of three files failing while the third passed. Run every new check in **both** directions —
make it fail on purpose — and re-check that your negative test tests what you think (the first one
here picked an unstamped doc, which is exempt by design, and the resulting PASS looked like a bug).

**Guard scope is a decision; scope gaps are where drift lands (S113→S118, five rounds).** The new
website checker defaulted its web root to `$PWD`, so run from the repo root it walked
`node_modules` and failed on files it has no business policing. **Write down what a guard does NOT
cover, in the guard.** And de-literalize whatever drifts twice.

**A subagent lane's self-report is not a gate (S116, held again in S118).** Lanes produced sound
logic and honest evidence and still shipped: an app layer bound to types that do not exist, a
nonexistent action tag under a "verified" comment, and a stale comment naming the wrong test
target. Read every diff yourself and correct provenance centrally before committing.

**⚠ The concurrent-session hazard is real and it happened (S118).** A second autonomous session
wrote `ios/` at the same time as this one, and its completion notifications arrived here. It
produced a flat `PulseKit` while this session produced the layered contract-derived one — two
implementations of the same public types in one target, which cannot compile. Handling that worked:
**quarantine, never delete** (its files were preserved outside the repo), keep the contract-derived
version as canonical, and harvest what was genuinely better (its `PlayerView` and its
"honest-absent" principle both survive). If HEAD moves or the tree dirties with work you did not
do, STOP and inspect before committing.

**Tag/main parity:** `main` carries rounds 7–11 plus D-186. **The moment a fix an evaluator would
meet lands on `main`, it belongs in a release** — "it rides the next one" is exactly the ruling
round 6 had to overturn.

**Open engineering debt (loop-owned, non-blocking):** the app's `KeychainService` duplicates
PulseKit's `TokenStore`/`KeychainTokenStore` — two ways to store a credential, the divergence shape
this repo keeps getting bitten by · cluster node-alerting rework (LIM-10, waits on a real cluster) ·
verify the `ClusterNodeDTO.lastUpdateTime` unit against AMS source · surface `stream_ingest_error`
(LIM-27) · thread the owning node through per-app polling (LIM-28) · poller/discovery cadence
consolidation · helm NetworkPolicy golden · the `SettingsPage` ARIA test flake under parallel
execution · **release: switch the candidate push to buildx `push-by-digest=true`** so promoted
images stop carrying a public `candidate-<sha>` alias (round 6 H-09) · **probe whether AMS's
`ams-webrtc-stats` shape can restore per-stream FPS** now that the console-endpoint assumption
behind LIM-01 turned out to be wrong (LIM-04 rests on a similar inference) · self-host the IBM Plex
OFL woff2 files (the website and the app both fall back to system fonts today, which `tokens.json`
does not intend).

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
  automatic. Health at last check (S118, 2026-07-28): all three `/healthz` components `ok`,
  **1,355,548** server events, newest 2 s old, collector actively ingesting.
- **The iOS app exists and is CI-verified** (D-186). `ios/PulseKit` — Foundation-only, **259 tests
  green on Linux**, which is the point of the split: no Apple toolchain exists on this VPS, so
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
  ⚠ **`.github/branch-protection.sh` must be re-run to apply the `ios-kit` context** — the script
  is the source of truth and the live setting follows it, not the other way round.
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
