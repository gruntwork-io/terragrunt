package runnerpool_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const (
	// memRoot is the in-memory filesystem root every runner pool fixture lives under.
	memRoot = "/repo"

	// tfVersionOutput is what the fake exec reports to a Terraform version probe.
	tfVersionOutput = "OpenTofu v1.9.0"

	// invalidHCL fails config parsing, so a unit using it fails before any Terraform binary is needed.
	invalidHCL = "this is ) not ( valid hcl"
)

func TestCloneUnitOptions_WithStackOpts(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(memRoot, "terragrunt.hcl")

	stackOpts, err := options.NewTerragruntOptionsForTest(
		filepath.Join(memRoot, "stack", "terragrunt.hcl"),
	)
	require.NoError(t, err)

	unit := component.NewUnit(memRoot)
	l := thlogger.CreateLogger()

	opts, logger, err := runnerpool.CloneUnitOptions(stackOpts, unit, configPath, "", l)

	require.NoError(t, err)
	require.NotNil(t, opts)
	assert.NotNil(t, logger)
	assert.Equal(t, configPath, opts.OriginalTerragruntConfigPath)
	assert.NotEmpty(t, opts.DownloadDir)
}

func TestBuildUnitOpts_BasicUnit(t *testing.T) {
	t.Parallel()

	stackOpts, err := options.NewTerragruntOptionsForTest(filepath.Join(memRoot, "terragrunt.hcl"))
	require.NoError(t, err)

	unit := component.NewUnit(memRoot)
	l := thlogger.CreateLogger()

	unitOpts, unitLogger, err := runnerpool.BuildUnitOpts(l, stackOpts, unit)

	require.NoError(t, err)
	require.NotNil(t, unitOpts)
	assert.NotNil(t, unitLogger)
	assert.Contains(t, unitOpts.TerragruntConfigPath, "terragrunt.hcl")
}

func TestBuildUnitOpts_WithDiscoveryContext(t *testing.T) {
	t.Parallel()

	stackOpts, err := options.NewTerragruntOptionsForTest(filepath.Join(memRoot, "terragrunt.hcl"))
	require.NoError(t, err)

	unit := component.NewUnit(memRoot)
	unit.SetDiscoveryContext(&component.DiscoveryContext{
		Cmd:  tf.CommandNamePlan,
		Args: []string{"-input=false"},
	})

	l := thlogger.CreateLogger()

	unitOpts, _, err := runnerpool.BuildUnitOpts(l, stackOpts, unit)

	require.NoError(t, err)
	assert.Equal(
		t,
		tf.CommandNamePlan,
		unitOpts.TerraformCommand,
		"the discovery context command overrides the stack command",
	)
}

func TestBuildUnitOpts_WithSource(t *testing.T) {
	t.Parallel()

	moduleSource := "git::ssh://git@github.com/acme/modules.git//vpc"
	badSource := "no-slashes-here"

	testCases := []struct {
		assertErr      func(*testing.T, error)
		unitConfig     *config.TerragruntConfig
		name           string
		stackSource    string
		expectedSource string
	}{
		{
			name:        "no stack source",
			unitConfig:  &config.TerragruntConfig{},
			stackSource: "",
		},
		{
			name:           "unit has no parsed config inherits the stack source",
			stackSource:    "/local/modules",
			expectedSource: "/local/modules",
		},
		{
			name:           "unit has no terraform source inherits the stack source",
			unitConfig:     &config.TerragruntConfig{},
			stackSource:    "/local/modules",
			expectedSource: "/local/modules",
		},
		{
			name: "per-unit source is computed",
			unitConfig: &config.TerragruntConfig{
				Terraform: &config.TerraformConfig{Source: &moduleSource},
			},
			stackSource:    "/local/modules",
			expectedSource: "/local/modules//vpc",
		},
		{
			name: "unparsable source errors",
			unitConfig: &config.TerragruntConfig{
				Terraform: &config.TerraformConfig{Source: &badSource},
			},
			stackSource: "/local/modules",
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var target config.ParsingModulePathError

				require.ErrorAs(t, err, &target)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stackOpts, err := options.NewTerragruntOptionsForTest(
				filepath.Join(memRoot, "terragrunt.hcl"),
			)
			require.NoError(t, err)

			stackOpts.Source = tc.stackSource

			unit := component.NewUnit(memRoot)
			if tc.unitConfig != nil {
				unit = unit.WithConfig(tc.unitConfig)
			}

			unitOpts, _, err := runnerpool.BuildUnitOpts(thlogger.CreateLogger(), stackOpts, unit)

			if tc.assertErr != nil {
				tc.assertErr(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expectedSource, unitOpts.Source, "the unit source is derived from the stack source")
		})
	}
}

func TestBuildUnitOpts_UnitPathIsAConfigFile(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(memRoot, "terragrunt.hcl")

	stackOpts, err := options.NewTerragruntOptionsForTest(configPath)
	require.NoError(t, err)

	unitOpts, _, err := runnerpool.BuildUnitOpts(
		thlogger.CreateLogger(),
		stackOpts,
		component.NewUnit(configPath),
	)
	require.NoError(t, err)
	assert.Equal(t, configPath, unitOpts.TerragruntConfigPath)
}

func TestCloneUnitOptions_WithCustomDownloadDir(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(memRoot, "terragrunt.hcl")

	stackOpts, err := options.NewTerragruntOptionsForTest(
		filepath.Join(memRoot, "stack", "terragrunt.hcl"),
	)
	require.NoError(t, err)

	stackOpts.DownloadDir = "/custom/download"

	unit := component.NewUnit(memRoot)
	l := thlogger.CreateLogger()

	opts, _, err := runnerpool.CloneUnitOptions(stackOpts, unit, configPath, "", l)

	require.NoError(t, err)
	assert.Equal(t, "/custom/download", opts.DownloadDir)
}

// memVenv returns an in-memory venv answering every invocation with output on stdout, so nothing is spawned.
func memVenv(output string) *venv.Venv {
	return venvtest.New().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte(output + "\n")}
	})
}

// writeUnit writes a unit directory holding body as its terragrunt.hcl into the in-memory filesystem of v.
func writeUnit(t *testing.T, v *venv.Venv, root, name, body string) string {
	t.Helper()

	dir := filepath.Join(root, name)
	require.NoError(t, v.FS.MkdirAll(dir, 0o755))
	require.NoError(
		t,
		vfs.WriteFile(v.FS, filepath.Join(dir, "terragrunt.hcl"), []byte(body), 0o644),
	)
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(dir, "main.tf"), []byte(""), 0o644))

	return dir
}

// newStackOpts returns stack options rooted at root for the given Terraform command.
func newStackOpts(t *testing.T, root, command string) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(root, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = root
	opts.RootWorkingDir = root
	opts.TerraformCommand = command
	opts.TerraformCliArgs = iacargs.New(command)

	return opts
}
