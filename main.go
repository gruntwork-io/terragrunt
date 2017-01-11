package main

import (
	"os"
	"os/exec"
	"syscall"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/errors"
)

// This variable is set at build time using -ldflags parameters. For more info, see:
// http://stackoverflow.com/a/11355611/483528
var VERSION string

// The main entrypoint for Terragrunt
func main() {
	defer errors.Recover(checkForErrorsAndExit)

	app := cli.CreateTerragruntCli(VERSION)
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
			logger.Println(err)
		}
		// exit with the underlying error code
		var retCode int = 1
		if exiterr, ok := errors.Unwrap(err).(*exec.ExitError); ok {
			status := exiterr.Sys().(syscall.WaitStatus)
			retCode = status.ExitStatus()
		}
		os.Exit(retCode)
	}

}