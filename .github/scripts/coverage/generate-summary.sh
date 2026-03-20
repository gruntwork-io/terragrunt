#!/usr/bin/env bash

set -euo pipefail

COVER_FILE="${1:-coverage.out}"
OUTPUT="${2:-coverage-summary.json}"

if [[ ! -f "$COVER_FILE" ]]; then
	echo "Error: coverage file '$COVER_FILE' not found"
	exit 1
fi

TOTAL=$(go tool cover -func="$COVER_FILE" | grep '^total:' | awk '{print $NF}' | tr -d '%')

# Build per-package JSON: average coverage per package (go tool cover reports per-function)
PACKAGES_JSON=$(go tool cover -func="$COVER_FILE" \
	| grep -v '^total:' \
	| awk -F'\t+' '{
		# Field 1: file:line, Field 3: coverage%
		split($1, parts, ":")
		file = parts[1]
		# Strip filename to get package path
		n = split(file, segs, "/")
		pkg = ""
		for (i = 1; i < n; i++) {
			if (pkg != "") pkg = pkg "/"
			pkg = pkg segs[i]
		}
		pct = $NF
		gsub(/%/, "", pct)
		sum[pkg] += pct
		count[pkg]++
	}
	END {
		first = 1
		printf "{"
		for (p in sum) {
			if (!first) printf ","
			printf "\"%s\":%.1f", p, sum[p]/count[p]
			first = 0
		}
		printf "}"
	}')

jq -n \
	--arg total "$TOTAL" \
	--arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
	--arg commit "${GITHUB_SHA:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}" \
	--arg ref "${GITHUB_REF:-$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)}" \
	--argjson pkgs "$PACKAGES_JSON" \
	'{total_pct: ($total | tonumber), timestamp: $ts, commit: $commit, ref: $ref, packages: $pkgs}' >"$OUTPUT"

# Generate HTML report
HTML_OUTPUT="${OUTPUT%.json}.html"
go tool cover -html="$COVER_FILE" -o "$HTML_OUTPUT"

echo "Coverage: ${TOTAL}%"
echo "Written to $OUTPUT"
echo "HTML report: $HTML_OUTPUT"
