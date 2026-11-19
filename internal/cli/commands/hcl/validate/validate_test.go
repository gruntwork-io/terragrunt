package validate_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/validate"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetVarFlagsFromExtraArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		args             []string
		expectedVars     []string
		expectedVarFiles []string
	}{
		{
			name:             "QuotesStayPartOfTheArgument",
			args:             []string{`-var='hello=world'`, `-var-file="other_vars.tfvars"`},
			expectedVars:     []string{"'hello"},
			expectedVarFiles: []string{`"other_vars.tfvars"`},
		},
		{
			name:             "BackslashesInVarFileName",
			args:             []string{`-var-file=C:\work\unit\varfiles\main.tfvars`},
			expectedVars:     []string{},
			expectedVarFiles: []string{`C:\work\unit\varfiles\main.tfvars`},
		},
		{
			name:             "MixedWithOtherIrrelevantArgs",
			args:             []string{"-lock=true", "-var=enabled=true", "-refresh=false"},
			expectedVars:     []string{"enabled"},
			expectedVarFiles: []string{},
		},
		{
			name:             "None",
			args:             []string{"-lock=true", "-refresh=false"},
			expectedVars:     []string{},
			expectedVarFiles: []string{},
		},
		{
			name:             "SpaceInVarFileName",
			args:             []string{"-var-file=this is a test.tfvars"},
			expectedVars:     []string{},
			expectedVarFiles: []string{"this is a test.tfvars"},
		},
		{
			name:             "ValueInNextArgument",
			args:             []string{"-var", "enabled=true", "-var-file", "main.tfvars"},
			expectedVars:     []string{"enabled"},
			expectedVarFiles: []string{"main.tfvars"},
		},
		{
			name:             "DoubleDash",
			args:             []string{"--var=enabled=true", "--var-file=main.tfvars"},
			expectedVars:     []string{"enabled"},
			expectedVarFiles: []string{"main.tfvars"},
		},
		{
			name:             "DoubleDashWithValueInNextArgument",
			args:             []string{"--var", "enabled=true", "--var-file", "main.tfvars"},
			expectedVars:     []string{"enabled"},
			expectedVarFiles: []string{"main.tfvars"},
		},
		{
			name:             "EqualsSignInVarValue",
			args:             []string{"-var=first=a=b", "-var", "second=a=b"},
			expectedVars:     []string{"first", "second"},
			expectedVarFiles: []string{},
		},
		{
			name:             "NextArgumentIsTheValueEvenWhenItLooksLikeAFlag",
			args:             []string{"-var-file", "-var=enabled=true", "-lock=true"},
			expectedVars:     []string{},
			expectedVarFiles: []string{"-var=enabled=true"},
		},
		{
			name: "FlagsWithAVarPrefix",
			args: []string{
				"-variable=enabled=true",
				"-var-files=main.tfvars",
				"---var=enabled=true",
				"var=enabled=true",
			},
			expectedVars:     []string{},
			expectedVarFiles: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			vars, varFiles, err := validate.GetVarFlagsFromArgList(tc.args)
			require.NoError(t, err)
			sort.Strings(vars)
			sort.Strings(varFiles)
			assert.Equal(t, tc.expectedVars, vars)
			assert.Equal(t, tc.expectedVarFiles, varFiles)
		})
	}
}

func TestGetVarFlagsFromArgListRejectsFlagWithoutValue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		expectedFlag string
		args         []string
	}{
		{name: "Var", expectedFlag: "var", args: []string{"-lock=true", "-var"}},
		{name: "VarFile", expectedFlag: "var-file", args: []string{"-var=enabled=true", "-var-file"}},
		{name: "DoubleDashVar", expectedFlag: "var", args: []string{"--var"}},
		{name: "DoubleDashVarFile", expectedFlag: "var-file", args: []string{"--var-file"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := validate.GetVarFlagsFromArgList(tc.args)

			var missingValueErr validate.MissingVarFlagValueError

			require.ErrorAs(t, err, &missingValueErr)
			assert.Equal(t, tc.expectedFlag, missingValueErr.Flag)
		})
	}
}

