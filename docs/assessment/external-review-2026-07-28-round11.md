# External review — Round 11 · FINAL (2026-07-28)

**Reviewed tree:** `6c5dbd9` (`main`, `v0.4.4-6-g6c5dbd9`) — the D-184 round-10-dispositions commit · **published Latest:** still tag `v0.4.4` = `34a25fc` (unchanged; no `v0.4.5`)
**main at review time:** same `6c5dbd9`
**Reviewer:** Claude (Cowork session; same reviewer as rounds 6–10) · **Phases completed:** blackbox (delta) / docs / code / security-fix verification
**Finding ID prefix:** `N-` (M- was consumed by D-184's own self-audit items M-01…M-04, so this round skips to N- to avoid a ledger collision)
**Prior-round input:** `docs/assessment/external-review-2026-07-27-round10.md` (Disposition column filled by D-184)
**Verdict:** **Ready to submit once G-02 (`CLICKHOUSE_PASSWORD` rotation) closes.** This is a legitimate final round *because it did adversarial work rather than bless the tree*: it verified D-184's three fixes against the code, found one further residual of the exact SSRF class D-184 was closing (N-01, LOW), and **retracts a MEDIUM finding of my own (L-01) that turned out to be a mirror artifact.** The single blocker is unchanged and operator-only; one tag cut carries the accumulated fixes to the marketplace artifact.

---

## Preface — round 10 was wrong to declare convergence, and this is why round 11 exists

Round 10 concluded "the product has converged; stop reviewing." D-184 then took my ~90%-reassurance round, attacked its all-clear across nine adversarial lanes, and **found three real defects my security pass had missed** (two of them security). That is decisive evidence that *declaring done from a reassurance-heavy round is exactly when defects hide.* So round 11's job was not to re-bless — it was to (1) independently verify D-184's fixes against the tree, (2) continue the adversarial sweep one step further than D-184 did, and (3) own my errata precisely. All three are below. The stopping verdict at the end is therefore stated as a *decision with named residual risk*, not as "the product is provably complete."

---

## Environment and limits

- Device bridge up; source read grep-exact at `6c5dbd9`. **Critical correction to my own prior understanding (see Errata E-1):** the folder my bridge mounts is a **mirror** that receives the maintainer's commits but does **not** propagate my writes back to the canonical repo. My `device_commit_files` "written" successes in rounds 6/9/10 landed in that mirror; per D-184's five-probe verification, the round-6/round-7 ledgers never reached the real repository. This round I treat the maintainer's in-repo `git` probes as authoritative for the canonical tree and my mount as a read-only view of their commits.
- Published surface: Latest still `v0.4.4` (confirmed prior round; no tag has appeared). No black-box re-run — no new artifact exists.
- **Could NOT be tested (standing):** `docker pull`/run, `cosign`/`helm` execution, live AMS, runtime SSRF dial (I verify the guard is *wired*, not that a live socket refuses IMDS), live multi-node cluster, SHA256SUMS bytes.

---

## Findings

| ID | Sev | Phase | Subject | Claim (falsifiable) | Evidence | Proposed fix | Disposition |
|---|---|---|---|---|---|---|---|
| N-01 | LOW | code/security | The AMS **poll-loop** client is the 9th outbound client and is not SSRF-guarded, and create-source validates scheme only | `amsclient.New` (`client.go → New`) builds `&http.Client{Timeout:…}` with **no `DialControl`**; `amsSourceFromAPI` validates only that `rest_url` is http/https (**no `ssrfguard.IsDenied`**); create and test are separate endpoints, so a source with `rest_url=http://169.254.169.254/` is accepted at create and then dialled every interval by the collector with no guard — the one operator-supplied outbound client outside D-184's M-01 8-client sweep | `server/pkg/amsclient/client.go → New` (no DialControl); `server/internal/api/server.go → amsSourceFromAPI` (scheme-only check; the only `IsDenied` in the file is in `handleTestSource`); `serve.go → amsclient.New(...)` wires the collector client; `restpoller`/`serve.go` grep for `DialControl` → none | Wire `ssrfguard.DialControl` into `amsclient`'s `http.Client.Transport` (mirror `webhook.go`/`s3.go`), and/or add `ssrfguard.IsDenied` to `amsSourceFromAPI` — closing both the create-time literal and the poll-time rebind. Impact is **bounded** (see detail): the poll loop only ever requests fixed AMS paths, so IMDS credential paths are unreachable; authenticated-operator only | **GAP CONFIRMED · REACHABILITY REFUTED · SEVERITY RAISED · FIXED (D-185).** Fact (a) holds exactly as filed. Facts (b) and (c) hold in isolation but the chain they build does not: `ams_sources.rest_url` is **never polled**. Its only consumers are the *guarded* `handleTestSource`, the API read shape and the write path (`server.go:2008,2021,2736,2768`); the poll client is built once at `serve.go:241` from `cfg.AMSBaseURL` = `PULSE_AMS_URL` (`cmd/pulse/config.go:242`). No API-supplied URL reaches an unguarded dialer, so the proposed `POST /admin/sources` reproduction does not reproduce. **The bounding argument is refuted in the other direction:** `CheckRedirect` follows up to 10 hops to a `Location` the *responder* picks, so the request path is not ours to bound — and `getJSON` copies up to 4 KB of a non-2xx **body** into the error that `restpoller` stores and **unauthenticated** `/healthz` republishes (`client.go:650` → `restpoller.go:150` → `server.go:999`; `/healthz` is registered under "Operational (unauthenticated)", `server.go:446`). That is not blind SSRF and not merely link-local reachability. Fixed at the dialer plus the disclosure channel — see the maintainer close below |

---

## Finding detail

### N-01 — AMS poll-loop outbound client is unguarded; create validates scheme only  [LOW · code/security]
**Claim.** D-184's M-01 correctly swept the outbound clients and guarded email, cert-expiry, and S3 (bringing the guarded set to eight). The **collector's AMS poll client is a ninth** and was in neither bucket. Three verified facts combine:
1. `amsclient.New` (`server/pkg/amsclient/client.go → New`) constructs `hc := &http.Client{Timeout: timeout}` with a `CheckRedirect` but **no `net.Dialer.Control = ssrfguard.DialControl`** — confirmed by grepping every `DialControl` site in `server/` (the collector/restpoller/serve.go wiring has none).
2. `amsSourceFromAPI` (`server/internal/api/server.go`) validates `rest_url` **only** for an http/https scheme (`"rest_url must use http or https scheme"`); it does **not** call `ssrfguard.IsDenied`. The sole `IsDenied` in the file is inside `handleTestSource` (a different endpoint).
3. `handleCreateSource` does not invoke the test path. So `POST /api/v1/admin/sources {rest_url:"http://169.254.169.254/"}` is accepted, and the collector then dials that address on every poll with the unguarded client.
**Reproduction (proposed — not executable here).** As an authenticated admin: `POST /api/v1/admin/sources` with a link-local `rest_url`; observe the source created (create does not run the SSRF-guarded test); the restpoller dials it. Contrast `POST /api/v1/admin/sources/{id}/test`, which *does* refuse it at dial time.
**Why it matters — and why it is LOW, not higher.** It is a real instance of the exact class M-01 exists to close ("the guard was wiring-dependent; fix each component's own dial"), and the asymmetry is telling: the *test* path is dial-time rebinding-safe while the *actual poll* path is not, which reads as an oversight, not a deliberate exemption (an exempt surface wouldn't guard the test path either). **But the blast radius is small:** the poll loop only ever appends fixed AMS request paths (`/rest/v2/applications`, `/rest/v2/version`, `/{app}/rest/v2/broadcasts/list/...`), so unlike the S3 client (operator-controlled path → could reach IMDS credential endpoints), this client **cannot fetch the IMDS credential path** — it is SSRF *reachability* to link-local, not credential exfiltration, and requires an authenticated operator. That places it in the email/cert-expiry impact tier, at the low end.
**Proposed fix.** Add `Control: ssrfguard.DialControl` to `amsclient`'s dialer (one transport, mirroring `webhook.go`); optionally also `ssrfguard.IsDenied` in `amsSourceFromAPI` so a literal is refused at create with a named error. Either alone closes the literal case; the dialer hook additionally closes DNS-rebinding on the poll path.
**Confidence.** High on the mechanism (all three facts read directly from the tree at `6c5dbd9`, not a mirror artifact). Medium on whether it merits fixing before submit vs. as fast-follow — I lean fast-follow, given the bounded impact, but it is the kind of consistency gap a security-literate marketplace reviewer will grep for after seeing the guard advertised.

---

## Prior-round re-audit — D-184 dispositions and self-found items, verified against the tree

Per the brief's core rule, verified against code at `6c5dbd9`, not the changelog.

| ID | D-184 said | Verified state now | Note |
|---|---|---|---|
| **L-01** (my round-10 MEDIUM: review-chain asserts a falsehood) | "objection confirmed, evidence refuted" | **EVIDENCE RETRACTED — maintainer is correct** | See Errata E-1. Their five-probe verification (`porcelain --untracked-files=all --ignored` empty; `git log --all` no history; `find -xdev` only rounds 8/9; one worktree; no index.lock) is authoritative for the canonical repo; my "files exist" evidence was a **mirror** artifact I misattributed. The *objection* (the doc overreached by claiming what my deliveries contained) was adopted — review-chain.md now scopes its claim to "what reached this repository," which is correct and is a real precision improvement. Net: finding right to object, wrong on evidence and severity |
| **L-02** (my round-10 LOW: README stamp lag) | "refuted as filed; class 4× wider; closed mechanically" | **VERIFIED FIXED (better than filed)** | The README stamp was actually correct; the real drift was in `faq.md`/`overview.md`/`ARCHITECTURE.md` + four non-date stamps. Now enforced by `.github/check-doc-stamps.sh` + a CI job (the fourth appearance of this class, correctly closed with a guard rather than a manual edit). Confirmed the script and ci.yml job exist in the D-184 diff |
| **M-01** (D-184 self-found, SECURITY): SSRF guard missing from email/cert-expiry/S3 | fixed at each component's own dial | **VERIFIED FIXED** | `channels/channels.go → EmailChannel.Send` dials via `ssrfguard.DialControl`; `alert/wave2.go → DaysUntilExpiry` uses `tls.Dialer{NetDialer:{Control: ssrfguard.DialControl}}`; `reports/s3.go` transport wired. New `email_ssrf_test.go` + `wave2_ssrf_test.go` pin them. This is the gap **my round-10 security "Strong/standout" verdict missed** — see Errata E-2 |
| **M-02** (D-184 self-found, SECURITY): default `/ingest/beacon` path lacked the A10 field limits I credited | both paths share one helper | **VERIFIED FIXED** | `server.go` now carries "D-184: apply A10 field limits via the shared helper from the beacon package." My round-10 credit was against the dedicated beacon server; the documented-default main-port handler was weaker — Errata E-2 |
| **M-03** (D-184 self-found): paid alert channels kept delivering after a tier downgrade | runtime gate at sync AND delivery | **VERIFIED FIXED — and the class is now consistent** | `evaluator.go` adds `channelEntitlementGate`, checked in `syncRegistryFromStore` **and** before `Send` (tier-skip ≠ `delivery_failure`), mirroring `prober` D-108; `evaluator_entitlement_test.go` (+463) pins it. I independently checked the natural sibling — the **report scheduler already had** this gate (`scheduler.go → CheckReports()` skips runs post-downgrade, wired at `serve.go`), so prober ✓ / reports ✓ / alerts ✓ are now uniform. No report-side residual |
| **M-04** (D-184 self-found): beacon identity fields length-unbounded | deliberately NOT fixed (truncating `session_id` corrupts `uniq()`; rejecting needs a frozen-contract change) — operator ruling | **CONFIRMED as an honest deferral** | Correct reasoning: truncation would silently merge sessions and corrupt `uniq(session_id)`; this is a genuine contract decision, not a dodge. Appropriately left to the operator |
| **G-02 / F-01** (rotate `CLICKHOUSE_PASSWORD`) | still open | **STILL OPEN** | Eleventh consecutive round as the sole submission blocker; operator-only |

---

## Security posture — updated after D-184 + N-01

Round 10's "Strong" verdict was **premature**: it rested on an outbound-SSRF audit that checked one client (webhook) and generalized, and a beacon audit that checked the stronger of two paths. Post-D-184 the picture is genuinely strong *and now nearly complete*, with one residual:

- **Outbound SSRF, complete sweep (this round):** nine operator-relevant outbound clients. Guarded: prober-http, prober-rtmp (tls), webhook, source-test, **email**, **cert-expiry**, **S3** (last three fixed in D-184). Fixed-endpoint, not sinks: telegram (`api.telegram.org`; custom URL is test-only), pagerduty (fixed events API). **Unguarded residual: the amsclient poll loop (N-01).** With N-01 fixed, the outbound-SSRF surface is closed for every operator-supplied-URL client.
- **Beacon ingest:** both `/ingest/beacon` implementations now share the field-limit helper (M-02); token-gated, 64 KB/100-event caps, per-token rate limit + eviction. Residual M-04 (unbounded identity fields) is a disclosed, operator-ruled contract decision.
- **Runtime tier enforcement:** now uniform across prober / reports / alerts (M-03 closed the alerts gap). Unknown-tier denial, TOCTOU node-limit mutex, lazy-expiry-to-Free all still hold (rounds 6–10).
- **Unchanged strong controls** (re-confirmed, not re-litigated): AES-256-GCM at rest with fail-closed key validation; HMAC token storage + constant-time webhook HMAC; bearer-header-only auth with WS-subprotocol token; audit trail; cosign-by-digest + SBOM + Trivy + CodeQL supply chain.
- **The one real exposure remains G-02** (operator-gated secret in git history) — not remotely exploitable, but only rotation closes it.

**Security verdict:** submission-ready on the merits once **G-02 is rotated and N-01 is wired** (N-01 is fast-follow-acceptable given bounded impact; G-02 is the hard gate).

---

## PRD conformance — unchanged from round 10 (spot-re-confirmed)

No feature code changed the PRD picture since round 10 (D-184 was security/enforcement hardening + docs). The round-10 matrix stands: **F1–F10 = 8 MET, 2 PARTIAL** (F7 cluster origin/edge dedup — structurally blocked by AMS, disclosed LIM-10; F9 error/rebuffer anomaly — deferred, disclosed LIM-06), every gap traced to a numbered LIM, no undisclosed overclaim. M-03's fix strengthens F5/F6 tier integrity (paid channels/reports now genuinely stop at runtime downgrade, matching the monetization model in PRD §7.11).

---

## Can this be the last round? — the honest answer

**Yes — round 11 can be the last *review* round, and it is a defensible place to stop *because it stopped finding things by attacking, not by reassuring.* But the close is conditional and the residual risk is named, not waved away.**

What makes stopping defensible now (that was *not* true at round 10):
1. **The adversarial sweep is near-exhausted, demonstrably.** D-184 attacked the round-10 all-clear and found three items; round 11 attacked one class further (outbound SSRF) and found exactly one more, LOW, of an already-closing class — and found no tenth outbound client, and confirmed the runtime-downgrade class is now uniform. The decay H9→I6→J3→K2→L2→(D-184:3)→N1 is real and now bottoming in LOW/bounded territory.
2. **The remaining classes need surfaces this format cannot reach:** a live pentest (to confirm the SSRF dials actually refuse on a socket), a live multi-node cluster (LIM-10), and a live Kafka broker (LIM-19). No amount of static review closes those; they are operator-gated lab work, correctly disclosed.
3. **The blocker has been identical for eleven rounds** and is not a review artifact: rotate `CLICKHOUSE_PASSWORD`.

What honesty requires me to add (the round-10 lesson):
- I will **not** repeat "it's converged, nothing left." D-184 proved a fresh adversarial pass can still extract value from a reassurance-heavy round. The correct claim is narrower: *the static-review surface this format can reach is now swept to LOW residuals of already-fixed classes; further product assurance requires live testing, not another read-through.*
- Therefore the stopping criterion is a **decision**, not a proof of completeness: stop the read-through loop, fix N-01 as fast-follow, and shift remaining assurance to the operator-gated live lanes.

**The close, as one motion (unchanged shape from rounds 8–10, now with N-01 folded in):**
1. **Rotate `CLICKHOUSE_PASSWORD`** (G-02) — the only hard blocker.
2. **Wire `ssrfguard.DialControl` into `amsclient`** (N-01) — one transport; fast-follow-acceptable.
3. **Cut `v0.4.5`** from current main so all of D-180…D-184 (guard #19, breakdown caps, the three D-184 security/enforcement fixes, the doc-stamp guard) reach a *tagged* release — the marketplace artifact is still `v0.4.4` and lacks every fix since.
4. **Submit against `v0.4.5`.**

**Single go/no-go line:** *Not blocked by review. Blocked by one rotation, one one-line SSRF wire, and one tag. Do those, submit, and stop reading — remaining assurance is live-lab work, not another round.*

**Process note (so this ledger reaches you):** because my bridge writes to a mirror that does not propagate (Errata E-1), the reliable path for this file into the repo is **you saving the attached file** into `docs/assessment/` — the same way rounds 8–10 arrived. My `device_commit_files` attempt is best-effort and may not reach the canonical tree.

---

## Errata — my own prior-round errors (the substantive part of this round)

**E-1 — I retract L-01's evidence and MEDIUM severity. J-03, K-01 and L-01 were one mistake, repeated three times.**
I filed, across three rounds, escalating claims that the round-6/round-7 ledgers "exist untracked in the repo" — culminating in L-01's byte sizes and a `head -1` transcript, used to accuse `review-chain.md` of asserting a falsehood. D-184's five-probe verification at `a59c2ca` (empty `--untracked-files=all --ignored`; no `git log --all` history; `find -xdev` returns only rounds 8/9; single worktree; no `index.lock`) shows those files never reached the canonical repository. The mechanism, which I did not understand until now: **my device bridge mounts a mirror that receives the maintainer's commits but does not push my writes back.** My `device_commit_files` "written" successes were writes into that mirror; the files were real *there* and invisible *here*. My round-11 mount now agrees with the maintainer (the mirror was re-synced and my untracked files are gone). So an Ant Media reviewer — who clones from GitHub, i.e. the canonical repo — would never have seen the files L-01 said they'd find. **L-01's core assertion was wrong.** What survives is only the narrow objection the maintainer adopted: the doc should not claim knowledge of my delivery channel, only of what arrived. I should have caught the mirror hypothesis at round 8 when a "committed" file failed to appear; instead I re-asserted it twice more. That is the single largest methodological error of my ten rounds, and it is fully on me.

**E-2 — my round-10 security "Strong / SSRF is the standout" verdict missed two real security gaps (M-01, M-02).** I verified `webhook.go`'s SSRF wiring and generalized to a "Strong" all-clear without auditing the other outbound clients — so I missed that email, cert-expiry, and S3 were unguarded (M-01, an operator-reachable IMDS dial). I credited beacon ingest's field limits against the dedicated server without checking the documented-default main-port path, which lacked them (M-02). A security section that reads as exhaustive but checks one representative of each class and generalizes is exactly the failure mode that lets an adversarial re-audit find real bugs — which is what happened. Round 11's completeness sweep (nine clients enumerated, N-01 surfaced) is the corrective, and I now state coverage as "these specific N sinks, verified" rather than "the class is Strong."

**E-3 — round 10's "product has converged, stop reviewing" was premature.** It was rendered wrong within one maintainer session. The corrected stance is above: stop the *read-through* loop deliberately, with residual risk named and shifted to live lanes — not "it's done."

**Standing errata carried forward, unretracted:** round 8's GeoBreakdown "all-clear" (wrong — the bigger instance) and J-02's viewer-controlled-cardinality premise (enum-bounded); both owned in prior rounds. The pattern across E-1/E-2 and those: **I generalize from a representative instead of enumerating, and I trust a success signal (a "written" receipt, a guarded webhook) without confirming its effect.** That is the habit to leave on the record for whoever runs round 12, if there is one.

---

## Maintainer close — D-185 (2026-07-28)

Verified against the code at `6c5dbd9` before anything was changed, per the brief's core rule.
Eleven adversarial lanes ran: two on N-01 itself, five on this round's **reassurances**, three on
the D-184 delta that no sweep had yet covered, one on release readiness. Every lane was told to
default to refuted; every surviving claim was then re-read by hand, and two lane claims did not
survive that (noted below). **Round 11's own recommendation — audit the exculpations — is what
produced most of what follows.**

### N-01 — the finding

**Confirmed:** `amsclient.New` had no `Control` hook. It was the last outbound client without one.

**Refuted:** the reachability. `ams_sources.rest_url` is not polled by anything; the collector's
client is built from `PULSE_AMS_URL` at startup. An operator POSTing a link-local source creates a
row that is only ever dialled by the **guarded** test endpoint. The filed reproduction cannot
reproduce.

**Raised, not lowered.** The reviewer graded this LOW on the argument that "the poll loop only ever
appends fixed AMS request paths, so IMDS credential paths are unreachable." Two things break that:

1. **The path is not ours.** `CheckRedirect` follows up to ten hops and constrains neither host nor
   path. Whatever the poll client talks to chooses the next URL — so a hostile or compromised AMS,
   which is a *separate trust domain* from Pulse, can point the next hop at
   `169.254.169.254/latest/meta-data/iam/security-credentials/…`. `TestClient_RefusesRedirectToIMDS`
   pins this; on the pre-fix client it fails with a dial timeout to that exact URL.
2. **The response comes back out.** `getJSON` copies up to 4 KB of a non-2xx body into its error,
   the poller stores it as `lastErr`, and `/healthz` — unauthenticated by design — republishes it in
   `components.collector.message`. So the SSRF was not blind, and *independently of any SSRF* an
   ordinary AMS 401/500 page was already being echoed to anyone who could reach the port.

**Fixed (both halves):**
- `ssrfguard.DialControl` on the amsclient dialer, `Proxy` disabled — the same ruling as prober
  (D-130) and S3 (D-184): an egress proxy dials on our behalf and defeats a resolved-IP guard.
  *Behaviour change, stated rather than buried:* this client previously inherited
  `http.DefaultTransport` and therefore honoured `HTTP(S)_PROXY`. Nothing documents AMS polling
  through a proxy.
- `amsclient.HealthSafeError` strips the upstream body for non-operator-only surfaces; the poller
  uses it for the health snapshot, and the full error still goes to the logs, where the audience is
  the operator. The login path was made a typed error for the same reason — it was a *second* route
  by which a body reached `/healthz`, and sanitizing only `getJSON` would have missed it.
- `url.PathEscape` on the `app` segment in all four path builders. `streamID` was already escaped
  and `app` was not, in the same `Sprintf` — and `app` arrives from the *remote* server's
  application list.

**Not adopted:** `ssrfguard.IsDenied` in `amsSourceFromAPI`. It would read as defence in depth but
guards a path that reaches no dialer, and the endpoint it protects already refuses at dial time with
a named error. Adding a check whose only effect is to look thorough is how a guard list starts
drifting from what it actually guards.

### The reassurances — five items the round's all-clear covered

| Reviewer said | Verdict | What we found |
|---|---|---|
| "nine outbound clients… no tenth" | **UPHELD** | Independently enumerated every egress in the repo. The count is right and the classification is right: telegram's custom-URL constructor and PagerDuty's `SetAPIURL` are called from `_test.go` only, so both really are fixed-endpoint. One clerical slip: the prose lists seven guarded clients while the same round's own arithmetic says eight — **slack** (`channels.go:265`) is missing from the list, not from the guard |
| M-01 "VERIFIED FIXED" | **UPHELD** | Email has no `smtp.SendMail`/`smtp.Dial` bypass and STARTTLS upgrades in place on the guarded connection; cert-expiry has no OCSP/CRL side-fetch; S3 is a hand-rolled SigV4 client, not the AWS SDK, so there is no separate IMDS credential-provider client to leave unguarded (the obvious way this fix could have been incomplete). The three SSRF tests were re-run against a reverted tree and do fail |
| M-02 "VERIFIED FIXED" | **UPHELD** | Exactly two ingest paths; both call `beacon.ApplyA10FieldLimits`; body cap, event cap, rate limit and ordering are identical on both; `truncateUTF8` does backtrack off a split rune |
| M-04 "an honest deferral" | **REVERSED — fixed** | The reasoning is correct about *truncation* and never reaches *rejection*, and the deferral was priced without the amplification. The identity fields are copied onto every row a batch produces, so a 64 KB body carrying a 30 KB `session_id` and 100 minimal events writes ~50× its own size. Contract now caps `session_id`/`stream_id`/`app` at 256 **characters** (~7× a UUIDv4 — characters, not bytes, because that is what JSON Schema `maxLength` means; the first cut used `len()` and would have rejected a payload the published contract calls valid) and the server **rejects** past it — which is precisely the operation that does *not* corrupt `uniq(session_id)`, since nothing oversized is ever stored. `player_kind` was the last unbounded per-row string and is truncated, not rejected: it is not an identity key |
| M-03 "the report scheduler already had this gate… **no report-side residual**… prober / reports / alerts are now uniform" | **PARTIALLY REFUTED — fixed** | The report half is right. The *uniformity* is not: there are **four** tier-gated background loops, not three. The **anomaly detector** (`serve.go:614`, `anomaly.go:310 → Run`) had no runtime gate — its API is gated by `CheckAnomalies`, but the loop went on recomputing and writing baselines for a downgraded tenant. This is the exact class D-108 established and M-03 extended, found by enumerating the set instead of accepting the list. **Chasing it surfaced a larger, pre-existing leak in the opposite direction:** anomaly detection is advertised Enterprise-only (`overview.md:220`, `listing.md:138`) and `GET /anomalies` enforces it, but alert-rule create/update **never** did — a Pro tenant could POST `rule_type=anomaly` and get a 201 (demonstrated: with the new gate removed, the test receives exactly that). Gating only the baseline loop would have left those rules in place to read staler and staler baselines and stop firing **silently**, which is worse than the leak. So all three points are now gated — compute, config and evaluation — and two older contract tests that were passing only because no tier gate existed now run on an Enterprise fixture |

### Also found — in the D-184 delta, which no sweep had covered

- **The doc-stamp guard did not block anything.** L-02 was closed "mechanically" with a `doc-stamps`
  CI job, but `.github/branch-protection.sh` never listed it among the required contexts (confirmed
  live: `gh api …/protection` returns 13 contexts, no `doc-stamps`, no `shellcheck`). A job that
  runs and reports but cannot fail a merge is advisory. **A guard added without its branch-protection
  context reproduces the very defect it was written to prevent** — so the script now carries that
  rule in a comment next to the list, and both contexts are added. `compose-boot` is deliberately
  left out, with the reason recorded: it pulls from GHCR and would make every merge depend on a
  third party's uptime.

### Errata acknowledged

E-1 (L-01 retraction) is **accepted and matched from this side** — the five probes re-ran clean at
HEAD, and `review-chain.md` now records that both vantages agree rather than resting on our
inference about a channel we cannot observe. E-2 and E-3 are accurate self-assessments. The pattern
the reviewer names in their own errata — *generalizing from a representative instead of
enumerating* — is precisely what produced the two items above: the "no tenth client" sweep was an
enumeration and it held; the "uniform across prober / reports / alerts" sentence was a list, and the
set had a fourth member.

### One claim in this round we could not match, and it does not favour the reviewer

The PRD section reports "F1–F10 = 8 MET, 2 PARTIAL". Our own `prd-validation-matrix.md` scores
F1–F9 **PARTIALLY** and only F10 **FULLY**. The two are measuring different things — that matrix
scores *live-validated acceptance criteria* at v0.3.0 with D-179 supersessions inline, not feature
presence — so this is a vocabulary mismatch, not an overclaim. Worth stating plainly because it
runs the safe way: **our internal scoring is the stricter of the two.** LIM-06 and LIM-10 were
re-checked and are present and user-visible (`known-limitations.md:154,230`; surfaced in
`README.md` and `docs/overview.md`).

### Not fixed, deliberately

- **The alert retry loop does not re-check the entitlement gate between attempts** (~5 s window at
  default backoff). Real, and left alone: the sync gate removes the channel within one interval, a
  downgrade landing inside a 5 s retry window changes at most two delivery attempts, and re-reading
  a licence inside a retry loop buys less than it costs in moving parts. Recorded rather than
  silently dropped.
- **`check-doc-stamps.sh` validates stamps inside fenced code blocks.** A false *positive*, not a
  miss: it can only fail CI on a doc that shows an example stamp, never pass a real stale one. Fix
  when a doc actually needs the example.
- **The dial error on `/healthz` still names the AMS host.** That address is the operator's own
  `PULSE_AMS_URL`, and `/healthz` already publishes `ams_env_configured` by design; stripping it
  would cost the web UI its degraded-reason display and gain nothing against a caller who can
  already tell. The upstream-controlled **body** was the part that was not ours to republish, and
  that is what was removed.
- **G-02** — unchanged, operator-only, eleventh round. Re-checked silently this session: the live
  prefix still matches 2 commits.

### Verdict on stopping

Agreed, with the reviewer's own framing: stop the read-through loop as a **decision**, not as a
proof of completeness. Round 11 argued the sweep is bottoming out at LOW residuals; this session's
audit of that round found five further items, one of which (`M-03`'s uniformity claim) is the same
class the reviewer had just declared uniform, and one of which (the advisory doc-stamps guard)
means a class declared "closed mechanically" was not closed at all. **That is the third consecutive
round in which auditing the reassurances outproduced the filed findings.** The honest conclusion is
not "nothing is left" but "this format's yield is now concentrated entirely in the exculpations,
and the remaining product risk needs a live lab" — a pentest for the dials, a two-node cluster for
LIM-10, a Kafka broker for LIM-19. All three are operator-gated and disclosed.

Submission remains blocked by exactly one thing, and it is not review: **rotate
`CLICKHOUSE_PASSWORD`**, then cut `v0.4.5` so D-180…D-185 reach a tagged artifact, then submit
against it.
