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

# jq builds the aggregate in a single streaming pass, reading events from the file
# and writing straight to OUTPUT. Metadata is folded in via small --arg values so
# nothing large crosses the command line (a prior version passed the whole aggregate
# through --argjson and hit ARG_MAX on the full suite).
# Filtered to terminal events only: package row has Test absent/empty; test row has
# Test non-empty. Malformed (non-JSON) lines are skipped via fromjson?.
jq -n --raw-input \
	--arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
	--arg commit "${GITHUB_SHA:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}" \
	--arg ref "${GITHUB_REF:-$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)}" '
	reduce (
		inputs
		| select(length > 0)
		| fromjson?
		| select(.Action == "pass" or .Action == "fail" or .Action == "skip")
	) as $e (
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
	| . + {generated_at: $ts, commit: $commit, ref: $ref}
' "$EVENTS" >"$OUTPUT"

echo "=== Timing summary ==="
TOTAL=$(jq -r '.total_sec' "$OUTPUT")
PKGS=$(jq -r '.packages | length' "$OUTPUT")
printf "Total: %.1fs across %d packages\n" "$TOTAL" "$PKGS"
echo ""
echo "Top 5 slowest packages:"
jq -r '.packages | to_entries | sort_by(-.value.wall_sec) | .[:5][] | "  \(.value.wall_sec)s\t\(.key)"' "$OUTPUT"
echo ""
echo "Written to $OUTPUT"
