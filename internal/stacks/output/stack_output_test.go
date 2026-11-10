package output_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/stacks/output"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const vpcStack = `
unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"
}
`

func TestStackOutput_WithoutUnitsToRead(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files map[string]string
		want  cty.Value
		name  string
	}{
		{
			name:  "no stack files",
			files: map[string]string{"README.md": ""},
			want:  cty.NilVal,
		},
		{
			name: "only disabled units",
			files: map[string]string{config.DefaultStackFile: `
unit "vpc" {
  source  = "../units/vpc"
  path    = "vpc"
  enabled = false
}
`},
			want: cty.EmptyObjectVal,
		},
		{
			name: "only stacks",
			files: map[string]string{config.DefaultStackFile: `
stack "network" {
  source = "../stacks/network"
  path   = "network"
}

stack "legacy" {
  source  = "../stacks/legacy"
  path    = "legacy"
  enabled = false
}
`},
			want: cty.EmptyObjectVal,
		},
		{
			name: "excluded generated unit",
			files: map[string]string{
				config.DefaultStackFile: vpcStack,
				filepath.Join(config.StackDir, "vpc", config.DefaultTerragruntConfigPath): `
exclude {
  if      = true
  actions = ["all"]
}
`,
			},
			want: cty.EmptyObjectVal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := runStackOutput(t, tc.files, nil)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got, "nothing should be read from units that were not declared or were excluded")
		})
	}
}

func TestStackOutput_UnitWithoutGeneratedConfigFails(t *testing.T) {
	t.Parallel()

	_, err := runStackOutput(t, map[string]string{config.DefaultStackFile: vpcStack}, nil)

	var unitErr output.UnitOutputError

	require.ErrorAs(t, err, &unitErr, "a unit whose outputs cannot be read should be named in the error")
	assert.Equal(t, "vpc", unitErr.UnitName)
	assert.Equal(t, "vpc", filepath.Base(unitErr.UnitDir))
	assert.ErrorAs(t, err, new(config.TerragruntConfigNotFoundError), "the cause should be kept")
}

func TestStackOutput_InvalidConfigFails(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files map[string]string
		name  string
	}{
		{
			name:  "invalid stack file",
			files: map[string]string{config.DefaultStackFile: `unit "vpc" {`},
		},
		{
			name: "invalid values file",
			files: map[string]string{
				config.DefaultStackFile: vpcStack,
				"terragrunt.values.hcl": "x = (",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := runStackOutput(t, tc.files, nil)

			require.Error(t, err)
			assert.Equal(t, cty.NilVal, got)
			assert.NotErrorAs(t, err, new(output.UnitOutputError), "no unit should be read when parsing fails")
		})
	}
}

func TestStackOutput_GitFilterNeedsGit(t *testing.T) {
	t.Parallel()

	gitFilter, err := filter.Parse("[main...HEAD]")
	require.NoError(t, err)

	_, err = runStackOutput(t, map[string]string{config.DefaultStackFile: vpcStack}, filter.Filters{gitFilter})

	require.ErrorIs(t, err, vexec.ErrNoSpawn, "a git filter should build worktrees with git before reading outputs")
}

func TestUnitOutputError_WrapsCause(t *testing.T) {
	t.Parallel()

	errCause := errors.New("output failed")
	err := output.UnitOutputError{Err: errCause, UnitName: "vpc", UnitDir: "/stack/.terragrunt-stack/vpc"}

	require.ErrorIs(t, err, errCause)
	assert.Contains(t, err.Error(), err.UnitName, "the message should name the unit")
	assert.Contains(t, err.Error(), err.UnitDir, "the message should name the unit directory")
	assert.Contains(t, err.Error(), errCause.Error(), "the message should include the cause")
}

func runStackOutput(t *testing.T, files map[string]string, filters filter.Filters) (cty.Value, error) {
	t.Helper()

	root := venvtest.Root("/repo")
	v := venv.MemVenv().WithFS(venvtest.NewFS(t, root, files))

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(root, config.DefaultTerragruntConfigPath))
	require.NoError(t, err)

	opts.WorkingDir = root
	opts.Filters = filters

	return output.StackOutput(config.WithCaches(t.Context()), logger.CreateLogger(), v, opts)
}
