# External marketplace review — Round 4 (2026-07-27) · review + disposition

**Reviewed tree:** `bea0108` on `main` (`v0.4.2-3-gbea0108`) — the reviewer's tree-state
table was verified exactly correct, including the tag position, the 3-commit delta, chart
0.3.1 on main, and the release timestamp. **This is the first round with no stale premise
to correct.**

**Executed in:** SESSION-109 / D-176.

**Reviewer's verdict:** *"the residual risk is now concentrated almost entirely in claims,
not code, and claims are the cheapest thing in this repo to fix."* Assessed as fair and
acted on accordingly.

**Reviewer's blocked checks (accepted as honestly declared, not treated as passes):**
anonymous `docker pull` / cosign / SBOM inspection (sandbox proxy forbids ghcr.io),
`go test` on main (Go 1.25 toolchain download blocked), live cluster behaviour, and DNS
resolution of the beyondkaira.com hosts.

---

## Method

Per the standing protocol (`RESUME-PROMPT.md` §"when its findings arrive"), **every finding
was verified against the tree before any fix was written**. Round 3's lesson — roughly a
third of reviewer claims historically did not survive verification — held less this time:
this reviewer was accurate almost throughout.

**Result: 14 of 14 findings CONFIRMED. One embedded sub-claim REFUTED.**

