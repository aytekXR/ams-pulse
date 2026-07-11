# qa/realams/Makefile — Real-AMS validation harness runner
#
# Usage (from repo root):
#   make -C qa/realams validate-p0           # run the P0 suite
#   make -C qa/realams validate-TC-L-01      # run one scenario by TC-ID
#   make validate-realams-p0                 # delegated from root Makefile
#
# Design spec: docs/assessment/validation-environment.md
# Scenario matrix: docs/assessment/scenario-matrix.md

SHELL := /bin/bash

REALAMS_DIR  := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
HARNESS_DIR  := $(REALAMS_DIR)harness
SCENARIOS_DIR := $(REALAMS_DIR)scenarios
EVIDENCE_ROOT := $(REALAMS_DIR)evidence

# ── P0 scenario list ──────────────────────────────────────────────────────────
# Ordered: read-only/health, probes, lifecycle, viewer/ingest, webhook.
P0_SCENARIOS := \
  TC-H-02 TC-FL-01 TC-FL-02 TC-H-01 \
  TC-APP-01 TC-APP-02 \
  TC-WH-01 TC-WH-03 \
  TC-P-05 TC-P-06 TC-P-03 TC-P-04 TC-P-01 \
  TC-L-01 TC-F-01 TC-L-03 TC-F-02 TC-L-02 \
  TC-V-04 TC-V-01 TC-V-02 TC-V-03 \
  TC-I-01 TC-I-02 TC-I-06 \
  TC-WH-02

# ── P1 scenario list ──────────────────────────────────────────────────────────
# Ordered: anomaly/analytics read-only first, viewer/ingest, probes, lifecycle,
# stress dead last (per wave work-order constraint).
P1_SCENARIOS := \
  TC-AN-03 TC-AN-05 TC-A-08 TC-A-09 \
  TC-H-06 TC-H-04 TC-H-05 \
  TC-A-05 TC-A-06 \
  TC-V-09 \
  TC-P-07 TC-P-08 \
  TC-V-05 TC-V-06 TC-V-07 TC-V-08 \
  TC-I-04 TC-I-05 TC-I-07 \
  TC-F-06 TC-F-08 \
  TC-L-04 TC-L-05 \
  TC-S-01

.PHONY: help auth validate-p0 validate-p1 validate-all clean-evidence

help: ## List targets
	@grep -E '^[a-zA-Z0-9_%/-]+:.*?## ' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  %-26s %s\n", $$1, $$2}'

# ── auth ──────────────────────────────────────────────────────────────────────

auth: ## Authenticate to AMS once (idempotent; reuses cookie if still valid)
	bash $(HARNESS_DIR)/auth.sh

# ── validate-p0 ───────────────────────────────────────────────────────────────

validate-p0: auth ## Run P0 scenarios in order; SKIP(77)/MISSING do not fail; FAIL → exit 1
	@fail=0; pass=0; skip=0; results=""; \
	for tc in $(P0_SCENARIOS); do \
	  script=$$(ls "$(SCENARIOS_DIR)/$${tc}"-*.sh 2>/dev/null | head -1 || true); \
	  if [ -z "$$script" ]; then \
	    verdict=MISSING; skip=$$((skip+1)); evdir="-"; \
	  else \
	    stamp=$$(mktemp); \
	    bash "$$script"; rc=$$?; \
	    evdir=$$(ls -td "$(EVIDENCE_ROOT)/S*-$${tc}-"* 2>/dev/null | head -1 || echo '-'); \
	    case "$$rc" in \
	      0)  if [ -f "$$evdir/verdict.txt" ] && [ "$$evdir/verdict.txt" -nt "$$stamp" ] \
	             && head -1 "$$evdir/verdict.txt" | grep -q '^PASS'; then \
	            verdict=PASS; pass=$$((pass+1)); \
	          else \
	            verdict=NOEVID; fail=$$((fail+1)); \
	          fi ;; \
	      77) verdict=SKIP; skip=$$((skip+1)) ;; \
	      *)  verdict=FAIL; fail=$$((fail+1)) ;; \
	    esac; \
	    rm -f "$$stamp"; \
	  fi; \
	  results="$${results}$${tc}|$${verdict}|$${evdir}\n"; \
	done; \
	printf '\n%-14s  %-8s  %s\n' SCENARIO VERDICT EVIDENCE_DIR; \
	printf '%s\n' '----------------------------------------------------------------------'; \
	printf '%b' "$$results" | while IFS='|' read -r sc v e; do \
	  printf '%-14s  %-8s  %s\n' "$$sc" "$$v" "$$e"; \
	done; \
	printf '\nP0 summary: pass=%s  skip=%s  fail=%s\n' "$$pass" "$$skip" "$$fail"; \
	[ "$$fail" -eq 0 ]

# ── validate-p1 ───────────────────────────────────────────────────────────────

