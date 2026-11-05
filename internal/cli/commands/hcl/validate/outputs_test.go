package validate_test

import (
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/config"
)

// outputsContractFiles is a repository whose vpc unit declares its outputs and whose app unit reads them.
var outputsContractFiles = map[string]string{
	"vpc/main.tf": `
output "vpc_id" {
  value = "vpc"
}

output "subnet_ids" {
  value = []
}
`,
	"vpc/terragrunt.hcl": `
outputs {
  output "vpc_id" {
    mock_value = "vpc-mock"
  }

  output "subnet_ids" {
    mock_value = []
  }
}
`,
	"app/main.tf": `variable "vpc_id" {}`,
	"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
`,
}

// generatedOutputsConfig declares two outputs, one of which a generate block adds to the module.
const generatedOutputsConfig = `
generate "subnets" {
  path      = "subnets.tf"
  if_exists = "overwrite"
  contents  = <<EOF
output "subnet_ids" {
  value = []
}
EOF
}

outputs {
  output "vpc_id" {
    mock_value = "vpc-mock"
  }

  output "subnet_ids" {
    mock_value = []
  }
}
`

// vpcModuleWithoutSubnets is a vpc module that declares only vpc_id.
const vpcModuleWithoutSubnets = `
output "vpc_id" {
  value = "vpc"
}
`

// expandedContractFiles is a repository whose a and b units each declare an output of their own, for an app unit
// that reads them through one expanded dependency.
var expandedContractFiles = map[string]string{
	"a/main.tf": `
output "a_id" {
  value = "a"
}
`,
	"a/terragrunt.hcl": `
outputs {
  output "a_id" {
    mock_value = "a-mock"
  }
}
`,
	"b/main.tf": `
output "b_id" {
  value = "b"
}
`,
	"b/terragrunt.hcl": `
outputs {
  output "b_id" {
    mock_value = "b-mock"
  }
}
`,
	"app/main.tf": `variable "id" {}`,
}

func TestRunValidateOutputsContracts(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		overrides       map[string]string
		name            string
		expectedSummary string
		expectedFile    string
		expectedLine    int
	}{
		{
			name: "contracts hold",
		},
		{
			name: "a declared output missing from the module is reported at its declaration",
			overrides: map[string]string{
				"vpc/terragrunt.hcl": `
outputs {
  output "vpc_id" {
    mock_value = "vpc-mock"
  }

  output "nat_id" {
    mock_value = "nat-mock"
  }
}
`,
			},
			expectedSummary: "Declared output not in module",
			expectedFile:    "vpc/terragrunt.hcl",
			expectedLine:    7,
		},
		{
			name: "a reference to an undeclared output is reported at the reference",
			overrides: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
  nat_id = dependency.vpc.outputs.nat_id
}
`,
			},
			expectedSummary: "Undeclared dependency output",
			expectedFile:    "app/terragrunt.hcl",
			expectedLine:    8,
		},
		{
			name: "a generated output satisfies its declaration",
			overrides: map[string]string{
				"vpc/main.tf":        vpcModuleWithoutSubnets,
				"vpc/terragrunt.hcl": generatedOutputsConfig,
			},
		},
		{
			name: "a stale generated file is rewritten before the check",
			overrides: map[string]string{
				"vpc/main.tf": vpcModuleWithoutSubnets,
				"vpc/subnets.tf": `
output "stale" {
  value = ""
}
`,
				"vpc/terragrunt.hcl": generatedOutputsConfig,
			},
		},
		{
			name: "a dependency without an outputs block is not checked",
			overrides: map[string]string{
				"vpc/terragrunt.hcl": ``,
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    nat_id = "nat-mock"
  }
}

