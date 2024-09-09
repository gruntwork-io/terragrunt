package main

import (
	goErrors "errors"
	"io"
	"os"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/formats"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-multierror"
)

// The main entrypoint for Terragrunt
func main() {
	logger := newLogger(os.Stderr)

	defer errors.Recover(checkForErrorsAndExit(logger))

	app := cli.NewApp(logger, os.Stdout, os.Stderr)
	err := app.Run(os.Args)

	checkForErrorsAndExit(logger)(err)
}

// If there is an error, display it in the console and exit with a non-zero exit code. Otherwise, exit 0.
func checkForErrorsAndExit(logger log.Logger) func(error) {
	return func(err error) {
		if err == nil {
			os.Exit(0)
		} else {
			logger.Debugf(printErrorWithStackTrace(err))
			logger.Errorf(err.Error())

			// exit with the underlying error code
			exitCode, exitCodeErr := util.GetExitCode(err)
			if exitCodeErr != nil {
				exitCode = 1

				logger.Errorf("Unable to determine underlying exit code, so Terragrunt will exit with error code 1")
			}

			if explain := shell.ExplainError(err); len(explain) > 0 {
				logger.Errorf("Suggested fixes: \n%s", explain)
			}

			os.Exit(exitCode)
		}
	}
}

func printErrorWithStackTrace(err error) string {
	var multierror *multierror.Error

	if goErrors.As(err, &multierror) {
		var errsStr []string
		for _, err := range multierror.Errors {
			errsStr = append(errsStr, errors.PrintErrorWithStackTrace(err))
		}

		return strings.Join(errsStr, "\n")
	}

	return errors.PrintErrorWithStackTrace(err)
}

func newLogger(out io.Writer) log.Logger {
	logger := log.New(log.WithOutput(out), log.WithFormatter(options.DefaultLogFormatter))

	if levelStr := os.Getenv(commands.TerragruntLogLevelEnvName); levelStr != "" {
		level, err := log.ParseLevel(levelStr)
		if err != nil {
			err := errors.Errorf("Could not parse log level from environment variable %s=%s, %w", commands.TerragruntLogLevelEnvName, levelStr, err)
			checkForErrorsAndExit(logger)(err)
		}

		logger.SetOptions(log.WithLevel(level))
	}

	if formatterStr := os.Getenv(commands.TerragruntLogFormatEnvName); formatterStr != "" {
		formatter, err := formats.ParseFormat(formatterStr, formats.PrettyFormatName)
		if err != nil {
			err = errors.Errorf("Could not parse log format from environment variable %s=%s, %w", commands.TerragruntLogFormatEnvName, formatterStr, err)
			checkForErrorsAndExit(logger)(err)
		}

		logger.SetOptions(log.WithFormatter(formatter))
	}

	return logger
}
