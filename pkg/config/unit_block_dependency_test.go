package config_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// blockDependencyCase is a unit config in which an attribute of a block reads the output `data` of the dependency on
// ../vpc, which is "data-from-state".
type blockDependencyCase struct {
	// check asserts what the run config holds.
	check func(t *testing.T, cfg *config.TerragruntConfig)
	// checkQueue asserts what the queue config holds.
	checkQueue func(t *testing.T, cfg *config.TerragruntConfig)
	// body is the unit config beside its dependency block.
	body string
}

// blockDependencyCases maps a name to a [blockDependencyCase].
func blockDependencyCases() map[string]blockDependencyCase {
	terraformCase := func(attribute string, check func(t *testing.T, tf *config.TerraformConfig)) blockDependencyCase {
		return blockDependencyCase{
			body: "terraform {\n  " + attribute + "\n}\n",
			check: func(t *testing.T, cfg *config.TerragruntConfig) {
				t.Helper()

				require.NotNil(t, cfg.Terraform)
				check(t, cfg.Terraform)
			},
			checkQueue: func(t *testing.T, cfg *config.TerragruntConfig) {
				t.Helper()

				require.NotNil(t, cfg.Terraform)
				assert.Equal(t, &config.TerraformConfig{}, cfg.Terraform)
			},
		}
	}

	remoteStateCase := func(attributes string, check func(t *testing.T, rs *remotestate.RemoteState)) blockDependencyCase {
		return blockDependencyCase{
			body: "remote_state {\n" + attributes + "\n  config = { path = \"state.tfstate\" }\n}\n",
			check: func(t *testing.T, cfg *config.TerragruntConfig) {
				t.Helper()

				require.NotNil(t, cfg.RemoteState)
				check(t, cfg.RemoteState)
			},
			checkQueue: func(t *testing.T, cfg *config.TerragruntConfig) {
				t.Helper()

				assert.Nil(t, cfg.RemoteState)
			},
		}
	}

	errorsCase := func(block string, check func(t *testing.T, errs *config.ErrorsConfig)) blockDependencyCase {
		return blockDependencyCase{
			body: "errors {\n" + block + "\n}\n",
			check: func(t *testing.T, cfg *config.TerragruntConfig) {
				t.Helper()

				require.NotNil(t, cfg.Errors)
				check(t, cfg.Errors)
			},
			checkQueue: func(t *testing.T, cfg *config.TerragruntConfig) {
				t.Helper()

				assert.Nil(t, cfg.Errors)
			},
		}
	}

	const isData = `dependency.vpc.outputs.data == "data-from-state"`

	return map[string]blockDependencyCase{
		"terraform version": {
			body: `
terraform {
  source  = "tfr:///terraform-aws-modules/vpc/aws"
  version = dependency.vpc.outputs.data
}
`,
			check: func(t *testing.T, cfg *config.TerragruntConfig) {
				t.Helper()

				require.NotNil(t, cfg.Terraform)
				assert.Equal(t, new("data-from-state"), cfg.Terraform.Version)
			},
			checkQueue: func(t *testing.T, cfg *config.TerragruntConfig) {
				t.Helper()

				require.NotNil(t, cfg.Terraform)
				assert.Nil(t, cfg.Terraform.Version)
				assert.Equal(t, new("tfr:///terraform-aws-modules/vpc/aws"), cfg.Terraform.Source)
			},
		},
		"terraform update_source_with_cas": terraformCase(
			"update_source_with_cas = "+isData,
			func(t *testing.T, tf *config.TerraformConfig) {
				t.Helper()

				assert.Equal(t, new(true), tf.UpdateSourceWithCAS)
			},
		),
		"terraform mutable": terraformCase("mutable = "+isData, func(t *testing.T, tf *config.TerraformConfig) {
			t.Helper()

			assert.Equal(t, new(true), tf.Mutable)
		}),
		"terraform include_in_copy": terraformCase(
			"include_in_copy = [dependency.vpc.outputs.data]",
			func(t *testing.T, tf *config.TerraformConfig) {
				t.Helper()

				assert.Equal(t, new([]string{"data-from-state"}), tf.IncludeInCopy)
			},
		),
		"terraform exclude_from_copy": terraformCase(
			"exclude_from_copy = [dependency.vpc.outputs.data]",
			func(t *testing.T, tf *config.TerraformConfig) {
				t.Helper()

				assert.Equal(t, new([]string{"data-from-state"}), tf.ExcludeFromCopy)
			},
		),
		"terraform copy_terraform_lock_file": terraformCase(
			"copy_terraform_lock_file = "+isData,
			func(t *testing.T, tf *config.TerraformConfig) {
				t.Helper()

				assert.Equal(t, new(true), tf.CopyTerraformLockFile)
			},
		),
		"terraform no_dot_terragrunt_cache": terraformCase(
			"no_dot_terragrunt_cache = "+isData,
			func(t *testing.T, tf *config.TerraformConfig) {
				t.Helper()

				assert.Equal(t, new(true), tf.NoDotTerragruntCache)
			},
		),
		"terraform attribute beside one that reads no dependency": terraformCase(
			"mutable = true\n  include_in_copy = [dependency.vpc.outputs.data]",
			func(t *testing.T, tf *config.TerraformConfig) {
				t.Helper()

				assert.Equal(t, new(true), tf.Mutable)
				assert.Equal(t, new([]string{"data-from-state"}), tf.IncludeInCopy)
			},
		),
		"remote_state backend": remoteStateCase(
			"  backend = "+isData+` ? "local" : "s3"`,
			func(t *testing.T, rs *remotestate.RemoteState) {
				t.Helper()

				assert.Equal(t, "local", rs.BackendName)
			},
		),
		"remote_state generate": remoteStateCase(`
  backend  = "local"
  generate = { path = "${dependency.vpc.outputs.data}.tf", if_exists = "overwrite" }
`,
			func(t *testing.T, rs *remotestate.RemoteState) {
				t.Helper()

				assert.Equal(
					t,
					&remotestate.ConfigGenerate{Path: "data-from-state.tf", IfExists: "overwrite"},
					rs.Generate,
				)
			},
		),
		"remote_state disable_init": remoteStateCase(
			"  backend = \"local\"\n  disable_init = "+isData,
			func(t *testing.T, rs *remotestate.RemoteState) {
				t.Helper()

				assert.True(t, rs.DisableInit)
			},
		),
		"remote_state disable_dependency_optimization": remoteStateCase(
			"  backend = \"local\"\n  disable_dependency_optimization = "+isData,
			func(t *testing.T, rs *remotestate.RemoteState) {
				t.Helper()

				assert.True(t, rs.DisableDependencyOptimization)
			},
		),
		"errors retry": errorsCase(`
  retry "r" {
    retryable_errors   = [".*${dependency.vpc.outputs.data}.*"]
    max_attempts       = length(dependency.vpc.outputs.data)
    sleep_interval_sec = 1
  }
`,
			func(t *testing.T, errs *config.ErrorsConfig) {
				t.Helper()

				require.Len(t, errs.Retry, 1)
				assert.Equal(t, []string{".*data-from-state.*"}, errs.Retry[0].RetryableErrors)
				assert.Equal(t, len("data-from-state"), errs.Retry[0].MaxAttempts)
			},
		),
		"errors retry with try": errorsCase(`
  retry "r" {
    retryable_errors   = try([dependency.vpc.outputs.data], ["fallback"])
    max_attempts       = 3
    sleep_interval_sec = 1
  }
`,
			func(t *testing.T, errs *config.ErrorsConfig) {
				t.Helper()

				require.Len(t, errs.Retry, 1)
				assert.Equal(t, []string{"data-from-state"}, errs.Retry[0].RetryableErrors)
			},
		),
		"errors ignore": errorsCase(`
  ignore "i" {
    ignorable_errors = [dependency.vpc.outputs.data]
    signals          = { x = dependency.vpc.outputs.data }
  }
`,
			func(t *testing.T, errs *config.ErrorsConfig) {
				t.Helper()

				require.Len(t, errs.Ignore, 1)
				assert.Equal(t, []string{"data-from-state"}, errs.Ignore[0].IgnorableErrors)
				assert.Equal(t, map[string]cty.Value{"x": cty.StringVal("data-from-state")}, errs.Ignore[0].Signals)
			},
		),
		"errors retry beside an ignore block that reads no dependency": errorsCase(`
  retry "r" {
    retryable_errors   = [dependency.vpc.outputs.data]
    max_attempts       = 2
    sleep_interval_sec = 1
  }

  ignore "i" {
    ignorable_errors = ["literal"]
  }
`,
			func(t *testing.T, errs *config.ErrorsConfig) {
				t.Helper()

				require.Len(t, errs.Retry, 1)
				assert.Equal(t, []string{"data-from-state"}, errs.Retry[0].RetryableErrors)
				require.Len(t, errs.Ignore, 1)
				assert.Equal(t, []string{"literal"}, errs.Ignore[0].IgnorableErrors)
			},
		),
	}
}

