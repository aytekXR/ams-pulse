#!/usr/bin/env bash
# Enable branch protection on main so broken changes cannot merge (W1 / D-020).
#
# Prerequisites:
#   - gh CLI installed and authenticated as a repo ADMIN of aytekXR/ams-pulse
#     (gh is NOT installed on the build VPS — run this from an admin's machine,
#      or: `! gh auth login` in a Claude Code session once gh is available).
#   - The `ci` workflow has run at least once on a PR so GitHub knows the check
#     names (contexts) below.
#
# Required status checks = the 15-context list below: the ci.yml job names, CodeQL
# analyses, and the e2e workflow jobs (e2e / csp-e2e / web-e2e were promoted to
# hard gates at D-162; sdk-swift added D-153; shellcheck + doc-stamps added D-185).
# Keep this list in sync with what `gh api .../protection/required_status_checks`
# reports — this script is the restore path if protection is ever reset.
#
# D-185 (external review round 11): a job that runs but is not listed here does
# NOT block a merge. D-184 closed the doc-stamp drift class "mechanically" with
# the doc-stamps job — but the job was advisory, so the class was not actually
# closed. Adding a guard job without adding its context here reproduces exactly
# the defect the guard was written to prevent. **When you add a CI job that is
# meant to gate, add it here in the same change.**
#
# Deliberately NOT required: `compose-boot`. It pulls the pinned image from GHCR,
# so requiring it would make every merge depend on registry availability. It is
# still a real gate on main; it is just not allowed to hold a PR hostage to a
# third party's uptime.
#
# Fallback local verification (no GitHub needed): make build && make test
set -euo pipefail

REPO="${REPO:-aytekXR/ams-pulse}"
BRANCH="${BRANCH:-main}"

echo "Enabling branch protection on ${REPO}:${BRANCH} ..."

gh api -X PUT "repos/${REPO}/branches/${BRANCH}/protection" \
  --header "Accept: application/vnd.github+json" \
  --input - <<'EOF'
{
  "required_status_checks": {
    "strict": true,
    "contexts": ["contracts", "server", "web", "sdk", "docker-build", "helm", "compose", "Analyze (go)", "Analyze (javascript-typescript)", "e2e", "csp-e2e", "web-e2e", "sdk-swift", "shellcheck", "doc-stamps"]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "required_approving_review_count": 1,
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": false
  },
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "block_creations": false,
  "required_conversation_resolution": true
}
EOF

echo "Branch protection applied to ${REPO}:${BRANCH}."
