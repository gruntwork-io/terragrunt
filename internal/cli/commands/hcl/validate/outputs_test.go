package validate_test

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"net/http"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/validate"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

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

const vpcModuleWithoutSubnets = `
output "vpc_id" {
  value = "vpc"
}
`

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
			name: "a for variable named dependency is not a reference",
			overrides: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
  ids    = [for dependency in [{ vpc = { outputs = { nat_id = "nat" } } }] : dependency.vpc.outputs.nat_id]
}
`,
			},
		},
		{
			name: "a reference through a string index on the label is reported",
			overrides: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    nat_id = "nat-mock"
  }
}

inputs = {
  nat_id = dependency["vpc"].outputs.nat_id
}
`,
			},
			expectedSummary: "Undeclared dependency output",
			expectedFile:    "app/terragrunt.hcl",
			expectedLine:    11,
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

			result := runValidate(t, &validateRun{files: files, experiments: []string{experiment.OutputsBlock}, outputs: true})

			if tc.expectedSummary == "" {
				require.NoError(t, result.err)
				assert.Empty(t, result.diags)

				return
			}

			require.Error(t, result.err)
			require.Len(t, result.diags, 1)

			diag := result.diags[0]

			assert.Equal(t, tc.expectedSummary, diag.Summary)
			require.NotNil(t, diag.Range)
			assert.Equal(t, filepath.Join(result.root, filepath.FromSlash(tc.expectedFile)), diag.Range.Filename)
			assert.Equal(t, tc.expectedLine, diag.Range.Start.Line)
		})
	}
}

// TestRunValidateOutputsContractsExpandedDependency checks a reference to one instance against that instance's outputs.
func TestRunValidateOutputsContractsExpandedDependency(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		appConfig       string
		expectedSummary string
		expectedAddress string
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
			name: "a reference through an attribute key checks that instance",
			appConfig: `
dependency "svc" {
  expansion {
    for_each = toset(["a", "b"])
  }

  config_path = "../${each.key}"
}

inputs = {
  id = dependency.svc.b.outputs.b_id
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
			expectedAddress: "svc[a]",
			expectedLine:    11,
		},
		{
			name: "an output another instance declares is reported through an attribute key",
			appConfig: `
dependency "svc" {
  expansion {
    for_each = toset(["a", "b"])
  }

  config_path = "../${each.key}"
}

inputs = {
  id = dependency.svc.a.outputs.b_id
}
`,
			expectedSummary: "Undeclared dependency output",
			expectedAddress: "svc[a]",
			expectedLine:    11,
		},
		{
			name: "an instance named outputs is read through the outputs marker",
			appConfig: `
dependency "svc" {
  expansion {
    for_each = { outputs = "a", b = "b" }
  }

  config_path = "../${each.value}"
}

inputs = {
  id = dependency.svc.outputs.outputs.a_id
}
`,
		},
		{
			name: "an output another instance declares is reported for an instance named outputs",
			appConfig: `
dependency "svc" {
  expansion {
    for_each = { outputs = "a", b = "b" }
  }

  config_path = "../${each.value}"
}

inputs = {
  id = dependency.svc.outputs.outputs.b_id
}
`,
			expectedSummary: "Undeclared dependency output",
			expectedAddress: "svc[outputs]",
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

			result := runValidate(t, &validateRun{files: files, experiments: []string{experiment.OutputsBlock}, outputs: true})

			if tc.expectedSummary == "" {
				require.NoError(t, result.err)
				assert.Empty(t, result.diags)

				return
			}

			require.Error(t, result.err)
			require.Len(t, result.diags, 1)

			diag := result.diags[0]

			assert.Equal(t, tc.expectedSummary, diag.Summary)
			require.NotNil(t, diag.Range)
			assert.Equal(t, filepath.Join(result.root, "app", config.DefaultTerragruntConfigPath), diag.Range.Filename)
			assert.Equal(t, tc.expectedLine, diag.Range.Start.Line)
			assert.Contains(t, diag.Detail, `dependency "`+tc.expectedAddress+`"`)
		})
	}
}

var overriddenIncludeFiles = map[string]string{
	"legacy/main.tf": `
output "legacy_id" {
  value = "legacy"
}
`,
	"legacy/terragrunt.hcl": `
outputs {
  output "legacy_id" {
    mock_value = "legacy-mock"
  }
}
`,
	"current/main.tf": `
output "id" {
  value = "current"
}
`,
	"current/terragrunt.hcl": `
outputs {
  output "id" {
    mock_value = "current-mock"
  }
}
`,
	"root.hcl": `
dependency "vpc" {
  config_path = "${get_terragrunt_dir()}/../legacy"
}

inputs = {
  id = dependency.vpc.outputs.legacy_id
}
`,
	"app/main.tf": `variable "id" {}`,
}

// TestRunValidateOutputsContractsIncludedReferences checks merged and exposed includes against the unit's dependencies.
func TestRunValidateOutputsContractsIncludedReferences(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files          map[string]string
		name           string
		expectedFile   string
		expectedDetail string
		expectedLine   int
	}{
		{
			name: "an include that is neither merged nor exposed is not checked",
			files: withFiles(overriddenIncludeFiles, map[string]string{
				"app/terragrunt.hcl": `
include "root" {
  path           = find_in_parent_folders("root.hcl")
  merge_strategy = "no_merge"
  expose         = false
}

dependency "vpc" {
  config_path = "../current"
}

inputs = {
  id = dependency.vpc.outputs.id
}
`,
			}),
		},
		{
			name: "a merged include is checked against the dependencies the unit overrides",
			files: withFiles(overriddenIncludeFiles, map[string]string{
				"app/terragrunt.hcl": `
include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "vpc" {
  config_path = "../current"
}

inputs = {
  id = dependency.vpc.outputs.id
}
`,
			}),
			expectedFile:   "root.hcl",
			expectedDetail: "Declared outputs: id.",
			expectedLine:   7,
		},
		{
			name: "an exposed include is checked against the dependencies the unit overrides",
			files: withFiles(overriddenIncludeFiles, map[string]string{
				"app/terragrunt.hcl": `
include "root" {
  path           = find_in_parent_folders("root.hcl")
  merge_strategy = "no_merge"
  expose         = true
}

dependency "vpc" {
  config_path = "../current"
}

inputs = {
  id = dependency.vpc.outputs.id
}
`,
			}),
			expectedFile:   "root.hcl",
			expectedDetail: "Declared outputs: id.",
			expectedLine:   7,
		},
		{
			name: "a reference in an included file",
			files: map[string]string{
				"root.hcl": `
dependency "vpc" {
  config_path = "${get_terragrunt_dir()}/../vpc"

  mock_outputs = {
    nat_id = "nat-mock"
  }
}

inputs = {
  nat_id = dependency.vpc.outputs.nat_id
}
`,
				"app/terragrunt.hcl": `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`,
			},
			expectedFile:   "root.hcl",
			expectedDetail: "Declared outputs: vpc_id, subnet_ids.",
			expectedLine:   11,
		},
		{
			name: "a reference in the autoinclude file",
			files: map[string]string{
				"app/terragrunt.hcl": ``,
				"app/terragrunt.autoinclude.hcl": `
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
			expectedFile:   filepath.Join("app", config.DefaultAutoIncludeFile),
			expectedDetail: "Declared outputs: vpc_id, subnet_ids.",
			expectedLine:   11,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			files := map[string]string{
				"vpc/main.tf":        outputsContractFiles["vpc/main.tf"],
				"vpc/terragrunt.hcl": outputsContractFiles["vpc/terragrunt.hcl"],
				"app/main.tf":        `variable "nat_id" {}`,
			}
			maps.Copy(files, tc.files)

			result := runValidate(t, &validateRun{files: files, experiments: []string{experiment.OutputsBlock}, outputs: true})

			if tc.expectedFile == "" {
				require.NoError(t, result.err)
				assert.Empty(t, result.diags)

				return
			}

			require.Error(t, result.err)

			found := false

			for _, diag := range result.diags {
				if diag.Summary != "Undeclared dependency output" {
					continue
				}

				found = true

				require.NotNil(t, diag.Range)
				assert.Equal(t, filepath.Join(result.root, tc.expectedFile), diag.Range.Filename)
				assert.Equal(t, tc.expectedLine, diag.Range.Start.Line)
				assert.Contains(t, diag.Detail, tc.expectedDetail)
			}

			assert.True(t, found, "no undeclared output diagnostic")
		})
	}
}

