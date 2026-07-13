# SESSION-29 — operator-intake gate + highest-leverage tail (planned at S28 close, D-090)

> Written by SESSION-28 close (D-090, 2026-07-13). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/ROADMAP-V2.md` §2.18 + `RESUME-PROMPT.md` before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12): review the backlog, revise this plan

Before dispatching: re-read ROADMAP-V2 §2 (§2.18 item 6 is the operator-gated
remainder; then §2.11-tail/§2.6) + the final-assessment §5 roadmap and REVISE
this plan if a higher-leverage move exists. This file is a starting point, not
a contract. Record any revision in the D-091 open block. Carry this header into
SESSION-30.md.

## Mission

Exit = (a) **operator intake applied** — SIX items now
(operator-expected.md ⚡): AMS license landed? → the open sweep will show a
NON-null diff (expected signal, not incident) → record delta + re-validate
the Enterprise surface (`make validate-realams-p0` candidates); trial key
minted? → embed in listing draft; GHCR flipped public? → verify anonymous
`docker pull ghcr.io/aytekxr/ams-pulse:0.4.0` + install.sh with the DEFAULT
image (still the last unverified quickstart leg); final-assessment
reviewed? → execute edits / mark approved; Ant Media contact? → fold listing
requirements into docs/marketplace/; **NEW: Pro MaxNodes ruling** ("Pro
nodes = N") → one-line license.go:118-ish change + listing-draft
NEEDS-RECONCILE flag cleared + PRD-vs-code note. (b) the highest-leverage
batch from the S29 carries below; (c) CI promotions if run date ≥
2026-07-23 (else skip carry ×18); (d) standing re-checks + AMS observation
at open. PR-first, ≤2 pushes.

## S29 carries (from D-090; pick by leverage after intake)

1. **F10 tail [M]** — probe-stats UI surface (rtt/jitter/loss/ice_state
   columns exist on ProbesPage since S16; what remains per §2.11: RTMP
   AMF0 connect for full RTMP probe depth). Check §2.11 for the exact
   remaining scope before sizing.
2. **Browser-accept of the trial banner [operator-assisted]** — realams
   :18090 now RUNS v0.4.0 (S28 rebuild, fresh token); if the operator is
   present, walk them through the ssh tunnel; else keep standing.
3. **SRT loss validation [S, test-only]** — final-assessment §5 P1: run
   the TC-I-05 variant with an SRT publisher vs the live AMS; document
   whether packetLostRatio is ARQ-corrected; DG-18 variant note.
4. **D-V2-1 unsigned-webhook ingest mode [operator decision first]** —
   still the top open P0 in final-assessment §5; NOT buildable without
   the operator accepting the network-trust model (§2.6). Re-surface
   only; do not build unprompted.
5. **Marketplace upload prep [gated]** — the moment items 2–5 land:
   embed trial key, export remaining screenshots (operator-manual set),
   flip docs/marketplace/ out of DRAFT-INTERNAL (needs final-assessment
   approval, D-081 gate).
6. **Small honesty tail:** known-limitations.md — consider adding the
   kafka first-start history-replay + plaintext-only rows (S28's
   kafka-integration.md discloses them; the 18-row disclosure doc may
   want parity — verify against docs/known-limitations.md before adding).

## ⚠️ Check these BEFORE dispatching anything

1. **`docs/operator-expected.md` — SIX items** (see Mission (a)). If
   answered → act; else re-surface. Do NOT revert
   `deploy/config/Caddyfile.prod` on disk (D-082 standing).
2. **Concurrent-session hazard (D-062, recurrent).** Foreign work →
   inspect → preserve on a branch → reset; never revert working files.
   ALSO: the session shell may lack the docker group —
   use `sg docker -c "…"` (S28 env note in D-090).
3. **AMS post-expiry state.** SEVEN byte-identical sweeps post-lapse; no
   antmedia restart (StartedAt 2026-07-12T06:52Z). At open:
   `PULSE_TOKEN=<any> bash qa/realams/harness/expiry-sweep.sh s29open`
   + diff vs `S21-sweep-preexpiry-20260712T014135Z/stable.txt`. The
   realams overview line is VALID again (fresh token auto-extracts since
   the S28 rebuild) — but its counts reflect the CURRENT stack, not the
   S21 baseline; compare structurally. If the diff is non-null on the
   AMS lines → probably the operator's new license: record delta,
   re-validate, never restart/fix AMS.
4. **Prod runs v0.3.0-34-g58a9c84 since S27** (rollback tag pre-d089
   stands). Read-only health check at open; **next rollout carries
   D-089+D-090** (trial lifecycle + baked migrations + web banner +
   fleet-status contract CR + FleetPage tile removal — note the CR is
   backward-compatible for consumers: the server stops CLAIMING "down",
   it never emitted it).
5. **S28 merge evidence:** append the PR/merge line to decisions.md
   D-090 (S29's PR carries it).

## Gates (ORCH, before any commit)

- Any Go change: FULL §8 (CI-faithful: golang:1.25 docker w/ pulse-gomod
  + pulse-gobuildcache volumes + safe.directory /repo [S28 gotcha —
  bind-mount VCS stamping], 0 FAIL / 0 unexpected SKIP, coverage ≥ floor
  70.2 (S28 actual 76.1), gofmt-on-emptiness, vet, contract-drift clean).
  Integration suite when store/query/api change. Web: gen:api drift +
  build + LINT + vitest (the S27 lesson — lint is part of faithful CI).
- Never trust a tree a dead/stalled agent left (D-082/D-086) — re-derive
  RED proofs in pristine worktrees. A pin that can t.Skip is not a pin.
- Harness/bash edits: `bash -n` + shellcheck + memory
  `shell-harness-false-green-patterns` (BINDING).
- docs/marketplace/ stays DRAFT-INTERNAL until the operator approves the
  final assessment (D-081 external gate). Nothing external.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md **D-091** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-30; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` + PushNotification.
5. Write `sessions/SESSION-30.md` (carry the standing directive header).
