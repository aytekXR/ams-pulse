# SESSION-106 — 2026-07-26 — D-173: second external marketplace review (REVIEW-MP2-2026-07-25) verified & executed

**Operator input:** the full text of a second independent "Ant Media Marketplace Compliance &
Readiness Review" (dated 2026-07-25, snapshot = v0.4.1 + S105 unreleased) + "using workflows look
at those external reviews and fix them if relevant". Ultracode on. The review is recorded verbatim
in `docs/assessment/marketplace-compliance-review-2026-07-25.md`.

## Protocol

1. **Gate reads first** (per RESUME-PROMPT): PR #214 merged, CI green; prod collector component
   `ok`, exactly 720 rows/h over the last 24 h (last event seconds-fresh), 1,306,068 total rows;
   open PRs = the operator-held Dependabot queue only. Prod v0.4.0-139 untouched all session.
2. **Verify before fixing** — 14-agent adversarial workflow (`wf_06452167-d71`): one verifier per
   review issue (A–L, P3 batch) + one dedicated agent fetching the actual AMS source from GitHub.
   Verifier fix-plans were written to `.s106-plans/` (session-scratch, not committed).
3. **Live probe** (orchestrator, read-only GETs against the real standalone AMS 3.0.3 Enterprise,
   cookie auth): flat `/rest/v2/cluster/nodes` → **404**; paginated `/cluster/nodes/0/50` →
   **500** `NoSuchBeanDefinitionException: No bean named 'tomcat.cluster'`; `/cluster-mode-status`
   → `{"success":false}`. This contradicts the review's "the paginated route also 404s outside
   cluster mode" assumption and shaped the fix design (probe first, 404+500 → standalone).
4. **Fix** — 9-lane ownership-partitioned workflow (`wf_a0d72b9e-5c2`): go-cluster,
   go-license-webhook, deploy, helm, contracts, docs-marketplace, docs-hygiene, sdk, ci.
5. **Orchestrator gates + followup closure** (below), session records, single commit, PR.

## Verification verdicts (headline evidence)

| Issue | Verdict | Note |
|---|---|---|
| A cluster flat-vs-paginated | CONFIRMED (source + live) | `ClusterRestServiceV2.java` fetched at tag `ams-v3.0.3` AND master: class `@Path("/v2/cluster")`, methods only `node-count`, `nodes/{offset}/{size}`, `DELETE node/{id}`. Verifier bonus find: `ClusterNodeDTO` decoded invented keys — only `ip` overlaps the real wire shape (`id`, `lastUpdateTime`, `memory`, `cpu`, `dbQueryAveargeTimeMs`, `status`), so nodes would decode all-zero even with correct pagination. |
| B v0.4.2 uncut | CONFIRMED | 8 functional 0.4.1 pins listed; guard covers 4 files, misses all 8; tagging is manual-annotated. |
| C base-compose CH auth | CONFIRMED | Base file has zero `environment:` on clickhouse; README `-f` command skips the override; CI only ran `config --quiet`. |
| D helm defects | CONFIRMED | All 9 sub-claims, incl. both new-variant proofs. |
| E listing copy | CONFIRMED | All 9 sub-claims (identifier count methodology differs, materially true). |
| F licensegen 5 nodes | CONFIRMED | `buildEntitlements` is claim-driven (by design); mint tool was the defect; test at :136 masked it. |
| G quickstart traps | CONFIRMED | All 5 (trap deletes .env with stack running; key regen; ok-only gate vs degraded-200; main-branch download; no CI). |
| H helm CH default user | CONFIRMED | existingSecret branch changed only comments; probes are in-pod (fix is safe). |
| I OpenAPI | **PARTIAL** | servers-drift + WS-subprotocol + /metrics prose CONFIRMED; "`?token=` rejected" **REFUTED** — `downloadAuthMiddleware` (server.go:784) accepts it; the review confused it with `bearerAuthMiddleware`. `wsTokenQuery` stays in the spec. |
| J error webhooks + GET-only | CONFIRMED | |
| K hygiene | CONFIRMED | incl. `operator-expected.md` "leak cleaned" claim vs 5 files still carrying the IP. |
| L CI fidelity | CONFIRMED | All 11 sub-claims. |
| P3 batch | 22/23 CONFIRMED | (4a) REFUTED — AMS-INTEGRATION.md:360-361 does document HTTP-Basic source-test. |

## What shipped (by lane; all lanes ran under strict file-ownership, no CHANGELOG/handoff writes)

