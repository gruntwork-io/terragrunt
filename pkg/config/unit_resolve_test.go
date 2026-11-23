package config_test

import (
	"context"
	"maps"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/hcl/v2"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

func TestResolveRunConfigDecodesTheOutputsBlock(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	files := map[string]string{
		"vpc/main.tf": `
output "vpc_id" {
  value = "vpc-real"
}
`,
		"vpc/terragrunt.hcl": `
outputs {
  mock_strategy {
    allowed_commands = ["plan"]
    merge_with_state = "shallow"
  }

  output "vpc_id" {
    mock_value = "vpc-mock"
  }

  output "subnet_ids" {
    mock_value = ["subnet-mock"]
  }
}
`,
		"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    vpc_id = "app-mock"
  }
}

inputs = {
  vpc_id     = dependency.vpc.outputs.vpc_id
  subnet_ids = dependency.vpc.outputs.subnet_ids
}
`,
	}

	planWithExperiment := func(t *testing.T, pctx *config.ParsingContext) {
		t.Helper()

		require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

		pctx.OriginalTofuCommand = "plan"
	}

	testCases := []struct {
		configure func(t *testing.T, pctx *config.ParsingContext)
		check     func(t *testing.T, result unitResult)
		outputs   map[string]string
		name      string
		cfgPath   string
	}{
		{
			name:      "declaring unit",
			cfgPath:   filepath.Join(root, "vpc", config.DefaultTerragruntConfigPath),
			configure: planWithExperiment,
			check: func(t *testing.T, result unitResult) {
				t.Helper()

				require.NoError(t, result.runConfigErr)
				assert.Equal(t, []string{"vpc_id", "subnet_ids"}, result.cfg.Outputs.Names())
			},
		},
		{
			name:      "declaring unit without the experiment",
			cfgPath:   filepath.Join(root, "vpc", config.DefaultTerragruntConfigPath),
			configure: func(*testing.T, *config.ParsingContext) {},
			check: func(t *testing.T, result unitResult) {
				t.Helper()

				var experimentErr config.OutputsBlockRequiresExperimentError

				require.ErrorAs(t, result.runConfigErr, &experimentErr)
			},
		},
		{
			name:      "dependent reading state",
			cfgPath:   filepath.Join(root, "app", config.DefaultTerragruntConfigPath),
			configure: planWithExperiment,
			outputs:   map[string]string{filepath.Join(root, "vpc"): outputsJSON(map[string]string{"vpc_id": "vpc-real"})},
			check: func(t *testing.T, result unitResult) {
				t.Helper()

				require.NoError(t, result.runConfigErr)
				assert.Equal(t, "vpc-real", result.cfg.Inputs["vpc_id"])
				assert.Equal(t, []any{"subnet-mock"}, result.cfg.Inputs["subnet_ids"])
			},
		},
		{
			name:      "dependent reading mocks",
			cfgPath:   filepath.Join(root, "app", config.DefaultTerragruntConfigPath),
			configure: planWithExperiment,
			outputs:   map[string]string{filepath.Join(root, "vpc"): "{}"},
			check: func(t *testing.T, result unitResult) {
				t.Helper()

				require.NoError(t, result.runConfigErr)
				assert.Equal(t, "app-mock", result.cfg.Inputs["vpc_id"])
				assert.Equal(t, []any{"subnet-mock"}, result.cfg.Inputs["subnet_ids"])
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.check(t, resolveUnit(t, unitCase{
				newFS:     memFS(root, files),
				cfgPath:   tc.cfgPath,
				configure: func(pctx *config.ParsingContext) { tc.configure(t, pctx) },
				outputs:   tc.outputs,
			}))
		})
	}
}

// dependencyAttributeCases maps a name to a unit config in which a run attribute reads an output of the dependency on
// ../vpc.
func dependencyAttributeCases() map[string]string {
	const dependency = `
dependency "vpc" {
  config_path = "../vpc"
}
`

	return map[string]string{
		"download_dir":                 dependency + "download_dir = dependency.vpc.outputs.data\n",
		"iam_assume_role_duration":     dependency + "iam_assume_role_duration = length(dependency.vpc.outputs.data)\n",
		"iam_assume_role_session_name": dependency + "iam_assume_role_session_name = dependency.vpc.outputs.data\n",
		"attribute beside one that reads no dependency": dependency +
			"download_dir = dependency.vpc.outputs.data\nterraform_binary = \"tofu\"\n",
	}
}

// unitVersionAttributeCases maps a name to a unit config in which a version attribute references the dependency on
// ../vpc.
func unitVersionAttributeCases() map[string]string {
	const dependency = `
dependency "vpc" {
  config_path = "../vpc"
}
`

	return map[string]string{
		"terraform_binary":              dependency + "terraform_binary = dependency.vpc.outputs.data\n",
		"terraform_version_constraint":  dependency + "terraform_version_constraint = dependency.vpc.outputs.data\n",
		"terragrunt_version_constraint": dependency + "terragrunt_version_constraint = dependency.vpc.outputs.data\n",
		"untaken branch": dependency + `
locals {
  use = false
}

terraform_version_constraint = local.use ? dependency.vpc.outputs.data : ">= 1.0"
`,
	}
}

func TestResolveRunConfigDecodesAttributesThatReadDependencyOutputs(t *testing.T) {
	t.Parallel()

	for name, content := range dependencyAttributeCases() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := resolveDependencyAttributeUnit(t, name, content)

			require.NoError(t, result.runConfigErr)
		})
	}
}

func TestResolveRunConfigRejectsVersionAttributesThatReferenceDependency(t *testing.T) {
	t.Parallel()

	for name, content := range unitVersionAttributeCases() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := resolveDependencyAttributeUnit(t, name, content)

			requireDependencyReferenceDiagnostic(t, result.runConfigErr)
		})
	}
}

// newQueueParsingContext returns a context for parsing cfgPath the way discovery parses a unit.
func newQueueParsingContext(t *testing.T, cfgPath string) (context.Context, *config.ParsingContext) {
	t.Helper()

	ctx, run := newTestUnitParsingContext(t, cfgPath)
	run.SkipOutput = true

	return ctx, run.WithDecodeList(
		config.TerraformSource,
		config.DependenciesBlock,
		config.DependencyBlock,
		config.TerragruntFlags,
		config.FeatureFlagsBlock,
		config.ExcludeBlock,
		config.ErrorsBlock,
		config.TerragruntVersionConstraints,
		config.EngineBlock,
	).WithSkipOutputsResolution()
}

func TestQueueConfigDecodesBesideAttributesThatReadDependencyOutputs(t *testing.T) {
	t.Parallel()

	cases := dependencyAttributeCases()
	cases["dependency whose config_path reads another dependency"] = `
dependency "vpc" {
  config_path = "../vpc"
}

dependency "next" {
  config_path = dependency.vpc.outputs.data
}
`

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := resolveDependencyAttributeQueueConfig(t, name, content)
			require.NoError(t, err)
		})
	}
}

func TestQueueConfigRejectsVersionAttributesThatReferenceDependency(t *testing.T) {
	t.Parallel()

	for name, content := range unitVersionAttributeCases() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := resolveDependencyAttributeQueueConfig(t, name, content)

			requireDependencyReferenceDiagnostic(t, err)
		})
	}
}

// requireDependencyReferenceDiagnostic asserts that err is the one diagnostic a version attribute that references
// `dependency` reports.
func requireDependencyReferenceDiagnostic(t *testing.T, err error) {
	t.Helper()

	var diags hcl.Diagnostics
	require.ErrorAs(t, err, &diags)
	require.Len(t, diags, 1)
	assert.Equal(t, "Dependency reference not allowed", diags[0].Summary)
}

// resolveDependencyAttributeUnit resolves the unit config content in app/, beside an empty unit in vpc/.
func resolveDependencyAttributeUnit(t *testing.T, name, content string) unitResult {
	t.Helper()

	root := venvtest.Root("/fixture-" + name)

	return resolveUnit(t, unitCase{
		newFS: memFS(root, map[string]string{
			"app/terragrunt.hcl": content,
			"app/main.tf":        "",
			"vpc/terragrunt.hcl": "",
			"vpc/main.tf":        "",
		}),
		cfgPath:   filepath.Join(root, "app", config.DefaultTerragruntConfigPath),
		configure: func(*config.ParsingContext) {},
	})
}

// resolveDependencyAttributeQueueConfig returns the queue config of the unit config content in app/, beside an empty
// unit in vpc/, as discovery resolves it.
func resolveDependencyAttributeQueueConfig(t *testing.T, name, content string) (*config.TerragruntConfig, error) {
	t.Helper()

	root := venvtest.Root("/fixture-queue-" + name)
	cfgPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
	v := venv.MemVenv().WithFS(memFS(root, map[string]string{"app/terragrunt.hcl": content, "vpc/terragrunt.hcl": ""})(t))
	ctx, run := newQueueParsingContext(t, cfgPath)

	return new(config.Store).ResolveUnitQueueConfig(
		ctx,
		logger.CreateLogger(),
		v,
		run,
		cfgPath,
	)
}

func TestResolveRunConfigRejectsEmptyDependencyOutputs(t *testing.T) {
	t.Parallel()

	const dependency = `
dependency "vpc" {
  config_path = "../vpc"
}
`

	const inputs = `
inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
`

	testCases := []struct {
		files map[string]string
		name  string
	}{
		{
			name:  "unit that reads the output",
			files: map[string]string{"app/terragrunt.hcl": dependency + inputs},
		},
		{
			name: "included file that reads the output",
			files: map[string]string{
				"root.hcl": inputs,
				"app/terragrunt.hcl": dependency + `
include "root" {
  path = "../root.hcl"
}
`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture-empty-outputs-" + tc.name)
			files := map[string]string{"vpc/terragrunt.hcl": "", "vpc/main.tf": "", "app/main.tf": ""}
			maps.Copy(files, tc.files)

			result := resolveUnit(t, unitCase{
				newFS:   memFS(root, files),
				cfgPath: filepath.Join(root, "app", config.DefaultTerragruntConfigPath),
				outputs: map[string]string{filepath.Join(root, "vpc"): "{}"},
				configure: func(pctx *config.ParsingContext) {
					require.NoError(t, pctx.Experiments.EnableExperiment(experiment.EmptyDependencyOutputs))
				},
			})

			require.ErrorAs(t, result.runConfigErr, new(config.TerragruntOutputTargetNoOutputs))
		})
	}
}

func TestQueueConfigSkipsAttributesDiscoveryNeverReads(t *testing.T) {
	t.Parallel()

	const unset = `get_env("TG_TEST_QUEUE_UNSET")`

	attributes := map[string]string{
		"download_dir":                 "download_dir = " + unset + "\n",
		"iam_assume_role_duration":     "iam_assume_role_duration = tonumber(" + unset + ")\n",
		"iam_assume_role_session_name": "iam_assume_role_session_name = " + unset + "\n",
	}

	layouts := map[string]func(attribute string) map[string]string{
		"unit": func(attribute string) map[string]string {
			return map[string]string{"app/terragrunt.hcl": attribute, "app/main.tf": ""}
		},
		"exposed include with a dependency": func(attribute string) map[string]string {
			return map[string]string{
				"root.hcl": `
locals {
  skip = false
}

dependency "vpc" {
  config_path  = "../vpc"
  skip_outputs = true
}
` + attribute,
				"app/terragrunt.hcl": `
include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

exclude {
  if      = include.root.locals.skip
  actions = ["all"]
}
`,
				"app/main.tf":        "",
				"vpc/terragrunt.hcl": "",
				"vpc/main.tf":        "",
			}
		},
	}

	for layout, files := range layouts {
		for name, attribute := range attributes {
			t.Run(layout+"/"+name, func(t *testing.T) {
				t.Parallel()

				root := venvtest.Root("/fixture-queue-skips-" + layout + "-" + name)
				cfgPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
				newFS := memFS(root, files(attribute))

				ctx, run := newQueueParsingContext(t, cfgPath)
				v := venv.MemVenv().WithFS(newFS(t))
				_, err := new(config.Store).ResolveUnitQueueConfig(
					ctx, logger.CreateLogger(), v, run, cfgPath,
				)
				require.NoError(t, err)

				result := resolveUnit(t, unitCase{
					newFS:     newFS,
					cfgPath:   cfgPath,
					configure: func(*config.ParsingContext) {},
				})

				require.Error(t, result.runConfigErr)
			})
		}
	}
}

func TestQueueConfigReportsUnsupportedArgumentsOfAnExposedInclude(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture-queue-exposed-unsupported")
	cfgPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
	v := venv.MemVenv().WithFS(memFS(root, map[string]string{
		"root.hcl": `
unsupported = true

locals {
  binary = "tofu"
}
`,
		"app/terragrunt.hcl": `
include "root" {
  path   = "../root.hcl"
  expose = true
}

terraform_binary = include.root.locals.binary
`,
		"app/main.tf": "",
	})(t))

	ctx, run := newQueueParsingContext(t, cfgPath)

	_, err := new(config.Store).ResolveUnitQueueConfig(
		ctx, logger.CreateLogger(), v, run, cfgPath,
	)

	var unsupported []string

	for _, diags := range walkErrors(t, err).diags {
		for _, diag := range diags {
			if diag.Summary == "Unsupported argument" && diag.Subject != nil {
				unsupported = append(unsupported, diag.Subject.Filename)
			}
		}
	}

	assert.Equal(t, []string{filepath.Join(root, "root.hcl")}, unsupported)
}
