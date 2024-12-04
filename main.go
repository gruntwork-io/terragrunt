package main

import (
	"context"
	"os"

	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

// The main entrypoint for Terragrunt
func main() {
	var exitCode shell.DetailedExitCode

	ctx := context.Background()
	ctx = shell.ContextWithDetailedExitCode(ctx, &exitCode)

	opts := options.NewTerragruntOptions()
	parseAndSetLogEnvs(opts)

	defer errors.Recover(checkForErrorsAndExit(opts.Logger, exitCode.Get()))

	app := cli.NewApp(opts)
	err := app.RunContext(ctx, os.Args)

	checkForErrorsAndExit(opts.Logger, exitCode.Get())(err)
}

// If there is an error, display it in the console and exit with a non-zero exit code. Otherwise, exit 0.
func checkForErrorsAndExit(logger log.Logger, exitCode int) func(error) {
	return func(err error) {
		if err == nil {
			os.Exit(exitCode)
		} else {
			logger.Error(err.Error())
			logger.Trace(errors.ErrorStack(err))

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

func parseAndSetLogEnvs(opts *options.TerragruntOptions) {
	if levelStr := os.Getenv(commands.TerragruntLogLevelEnvName); levelStr != "" {
		level, err := log.ParseLevel(levelStr)
		if err != nil {
			err := errors.Errorf("Could not parse log level from environment variable %s=%s, %w", commands.TerragruntLogLevelEnvName, levelStr, err)
			checkForErrorsAndExit(opts.Logger, 0)(err)
		}

		opts.Logger.SetOptions(log.WithLevel(level))
	}
}
