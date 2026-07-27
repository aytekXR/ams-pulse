# External review — Round 10 · FINAL (2026-07-27)

**Reviewed tree:** `a59c2ca` (`main`, `v0.4.4-5-ga59c2ca`) — the D-183 round-9-fixes commit ·
**published Latest:** tag `v0.4.4` = commit `34a25fc` (re-confirmed via `/releases/latest`, GHCR,
README, VERSION — all five signals agree, no `v0.4.5`)
**Reviewer:** Claude (Cowork session; same reviewer as rounds 6–9) · **Phases:** blackbox / docs /
code / security deep-dive / PRD F1–F10 conformance
**Finding prefix:** `L-` · **Prior-round input:** `external-review-2026-07-27-round9.md`
**Reviewer verdict:** *"Ready to submit the moment G-02 (`CLICKHOUSE_PASSWORD` rotation) closes.
One more tag is needed, not one more review round."*

**Maintainer disposition:** D-184 (§S116). Transcribed from the round-10 delivery (chat text —
see the delivery-channel note below), Disposition column filled per the protocol in
`EXTERNAL-REVIEW-PROMPT.md`.

---

## Maintainer summary (D-184)

**Both filed findings are REFUTED as filed, and both point at real defects the reviewer did not
find.** This is the inverse of the usual round: the reviewer's evidence failed on both items, but
neither finding was noise — each was aimed at a genuine class, and each class turned out to be
wider than filed once swept.

- **L-01** accused `review-chain.md` of containing a false statement, on the strength of "four
  independent receipts". The receipts describe a working tree that is not this repository: the
  two files do not exist here, have no history on any ref, are nowhere on the operator's
  filesystem, and the `.git/index.lock` the finding rests on does not exist either. **But L-01's
  underlying objection is correct** — the file did assert something about the reviewer's
  *delivery contents*, which this side cannot observe. That sentence is now scoped to what we can
  verify. More usefully, chasing L-01's evidence identified **the mechanism behind three rounds of
  this dispute** (J-03 → K-01 → L-01): the reviewer's device bridge is attached to a mirror of the
  repository, not the repository. Chat-delivered prose arrives here; bridge-written files never do.
  Both sides have been describing their own vantage accurately and disagreeing for three rounds.
- **L-02** filed the README footer stamp as stale. **It was correct** — README was last *modified*
  in D-182 and its stamp said D-182, which is what a last-updated stamp is for. Sweeping the class
  tree-wide found the drift in **seven other stamps across five other files**, including two
  customer-facing docs (`faq.md`, `overview.md` — the latter 13 sessions stale). Fourth
  consecutive round in which this class landed in whatever file the previous fix did not touch, so
  it is now closed **mechanically** rather than by hand: `.github/check-doc-stamps.sh` + a CI job.
  The checker found four instances that this session's own manual sweep had missed.
- **The round's exculpations were audited adversarially, and this is where the session's real
  work turned out to be.** The round is ~90% reassurance — a security all-clear over a core the
  loop had never read, plus a PRD conformance table — and S114's lesson is that a reviewer's
  "this one is safe" is the one place nobody looks twice. Nine lanes, each told to default to
  refuted; every surviving claim then re-verified by hand. **The all-clear holds in five areas of
  nine and fails in three**, yielding four maintainer-found defects: the SSRF guard is missing
  from three outbound clients that dial API-supplied addresses (**M-01**), `/ingest/beacon` has
  two implementations and the *documented default* is the one without the field limits the
  reviewer credited (**M-02**), paid alert channels keep delivering after a tier downgrade while
  the prober one package over has exactly the runtime gate they lack (**M-03**), and four beacon
  identity fields are length-unbounded (**M-04**, deliberately not fixed — see below). M-01
  through M-03 are fixed in D-184 under TDD. Full method and evidence in the *Exculpation audit*
  section.
- **G-02 remains open and remains the only submission blocker.** Re-checked silently this session:
  the live `CLICKHOUSE_PASSWORD` 32-char prefix still matches 2 commits in public history.

---

## Findings — as filed, with disposition