func TestGetVarFlagsFromArgListRejectsVarWithoutEqualsSign(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
	}{
		{name: "ValueAfterEqualsSign", args: []string{"-var=enabled"}},
		{name: "ValueInNextArgument", args: []string{"-var", "enabled"}},
		{name: "EmptyValue", args: []string{"-var="}},
		{name: "NextArgumentIsAnotherFlag", args: []string{"-var", "-refresh"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := validate.GetVarFlagsFromArgList(tc.args)
			require.ErrorIs(t, err, validate.ErrMalformedVarFlag)
		})
	}
}

func TestRunValidateInputsCountsVarFlagsWithValueInNextArgument(t *testing.T) {
	t.Parallel()

	result := runValidate(t, &validateRun{
		files: map[string]string{
			"main.tf": `variable "region" {}
variable "team" {}
`,
			"extra.tfvars": `team = "platform"` + "\n",
			"terragrunt.hcl": `terraform {
  extra_arguments "vars" {
    commands  = ["apply", "plan"]
    arguments = ["-var", "region=us-east-1", "-var-file", "${get_terragrunt_dir()}/extra.tfvars"]
  }
}
`,
		},
		inputs: true,
	})

	require.NoError(t, result.err)
}

func TestRunValidateInputsReadsRelativeVarFileFromUnitDir(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		arguments string
	}{
		{name: "ValueInNextArgument", arguments: `["-var-file", "common.tfvars"]`},
		{name: "ValueAfterEqualsSign", arguments: `["-var-file=common.tfvars"]`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runValidate(t, &validateRun{
				files: map[string]string{
					"live/unit/main.tf":       `variable "env" {}` + "\n",
					"live/unit/common.tfvars": `env = "prod"` + "\n",
					"live/unit/terragrunt.hcl": `terraform {
  extra_arguments "vars" {
    commands  = ["apply", "plan"]
    arguments = ` + tc.arguments + `
  }
}
`,
				},
				inputs: true,
			})

			require.NoError(t, result.err)
		})
	}
}

func TestRunValidateInputsCountsVarFlagWithValueInNextExtraArgumentsBlock(t *testing.T) {
	t.Parallel()

	result := runValidate(t, &validateRun{
		files: map[string]string{
			"main.tf": `variable "region" {}` + "\n",
			"terragrunt.hcl": `terraform {
  extra_arguments "flag" {
    commands  = ["apply", "plan"]
    arguments = ["-var"]
  }

  extra_arguments "value" {
    commands  = ["apply", "plan"]
    arguments = ["region=us-east-1"]
  }
}
`,
		},
		inputs: true,
	})

	require.NoError(t, result.err)
}

func TestRunValidateInputsCountsCLIVarFlagWithValueInNextArgument(t *testing.T) {
	t.Parallel()

	result := runValidate(t, &validateRun{
		files: map[string]string{
			"main.tf":        `variable "region" {}` + "\n",
			"terragrunt.hcl": "",
		},
		tfCLIArgs: []string{"-var", "region=us-east-1"},
		inputs:    true,
	})

	require.NoError(t, result.err)
}

// writeValidateInputsUnits lays out two units: unit-a satisfies its required
// `region` input through an extra_arguments env_vars TF_VAR_region, while
// unit-b requires `region` and never sets it (unless bInputs provides it).
func writeValidateInputsUnits(t *testing.T, bInputs string) string {
	t.Helper()

	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	moduleTF := `variable "region" {}` + "\n"
	unitACfg := `terraform {
  extra_arguments "set_region" {
    commands = ["apply", "plan"]
    env_vars = {
      TF_VAR_region = "us-east-1"
    }
  }
}
`

	for unit, files := range map[string]map[string]string{
		"unit-a": {"terragrunt.hcl": unitACfg, "main.tf": moduleTF},
		"unit-b": {"terragrunt.hcl": bInputs, "main.tf": moduleTF},
	} {
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, unit), 0755))

		for name, content := range files {
			require.NoError(
				t,
				os.WriteFile(filepath.Join(tmpDir, unit, name), []byte(content), 0644),
			)
		}
	}

	return tmpDir
}

