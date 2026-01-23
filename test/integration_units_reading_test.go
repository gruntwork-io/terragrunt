//go:build sops

// sops tests assume that you're going to import the test_pgp_key.asc file into your GPG keyring before
// running the tests. We're not gonna assume that everyone is going to do this, so we're going to skip
// these tests by default.
//
// You can import the key by running the following command:
//
//	gpg --import --no-tty --batch --yes ./test/fixtures/sops/test_pgp_key.asc

package test_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureUnitsReading = "fixtures/units-reading/"
)

func TestSOPSUnitsReading(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureUnitsReading)

	testCases := []struct {
		name           string
		unitsReading   []string
		unitsExcluding []string
		unitsIncluding []string
		expectedUnits  []string
	}{
		{
			name:         "empty",
			unitsReading: []string{},
			expectedUnits: []string{
				"indirect",
				"reading-from-tf",
				"reading-hcl",
				"reading-hcl-and-tfvars",
				"reading-json",
				"reading-sops",
				"reading-tfvars",
			},
		},
		{
			name: "reading_hcl",
			unitsReading: []string{
				"shared.hcl",
			},
			expectedUnits: []string{
				"reading-hcl",
				"reading-hcl-and-tfvars",
			},
		},
		{
			name: "reading_tfvars",
			unitsReading: []string{
				"shared.tfvars",
			},
			expectedUnits: []string{
				"reading-tfvars",
				"reading-hcl-and-tfvars",
			},
		},
		{
			name: "reading_json",
			unitsReading: []string{
				"shared.json",
			},
			expectedUnits: []string{
				"reading-from-tf",
				"reading-json",
			},
		},
		{
			name: "reading_sops",
			unitsReading: []string{
				"secrets.txt",
			},
			expectedUnits: []string{
				"reading-sops",
			},
		},
		{
			name: "reading_from_hcl_with_exclude",
			unitsReading: []string{
				"shared.hcl",
			},
			unitsExcluding: []string{
				"reading-hcl-and-tfvars",
			},
			expectedUnits: []string{
				"reading-hcl",
			},
		},
		{
			name: "reading_from_hcl_with_include",
			unitsReading: []string{
				"shared.hcl",
			},
			unitsIncluding: []string{
				"reading-tfvars",
			},
			expectedUnits: []string{
				"reading-hcl",
				"reading-hcl-and-tfvars",
				"reading-tfvars",
			},
		},
		{
			name: "reading_from_hcl_with_include_and_exclude",
			unitsReading: []string{
				"shared.hcl",
				"shared.tfvars",
			},
			unitsIncluding: []string{
				"reading-tfvars",
			},
			unitsExcluding: []string{
				"reading-hcl-and-tfvars",
			},
			expectedUnits: []string{
				"reading-hcl",
				"reading-tfvars",
			},
		},
		{
			name: "indirect",
			unitsReading: []string{
				filepath.Join("indirect", "src", "test.txt"),
			},
			expectedUnits: []string{
				"indirect",
			},
		},
	}

	includedLogEntryRegex := regexp.MustCompile(`=> Unit ([^ ]+) \(excluded: false`)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureUnitsReading)
			rootPath := filepath.Join(tmpEnvPath, testFixtureUnitsReading)
			rootPath, err := filepath.EvalSymlinks(rootPath)
			require.NoError(t, err)

			cmd := "terragrunt run --all plan --non-interactive --working-dir " + rootPath

			for _, f := range tc.unitsReading {
				cmd = cmd + " --queue-include-units-reading " + f
			}

			for _, unit := range tc.unitsIncluding {
				cmd = cmd + " --queue-include-dir " + unit
			}

			for _, unit := range tc.unitsExcluding {
				cmd = cmd + " --queue-exclude-dir " + unit
			}

			_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)

			includedUnits := []string{}

			for _, line := range strings.Split(stderr, "\n") {
				if includedLogEntryRegex.MatchString(line) {
					includedUnits = append(includedUnits, includedLogEntryRegex.FindStringSubmatch(line)[1])
				}
			}

			assert.ElementsMatch(t, tc.expectedUnits, includedUnits)
		})
	}
}

