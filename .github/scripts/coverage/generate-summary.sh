#!/usr/bin/env bash

set -euo pipefail

COVER_FILE="${1:-coverage.out}"
OUTPUT="${2:-coverage-summary.json}"

if [[ ! -f "$COVER_FILE" ]]; then
	echo "Error: coverage file '$COVER_FILE' not found" >&2
	exit 1
fi

TOTAL=$(go tool cover -func="$COVER_FILE" | grep '^total:' | awk '{print $NF}' | tr -d '%')

# Build per-package JSON: average coverage per package (go tool cover reports per-function)
# awk outputs TSV lines, jq handles JSON escaping safely
PACKAGES_JSON=$(go tool cover -func="$COVER_FILE" |
	grep -v '^total:' |
	awk -F'\t+' '{
		split($1, parts, ":")
		file = parts[1]
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
		for (p in sum) printf "%s\t%.1f\n", p, sum[p]/count[p]
	}' | jq -Rs 'split("\n") | map(select(length > 0) | split("\t") | {(.[0]): (.[1] | tonumber)}) | add // {}')

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

echo "=== Coverage: ${TOTAL}% ==="
echo ""
jq -r '.packages | to_entries | sort_by(.value) | .[] | "\(.value)%\t\(.key)"' "$OUTPUT" | column -t -s $'\t'
echo ""
echo "Written to $OUTPUT"
echo "HTML report: $HTML_OUTPUT"