| ID | Sev (filed) | Sev (agreed) | Phase | Subject | Verified? | Disposition |
|---|---|---|---|---|---|---|
| **L-01** | MEDIUM | LOW | docs | `review-chain.md` states as fact something a `git status` in its own directory disproves | **Objection CONFIRMED; supporting evidence REFUTED (third consecutive round)** | **FIXED, and the mechanism behind the recurring dispute identified.** The wording objection is right: the file claimed what the reviewer's *deliveries contained*, which we cannot observe — only their effect here. It now claims only that no round-6/7 file has ever reached this repository, which is verifiable and stays true regardless of what the reviewer's channel reports. The evidence is refuted on five independent probes (below). Chasing it produced the useful result: the reviewer's bridge writes to a mirror, so its file receipts are not evidence about this tree — and rounds 6–7 prose is still recoverable, but only via chat text, the channel that demonstrably works |
| **L-02** | LOW | LOW | docs | README footer D-stamp lags the tree by one session | **REFUTED as filed; class CONFIRMED and 4× wider** | **FIXED wider than filed, and closed mechanically.** The README stamp was accurate — the file was last modified in `9928f70` (D-182) and said D-182. A stamp naming the session in which the file last changed is not "lagging the tree"; that is its job. The real drift was in `faq.md` (said D-176, last changed D-180), `overview.md` (said D-163, last changed D-176), `ARCHITECTURE.md` (said D-179, last changed D-181) and four non-date stamps in `ARCHITECTURE.md` §§ and `runbooks/alerting.md`. All eight stamps are now date-valued; `.github/check-doc-stamps.sh` enforces it in CI |
| **G-02 / F-01** | — | — | security | rotate `CLICKHOUSE_PASSWORD` | **STILL OPEN** | **Operator-gated, unchanged.** Tenth consecutive round as the sole submission blocker. Nothing in D-184 touches it — only the operator can |

---

## Reviewer premises we refuted

- **"`docs/assessment/external-review-2026-07-27-round6.md` (42,463 bytes) and `…round7.md`
  (24,066 bytes) are present on disk right now, both my verbatim ledgers"** (L-01) — refuted on
  five independent probes at `a59c2ca`:

  | Probe | Result |
  |---|---|
  | `git status --porcelain --untracked-files=all --ignored docs/assessment/` | empty |
  | `git log --all -- …round6.md …round7.md` | no history on any ref |
  | `find / -xdev -name 'external-review-2026-07-27-round*.md'` | only round 8 and round 9 |
  | `git worktree list`; search for other checkouts | one worktree, one checkout on the box |
  | `ls -la .git/index.lock` | **absent** |

  The byte sizes also do not match the similarly-named maintainer files that *do* exist:
  `marketplace-compliance-review-2026-07-27-round6.md` is 12,422 bytes, not 42,463.

- **"a stale zero-byte `.git/index.lock` is present in the connected repo, and git write ops in the
  folder currently error (`unable to unlink index.lock: Operation not permitted`)"** — refuted.
  No lock exists, `.git` is writable, the repo is on native ext4, and D-183 was committed and
  pushed from this tree. A lock its own owner cannot unlink on a native ext4 home directory is
  characteristic of an overlay or bind-mounted sandbox. **This is the tell that identified the
  mechanism**, so the observation was valuable even though the conclusion drawn from it was wrong.

- **"round 9's delivery demonstrably carried three files … proving the delivery channel reaches
  them"** — refuted for this repository. Rounds 8 and 9 entered the tree in maintainer commits
  `9928f70` and `a59c2ca`, transcribed from chat text. No bridge-written file has ever arrived.
  The receipts are presumably accurate *about the mirror*.

- **"README@main footer reads D-182 while the tree is at D-183 … the same I-03/I-01-class prose
  drift recurs one notch"** (L-02) — refuted. `git log -- README.md` shows the last modification is
  `9928f70` (D-182); `a59c2ca` (D-183) does not touch README.md. The stamp was correct.

**Symmetry note, recorded deliberately.** D-183 refuted K-01's evidence and was right to; L-01 then
accused D-183 of writing a falsehood while doing so, and was wrong on the facts but right that the
sentence overreached. Neither side has been dishonest in any round of this dispute. The cause was
structural and is now named.

---

## Finding detail

### L-01 — the review ledger's own honesty artifact contains a falsifiable statement  [filed MEDIUM · docs]