func TestUnitsReadingWithFilter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		unitsReading   []string
		unitsExcluding []string
		unitsIncluding []string
		expectedUnits  []string
	}{
		{
			name:         "empty",
			unitsReading: []string{},
			expectedUnits: []string{
				"indirect",
				"reading-from-tf",
				"reading-hcl",
				"reading-hcl-and-tfvars",
				"reading-json",
				"reading-sops",
				"reading-tfvars",
			},
		},
		{
			name: "reading_hcl",
			unitsReading: []string{
				"shared.hcl",
			},
			expectedUnits: []string{
				"reading-hcl",
				"reading-hcl-and-tfvars",
			},
		},
		{
			name: "reading_tfvars",
			unitsReading: []string{
				"shared.tfvars",
			},
			expectedUnits: []string{
				"reading-tfvars",
				"reading-hcl-and-tfvars",
			},
		},
		{
			name: "reading_json",
			unitsReading: []string{
				"shared.json",
			},
			expectedUnits: []string{
				"reading-from-tf",
				"reading-json",
			},
		},
		{
			name: "reading_sops",
			unitsReading: []string{
				"secrets.txt",
			},
			expectedUnits: []string{
				"reading-sops",
			},
		},
		{
			name: "reading_from_hcl_with_exclude",
			unitsReading: []string{
				"shared.hcl",
			},
			unitsExcluding: []string{
				"reading-hcl-and-tfvars",
			},
			expectedUnits: []string{
				"reading-hcl",
			},
		},
		{
			name: "reading_from_hcl_with_include",
			unitsReading: []string{
				"shared.hcl",
			},
			unitsIncluding: []string{
				"reading-tfvars",
			},
			expectedUnits: []string{
				"reading-hcl",
				"reading-hcl-and-tfvars",
				"reading-tfvars",
			},
		},
		{
			name: "reading_from_hcl_with_include_and_exclude",
			unitsReading: []string{
				"shared.hcl",
				"shared.tfvars",
			},
			unitsIncluding: []string{
				"reading-tfvars",
			},
			unitsExcluding: []string{
				"reading-hcl-and-tfvars",
			},
			expectedUnits: []string{
				"reading-hcl",
				"reading-tfvars",
			},
		},
		{
			name: "indirect",
			unitsReading: []string{
				filepath.Join("indirect", "src", "test.txt"),
			},
			expectedUnits: []string{
				"indirect",
			},
		},
	}

	includedLogEntryRegex := regexp.MustCompile(`=> Unit ([^ ]+) \(excluded: false`)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureUnitsReading)
			rootPath := filepath.Join(tmpEnvPath, testFixtureUnitsReading)
			rootPath, err := filepath.EvalSymlinks(rootPath)
			require.NoError(t, err)

			cmd := "terragrunt run --all plan --non-interactive --working-dir " + rootPath

			for _, f := range tc.unitsReading {
				cmd = cmd + " --filter reading=" + filepath.Join(rootPath, f)
			}

			for _, unit := range tc.unitsIncluding {
				cmd = cmd + " --filter " + filepath.Join(rootPath, unit)
			}

			for _, unit := range tc.unitsExcluding {
				cmd = cmd + " --filter !" + filepath.Join(rootPath, unit)
			}

			_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)

			includedUnits := []string{}

			for _, line := range strings.Split(stderr, "\n") {
				if includedLogEntryRegex.MatchString(line) {
					includedUnits = append(includedUnits, includedLogEntryRegex.FindStringSubmatch(line)[1])
				}
			}

			assert.ElementsMatch(t, tc.expectedUnits, includedUnits)
		})
	}
}

// TestQueueStrictIncludeWithUnitsReading tests that --queue-include-units-reading works correctly
// with --queue-include-units-reading when no --queue-include-dir is specified.
// This reproduces the bug where units reading the specified file were not included.
func TestQueueStrictIncludeWithUnitsReading(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureUnitsReading)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureUnitsReading)
	rootPath := filepath.Join(tmpEnvPath, testFixtureUnitsReading)
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	// Test the bug scenario: --queue-include-units-reading
	// without --queue-include-dir. Units reading shared.hcl should be included.
	cmd := "terragrunt run --all plan --non-interactive --working-dir " + rootPath +
		" --queue-include-units-reading shared.hcl"

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err, "Command should succeed and include units reading shared.hcl")

	includedLogEntryRegex := regexp.MustCompile(`=> Unit ([^ ]+) \(excluded: false`)
	includedUnits := []string{}

	for _, line := range strings.Split(stderr, "\n") {
		if includedLogEntryRegex.MatchString(line) {
			includedUnits = append(includedUnits, includedLogEntryRegex.FindStringSubmatch(line)[1])
		}
	}

	// Units that read shared.hcl should be included
	expectedUnits := []string{
		"reading-hcl",
		"reading-hcl-and-tfvars",
	}
	assert.ElementsMatch(t, expectedUnits, includedUnits,
		"Units reading shared.hcl should be included when using --queue-include-units-reading")
}
