package commands_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewGroupsCommandsByCategory pins which top-level commands each help category lists.
func TestNewGroupsCommandsByCategory(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	cmds := commands.New(logger.CreateLogger(), opts, venv.MemVenv())

	testCases := []struct {
		category string
		commands []string
	}{
		{category: commands.MainCommandsCategoryName, commands: []string{"run", "stack", "exec", "backend"}},
		{category: commands.CatalogCommandsCategoryName, commands: []string{"catalog", "scaffold"}},
		{category: commands.PortalCommandsCategoryName, commands: []string{"login"}},
		{category: commands.DiscoveryCommandsCategoryName, commands: []string{"find", "list", "browse"}},
		{
			category: commands.ConfigurationCommandsCategoryName,
			commands: []string{"hcl", "info", "dag", "render", "mcp", "help", "version", "aws-provider-patch"},
		},
		{category: commands.ShortcutsCommandsCategoryName, commands: []string{tf.CommandNamePlan, tf.CommandNameApply}},
	}

	for _, tc := range testCases {
		t.Run(tc.category, func(t *testing.T) {
			t.Parallel()

			for _, name := range tc.commands {
				cmd := cmds.Get(name)
				require.NotNil(t, cmd, "command %s should be registered", name)
				require.NotNil(t, cmd.Category, "command %s should have a category", name)
				assert.Equal(t, tc.category, cmd.Category.Name, "command %s should be listed under %s", name, tc.category)
			}
		})
	}
}

// TestNewShortcutsCommandsMirrorRunSubcommands pins that each OpenTofu shortcut reuses the run subcommand it stands for.
func TestNewShortcutsCommandsMirrorRunSubcommands(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	v := venv.MemVenv()

	shortcuts := commands.NewShortcutsCommands(l, opts, v)
	runCmd := run.NewCommand(l, opts, v)

	wantNames := []string{
		tf.CommandNameInit,
		tf.CommandNameValidate,
		tf.CommandNamePlan,
		tf.CommandNameApply,
		tf.CommandNameDestroy,
		tf.CommandNameForceUnlock,
		tf.CommandNameImport,
		tf.CommandNameOutput,
		tf.CommandNameRefresh,
		tf.CommandNameShow,
		tf.CommandNameState,
		tf.CommandNameTest,
	}
	assert.ElementsMatch(t, wantNames, shortcuts.Names(), "only the curated OpenTofu commands should get shortcuts")
	require.Less(t, len(shortcuts), len(runCmd.Subcommands), "run should have subcommands without a shortcut")

	for _, shortcut := range shortcuts {
		runSubCmd := runCmd.Subcommands.Get(shortcut.Name)
		require.NotNil(t, runSubCmd, "shortcut %s should mirror a run subcommand", shortcut.Name)
		assert.Equal(t, runSubCmd.Usage, shortcut.Usage, "shortcut %s should share run's usage", shortcut.Name)
		assert.Equal(t, runCmd.Flags.Names(), shortcut.Flags.Names(), "shortcut %s should accept run's flags", shortcut.Name)
		assert.True(
			t,
			shortcut.DisabledErrorOnUndefinedFlag,
			"shortcut %s should pass unknown flags to OpenTofu",
			shortcut.Name,
		)
	}
}
