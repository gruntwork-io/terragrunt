// Package mcp provides the `terragrunt mcp` command, which serves Terragrunt
// operations to AI agents over the Model Context Protocol.
//
// The server denies every capability in [allCapabilities] and grants each on
// its own. A denial fails the operation that needed it while the tool call
// still answers, and its result includes a "degraded" list naming what was
// substituted.
//
// Each tool call runs against a venv narrowed by [venv.Restrict]. Exec stays
// bounded once granted: it allows the commands Terragrunt runs itself, plus
// the invocations an --allow-cmd pattern matches. A program a configuration
// names beyond those is refused, and the parse-only tools offer it for
// approval through [runWithCommandApproval] rather than answering without it.
//
// A tool call works against the real filesystem. Terragrunt writes where its
// work requires, such as a dependency's source into a .terragrunt-cache, and
// an HCL function like file() reads anything the invoking user can.
// [resolveWorkingDir] bounds the working_dir argument rather than what the
// parse then reaches for.
//
// A tool call starts from an empty environment unless [venv.CapabilityEnv] is
// granted, in which case it sees a copy of the server's own. The command also
// strips its own process environment the same way before serving, as
// [IsolatedEnv] describes, because go-getter and the cloud SDKs read that
// directly instead of reading them from the venv.
package mcp

import (
	"context"
	"errors"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/providercaching"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/pkg/venv"
)

const (
	// CommandName is the name of the mcp command.
	CommandName = "mcp"

	// AllowFlagName is the name of the flag that grants a capability the
	// server otherwise denies.
	AllowFlagName = "allow"

	// AllowCmdFlagName is the name of the flag that allows a command a
	// configuration runs, matched on its program and arguments.
	AllowCmdFlagName = "allow-cmd"

	// AllowApplyFlagName is the name of the flag that adds the apply and
	// destroy tools, which change real infrastructure.
	AllowApplyFlagName = "dangerously-allow-apply"
)

// NewFlags returns the flags for the mcp command.
func NewFlags(opts *Options, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return clihelper.Flags{
		flags.NewFlag(&clihelper.SliceFlag[string]{
			Name:        AllowFlagName,
			EnvVars:     tgPrefix.EnvVars(AllowFlagName),
			Destination: &opts.Allow,
			Usage:       "Grant a capability the server otherwise denies. Repeatable.",
		}),
		flags.NewFlag(&clihelper.SliceFlag[string]{
			Name:        AllowCmdFlagName,
			EnvVars:     tgPrefix.EnvVars(AllowCmdFlagName),
			Destination: &opts.AllowCmds,
			Usage:       "Allow MCP tools to run a command a configuration names, such as one a run_cmd or a hook calls, given as a pattern like 'jq **'. Repeatable. Requires --allow=exec.",
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        AllowApplyFlagName,
			EnvVars:     tgPrefix.EnvVars(AllowApplyFlagName),
			Destination: &opts.AllowApply,
			Usage:       "Serve apply and destroy tools, which change and tear down real infrastructure. Each run is gated on a person accepting it in your MCP client. Requires --allow=exec.",
		}),
		shared.NewTFPathFlag(opts.TerragruntOptions),
	}
}

// NewCommand returns the mcp command, which serves Terragrunt operations over
// the Model Context Protocol on stdio.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *clihelper.Command {
	cmdOpts := NewOptions(opts)

	return &clihelper.Command{
		Name: CommandName,
		Usage: flags.ExperimentUsage(
			opts.Experiments,
			experiment.MCPCommand,
			"Serve Terragrunt operations to AI agents over the Model Context Protocol (stdio).",
		),
		Flags: NewFlags(cmdOpts, nil),
		Before: func(_ context.Context, _ *clihelper.Context) error {
			if !opts.Experiments.Evaluate(experiment.MCPCommand) {
				return clihelper.NewExitError(ErrExperimentRequired, clihelper.ExitCodeGeneralError)
			}

			granted, err := venv.ParseCapabilities(cmdOpts.Allow, allCapabilities)
			if err != nil {
				return clihelper.NewExitError(err, clihelper.ExitCodeGeneralError)
			}

			if cmdOpts.AllowApply && !granted.Has(venv.CapabilityExec) {
				return clihelper.NewExitError(ErrApplyRequiresExec, clihelper.ExitCodeGeneralError)
			}

			allowCmds, err := venv.ParseAllowCmds(cmdOpts.AllowCmds)
			if err != nil {
				return clihelper.NewExitError(err, clihelper.ExitCodeGeneralError)
			}

			if err = (venv.Policy{Granted: granted, AllowCmds: allowCmds}).Validate(); err != nil {
				return clihelper.NewExitError(err, clihelper.ExitCodeGeneralError)
			}

			// RunAction's auto-provider-cache setup probes a real
			// `tofu version` before the Action runs, so it spawns with the
			// unmodified venv and would hand that subprocess the client's
			// request stream as its stdin.
			opts.NoAutoProviderCacheDir = true

			return nil
		},
		Action: providercaching.Wrap(l, opts, v, func(ctx context.Context, _ *clihelper.Context) error {
			granted, err := venv.ParseCapabilities(cmdOpts.Allow, allCapabilities)
			if err != nil {
				return clihelper.NewExitError(err, clihelper.ExitCodeGeneralError)
			}

			cleanup, err := isolateProcessEnv(v, granted)
			if err != nil {
				return err
			}

			return errors.Join(Run(ctx, l, v, cmdOpts), cleanup())
		}),
	}
}