inputs = {
  nat_id = dependency.vpc.outputs.nat_id
}
`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			files := make(map[string]string, len(outputsContractFiles))
			maps.Copy(files, outputsContractFiles)
			maps.Copy(files, tc.overrides)

			result := runValidate(t, validateRun{files: files, experiment: experiment.OutputsBlock})

			if tc.expectedSummary == "" {
				require.NoError(t, result.err)
				assert.Empty(t, result.diags)

				return
			}

			require.Error(t, result.err)

			// Evaluating a reference against the mock object raises HCL's own "Unsupported attribute" as well, so
			// the contract diagnostic is found by its summary rather than being the only one.
			summaries := make([]string, 0, len(result.diags))
			found := false

			for _, diag := range result.diags {
				summaries = append(summaries, diag.Summary)

				if diag.Summary != tc.expectedSummary {
					continue
				}

				found = true

				require.NotNil(t, diag.Range)
				assert.Equal(t, filepath.Join(result.root, filepath.FromSlash(tc.expectedFile)), diag.Range.Filename)
				assert.Equal(t, tc.expectedLine, diag.Range.Start.Line)
			}

			assert.True(t, found, "no %q diagnostic among: %s", tc.expectedSummary, strings.Join(summaries, " | "))
		})
	}
}

// TestRunValidateOutputsContractsExpandedDependency pins that a reference to one instance of an expanded
// dependency is checked against the outputs that instance declares.
func TestRunValidateOutputsContractsExpandedDependency(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		appConfig       string
		expectedSummary string
		expectedLine    int
	}{
		{
			name: "a reference through an each key checks that instance",
			appConfig: `
dependency "svc" {
  expansion {
    for_each = toset(["a", "b"])
  }

  config_path = "../${each.key}"
}

inputs = {
  id = dependency.svc["b"].outputs.b_id
}
`,
		},
		{
			name: "a reference through a count index checks that instance",
			appConfig: `
dependency "svc" {
  expansion {
    count = 2
  }

  config_path = "../${["a", "b"][count.index]}"
}

inputs = {
  id = dependency.svc[1].outputs.b_id
}
`,
		},
		{
			name: "an output another instance declares is reported",
			appConfig: `
dependency "svc" {
  expansion {
    for_each = toset(["a", "b"])
  }

  config_path = "../${each.key}"
}

inputs = {
  id = dependency.svc["a"].outputs.b_id
}
`,
			expectedSummary: "Undeclared dependency output",
			expectedLine:    11,
		},
		{
			name: "a reference through a dynamic key is not checked",
			appConfig: `
locals {
  name = "b"
}

dependency "svc" {
  expansion {
    for_each = toset(["a", "b"])
  }

  config_path = "../${each.key}"
}

inputs = {
  id = dependency.svc[local.name].outputs.b_id
}
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			files := make(map[string]string, len(expandedContractFiles)+1)
			maps.Copy(files, expandedContractFiles)
			files["app/terragrunt.hcl"] = tc.appConfig

			result := runValidate(t, validateRun{files: files, experiment: experiment.OutputsBlock})

			if tc.expectedSummary == "" {
				require.NoError(t, result.err)
				assert.Empty(t, result.diags)

				return
			}

			require.Error(t, result.err)

			found := false

			for _, diag := range result.diags {
				if diag.Summary != tc.expectedSummary {
					continue
				}

				found = true

				require.NotNil(t, diag.Range)
				assert.Equal(t, filepath.Join(result.root, "app", config.DefaultTerragruntConfigPath), diag.Range.Filename)
				assert.Equal(t, tc.expectedLine, diag.Range.Start.Line)
				assert.Contains(t, diag.Detail, `dependency "svc[a]"`)
			}

			assert.True(t, found, "no %q diagnostic", tc.expectedSummary)
		})
	}
}

func TestRunValidateOutputsBlockRequiresExperiment(t *testing.T) {
	t.Parallel()

	result := runValidate(t, validateRun{files: outputsContractFiles})

	var experimentErr config.OutputsBlockRequiresExperimentError

	require.ErrorAs(t, result.err, &experimentErr)
	assert.Equal(t, filepath.Join(result.root, "vpc", config.DefaultTerragruntConfigPath), experimentErr.ConfigPath)
}