// TestRunValidateOutputsContractsReferencesInVariablesBlock checks a reference inside a variables block.
func TestRunValidateOutputsContractsReferencesInVariablesBlock(t *testing.T) {
	t.Parallel()

	files := withFiles(outputsContractFiles, map[string]string{
		"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

variables {
  inputs = {
    vpc_id = dependency.vpc.outputs.vpc_id
    nat_id = dependency.vpc.outputs.nat_id
  }
}
`,
	})

	result := runValidate(t, &validateRun{
		files:       files,
		experiments: []string{experiment.OutputsBlock, experiment.VariablesBlock},
		outputs:     true,
	})

	require.Error(t, result.err)

	found := 0

	for _, diag := range result.diags {
		if diag.Summary != "Undeclared dependency output" {
			continue
		}

		found++

		require.NotNil(t, diag.Range)
		assert.Equal(t, filepath.Join(result.root, "app", config.DefaultTerragruntConfigPath), diag.Range.Filename)
		assert.Equal(t, 9, diag.Range.Start.Line)
	}

	assert.Equal(t, 1, found, "one undeclared output diagnostic expected")
}

// TestRunValidateOutputsContractsLeavesUnitDirectory expects no generated file in the unit directory after the check.
func TestRunValidateOutputsContractsLeavesUnitDirectory(t *testing.T) {
	t.Parallel()

	files := withFiles(outputsContractFiles, map[string]string{
		"vpc/main.tf":        vpcModuleWithoutSubnets,
		"vpc/terragrunt.hcl": generatedOutputsConfig,
	})

	result := runValidate(t, &validateRun{files: files, experiments: []string{experiment.OutputsBlock}, outputs: true})

	require.NoError(t, result.err)
	assert.Empty(t, result.diags)
	assert.False(t, vfs.Exists(result.fs, filepath.Join(result.root, "vpc", "subnets.tf")))
}

// TestRunValidateOutputsContractsInputsMode expects --inputs to run the same checks and report them as errors.
func TestRunValidateOutputsContractsInputsMode(t *testing.T) {
	t.Parallel()

	files := withFiles(outputsContractFiles, map[string]string{
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
		"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    nope = "nope-mock"
  }
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
  nope   = dependency.vpc.outputs.nope
  nope2  = dependency.vpc.outputs.nope
}
`,
	})

	result := runValidate(t, &validateRun{
		files:       files,
		experiments: []string{experiment.OutputsBlock},
		inputs:      true,
		outputs:     true,
	})

	require.Error(t, result.err)

	var summaries []string

	for _, leaf := range joinedLeaves(result.err) {
		if diag, ok := errors.AsType[*hcl.Diagnostic](leaf); ok {
			summaries = append(summaries, diag.Summary)
		}
	}

	assert.ElementsMatch(t, []string{
		"Declared output not in module",
		"Undeclared dependency output",
		"Undeclared dependency output",
	}, summaries)
}

