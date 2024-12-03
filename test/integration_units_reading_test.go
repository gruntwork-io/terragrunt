package test_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureUnitsReading = "fixtures/units-reading/"
)

func TestUnitsReading(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureUnitsReading)

	tc := []struct {
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
	}

	includedLogEntryRegex := regexp.MustCompile(`=> Module ./([^ ]+) \(excluded: false`)

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureUnitsReading)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureUnitsReading)

			cmd := "terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir " + rootPath

			for _, unit := range tt.unitsReading {
				cmd = cmd + " --terragrunt-queue-include-units-reading " + unit
			}

			for _, unit := range tt.unitsIncluding {
				cmd = cmd + " --terragrunt-include-dir " + unit
			}

			for _, unit := range tt.unitsExcluding {
				cmd = cmd + " --terragrunt-exclude-dir " + unit
			}

			_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)

			includedUnits := []string{}
			for _, line := range strings.Split(stderr, "\n") {
				if includedLogEntryRegex.MatchString(line) {
					includedUnits = append(includedUnits, includedLogEntryRegex.FindStringSubmatch(line)[1])
				}
			}

			assert.ElementsMatch(t, tt.expectedUnits, includedUnits)
		})
	}
}

func TestUnitsReadingRaceCondition(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureUnitsReading)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureUnitsReading)

	expectedUnits := []string{"reading-hcl", "reading-hcl-and-tfvars"}

	logger := createLogger()

	// Create synthetic units to increase the likelihood of a race condition
	for i := 0; i < 100; i++ {
		iAsString := strconv.Itoa(i)

		newDirName := "reading-hcl-" + iAsString
		newDir := util.JoinPath(rootPath, newDirName)
		require.NoError(t, util.CopyFolderContents(logger, util.JoinPath(testFixtureUnitsReading, "reading-hcl"), newDir, ".terragrunt-test", []string{}))
		expectedUnits = append(expectedUnits, newDirName)

		newDirName = "reading-hcl-and-tfvars-" + iAsString
		newDir = util.JoinPath(rootPath, newDirName)
		require.NoError(t, util.CopyFolderContents(logger, util.JoinPath(testFixtureUnitsReading, "reading-hcl-and-tfvars"), newDir, ".terragrunt-test", []string{}))
		expectedUnits = append(expectedUnits, newDirName)
	}

	cmd := "terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir " + rootPath + " --terragrunt-queue-include-units-reading shared.hcl"

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	includedLogEntryRegex := regexp.MustCompile(`=> Module ./([^ ]+) \(excluded: false`)

	includedUnits := []string{}
	for _, line := range strings.Split(stderr, "\n") {
		if includedLogEntryRegex.MatchString(line) {
			includedUnits = append(includedUnits, includedLogEntryRegex.FindStringSubmatch(line)[1])
		}
	}

	assert.ElementsMatch(t, expectedUnits, includedUnits)
}
