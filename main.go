package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// The main entrypoint for Terragrunt
func main() {
	exitCode := tf.NewDetailedExitCodeMap()

	opts := options.NewTerragruntOptions()

	l := log.New(
		log.WithOutput(opts.Writers.ErrWriter),
		log.WithLevel(options.DefaultLogLevel),
		log.WithFormatter(format.NewFormatter(format.NewPrettyFormatPlaceholders())),
	)

	// Immediately parse the `TG_LOG_LEVEL` environment variable, e.g. to set the TRACE level.
	if err := global.NewLogLevelFlag(l, opts, nil).Parse(os.Args); err != nil {
		l.Error(err.Error())
		os.Exit(1)
	}

	defer func() {
		rec := recover()
		if rec == nil {
			return
		}

		err, isErr := rec.(error)
		if !isErr {
			err = fmt.Errorf("%v", rec)
		}

		if opts.TerraformCliArgs.Contains(tf.FlagNameDetailedExitCode) {
			checkForErrorsAndExit(l, exitCode.GetFinalDetailedExitCode())(err)
			return
		}

		checkForErrorsAndExit(l, exitCode.GetFinalExitCode())(err)
	}()

	app := cli.NewApp(l, opts)

	ctx := setupContext(l, exitCode)
	err := app.RunContext(ctx, os.Args)

	if opts.TerraformCliArgs.Contains(tf.FlagNameDetailedExitCode) {
		checkForErrorsAndExit(l, exitCode.GetFinalDetailedExitCode())(err)

		return
	}

	checkForErrorsAndExit(l, exitCode.GetFinalExitCode())(err)
}

// If there is an error, display it in the console and exit with a non-zero exit code. Otherwise, exit 0.
func checkForErrorsAndExit(l log.Logger, exitCode int) func(error) {
	return func(err error) {
		if err == nil {
			os.Exit(exitCode)
		}

		// User declined a destructive run-all prompt. Exit 0 without
		// printing an error message, since they already declined at
		// the prompt.
		if errors.Is(err, runall.ErrUserCancelled) {
			os.Exit(0)
		}

		l.Error(err.Error())

		// exit with the underlying error code
		exitCoder, exitCodeErr := util.GetExitCode(err)
		if exitCodeErr != nil {
			exitCoder = 1
		}

		if explain := shell.ExplainError(err); len(explain) > 0 {
			l.Errorf("Suggested fixes: \n%s", explain)
		}

		os.Exit(exitCoder)
	}
}

func setupContext(l log.Logger, exitCode *tf.DetailedExitCodeMap) context.Context {
	ctx := context.Background()
	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	return log.ContextWithLogger(ctx, l)
}