// TestRunValidateOutputsContractsSkipsBackendCheck passes a remote_state block without generate and no module backend.
func TestRunValidateOutputsContractsSkipsBackendCheck(t *testing.T) {
	t.Parallel()

	files := withFiles(outputsContractFiles, map[string]string{
		"vpc/terragrunt.hcl": `
remote_state {
  backend = "local"
  config = {
    path = "terraform.tfstate"
  }
}

outputs {
  output "vpc_id" {
    mock_value = "vpc-mock"
  }

  output "subnet_ids" {
    mock_value = []
  }
}
`,
	})

	result := runValidate(t, &validateRun{files: files, experiments: []string{experiment.OutputsBlock}, outputs: true})

	require.NoError(t, result.err)
	assert.Empty(t, result.diags)
}

// TestRunValidateOutputsContractsReportsModuleParseErrors expects a diagnostic against a module file failing to parse.
func TestRunValidateOutputsContractsReportsModuleParseErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		module          string
		expectedSummary string
		expectedLine    int
	}{
		{
			name: "a file that does not parse",
			module: `
output "vpc_id" {
  value = "vpc"
`,
			expectedSummary: "Unclosed configuration block",
			expectedLine:    2,
		},
		{
			name: "an output block without a name",
			module: `
output {
  value = "vpc"
}
`,
			expectedSummary: "Missing name for output",
			expectedLine:    2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			files := withFiles(outputsContractFiles, map[string]string{"vpc/main.tf": tc.module})

			result := runValidate(t, &validateRun{files: files, experiments: []string{experiment.OutputsBlock}, outputs: true})

			require.Error(t, result.err)
			assert.NotErrorAs(t, result.err, new(validate.OutputsContractError), "reported as a diagnostic")

			found := 0

			for _, diag := range result.diags {
				if diag.Summary != tc.expectedSummary {
					continue
				}

				found++

				require.NotNil(t, diag.Range)
				assert.Equal(t, filepath.Join(result.root, "vpc", "main.tf"), diag.Range.Filename)
				assert.Equal(t, tc.expectedLine, diag.Range.Start.Line)
			}

			assert.Equal(t, 1, found, "one %q diagnostic expected", tc.expectedSummary)
		})
	}
}

// TestRunValidateOutputsContractsGeneratesLikeARun expects if_exists = "error" on an existing absolute path to fail.
func TestRunValidateOutputsContractsGeneratesLikeARun(t *testing.T) {
	t.Parallel()

	files := withFiles(outputsContractFiles, map[string]string{
		"vpc/extra.hcl": `extra = true`,
		"vpc/terragrunt.hcl": `
generate "extra" {
  path      = "${get_terragrunt_dir()}/extra.hcl"
  if_exists = "error"
  contents  = ""
}

outputs {
  output "vpc_id" {
    mock_value = "vpc-mock"
  }
}
`,
	})

	result := runValidate(t, &validateRun{files: files, experiments: []string{experiment.OutputsBlock}, outputs: true})

	require.ErrorAs(t, result.err, new(validate.OutputsContractError))
}

// TestRunValidateOutputsContractsReadsLinkedModuleFiles counts a symlinked module file, on the OS filesystem.
func TestRunValidateOutputsContractsReadsLinkedModuleFiles(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("creating a symlink on Windows takes a privilege the runner may not have")
	}

	files := withFiles(outputsContractFiles, map[string]string{
		"shared/outputs.tf": outputsContractFiles["vpc/main.tf"],
	})
	delete(files, "vpc/main.tf")

	result := runValidate(t, &validateRun{
		files:       files,
		links:       map[string]string{"vpc/outputs.tf": "shared/outputs.tf"},
		experiments: []string{experiment.OutputsBlock},
		outputs:     true,
		onDisk:      true,
	})

	require.NoError(t, result.err)
	assert.Empty(t, result.diags)
}

// TestRunValidateOutputsContractsFetchesSource checks the declared outputs against the module the unit's source names.
func TestRunValidateOutputsContractsFetchesSource(t *testing.T) {
	t.Parallel()

	files := withFiles(outputsContractFiles, map[string]string{
		"vpc/terragrunt.hcl": `
terraform {
  source = "${get_terragrunt_dir()}/../modules/vpc"
}

outputs {
  output "vpc_id" {
    mock_value = "vpc-mock"
  }

  output "subnet_ids" {
    mock_value = []
  }
}
`,
		"modules/vpc/main.tf": outputsContractFiles["vpc/main.tf"],
	})
	delete(files, "vpc/main.tf")

	result := runValidate(t, &validateRun{files: files, experiments: []string{experiment.OutputsBlock}, outputs: true})

	require.NoError(t, result.err)
	assert.Empty(t, result.diags)
	assert.True(t, vfs.Exists(result.fs, filepath.Join(result.root, "vpc", ".terragrunt-cache")))
	assert.False(t, vfs.Exists(result.fs, filepath.Join(result.root, ".terragrunt-cache")))
}

// TestRunValidateOutputsContractsRunsNoHook expects the fetch of a source to run none of the hooks of the unit.
func TestRunValidateOutputsContractsRunsNoHook(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		hook    string
		command string
	}{
		{name: "an after hook of terragrunt-read-config", hook: "after_hook", command: "terragrunt-read-config"},
		{name: "a before hook of init-from-module", hook: "before_hook", command: "init-from-module"},
		{name: "an after hook of init-from-module", hook: "after_hook", command: "init-from-module"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			files := withFiles(outputsContractFiles, map[string]string{
				"vpc/terragrunt.hcl": `
terraform {
  source = "${get_terragrunt_dir()}/../modules/vpc"

  ` + tc.hook + ` "mark" {
    commands = ["` + tc.command + `"]
    execute  = ["touch", "hook-ran"]
  }
}

outputs {
  output "vpc_id" {
    mock_value = "vpc-mock"
  }

  output "subnet_ids" {
    mock_value = []
  }
}
`,
				"modules/vpc/main.tf": outputsContractFiles["vpc/main.tf"],
			})
			delete(files, "vpc/main.tf")

			result := runValidate(t, &validateRun{files: files, experiments: []string{experiment.OutputsBlock}, outputs: true})

			require.NoError(t, result.err)
			assert.Empty(t, result.diags)
		})
	}
}