validate-p1: auth ## Run P1 scenarios in order; SKIP(77)/MISSING do not fail; FAIL → exit 1
	@fail=0; pass=0; skip=0; results=""; \
	for tc in $(P1_SCENARIOS); do \
	  script=$$(ls "$(SCENARIOS_DIR)/$${tc}"-*.sh 2>/dev/null | head -1 || true); \
	  if [ -z "$$script" ]; then \
	    verdict=MISSING; skip=$$((skip+1)); evdir="-"; \
	  else \
	    stamp=$$(mktemp); \
	    bash "$$script"; rc=$$?; \
	    evdir=$$(ls -td "$(EVIDENCE_ROOT)/S*-$${tc}-"* 2>/dev/null | head -1 || echo '-'); \
	    case "$$rc" in \
	      0)  if [ -f "$$evdir/verdict.txt" ] && [ "$$evdir/verdict.txt" -nt "$$stamp" ] \
	             && head -1 "$$evdir/verdict.txt" | grep -q '^PASS'; then \
	            verdict=PASS; pass=$$((pass+1)); \
	          else \
	            verdict=NOEVID; fail=$$((fail+1)); \
	          fi ;; \
	      77) verdict=SKIP; skip=$$((skip+1)) ;; \
	      *)  verdict=FAIL; fail=$$((fail+1)) ;; \
	    esac; \
	    rm -f "$$stamp"; \
	  fi; \
	  results="$${results}$${tc}|$${verdict}|$${evdir}\n"; \
	done; \
	printf '\n%-14s  %-8s  %s\n' SCENARIO VERDICT EVIDENCE_DIR; \
	printf '%s\n' '----------------------------------------------------------------------'; \
	printf '%b' "$$results" | while IFS='|' read -r sc v e; do \
	  printf '%-14s  %-8s  %s\n' "$$sc" "$$v" "$$e"; \
	done; \
	printf '\nP1 summary: pass=%s  skip=%s  fail=%s\n' "$$pass" "$$skip" "$$fail"; \
	[ "$$fail" -eq 0 ]

# ── validate-all ──────────────────────────────────────────────────────────────

validate-all: auth ## Run every TC-*.sh in scenarios/ (sorted); SKIP(77) OK; FAIL → exit 1
	@fail=0; pass=0; skip=0; results=""; \
	for script in $$(ls "$(SCENARIOS_DIR)"/TC-*.sh 2>/dev/null | sort || true); do \
	  tc=$$(basename "$$script" .sh | cut -d- -f1-3); \
	  stamp=$$(mktemp); \
	  bash "$$script"; rc=$$?; \
	  evdir=$$(ls -td "$(EVIDENCE_ROOT)/S*-$${tc}-"* 2>/dev/null | head -1 || echo '-'); \
	  case "$$rc" in \
	    0)  if [ -f "$$evdir/verdict.txt" ] && [ "$$evdir/verdict.txt" -nt "$$stamp" ] \
	           && head -1 "$$evdir/verdict.txt" | grep -q '^PASS'; then \
	          verdict=PASS; pass=$$((pass+1)); \
	        else \
	          verdict=NOEVID; fail=$$((fail+1)); \
	        fi ;; \
	    77) verdict=SKIP; skip=$$((skip+1)) ;; \
	    *)  verdict=FAIL; fail=$$((fail+1)) ;; \
	  esac; \
	  rm -f "$$stamp"; \
	  results="$${results}$${tc}|$${verdict}|$${evdir}\n"; \
	done; \
	printf '\n%-14s  %-8s  %s\n' SCENARIO VERDICT EVIDENCE_DIR; \
	printf '%s\n' '----------------------------------------------------------------------'; \
	printf '%b' "$$results" | while IFS='|' read -r sc v e; do \
	  printf '%-14s  %-8s  %s\n' "$$sc" "$$v" "$$e"; \
	done; \
	printf '\nAll-scenarios summary: pass=%s  skip=%s  fail=%s\n' "$$pass" "$$skip" "$$fail"; \
	[ "$$fail" -eq 0 ]

# ── validate-<TC-ID> ─────────────────────────────────────────────────────────
# Pattern rule — explicit targets (validate-p0, validate-all) take precedence.
# Resolves scenario by glob so exact slug doesn't matter.

validate-%: auth ## Run one scenario by TC-ID (e.g. make validate-TC-L-01)
	@tc="$*"; \
	script=$$(ls "$(SCENARIOS_DIR)/$${tc}"-*.sh 2>/dev/null | head -1 || true); \
	if [ -z "$$script" ]; then \
	  echo "[validate] ERROR: no script found matching scenarios/$${tc}-*.sh" >&2; \
	  exit 2; \
	fi; \
	echo "[validate] Running $$script" >&2; \
	bash "$$script"

# ── clean-evidence ────────────────────────────────────────────────────────────
# Containers sometimes write root-owned files; use Docker to delete them safely.

clean-evidence: ## Remove all evidence artifacts (handles root-owned files via Docker)
	@echo "[clean-evidence] Removing $(EVIDENCE_ROOT)/*"
	@sg docker -c "docker run --rm \
	  -v '$(EVIDENCE_ROOT)':/s \
	  alpine sh -c 'rm -rf /s/*'"
	@echo "[clean-evidence] Done."
