package helpers_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/version"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunTerragruntCommand(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	require.NoError(t, helpers.RunTerragruntCommand(t, "terragrunt --version", &stdout, &stderr))
	assert.Equal(t, "terragrunt version "+version.GetVersion()+"\n", stdout.String())
}

func TestRunTerragruntVersionCommand(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	require.NoError(t, helpers.RunTerragruntVersionCommand(t, "v9.9.9", "terragrunt --version", &stdout, &stderr))
	assert.Equal(t, "terragrunt version v9.9.9\n", stdout.String())
}

func TestRunTerragruntCommandWithContextReturnsError(t *testing.T) {
	t.Parallel()

	dir := pkgTInputsUnit(t, "")

	var stdout, stderr bytes.Buffer

	err := helpers.RunTerragruntCommandWithContext(
		t,
		t.Context(),
		"terragrunt hcl validate --inputs --non-interactive --working-dir "+dir,
		&stdout,
		&stderr,
	)
	require.Error(t, err, "a missing required input fails validation")
	assert.Contains(t, stderr.String(), "The following required inputs are missing")
}

func TestRunTerragruntCommandWithOutputLogFormat(t *testing.T) {
	t.Parallel()

	dir := pkgTInputsUnit(t, `input = "x"`)

	t.Run("key-value is added by default", func(t *testing.T) {
		t.Parallel()

		stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
			t,
			"terragrunt hcl validate --inputs --non-interactive --working-dir "+dir,
		)
		require.NoError(t, err)
		assert.Empty(t, stdout)
		assert.Contains(t, stderr, "level=info")
		assert.Contains(t, stderr, "All required inputs are passed in by terragrunt")
	})

	t.Run("an explicit log format is kept", func(t *testing.T) {
		t.Parallel()

		_, stderr, err := helpers.RunTerragruntCommandWithOutputWithContext(
			t,
			t.Context(),
			"terragrunt hcl validate --inputs --non-interactive --log-format=json --working-dir "+dir,
		)
		require.NoError(t, err)
		assert.Contains(t, stderr, `"level":"info"`)
		assert.NotContains(t, stderr, "level=info")
	})
}

func TestRunTerragruntCommandWithVenv(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	v := helpers.RunVenv(t).WithWriter(&stdout).WithErrWriter(&stderr)

	require.NoError(t, helpers.RunTerragruntCommandWithVenv(t, t.Context(), v, "terragrunt --version"))
	assert.Equal(t, "terragrunt version "+version.GetVersion()+"\n", stdout.String())
}

func TestRunTerragruntCommandWithOutputWithVenv(t *testing.T) {
	t.Parallel()

	stdout, _, err := helpers.RunTerragruntCommandWithOutputWithVenv(t, helpers.RunVenv(t), "terragrunt --version")
	require.NoError(t, err)
	assert.Equal(t, "terragrunt version "+version.GetVersion()+"\n", stdout)
}

func TestRunTerragruntRedirectOutput(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	helpers.RunTerragruntRedirectOutput(t, "terragrunt --version", &stdout, &stderr)
	assert.Equal(t, "terragrunt version "+version.GetVersion()+"\n", stdout.String())
}

func TestRunTerragrunt(t *testing.T) {
	t.Parallel()

	// Output goes to the process stdout; the helper fails the test on error.
	helpers.RunTerragrunt(t, "terragrunt --version")
}

func TestRunTerragruntCommandPassesArgsAfterDoubleDash(t *testing.T) {
	t.Parallel()

	pkgTRequireOnPath(t, helpers.WrappedBinary(t.Context()))

	dir := t.TempDir()
	pkgTWriteFiles(t, dir, map[string]string{"terragrunt.hcl": ""})

	// The log format flag goes before "--", so the wrapped binary receives only "version".
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --non-interactive --working-dir "+dir+" -- version",
	)
	require.NoError(t, err)
	assert.Regexp(t, `(OpenTofu|Terraform) v\d+\.\d+\.\d+`, stdout)
}