// TestRunValidateOutputsContractsGeneratesTheBackendFile expects remote_state generate to honor if_exists on a file.
func TestRunValidateOutputsContractsGeneratesTheBackendFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		ifExists string
		fails    bool
	}{
		{name: "an existing backend file is overwritten", ifExists: "overwrite"},
		{name: "an existing backend file is an error", ifExists: "error", fails: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			files := withFiles(outputsContractFiles, map[string]string{
				"vpc/backend.tf": "terraform {}\n",
				"vpc/terragrunt.hcl": `
remote_state {
  backend = "local"

  generate = {
    path      = "backend.tf"
    if_exists = "` + tc.ifExists + `"
  }

  config = {
    path = "terraform.tfstate"
  }
}

outputs {
  output "vpc_id" {
    mock_value = "vpc-mock"
  }

  output "subnet_ids" {
    mock_value = []
  }
}
`,
			})

			result := runValidate(t, &validateRun{files: files, experiments: []string{experiment.OutputsBlock}, outputs: true})

			if tc.fails {
				require.ErrorAs(t, result.err, new(validate.OutputsContractError))

				return
			}

			require.NoError(t, result.err)
			assert.Empty(t, result.diags)

			backend, err := vfs.ReadFile(result.fs, filepath.Join(result.root, "vpc", "backend.tf"))
			require.NoError(t, err)
			assert.Equal(t, "terraform {}\n", string(backend), "the unit directory keeps its own file")
		})
	}
}

// TestRunValidateOutputsContractsPointsAtDependencyErrors expects each dependency read error at one place.
func TestRunValidateOutputsContractsPointsAtDependencyErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files           map[string]string
		name            string
		expectedSummary string
		expectedFile    string
		queries         []string
		expectedLine    int
	}{
		{
			name: "an unparsable dependency config is reported once, by its own parse",
			files: map[string]string{
				"vpc/terragrunt.hcl": `
outputs {
  output "vpc_id" {
`,
			},
			expectedSummary: "Unclosed configuration block",
			expectedFile:    "vpc/terragrunt.hcl",
			expectedLine:    3,
		},
		{
			name: "an unreadable dependency config the run leaves out is reported at the config_path that names it",
			files: map[string]string{
				"vpc/terragrunt.hcl": `
outputs {
  output "vpc_id" {
`,
			},
			queries:         []string{"./app"},
			expectedSummary: "Dependency config not readable",
			expectedFile:    "app/terragrunt.hcl",
			expectedLine:    3,
		},
		{
			name: "a mock value that reads a dependency output is reported at its declaration",
			files: map[string]string{
				"net/main.tf": `
output "cidr" {
  value = "10.0.0.0/16"
}
`,
				"net/terragrunt.hcl": `
outputs {
  output "cidr" {
    mock_value = "10.0.0.0/16"
  }
}
`,
				"vpc/main.tf": `
output "vpc_id" {
  value = "vpc"
}

output "subnet_ids" {
  value = []
}
`,
				"vpc/terragrunt.hcl": `
dependency "net" {
  config_path = "../net"
}

outputs {
  output "vpc_id" {
    mock_value = dependency.net.outputs.cidr
  }

  output "subnet_ids" {
    mock_value = []
  }
}
`,
			},
			expectedSummary: "Unknown mock value",
			expectedFile:    "vpc/terragrunt.hcl",
			expectedLine:    7,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runValidate(t, &validateRun{
				files:       withFiles(outputsContractFiles, tc.files),
				experiments: []string{experiment.OutputsBlock},
				outputs:     true,
				queries:     tc.queries,
			})

			require.Error(t, result.err)

			found := 0

			for _, diag := range result.diags {
				if diag.Summary != tc.expectedSummary {
					continue
				}

				found++

				require.NotNil(t, diag.Range)
				assert.Equal(t, filepath.Join(result.root, filepath.FromSlash(tc.expectedFile)), diag.Range.Filename)
				assert.Equal(t, tc.expectedLine, diag.Range.Start.Line)
				assert.NotContains(t, diag.Detail, "..")
			}

			assert.Equal(t, 1, found, "one %q diagnostic expected", tc.expectedSummary)
		})
	}
}

// TestRunValidateOutputsContractsLeavesValidatedDependencyToItsOwnParse expects one diagnostic for a broken dependency.
func TestRunValidateOutputsContractsLeavesValidatedDependencyToItsOwnParse(t *testing.T) {
	t.Parallel()

	files := withFiles(outputsContractFiles, map[string]string{
		"vpc/terragrunt.hcl": `
outputs {
  output "vpc_id" {
`,
		"web/main.tf":        outputsContractFiles["app/main.tf"],
		"web/terragrunt.hcl": outputsContractFiles["app/terragrunt.hcl"],
	})

	result := runValidate(t, &validateRun{files: files, experiments: []string{experiment.OutputsBlock}, outputs: true})

	require.Error(t, result.err)
	require.Len(t, result.diags, 1)
	assert.Equal(t, "Unclosed configuration block", result.diags[0].Summary)
	require.NotNil(t, result.diags[0].Range)
	assert.Equal(t, filepath.Join(result.root, "vpc", config.DefaultTerragruntConfigPath), result.diags[0].Range.Filename)
}

