package validate_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/validate"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
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
			"VarsWithQuotes",
			[]string{`-var='hello=world'`, `-var="foo=bar"`, `-var="'"enabled"'"=false`},
			[]string{"'enabled'", "foo", "hello"},
			[]string{},
		},
		{
			"VarFilesWithQuotes",
			[]string{`-var-file='terraform.tfvars'`, `-var-file="other_vars.tfvars"`},
			[]string{},
			[]string{"other_vars.tfvars", "terraform.tfvars"},
		},
		{
			"MixedWithOtherIrrelevantArgs",
			[]string{"-lock=true", "-var=enabled=true", "-refresh=false"},
			[]string{"enabled"},
			[]string{},
		},
		{
			"None",
			[]string{"-lock=true", "-refresh=false"},
			[]string{},
			[]string{},
		},
		{
			"SpaceInVarFileName",
			[]string{"-var-file='this is a test.tfvars'"},
			[]string{},
			[]string{"this is a test.tfvars"},
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

	v := venvtest.New().WithExec(exec).WithFS(vfs.NewOSFS())
	l := logger.CreateLogger()

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.AuthProviderCmd = "fake-auth-provider"

	err = validate.RunValidateInputs(t.Context(), l, v, opts)
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

	v := venvtest.New().WithFS(vfs.NewOSFS())
	l := logger.CreateLogger()

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
	require.NoError(t, err)

	err = validate.RunValidateInputs(t.Context(), l, v, opts)
	require.NoError(t, err)
}

func TestRunValidateInputsWithTosetDependencyOutputs(t *testing.T) {
	t.Parallel()

	fixtureDir, err := filepath.Abs("../../../../../test/fixtures/validate-inputs/success-toset-dependency/consumer")
	require.NoError(t, err)

	v := venvtest.New().WithFS(vfs.NewOSFS())
	l := logger.CreateLogger()

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(fixtureDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = fixtureDir

	err = validate.RunValidateInputs(t.Context(), l, v, opts)
	require.NoError(t, err)
}