**Reviewer's claim.** `review-chain.md` (rewritten in D-183 to disposition K-01) asserts (1) in the
K-01 disposition row, "The claimed untracked file does not exist and never did"; (2) in
`review-chain.md` itself, "neither ledger has been attached to any delivery, including round 9's,
which offered to re-supply both and again contained no files." The reviewer reports both false
against the operator's working tree, citing `git status --porcelain` showing two `??` entries,
`head -1` of each file, and `wc -c` of 42,463.

**Reviewer's rationale (recorded in full because it is fair-minded).** The reviewer explicitly
declined to allege dishonesty, offered the charitable mechanism (a maintainer working from a
separate session's partial view), flagged the stale `index.lock` as the plausible reason files were
"delivered but never `git add`ed", and confined the finding to what a `git status` proves. They
also rated their own severity down from MEDIUM on the merits and up again on the "false statement
inside the honesty ledger" character.

**Disposition: objection CONFIRMED, evidence REFUTED, mechanism IDENTIFIED.**

*Statement (1) is accurate.* No file of that name exists here, tracked or untracked, and none ever
has — five probes above, re-run from scratch this session rather than inherited from D-183.

*Statement (2) overreached, and L-01 is right about that.* "…again contained no files" is a claim
about what the reviewer's delivery contained. We cannot see the reviewer's outbound channel; we can
see only that nothing arrived. The sentence now reads as a claim about arrival, not about contents,
and is true independently of what any receipt on the other side reports. The same overreach appears
in D-183's round-9 ledger ("the round-9 delivery contained no attachments") and is corrected there
by this file rather than by rewriting the closed round-9 record.

*The mechanism.* The `index.lock` detail is what resolves it. The reviewer sees a lock that cannot
be unlinked; this tree has no lock and git writes fine. Combined with files that exist there and
not here, the only consistent explanation is that the bridge is attached to a **mirror** of the
repository. Writes land in the mirror, are reported successful to the review session, and never
reach the operator. Rounds 8 and 9 arrived because they were pasted as chat text and transcribed by
the maintainer. **Recorded in `review-chain.md` so round 11 does not spend a fourth round on it.**

*What this changes going forward:* the reviewer's file-write receipts are not evidence about this
tree, and our inferences about their delivery's contents are not evidence about their session.
Rounds 6–7 prose remains recoverable — by pasting it as chat text.

**Severity agreed at LOW,** not MEDIUM: the filed severity rested on the statement being false, and
the statement was true. The surviving defect is a scope-of-claim imprecision in an internal
bookkeeping document, with zero product blast radius.

### L-02 — README footer stamp one session stale  [filed LOW · docs]

**Reviewer's claim.** README@main stamps "Last updated: 2026-07-27 (D-182)" at a D-183 tree; guard
#19 pins the release-pointer version but not the D-stamp, so the I-03/I-01-class drift recurs.
Proposed fix: drop the session-scoped D-number from the footer, or add it to guard #19.

**Disposition: REFUTED as filed; the class is CONFIRMED and four times wider; FIXED mechanically.**

The README stamp was **correct**. `git log -- README.md` → last modification `9928f70` = D-182;
`a59c2ca` = D-183 does not touch README.md. A "last updated" stamp naming the session in which the
file last changed is doing exactly its job; "lags the tree" is not the defect definition.

The reviewer's *instinct* was right and their scope was one file wide. Sweeping every tracked `.md`:

| File | Declared | Last actually changed in | Verdict |
|---|---|---|---|
| `README.md` | D-182 | D-182 | **correct as filed-against** |
| `docs/faq.md` | D-176 | D-180 | stale — customer-facing |
| `docs/overview.md` | D-163 | D-176 | stale by 13 sessions — customer-facing |
| `docs/ARCHITECTURE.md` | D-179 | D-181 | stale |
| `docs/compatibility.md` | D-183 | D-183 | correct |
| `agents/handoffs/decisions.md` | — | — | **false positive in our own first sweep** (body text, not a header stamp) |
| `brandkit/uploads/ARCHITECTURE.md` | D-062 | D-071 | frozen upload snapshot — correctly excluded |

Plus four **non-date** stamps that a first-match-per-file grep missed entirely and the mechanical
checker caught: `docs/ARCHITECTURE.md` §§ 45/68/97 ("Wave-3-Plus complete (2026-06-15)") and
`docs/runbooks/alerting.md:5` ("V3b fix-loop (2026-06-15)").

**Fix — de-literalized, per the standing rule that whatever drifts twice stops being a literal.**
Every stamp is now **date-valued**; session D-numbers survive as prose provenance *after* the date,
where they are informative and cannot rot into a false claim. Neither of the reviewer's two proposed
fixes was adopted as stated: option (b) — add the D-stamp to guard #19 — is not implementable in the
form proposed, because guard #19 runs at *tag* time against `TAG_VERSION` and a session D-number has
no relationship to the tag being cut. Option (a) is adopted and generalized past the one file.

**The class is now closed mechanically:** `.github/check-doc-stamps.sh`, run by a new `doc-stamps`
CI job, enforces two things — (A) every stamp's value is an ISO date, always fatal; (B) if a change
modifies a stamped doc, that change must also move its stamp, fatal whenever a base ref is
available. Check B is what actually catches forgetting, and unlike a stamp-date-vs-commit-date
comparison it produces no false positive when a PR merges days after it was written. **What the
guard does NOT cover is written at the top of the script** (docs with no stamp; the truthfulness of
the date as opposed to its shape; prose D-numbers after the date; the three excluded
immutable-by-design trees) — per the rule earned in S113 and re-earned in S114 and S115.

---

## Exculpation audit — the round's security all-clear, adversarially tested (D-184)

Round 10 is roughly 90% reassurance: a security deep-dive over a core the loop had never audited,
declared clean, plus a PRD F1–F10 conformance table. **S114's lesson is that a reviewer's "this
one is safe" is the one place nobody looks twice** — round 8's `GeoBreakdown` all-clear was wrong
and was hiding a defect larger than the one it filed. So the round's exculpations were tested the
way its accusations would have been: nine adversarial lanes, one per claim cluster, each instructed
to default to refuted and to hunt specifically for the scoping gap (the sibling call site the
reviewer's one-file verification did not cover), then an independent second opinion over every
claimed refutation.

**Every surviving claim below was then re-verified by hand against the source before being written
down.** That mattered: the automated verifiers' prose and their structured output disagreed in two
lanes, and two claims that read as confirmed did not survive a direct read.

### Result: the all-clear holds in 5 areas of 9, and fails in 3

| Area | Reviewer's claim | Verdict |
|---|---|---|
| `ssrfguard` **policy** | link-local/IMDS + IMDSv6 denial, IPv4-mapped/NAT64/IPv4-compat reduction, scheme allowlist, `IsDenied(nil)` fails closed | **UPHELD** — every sub-claim confirmed at `ssrfguard.go`, with tests. Loopback and RFC-1918 are *deliberately* allowed (documented, for real AMS nodes) |
| Secret handling at rest | AES-256-GCM, domain-separated key derivation, fail-closed startup, HMAC tokens, bcrypt | **UPHELD** — no refutation survived |
| Auth transport hygiene | header-only bearer, two explicit exceptions, token-kind enforcement, no blocking under the limiter mutex | **UPHELD** — no refutation survived |
| Audit trail | every mutating admin/config handler records an actor | **UPHELD** — no mutating handler was found missing an audit call outside the enumerated out-of-scope list |
| PRD LIM citations | 16 numbered limitations cited across F1–F10 | **UPHELD** — every cited entry exists and says what it was used to say |
| Inbound webhook HMAC | constant-time, fail-closed, per-source isolation with no cross-source fallback | **UPHELD** — one claimed isolation gap (unknown source name falls back to the shared secret) was **refuted**: it is deliberate, documented and pinned by `TestPerSource_UnknownName_SharedSecretFallback_200` |
| **SSRF guard wiring** | "the prober and the webhook alert channel both fetch operator-stored URLs" — guard installed | **FAILS — 3 unguarded clients** → **M-01** |
| **Beacon ingest hardening** | "per-field 64-byte UTF-8-safe truncation (tenant, data-string values)" | **FAILS on the documented default path** → **M-02**, **M-04** |
| **Tier enforcement** | "every paid surface gated server-side in the handler or deeper" | **FAILS for runtime alert delivery** → **M-03** |

### M-01 — the SSRF guard is installed on five outbound clients and missing from three  [MEDIUM]

The guard itself is excellent, and the reviewer is right about that. What the all-clear did not
check is *coverage*: it verified the wiring in `channels/webhook.go` and generalized. Sweeping every
outbound client that dials an address a caller can supply:

| Client | Address source | `ssrfguard.DialControl` |
|---|---|---|
| Webhook channel, Slack channel, prober HTTP, RTMP probe, AMS connectivity test | operator config / API | **installed** |
| **Email channel** — `channels.go` `dialer := &net.Dialer{}` → `DialContext(ctx, "tcp", cfg.SMTPAddr)` | `smtp_addr` in the alert-channel config, **API-supplied** | **missing** |
| **CertChecker** — `wave2.go` `tls.Dialer{NetDialer: &net.Dialer{}}`, wired in prod at `serve.go:541` | `scope.StreamID` used as `host:port` on a `cert_expiry` rule, **API-supplied** | **missing** |
| **S3 uploader** — `reports/s3.go` `&http.Client{Timeout: 60s}` | `PULSE_S3_ENDPOINT`, environment | **missing** |

An authenticated operator can set `smtp_addr` to `169.254.169.254:25`, or create a `cert_expiry`
rule scoped to `169.254.169.254:443`, and Pulse will dial the cloud metadata service. **This is not
an unauthenticated hole** — all three require credentials that already grant configuration rights,
and on a single-tenant self-hosted install the operator could reach the metadata service anyway. It
is filed at MEDIUM because it is a **scoping gap in a control this codebase already decided it
wants**, and because a multi-tenant or delegated-admin deployment turns it into privilege
escalation. **Fixed in D-184**, and fixed in the *constructors* rather than at the call sites —
the class exists precisely because the guard was wiring-dependent.

### M-02 — `/ingest/beacon` has two implementations and the documented one is the unhardened one  [MEDIUM]

The A10 field limits the reviewer credits — tenant and event-`data` string values truncated to 64
bytes, UTF-8-safe — live in `internal/collector/beacon`, the **optional** listener enabled only by
`PULSE_INGEST_LISTEN_ADDR`. The main-port route `r.Post("/ingest/beacon", s.handleIngestBeacon)`
in `internal/api/server.go` is a **second, separate implementation** that applies neither.

The main-port route is the documented default: `docs/beacon-sdk.md` tells operators to set
`ingestUrl` to their Pulse base URL and the SDK posts to `ingestUrl + /ingest/beacon`.

The main-port handler is *not* generally unhardened — it enforces the Pro-tier licence gate, the
ingest token with `kind` check, the per-token 100 rps / burst 200 limit, and the 64 KB body cap,
and since **S101 it shares `ValidateRawBatch`** with the dedicated port. That shared validation is
the tell: **S101 fixed this exact divergence once, scoped to schema validation, and left the field
limits behind** — the repository's signature defect class, found for the fifth review round running.
**Fixed in D-184** by sharing one helper between both paths so they cannot diverge a third time.

### M-03 — paid alert channels keep delivering after a downgrade  [MEDIUM]

`internal/api/server.go` gates channel create, update **and** test-fire with
`lic.CheckChannelAllowed(type)`, and its own comments name the downgrade case ("a Free (or
downgraded) tenant cannot send live Slack/PagerDuty/webhook tests"). But the background evaluator
never consults it: `syncRegistryFromStore` rebuilds **every** stored channel and `deliver()` sends
to **every** registered channel with no entitlement check. A tenant that configures a webhook or
Slack channel on Business and drops to Free keeps receiving deliveries indefinitely.

The decisive evidence that this is a defect and not a design choice is that **the identical gate
already exists one package over**: `prober.executeProbe` consults an `EntitlementGate`, wired at
`serve.go` as `lic.CheckProbes`, with the comment "a tenant that downgrades below the probe tier
stops probing at runtime, not just at the HTTP CRUD boundary (S37 / D-108)". Alert delivery is the
same shape and never got the same treatment. **Fixed in D-184** by mirroring the prober's design.

### M-04 — `session_id` / `stream_id` / `app` / `player_kind` are length-unbounded  [LOW — recorded, deliberately NOT fixed]

Neither beacon path, and no schema rule, bounds these four fields; `validateBeaconBatch` requires
`session_id` and `stream_id` to be non-empty but not to be short, and
`contracts/events/beacon-event.schema.json` sets no `maxLength`. A holder of a valid ingest token
can write very large high-cardinality strings into ClickHouse, bounded only by the 64 KB body cap
and the per-token rate limit.

**Not fixed, on purpose, and this is the honest reason:** truncating a `session_id` would silently
**merge distinct sessions**, corrupting sessionization and the `uniq(session_id)` aggregates that
K-02 was about — a worse defect than the one it closes. Rejecting over-long ids instead is a
behavioural break that belongs in a contract change, and contracts are frozen (D-004). This needs a
product ruling on limit *and* failure mode, so it is recorded here and added to the operator's
decision-gated list rather than guessed at.

### Noted, examined, and deliberately not filed

- **6to4 (`2002::/16`) and Teredo (`2001::/32`) embedded-IPv4 forms are not reduced** by
  `embeddedIPv4`, which handles IPv4-mapped, NAT64 and IPv4-compatible. Reaching an IMDS address
  this way needs a 6to4 relay on the path; 6to4 is deprecated (RFC 7526) and effectively undeployed.
  Real but not reachable in any plausible deployment — recorded, not fixed.
- **Beacon rate limiting is per-token and runs after token validation**, and there is no global or
  per-IP limiter on the main router, so an attacker without a token can spin the SHA-256 +
  meta-store lookup path. This does not refute the reviewer's literal claim, which is about
  per-token limits and is true. It is a generic property of authenticated endpoints, mitigated in
  the reference deployment by the host nginx in front. Recorded.
- **The AMS REST client uses a plain `http.Client`** with a base URL from `PULSE_AMS_BASE_URL`.
  Environment-configured, not API-supplied, so it requires host access — no privilege boundary is
  crossed. Recorded, not fixed.

### What this says about the round

The reviewer's security work was **substantially right and genuinely valuable** — the policy
analysis in particular is accurate down to the NAT64 reduction, and five of nine areas came back
clean under adversarial attack. What failed was not the analysis but the **generalization step**:
"the webhook channel wires the guard" became "operator-controlled outbound URLs are guarded", and
"the beacon package truncates fields" became "beacon ingest truncates fields". Both were true of the
file that was read and false of the system. **That is the same generalization error the reviewer
has now corrected in themselves three times** (I-03 stamps, I-05 comment sites, J-03's span) and
committed to sweeping for — applied here to an exculpation rather than to a finding, where their own
errata rule did not think to look.

---

## Prior-round re-audit (round 9: K-01, K-02, plus the standing gate)

| Prior ID | D-183 disposition | Verified state now | Note |
|---|---|---|---|
| K-01 | CONFIRMED (wording) / REFUTED (evidence) — FIXED | **Both halves upheld; the refutation itself survives L-01's challenge** | The rounds 4–5 / rounds 6–7 separation is intact and accurate. L-01 attacked the evidence half and lost on five probes. D-183's one genuine overreach — describing the reviewer's delivery contents — is corrected by this round, which is L-01's real contribution |
| K-02 | FIXED wider than filed (4 sites) | **VERIFIED, and the reviewer now concurs** | Round 10 explicitly endorses the maintainer's refusal of the proposed wording, on the grounds that a race shifts the tail either way. Loop working as intended in both directions |
| G-02 / F-01 | STILL OPEN, operator-gated | **STILL OPEN** | Live prefix still matches 2 commits. Tenth round; the only remaining gate; not closeable by any review |

---

## Loop-closure — maintainer's position

The reviewer's verdict is *"the last review round, provided it is closed deliberately with the
rotate → cut → submit motion"*. **Agreed on the engineering, with one correction to the framing.**

The convergence signal is real: H(9) → I(6) → J(3) → K(2) → L(2, both refuted-as-filed). No new
product code defect has survived verification since round 6. This round's two findings were both
about bookkeeping, and one of them was about the review loop itself — the textbook signal to stop.

The correction: **round 10 was not a wasted round even though both findings were refuted.** It
produced the delivery-channel diagnosis that ends a three-round dispute, and its L-02 instinct —
wrong about the file it named — is what surfaced seven genuinely stale stamps and got the class
closed mechanically. A round whose findings are refuted can still be the round that pays for itself.

**The submission motion is unchanged and remains operator-gated:** rotate `CLICKHOUSE_PASSWORD`
(G-02) → cut `v0.4.5` so the D-180…D-184 fixes reach a release → submit against v0.4.5. Rounds
7–10's fixes, and the behavioural ones from D-181/D-182 (breakdown row caps, enrichment cardinality
bound, per-call RTT), still sit only on `main`. **Neither the rotation nor the tag is a maintainer
action** — both are recorded in `docs/operator-expected.md`.
