package format_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCommandExposesTheFormatFlags pins the command name, alias and flags users invoke.
func TestNewCommandExposesTheFormatFlags(t *testing.T) {
	t.Parallel()

	cmd := format.NewCommand(
		logger.CreateLogger(), options.NewTerragruntOptions(vexec.NewOSExec()), venvtest.New(),
	)

	assert.Equal(t, format.CommandName, cmd.Name)
	assert.Equal(t, []string{format.CommandNameAlias}, cmd.Aliases)

	flagNames := []string{
		format.FileFlagName,
		format.ExcludeDirFlagName,
		format.CheckFlagName,
		format.DiffFlagName,
		format.StdinFlagName,
	}

	for _, name := range flagNames {
		assert.NotNil(t, cmd.Flags.Get(name), name)
	}
}

// TestNewCommandActionFormatsTheWorkingDir pins that the action formats with the options the command was built from.
func TestNewCommandActionFormatsTheWorkingDir(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")
	fsys := venvtest.NewFS(t, root, map[string]string{"terragrunt.hcl": formatUnformatted})

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.WorkingDir = root

	cmd := format.NewCommand(logger.CreateLogger(), opts, venvtest.New().WithFS(fsys))

	require.NoError(t, cmd.Action(t.Context(), &clihelper.Context{}))
	assert.Equal(t, formatFormatted, readFixture(t, fsys, root, "terragrunt.hcl"))
}

// TestNewFlagsMapsEachEnvVarToOneFlag pins every env var of the command's own flags to the single flag it sets.
func TestNewFlagsMapsEachEnvVarToOneFlag(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		envVar      string
		wantFile    string
		wantExclude []string
		wantCheck   bool
		wantDiff    bool
		wantStdin   bool
	}{
		{envVar: "TG_FILE", wantFile: "true"},
		{envVar: "TG_HCLFMT_FILE", wantFile: "true"},
		{envVar: "TERRAGRUNT_HCLFMT_FILE", wantFile: "true"},
		{envVar: "TG_EXCLUDE_DIR", wantExclude: []string{"true"}},
		{envVar: "TG_HCLFMT_EXCLUDE_DIR", wantExclude: []string{"true"}},
		{envVar: "TERRAGRUNT_HCLFMT_EXCLUDE_DIR", wantExclude: []string{"true"}},
		{envVar: "TG_CHECK", wantCheck: true},
		{envVar: "TG_HCLFMT_CHECK", wantCheck: true},
		{envVar: "TERRAGRUNT_CHECK", wantCheck: true},
		{envVar: "TG_DIFF", wantDiff: true},
		{envVar: "TG_HCLFMT_DIFF", wantDiff: true},
		{envVar: "TERRAGRUNT_DIFF", wantDiff: true},
		{envVar: "TG_STDIN", wantStdin: true},
		{envVar: "TG_HCLFMT_STDIN", wantStdin: true},
		{envVar: "TERRAGRUNT_HCLFMT_STDIN", wantStdin: true},
	}

	for _, tc := range testCases {
		t.Run(tc.envVar, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			flagSet := format.NewFlags(logger.CreateLogger(), opts, venvtest.New(), nil)

			require.NoError(t, flagSet.Parse(clihelper.Args{}, map[string]string{tc.envVar: "true"}))

			assert.Equal(t, tc.wantFile, opts.HclFile, format.FileFlagName)
			assert.Equal(t, tc.wantExclude, opts.HclExclude, format.ExcludeDirFlagName)
			assert.Equal(t, tc.wantCheck, opts.Check, format.CheckFlagName)
			assert.Equal(t, tc.wantDiff, opts.Diff, format.DiffFlagName)
			assert.Equal(t, tc.wantStdin, opts.HclFromStdin, format.StdinFlagName)
		})
	}
}