func TestRunTerragruntValidateInputs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		inputs    string
		extraArgs []string
		success   bool
	}{
		{
			name:    "all inputs set",
			inputs:  `input = "x"`,
			success: true,
		},
		{
			name:    "missing required input",
			inputs:  "",
			success: false,
		},
		{
			name:    "unused input is allowed by default",
			inputs:  "input = \"x\"\n  unused = \"y\"",
			success: true,
		},
		{
			name:      "unused input fails in strict mode",
			inputs:    "input = \"x\"\n  unused = \"y\"",
			extraArgs: []string{"--strict"},
			success:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.RunTerragruntValidateInputs(t, pkgTInputsUnit(t, tc.inputs), tc.extraArgs, tc.success)
		})
	}
}

func TestRunTerragruntValidateInputsRunsFromNestedModule(t *testing.T) {
	t.Parallel()

	// The outer unit lacks its input, so only a run from the nested "module" directory passes.
	dir := pkgTInputsUnit(t, "")
	pkgTWriteFiles(t, dir, map[string]string{
		filepath.Join("module", "main.tf"):        pkgTInputsModule,
		filepath.Join("module", "terragrunt.hcl"): "inputs = {\n  input = \"x\"\n}\n",
	})

	helpers.RunTerragruntValidateInputs(t, dir, nil, true)
}

func TestRunValidateAllWithIncludeAndGetIncludedModules(t *testing.T) {
	t.Parallel()

	pkgTRequireOnPath(t, helpers.WrappedBinary(t.Context()))

	root := pkgTStackFixture(t)

	got := helpers.RunValidateAllWithIncludeAndGetIncludedModules(t, root, []string{"app"})
	assert.Equal(t, []string{"app"}, pkgTToSlash(got))
}

func TestRunValidateAllWithFilteredPlusDependenciesAndGetIncludedModules(t *testing.T) {
	t.Parallel()

	pkgTRequireOnPath(t, helpers.WrappedBinary(t.Context()))

	testCases := []struct {
		name  string
		units []string
		want  []string
	}{
		{
			name: "no filter runs every unit",
			want: []string{"app", "db", "other"},
		},
		{
			name:  "filter pulls in dependencies",
			units: []string{"app"},
			want:  []string{"app", "db"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := pkgTStackFixture(t)

			got := helpers.RunValidateAllWithFilteredPlusDependenciesAndGetIncludedModules(t, root, tc.units)
			assert.Equal(t, tc.want, pkgTToSlash(got))
		})
	}
}

const pkgTInputsModule = "variable \"input\" {}\n"

// pkgTInputsUnit writes a unit whose module requires one input, using inputs as the inputs block body.
func pkgTInputsUnit(t *testing.T, inputs string) string {
	t.Helper()

	dir := t.TempDir()
	pkgTWriteFiles(t, dir, map[string]string{
		"main.tf":        pkgTInputsModule,
		"terragrunt.hcl": "inputs = {\n  " + inputs + "\n}\n",
	})

	return dir
}

// pkgTStackFixture writes three units without providers, app depending on db, and returns the stack root.
func pkgTStackFixture(t *testing.T) string {
	t.Helper()

	root := helpers.TmpDirWOSymlinks(t)
	files := map[string]string{
		filepath.Join("app", "terragrunt.hcl"):   "dependencies {\n  paths = [\"../db\"]\n}\n",
		filepath.Join("db", "terragrunt.hcl"):    "",
		filepath.Join("other", "terragrunt.hcl"): "",
	}

	for _, unit := range []string{"app", "db", "other"} {
		files[filepath.Join(unit, "main.tf")] = "output \"name\" {\n  value = \"" + unit + "\"\n}\n"
	}

	pkgTWriteFiles(t, root, files)

	return root
}

// pkgTToSlash converts every path in paths to forward slashes.
func pkgTToSlash(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		out = append(out, filepath.ToSlash(path))
	}

	return out
}
