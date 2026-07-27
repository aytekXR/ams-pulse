# SESSION-112 — 2026-07-27 — external review round 6 (H-01…H-09) verified & executed; LIM-01 closed; v0.4.4 cut

**Decisions:** D-179 (review execution), D-180 (the v0.4.4 cut). **Operator directive:** *"A new review is landed… don't forget to update
resume prompts afterwards."*

Round 6 was reviewed by a sandbox with almost no egress — no image pull, no cosign, no live AMS.
So several findings arrived as **probes** rather than defects, honestly labelled as such. We ran
the probes. One of them, filed **LOW** with *"confidence medium-low… falsifiable in one minute on
the operator's VPS"*, closed **LIM-01** — the first entry in the Priority-1 limitation list,
disclosed since the product's first release.

**Result: all nine findings CONFIRMED against the tree. Eight fixed, one (H-02) left to the
operator. None refuted.** Dispositions:
`docs/assessment/marketplace-compliance-review-2026-07-27-round6.md`.

## Gate reads (first, as always)

| Gate | Reading |
|---|---|
| Prod health | 3/3 `/healthz` components `ok`; **1,328,195** server events (up from 1,323,315 at S111) — collector actively ingesting |
| `CLICKHOUSE_PASSWORD` | **Still un-rotated** — live prefix matches 2 commits in public history (checked silently) |
| Git drift | `origin/main` == local HEAD `2412d58`; clean tree |

## The headline — H-08 closed LIM-01

LIM-01 said standalone AMS cannot report CPU/memory/disk, so *"deploy Kafka to see CPU."* Four
curls against the live AMS 3.0.3 showed `/rest/v2/cpu-status`, `/system-memory-status`,
`/jvm-memory-status` and **`/rest/v2/system-resources`** all return HTTP 200 with populated data
on a **standalone** node.

`system-resources` is the whole answer: `cpuUsage`, `systemMemoryInfo` **and** `fileSystemInfo`,
plus the entire `system-status` body nested under `systemInfo` and the entire `/rest/v2/version`
body under `softwareVersion` — **one call replacing two**. Pulse now prefers it and falls back to
`system-status` on 404/405, where the gauges stay honestly absent rather than zero-filled. The
route exists from `ams-v2.10.0` onward, the full range Pulse claims to support.

**Why we had it wrong, which is the part worth keeping.** `normalize.go` carried a careful
comment explaining that the old code parsed `cpuUsage`/`systemMemoryInfo`/`fileSystemInfo` and
got zeros, and that those fields "do not exist". They exist — the code had been reading the right
shape from the **wrong endpoint**. `capability-map.md` recorded a verified premise
(*`/rest/v2/system-status` omits CPU/mem/disk*) and an inferred conclusion (*therefore AMS cannot
report it on standalone*) that was never probed. That conclusion became a Priority-1 disclosure,
a Kafka integration and a roadmap item, and survived every review because nobody re-tested the
generalization.

Percentages reuse the Kafka path's formulas verbatim — both decode the same serialized
`StatsCollector` object, so a node must not report differently depending on which transport saw
it. Live-verified end-to-end through the real client and normalizer against production AMS:
`cpu_pct 62, mem_pct 88.97, disk_pct 83.24, version 3.0.3`. LIM-01 is **rewritten, not deleted**:
AMS computes `inUseMemory = totalMemory − freeMemory`, so `mem_pct` counts page cache and reads
75–90% on a healthy Linux host — a threshold-calibration note now, not a blank gauge.

## The rest

| ID | Fix |
|---|---|
| **H-01** HIGH | HLS overcount caveat added to feature bullet 1, the long description **and** the SS1 caption; "signed release binaries" → "checksummed". Stated character counts re-verified unaffected. |
| **H-02** MED | **Operator decision, prepared not taken.** The `v0.4.3`→`main` delta now includes a code change closing LIM-01, so the case for a `v0.4.4` retarget is stronger than the reviewer knew. One tag push after main's post-merge CI. |
| **H-03** MED | `SHA256SUMS` moved to a final step covering **all four** assets, asserting 4 lines and self-verifying before upload. "Verify your downloads" published in `install.md` + `SECURITY.md`. |
| **H-04** MED | Source-derived `v2.17.1` profile (read from `ams-v2.17.1`, not invented). Every field Pulse consumes is identical across 2.14 → 3.0.3. Compatibility rows added; **2.15 stated as untested**. |
| **H-05** MED | Anchored cosign regexp — **run against the published `0.4.3`**: anchored passes, `refs/heads/` variant fails and printed the exact SAN. |
| **H-06** LOW | Installer exits **2** when degraded, token still printed. Clean-room tested both directions. |
| **H-07** LOW | ARCHITECTURE §3 names `internal/cluster`, **and** a new boundary test fails CI on the next drift (mutation-proved). |
| **H-09** LOW | Proposed fix is unimplementable (package version == digest == the release). Alternatives weighed and recorded; `SECURITY.md` explains the alias to anyone diffing tags. |

