// Package stack provides the command to stack.
package stack

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// CommandName stack command name.
	CommandName = "stack"
	// OutputFormatFlagName is the name of the flag that selects the format of
	// `terragrunt stack output`: json or raw.
	OutputFormatFlagName = "format"
	// JSONFormatFlagName is the name of the flag that is shorthand for
	// --format=json on `terragrunt stack output`.
	JSONFormatFlagName = "json"
	// RawFormatFlagName is the name of the flag that is shorthand for
	// --format=raw on `terragrunt stack output`.
	RawFormatFlagName = "raw"
	// NoStackValidate is the name of the hidden flag that skips validating a
	// stack after generating it.
	NoStackValidate = "no-stack-validate"

	generateCommandName = "generate"
	runCommandName      = "run"
	outputCommandName   = "output"
	cleanCommandName    = "clean"

	rawOutputFormat  = "raw"
	jsonOutputFormat = "json"
)

// NewCommand builds the command for stack.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv, s *rewrite.Store) *clihelper.Command {
	return &clihelper.Command{
		Name:  CommandName,
		Usage: "Terragrunt stack commands.",
		Subcommands: clihelper.Commands{
			&clihelper.Command{
				Name:  generateCommandName,
				Usage: "Generate a stack from a terragrunt.stack.hcl file",
				Action: func(ctx context.Context, _ *clihelper.Context) error {
					return RunGenerate(ctx, l, v, s, opts.OptionsFromContext(ctx))
				},
				Flags: NewGenerateFlags(l, opts, v),
			},
			&clihelper.Command{
				Name:  runCommandName,
				Usage: "Run a command on the stack generated from the current directory",
				Action: func(ctx context.Context, _ *clihelper.Context) error {
					return Run(ctx, l, v, s, opts.OptionsFromContext(ctx))
				},
				Flags: NewRunFlags(l, opts, v),
			},
			&clihelper.Command{
				Name:  outputCommandName,
				Usage: "Run fetch stack output",
				Action: func(ctx context.Context, cliCtx *clihelper.Context) error {
					index := ""
					if val := cliCtx.Args().Get(0); val != "" {
						index = val
					}

					return RunOutput(ctx, l, v, s, opts.OptionsFromContext(ctx), index)
				},
				Flags: NewOutputFlags(l, opts, v),
			},
			&clihelper.Command{
				Name:  cleanCommandName,
				Usage: "Clean the stack generated from the current directory",
				Action: func(ctx context.Context, _ *clihelper.Context) error {
					return RunClean(ctx, l, v, opts.OptionsFromContext(ctx))
				},
			},
		},
		Action: clihelper.ShowCommandHelp,
	}
}

// NewGenerateFlags returns the flags of `terragrunt stack generate`, bound to
// opts: the ones that reach stack config parsing and the source download, and
// the ones that select stacks.
func NewGenerateFlags(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) clihelper.Flags {
	cmdFlags := clihelper.Flags{
		newNoStackValidateFlag(opts),
		shared.NewNoStackGenerateFlag(opts),
		shared.NewSourceUpdateFlag(opts),
		shared.NewParallelismFlag(opts),
	}

	cmdFlags = cmdFlags.Add(newConfigParsingFlags(opts)...)
	cmdFlags = cmdFlags.Add(shared.NewSelectionFlags(l, opts, v)...)

	return cmdFlags.Sort()
}

// NewRunFlags returns the flags of `terragrunt stack run`, bound to opts. They
// are the flags of `terragrunt run --all`, which it ends in, and the ones that
// control generating the stack first.
func NewRunFlags(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) clihelper.Flags {
	cmdFlags := clihelper.Flags{
		newNoStackValidateFlag(opts),
		shared.NewNoStackGenerateFlag(opts),
		shared.NewNoAutoRetryFlag(opts),
		shared.NewNoAutoApproveFlag(opts),
		shared.NewNoRunHooksFlag(opts),
		shared.NewSourceUpdateFlag(opts),
		shared.NewInputsDebugFlag(opts, nil),
		shared.NewDisableCommandValidationFlag(opts),
		shared.NewFailFastFlag(opts),
		shared.NewParallelismFlag(opts),
	}

	cmdFlags = cmdFlags.Add(newConfigParsingFlags(opts)...)
	cmdFlags = cmdFlags.Add(shared.NewOutDirFlags(opts)...)
	cmdFlags = cmdFlags.Add(shared.NewDestroyDependenciesCheckFlags(opts)...)
	cmdFlags = cmdFlags.Add(shared.NewReportFlags(opts)...)
	cmdFlags = cmdFlags.Add(shared.NewQueueRunFlags(opts, nil)...)
	cmdFlags = cmdFlags.Add(shared.NewSelectionFlags(l, opts, v)...)

	return cmdFlags.Sort()
}

