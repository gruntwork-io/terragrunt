#!/usr/bin/env bash

# Compares two timing-summary.json files (output of extract-timing.sh) and emits a
# comparison JSON used by the weekly report.
#
# Output schema:
# {
#   "current_total_sec": 1234.5,
#   "previous_total_sec": 1100.0,
#   "total_delta_sec": 134.5,
#   "slow_packages": [
#     {"package": "...", "current_sec": 87.4, "previous_sec": 70.1, "delta_sec": 17.3,
#      "top_tests": [{"name":"TestX","current_sec":4.2,"previous_sec":3.1,"delta_sec":1.1}, ...]}
#   ],
#   "top_regressions": [
#     {"package":"...", "current_sec":..., "previous_sec":..., "delta_sec":...}
#   ]
# }

set -euo pipefail

CURRENT="${1:?Usage: compare-timing.sh <current.json> <previous.json> [output.json]}"
PREVIOUS="${2:?Usage: compare-timing.sh <current.json> <previous.json> [output.json]}"
OUTPUT="${3:-timing-comparison.json}"

if [[ ! -f "$CURRENT" ]]; then
	echo "Error: current timing file '$CURRENT' not found" >&2
	exit 1
fi

if [[ ! -f "$PREVIOUS" ]]; then
	echo "No previous timing data found - emitting current-only report."
	jq '{
		baseline: true,
		current_total_sec: .total_sec,
		previous_total_sec: null,
		total_delta_sec: null,
		slow_packages: (
			.packages | to_entries | sort_by(-.value.wall_sec) | .[:5] | map({
				package: .key,
				current_sec: .value.wall_sec,
				previous_sec: null,
				delta_sec: null,
				top_tests: (
					.value.tests | to_entries | sort_by(-.value) | .[:5] | map({
						name: .key,
						current_sec: .value,
						previous_sec: null,
						delta_sec: null
					})
				)
			})
		),
		top_regressions: []
	}' "$CURRENT" >"$OUTPUT"
	exit 0
fi

jq -n \
	--slurpfile curr "$CURRENT" \
	--slurpfile prev "$PREVIOUS" \
	'
	($curr[0]) as $c |
	($prev[0]) as $p |
	($c.packages) as $cp |
	($p.packages) as $pp |

	# Build per-package comparison
	([$cp | keys[], ($pp | keys[])] | unique) as $all_pkgs |

	($all_pkgs | map(
		. as $pkg |
		($cp[$pkg].wall_sec // null) as $cs |
		($pp[$pkg].wall_sec // null) as $ps |
		{
			package: $pkg,
			current_sec: $cs,
			previous_sec: $ps,
			delta_sec: (
				if ($cs != null) and ($ps != null)
				then (($cs - $ps) * 10 | round / 10)
				else null end
			),
			current_tests: ($cp[$pkg].tests // {}),
			previous_tests: ($pp[$pkg].tests // {})
		}
	)) as $pkg_diff |

	# Top 5 slowest packages by current wall time
	([$pkg_diff[] | select(.current_sec != null)] | sort_by(-.current_sec) | .[:5] | map(
		. as $row |
		{
			package: $row.package,
			current_sec: $row.current_sec,
			previous_sec: $row.previous_sec,
			delta_sec: $row.delta_sec,
			top_tests: (
				$row.current_tests | to_entries | sort_by(-.value) | .[:5] | map(
					. as $t |
					{
						name: $t.key,
						current_sec: $t.value,
						previous_sec: ($row.previous_tests[$t.key] // null),
						delta_sec: (
							if ($row.previous_tests[$t.key] // null) != null
							then (($t.value - $row.previous_tests[$t.key]) * 100 | round / 100)
							else null end
						)
					}
				)
			)
		}
	)) as $slow_packages |

	# Top 5 packages by largest positive wall-time regression
	([$pkg_diff[] | select(.delta_sec != null and .delta_sec > 0)]
	 | sort_by(-.delta_sec) | .[:5] | map({
		package: .package,
		current_sec: .current_sec,
		previous_sec: .previous_sec,
		delta_sec: .delta_sec
	})) as $top_regressions |

	{
		baseline: false,
		current_total_sec: ($c.total_sec // 0),
		previous_total_sec: ($p.total_sec // 0),
		total_delta_sec: ((($c.total_sec // 0) - ($p.total_sec // 0)) * 10 | round / 10),
		slow_packages: $slow_packages,
		top_regressions: $top_regressions
	}
	' >"$OUTPUT"

# Console summary
echo "=== Timing Comparison ==="
printf "Total: %.1fs (was %.1fs, delta: %+.1fs)\n" \
	"$(jq -r '.current_total_sec' "$OUTPUT")" \
	"$(jq -r '.previous_total_sec' "$OUTPUT")" \
	"$(jq -r '.total_delta_sec' "$OUTPUT")"
echo ""
echo "Top 5 slowest packages:"
jq -r '.slow_packages[] | "  \(.current_sec)s (\(.delta_sec // "n/a")s)\t\(.package)"' "$OUTPUT"
echo ""
if [[ $(jq '.top_regressions | length' "$OUTPUT") -gt 0 ]]; then
	echo "Top 5 wall-time regressions:"
	jq -r '.top_regressions[] | "  +\(.delta_sec)s (was \(.previous_sec)s)\t\(.package)"' "$OUTPUT"
fi
echo ""
echo "Written to $OUTPUT"
