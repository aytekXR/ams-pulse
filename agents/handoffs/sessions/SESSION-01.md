# SESSION-01 — Release engineering + D-056 prod rollout («dockerization first»)

> Written 2026-07-08 (D-057) from the 9-scout verified audit. Paste-ready prompt.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP.md`
> (plan of record, §3.S1) + `RESUME-PROMPT.md` §7/§8 (TDD + verification, binding) first.
> Operator directive: dockerization/release path FIRST — this session makes the image shippable
> and brings prod current.

## Mission

Exit = ROADMAP G1+G2: a stranger can pull a **versioned, cosign-signed, Trivy-scanned, SBOM'd,
multi-arch** image from a canonical GHCR path published by a **CI-gated** tag pipeline; `pulse
version` reports real version/commit/date; `v0.1.0` exists; `main` is branch-protected; **prod
runs current main (D-056 beacon fix live)**; backup cycle 2 + keep-7 confirmed.

## Preconditions (re-verify cheaply — fix this prompt if stale)

- HEAD is `175095a` or later; tree clean; last 3 ci runs on main green (`gh run list --branch main -L 3`).
- `gh auth status` = owner `aytekXR` (needed for WO-6; if the token lacks admin scope, WO-6
  becomes operator O6).
- Prod pulse container still predates D-056 (`sg docker -c "docker ps"` CreatedAt 2026-07-07 09:30).
- Backup volume has cycle-1 artifacts `pulse-20260707-073113`; cycle 2 expected ~07:31 UTC 2026-07-08.
- Evidence anchors (verified 2026-07-08): `main.go:39-44` Version/GitCommit/BuildDate vars;
  `deploy/docker/pulse.Dockerfile:24` build without ldflags, `:18` floating `golang:1.25-alpine`;
  Makefile:30 no ldflags; release.yml:15 no `needs`/gate, `:33` `images: ghcr.io/aytekxr/ams-pulse`,
  `:40-48` no `platforms:`; hardened.yml:36 `caddy:2-alpine` floating; helm values.yaml:13
  `ghcr.io/pulse-analytics/pulse`; no `.github/dependabot.yml`; branch protection API → 404.

## Work orders (one Workflow: disjoint-scope authors → TDD → adversarial verify → ORCH gate/commit; WO-5/WO-6 are ORCH-sequential AFTER the commits land)

### WO-1 — Version stamping end-to-end · scope `deploy/docker/` + `Makefile` + `server/cmd/pulse` + `.github/workflows` · [M]
- **Now:** every build reports `pulse dev (commit unknown, built unknown)` — no `-ldflags`
  anywhere (Dockerfile:24, Makefile:30, release.yml, ci.yml docker-build).
- **Change:** `ARG VERSION COMMIT BUILD_DATE` in the Dockerfile fed to
  `-ldflags "-X main.Version=… -X main.GitCommit=… -X main.BuildDate=…"`; Makefile derives them
  from `git describe --tags --always --dirty` / `git rev-parse --short HEAD` / `date -u`;
  ci.yml docker-build and release.yml pass build-args (release: the tag via metadata-action).
- **TDD:** RED unit test first on a small `versionString()` helper (extract from the version
  command if needed) asserting format + that stamped values appear; PLUS a falsifiable pipeline
  assert: ci.yml docker-build step runs `docker run --rm <img> pulse version` and **fails if the
  output contains `dev` or `unknown`** (mutation-proof: revert the ldflags → step must fail).
- **Verify:** local docker build with args → `pulse version` shows them; ci.yml green.

### WO-2 — release.yml hardening (gate, multi-arch, scan, SBOM, sign) · scope `.github/workflows/release.yml` · [M]
- **Now:** tag push publishes immediately: no CI gate (`:15`), single-arch (`:40-48`), no Trivy,
  no SBOM/provenance, no cosign; permissions only `contents: read` + `packages: write` (`:10-13`).
- **Change, in job order:** (1) first step verifies the tagged commit has a **successful `ci`
  run** via `gh api` (fail otherwise); (2) qemu+buildx, `platforms: linux/amd64,linux/arm64`;
  (3) build locally → **Trivy scan `--exit-code 1 --severity HIGH,CRITICAL`** BEFORE push;
  (4) push with `provenance: true` + `sbom: true` (or syft-attach); (5) cosign keyless sign
  (add `id-token: write`); (6) `workflow_dispatch` dry-run → build+scan, skip push/sign.
  ⚠️ Off-tag refs make metadata-action's `type=semver` patterns produce ZERO tags — the dry-run
  must tag via a `version` input (`type=raw`, default `0.0.0-dry`), not semver, or build-push
  fails on an empty tag list. Also note: the existing `type=raw,value=latest,enable={{is_default_branch}}`
  (release.yml:38) never fires on tag refs — fix to enable on `v*` tags or drop `latest`.
- **TDD (infra):** dispatch the dry-run on a branch → assert scan+SBOM steps executed and
  publish steps skipped; the real proof is WO-6's tag run. Document the falsifiable asserts.
- **Verify:** `gh workflow run release.yml -f dry_run=true` green; YAML lint; no secret exposure.

### WO-3 — Canonical image path + Helm P0 ref · scope `deploy/helm/pulse/values.yaml` + `docs/runbooks/install.md` (warning only) · [S]
- **Now:** Helm references `ghcr.io/pulse-analytics/pulse:0.1.0` (values.yaml:13) — never
  published; release.yml publishes `ghcr.io/aytekxr/ams-pulse`. Any `helm install` → ErrImagePull.
- **Change:** canonical path = `ghcr.io/aytekxr/ams-pulse` (matches the repo; revisit at a rename).
  Fix values.yaml; add an "EXPERIMENTAL — full parity lands S6" warning to the install.md Helm
  section (full Helm parity is S6 scope — do NOT do it here).
- **TDD:** helm golden-file test (ci.yml `helm` job) updated RED first → template shows new ref.
- **Verify:** `helm template` + existing golden diffs green.

### WO-4 — Digest pinning + dependabot · scope `deploy/docker/pulse.Dockerfile` + `deploy/docker-compose.hardened.yml` + `.github/dependabot.yml` · [S]
- **Now:** golang builder (Dockerfile:18) and caddy (hardened.yml:36) float on tags (node+alpine+CH
  are digest-pinned); no dependabot/renovate at all.
- **Change:** pin both by digest (comment: refresh procedure); new `.github/dependabot.yml`
  covering gomod (`/server`), npm (`/web`, `/sdk/beacon-js`), docker (`/deploy/docker`),
  github-actions — weekly, grouped minor/patch.
- **TDD (infra):** falsifiable assert = rebuild the image from the pinned digest reproduces the
  same golang version; dependabot file passes `gh api /repos/.../dependabot` schema on push (or
  at minimum actionlint/yaml lint).
- **Verify:** full image build green; hardened staging stack boots with pinned caddy.

### WO-5 — Prod rollout carrying D-056 (ORCH, after WO-1..4 merged + CI green) · [M]
- **Now:** prod runs the pre-D-056 image → beacon ingest 401s; D-056 + D-055 + this session's
  image changes are unshipped. Backup cycle 2 due ~07:31 UTC today.
- **Change:** §8.7 **staging-verify first** on an isolated compose project (NOT pulse-prod) —
  boot the new image incl. one-shot migrate (D-054 lesson: every invocation env needs
  PULSE_SECRET_KEY), smoke the beacon path with a scratch ingest token (expect 202 with a mock
  license or 403-not-401 on Free — proves D-056). Then prod: rebuild image, 5-overlay combo
  `up -d`, §8.8 smoke: `/healthz` ok via `--resolve`, webhook signed→200/bad-sig→401, admin token
  authenticates, `pulse version` shows the stamp, limits bound, logs clean.
- **Backup check:** cycle-2 dated artifacts present + keep-7 intact; sidecar log shows next sleep.
- **Rollback:** runbook §5 (re-point to prior image). Never `down -v` on pulse-data.

### WO-6 — Branch protection + v0.1.0 + branch cleanup (ORCH, last) · [S]
- **Now:** `main` unprotected (API 404); zero tags; stale `ams-integration` on local+origin.
- **Change:** confirm `git log main..ams-integration` is empty → delete the ref locally + on
  origin; run `.github/branch-protection.sh` → verify via
  `gh api repos/aytekXR/ams-pulse/branches/main/protection` (200); push `v0.1.0` → watch
  release.yml → after publish: `cosign verify` the image, `docker pull` it, `pulse version` = v0.1.0.
- **Careful (verified 2026-07-08):** the script's contexts are already correct for today
  (`contracts server web sdk docker-build helm compose` — web-e2e correctly absent until S4,
  branch-protection.sh:30). It sets `required_pull_request_reviews` (1 approval) but
  `enforce_admins: false` (:32-37) → the owner's direct pushes to main — i.e. this session
  workflow — keep working. Do NOT set `enforce_admins: true` while sessions push directly to
  main. If the gh token lacks admin scope → hand WO-6 to the operator (O6) with exact commands.

## Gates (ORCH, before any commit)

- Full `-race` suite, REPO-ROOT mount, 0 FAIL / 0 unexpected SKIP (coverage unchanged is OK —
  this is an infra session; do not let it REGRESS below 59.5%/floor 58).
- Reproduce EVERY ci.yml step the changes touch: docker-build with the new args, helm golden
  diffs, web/sdk untouched-but-run (D-053/D-055 lesson: partial gate = CI red).
- `caddy validate` if any Caddy file moved; compose `config -q` for every overlay combo used.
- No secrets in any diff (`deploy/.env`, tokens, license keys).

## Closing protocol (ROADMAP §6 — session NOT done without these)

1. Commits by explicit path per scope (`deploy/... D-058: …`, `ci ... D-058: …`); push;
   `gh run watch` → green; verify the tag-triggered release run separately.
2. decisions.md D-058 (evidence: run IDs, cosign verify output, prod smoke results, backup cycle-2
   listing). RESUME-PROMPT ▶ START HERE → SESSION-02.
3. ROADMAP §3.S1 → ✅ (+refs); §4 ledger row (coverage unchanged); §5: O6 resolved or handed over;
   note prod now ≥ D-056.
4. **Write `sessions/SESSION-02.md`** from ROADMAP §3.S2 + actuals — re-verify its evidence
   anchors (coverage numbers per package from the gate run, the 15 uncovered api handlers, the
   conformance-harness trap lines) so S2 starts on fresh facts. If this session is cut short,
   SESSION-02.md = resume prompt for the S1 remainder instead.
