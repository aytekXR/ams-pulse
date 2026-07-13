# SESSION-28 — operator-intake gate + marketplace tail (planned at S27 close, D-089)

> Written by SESSION-27 close (D-089, 2026-07-13). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/ROADMAP-V2.md` §2.18 + `RESUME-PROMPT.md` before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12): review the backlog, revise this plan

Before dispatching: re-read ROADMAP-V2 §2 (§2.18 is TOP PRIORITY until done,
then §2.17/§2.11-tail/§2.5) + the final-assessment §5 roadmap and REVISE this
plan if a higher-leverage move exists. This file is a starting point, not a
contract. Record any revision in the D-090 open block. Carry this header into
SESSION-29.md.

## Mission

Exit = (a) **operator intake applied** — S27 produced FIVE operator items
(operator-expected.md ⚡): AMS license landed? → re-sweep + Enterprise
re-validation; official trial key minted? → embed in listing draft; GHCR
flipped public? → verify anonymous `docker pull ghcr.io/aytekxr/ams-pulse:0.4.0`
+ run install.sh with the DEFAULT image (the last unverified quickstart leg);
final-assessment reviewed? → execute edits / mark approved; Ant Media contact? →
fold listing requirements into docs/marketplace/. (b) the highest-leverage
batch from the S28 carries below; (c) CI promotions if run date ≥ 2026-07-23
(else skip carry ×17); (d) standing re-checks + AMS observation at open.
PR-first, ≤2 pushes.

## S28 carries (from D-089; pick by leverage after intake)

1. **AMS-INTEGRATION.md §4.5 stale [XS–S]** — still claims `recording_gb`
   always 0 / "no VoD REST poll path"; false since S23/D-085. A4+V3 both
   flagged it (out of A4's scope). Fix + re-read the whole §4 for
   BUG-002-era staleness.
2. **docs/kafka-integration.md (DG-15) [S]** — highest-impact unwritten
   doc gap (standalone Fleet gauges blank without Kafka).
3. **realams stack fresh rebuild [XS, sanctioned]** — `down -v` realams
   ONLY (never prod), rebuild on current main; fixes the orphaned harness
   token (memory realams-token-log-extract) and puts the trial-banner
   build on the validation stack for browser-accept.
4. **Listing PNG exports [XS–S]** — per docs/marketplace/screenshot-list.md
   (brandkit/ui source; needs a rendering step — check feasibility, else
   mark operator-manual).
5. **Pro MaxNodes=10 vs PRD §7.11 "1–2 nodes" reconcile [XS decision]** —
   flagged NEEDS-RECONCILE in listing-draft.md; surface to operator if no
   code ruling is obvious (code currently governs).
6. **Deferred S27 originals:** F10 tail [M] (probe-stats UI + RTMP AMF0),
   §2.17.1 viewer_count zero-mean product ruling [S], §2.17.2 parity-map
   derivation [XS], §2.17.3 status="down" reachability [XS–S, may need
   contract CR], §2.5 O(N²) rebuildSnapshot [M].

## ⚠️ Check these BEFORE dispatching anything

1. **`docs/operator-expected.md` — FIVE items** (see Mission (a)). If
   answered → act; else re-surface. Do NOT revert
   `deploy/config/Caddyfile.prod` on disk (D-082 standing).
2. **Concurrent-session hazard (D-062, recurrent).** Foreign work →
   inspect → preserve on a branch → reset; never revert working files.
3. **AMS post-expiry state.** SIX byte-identical sweeps post-lapse; no
   antmedia restart (StartedAt 2026-07-12T06:52Z). At open:
   `PULSE_TOKEN=<any-or-placeholder> bash qa/realams/harness/expiry-sweep.sh s28open`
   + diff vs `S21-sweep-preexpiry-20260712T014135Z/stable.txt` (exclude the
   pulse-realams.overview line unless the realams stack was rebuilt —
   carry #3). **If the operator's new AMS license landed, the diff will
   NOT be null — that's the expected signal, not an incident:** record the
   delta (licence-status, versionType, apps), then re-validate the
   Enterprise surface (`make validate-realams-p0` candidates).
   If restarted AND features 403: observe + report, never restart/fix AMS.
4. **Prod runs v0.3.0-34-g58a9c84 since S27** (rollback tag pre-d089
   stands). Read-only health check at open; next rollout carries D-089
   (trial lifecycle + baked migrations + web banner).
5. **v0.4.0 release evidence:** confirm the tag-triggered release workflow
   went green (Trivy/SBOM/cosign) and `ghcr.io/aytekxr/ams-pulse:0.4.0`
   exists; append evidence to decisions.md D-089. If it FAILED, fixing it
   is P0 — the quickstart pins that image.
6. **S27 merge evidence:** append the PR/merge line to decisions.md D-089
   (S28's PR carries it).

## Gates (ORCH, before any commit)

- Any Go change: FULL §8 (repo-root mount `-race`, 0 FAIL / 0 unexpected
  SKIP — 3 env-gated infra skips characterized D-085..D-088, coverage ≥
  floor 70.2 (S27 actual 76.1), gofmt-on-emptiness, vet, contract-drift
  clean). Integration suite when store/query/api change.
- Never trust a tree a dead/stalled agent left (D-082/D-086) — re-derive
  RED proofs in pristine worktrees. A pin that can t.Skip is not a pin.
- Harness/bash edits: `bash -n` + shellcheck + memory
  `shell-harness-false-green-patterns` (BINDING).
- docs/marketplace/ stays DRAFT-INTERNAL until the operator approves the
  final assessment (D-081 external gate). Nothing external.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md **D-090** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-29; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` + PushNotification.
5. Write `sessions/SESSION-29.md` (carry the standing directive header).