// TestRunValidateOutputsContractsKeepsDistinctDiagnosticsAtOneRange expects a diagnostic per unit at one shared range.
func TestRunValidateOutputsContractsKeepsDistinctDiagnosticsAtOneRange(t *testing.T) {
	t.Parallel()

	files := withFiles(overriddenIncludeFiles, map[string]string{
		"root.hcl": `
inputs = {
  id = dependency.vpc.outputs.nope
}
`,
		"app/main.tf":    ``,
		"envs/a/main.tf": `variable "id" {}`,
		"envs/a/terragrunt.hcl": `
include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "vpc" {
  config_path = "../../legacy"

  mock_outputs = {
    nope = "nope-mock"
  }
}
`,
		"envs/b/main.tf": `variable "id" {}`,
		"envs/b/terragrunt.hcl": `
include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "vpc" {
  config_path = "../../current"

  mock_outputs = {
    nope = "nope-mock"
  }
}
`,
	})

	result := runValidate(t, &validateRun{files: files, experiments: []string{experiment.OutputsBlock}, outputs: true})

	require.Error(t, result.err)

	details := make([]string, 0, len(result.diags))

	for _, diag := range result.diags {
		if diag.Summary != "Undeclared dependency output" {
			continue
		}

		require.NotNil(t, diag.Range)
		assert.Equal(t, filepath.Join(result.root, "root.hcl"), diag.Range.Filename)
		assert.Equal(t, 3, diag.Range.Start.Line)

		details = append(details, diag.Detail)
	}

	require.Len(t, details, 2)
	slices.Sort(details)
	assert.Contains(t, details[0], "Declared outputs: id.")
	assert.Contains(t, details[1], "Declared outputs: legacy_id.")
}

func TestRunValidateOutputsBlockRequiresExperiment(t *testing.T) {
	t.Parallel()

	result := runValidate(t, &validateRun{files: outputsContractFiles})

	var experimentErr config.OutputsBlockRequiresExperimentError

	require.ErrorAs(t, result.err, &experimentErr)
	assert.Equal(t, filepath.Join(result.root, "vpc", config.DefaultTerragruntConfigPath), experimentErr.ConfigPath)
}

// withFiles returns the files of base with those of overrides added or replaced.
func withFiles(base, overrides map[string]string) map[string]string {
	files := make(map[string]string, len(base)+len(overrides))
	maps.Copy(files, base)
	maps.Copy(files, overrides)

	return files
}

// versionProbe returns an exec that answers the version probe of any binary with output and refuses other commands.
const stsAccessDenied = `<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <Error>
    <Type>Sender</Type>
    <Code>AccessDenied</Code>
    <Message>the role cannot be assumed</Message>
  </Error>
  <RequestId>00000000-0000-0000-0000-000000000000</RequestId>
</ErrorResponse>`

func versionProbe(outputs map[string]string) vexec.Exec {
	return vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		output, known := outputs[filepath.Base(inv.Name)]
		if known && slices.Contains(inv.Args, "-version") {
			return vexec.Result{Stdout: []byte(output + "\n")}
		}

		return vexec.Result{Err: errors.New("only the version probe of a known binary runs")}
	})
}

// joinedLeaves returns the errors err joins, or err itself when it joins none.
func joinedLeaves(err error) []error {
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return []error{err}
	}

	var leaves []error

	for _, inner := range joined.Unwrap() {
		leaves = append(leaves, joinedLeaves(inner)...)
	}

	return leaves
}

// TestRunValidateOutputsContractsFollowsCopyExclusions expects a file exclude_from_copy leaves out to declare nothing.
func TestRunValidateOutputsContractsFollowsCopyExclusions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		terraform   string
		legacyFile  string
		experiments []string
		reported    bool
	}{
		{
			name:        "a file exclude_from_copy leaves out does not declare the output",
			terraform:   `exclude_from_copy = ["legacy.tf"]`,
			legacyFile:  "vpc/legacy.tf",
			experiments: []string{experiment.OutputsBlock},
			reported:    true,
		},
		{
			name:        "a unit that runs in place keeps every file",
			terraform:   "exclude_from_copy       = [\"legacy.tf\"]\n  no_dot_terragrunt_cache = true",
			legacyFile:  "vpc/legacy.tf",
			experiments: []string{experiment.OutputsBlock, experiment.NoDotTerragruntCache},
			reported:    false,
		},
		{
			name:        "a file whose name starts with a dot does not declare the output, even in place",
			terraform:   "no_dot_terragrunt_cache = true",
			legacyFile:  "vpc/.legacy.tf",
			experiments: []string{experiment.OutputsBlock, experiment.NoDotTerragruntCache},
			reported:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			files := map[string]string{
				"vpc/main.tf": `output "current" { value = "current" }`,
				tc.legacyFile: `output "retired" { value = "retired" }`,
				"vpc/terragrunt.hcl": `
terraform {
  ` + tc.terraform + `
}

outputs {
  output "retired" {
    mock_value = "mock-retired"
  }
}
`,
			}

			result := runValidate(t, &validateRun{files: files, experiments: tc.experiments, outputs: true})

			if !tc.reported {
				require.NoError(t, result.err)
				assert.Empty(t, result.diags)

				return
			}

			require.Error(t, result.err)
			require.Len(t, result.diags, 1)
			assert.Equal(t, "Declared output not in module", result.diags[0].Summary)
			assert.Contains(t, result.diags[0].Detail, `output "retired"`)
		})
	}
}