// TestRunValidateInputsDoesNotLeakEnvBetweenUnits pins per-unit env isolation:
// unit-a's TF_VAR_region from extra_arguments must not satisfy unit-b's
// required `region` input, and credentials obtained from the auth provider
// during per-unit preparation must not escape into the shared environment.
func TestRunValidateInputsDoesNotLeakEnvBetweenUnits(t *testing.T) {
	t.Parallel()

	tmpDir := writeValidateInputsUnits(t, "")

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if inv.Name == "fake-auth-provider" {
			return vexec.Result{Stdout: []byte(`{"envs":{"AWS_ACCESS_KEY_ID":"fake-key"}}`)}
		}

		return vexec.Result{ExitCode: 127, Stderr: []byte("unexpected invocation: " + inv.Name)}
	})

	v := venv.MemVenv().WithExec(exec).WithFS(vfs.NewOSFS())
	l := logger.CreateLogger()

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.AuthProviderCmd = "fake-auth-provider"

	err = validate.RunValidateInputs(config.WithCaches(t.Context()), l, v, new(rewrite.Store), opts)
	require.Error(
		t,
		err,
		"unit-b never sets region, so validation must fail even after unit-a merged TF_VAR_region into its own env",
	)

	assert.NotContains(t, v.Env, "TF_VAR_region")
	assert.NotContains(t, v.Env, "AWS_ACCESS_KEY_ID")
}

// TestRunValidateInputsPassesWhenInputsDefined is the companion control: with
// unit-b defining its own region input, the same setup validates cleanly, so
// the error in the leak test above is unit-b's missing input rather than a
// scaffolding failure.
func TestRunValidateInputsPassesWhenInputsDefined(t *testing.T) {
	t.Parallel()

	tmpDir := writeValidateInputsUnits(t, "inputs = {\n  region = \"eu-west-1\"\n}\n")

	v := venv.MemVenv().WithFS(vfs.NewOSFS())
	l := logger.CreateLogger()

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
	require.NoError(t, err)

	err = validate.RunValidateInputs(config.WithCaches(t.Context()), l, v, new(rewrite.Store), opts)
	require.NoError(t, err)
}

// TestRunValidateReportsOneParserDiagnosticPerRange expects one diagnostic for a parser cascade at one source range.
func TestRunValidateReportsOneParserDiagnosticPerRange(t *testing.T) {
	t.Parallel()

	result := runValidate(t, &validateRun{files: map[string]string{
		"app/terragrunt.hcl": "locals {\n  t =\n}\n",
	}})

	require.Error(t, result.err)
	require.Len(t, result.diags, 1)
	assert.Equal(t, "Invalid expression", result.diags[0].Summary)
	require.NotNil(t, result.diags[0].Range)
	assert.Equal(t, 2, result.diags[0].Range.Start.Line)
}

func TestRunValidateReportsValuesKeysThatAreNotIdentifiers(t *testing.T) {
	t.Parallel()

	result := runValidate(t, &validateRun{files: map[string]string{
		"live/terragrunt.stack.hcl": `unit "app" {
  source = "../app"
  path   = "app"
  values = {
    "a b" = "space"
  }
}
`,
		"app/terragrunt.hcl": "",
	}})

	require.Error(t, result.err)
	require.Len(t, result.diags, 1)
	assert.Equal(t, "Invalid values key", result.diags[0].Summary)
	require.NotNil(t, result.diags[0].Range)
	assert.Equal(t, 5, result.diags[0].Range.Start.Line)
}

// TestRunValidateReportsVersionAttributeReferencingDependency pins that a version attribute that references a
// dependency output reports one diagnostic pointing at the reference.
func TestRunValidateReportsVersionAttributeReferencingDependency(t *testing.T) {
	t.Parallel()

	result := runValidate(t, &validateRun{files: map[string]string{
		"dep/terragrunt.hcl": "",
		"app/terragrunt.hcl": `dependency "dep" {
  config_path = "../dep"
}

terraform_binary = dependency.dep.outputs.binary
`,
	}})

	require.Error(t, result.err)
	require.Len(t, result.diags, 1)
	assert.Equal(t, "Dependency reference not allowed", result.diags[0].Summary)
	require.NotNil(t, result.diags[0].Range)
	assert.Equal(t, filepath.Join(result.root, "app", "terragrunt.hcl"), result.diags[0].Range.Filename)
	assert.Equal(t, 5, result.diags[0].Range.Start.Line)
	assert.Equal(t, 20, result.diags[0].Range.Start.Column)
	assert.Equal(t, 49, result.diags[0].Range.End.Column)
}
