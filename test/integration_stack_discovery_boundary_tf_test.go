//go:build tf

package test_test

import (
	"path/filepath"
	"regexp"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// boundaryStackPlannedUnit matches working tree unit prefixes on tofu output, skipping worktree dependency reads.
var boundaryStackPlannedUnit = regexp.MustCompile(`prefix=([^.\s]\S*) tf-path=`)

// TestTFStackRunDiscoveryBoundary checks the exact set of units planned for each bounded diff.
func TestTFStackRunDiscoveryBoundary(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		errAs    any
		name     string
		changed  string
		removed  string
		workDir  string
		args     string
		expected []string
	}{
		{
			name: "without changes",
			args: boundaryStackBoundedFilter,
		},
		{
			name:     "changed read file",
			changed:  boundaryStackRolesFile,
			args:     boundaryStackBoundedFilter,
			expected: []string{boundaryStackRolesUnitDir},
		},
		{
			name:    "changed live stack file without generated unit changes",
			changed: boundaryStackLiveStackFile,
			args:    boundaryStackBoundedFilter,
		},
		{
			name:    "changed catalog unit outside the boundary",
			changed: boundaryStackCatalogUnit,
			args:    boundaryStackBoundedFilter,
		},
		{
			name:    "removed catalog unit outside the boundary",
			removed: boundaryStackUnusedUnit,
			args:    boundaryStackBoundedFilter,
		},
		{
			name:     "deleted read file inside the boundary",
			removed:  boundaryStackStandaloneRead,
			args:     boundaryStackBoundedFilter,
			expected: []string{boundaryStackStandaloneDir},
		},
		{
			name:     "flag boundary inside the working directory",
			changed:  boundaryStackRolesFile,
			args:     "--discovery-boundary ./live --filter '...[main...HEAD]'",
			expected: []string{boundaryStackRolesUnitDir},
		},
		{
			name:     "parent boundary from a subdirectory",
			changed:  boundaryStackRolesFile,
			workDir:  "live/accounts",
			args:     "--filter '(..)...[main...HEAD]'",
			expected: []string{boundaryStackRolesUnitDir},
		},
		{
			name:     "changed sibling unit under a parent boundary",
			changed:  boundaryStackStandaloneDir + "/terragrunt.hcl",
			workDir:  "live/accounts",
			args:     "--filter '(..)...[main...HEAD]'",
			expected: []string{boundaryStackStandaloneDir},
		},
		{
			name:    "changed unit outside disjoint boundaries",
			changed: boundaryStackOtherUnit,
			args:    "--filter '(./live/accounts)...[main...HEAD]' --filter '(./live/standalone)...[main...HEAD]'",
		},
		{
			name:     "changed unit inside one of disjoint boundaries",
			changed:  boundaryStackStandaloneDir + "/terragrunt.hcl",
			args:     "--filter '(./live/accounts)...[main...HEAD]' --filter '(./live/standalone)...[main...HEAD]'",
			expected: []string{boundaryStackStandaloneDir},
		},
		{
			name:    "flag boundary outside the working directory",
			changed: boundaryStackRolesFile,
			workDir: "live",
			args:    "--discovery-boundary ../catalog --filter '...[main...HEAD]'",
			errAs:   &discovery.DiscoveryBoundaryScopeError{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir, runner := setupBoundaryStackRepo(t)

			if tc.changed != "" {
				appendBoundaryStackChange(t, runner, filepath.Join(tmpDir, tc.changed))
			}

			if tc.removed != "" {
				removeBoundaryStackPath(t, runner, filepath.Join(tmpDir, tc.removed))
			}

			stdout, stderr, err := runBoundaryStackCommand(
				t,
				filepath.Join(tmpDir, filepath.FromSlash(tc.workDir)),
				"stack run plan",
				tc.args,
			)

			if tc.errAs != nil {
				require.ErrorAs(t, err, tc.errAs)
				return
			}

			require.NoError(t, err, "stderr: %s", stderr)
			assert.ElementsMatch(t, tc.expected, plannedUnits(stdout+stderr))
			assert.NotContains(t, stdout+stderr, "catalog/units")
		})
	}
}

// plannedUnits returns the distinct working tree units that printed tofu output.
func plannedUnits(output string) []string {
	seen := make(map[string]struct{})

	var units []string

	for _, match := range boundaryStackPlannedUnit.FindAllStringSubmatch(output, -1) {
		unit := filepath.ToSlash(match[1])
		if _, ok := seen[unit]; ok {
			continue
		}

		seen[unit] = struct{}{}
		units = append(units, unit)
	}

	return units
}
