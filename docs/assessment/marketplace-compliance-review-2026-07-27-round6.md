# External review — Round 6 (2026-07-27) · review + disposition

**Reviewed tree:** `2412d58` (`main`, `v0.4.3-2-g2412d58`) · **submission target at review time:**
tag `v0.4.3` = commit `669952e`
**Reviewer verdict:** *Not ready to submit today* — one listing overclaim (H-01), the open
credential rotation (G-02), and a decision on the tag-vs-main gap (H-02).
**Executed in:** SESSION-112 / D-179.

**Our verdict after verification: all nine findings CONFIRMED against the tree. Eight are
fixed; one (H-02) is an operator decision, prepared but not taken.** No finding was refuted.

The reviewer ran in a sandbox with almost no egress — no image pull, no cosign, no live AMS.
Several findings were therefore raised as *probes* rather than defects. **We ran the probes.**
One of them, H-08, filed LOW with "medium-low confidence", turned out to close **LIM-01** —
the first entry in the Priority-1 limitation list, disclosed since the product's first release.
That is the headline result of this round.

---

## Disposition table

| ID | Sev | Subject | Verified? | Disposition |
|---|---|---|---|---|
| **H-01** | HIGH | Listing omits the HLS viewer-count accuracy caveat | **CONFIRMED** | **FIXED.** The caveat now appears in all three places a reader meets the claim: feature bullet 1, the long-form description, and the SS1 screenshot caption — mirroring the egress-caveat pattern the reviewer identified as the house style. Also corrected in passing: "signed release binaries" → "checksummed release binaries (the image is cosign-signed)", which the reviewer flagged inside H-03 as a slight overstatement. Verified the stated character counts (title 42, short description 240) are unaffected — neither edited string is inside them. |
| **H-02** | MEDIUM | `v0.4.3` lacks main's evaluator-facing corrections | **CONFIRMED** | **OPEN — operator decision, and the case for cutting is now stronger than the reviewer knew.** Every hunk they cite is real (`git diff v0.4.3..HEAD`). This round adds materially more to the gap than round 5's copy fixes: a **code change closing LIM-01**, an anchored cosign command, a 4-asset `SHA256SUMS`, and an installer exit-code contract. Recommendation: cut **v0.4.4** and retarget the submission. Everything is staged; the cut is one tag push after main's post-merge CI goes green. Not taken autonomously — release cuts are operator-gated. |
| **H-03** | MEDIUM | `SHA256SUMS` covers 2 of 4 assets; no doc uses it | **CONFIRMED** | **FIXED.** `SHA256SUMS` generation moved out of the binary-build step into a new final step that runs after the SDK tarball and Helm chart exist, and checksums **all four** assets with bare filenames so `sha256sum -c` works from one download directory. The step asserts a 4-line result and runs `sha256sum -c` on itself before uploading — without that assertion a silently missing artifact would republish a short file and read as success, which is the defect itself. Both directions rehearsed locally (4 lines pass; missing SDK tarball aborts the step). Customer-facing instructions added: a **Verify your downloads** section in `install.md` and a verification table in `SECURITY.md` covering all four artifacts plus the image. |
| **H-04** | MEDIUM | "AMS 2.10+" has zero coverage for the maintained 2.x line | **CONFIRMED** | **FIXED — with source evidence rather than a guessed profile.** Confirmed upstream: 2.15.0, 2.16.x, 2.17.0/.1 all exist (2.17.1 released 2026-02-12) and none was covered. Added a **`v2.17.1` mock profile** derived from AMS's own source at tag `ams-v2.17.1` — the LIM-10 technique, not invention. Cross-checking `Broadcast.java` and `ClusterNode.java` at `ams-v2.14.0` / `2.16.2` / `2.17.1` / `3.0.3` shows **every field Pulse consumes is present and identically typed across the whole range**; 2.17.1 → 3.0.3 removes only `conferenceMode` and `subTrackStreamIds`, neither of which Pulse reads. `compatibility.md` gains 2.17/2.16/2.15 rows and a precise "what 2.10+ means" block that says plainly that **2.15 is untested**; the listing's compatibility line now names the profiled versions instead of implying the range. The new profile also uses the *real* cluster-node field names, making it better evidence than the older profiles it sits beside. |
| **H-05** | MEDIUM | Unanchored cosign identity regexp | **CONFIRMED** | **FIXED, and verified against the published image rather than reasoned about.** README and both workflow comments now use `^https://github\.com/aytekXR/ams-pulse/\.github/workflows/release\.yml@refs/tags/v.+$`. Ran cosign v3.0.2 against the published `0.4.3`: the anchored form **passes** (digest `sha256:75a76c67…727b4`), and the same regexp with `refs/heads/` **fails** — which also printed the exact SAN, `…/release.yml@refs/tags/v0.4.3`, confirming the anchors match reality. A note explains why the anchoring is load-bearing so it is not "simplified" later. |
| **H-06** | LOW | Installer exits 0 on a degraded install | **CONFIRMED** | **FIXED, and tested end-to-end in a clean room.** `install.sh` now exits **2** when the collector cannot reach AMS, with an exit-code table in the script header and in `install.md`. The nonzero status is set at the top of the degraded branch but returned at the very end, so the admin token and next-steps still print — an operator loses nothing. Verified both directions on this VPS: unreachable AMS → **exit 2** with the token still shown; the real AMS 3.0.3 → **exit 0**, "Pulse is healthy."; both stacks torn down with no residue. **This nearly broke CI:** the trailing `exit` makes ShellCheck's reachability pass call the `trap cleanup EXIT` body dead code — SC2317 on 0.9.0, which is exactly what CI runs. Suppressed explicitly with both codes and a comment; re-verified clean under 0.9.0 *and* 0.11.0. |
| **H-07** | LOW | `internal/cluster` sits outside the documented AMS boundary | **CONFIRMED** | **FIXED — as an enforced rule, not a doc edit.** ARCHITECTURE §3 rule 2 now names `internal/cluster` as part of the collector boundary (it *is* the cluster collector) and records the composition-root exception. The reviewer's sub-claim that `internal/domain` mentions amsclient only in a comment was independently re-verified — correct. Because a prose rule is what allowed the drift in the first place, added `TestAMSBoundary_ImportersAreCollectorOnly`: it parses every non-test Go file and fails on any amsclient importer outside an explicit allow-list, plus a companion test that fails on **stale** allow-list entries. Mutation-proved: removing `internal/cluster` from the list makes it fail and name the file. The next drift fails CI instead of waiting for round 7. |
| **H-08** | LOW | LIM-01 gauges may be fillable via console endpoints Pulse never calls | **CONFIRMED — and larger than filed** | **FIXED; LIM-01 CLOSED.** The reviewer could not run the probe. We did, against the live AMS 3.0.3: `/rest/v2/cpu-status`, `/system-memory-status`, `/jvm-memory-status` and **`/rest/v2/system-resources`** all return HTTP 200 with populated data on a **standalone** node. `system-resources` is the whole answer — it carries `cpuUsage`, `systemMemoryInfo` **and** `fileSystemInfo`, plus the entire `system-status` body under `systemInfo` and the entire `/rest/v2/version` body under `softwareVersion`. Pulse now prefers it, falling back to `system-status` on 404/405 (gauges honestly absent there, never zero-filled). One call now replaces two. Percentages deliberately reuse the Kafka path's formulas, because both decode the same serialized `StatsCollector` object. **Live-verified end-to-end** through the real client and normalizer: `cpu_pct 62, mem_pct 88.97, disk_pct 83.24, version 3.0.3`. `/rest/v2/system-resources` exists back to `ams-v2.10.0`, so this spans the entire claimed compatibility range. LIM-01 is rewritten rather than deleted, and now carries the one caveat that survives: AMS computes `inUseMemory = totalMemory − freeMemory`, so `mem_pct` counts page cache and reads 75–90% on a healthy Linux host — a threshold-calibration note, not a blank gauge. |
| **H-09** | LOW | `candidate-*` tags persist on promoted images | **CONFIRMED — proposed fix is not implementable** | **DOCUMENTED; deferred with reasons.** The tags do persist, exactly as reported. But "delete the candidate tag via the same `GHCR_CLEANUP_TOKEN` path" cannot work: that endpoint deletes a package *version*, which **is** the manifest digest, and on the success path that digest carries `X.Y.Z`/`latest` — deleting it would delete the release along with its SBOM, provenance and signature. GHCR exposes no delete-a-single-tag API. Two alternatives were weighed and both rejected for now: a separate quarantine **package** would leave SBOM/provenance referrers in the wrong repo (a real supply-chain regression traded for a cosmetic one), and buildx `push-by-digest=true` — the genuinely correct fix — changes the publish mechanism and cannot be exercised by the dry-run path, only by a real tag. Changing that untested days before submission is the wrong trade. Both alternatives are now recorded in `release.yml` so the next person does not re-derive them, and `SECURITY.md` explains to any reviewer who diffs tags that `candidate-<sha>` is an **alias of the released digest**, not a separate or unscanned image. |

