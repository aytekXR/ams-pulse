# SESSION-NN — <title from ROADMAP §3.SN> (TEMPLATE — copy, fill, delete this line)

> Written by SESSION-(NN-1) on <date> per ROADMAP §6. Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP.md`
> (plan of record) + `RESUME-PROMPT.md` §7/§8 (TDD + verification, binding) before dispatching.

## Mission (one paragraph)

<What this session makes true, phrased as exit criteria. Copy the ROADMAP §3.SN Exit line.>

## Preconditions (re-verify cheaply before dispatching — fix this prompt if stale)

- `git log --oneline -3` shows <expected HEAD or later>; working tree clean; CI green (`gh run list --branch main -L 3`).
- <File:line claims this session's work orders rely on — each was verified when this prompt was written on <date>.>
- <Anything the previous session left pending that this one consumes.>

## Work orders (dispatch as a Workflow: disjoint-scope authors → TDD → adversarial verify → ORCH gate/commit)

### WO-1 — <title> · scope `<single-writer scope>` · [S/M/L]
- **Now:** <verified current behavior, file:line>
- **Change:** <what to build>
- **TDD:** <the failing test to write FIRST, its file, and what it asserts; for infra: the falsifiable scripted verification>
- **Verify:** <exact command(s)>

### WO-2 — …

## Gates (ORCH, before any commit)

- Full `-race` suite, REPO-ROOT mount, 0 FAIL / 0 unexpected SKIP; coverage ≥ <target>, ratchet FLOOR to <value> if met.
- Reproduce EVERY ci.yml step touched by this session's changes (D-053/D-055 lesson).
- <Session-specific gates: staging-verify, smoke, contract drift, e2e dispatch…>

## Closing protocol (ROADMAP §6 — the session is NOT done without these)

1. Commit by explicit path; push; `gh run watch` → green.
2. decisions.md: new D-0NN with evidence. RESUME-PROMPT ▶ START HERE updated.
3. ROADMAP §3.SN → ✅ (+refs); §4 coverage ledger; §5 operator ledger.
4. **Write `sessions/SESSION-(NN+1).md`** from ROADMAP §3.S(N+1) + this session's actuals.
   If cut short: SESSION-(NN+1).md = resume prompt for the remainder instead.
