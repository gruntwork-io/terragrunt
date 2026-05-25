#!/usr/bin/env bash

# Rolls up `go test -json` events (ndjson) into a per-package / per-test wall-time JSON.
#
# Output schema:
# {
#   "generated_at": "2026-05-25T09:00:00Z",
#   "commit": "<sha>",
#   "ref": "<ref>",
#   "total_sec": 1234.5,
#   "packages": {
#     "github.com/.../config": {
#       "wall_sec": 87.4,
#       "tests": { "TestParseHCL": 0.43, "TestParseHCL/sub": 0.10 }
#     }
#   }
# }
#
# Package wall_sec is taken from the package-level "pass"/"fail"/"skip" event (Test field empty).
# Per-test seconds come from "pass"/"fail"/"skip" events with a Test field set; subtest names
# (e.g. TestX/sub) are kept verbatim so callers can drill down.

set -euo pipefail

EVENTS="${1:?Usage: extract-timing.sh <events.ndjson> <output.json>}"
OUTPUT="${2:?Usage: extract-timing.sh <events.ndjson> <output.json>}"

if [[ ! -f "$EVENTS" ]]; then
	echo "Error: events file '$EVENTS' not found" >&2
	exit 1
fi

# jq builds the aggregate in a single pass. Filtered to terminal events only.
# Package row: Test field absent/empty. Test row: Test field non-empty.
TIMING_JSON=$(jq -s --raw-input '
	split("\n")
	| map(select(length > 0) | fromjson)
	| map(select(.Action == "pass" or .Action == "fail" or .Action == "skip"))
	| reduce .[] as $e (
		{packages: {}};
		if ($e.Test // "") == "" then
			.packages[$e.Package] = (
				(.packages[$e.Package] // {wall_sec: 0, tests: {}})
				| .wall_sec = ($e.Elapsed // 0)
			)
		else
			.packages[$e.Package] = (
				(.packages[$e.Package] // {wall_sec: 0, tests: {}})
				| .tests[$e.Test] = ($e.Elapsed // 0)
			)
		end
	)
	| .total_sec = ([.packages[].wall_sec] | add // 0)
' "$EVENTS")

jq -n \
	--arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
	--arg commit "${GITHUB_SHA:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}" \
	--arg ref "${GITHUB_REF:-$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)}" \
	--argjson body "$TIMING_JSON" \
	'$body + {generated_at: $ts, commit: $commit, ref: $ref}' >"$OUTPUT"

echo "=== Timing summary ==="
TOTAL=$(jq -r '.total_sec' "$OUTPUT")
PKGS=$(jq -r '.packages | length' "$OUTPUT")
printf "Total: %.1fs across %d packages\n" "$TOTAL" "$PKGS"
echo ""
echo "Top 5 slowest packages:"
jq -r '.packages | to_entries | sort_by(-.value.wall_sec) | .[:5][] | "  \(.value.wall_sec)s\t\(.key)"' "$OUTPUT"
echo ""
echo "Written to $OUTPUT"
