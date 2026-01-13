package commands

import (
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
)

var (
	shortcutCommandNames = []string{
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
)

func NewShortcutsCommands(l log.Logger, opts *options.TerragruntOptions) clihelper.Commands {
	var (
		runCmd = run.NewCommand(l, opts)
		cmds   = make(clihelper.Commands, 0, len(runCmd.Subcommands))
	)

	for _, runSubCmd := range runCmd.Subcommands {
		if isNotShortcutCmd := !slices.Contains(shortcutCommandNames, runSubCmd.Name); isNotShortcutCmd {
			continue
		}

		cmd := &clihelper.Command{
			Name:                         runSubCmd.Name,
			Usage:                        runSubCmd.Usage,
			Flags:                        runCmd.Flags,
			CustomHelp:                   runSubCmd.CustomHelp,
			Action:                       runSubCmd.Action,
			DisabledErrorOnUndefinedFlag: true,
		}

		cmds = append(cmds, cmd)
	}

	return cmds
}