// NewOutputFlags returns the flags of `terragrunt stack output`, bound to
// opts: the ones that set the output format, the ones that reach config
// parsing, and the ones that select stacks.
func NewOutputFlags(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) clihelper.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}

	cmdFlags := clihelper.Flags{
		newNoStackValidateFlag(opts),
		shared.NewParallelismFlag(opts),
	}

	cmdFlags = cmdFlags.Add(newConfigParsingFlags(opts)...)
	cmdFlags = cmdFlags.Add(shared.NewSelectionFlags(l, opts, v)...)

	return append(
		cmdFlags.Sort(),
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        OutputFormatFlagName,
			EnvVars:     tgPrefix.EnvVars(OutputFormatFlagName),
			Destination: &opts.StackOutputFormat,
			Usage:       "Stack output format. Valid values are: json, raw",
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:  RawFormatFlagName,
			Usage: "Stack output in raw format",
			Action: func(_ context.Context, _ *clihelper.Context, value bool) error {
				opts.StackOutputFormat = rawOutputFormat
				return nil
			},
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:  JSONFormatFlagName,
			Usage: "Stack output in json format",
			Action: func(_ context.Context, _ *clihelper.Context, value bool) error {
				opts.StackOutputFormat = jsonOutputFormat
				return nil
			},
		}),
	)
}

func newNoStackValidateFlag(opts *options.TerragruntOptions) *flags.Flag {
	tgPrefix := flags.Prefix{flags.TgPrefix}

	return flags.NewFlag(&clihelper.BoolFlag{
		Name:        NoStackValidate,
		EnvVars:     tgPrefix.EnvVars(NoStackValidate),
		Destination: &opts.NoStackValidate,
		Hidden:      true,
		Usage:       "Disable automatic stack validation after generation.",
	})
}

// newConfigParsingFlags returns the shared flags that reach config parsing,
// which every stack command with flags does: stack files call HCL functions,
// and unit configs resolve dependency outputs.
func newConfigParsingFlags(opts *options.TerragruntOptions) clihelper.Flags {
	parsingFlags := clihelper.Flags{
		shared.NewConfigFlag(opts, nil),
		shared.NewDownloadDirFlag(opts, nil),
		shared.NewTFPathFlag(opts),
		shared.NewTFForwardStdoutFlag(opts),
		shared.NewVersionManagerFileNameFlag(opts),
		shared.NewNoAutoInitFlag(opts, nil),
		shared.NewSourceFlag(opts, nil),
		shared.NewSourceMapFlag(opts, nil),
		shared.NewUsePartialParseConfigCacheFlag(opts),
		shared.NewAuthProviderCmdFlag(opts, nil),
		shared.NewNoDiscoveryAuthProviderCmdFlag(opts, nil),
	}

	parsingFlags = parsingFlags.Add(shared.NewDependencyOutputFlags(opts)...)
	parsingFlags = parsingFlags.Add(shared.NewProviderCacheFlags(opts)...)
	parsingFlags = parsingFlags.Add(shared.NewEngineFlags(opts)...)
	parsingFlags = parsingFlags.Add(shared.NewBackendFlags(opts, nil)...)
	parsingFlags = parsingFlags.Add(shared.NewFeatureFlags(opts, nil)...)
	parsingFlags = parsingFlags.Add(shared.NewIAMAssumeRoleFlags(opts, nil)...)
	parsingFlags = parsingFlags.Add(shared.NewCASFlags(opts, nil)...)

	return parsingFlags
}
