package main

import (
	"os"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/errors"
)

// This variable is set at build time using -ldflags parameters. For more info, see:
// http://stackoverflow.com/a/11355611/483528
var VERSION string

// The main entrypoint for Terragrunt
func main() {
	app := cli.CreateTerragruntCli(VERSION)
	err := app.Run(os.Args)

	if err != nil {
		printError(err)
		os.Exit(1)
	}
}

// Display the given error in the console
func printError(err error) {
	if os.Getenv("TERRAGRUNT_DEBUG") != "" {
		util.Logger.Println(errors.PrintErrorWithStackTrace(err))
	} else {
		util.Logger.Println(err)
	}
}