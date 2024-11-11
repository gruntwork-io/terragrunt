package test_test

import (
	"testing"

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

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := copyEnvironment(t, testFixtureUnitsReading)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureUnitsReading)

			cmd := "terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir " + rootPath

			for _, unit := range tt.unitsReading {
				cmd = cmd + " --terragrunt-queue-include-units-reading " + unit
			}

			for _, unit := range tt.unitsIncluding {
				cmd = cmd + " --terragrunt-include-dir " + unit
			}

			for _, unit := range tt.unitsExcluding {
				cmd = cmd + " --terragrunt-exclude-dir " + unit
			}

			_, stderr, err := runTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)

			for _, unit := range tt.expectedUnits {
				assert.Contains(t, stderr, unit+" (excluded: false")
			}
		})
	}
}
