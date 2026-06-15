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
# Required status checks = the ci.yml job names. The e2e workflow (e2e.yml) is
# intentionally NOT required: it spins a full Docker Compose stack and runs only
# on pull_request + workflow_dispatch — too slow/heavy for a mandatory gate until
# GitHub-hosted-runner timing is validated (D-020 limitation).
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
    "contexts": ["contracts", "server", "web", "sdk", "docker-build", "helm", "compose"]
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
