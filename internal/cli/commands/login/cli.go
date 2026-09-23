// Package login signs the user in to the Gruntwork Developer Portal via the
// `terragrunt login` command, so a later command reaches the catalog their
// organization defined there.
package login

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// CommandName is the name of the login command.
	CommandName = "login"

	// ForceFlagName is the name of the flag that signs the user in again even
	// though a credential from an earlier login has not expired.
	ForceFlagName = "force"
)

// Command is the whole invocation a user is told to run. While tg-login is
// gated it carries the flag, because an experiment switched on by flag is gone
// by the next invocation and the bare command would be turned away. A completed
// experiment is on for good, so the flag drops out on its own.
func Command(exps experiment.Experiments) string {
	if exp := exps.Find(experiment.TGLogin); exp != nil && exp.Status == experiment.StatusCompleted {
		return "terragrunt " + CommandName
	}

	return "terragrunt --experiment " + experiment.TGLogin + " " + CommandName
}

// NewCommand returns the login command.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *clihelper.Command {
	cmdOpts := NewOptions(opts)
	tgPrefix := flags.Prefix{CommandName}.Prepend(flags.TgPrefix)

	return &clihelper.Command{
		Name:  CommandName,
		Usage: "Sign in to the Gruntwork Developer Portal.",
		Flags: clihelper.Flags{
			flags.NewFlag(&clihelper.BoolFlag{
				Name:        ForceFlagName,
				EnvVars:     tgPrefix.EnvVars(ForceFlagName),
				Usage:       "Replace the credential from an existing, unexpired login.",
				Destination: &cmdOpts.Force,
			}),
		},
		Before: func(_ context.Context, _ *clihelper.Context) error {
			if !opts.Experiments.Evaluate(experiment.TGLogin) {
				return clihelper.NewExitError(ErrExperimentRequired, clihelper.ExitCodeGeneralError)
			}

			return nil
		},
		Action: func(ctx context.Context, _ *clihelper.Context) error {
			return Run(ctx, l, v, cmdOpts)
		},
	}
}
