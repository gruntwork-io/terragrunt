package main

import (
	"os"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

// The main entrypoint for Terragrunt
func main() {
	defer errors.Recover(checkForErrorsAndExit)

	app := cli.NewApp(os.Stdout, os.Stderr)
	err := app.Run(os.Args)

	checkForErrorsAndExit(err)
}

// If there is an error, display it in the console and exit with a non-zero exit code. Otherwise, exit 0.
func checkForErrorsAndExit(err error) {
	if err == nil {
		os.Exit(0)
	} else {
		util.GlobalFallbackLogEntry.Debugf(errors.PrintErrorWithStackTrace(err))
		util.GlobalFallbackLogEntry.Errorf(err.Error())

		// exit with the underlying error code
		exitCode, exitCodeErr := shell.GetExitCode(err)
		if exitCodeErr != nil {
			exitCode = 1
			util.GlobalFallbackLogEntry.Errorf("Unable to determine underlying exit code, so Terragrunt will exit with error code 1")
		}
		if explain := shell.ExplainError(err); len(explain) > 0 {
			util.GlobalFallbackLogEntry.Errorf("Suggested fixes: \n%s", explain)
		}
		os.Exit(exitCode)
	}

}