## Two things that nearly bit

- **CI would have failed on H-06.** Adding a trailing `exit` makes ShellCheck call the
  `trap cleanup EXIT` body dead code — **SC2317 on 0.9.0, the version CI installs** (the newer
  container image reports it as SC2329, so testing only with `:stable` would have missed it).
  Suppressed with both codes; verified clean on 0.9.0 *and* 0.11.0.
- **`compatibility.md` had nine drifting line-number citations** — the G-05 class, in a file
  nobody re-checked. Inserting the 2.17 profile invalidated three immediately. Now symbol-based;
  the file is at zero numbered citations.

## Gates

`go vet` clean · `gofmt -l` empty · full `go test ./... -race` green with the **repo-root mount**
(0 FAIL, 309 api tests, 4 skips — all known environment-gated: Kafka broker, ajv fixtures,
poppler) · ShellCheck clean on 0.9.0 and 0.11.0 · both workflows parse as YAML · 55 relative doc
links resolve, anchors included · clean-room installs torn down with no residue.

## The cut (D-180)

The operator took H-02 mid-session and authorised **v0.4.4**. Round 5 had ruled that copy fixes
ride the next release; that ruling predated the code, and D-179 put a Priority-1 limitation fix
in the delta. Chart semver went `0.3.1` → `0.3.2` because D-178 had touched `values.yaml` and
`helm push` **overwrites** a published chart version — guard check #16 exists for exactly that.

**The version guard was dry-run locally before tagging**, by extracting the step from
`release.yml` and running it against a throwaway tag. It caught check #12 on the first pass —
`listing.md` still read "ships all ten analytics features in v0.4.3". One-line fix locally
instead of a failed pipeline. Re-run: **all 18 checks PASS**.

`docs/operator-expected.md` was also pruned to **open items only** (operator directive):
history, "what changed" narration and the loop-owned engineering-debt section removed; eleven
actionable items remain.

## Post-release verification (the published v0.4.4, as a customer sees it)

Pipeline green is not evidence; the artifacts are. All checked anonymously after the tag:

| Check | Result |
|---|---|
| **`SHA256SUMS` covers all four assets (H-03)** | **343 B, 4 lines** — was 168 B / 2 lines. The documented command `sha256sum --ignore-missing -c SHA256SUMS` returns **OK for all four** (both binaries, the beacon tarball, the Helm chart) |
| Binary identity | `./pulse-linux-amd64 version` → `pulse v0.4.4 (commit 34a25fc4)` — the merged SHA |
| Anonymous image | `0.4.4` ≡ `latest` → same digest `sha256:81673359…45df`, no auth; `amd64` + `arm64` both present |
| **Published cosign command (H-05)** | The **exact anchored block from README@v0.4.4** verifies the published image; reported digest matches the anonymously-resolved manifest |
| Anonymous OCI chart | `helm pull oci://ghcr.io/aytekxr/charts/pulse --version 0.3.2` → `version 0.3.2 / appVersion 0.4.4` |
| **H-02 closed at the tag** | README@v0.4.4 carries the cosign-v3 note **and** the anchored regexp; `install.md`@v0.4.4 carries "Verify your downloads" and the installer exit-code table; every `ams-pulse:` pin at the tag is `0.4.4` |
| Installer at the tag | `PULSE_REF:-v0.4.4`, `PULSE_IMAGE` `0.4.4`, the `EXIT_CODE=2` branch, and the G-04 port exemption — byte-identical to the local tree |

## Left for the operator

1. **Rotate `CLICKHOUSE_PASSWORD`** — now the *only* thing blocking submission.
2. The AMS trial licence on the validation VPS shows `endDate 2026-07-27` — expiring today.
   Affects future live validation, not any shipped claim.
