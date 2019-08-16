package main

import (
	"os"

	"github.com/fatih/color"

	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

// This variable is set at build time using -ldflags parameters. For more info, see:
// http://stackoverflow.com/a/11355611/483528
var VERSION string

// The main entrypoint for Terragrunt
func main() {
	defer errors.Recover(checkForErrorsAndExit)

	app := cli.CreateTerragruntCli(VERSION, os.Stdout, os.Stderr)
	err := app.Run(os.Args)

	checkForErrorsAndExit(err)
}

// If there is an error, display it in the console and exit with a non-zero exit code. Otherwise, exit 0.
func checkForErrorsAndExit(err error) {
	if err == nil {
		os.Exit(0)
	} else {
		logger := util.CreateLogger("")
		if os.Getenv("TERRAGRUNT_DEBUG") != "" {
			logger.Println(errors.PrintErrorWithStackTrace(err))
		} else {
			// Log error in red so that it is highlighted
			util.ColorLogf(logger, color.New(color.FgRed), err.Error())
		}
		// exit with the underlying error code
		exitCode, exitCodeErr := shell.GetExitCode(err)
		if exitCodeErr != nil {
			exitCode = 1
			logger.Println("Unable to determine underlying exit code, so Terragrunt will exit with error code 1")
		}
		os.Exit(exitCode)
	}

}
