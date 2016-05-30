package main

import (
	"os"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/gruntwork-io/terragrunt/cli"
)

// This variable is set at build time using -ldflags parameters. For more info, see:
// http://stackoverflow.com/a/11355611/483528
var VERSION string

func main() {
	app := cli.CreateTerragruntCli(VERSION)
	err := app.Run(os.Args)

	if err != nil {
		util.Logger.Println(err)
		os.Exit(1)
	}
}