# SESSION-101 — the PROVE-IT session found the D-164 fix itself was broken (D-166)

> Opened 2026-07-24 under `sessions/SESSION-100.md` §SESSION-101 + the RESUME-PROMPT's
> independent pre-listing review block (REVIEW-EXT-2026-07-24). Repo `/home/aytek/repo/ams-pulse`
> on the VPS (**this host IS prod**; no SSH). Prod stays **v0.4.0-139-gf9e9c69**, tier
> `enterprise`, collecting — **NO prod roll this session.** Work shipped on branch
> `s101-d166-prelisting-fix-wave-v041` → **PR #204**.

## Goal

Two charters, both owed at open: (1) close the carried **D-164 PROVE-IT** — prove the new
`/healthz` collector-staleness signal and `deployment.sh` step 6 actually fire against the
failure they were written for, in an **isolated** stack, never prod; (2) act on the
independent third-party **pre-listing review** (REVIEW-EXT-2026-07-24) — fold its blockers and
fast-follows into the submission gate rather than re-running it as an arc.

## ★ The headline: the D-164 fix's own gate was broken

D-164 (S100) added a `deployment.sh` step 6 that was supposed to refuse a deploy unless Pulse
is actually *collecting*. Exercised for the first time this session against a deliberately
unreachable AMS (isolated `pulse-d164-verify` stack, loopback 38090-92, prod never touched):
**step 6 passed and finished the deploy anyway.** Root cause — it grepped `"status":"ok"`
against the *whole* `/healthz` body, and the `clickhouse` + `meta_store` component objects each
carry their own `"status":"ok"`, so the document matched even while the `collector` component
read `degraded`. The assertion written specifically to stop a blind-collector deploy would not
have stopped one.

Fixed to extract the `"collector":{…}` object first and assert `ok` on *that* (fail-closed: an
empty extraction fails the gate). Re-proven live: negative (dead AMS) → step 6 FAILS + rollback
+ exit 1; positive (reachable mock) → exit 0.

The `/healthz` half of D-164 was genuinely sound: never-succeeded-since-boot degrades at the
30 s floor with the cause named; the prod-outage replay (healthy → AMS stopped) ages out to
`degraded` in ~31 s; recovery to `ok` needs no restart; HTTP 200 throughout. Only the
deploy-script gate was wrong. Lesson recorded: a health gate over a composite document must
scope to the component it claims to check.

## What shipped (D-166, PR #204)

Blockers 2-3 and fast-follows 4-7 from the review, plus the lower items and the D-165
leftovers, plus v0.4.1 prep. Full itemisation is in `decisions.md` D-166; the short list:

- **Blocker 2 (pricing inversion):** Business `MaxNodes` 5 → 50; ladder now monotonic
  (Free 1 / Pro 10 / Business 50 / Enterprise ∞) with a regression test.
- **Blocker 3 (silent Free downgrade):** official vendor pubkey is now the embedded default and
  ships on every documented compose path (`.env.example` uncommented, `real-ams.yml` defaulted).
- **Blocker 1 (release integrity):** v0.4.1 **prepared** — VERSION, Helm tag (goldens
  regenerated, byte-identical to a real render), quickstart/README pins, CHANGELOG `[0.4.1]`.
  The tag cuts on merge; the **GHCR public flip is operator-manual** (no GitHub API — `PATCH
  /user/packages/...` 404, probed side-effect-free).
- **Fast-follow 4:** `ssrfguard.DialControl` on the source-test path + webhook/Slack channels.
- **Fast-follow 5:** `alert.ValidateRuleSpec` wired into create/update (422 on the hostile
  payloads); wildcard `node_down` fires; reader errors hold state; `viewer_drop_pct` →
  `viewer_count_floor` (deprecated alias kept).
- **Fast-follow 6:** new api tokens default to `["read"]` (was silent admin).
- **Fast-follow 7:** alerting-runbook + `PULSE_BASE_URL` (now wired) docs corrected.
- **Lower / D-165 leftovers:** main-port beacon validation, `/metrics` + SHA-256 boot warnings,
  **backup startup race (ROADMAP §2.46) fixed**, SDK unload-beacon auth, onboarding-dup, Helm
  tag, MIGRATION.md IP/user scrub, override.yml security warning.