The refuted sub-claim is recorded below rather than silently dropped, because the
disposition table is itself something the pack trades on (F-11's point).

---

## Disposition table

| ID | Verdict | Disposition |
|---|---|---|
| **F-01** Prod ClickHouse password prefix in public git history; rotation pending | **CONFIRMED — still open** | Verified directly this session *without printing the secret*: the live 48-char `CLICKHOUSE_PASSWORD`'s first 32 characters still return hits under `git log -S`. The source scrub on `main` is confirmed genuine (dummy literal + explanatory comment; zero tree-wide traces of the old value). **Operator-gated — remains queue item #1.** No code action available to the loop. |
| **F-02** `release-notes.md` claims unshipped/false behaviour for 0.4.2 | **CONFIRMED (both halves)** | (a) `stream_ingest_error` has **zero read paths** (only the domain constant, the webhook write, and migration 0011) — reworded to "recorded to ClickHouse… queryable today; dashboard/alert not shipped (LIM-27)". (b) The plain compose path publishes **no ports** and CI boots base **+ evaluator overlay** — reworded to name the two-file evaluator command and to state that the base file deliberately publishes nothing. |
| **F-03** README/LIM-10 assert cluster capabilities that are structurally inert | **CONFIRMED** | Verified the whole chain: `client.go` marks `role`/`version` as alias-only ("AMS has no role field on this endpoint"), `discovery.go` defaults every node to `origin`, `IsEdgeStream` requires `role == "edge"`. **Fixed wider than the review asked:** the claim had propagated to **six** documents, not two — README (2 rows), `overview.md`, `product.md`, `api-guide.md`, `ARCHITECTURE.md` (2 rows), and **`demo-video-script.md`, the voiceover the operator is about to re-record**. LIM-10 reframed from a *confidence* gap to the *provable* fact. `LiveOverview`'s hardcoded `"standalone"` role now resolves through cluster discovery like `FleetNodes` (code fix). |
| **F-04** Degraded-node ladder still unreliable during cluster API outages | **CONFIRMED (all four mechanisms)** | Verified: `Degraded()` needs `ConsecAPIErrors >= 3` while `wireNodeEviction` uses the same `3×PollInterval`; discovery's event map omits `consec_api_errors` (the poller adds it post-normalize) and the aggregator full-replaces; `poll()` returns on `resolveApps` error before the cluster branch. **Disclosed, not fixed** — added to LIM-10 as explicit product behaviour, and the CHANGELOG's R1 entry now carries a scope correction. Rationale: every candidate fix retunes alert timing, and there is no live cluster to verify against; trading a missed alert for a false one is not an improvement. Tracked debt, gated on the operator's 2-node cluster. |
| **F-05** R6's `down` state is invisible; `lastUpdateTime` unit unverified | **CONFIRMED** | Verified `status="down"` is consumed only by the inert `IsEdgeStream`, is absent from the emitted event, and that the Fleet API has no `down` value at all. **Two fixes shipped:** the `lastUpdateTime` epoch-ms assumption is now recorded as an **explicit unverified assumption** in `AMS-INTEGRATION.md` §1.1 (with the "if AMS emits seconds, every node is silently down" consequence spelled out), and `qa/mock-ams` no longer hardcodes a **2024** timestamp — which had put every mock cluster node permanently in the `down` state with nothing surfacing it. Surfacing `down` through event → aggregator → Fleet API is disclosed in LIM-10 and tracked (it changes the Fleet UI contract). |
| **F-06** Mode-flip blind window; probe-unreachable re-opens the N1 hole | **CONFIRMED** | Verified the early-return, the success-only clearing of `lastClusterNodeIDs`, and that `Discovery.nodes` has no `delete()` anywhere. Disclosed in LIM-10 (the ~4–20 min window is stated). Not fixed for the same reason as F-04. |
| **F-07** Trivy candidate-tag cleanup cannot execute (wrong token + endpoint) | **CONFIRMED — and worse than described** | The reviewer expected the warning branch; in fact the **list** call fails first with `GITHUB_TOKEN`, so the step exits **0** via the "nothing to clean up" notice — reporting success while never deleting anything. **Fixed:** switched to `/users/{owner}/packages/...`, a dedicated `GHCR_CLEANUP_TOKEN` secret, an explicit HTTP-status assertion, and a loud warning when the secret is absent. CHANGELOG entry corrected. New optional operator item (#9) — nothing breaks without it. |
| **F-08** Quickstart `PULSE_HOST_PORT` broken on the documented path; re-runs hard-fail | **CONFIRMED (both)** | Verified `install.sh` pins `PULSE_REF=v0.4.2` and that the **tag's** compose hardcodes `8090:8090` while only `main`'s parameterises it. **Fixed:** the port preflight now exempts a listener owned by the quickstart's own stack (restoring the idempotent re-run path N5/R11/R12 exist to protect); a new preflight **fails loudly** when the pinned compose cannot honour a requested `PULSE_HOST_PORT` instead of health-polling a dead port for 90 s; the header's `\| bash` example corrected to `\| bash -s --`. The parameterised compose itself ships by **cutting v0.4.3**. `install.md`'s `main`-ref downloads were **kept deliberately** (the compose carries its own image pin, so that path is self-consistent; repointing to the tag would reintroduce the hardcoded-8090 bug) — rationale now documented in place. |
| **F-09** Helm deprecated-alias promise inverted; guard #16 inert | **CONFIRMED (both)** | Verified `\| default` can never fall back while both keys are chart defaults, and that `actions/checkout@v4` at depth 1 leaves `PREV_TAG` empty so #16 silently no-ops. **Fixed:** the default moved out of `values.yaml` into the template so precedence is genuinely *new key → deprecated alias → 768 MB*; **verified by rendering all four cases** (defaults / alias-only / new-key-only / both) — the alias-only case now correctly yields 6 GB instead of silently snapping to 768 MB. Added `fetch-depth: 0` + `fetch-tags: true`; #16 now **fails loudly** rather than skipping when it cannot resolve the previous tag, and its diff is scoped to `templates/` + `values.yaml` so it no longer degenerates into "always bump". Goldens regenerated with CI's pinned helm 3.17.0 — comment-only diff, rendered values unchanged. |
| **F-10** `install.md` residue batch (tier inversion + 6 more) | **CONFIRMED — counts re-verified independently** | The Business tier row said **5** nodes against `license.go`'s 50 → fixed. Schema counts re-counted from `contracts/db/`: meta **16** (not 14), ClickHouse **11 tables / 7 MVs** (not 9/5) — the reviewer's numbers were right and my own first count was wrong; corrected against the contracts, with the two missing table names added. "All variables are listed below" (38 rows vs the 69 `admin-guide.md` documents) → relabelled a subset with a pointer. Clean-install status contradiction → aligned to the D-168 fact. `make up` recommended as the released-image path → replaced with the actual released-image command plus an explicit "do not use `make up` here". README's quickstart-directory link → replaced with the runnable command. README's build-from-source compose → evaluator overlay added so it yields a reachable UI. |
| **F-11** Disposition-table overstatements | **CONFIRMED** | All corrected: the `0.4.1` pull narration in `submission-package.md`; the two missing R15 code comments (added at the stream and publish-end sites); "roughly 50" `PULSE_*` vars → **69**; "four more" → **five** in the round-3 record. The "All 14 are fixed" line in `operator-expected.md` is superseded by this session's rewritten status block, which states plainly that R9/R15 were **disclosed**, not fixed. |
| **F-12** Production IP: rationale sound, "functional" framing wrong for 3 of 8 | **CONFIRMED** | Verified `deploy/nginx/ams.beyondkaira.com.conf`'s live directive is `proxy_pass http://127.0.0.1:5080` — the IP survives only in a comment about the **retired Caddy** path, so naming that file as the *functional* example was wrong. All three non-functional occurrences scrubbed (the nginx comment and both `self-hosted-ams.md` lines). The five genuinely functional ones (the `curl --resolve` commands and the load-lane guard) are kept, per D-174. Rotation (F-01) remains the part that matters. |
| **F-13** Disclosure gaps: LIM-28 depth, LIM-01 field names, Kafka EXPERIMENTAL | **CONFIRMED** | LIM-28 extended with the consequences the reviewer correctly identified: apps on other nodes are **invisible rather than mislabelled**, per-viewer QoE via REST is largely absent on clusters, and — newly documented — **`PULSE_AMS_URL` must point at one origin node; load balancing breaks `detectEnded` and the per-host cookie jar**. LIM-01's `cpuUsage`/`memoryUsage` corrected to the real wire names `cpu`/`memory`. EXPERIMENTAL marker added at `README.md` and `install.md`'s Kafka rows. |
| **F-14** Smaller verified items (batch) | **CONFIRMED** | Fixed: `CPUPctOK`/`MemPctOK` returning `(0, true)` when the field is absent entirely — a fabricated measured-0% through a different input than R5 fixed; **the test that pinned this as correct was rewritten** and a new regression test added. Poller now skips identity-less cluster DTOs (discovery guarded this; the poller did not). Guard #13's substring match anchored (`0.4.20` would have passed a `0.4.2` release). `alerting.md`'s two false `node_degraded` descriptions corrected. `compatibility.md` stamp refreshed. README's "prod runs v0.4.2" corrected to the true v0.4.0-139. Remaining sub-items (R1 fan-out row volume — cosmetic per the reviewer's own analysis; guard #11's false-positive *risk*, which no current content triggers) left as-is. |

### Refuted

| Claim | Why it does not hold |
|---|---|
| F-13: *"LIM-28's roadmap cites a broadcast `originAdress` field that exists nowhere in the tree (unverifiable forward claim)"* | The field **is** in the tree: `server/pkg/amsclient/testdata/broadcasts_real_liveapp.json` and `broadcasts_real_test123_v303.json` — **real AMS 3.0.3 response captures**. It is not yet decoded into the DTO, which is presumably what the reviewer's search matched on, but the roadmap claim is grounded in captured wire data rather than speculation. LIM-28 now says so explicitly, with the path. |

---

## What this round did **not** change

Deliberate non-actions, stated so they are not mistaken for oversights:

- **F-04/F-05/F-06 cluster rework** — disclosed in LIM-10, not fixed. Every candidate fix
  changes alert timing, and no live multi-node cluster exists to verify against. The repo's
  standing rule is to verify rather than guess; guessing here risks trading a missed alert
  for a false one. Gated on operator queue item 7.
- **`install.md`'s `main`-ref downloads** — kept, with the rationale now documented. The
  compose file carries its own image pin, so that path is self-consistent; repointing it at
  the tag would hand users the hardcoded-8090 compose that F-08 is about.
- **Guard #11's false-positive risk** — real, but no current README content triggers it, and
  every plausible tightening weakens the check it exists to perform.

---

## Gate results at close

See `agents/handoffs/sessions/SESSION-109.md` for the full run log.
