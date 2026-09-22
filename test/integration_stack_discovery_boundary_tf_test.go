//go:build tf

package test_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTFStackRunDiscoveryBoundary pins the #6988 command and the units each diff plans.
func TestTFStackRunDiscoveryBoundary(t *testing.T) {
	t.Parallel()

	gitDiscoveryBoundary := " --experiment " + experiment.GitDiscoveryBoundary

	testCases := []struct {
		errAs    any
		name     string
		changed  string
		workDir  string
		args     string
		errText  string
		expected []string
	}{
		{
			name: "without changes",
			args: boundaryStackBoundedFilter,
		},
		{
			name:    "read file change without the experiment",
			changed: boundaryStackRolesFile,
			args:    boundaryStackBoundedFilter,
			errText: "roles.yml",
		},
		{
			name:    "catalog unit change without the experiment",
			changed: boundaryStackCatalogUnit,
			args:    boundaryStackBoundedFilter,
			errAs:   &discovery.DiscoveryBoundaryScopeError{},
		},
		{
			name:     "read file change with the experiment",
			changed:  boundaryStackRolesFile,
			args:     boundaryStackBoundedFilter + gitDiscoveryBoundary,
			expected: []string{boundaryStackRolesUnitDir},
		},
		{
			name:    "live stack file change without generated unit changes with the experiment",
			changed: boundaryStackLiveStackFile,
			args:    boundaryStackBoundedFilter + gitDiscoveryBoundary,
		},
		{
			name:    "catalog unit change with the experiment",
			changed: boundaryStackCatalogUnit,
			args:    boundaryStackBoundedFilter + gitDiscoveryBoundary,
		},
		{
			name:     "flag boundary with the experiment",
			changed:  boundaryStackRolesFile,
			args:     "--discovery-boundary ./live --filter '...[main...HEAD]'" + gitDiscoveryBoundary,
			expected: []string{boundaryStackRolesUnitDir},
		},
		{
			name:     "parent boundary from a subdirectory with the experiment",
			changed:  boundaryStackRolesFile,
			workDir:  "live/accounts",
			args:     "--filter '(..)...[main...HEAD]'" + gitDiscoveryBoundary,
			expected: []string{boundaryStackRolesUnitDir},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir, runner := setupBoundaryStackRepo(t)

			if tc.changed != "" {
				appendBoundaryStackChange(t, runner, filepath.Join(tmpDir, tc.changed))
			}

			stdout, stderr, err := runBoundaryStackCommand(
				t,
				filepath.Join(tmpDir, filepath.FromSlash(tc.workDir)),
				"stack run plan",
				tc.args,
			)

			switch {
			case tc.errAs != nil:
				require.ErrorAs(t, err, tc.errAs)
			case tc.errText != "":
				require.ErrorContains(t, err, tc.errText)
			default:
				require.NoError(t, err, "stderr: %s", stderr)

				for _, unit := range tc.expected {
					assert.Contains(t, stdout+stderr, "prefix="+unit+" tf-path=")
				}

				assert.NotContains(t, stdout+stderr, "catalog/units")
			}
		})
	}
}