**Gates:** Go 26/26 pkgs under `-race` (vet + gofmt clean), web 682/682 + lint, SDK 68/68 +
3.53 KB, helm lint + 3 goldens byte-identical. D-164 signals proven live; prod healthy
throughout (720 rows/h, tier enterprise).

## ⚠ Environment truths for whoever reads this cold

- **The concurrent-session question (S100 §4) is RESOLVED.** The "second writer" was the D-165
  release-readiness review session; it is DONE (PR #202 merged). The tree is single-writer again
  — but a **rogue `git restore` fired mid-wave** and reverted the tracked-file edits of four
  fix agents (their new untracked test files survived), plus the RESUME-PROMPT and the
  `deployment.sh` seam. All were re-applied (b3/b5/b6 via a recovery workflow keyed to their own
  reports; b1's server.go by hand from its surviving acceptance test). If you see edits vanish
  under you, **`git status` + re-read before re-doing** — and commit by explicit path, never
  `git add -A`.
- **Prod is on v0.4.0-139-gf9e9c69 and NONE of D-166 has reached it.** The fixes are code on the
  branch; they reach prod only on the next roll after #204 merges AND a stamped rebuild+deploy
  via `deployment.sh`. No prod roll happened this session.
- **v0.4.1 is a tag-on-merge.** `release.yml` fires on `v*`. After the tag builds and pushes to
  GHCR, the package is still **private** until the operator flips it (the one new blocking item).
- Still true from S100: host nginx owns :80/:443 (never touch it/certbot without operator
  direction); Go gates only via docker (`-buildvcs=false`); never `docker compose down -v` on
  prod (safe ONLY for the throwaway verify project name).

## Open on the operator (see `docs/operator-expected.md` ★S101)

1. **★ NEW, blocking-for-listing: flip GHCR `ams-pulse` package to public** — but only *after*
   the v0.4.1 image is pushed (i.e. after #204 merges and the tag builds). No API exists for it.
   Once public, the loop can run the anonymous clean-room install re-verify autonomously.
2. Carried, unchanged: the permanent 2026-07-23 08:58–16:44 UTC data hole; the 18-PR Dependabot
   ruling; final **pricing** sign-off (the inversion is fixed in code; numbers still PROPOSED);
   the 6-step submission sequence (docs-pack review, support/SLA, load lane, demo video, Ankush
   reply); rotate the chat-exposed + VPS-group-readable `deploy/.env` / `oguz-testing.md` secrets.

## What landed this session (updated at close)

PR #204 **merged** to `main` (`e45ea8b`); the `v0.4.1` tag is **pushed** and `release.yml`
**built + pushed `ghcr.io/aytekxr/ams-pulse:0.4.1`** (multi-arch, cosign-signed) to the still-private
package. Two CI-caught corrections happened before merge: the `e2e` hard gate flagged that the new
alert-rule validator over-rejected `window_s:0` + omitted severity (both legitimate) — corrected to
reject only negative/over-cap windows and non-empty unknown severities (decisions.md D-166 addendum),
and a manual-suite follow-up was filed (ROADMAP §2.47, cpu_pct threshold rule). Handoff docs were
updated to the post-tag state on branch `s101-post-tag-handoff`.

## Next session

**The only thing between here and a listable artifact is the operator's GHCR public flip** — the
`0.4.1` image now exists, so the "wait for the image" precondition is met (operator-expected ★S101 §1;
no API for the flip). When the operator reports the package public, run the **anonymous clean-room
install** (isolated compose project, `docker pull ghcr.io/aytekxr/ams-pulse:0.4.1` with no auth →
quickstart → `/healthz` collector `ok` → live dashboard) and record it as the D-166 close-out — that
is blocker 1's last mile and is fully autonomous. Until then, back to the low-frequency gate; **keep
reading prod health at every gate** (a ClickHouse `server_events` count + the `collector` component of
`/healthz`) — that read, not the checklist, has caught the last two live regressions. Prod stays
v0.4.0-139 (D-166 is released, NOT rolled to prod).