- **go-cluster** — `ClusterNodes()`: `ClusterModeStatus()` probe (mutex-guarded cache,
  re-probe every 60 calls ≈ 5 min at the 5 s cadence) → paginate `{offset}/{size}` (50/page,
  short-page terminator); 404 AND 500 on the paginated path → standalone fallback (cache
  updated). DTO realigned to the real AMS wire shape with tolerant legacy aliases +
  `PrimaryID()/CPUPct()/MemPct()` accessors (consumers in normalize.go/discovery.go updated;
  AMS serializes no `role` on this endpoint — role now empty/unknown). Dead `NodeInfo`
  deleted. WebRTC-stats poll errors now logged. mock-ams: flat route 404s, paginated 500s
  when standalone, `-cluster` flag serves multi-page real-shape fixtures; added
  `system-status`, `version`, `vods/list`, `users/authenticate` (session cookie), strict
  segment matching (no more `strings.Contains "/list"`); header comment fixed. All flat-path
  test stubs migrated; new pagination-boundary / standalone-500 / mode-status tests.
  ⚠ This lane died at the report stage (session limit) — its edits were complete; the
  orchestrator re-ran its gates and reviewed the diff line-by-line.
- **go-license-webhook** — licensegen Business 5→50; :136 test → 50; `TestMintPathNodeLadder`
  locks the whole ladder. Webhook: `endpointFailed`/`publishTimeoutError`/`encoderNotOpenedError`
  → new `stream_ingest_error` domain event (schema enum + JSON/form tests); unknown actions
  counted+logged. `PULSE_AMS_LOGIN_EMAIL` `_FILE` support. Placeholder-`changeme` secret-key
  rejection at boot (serve + migrate; no CI lane boots with placeholder values — verified).
  Descope: `server/internal/config` NOT deleted (cmd/pulse imports `GetSecret`); `Load()`
  marked not-wired in-code instead.
- **deploy** — base compose `CLICKHOUSE_SKIP_USER_SETUP: "1"` (expose-only ports, commented);
  prod+hardened CH healthchecks use `$$`-runtime expansion (no password in `docker inspect`);
  backup compose `CLICKHOUSE_USER` default; quickstart compose plain-HTTP warning block;
  `install.sh`: CREATED_ENV/STACK_STARTED-scoped cleanup, secret-key reuse, ok-OR-degraded
  gate + component-scoped collector diagnosis (warn+exit 0 when Pulse is up but AMS is
  unreachable), `PULSE_REF` for all raw downloads (main now; the cut flips it — guard-enforced);
  `clickhouse-low-footprint.xml` created (was referenced but missing; never existed in git);
  `.env.example` PULSE_BASE_URL text corrected.
- **helm** — `PULSE_INGEST_LISTEN_ADDR` (default `:8091`) + `PULSE_REPORTS_DIR` rendered; PVC
  created whenever `pulse.persistence.enabled` (postgres mode schedulable); real S3 env names
  + bucket/endpoint/region/prefix values; duplicate `volumes:` key fixed; existingSecret →
  default user localhost-only + `access_management 0`; opt-in ClickHouse NetworkPolicy;
  chart 0.2.0→0.3.0; README/NOTES truthful; goldens regenerated + 2 new variants
  (`ch-persistence-off`, `existing-secret`).
- **contracts** — per-path `servers: [{url: /}]` on the 8 root-mounted paths; WS
  `Sec-WebSocket-Protocol: pulse.v1, <token>` documented; `/metrics` Business+ gate stated;
  `/admin/sources` honest ("stores config; does not start extra pollers"); `schema.d.ts`
  regenerated (JSDoc-only diff, zero type churn); `docs/api/index.html` rebuilt via redocly.
- **docs-marketplace** — `listing.md` created (clean external copy; PolyForm disclosure on
  Free; tier fixes; egress caveat; long description; category proposed; zero internal IDs —
  grep-proven); `listing-draft.md` demoted to internal; `release-notes.md` plain prose;
  `submission-package.md` re-indexed; `submission-process.md` §4 honest matrix wording.
- **docs-hygiene** — VPS IP → `<VPS_IP>` in the 5 review-cited files; `final-assessment.md`
  restamped v0.4.1 + superseded-notice + rows 7/8/12 reconciled; `licensing.md` official-key
  default; `licensing-public.md` §6 → `listing.md`; G-21 SETTLED wording in
  compatibility.md/known-limitations.md (LIM-10 rewritten: source+standalone-live verified,
  multi-node pending); AMS-INTEGRATION endpoint table updated (paginated + probe; NodeInfo
  row gone; PULSE_SECRET_KEY hard-fail truth; read-only wording with the login-POST caveat,
  also in faq.md); install.md P3 fixes (migrations baked in, base-compose migrate,
  `pulse_meta.db`, Go 1.25); productionize.md synced.
