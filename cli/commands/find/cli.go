// Package find provides the ability to find Terragrunt configurations in your codebase
// via the `terragrunt find` command.
package find

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName  = "find"
	CommandAlias = "fd"

	FormatFlagName = "format"

	JSONFlagName  = "json"
	JSONFlagAlias = "j"

	DAGFlagName = "dag"

	HiddenFlagName = "hidden"
	Dependencies   = "dependencies"
	External       = "external"
	Exclude        = "exclude"

	QueueConstructAsFlagName  = "queue-construct-as"
	QueueConstructAsFlagAlias = "as"
)

func NewFlags(opts *Options, cmdPrefix flags.Prefix) cli.Flags {
	return cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        FormatFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(FormatFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(FormatFlagName),
			Destination: &opts.Format,
			Usage:       "Output format for find results. Valid values: text, json.",
			DefaultText: FormatText,
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        JSONFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(JSONFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(JSONFlagName),
			Aliases:     []string{JSONFlagAlias},
			Destination: &opts.JSON,
			Usage:       "Output in JSON format (equivalent to --format=json).",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        DAGFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(DAGFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(DAGFlagName),
			Destination: &opts.DAG,
			Usage:       "Use DAG mode to sort and group output.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        HiddenFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(HiddenFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(HiddenFlagName),
			Destination: &opts.Hidden,
			Usage:       "Include hidden directories in find results.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        Dependencies,
			EnvVars:     flags.EnvVarsWithTgPrefix(Dependencies),
			ConfigKey:   cmdPrefix.ConfigKey(Dependencies),
			Destination: &opts.Dependencies,
			Usage:       "Include dependencies in the results (only when using --format=json).",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        Exclude,
			EnvVars:     flags.EnvVarsWithTgPrefix(Exclude),
			ConfigKey:   cmdPrefix.ConfigKey(Exclude),
			Destination: &opts.Exclude,
			Usage:       "Display exclude configurations in the results (only when using --format=json).",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        External,
			EnvVars:     flags.EnvVarsWithTgPrefix(External),
			ConfigKey:   cmdPrefix.ConfigKey(External),
			Destination: &opts.External,
			Usage:       "Discover external dependencies from initial results, and add them to top-level results.",
		}),
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        QueueConstructAsFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(QueueConstructAsFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(QueueConstructAsFlagName),
			Destination: &opts.QueueConstructAs,
			Usage:       "Construct the queue as if a specific command was run.",
			Aliases:     []string{QueueConstructAsFlagAlias},
		}),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmdOpts := NewOptions(opts)
	cmdPrefix := flags.Prefix{CommandName}

	return &cli.Command{
		Name:    CommandName,
		Aliases: []string{CommandAlias},
		Usage:   "Find relevant Terragrunt configurations.",
		Flags:   NewFlags(cmdOpts, cmdPrefix),
		Before: func(ctx *cli.Context) error {
			if cmdOpts.JSON {
				cmdOpts.Format = FormatJSON
			}

			if cmdOpts.DAG {
				cmdOpts.Mode = ModeDAG
			}

			// Requesting a specific command to be used for queue construction
			// implies DAG mode.
			if cmdOpts.QueueConstructAs != "" {
				cmdOpts.Mode = ModeDAG
			}

			if err := cmdOpts.Validate(); err != nil {
				return cli.NewExitError(err, cli.ExitCodeGeneralError)
			}

			return nil
		},
		Action: func(ctx *cli.Context) error {
			return Run(ctx, cmdOpts)
		},
	}
}