// blockDependencyConfig returns the unit config of body, beside its dependency block on ../vpc.
func blockDependencyConfig(body string) string {
	return `
dependency "vpc" {
  config_path = "../vpc"
}
` + body
}

func TestResolveRunConfigDecodesBlockAttributesThatReadDependencyOutputs(t *testing.T) {
	t.Parallel()

	for name, tc := range blockDependencyCases() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := resolveDependencyAttributeUnit(t, "block-"+name, blockDependencyConfig(tc.body))

			require.NoError(t, result.parseErr)
			assert.Zero(t, result.parseExecs, "the parse ran tofu")
			require.NoError(t, result.runConfigErr)
			tc.check(t, result.cfg)
		})
	}
}

func TestQueueConfigLeavesOutBlockAttributesThatReadDependencyOutputs(t *testing.T) {
	t.Parallel()

	for name, tc := range blockDependencyCases() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cfg, err := resolveDependencyAttributeQueueConfig(t, "block-"+name, blockDependencyConfig(tc.body))
			require.NoError(t, err)
			require.NotNil(t, cfg)
			tc.checkQueue(t, cfg)
		})
	}
}

func TestPartialConfigReportsBlockAttributesThatReadDependencyOutputs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		body    string
		section config.PartialDecodeSectionType
	}{
		{
			name:    "remote_state backend",
			section: config.RemoteStateBlock,
			body: `
remote_state {
  backend = dependency.vpc.outputs.data
  config  = { path = "state.tfstate" }
}
`,
		},
		{
			name:    "remote_state disable_dependency_optimization",
			section: config.RemoteStateBlock,
			body: `
remote_state {
  backend                         = "local"
  disable_dependency_optimization = dependency.vpc.outputs.data == "data-from-state"
  config                          = { path = "state.tfstate" }
}
`,
		},
		{
			name:    "terraform include_in_copy",
			section: config.TerraformBlock,
			body: `
terraform {
  include_in_copy = [dependency.vpc.outputs.data]
}
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture-partial-" + tc.name)
			cfgPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
			v := venv.MemVenv().WithFS(memFS(root, map[string]string{
				"app/terragrunt.hcl": blockDependencyConfig(tc.body),
				"vpc/terragrunt.hcl": "",
			})(t))
			ctx, run := newTestUnitParsingContext(t, cfgPath)

			_, err := new(config.Store).ResolvePartialConfig(
				ctx,
				logger.CreateLogger(),
				v,
				run.WithDecodeList(tc.section),
				cfgPath,
			)

			require.ErrorAs(t, err, new(config.PartialParseReadsDependencyError))
		})
	}
}
