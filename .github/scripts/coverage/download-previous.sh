#!/usr/bin/env bash

set -euo pipefail

REPO="${REPO:?Required environment variable REPO}"
BRANCH="${BRANCH:?Required environment variable BRANCH}"
CURRENT_RUN_ID="${CURRENT_RUN_ID:?Required environment variable CURRENT_RUN_ID}"
OUTPUT_DIR="${1:-previous}"

mkdir -p "$OUTPUT_DIR"

# Find last successful CI run on this branch (excluding current)
PREV_RUN_ID=$(gh api "repos/${REPO}/actions/workflows/ci.yml/runs?branch=${BRANCH}&status=success&per_page=5" \
	--jq "[.workflow_runs[] | select(.id != ${CURRENT_RUN_ID})][0].id")

if [[ -z "$PREV_RUN_ID" || "$PREV_RUN_ID" == "null" ]]; then
	echo "No previous successful run found on $BRANCH"
	echo "has_previous=false"
	exit 0
fi

echo "Previous run: $PREV_RUN_ID"

# Find the coverage-summary.json artifact in that run
ARTIFACT_ID=$(gh api "repos/${REPO}/actions/runs/${PREV_RUN_ID}/artifacts" \
	--jq '[.artifacts[] | select(.name == "coverage-summary.json")][0].id')

if [[ -z "$ARTIFACT_ID" || "$ARTIFACT_ID" == "null" ]]; then
	echo "No coverage artifact in previous run"
	echo "has_previous=false"
	exit 0
fi

# Download and extract
gh api "repos/${REPO}/actions/artifacts/${ARTIFACT_ID}/zip" >/tmp/prev-coverage.zip
unzip -o /tmp/prev-coverage.zip -d "$OUTPUT_DIR/"
echo "has_previous=true"
