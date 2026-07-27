# External review — Round 7 (2026-07-27) · review + disposition

**Reviewed tree:** `463927e` (`main`, `v0.4.4-2-g463927e`) · **submission target:** tag `v0.4.4`
= commit `34a25fc`
**Reviewer verdict:** *Ready to submit as soon as G-02 (the `CLICKHOUSE_PASSWORD` rotation)
closes* — all nine round-6 dispositions re-verified against the tree **and the published
artifacts**, six new findings, none blocking.
**Executed in:** SESSION-113 / D-181.

**Our verdict: all six findings CONFIRMED. All six fixed. None refuted — and two were larger
than filed.**

This is the first round where the reviewer independently re-verified every prior disposition
against the *published* artifacts rather than the changelog, and confirmed the H-02 recurrence
pattern is broken (post-tag drift is now internal-only). Their headline is a fair one:
**one operator-gated blocker remains**, and it is the same one it has been since round 4.

---

## Disposition table

| ID | Sev | Subject | Verified? | Disposition |
|---|---|---|---|---|
| **I-01** | MEDIUM | README contradicts itself about the current release | **CONFIRMED** | **FIXED — and the class is now guarded, not just the instance.** All three strings corrected: the `Releases: … (current: **v0.4.3**)` pointer, the `Last updated … latest release **v0.4.3**` footer, and the prod-roll sentence. The cosign-history range was **de-literalized** rather than bumped — "fails on `0.3.0` **and every release since**" cannot go stale again, whereas "through `0.4.3`" was guaranteed to. The reviewer's diagnosis of *why* it slipped is exactly right: guard #17 is deliberately scoped to runnable `ams-pulse:<semver>` pins, so English release-pointers sat outside every window. **New release-guard check #19** now pins both prose patterns to the tag by exact-string match — no bare version grep, so prose legitimately naming an older release still does not trip it. Mutation-proved in both directions: PASS on the current tree; reverting the pointer to `v0.4.3` fails with a message that prints what it found. |
| **I-02** | LOW | Helm README's chart-version example describes the previous chart | **CONFIRMED** | **FIXED by removing the literals, not by bumping them.** `deploy/helm/pulse/README.md` said "0.3.1 carries appVersion 0.4.3" while shipping 0.3.2/0.4.4. Rewritten to point at `Chart.yaml` instead of naming a pair, so it cannot drift a third time — the reviewer's own preferred option. Guard #18 was working exactly as scoped (every runnable `--version 0.3.2` pin was correct); the prose was the gap. |
| **I-03** | LOW | "Last updated" stamps lag the round that edited them | **CONFIRMED — and understated** | **FIXED, plus two more the review did not catch.** `compatibility.md` stamped D-176 while D-179 had rewritten ~95 lines of it. Sweeping every stamped doc found the same lag in **`ARCHITECTURE.md`** (stamped D-161; D-179 rewrote §3 rule 2) and **`licensing.md`** (stamped D-066; D-173 replaced the dev key with the official verification key — a materially load-bearing edit sitting under an 18-decision-old stamp). All three corrected. |
| **I-04** | LOW | Fallback-path `api_latency_ms` spans up to three calls | **CONFIRMED — reproduced numerically** | **FIXED.** The reviewer read this off the diff; we turned it into a failing test first. With a slow 404 on `system-resources` and a slow `/rest/v2/version`, the emitted `api_latency_ms` was **603 ms** — precisely the sum of the two legs that should not have been in the window. Each call is now timed separately and `sysRTTMS` carries the RTT of the call that actually produced the event, with `GetVersion` back outside the window exactly as it was pre-D-179. D-087's stated intent ("measure RTT around the … call specifically") holds again on both paths. Two regression tests pin it — one per path — using deliberate per-route delays rather than wall-clock luck. |
| **I-05** | LOW | Stale "standalone AMS 3.x never reports cpu_pct" rationale comments | **CONFIRMED — and materially undercounted** | **FIXED at eight sites, not the three cited.** The review named `alert/wave3.go` plus two test echoes; a repo-wide sweep also found the same false rationale in **`store/meta/anomaly.go`**, **`anomaly/anomaly.go`** (twice) and **`alert/wave2_d087_test.go`** (three times) — all asserting that standalone AMS never reports these metrics, which D-179 made false. Every site now states the durable invariant instead: *a node snapshot may lack the key; skip on ABSENCE, never on an assumption about which AMS versions report what.* The guard behaviour is unchanged and correct; only its justification was wrong. Comment-only change — the full alert/anomaly/meta suites pass unchanged under `-race`. |
| **I-06** | LOW | LIM-10 understates its own scope | **CONFIRMED** | **FIXED.** D-179 proved `ClusterNode` is field-identical across `ams-v2.14.0` / `2.16.2` / `2.17.1` / `3.0.3`, recorded it in the round-6 "Found by us" section — and never carried it into the disclosure. LIM-10's title and body now say **AMS 2.14–3.x**, name the four verified tags, and add an explicit sentence for the case the reviewer identified: a prospect evaluating Pulse for a 2.16/2.17 cluster, who could previously have read "3.x" and hoped dedup activates on their version. The observation existed; the edit had not landed. |

---

## Prior-round items still open

| ID | State |
|---|---|
| **G-02 / F-01** — rotate `CLICKHOUSE_PASSWORD` | **STILL OPEN, operator-gated.** Re-checked silently this session: the live value's 32-hex prefix still matches 2 commits in public history. It is the only blocker both the reviewer and the loop agree on. |

---

## Errata and corrections to the reviewer

- **No reviewer claim was refuted this round.** Every one of the six reproduced exactly as
  described, including I-04, which was read off a diff without the ability to run anything.
- **Two findings were undercounted, in the safe direction.** I-03 named one stale stamp; there
  were three. I-05 named three stale comments; there were eight. Both were framed as "this
  class exists here" rather than "this is the exhaustive list", which is the framing that makes
  a sweep worth running.
- **The reviewer's own errata are accepted as written.** Their H-09 retraction is correct (GHCR
  deletes by digest, and on the success path that digest *is* the release), and their H-08
  self-calibration — that value-if-true deserved more weight than the confidence discount they
  applied — is the right lesson to carry into future probe-grade findings. Recorded here so
  round 8 inherits it.

## Found by us, alongside the review

- **`ARCHITECTURE.md` and `licensing.md` carried stale stamps too** (see I-03). The stamp sweep
  is worth repeating whenever a round edits documentation, because the stamp is the file's own
  freshness claim and this repo treats it as load-bearing.
- **Five extra sites of the I-05 false rationale** across the anomaly and meta-store packages.

---

## What this round did not change

- **No release cut.** All six findings are prose or comments except I-04, whose impact is
  confined to the fallback path on deployments that do not serve `/rest/v2/system-resources`
  (absent since `ams-v2.10.0`, so the population is small by the project's own evidence). They
  ride the next natural cut, which is the reviewer's own recommendation — and guard #19 now
  ensures the README pointers cannot be forgotten at that cut.
- **No rotation, no prod roll.** Operator-gated. Prod was read-only this session and healthy
  (3/3 `/healthz` components `ok`, collector ingesting).
- **Cluster engineering (LIM-10)** stays disclosed and gated on a live multi-node cluster —
  though its disclosure is now correctly scoped to 2.14–3.x.
