// Package find provides the ability to find Terragrunt configurations in your codebase
// via the `terragrunt find` command.
package find

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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
	Include        = "include"

	QueueConstructAsFlagName  = "queue-construct-as"
	QueueConstructAsFlagAlias = "as"
)

func NewFlags(opts *Options, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        FormatFlagName,
			EnvVars:     tgPrefix.EnvVars(FormatFlagName),
			Destination: &opts.Format,
			Usage:       "Output format for find results. Valid values: text, json.",
			DefaultText: FormatText,
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        JSONFlagName,
			EnvVars:     tgPrefix.EnvVars(JSONFlagName),
			Aliases:     []string{JSONFlagAlias},
			Destination: &opts.JSON,
			Usage:       "Output in JSON format (equivalent to --format=json).",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        DAGFlagName,
			EnvVars:     tgPrefix.EnvVars(DAGFlagName),
			Destination: &opts.DAG,
			Usage:       "Use DAG mode to sort and group output.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        HiddenFlagName,
			EnvVars:     tgPrefix.EnvVars(HiddenFlagName),
			Destination: &opts.Hidden,
			Usage:       "Include hidden directories in find results.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        Dependencies,
			EnvVars:     tgPrefix.EnvVars(Dependencies),
			Destination: &opts.Dependencies,
			Usage:       "Include dependencies in the results (only when using --format=json).",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        Exclude,
			EnvVars:     tgPrefix.EnvVars(Exclude),
			Destination: &opts.Exclude,
			Usage:       "Display exclude configurations in the results (only when using --format=json).",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        Include,
			EnvVars:     tgPrefix.EnvVars(Include),
			Destination: &opts.Include,
			Usage:       "Display include configurations in the results (only when using --format=json).",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        External,
			EnvVars:     tgPrefix.EnvVars(External),
			Destination: &opts.External,
			Usage:       "Discover external dependencies from initial results, and add them to top-level results.",
		}),
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        QueueConstructAsFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueConstructAsFlagName),
			Destination: &opts.QueueConstructAs,
			Usage:       "Construct the queue as if a specific command was run.",
			Aliases:     []string{QueueConstructAsFlagAlias},
		}),
	}
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	cmdOpts := NewOptions(opts)

	return &cli.Command{
		Name:    CommandName,
		Aliases: []string{CommandAlias},
		Usage:   "Find relevant Terragrunt configurations.",
		Flags:   NewFlags(cmdOpts, nil),
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
			return Run(ctx, l, cmdOpts)
		},
	}
}