---

## Prior-round items still open

| ID | State |
|---|---|
| **G-02 / F-01** — rotate `CLICKHOUSE_PASSWORD` | **STILL OPEN, operator-gated.** Re-checked silently this session: the live value's 32-hex prefix still matches 2 commits in public history. Unchanged as the #1 pre-submission gate. |

---

## Errata and corrections to the reviewer

Recorded in both directions, per the standing protocol.

- **H-09's proposed fix is wrong**, for the reason above. The finding itself is right.
- **H-08 was filed LOW with "medium-low confidence"** and a self-aware note that it was
  "falsifiable in one minute on the operator's VPS". It was, and it closed a Priority-1
  limitation. The severity was understated, which is the correct direction to err in when you
  cannot run the probe yourself — the finding was framed so that we could settle it, and that
  framing is why it paid off.
- **No reviewer claim was refuted this round.** Every citation checked out against the tree,
  including the two the reviewer flagged as uncertain (`internal/domain` comment-only mention;
  `SHA256SUMS` covering exactly two files).

## Found by us, alongside the review

- **`compatibility.md` carried nine drifting line-number citations** (`lines 134–171`,
  `ams_version_matrix_test.go:79`, `client.go:97`, …) — the exact class G-05 removed from
  `AMS-INTEGRATION.md`, in a file nobody re-checked. Inserting the 2.17 profile invalidated
  three of them immediately. All are now symbol- and name-based. `compatibility.md` is at zero
  numbered source citations, matching `AMS-INTEGRATION.md`.
- **`ClusterNode` is field-identical from 2.14 to 3.0.3** — no `role`, no `version` at any
  version. LIM-10 therefore applies to the whole 2.x line, not only 3.x, which the disclosure
  did not previously say.
- **The AMS trial licence on the validation VPS expires 2026-07-27** (visible in the
  `system-resources` capture). Flagged to the operator; it affects future live validation, not
  any shipped claim.

---

## What this round did not change

- **No release cut.** H-02 is the operator's call; see the disposition above.
- **No rotation, no prod roll.** Operator-gated. Prod was read-only this session and is
  healthy (3/3 `/healthz` components `ok`, 1,328,195 server events, collector ingesting).
- **Cluster engineering (LIM-10)** stays disclosed and gated on a live multi-node cluster.