// TestRunValidateOutputsContractsReadsTheEngineFiles expects the module files of the binary of the run to be read.
func TestRunValidateOutputsContractsReadsTheEngineFiles(t *testing.T) {
	t.Parallel()

	const (
		openTofu  = "OpenTofu v1.10.0"
		terraform = "Terraform v1.9.0"
	)

	testCases := []struct {
		probes          map[string]string
		name            string
		tfPath          string
		terraformBinary string
		missing         string
	}{
		{
			name:    "opentofu does not read the .tf file beside its .tofu twin",
			tfPath:  "tofu",
			probes:  map[string]string{"tofu": openTofu},
			missing: "retired",
		},
		{
			name:    "terraform does not read .tofu files",
			tfPath:  "terraform",
			probes:  map[string]string{"terraform": terraform},
			missing: "current",
		},
		{
			name:            "terraform_binary names the binary of the run",
			terraformBinary: "terraform",
			probes:          map[string]string{"tofu": openTofu, "terraform": terraform},
			missing:         "current",
		},
		{
			name:            "--tf-path wins over terraform_binary",
			tfPath:          "tofu",
			terraformBinary: "terraform",
			probes:          map[string]string{"tofu": openTofu, "terraform": terraform},
			missing:         "retired",
		},
		{
			name:    "the probe of the binary decides, not its name",
			tfPath:  "tofu",
			probes:  map[string]string{"tofu": terraform},
			missing: "current",
		},
		{
			name:   "a binary that cannot be probed reads every module file",
			tfPath: "tofu",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			terraformBinary := ""
			if tc.terraformBinary != "" {
				terraformBinary = `terraform_binary = "` + tc.terraformBinary + `"`
			}

			var exec vexec.Exec
			if tc.probes != nil {
				exec = versionProbe(tc.probes)
			}

			files := map[string]string{
				"vpc/main.tf":   `output "retired" { value = "retired" }`,
				"vpc/main.tofu": `output "current" { value = "current" }`,
				"vpc/terragrunt.hcl": terraformBinary + `
outputs {
  output "retired" {
    mock_value = "mock-retired"
  }

  output "current" {
    mock_value = "mock-current"
  }
}
`,
			}

			result := runValidate(t, &validateRun{
				files:       files,
				experiments: []string{experiment.OutputsBlock},
				outputs:     true,
				tfPath:      tc.tfPath,
				exec:        exec,
			})

			if tc.missing == "" {
				require.NoError(t, result.err)
				assert.Empty(t, result.diags)

				return
			}

			require.Error(t, result.err)
			require.Len(t, result.diags, 1)
			assert.Equal(t, "Declared output not in module", result.diags[0].Summary)
			assert.Contains(t, result.diags[0].Detail, `output "`+tc.missing+`"`)
		})
	}
}

// TestRunValidateOutputsContractsObtainsCredentialsBeforeTheFetch runs the auth provider command before the fetch.
func TestRunValidateOutputsContractsObtainsCredentialsBeforeTheFetch(t *testing.T) {
	t.Parallel()

	files := withFiles(outputsContractFiles, map[string]string{
		"vpc/terragrunt.hcl": `
terraform {
  source = "${get_terragrunt_dir()}/../modules/vpc"
}

outputs {
  output "vpc_id" {
    mock_value = "vpc-mock"
  }

  output "subnet_ids" {
    mock_value = []
  }
}
`,
		"modules/vpc/main.tf": outputsContractFiles["vpc/main.tf"],
	})
	delete(files, "vpc/main.tf")

	fsys := vfs.NewMemMapFS()
	cacheDir := filepath.Join(venvtest.Root("/repo"), "vpc", ".terragrunt-cache")

	var (
		mu            sync.Mutex
		ran           []string
		fetchedBefore bool
	)

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		mu.Lock()
		defer mu.Unlock()

		ran = append(ran, inv.Name)

		if inv.Name == "fetch-creds" {
			fetchedBefore = fetchedBefore || vfs.Exists(fsys, cacheDir)

			return vexec.Result{Stdout: []byte(`{"envs": {"TG_TEST_CREDENTIAL": "granted"}}`)}
		}

		return vexec.Result{Err: errors.New("only the credentials command runs")}
	})

	result := runValidate(t, &validateRun{
		fs:              fsys,
		files:           files,
		experiments:     []string{experiment.OutputsBlock},
		outputs:         true,
		exec:            exec,
		authProviderCmd: "fetch-creds",
	})

	require.NoError(t, result.err)
	assert.Empty(t, result.diags)

	mu.Lock()
	defer mu.Unlock()

	assert.Contains(t, ran, "fetch-creds")
	assert.False(t, fetchedBefore, "the credentials are obtained before the fetch begins")
	assert.True(t, vfs.Exists(fsys, cacheDir), "the fetch ran")
}

// TestRunValidateOutputsContractsAssumesTheUnitRoleBeforeTheFetch expects the unit's iam_role assumed before a fetch.
func TestRunValidateOutputsContractsAssumesTheUnitRoleBeforeTheFetch(t *testing.T) {
	t.Parallel()

	files := withFiles(outputsContractFiles, map[string]string{
		"vpc/terragrunt.hcl": `
iam_role = "arn:aws:iam::123456789012:role/module-reader"

terraform {
  source = "${get_terragrunt_dir()}/../modules/vpc"
}
` + outputsContractFiles["vpc/terragrunt.hcl"],
		"modules/vpc/main.tf": outputsContractFiles["vpc/main.tf"],
	})
	delete(files, "vpc/main.tf")

	fsys := vfs.NewMemMapFS()
	cacheDir := filepath.Join(venvtest.Root("/repo"), "vpc", ".terragrunt-cache")

	var (
		mu    sync.Mutex
		hosts []string
	)

	denied := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()

		hosts = append(hosts, req.URL.Host)

		return vhttp.Respond(http.StatusForbidden, []byte(stsAccessDenied), nil), nil
	})

	result := runValidate(t, &validateRun{
		fs:          fsys,
		files:       files,
		experiments: []string{experiment.OutputsBlock},
		outputs:     true,
		http:        denied,
		env: map[string]string{
			"AWS_REGION":            "us-east-1",
			"AWS_ACCESS_KEY_ID":     "caller-key-id",
			"AWS_SECRET_ACCESS_KEY": "caller-secret",
		},
	})

	require.Error(t, result.err)

	mu.Lock()
	defer mu.Unlock()

	require.NotEmpty(t, hosts)
	assert.Equal(t, "sts.us-east-1.amazonaws.com", hosts[0])
	assert.False(t, vfs.Exists(fsys, cacheDir), "nothing is fetched without the role")
}

