// Package analyzer provides failure analysis and reporting functionality.
package analyzer

import (
	"sort"

	"github.com/gruntwork-io/terragrunt/test/flake/types"
)

// GroupFailuresByTest groups failures by test name and calculates statistics.
func GroupFailuresByTest(failures []types.TestFailure, totalRuns int) []types.TestStats {
	byTest := make(map[string]*types.TestStats)

	for _, f := range failures {
		key := f.TestName
		if stats, exists := byTest[key]; exists {
			stats.TotalFailures++
			stats.Failures = append(stats.Failures, f)
			if f.FailedAt.After(stats.LastSeen) {
				stats.LastSeen = f.FailedAt
			}
			if f.FailedAt.Before(stats.FirstSeen) {
				stats.FirstSeen = f.FailedAt
			}
		} else {
			byTest[key] = &types.TestStats{
				TestName:      f.TestName,
				Package:       f.Package,
				TotalFailures: 1,
				Failures:      []types.TestFailure{f},
				FirstSeen:     f.FailedAt,
				LastSeen:      f.FailedAt,
			}
		}
	}

	// Calculate rates and convert to slice
	var stats []types.TestStats
	for _, s := range byTest {
		if totalRuns > 0 {
			s.FailureRate = float64(s.TotalFailures) / float64(totalRuns)
		}
		stats = append(stats, *s)
	}

	// Sort by failure count (descending)
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].TotalFailures > stats[j].TotalFailures
	})

	return stats
}

// GroupFailuresByJob groups failures by job name.
func GroupFailuresByJob(failures []types.TestFailure) map[string][]types.TestFailure {
	byJob := make(map[string][]types.TestFailure)

	for _, f := range failures {
		byJob[f.JobName] = append(byJob[f.JobName], f)
	}

	return byJob
}

// FilterByMinFailures filters test stats to only include tests with at least minFailures.
func FilterByMinFailures(stats []types.TestStats, minFailures int) []types.TestStats {
	var filtered []types.TestStats
	for _, s := range stats {
		if s.TotalFailures >= minFailures {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// BuildReport creates an analysis report from failures and run data.
func BuildReport(failures []types.TestFailure, totalRuns, failedRuns int) types.AnalysisReport {
	stats := GroupFailuresByTest(failures, totalRuns)

	return types.AnalysisReport{
		TotalRuns:     totalRuns,
		FailedRuns:    failedRuns,
		TotalFailures: len(failures),
		UniqueTests:   len(stats),
		TestStats:     stats,
	}
}
