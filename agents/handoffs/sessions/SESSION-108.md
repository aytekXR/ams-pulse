# SESSION-108 — 2026-07-26 — review round 3 (R1–R15) executed; parallel artifact audit found 5 more

**Decision:** D-175. **Review record + disposition table:**
`docs/assessment/marketplace-compliance-review-2026-07-26-round3.md`.
**Operator directive:** "Add this to your fixes", then "get ready for the next external
review; commit, open the PR, merge after CI, and tell me when you're ready."

## The stale premise (read this before the next review)

The review's executive verdict and its P0 #2 both rest on "v0.4.2 cut locally, **tag not yet
pushed** — GitHub Latest is still v0.4.1". That was true when it was authored and **false by
the time it arrived**: v0.4.2 was released 2026-07-26 17:44 UTC (verified directly — `0.4.2`
and `latest` same-digest and anonymously pullable, cosign-signed, all assets attached; the
Helm chart OCI package anonymously pullable too).

Consequences: R1/R2 could not "land before tagging" — v0.4.2 shipped **with** them, so they
ride the next release; and R10's stale doc stamps were already public, not merely pending.

**Two of the last three external reviews carried a stale premise about what was published.**
Verify the reviewer's assumptions, not just their findings.

## Verification summary (verify-first, before any edit)

| Finding | Verdict | Note |
|---|---|---|
| R1 cluster streak events use cfg.NodeID | CONFIRMED | aggregator drops unknown-key `api_unreachable` → `node_degraded` dead for the whole outage |
| R2 discovery fabricates zeros on the same key | CONFIRMED | collides with the poller post-N2; overwrites every 30 s |
| R3 wrong/stale line citations + env table | CONFIRMED | 32 citations; one named the wrong file entirely |
| R4 helm configmap scoping + parity | CONFIRMED — **my first read was wrong** | I initially refuted the numeric sub-claim, re-checked, and confirmed it: the chart wired `max_server_memory_usage` to the per-query value (512 MB) vs the XML's 768 MB |
| R5 NaN → fabricated 0 | CONFIRMED | narrow (needs a JMX hiccup) but real |
| R6 AMS liveness fields ignored | CONFIRMED | `status` + `lastUpdateTime` both decoded and unused |
| R7 candidate tag never cleaned up | CONFIRMED — **fix as written was destructive** | see below |
| R8 chart version unchanged | CONFIRMED | `helm push` overwrites a published version |
| R9 `stream_ingest_error` unreachable | CONFIRMED | no aggregator/alert/API/UI surface |
| R10 three stale v0.4.1 stamps | CONFIRMED | guard reads only `head -1`, so structurally blind |
| R11 pubkey overwritten on re-run | CONFIRMED | same class as N5 |
| R12 grep anchor misses `export ` | CONFIRMED | re-introduces Issue-G data loss |
| R13 VPS IP in 8 spots | **ALREADY DISPOSITIONED** | D-174 keeps functional occurrences deliberately; no change |
| R14 compose-boot skips the release commit | CONFIRMED | the gate requires a green `ci` on exactly that SHA |
| R15 cross-surface node-ID mismatch | CONFIRMED | disclosed (LIM-28) rather than guessed |

### R7 — the review's suggested fix would have deleted the release

The review says to delete the `candidate-<sha>` tag on failure. A GHCR **package version is a
digest**, not a tag, and there is no delete-a-single-tag API; promotion re-tags the *same*
digest. Deleting it unconditionally on the success path would have destroyed the published
release together with its SBOM, provenance and cosign signature. Implemented instead: delete
only when the candidate tag is the digest's **sole** tag — exactly the un-promoted case.

## Found independently — parallel verify-first audit of the PUBLISHED artifact

A 19-agent audit ran the clean-room evaluator path, artifact integrity, submission-package
completeness, doc↔code↔image drift, debt triage, and a prod/security read. Ten findings
survived adversarial verification; five mattered and are fixed here:

1. **Production `CLICKHOUSE_PASSWORD` prefix in public git history** — 32 of 48 hex chars, in
   `server/cmd/pulse/migrate_test.go`, since `98b011c`: the test written to *prevent*
   credential leaks used the live password as its input. Source fixed; **rotation is operator
   item #1**. Not remotely exploitable (ClickHouse is Docker-internal), but public and
   findable with `git log -S`.
2. **`ss1-dashboard.png` — the flagship listing image — showed a dead product:** all 8 streams
   `UNKNOWN` state/health, and `0 viewers / 0 publishers` in BY APPLICATION. Cause: capture
   mocks used field names `schema.d.ts` does not define (`state` vs `publisher_state`,
   `viewer_count` vs `viewers`). React renders missing fields as fallbacks, so the capture
   "succeeded" every time.
3. **`ss3-alerting.png`** showed only the Rules tab against a caption promising incident
   history; its history mock (`fired_at`/`rule_name`) also mismatched the schema, so every
   cell was an em dash. The screenshot **spec itself was impossible** — it asked for rules and
   history in one shot on a tabbed screen.
4. **SDK install docs advertised the 0.4.1 tarball** — the build whose `Pulse.init()` was a
   silent no-op (N7).
5. **The documented README evaluator command published no port** — `expose:`-only base file,
   and explicit `-f` flags suppress the auto-override ⇒ healthy stack, unreachable UI.

Also: `licensing-public.md` gated analytics CSV export at Business+ (code gates it at Pro+ via
`CheckDataAPI`); the internal "*Proposed pending Ant Media confirmation*" note was still inside
the paste-verbatim listing copy.

**Refuted:** the audit's claimed licensor-name mismatch — `LICENSE` and `licensing-public.md`
carry an identical string. Roughly a third of agent/reviewer claims don't survive checking.

## Shipped

Full inventory in D-175. Code (4 defects, all regression-tested): R1 streak fan-out, R2
conditional emission + anti-drift test, R5 presence-flagged readings, R6 AMS liveness.
Pipeline: R7 scoped cleanup, R8 chart 0.3.1 + guard #16, R14 local build, guard 13→16.
Docs/assets: R3, R4 (+5 goldens, `helm-golden-update` now covers all 5 rather than 3),
R9 LIM-27, R10, R11/R12, R15 LIM-28, screenshots regenerated, evaluator overlay.

## Gates

PR **#220 — 19/19 green**, squash-merged; post-merge `ci` + `e2e` + `codeql` on `main` all
green. PR **#221** (docs prep) follows. Local: server build + gofmt + vet + full `-race`;
SDK 70/70 @ 3.52 KB; contracts ajv + fixtures + redocly; shellcheck; actionlint; helm lint.

**Evaluator path verified live on the VPS** in an isolated project: HTTP 200, all components
`ok`, UI served, bound to `127.0.0.1` only; torn down with volumes, prod untouched.

**Known flake (tracked, not a regression):** `SettingsPage` tabpanel ARIA wiring fails only
under full-suite parallel execution locally and passes in isolation; CI ran it green.

## Deliberately NOT done

No prod roll (prod stayed v0.4.0-139, collector `ok`, 719 rows/h throughout) · no new tag —
these fixes ride the next release · no outbound/marketplace action · R13 left as D-174 decided.