// TestRunValidateOutputsContractsFetchesFromTheRegistryOfTheUnitEngine expects terraform_binary to pick the registry.
func TestRunValidateOutputsContractsFetchesFromTheRegistryOfTheUnitEngine(t *testing.T) {
	t.Parallel()

	files := withFiles(outputsContractFiles, map[string]string{
		"vpc/terragrunt.hcl": `
terraform_binary = "terraform"

terraform {
  source = "tfr:///example/vpc/aws?version=1.0.0"
}
` + outputsContractFiles["vpc/terragrunt.hcl"],
	})
	delete(files, "vpc/main.tf")

	var (
		mu    sync.Mutex
		hosts []string
	)

	missing := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()

		hosts = append(hosts, req.URL.Host)

		return vhttp.Respond(http.StatusNotFound, nil, nil), nil
	})

	result := runValidate(t, &validateRun{
		files:       files,
		experiments: []string{experiment.OutputsBlock},
		outputs:     true,
		http:        missing,
		exec:        versionProbe(map[string]string{"tofu": "OpenTofu v1.10.0", "terraform": "Terraform v1.9.0"}),
	})

	require.Error(t, result.err)

	mu.Lock()
	defer mu.Unlock()

	require.NotEmpty(t, hosts)
	assert.Equal(t, "registry.terraform.io", hosts[0])
}

// TestRunValidateOutputsContractsRejectsInPlaceWithSource expects the cache guard of a run before any fetch.
func TestRunValidateOutputsContractsRejectsInPlaceWithSource(t *testing.T) {
	t.Parallel()

	files := withFiles(outputsContractFiles, map[string]string{
		"vpc/terragrunt.hcl": `
terraform {
  source                  = "${get_terragrunt_dir()}/../modules/vpc"
  no_dot_terragrunt_cache = true
}

outputs {
  output "vpc_id" {
    mock_value = "vpc-mock"
  }

  output "subnet_ids" {
    mock_value = []
  }
}
`,
		"modules/vpc/main.tf": outputsContractFiles["vpc/main.tf"],
	})
	delete(files, "vpc/main.tf")

	result := runValidate(t, &validateRun{
		files:       files,
		experiments: []string{experiment.OutputsBlock, experiment.NoDotTerragruntCache},
		outputs:     true,
	})

	require.ErrorAs(t, result.err, new(validate.OutputsContractError))
	require.ErrorAs(t, result.err, new(runcfg.NoDotTerragruntCacheWithSourceError))
	assert.False(t, vfs.Exists(result.fs, filepath.Join(result.root, "vpc", ".terragrunt-cache")))
}