- **sdk** — beacon-js `SDK_VERSION` now build-time-injected from package.json (tsup
  `define` + `tsup.config.ts`; vitest define mirrors it; new test locks constant==package
  version); beacon-swift `pulseBeaconSDKVersion` → 0.4.1. ⚠ Lane died mid-report
  (connection); edits complete; orchestrator ran build/test/size: **3.52 KB / 15 KB gate**.
- **ci** — matrix workflow renamed "mock-ams wire-format profile tests" + real smoke path
  (`/LiveApp/rest/v2/broadcasts/list/0/10`); release.yml: Trivy scans the pushed multi-arch
  digest (both arches), e2e required for the release SHA, linux/amd64+arm64 binaries +
  SHA256SUMS attached, chart pushed to `oci://ghcr.io/aytekxr/charts`, version guard 4→10
  checks (compose ×2, quickstart compose, install.sh pin + PULSE_REF, helm values tag, helm
  README tag, SDK package.json, swift constant); ci.yml: shellcheck job, compose-boot job
  (exact README command, component-scoped healthz assertions, accepts collector-degraded).
- **orchestrator (session lead)** — kin-openapi v0.140.0 sticky path-level-servers router bug
  found live (NewRouter reuses the `servers` var across path iterations): fixed via
  `newSpecRouter` helper pinning explicit servers per path item; conformance tests updated to
  use real root paths (removing the old `/api/v1/healthz` workaround that encoded the spec
  bug). ci.yml golden diffs extended to the 2 new variants. IP redacted in capability-map +
  both review records. `.env.example`: changeme-rejection note, EMAIL `_FILE` doc.
  `config.Load` not-wired comment. CHANGELOG/D-173/session records.

## Deliberate keeps (not defects)

- VPS IP stays in: `agents/handoffs/**` (internal historical records, per repo convention),
  `deploy/nginx/ams.beyondkaira.com.conf` (vhost for a domain that publicly resolves there),
  `qa/realams/harness/load-env.sh.example` + `qa/realams/README.md` (the IP is the load-lane's
  prod-refusal **blocklist** — redacting would weaken the guard), `docs/operator-expected.md`
  (historical session headers).
- `wsTokenQuery` stays in the OpenAPI spec (review claim refuted).
- Kafka stays EXPERIMENTAL until AV-15; live multi-node cluster validation still pending.

## Gates (all orchestrator-run, container-based; no native Go on this host)

- Server: `gofmt -l` clean · `go vet ./...` clean · **full suite 26/26 pkgs ok** (golang:1.25,
  repo-root mount for the meta DDL).
- Web: `tsc --noEmit` clean · **682/682** vitest (one flaky act()-race failure under parallel
  load; clean re-run 682/682).
- SDK: tsup build ok · vitest ok · **size 3.52 KB / 15 KB**.
- Helm: lint ok · **all 5 goldens byte-match** fresh alpine/helm:3.17.0 renders.
- actionlint (rhysd/actionlint docker): clean on ci/release/matrix workflows.
- shellcheck: `install.sh` clean.
- Compose: `config --quiet` ok on every changed file.
- Prod: untouched, healthy (v0.4.0-139, collector `ok`, 720 rows/h steady).

## Workflow failures worth remembering

Two of nine fixer lanes died AFTER completing their edits but BEFORE reporting (go-cluster:
session limit; sdk: connection drop). Diagnosis path that worked: `git status` first — both
lanes' files were fully modified; the full test suite then proved go-cluster's work sound.
Re-running the lanes blind would have double-applied edits over a dirty tree. **Rule: on a
lane failure, assess the tree before resuming the workflow.**

## SESSION-107 job

Cut **v0.4.2** (S105's standing goal, now the only gate to an evaluator-consistent artifact).
⚠ Sequencing changed by this session's guard extension: the 8 deploy-surface pin repoints +
`PULSE_REF` → `v0.4.2` must land in the SAME pre-tag commit as the VERSION/Chart-appVersion/SDK
bumps + CHANGELOG `[Unreleased]`→`[0.4.2]` roll + helm-golden regeneration (appVersion label
churn), because the 10-check guard asserts all of them at tag time. Known transient: the new
compose-boot CI job pulls the pinned image tag, so on the cut PR it will fail until the 0.4.2
image exists — accept that one red job, merge, push the annotated tag on the merge commit
(release.yml builds/signs the image, attaches binaries+SUMS+SDK tarball, pushes the chart to
GHCR OCI — first run of those steps, watch them; npm iff NPM_TOKEN), then re-run compose-boot
→ green. Finish with the anonymous clean-room pull check (D-168 pattern) against 0.4.2 and a
docs pin-reference sweep. NOTE: the chart OCI package will start PRIVATE on GHCR and only the
operator can flip it public (no API — see memory/S101).
