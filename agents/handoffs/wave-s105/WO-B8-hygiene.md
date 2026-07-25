# WO-B8 (S105) — hygiene: issue template, IP scrub, poem relocation

## Scope — you may ONLY edit/create these files
- `.github/ISSUE_TEMPLATE/bug_report.yml` (create)
- `.github/ISSUE_TEMPLATE/config.yml` (create)
- `deploy/runbooks/upgrade-rollback.md`
- `poem.md` → moved (see item 3; you may use `git mv` for THIS move only — no other git commands)

## Work items

1. **Issue template** (`.github/ISSUE_TEMPLATE/bug_report.yml`): GitHub issue-forms YAML.
   Fields per docs/support.md §4's bug-report format (read it first): Pulse version + tier
   (point at `pulse diag` / Settings → License), AMS version/edition, deploy method
   (quickstart/compose/helm/source), what happened vs expected, repro steps, relevant logs
   (collapsed textarea), environment (OS/arch). Labels: bug. Keep it tight — 6-8 fields max.
   `config.yml`: `blank_issues_enabled: true`, contact link to support@beyondkaira.com (mailto)
   for Pro+ support per docs/support.md. Check with `gh api repos/aytekXR/ams-pulse --jq
   .has_discussions` whether Discussions exist — add that link only if true.
   Validate YAML parses (python yaml).

2. **`deploy/runbooks/upgrade-rollback.md`:** replace the literal VPS IP `161.97.172.146` at
   lines ~3, ~114, ~125, ~168 with `<VPS_IP>` (keep the beyondkaira.com hostname mentions —
   public DNS). Make sure surrounding commands still read correctly (e.g. ssh examples become
   `ssh <user>@<VPS_IP>`). This file is in the externally-linkable submission set; the other
   files containing the IP (assessment docs, qa/ load-lane guard) are intentionally untouched —
   the load-lane guard NEEDS the raw IP to refuse prod.

3. **`poem.md`:** relocate from repo root to `agents/poem.md` via `git mv poem.md agents/poem.md`
   (content unchanged — it is a harmless internal artifact, but a vendor-reviewed snapshot
   shouldn't have it at root).

## Definition of done
Template renders as valid issue-forms YAML; no raw VPS IP left in upgrade-rollback.md; poem
moved intact. Return: files changed/created, the has_discussions result, YAML validation output.