// TestRunValidateOutputsContractsChecksTheIncludesOfTheAutoinclude checks the files the autoinclude merges.
func TestRunValidateOutputsContractsChecksTheIncludesOfTheAutoinclude(t *testing.T) {
	t.Parallel()

	files := withFiles(outputsContractFiles, map[string]string{
		"root.hcl": `
inputs = {
  ghost = dependency.vpc.outputs.ghost
}
`,
		"app/terragrunt.autoinclude.hcl": `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`,
		"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    ghost = "ghost-mock"
  }
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
`,
	})

	result := runValidate(t, &validateRun{files: files, experiments: []string{experiment.OutputsBlock}, outputs: true})

	require.Error(t, result.err)
	require.Len(t, result.diags, 1)

	diag := result.diags[0]

	assert.Equal(t, "Undeclared dependency output", diag.Summary)
	require.NotNil(t, diag.Range)
	assert.Equal(t, filepath.Join(result.root, "root.hcl"), diag.Range.Filename)
	assert.Equal(t, 3, diag.Range.Start.Line)
}

// TestRunValidateOutputsContractsSkipsDependencyBlocksTheAutoincludeReplaces checks only the autoinclude block.
func TestRunValidateOutputsContractsSkipsDependencyBlocksTheAutoincludeReplaces(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files        map[string]string
		name         string
		expectedFile string
	}{
		{
			name: "a reference in a replaced block",
			files: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

dependency "db" {
  config_path = "../unused"

  mock_outputs = {
    unused = dependency.vpc.outputs.ghost
  }
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
`,
				"app/terragrunt.autoinclude.hcl": `
dependency "db" {
  config_path  = "../db"
  skip_outputs = true

  mock_outputs = {}
}
`,
			},
		},
		{
			name: "a reference in the block that replaces it",
			files: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

dependency "db" {
  config_path = "../unused"
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
`,
				"app/terragrunt.autoinclude.hcl": `
dependency "db" {
  config_path  = "../db"
  skip_outputs = true

  mock_outputs = {
    unused = dependency.vpc.outputs.ghost
  }
}
`,
			},
			expectedFile: filepath.Join("app", config.DefaultAutoIncludeFile),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			files := map[string]string{
				"vpc/main.tf":        outputsContractFiles["vpc/main.tf"],
				"vpc/terragrunt.hcl": outputsContractFiles["vpc/terragrunt.hcl"],
				"app/main.tf":        outputsContractFiles["app/main.tf"],
				"db/main.tf":         ``,
				"db/terragrunt.hcl":  ``,
			}
			maps.Copy(files, tc.files)

			result := runValidate(t, &validateRun{files: files, experiments: []string{experiment.OutputsBlock}, outputs: true})

			found := 0

			for _, diag := range result.diags {
				if diag.Summary != "Undeclared dependency output" {
					continue
				}

				found++

				require.NotNil(t, diag.Range)
				assert.Equal(t, filepath.Join(result.root, tc.expectedFile), diag.Range.Filename)
			}

			if tc.expectedFile == "" {
				require.NoError(t, result.err)
				assert.Empty(t, result.diags)

				return
			}

			require.Error(t, result.err)
			assert.Equal(t, 1, found, "one undeclared output diagnostic expected")
		})
	}
}

// TestRunValidateChecksOutputsContractsOnlyWithTheFlag expects no contract diagnostic until --outputs asks for one.
func TestRunValidateChecksOutputsContractsOnlyWithTheFlag(t *testing.T) {
	t.Parallel()

	files := withFiles(outputsContractFiles, map[string]string{"vpc/main.tf": vpcModuleWithoutSubnets})

	testCases := []struct {
		name     string
		outputs  bool
		reported bool
	}{
		{name: "without the flag"},
		{name: "with the flag", outputs: true, reported: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runValidate(t, &validateRun{
				files:       files,
				experiments: []string{experiment.OutputsBlock},
				outputs:     tc.outputs,
			})

			if !tc.reported {
				require.NoError(t, result.err)
				assert.Empty(t, result.diags)

				return
			}

			require.Error(t, result.err)
			require.Len(t, result.diags, 1)
			assert.Equal(t, "Declared output not in module", result.diags[0].Summary)
		})
	}
}

// TestRunOutputsFlagRequiresExperiment expects --outputs to fail while the outputs-block experiment is off.
func TestRunOutputsFlagRequiresExperiment(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")
	fsys := venvtest.NewFS(t, root, map[string]string{"app/terragrunt.hcl": "\n"})

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(root, config.DefaultTerragruntConfigPath))
	require.NoError(t, err)

	opts.HCLValidateOutputs = true

	v := venv.MemVenv().WithFS(fsys)

	err = validate.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, new(rewrite.Store), opts)

	var experimentErr validate.OutputsExperimentRequiredError

	require.ErrorAs(t, err, &experimentErr)
}

// TestRunValidateOutputsContractsParseEachConfigOnce expects one read per config, however many units share it.
func TestRunValidateOutputsContractsParseEachConfigOnce(t *testing.T) {
	t.Parallel()

	const rootInclude = `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`

	dependent := rootInclude + outputsContractFiles["app/terragrunt.hcl"]
	files := map[string]string{
		"root.hcl":           "\n",
		"vpc/main.tf":        outputsContractFiles["vpc/main.tf"],
		"vpc/terragrunt.hcl": rootInclude + outputsContractFiles["vpc/terragrunt.hcl"],
		"app/main.tf":        outputsContractFiles["app/main.tf"],
		"app/terragrunt.hcl": dependent,
		"web/main.tf":        outputsContractFiles["app/main.tf"],
		"web/terragrunt.hcl": dependent,
		"api/main.tf":        outputsContractFiles["app/main.tf"],
		"api/terragrunt.hcl": dependent,
		"api/terragrunt.autoinclude.hcl": `
dependency "network" {
  config_path = "../vpc"
}
`,
	}

	opens := func(outputs bool) map[string]int {
		fsys := newOpenCountingFS()
		result := runValidate(t, &validateRun{
			files:       files,
			fs:          fsys,
			experiments: []string{experiment.OutputsBlock},
			outputs:     outputs,
		})

		require.NoError(t, result.err)

		counts := map[string]int{}
		for _, path := range []string{"root.hcl", "app/terragrunt.hcl", "api/terragrunt.autoinclude.hcl"} {
			counts[path] = fsys.count(filepath.Join(result.root, filepath.FromSlash(path)))
		}

		return counts
	}

	without := opens(false)
	with := opens(true)

	for path, count := range without {
		assert.Equal(t, count+1, with[path], path)
	}
}

// TestRunValidateOutputsContractsOpenNoConfigAgainWithoutAContract expects the experiment alone to reread no unit.
func TestRunValidateOutputsContractsOpenNoConfigAgainWithoutAContract(t *testing.T) {
	t.Parallel()

	const rootInclude = `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`

	files := map[string]string{
		"root.hcl":           "\n",
		"vpc/main.tf":        outputsContractFiles["vpc/main.tf"],
		"vpc/terragrunt.hcl": rootInclude,
		"app/main.tf":        outputsContractFiles["app/main.tf"],
		"app/terragrunt.hcl": rootInclude + `
dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    vpc_id = "vpc-mock"
  }
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
`,
	}

	opens := func(experiments ...string) int {
		fsys := newOpenCountingFS()
		result := runValidate(t, &validateRun{
			files:       files,
			fs:          fsys,
			experiments: experiments,
			outputs:     len(experiments) > 0,
		})

		require.NoError(t, result.err)
		assert.Empty(t, result.diags)

		return fsys.count(filepath.Join(result.root, "app", config.DefaultTerragruntConfigPath))
	}

	without := opens()
	with := opens(experiment.OutputsBlock)

	assert.Positive(t, without)
	assert.Equal(t, without, with)
}

func newOpenCountingFS() *openCountingFS {
	return &openCountingFS{FS: vfs.NewMemMapFS(), created: time.Now(), opens: map[string]int{}}
}

type openCountingFS struct {
	vfs.FS
	created time.Time
	opens   map[string]int
	mu      sync.Mutex
}

// Stat reports every file as an hour old, past the window in which a cache reads a file back to compare it.
func (fsys *openCountingFS) Stat(name string) (fs.FileInfo, error) {
	info, err := fsys.FS.Stat(name)
	if err != nil {
		return nil, err
	}

	return settledInfo{FileInfo: info, modTime: fsys.created.Add(-time.Hour)}, nil
}

type settledInfo struct {
	fs.FileInfo
	modTime time.Time
}

func (info settledInfo) ModTime() time.Time {
	return info.modTime
}

func (fsys *openCountingFS) Open(name string) (vfs.File, error) {
	fsys.mu.Lock()
	fsys.opens[filepath.Clean(name)]++
	fsys.mu.Unlock()

	return fsys.FS.Open(name)
}

// count returns how often name was opened.
func (fsys *openCountingFS) count(name string) int {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()

	return fsys.opens[filepath.Clean(name)]
}
