package main

import (
	"context"
	"os"

	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// The main entrypoint for Terragrunt
func main() {
	var exitCode tf.DetailedExitCode

	opts := options.NewTerragruntOptions()
	app := cli.NewApp(opts)

	defer errors.Recover(checkForErrorsAndExit(opts.Logger, exitCode.Get()))

	ctx := setupContext(opts.Logger, &exitCode)
	err := app.RunContext(ctx, os.Args)

	checkForErrorsAndExit(opts.Logger, exitCode.Get())(err)
}

// If there is an error, display it in the console and exit with a non-zero exit code. Otherwise, exit 0.
func checkForErrorsAndExit(logger log.Logger, exitCode int) func(error) {
	return func(err error) {
		if err == nil {
			os.Exit(exitCode)
		} else {
			errStr := err.Error()
			if len(errStr) > 0 {
				// Capitalize the first letter of the error.
				firstLetter := cases.Upper(language.Und).String(string(errStr[0]))
				errStr = string([]rune(errStr)[:0]) + firstLetter + string([]rune(errStr)[1:])
			}

			logger.Error(errStr)

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

func setupContext(logger log.Logger, exitCode *tf.DetailedExitCode) context.Context {
	ctx := context.Background()
	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	return log.ContextWithLogger(ctx, logger)
}
