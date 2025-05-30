package main

import (
	"context"
	"os"

	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
)

// The main entrypoint for Terragrunt
func main() {
	var exitCode tf.DetailedExitCode

	opts := options.NewTerragruntOptions()

	l := log.New(
		log.WithOutput(opts.ErrWriter),
		log.WithLevel(options.DefaultLogLevel),
		log.WithFormatter(format.NewFormatter(format.NewPrettyFormatPlaceholders())),
	)

	// Immediately parse the `TG_LOG_LEVEL` environment variable, e.g. to set the TRACE level.
	if err := global.NewLogLevelFlag(l, opts, nil).Parse(os.Args); err != nil {
		l.Error(err.Error())
		os.Exit(1)
	}

	defer errors.Recover(checkForErrorsAndExit(l, exitCode.Get()))

	app := cli.NewApp(l, opts)

	ctx := setupContext(l, &exitCode)
	err := app.RunContext(ctx, os.Args)

	checkForErrorsAndExit(l, exitCode.Get())(err)
}

// If there is an error, display it in the console and exit with a non-zero exit code. Otherwise, exit 0.
func checkForErrorsAndExit(logger log.Logger, exitCode int) func(error) {
	return func(err error) {
		if err == nil {
			os.Exit(exitCode)
		} else {
			logger.Error(err.Error())

			if errStack := errors.ErrorStack(err); errStack != "" {
				logger.Trace(errStack)
			}

			// exit with the underlying error code
			exitCoder, exitCodeErr := util.GetExitCode(err)
			if exitCodeErr != nil {
				exitCoder = 1

				logger.Errorf("Unable to determine underlying exit code, so Terragrunt will exit with error code 1")
			}

			if explain := shell.ExplainError(err); len(explain) > 0 {
				logger.Errorf("Suggested fixes: \n%s", explain)
			}

			os.Exit(exitCoder)
		}
	}
}

func setupContext(l log.Logger, exitCode *tf.DetailedExitCode) context.Context {
	ctx := context.Background()
	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	return log.ContextWithLogger(ctx, l)
}
